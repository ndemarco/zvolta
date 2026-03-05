package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ndemarco/zvolta/internal/config"
	"github.com/ndemarco/zvolta/internal/daemon"
	"github.com/ndemarco/zvolta/internal/policy"
	"github.com/ndemarco/zvolta/internal/snapshot"
	"github.com/ndemarco/zvolta/internal/zfs"
	"github.com/spf13/cobra"
)

var (
	version    = "dev"
	configFile string
)

func main() {
	root := &cobra.Command{
		Use:   "zvolta",
		Short: "ZFS snapshot lifecycle management",
		Long:  "Zvolta manages ZFS snapshots with per-dataset policies stored as ZFS custom properties.",
	}

	root.PersistentFlags().StringVarP(&configFile, "config", "c", "/etc/zvolta/zvolta.toml", "path to server config file")

	root.AddCommand(daemonCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(snapCmd())
	root.AddCommand(listCmd())
	root.AddCommand(pruneCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadConfig() (config.Config, error) {
	return config.Load(configFile)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

func daemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the snapshot management daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			logger := newLogger(cfg.LogLevel)
			d := daemon.New(cfg, logger)
			return d.Run(context.Background())
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show managed datasets and their policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			client := zfs.NewClient(cfg.ZFSBinary)
			engine := &policy.Engine{ZFS: client}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "DATASET\tAUTOSNAP\tAUTOPRUNE\tFREQ\tHOURLY\tDAILY\tWEEKLY\tMONTHLY\tYEARLY")

			for _, root := range cfg.Datasets {
				policies, err := engine.ResolveAll(root)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error scanning %s: %v\n", root, err)
					continue
				}
				for _, p := range policies {
					freq := "-"
					if p.FrequentCount > 0 {
						freq = fmt.Sprintf("%d@%dm", p.FrequentCount, p.FrequentPeriod)
					}
					fmt.Fprintf(w, "%s\t%v\t%v\t%s\t%d\t%d\t%d\t%d\t%d\n",
						p.Dataset, p.AutoSnap, p.AutoPrune,
						freq, p.HourlyCount, p.DailyCount,
						p.WeeklyCount, p.MonthlyCount, p.YearlyCount)
				}
			}
			return w.Flush()
		},
	}
}

func snapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snap <dataset>",
		Short: "Manually trigger a snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			tierStr, _ := cmd.Flags().GetString("tier")
			tier, err := snapshot.ParseTier(tierStr)
			if err != nil {
				return err
			}
			logger := newLogger(cfg.LogLevel)
			mgr := &snapshot.Manager{
				ZFS:    zfs.NewClient(cfg.ZFSBinary),
				Prefix: cfg.SnapshotPrefix,
				Logger: logger,
			}
			return mgr.Create(args[0], tier, time.Now())
		},
	}
	cmd.Flags().String("tier", "hourly", "snapshot tier (frequent, hourly, daily, weekly, monthly, yearly)")
	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <dataset>",
		Short: "List snapshots for a dataset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			mgr := &snapshot.Manager{
				ZFS:    zfs.NewClient(cfg.ZFSBinary),
				Prefix: cfg.SnapshotPrefix,
				Logger: newLogger(cfg.LogLevel),
			}
			grouped, err := mgr.ListManaged(args[0])
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "TIER\tNAME\tTIMESTAMP\tAGE")
			now := time.Now()

			for _, tier := range snapshot.AllTiers() {
				snaps := grouped[tier]
				for _, s := range snaps {
					age := now.Sub(s.Parsed.Timestamp).Truncate(time.Minute)
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
						tier, s.Snapshot.SnapName,
						s.Parsed.Timestamp.Format(time.RFC3339),
						age.String())
				}
			}
			return w.Flush()
		},
	}
}

func pruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune <dataset>",
		Short: "Manually trigger pruning for a dataset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			logger := newLogger(cfg.LogLevel)
			client := zfs.NewClient(cfg.ZFSBinary)

			engine := &policy.Engine{ZFS: client}
			p, err := engine.Resolve(args[0])
			if err != nil {
				return err
			}

			mgr := &snapshot.Manager{
				ZFS:    client,
				Prefix: cfg.SnapshotPrefix,
				Logger: logger,
			}

			retention := make(map[snapshot.Tier]int)
			for _, tier := range snapshot.AllTiers() {
				if count := p.RetentionFor(tier); count > 0 {
					retention[tier] = count
				}
			}
			return mgr.PruneAll(args[0], retention, dryRun)
		},
	}
	cmd.Flags().Bool("dry-run", false, "show what would be pruned without deleting")
	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("zvolta %s\n", version)
		},
	}
}
