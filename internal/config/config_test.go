package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.SnapshotPrefix != "zvolta_" {
		t.Errorf("default prefix = %q, want %q", cfg.SnapshotPrefix, "zvolta_")
	}
	if cfg.ZFSBinary != "zfs" {
		t.Errorf("default binary = %q, want %q", cfg.ZFSBinary, "zfs")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log level = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad(t *testing.T) {
	content := `
snapshot_prefix = "test_"
zfs_binary = "/usr/sbin/zfs"
log_level = "debug"
schedule_offset = "5m"
datasets = ["tank/data", "tank/home"]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SnapshotPrefix != "test_" {
		t.Errorf("prefix = %q, want %q", cfg.SnapshotPrefix, "test_")
	}
	if cfg.ZFSBinary != "/usr/sbin/zfs" {
		t.Errorf("binary = %q, want %q", cfg.ZFSBinary, "/usr/sbin/zfs")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log level = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.ScheduleOffset.Duration != 5*time.Minute {
		t.Errorf("offset = %v, want %v", cfg.ScheduleOffset.Duration, 5*time.Minute)
	}
	if len(cfg.Datasets) != 2 || cfg.Datasets[0] != "tank/data" {
		t.Errorf("datasets = %v, want [tank/data tank/home]", cfg.Datasets)
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	content := `log_level = "verbose"`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid log level")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadEmptyPrefix(t *testing.T) {
	content := `snapshot_prefix = ""`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for empty prefix")
	}
}
