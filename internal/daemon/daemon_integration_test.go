//go:build integration

package daemon

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ndemarco/zvolta/internal/config"
	"github.com/ndemarco/zvolta/internal/snapshot"
	"github.com/ndemarco/zvolta/internal/zfs"
)

const testPool = "zvolta-test"
const testDataset = "zvolta-test/data"

func setupDaemon(t *testing.T) *Daemon {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Defaults()
	cfg.Datasets = []string{testPool}
	return New(cfg, logger)
}

func setProperty(t *testing.T, client *zfs.Client, dataset, prop, val string) {
	t.Helper()
	if err := client.SetProperty(dataset, prop, val); err != nil {
		t.Fatalf("SetProperty %s %s=%s: %v", dataset, prop, val, err)
	}
}

func clearAllProps(t *testing.T, client *zfs.Client) {
	t.Helper()
	for _, ds := range []string{testPool, testDataset} {
		for _, prop := range []string{
			"org.zvolta:autosnap", "org.zvolta:autoprune",
			"org.zvolta:snapshot-hourly", "org.zvolta:snapshot-daily",
			"org.zvolta:snapshot-frequent", "org.zvolta:snapshot-frequent-period",
			"org.zvolta:snapshot-weekly", "org.zvolta:snapshot-monthly",
			"org.zvolta:snapshot-yearly",
		} {
			_ = client.InheritProperty(ds, prop)
		}
	}
}

func destroyAllTestSnapshots(t *testing.T, mgr *snapshot.Manager, dataset string) {
	t.Helper()
	grouped, _ := mgr.ListManaged(dataset)
	for _, snaps := range grouped {
		for _, s := range snaps {
			_ = mgr.ZFS.DestroySnapshot(s.Snapshot.Dataset, s.Snapshot.SnapName)
		}
	}
}

func cleanAll(t *testing.T, d *Daemon) {
	t.Helper()
	clearAllProps(t, d.ZFS)
	destroyAllTestSnapshots(t, d.Snapshots, testDataset)
	destroyAllTestSnapshots(t, d.Snapshots, testPool)
}

func TestIntegrationTickCreatesSnapshots(t *testing.T) {
	d := setupDaemon(t)
	cleanAll(t, d)
	defer cleanAll(t, d)

	// Enable autosnap with hourly and daily on the dataset
	setProperty(t, d.ZFS, testDataset, "org.zvolta:autosnap", "on")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-hourly", "24")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-daily", "30")

	// Tick at a specific time — fresh start, so both tiers should fire
	now := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := d.tick(now); err != nil {
		t.Fatalf("tick: %v", err)
	}

	// Verify snapshots were created
	grouped, err := d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}

	if len(grouped[snapshot.TierHourly]) != 1 {
		t.Errorf("expected 1 hourly snapshot, got %d", len(grouped[snapshot.TierHourly]))
	}
	if len(grouped[snapshot.TierDaily]) != 1 {
		t.Errorf("expected 1 daily snapshot, got %d", len(grouped[snapshot.TierDaily]))
	}
}

func TestIntegrationTickIdempotentWithinBoundary(t *testing.T) {
	d := setupDaemon(t)
	cleanAll(t, d)
	defer cleanAll(t, d)

	setProperty(t, d.ZFS, testDataset, "org.zvolta:autosnap", "on")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-hourly", "24")

	// First tick at 14:30
	now := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := d.tick(now); err != nil {
		t.Fatalf("tick 1: %v", err)
	}

	// Second tick at 14:31 — same hour, should NOT create another hourly
	now2 := time.Date(2026, 6, 15, 14, 31, 0, 0, time.UTC)
	if err := d.tick(now2); err != nil {
		t.Fatalf("tick 2: %v", err)
	}

	grouped, err := d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}

	if len(grouped[snapshot.TierHourly]) != 1 {
		t.Errorf("expected still 1 hourly snapshot after second tick in same hour, got %d",
			len(grouped[snapshot.TierHourly]))
	}
}

