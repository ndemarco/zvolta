package snapshot

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/ndemarco/zvolta/internal/zfs"
)

// Manager handles snapshot creation, listing, and pruning.
type Manager struct {
	ZFS    *zfs.Client
	Prefix string
	Logger *slog.Logger
}

// Create takes a snapshot of the given dataset for the specified tier.
func (m *Manager) Create(dataset string, tier Tier, t time.Time) error {
	name := FormatName(m.Prefix, tier, t)
	m.Logger.Info("creating snapshot", "dataset", dataset, "name", name, "tier", string(tier))
	if err := m.ZFS.CreateSnapshot(dataset, name); err != nil {
		return fmt.Errorf("creating snapshot %s@%s: %w", dataset, name, err)
	}
	return nil
}

// ListManaged returns all Zvolta-managed snapshots for a dataset, grouped by tier.
func (m *Manager) ListManaged(dataset string) (map[Tier][]ManagedSnapshot, error) {
	snaps, err := m.ZFS.ListSnapshots(dataset)
	if err != nil {
		return nil, fmt.Errorf("listing snapshots for %s: %w", dataset, err)
	}

	result := make(map[Tier][]ManagedSnapshot)
	for _, s := range snaps {
		parsed, err := ParseName(s.SnapName, m.Prefix)
		if err != nil {
			// Not a Zvolta snapshot — skip
			continue
		}
		ms := ManagedSnapshot{
			Snapshot: s,
			Parsed:   parsed,
		}
		result[parsed.Tier] = append(result[parsed.Tier], ms)
	}

	// Sort each tier by timestamp (oldest first)
	for tier := range result {
		sort.Slice(result[tier], func(i, j int) bool {
			return result[tier][i].Parsed.Timestamp.Before(result[tier][j].Parsed.Timestamp)
		})
	}

	return result, nil
}

// LastSnapshotTimes returns the most recent snapshot time per tier for a dataset.
func (m *Manager) LastSnapshotTimes(dataset string) (map[Tier]time.Time, error) {
	grouped, err := m.ListManaged(dataset)
	if err != nil {
		return nil, err
	}
	result := make(map[Tier]time.Time)
	for tier, snaps := range grouped {
		if len(snaps) > 0 {
			result[tier] = snaps[len(snaps)-1].Parsed.Timestamp
		}
	}
	return result, nil
}

// Prune removes snapshots beyond the retention count for a tier. Keeps the
// newest `keep` snapshots, destroys the rest. Returns the names of destroyed
// snapshots. If dryRun is true, logs but does not destroy.
func (m *Manager) Prune(dataset string, tier Tier, keep int, dryRun bool) ([]string, error) {
	grouped, err := m.ListManaged(dataset)
	if err != nil {
		return nil, err
	}

	snaps := grouped[tier]
	if len(snaps) <= keep {
		return nil, nil
	}

	// Sorted oldest first — remove from the front
	toRemove := snaps[:len(snaps)-keep]
	var removed []string

	for _, s := range toRemove {
		if dryRun {
			m.Logger.Info("would prune", "snapshot", s.Snapshot.Name, "tier", string(tier))
		} else {
			m.Logger.Info("pruning", "snapshot", s.Snapshot.Name, "tier", string(tier))
			if err := m.ZFS.DestroySnapshot(s.Snapshot.Dataset, s.Snapshot.SnapName); err != nil {
				return removed, fmt.Errorf("destroying %s: %w", s.Snapshot.Name, err)
			}
		}
		removed = append(removed, s.Snapshot.Name)
	}

	return removed, nil
}

// PruneAll prunes all tiers for a dataset according to the given retention map.
func (m *Manager) PruneAll(dataset string, retention map[Tier]int, dryRun bool) error {
	for _, tier := range AllTiers() {
		keep, ok := retention[tier]
		if !ok || keep <= 0 {
			continue
		}
		if _, err := m.Prune(dataset, tier, keep, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// ManagedSnapshot combines ZFS snapshot info with parsed Zvolta name components.
type ManagedSnapshot struct {
	Snapshot zfs.Snapshot
	Parsed   ParsedName
}
