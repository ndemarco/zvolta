//go:build integration

package policy

import (
	"testing"

	"github.com/ndemarco/zvolta/internal/snapshot"
	"github.com/ndemarco/zvolta/internal/zfs"
)

const testPool = "zvolta-test"
const testDataset = "zvolta-test/data"

func testEngine() *Engine {
	return &Engine{ZFS: zfs.NewClient("")}
}

func setProps(t *testing.T, client *zfs.Client, dataset string, props map[string]string) {
	t.Helper()
	for k, v := range props {
		if err := client.SetProperty(dataset, k, v); err != nil {
			t.Fatalf("SetProperty %s=%s: %v", k, v, err)
		}
	}
}

func clearProps(t *testing.T, client *zfs.Client, dataset string) {
	t.Helper()
	for _, prop := range AllProperties() {
		_ = client.SetProperty(dataset, prop, "-")
	}
}

func TestIntegrationResolve(t *testing.T) {
	engine := testEngine()
	client := engine.ZFS

	setProps(t, client, testDataset, map[string]string{
		"org.zvolta:autosnap":                 "on",
		"org.zvolta:autoprune":                "on",
		"org.zvolta:snapshot-hourly":          "24",
		"org.zvolta:snapshot-daily":           "30",
		"org.zvolta:snapshot-frequent":        "12",
		"org.zvolta:snapshot-frequent-period": "5",
	})
	defer clearProps(t, client, testDataset)

	p, err := engine.Resolve(testDataset)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if !p.AutoSnap {
		t.Error("autosnap should be true")
	}
	if !p.AutoPrune {
		t.Error("autoprune should be true")
	}
	if p.HourlyCount != 24 {
		t.Errorf("hourly = %d, want 24", p.HourlyCount)
	}
	if p.DailyCount != 30 {
		t.Errorf("daily = %d, want 30", p.DailyCount)
	}
	if p.FrequentCount != 12 {
		t.Errorf("frequent = %d, want 12", p.FrequentCount)
	}
	if p.FrequentPeriod != 5 {
		t.Errorf("frequent period = %d, want 5", p.FrequentPeriod)
	}
	if !p.IsEnabled(snapshot.TierHourly) {
		t.Error("hourly should be enabled")
	}
}

func TestIntegrationInheritance(t *testing.T) {
	engine := testEngine()
	client := engine.ZFS

	// Set on parent pool
	setProps(t, client, testPool, map[string]string{
		"org.zvolta:autosnap":        "on",
		"org.zvolta:snapshot-hourly": "48",
	})
	defer clearProps(t, client, testPool)
	defer clearProps(t, client, testDataset)

	// Child should inherit
	p, err := engine.Resolve(testDataset)
	if err != nil {
		t.Fatalf("Resolve child: %v", err)
	}
	if !p.AutoSnap {
		t.Error("child should inherit autosnap=on")
	}
	if p.HourlyCount != 48 {
		t.Errorf("child hourly = %d, want 48 (inherited)", p.HourlyCount)
	}

	// Override on child
	setProps(t, client, testDataset, map[string]string{
		"org.zvolta:snapshot-hourly": "12",
	})

	p, err = engine.Resolve(testDataset)
	if err != nil {
		t.Fatalf("Resolve child after override: %v", err)
	}
	if p.HourlyCount != 12 {
		t.Errorf("child hourly = %d, want 12 (overridden)", p.HourlyCount)
	}
}

func TestIntegrationResolveAll(t *testing.T) {
	engine := testEngine()
	client := engine.ZFS

	setProps(t, client, testPool, map[string]string{
		"org.zvolta:autosnap":        "on",
		"org.zvolta:snapshot-hourly": "24",
	})
	defer clearProps(t, client, testPool)
	defer clearProps(t, client, testDataset)

	policies, err := engine.ResolveAll(testPool)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	// Should find at least the pool and child dataset
	if len(policies) < 2 {
		t.Errorf("expected at least 2 policies, got %d", len(policies))
	}

	found := false
	for _, p := range policies {
		if p.Dataset == testDataset {
			found = true
			if p.HourlyCount != 24 {
				t.Errorf("child hourly = %d, want 24", p.HourlyCount)
			}
		}
	}
	if !found {
		t.Errorf("child dataset %s not found in ResolveAll results", testDataset)
	}
}

func TestIntegrationDefaultDisabled(t *testing.T) {
	engine := testEngine()
	client := engine.ZFS

	// Clear everything — should default to autosnap=off
	clearProps(t, client, testDataset)
	clearProps(t, client, testPool)

	p, err := engine.Resolve(testDataset)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if p.AutoSnap {
		t.Error("autosnap should default to false when unset")
	}
}
