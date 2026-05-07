//go:build integration

package snapshot

import (
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ndemarco/zvolta/internal/zfs"
)

const testPool = "zvolta-test"
const testDataset = "zvolta-test/data"

func requirePool(t *testing.T, pool string) {
	t.Helper()
	if err := exec.Command("zfs", "list", pool).Run(); err != nil {
		t.Skipf("ZFS pool %q not available (run scripts/test-pool.sh to create it): %v", pool, err)
	}
}

func testManager() *Manager {
	return &Manager{
		ZFS:    zfs.NewClient(""),
		Prefix: "zvolta_",
		Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

// cleanSnapshots destroys all zvolta-managed snapshots on the test dataset.
func cleanSnapshots(t *testing.T, mgr *Manager) {
	t.Helper()
	grouped, _ := mgr.ListManaged(testDataset)
	for _, snaps := range grouped {
		for _, s := range snaps {
			_ = mgr.ZFS.DestroySnapshot(s.Snapshot.Dataset, s.Snapshot.SnapName)
		}
	}
}

func TestIntegrationCreateAndList(t *testing.T) {
	requirePool(t, testPool)
	mgr := testManager()
	cleanSnapshots(t, mgr)
	defer cleanSnapshots(t, mgr)

	now := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	tiers := []Tier{TierHourly, TierDaily}
	for _, tier := range tiers {
		if err := mgr.Create(testDataset, tier, now); err != nil {
			t.Fatalf("Create %s: %v", tier, err)
		}
	}

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
}

func TestIntegrationPrune(t *testing.T) {
	requirePool(t, testPool)
	mgr := testManager()
	cleanSnapshots(t, mgr)
	defer cleanSnapshots(t, mgr)

	// Use unique timestamps that don't overlap with other tests
	base := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := mgr.Create(testDataset, TierHourly, ts); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	grouped, err := mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(grouped[TierHourly]) != 5 {
		t.Fatalf("expected 5 hourly snapshots, got %d", len(grouped[TierHourly]))
	}

	removed, err := mgr.Prune(testDataset, TierHourly, 2, false)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 3 {
		t.Errorf("expected 3 removed, got %d", len(removed))
	}

	grouped, err = mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged after prune: %v", err)
	}
	if len(grouped[TierHourly]) != 2 {
		t.Errorf("expected 2 hourly after prune, got %d", len(grouped[TierHourly]))
	}

	// Verify the remaining are the newest two (13:00 and 14:00 UTC)
	for _, s := range grouped[TierHourly] {
		if s.Parsed.Timestamp.Hour() < 13 {
			t.Errorf("expected only newest to remain, got %v", s.Parsed.Timestamp)
		}
	}
}

func TestIntegrationDryRun(t *testing.T) {
	requirePool(t, testPool)
	mgr := testManager()
	cleanSnapshots(t, mgr)
	defer cleanSnapshots(t, mgr)

	base := time.Date(2025, 3, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := mgr.Create(testDataset, TierDaily, ts); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	removed, err := mgr.Prune(testDataset, TierDaily, 1, true)
	if err != nil {
		t.Fatalf("Prune dry-run: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("dry-run should report 2 removals, got %d", len(removed))
	}

	grouped, err := mgr.ListManaged(testDataset)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(grouped[TierDaily]) != 3 {
		t.Errorf("dry-run should not delete, expected 3, got %d", len(grouped[TierDaily]))
	}
}

func TestIntegrationLastSnapshotTimes(t *testing.T) {
	requirePool(t, testPool)
	mgr := testManager()
	cleanSnapshots(t, mgr)
	defer cleanSnapshots(t, mgr)

	ts1 := time.Date(2025, 4, 1, 8, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 4, 1, 9, 0, 0, 0, time.UTC)

	if err := mgr.Create(testDataset, TierHourly, ts1); err != nil {
		t.Fatalf("Create hourly ts1: %v", err)
	}
	if err := mgr.Create(testDataset, TierHourly, ts2); err != nil {
		t.Fatalf("Create hourly ts2: %v", err)
	}
	if err := mgr.Create(testDataset, TierDaily, ts1); err != nil {
		t.Fatalf("Create daily ts1: %v", err)
	}

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
}
