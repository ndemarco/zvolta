package zfs

import (
	"fmt"
	"strings"
	"testing"
)

// MockCommander records calls and returns preset output.
type MockCommander struct {
	Calls   [][]string
	Outputs map[string]string // key: joined args, value: stdout
	Errors  map[string]error
}

func NewMockCommander() *MockCommander {
	return &MockCommander{
		Outputs: make(map[string]string),
		Errors:  make(map[string]error),
	}
}

func (m *MockCommander) Run(args ...string) (string, error) {
	m.Calls = append(m.Calls, args)
	key := strings.Join(args, " ")
	if err, ok := m.Errors[key]; ok {
		return "", err
	}
	if out, ok := m.Outputs[key]; ok {
		return out, nil
	}
	return "", nil
}

func TestListDatasets(t *testing.T) {
	mock := NewMockCommander()
	mock.Outputs["list -H -o name -t filesystem,volume"] = "tank\ntank/data\ntank/home\n"
	client := &Client{Cmd: mock}

	datasets, err := client.ListDatasets()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(datasets) != 3 {
		t.Errorf("got %d datasets, want 3", len(datasets))
	}
	if datasets[1].Name != "tank/data" {
		t.Errorf("datasets[1] = %q, want %q", datasets[1].Name, "tank/data")
	}
}

func TestGetProperty(t *testing.T) {
	mock := NewMockCommander()
	mock.Outputs["get -H -o value org.zvolta:autosnap tank/data"] = "on\n"
	client := &Client{Cmd: mock}

	val, err := client.GetProperty("tank/data", "org.zvolta:autosnap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "on" {
		t.Errorf("got %q, want %q", val, "on")
	}
}

func TestGetPropertyUnset(t *testing.T) {
	mock := NewMockCommander()
	mock.Outputs["get -H -o value org.zvolta:autosnap tank/data"] = "-\n"
	client := &Client{Cmd: mock}

	val, err := client.GetProperty("tank/data", "org.zvolta:autosnap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "-" {
		t.Errorf("got %q, want %q", val, "-")
	}
}

func TestGetProperties(t *testing.T) {
	mock := NewMockCommander()
	mock.Outputs["get -H -o property,value org.zvolta:autosnap,org.zvolta:snapshot-hourly tank/data"] =
		"org.zvolta:autosnap\ton\norg.zvolta:snapshot-hourly\t24\n"
	client := &Client{Cmd: mock}

	props, err := client.GetProperties("tank/data", []string{"org.zvolta:autosnap", "org.zvolta:snapshot-hourly"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props["org.zvolta:autosnap"] != "on" {
		t.Errorf("autosnap = %q, want %q", props["org.zvolta:autosnap"], "on")
	}
	if props["org.zvolta:snapshot-hourly"] != "24" {
		t.Errorf("hourly = %q, want %q", props["org.zvolta:snapshot-hourly"], "24")
	}
}

func TestCreateSnapshot(t *testing.T) {
	mock := NewMockCommander()
	client := &Client{Cmd: mock}

	err := client.CreateSnapshot("tank/data", "zvolta_hourly_2026-03-05T14-00-00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	args := strings.Join(mock.Calls[0], " ")
	if args != "snapshot tank/data@zvolta_hourly_2026-03-05T14-00-00Z" {
		t.Errorf("got %q", args)
	}
}

func TestDestroySnapshot(t *testing.T) {
	mock := NewMockCommander()
	client := &Client{Cmd: mock}

	err := client.DestroySnapshot("tank/data", "zvolta_hourly_2026-03-05T14-00-00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := strings.Join(mock.Calls[0], " ")
	if args != "destroy tank/data@zvolta_hourly_2026-03-05T14-00-00Z" {
		t.Errorf("got %q", args)
	}
}

func TestListSnapshots(t *testing.T) {
	mock := NewMockCommander()
	mock.Outputs["list -H -o name,creation -t snapshot -s creation -r -d 1 tank/data"] =
		"tank/data@zvolta_hourly_2026-03-05T13-00-00Z\tWed Mar  5 13:00 2026\n" +
			"tank/data@zvolta_hourly_2026-03-05T14-00-00Z\tWed Mar  5 14:00 2026\n"
	client := &Client{Cmd: mock}

	snaps, err := client.ListSnapshots("tank/data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(snaps))
	}
	if snaps[0].Dataset != "tank/data" {
		t.Errorf("dataset = %q", snaps[0].Dataset)
	}
	if snaps[0].SnapName != "zvolta_hourly_2026-03-05T13-00-00Z" {
		t.Errorf("snapname = %q", snaps[0].SnapName)
	}
}

func TestCommandError(t *testing.T) {
	mock := NewMockCommander()
	mock.Errors["list -H -o name -t filesystem,volume"] = fmt.Errorf("zfs list: pool not imported")
	client := &Client{Cmd: mock}

	_, err := client.ListDatasets()
	if err == nil {
		t.Error("expected error")
	}
}
