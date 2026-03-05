//go:build integration

package zfs

import (
	"strings"
	"testing"
)

const testPool = "zvolta-test"
const testDataset = "zvolta-test/data"

func TestIntegrationListDatasets(t *testing.T) {
	client := NewClient("")
	datasets, err := client.ListDatasets()
	if err != nil {
		t.Fatalf("ListDatasets: %v", err)
	}

	found := false
	for _, ds := range datasets {
		if ds.Name == testDataset {
			found = true
		}
	}
	if !found {
		names := make([]string, len(datasets))
		for i, ds := range datasets {
			names[i] = ds.Name
		}
		t.Errorf("expected %s in dataset list, got: %v", testDataset, names)
	}
}

func TestIntegrationSetGetProperty(t *testing.T) {
	client := NewClient("")

	err := client.SetProperty(testDataset, "org.zvolta:autosnap", "on")
	if err != nil {
		t.Fatalf("SetProperty: %v", err)
	}

	val, err := client.GetProperty(testDataset, "org.zvolta:autosnap")
	if err != nil {
		t.Fatalf("GetProperty: %v", err)
	}
	if val != "on" {
		t.Errorf("got %q, want %q", val, "on")
	}

	// Clean up
	_ = client.SetProperty(testDataset, "org.zvolta:autosnap", "-")
}

func TestIntegrationGetPropertiesUnset(t *testing.T) {
	client := NewClient("")

	val, err := client.GetProperty(testDataset, "org.zvolta:snapshot-hourly")
	if err != nil {
		t.Fatalf("GetProperty: %v", err)
	}
	if val != "-" {
		t.Errorf("unset property should be %q, got %q", "-", val)
	}
}

func TestIntegrationSnapshotLifecycle(t *testing.T) {
	client := NewClient("")
	snapName := "zvolta_hourly_2026-03-05T14-00-00Z"

	// Create
	err := client.CreateSnapshot(testDataset, snapName)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// List and verify
	snaps, err := client.ListSnapshots(testDataset)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	found := false
	for _, s := range snaps {
		if s.SnapName == snapName {
			found = true
			if s.Dataset != testDataset {
				t.Errorf("dataset = %q, want %q", s.Dataset, testDataset)
			}
		}
	}
	if !found {
		t.Errorf("snapshot %s not found in list", snapName)
	}

	// Destroy
	err = client.DestroySnapshot(testDataset, snapName)
	if err != nil {
		t.Fatalf("DestroySnapshot: %v", err)
	}

	// Verify gone
	snaps, err = client.ListSnapshots(testDataset)
	if err != nil {
		t.Fatalf("ListSnapshots after destroy: %v", err)
	}
	for _, s := range snaps {
		if s.SnapName == snapName {
			t.Errorf("snapshot %s still exists after destroy", snapName)
		}
	}
}

func TestIntegrationGetPropertiesRecursive(t *testing.T) {
	client := NewClient("")

	// Set property on parent
	err := client.SetProperty(testPool, "org.zvolta:autosnap", "on")
	if err != nil {
		t.Fatalf("SetProperty on pool: %v", err)
	}
	defer client.SetProperty(testPool, "org.zvolta:autosnap", "-")

	results, err := client.GetPropertiesRecursive(testPool, "org.zvolta:autosnap")
	if err != nil {
		t.Fatalf("GetPropertiesRecursive: %v", err)
	}

	// Child should inherit
	childVal, ok := results[testDataset]
	if !ok {
		t.Fatalf("child dataset %s not in results: %v", testDataset, results)
	}
	if childVal != "on" {
		t.Errorf("inherited value = %q, want %q", childVal, "on")
	}
}

func TestIntegrationCreateSnapshotBadDataset(t *testing.T) {
	client := NewClient("")
	err := client.CreateSnapshot("nonexistent/dataset", "test-snap")
	if err == nil {
		t.Error("expected error for nonexistent dataset")
	}
	if !strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "not exist") {
		// Just verify it's an error — exact message varies by ZFS version
		t.Logf("error (expected): %v", err)
	}
}
