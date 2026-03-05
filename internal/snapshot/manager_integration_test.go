//go:build integration

package snapshot

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ndemarco/zvolta/internal/zfs"
)

const testDataset = "zvolta-test/data"

func testManager() *Manager {
	return &Manager{
		ZFS:    zfs.NewClient(""),
		Prefix: "zvolta_",
		Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func TestIntegrationCreateAndList(t *testing.T) {
	mgr := testManager()
	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)

	// Create snapshots across multiple tiers
	tiers := []Tier{TierHourly, TierDaily}
	for _, tier := range tiers {
		if err := mgr.Create(testDataset, tier, now); err != nil {
			t.Fatalf("Create %s: %v", tier, err)
		}
	}

	// List and verify
	grouped, err := mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}

	for _, tier := range tiers {
		snaps, ok := grouped[tier]
		if !ok || len(snaps) == 0 {
			t.Errorf("no snapshots found for tier %s", tier)
			continue
		}
		found := false
		for _, s := range snaps {
			if s.Parsed.Tier == tier && s.Parsed.Timestamp.Equal(now) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected snapshot for tier %s at %v", tier, now)
		}
	}

	// Clean up
	for _, tier := range tiers {
		name := FormatName("zvolta_", tier, now)
		_ = mgr.ZFS.DestroySnapshot(testDataset, name)
	}
}

func TestIntegrationPrune(t *testing.T) {
	mgr := testManager()

	// Create 5 hourly snapshots
	base := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := mgr.Create(testDataset, TierHourly, ts); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Verify 5 exist
	grouped, err := mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(grouped[TierHourly]) != 5 {
		t.Fatalf("expected 5 hourly snapshots, got %d", len(grouped[TierHourly]))
	}

	// Prune to keep 2
	removed, err := mgr.Prune(testDataset, TierHourly, 2, false)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 3 {
		t.Errorf("expected 3 removed, got %d", len(removed))
	}

	// Verify 2 remain (the newest)
	grouped, err = mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged after prune: %v", err)
	}
	if len(grouped[TierHourly]) != 2 {
		t.Errorf("expected 2 hourly after prune, got %d", len(grouped[TierHourly]))
	}

	// Verify the remaining are the newest two (13:00 and 14:00)
	for _, s := range grouped[TierHourly] {
		if s.Parsed.Timestamp.Hour() < 13 {
			t.Errorf("expected only 13:00 and 14:00 to remain, got %v", s.Parsed.Timestamp)
		}
	}

	// Clean up remaining
	for _, s := range grouped[TierHourly] {
		_ = mgr.ZFS.DestroySnapshot(testDataset, s.Snapshot.SnapName)
	}
}

func TestIntegrationDryRun(t *testing.T) {
	mgr := testManager()

	// Create 3 snapshots
	base := time.Date(2026, 3, 5, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := mgr.Create(testDataset, TierDaily, ts); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Dry-run prune to keep 1
	removed, err := mgr.Prune(testDataset, TierDaily, 1, true)
	if err != nil {
		t.Fatalf("Prune dry-run: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("dry-run should report 2 removals, got %d", len(removed))
	}

	// Verify all 3 still exist (dry-run shouldn't delete)
	grouped, err := mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(grouped[TierDaily]) != 3 {
		t.Errorf("dry-run should not delete, expected 3, got %d", len(grouped[TierDaily]))
	}

	// Clean up
	for _, s := range grouped[TierDaily] {
		_ = mgr.ZFS.DestroySnapshot(testDataset, s.Snapshot.SnapName)
	}
}

func TestIntegrationLastSnapshotTimes(t *testing.T) {
	mgr := testManager()

	ts1 := time.Date(2026, 3, 5, 8, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC)

	_ = mgr.Create(testDataset, TierHourly, ts1)
	_ = mgr.Create(testDataset, TierHourly, ts2)
	_ = mgr.Create(testDataset, TierDaily, ts1)

	times, err := mgr.LastSnapshotTimes(testDataset)
	if err != nil {
		t.Fatalf("LastSnapshotTimes: %v", err)
	}

	if !times[TierHourly].Equal(ts2) {
		t.Errorf("hourly last = %v, want %v", times[TierHourly], ts2)
	}
	if !times[TierDaily].Equal(ts1) {
		t.Errorf("daily last = %v, want %v", times[TierDaily], ts1)
	}

	// Clean up
	_ = mgr.ZFS.DestroySnapshot(testDataset, FormatName("zvolta_", TierHourly, ts1))
	_ = mgr.ZFS.DestroySnapshot(testDataset, FormatName("zvolta_", TierHourly, ts2))
	_ = mgr.ZFS.DestroySnapshot(testDataset, FormatName("zvolta_", TierDaily, ts1))
}