func TestIntegrationTickCrossesHourBoundary(t *testing.T) {
	d := setupDaemon(t)
	cleanAll(t, d)
	defer cleanAll(t, d)

	setProperty(t, d.ZFS, testDataset, "org.zvolta:autosnap", "on")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-hourly", "24")

	// Tick at 14:30
	if err := d.tick(time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick 1: %v", err)
	}

	// Tick at 15:01 — new hour boundary crossed
	if err := d.tick(time.Date(2026, 6, 15, 15, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick 2: %v", err)
	}

	grouped, err := d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}

	if len(grouped[snapshot.TierHourly]) != 2 {
		t.Errorf("expected 2 hourly snapshots after crossing hour boundary, got %d",
			len(grouped[snapshot.TierHourly]))
	}
}

func TestIntegrationTickPrunes(t *testing.T) {
	d := setupDaemon(t)
	cleanAll(t, d)
	defer cleanAll(t, d)

	setProperty(t, d.ZFS, testDataset, "org.zvolta:autosnap", "on")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:autoprune", "on")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-hourly", "2") // keep only 2

	// Simulate 4 hours of ticks
	base := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		now := base.Add(time.Duration(i) * time.Hour)
		if err := d.tick(now); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}

	// Should have exactly 2 (the retention count)
	grouped, err := d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}

	if len(grouped[snapshot.TierHourly]) != 2 {
		t.Errorf("expected 2 hourly snapshots (retention=2), got %d",
			len(grouped[snapshot.TierHourly]))
	}

	// Verify they're the newest two (12:00 and 13:00, not 10:00 and 11:00)
	for _, s := range grouped[snapshot.TierHourly] {
		if s.Parsed.Timestamp.Hour() < 12 {
			t.Errorf("expected only newest snapshots to survive, got %v", s.Parsed.Timestamp)
		}
	}
}

func TestIntegrationTickFrequent(t *testing.T) {
	d := setupDaemon(t)
	cleanAll(t, d)
	defer cleanAll(t, d)

	setProperty(t, d.ZFS, testDataset, "org.zvolta:autosnap", "on")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-frequent", "12")
	setProperty(t, d.ZFS, testDataset, "org.zvolta:snapshot-frequent-period", "5")

	// Tick at 14:00 — should create frequent
	if err := d.tick(time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick at 14:00: %v", err)
	}

	// Tick at 14:03 — not yet at next 5-min boundary
	if err := d.tick(time.Date(2026, 6, 15, 14, 3, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick at 14:03: %v", err)
	}

	grouped, err := d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged after 14:03: %v", err)
	}
	if len(grouped[snapshot.TierFrequent]) != 1 {
		t.Errorf("expected 1 frequent at 14:03 (boundary not crossed), got %d",
			len(grouped[snapshot.TierFrequent]))
	}

	// Tick at 14:05 — next boundary, should create another
	if err := d.tick(time.Date(2026, 6, 15, 14, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick at 14:05: %v", err)
	}

	grouped, err = d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged after 14:05: %v", err)
	}
	if len(grouped[snapshot.TierFrequent]) != 2 {
		t.Errorf("expected 2 frequent after 14:05 boundary, got %d",
			len(grouped[snapshot.TierFrequent]))
	}
}

func TestIntegrationTickDisabledDataset(t *testing.T) {
	d := setupDaemon(t)
	cleanAll(t, d)
	defer cleanAll(t, d)

	// Don't set autosnap — should be disabled by default
	now := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := d.tick(now); err != nil {
		t.Fatalf("tick: %v", err)
	}

	grouped, err := d.Snapshots.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}

	total := 0
	for _, snaps := range grouped {
		total += len(snaps)
	}
	if total != 0 {
		t.Errorf("expected 0 snapshots on disabled dataset, got %d", total)
	}
}
