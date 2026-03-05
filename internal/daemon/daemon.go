package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ndemarco/zvolta/internal/config"
	"github.com/ndemarco/zvolta/internal/policy"
	"github.com/ndemarco/zvolta/internal/scheduler"
	"github.com/ndemarco/zvolta/internal/snapshot"
	"github.com/ndemarco/zvolta/internal/zfs"
)

// Daemon is the main snapshot management daemon.
type Daemon struct {
	Config    config.Config
	ZFS       *zfs.Client
	Policy    *policy.Engine
	Scheduler *scheduler.Schedule
	Snapshots *snapshot.Manager
	Logger    *slog.Logger
}

// New creates a Daemon from a loaded config.
func New(cfg config.Config, logger *slog.Logger) *Daemon {
	client := zfs.NewClient(cfg.ZFSBinary)
	return &Daemon{
		Config:    cfg,
		ZFS:       client,
		Policy:    &policy.Engine{ZFS: client},
		Scheduler: &scheduler.Schedule{Offset: cfg.ScheduleOffset.Duration},
		Snapshots: &snapshot.Manager{
			ZFS:    client,
			Prefix: cfg.SnapshotPrefix,
			Logger: logger,
		},
		Logger: logger,
	}
}

const lockDir = "/var/run/zvolta"
const lockFile = "zvolta.pid"

// acquireLock writes a PID file and returns a cleanup function. Returns an
// error if another instance is running.
func acquireLock() (func(), error) {
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	path := filepath.Join(lockDir, lockFile)

	// Check for existing lock
	if data, err := os.ReadFile(path); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil {
			// Check if process is still running
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return nil, fmt.Errorf("another zvolta instance is running (pid %d)", pid)
				}
			}
		}
	}

	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644); err != nil {
		return nil, fmt.Errorf("writing lock file: %w", err)
	}

	return func() { os.Remove(path) }, nil
}

// Run starts the daemon main loop. It blocks until the context is cancelled
// or a termination signal is received.
func (d *Daemon) Run(ctx context.Context) error {
	unlock, err := acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	d.Logger.Info("zvolta daemon starting",
		"datasets", d.Config.Datasets,
		"prefix", d.Config.SnapshotPrefix,
		"offset", d.Config.ScheduleOffset.Duration.String(),
	)

	// Run immediately on start, then tick every minute
	if err := d.tick(time.Now()); err != nil {
		d.Logger.Error("tick failed", "error", err)
	}

	for {
		nextTick := scheduler.NextTick(time.Now())
		timer := time.NewTimer(time.Until(nextTick))

		select {
		case <-ctx.Done():
			timer.Stop()
			d.Logger.Info("daemon shutting down")
			return nil

		case sig := <-sigCh:
			timer.Stop()
			switch sig {
			case syscall.SIGHUP:
				d.Logger.Info("received SIGHUP, reloading config not yet implemented")
			case syscall.SIGTERM, syscall.SIGINT:
				d.Logger.Info("received shutdown signal", "signal", sig.String())
				return nil
			}

		case now := <-timer.C:
			if err := d.tick(now); err != nil {
				d.Logger.Error("tick failed", "error", err)
			}
		}
	}
}

// tick runs one cycle: resolve policies, create due snapshots, prune.
func (d *Daemon) tick(now time.Time) error {
	for _, root := range d.Config.Datasets {
		policies, err := d.Policy.ResolveAll(root)
		if err != nil {
			d.Logger.Error("failed to resolve policies", "root", root, "error", err)
			continue
		}

		for _, p := range policies {
			if err := d.processDataset(&p, now); err != nil {
				d.Logger.Error("failed to process dataset", "dataset", p.Dataset, "error", err)
			}
		}
	}
	return nil
}

func (d *Daemon) processDataset(p *policy.Policy, now time.Time) error {
	lastSnaps, err := d.Snapshots.LastSnapshotTimes(p.Dataset)
	if err != nil {
		return fmt.Errorf("getting last snapshot times: %w", err)
	}

	dueTiers := d.Scheduler.Due(p, now, lastSnaps)

	for _, tier := range dueTiers {
		if err := d.Snapshots.Create(p.Dataset, tier, now); err != nil {
			d.Logger.Error("snapshot creation failed",
				"dataset", p.Dataset,
				"tier", string(tier),
				"error", err,
			)
			continue
		}
	}

	// Prune if autoprune is enabled
	if p.AutoPrune {
		retention := make(map[snapshot.Tier]int)
		for _, tier := range snapshot.AllTiers() {
			if count := p.RetentionFor(tier); count > 0 {
				retention[tier] = count
			}
		}
		if err := d.Snapshots.PruneAll(p.Dataset, retention, false); err != nil {
			return fmt.Errorf("pruning %s: %w", p.Dataset, err)
		}
	}

	return nil
}
