# Zvolta — Development Guide

## Project
ZFS snapshot lifecycle management with Windows Previous Versions integration.

- **Language:** Go
- **Module:** `github.com/ndemarco/zvolta`
- **Config format:** TOML (server config), ZFS custom properties (per-dataset policy)
- **Key docs:** `DECISIONS.md`, `PLAN_PHASE1.md`, `PROJECT_INITIAL_DESCRIPTION.md`
- **Reference code:** `_reference/sanoid/` (git submodule — SANoid source for studying scheduling, retention, and edge cases)

## Build

```bash
go build -o zvolta ./cmd/zvolta
```

## Test

```bash
# Unit tests (no ZFS required)
go test ./...

# Integration tests (requires ZFS pool — see scripts/test-pool.sh)
go test -tags=integration ./...
```

## Project Structure

```
cmd/zvolta/          Main entry point, CLI subcommands (cobra)
internal/
  config/            TOML server config parsing
  policy/            ZFS property reading, inheritance, policy resolution
  zfs/               ZFS CLI wrapper (os/exec, no CGO)
  scheduler/         Clock-aligned snapshot scheduling
  snapshot/          Snapshot creation, listing, pruning
  daemon/            Daemon lifecycle, signal handling
configs/             Example config files
```

## Key Conventions

- **ZFS interaction:** Shell out to `zfs` CLI via `os/exec`. No CGO, no `go-libzfs`. (D1)
- **Snapshot names:** `{prefix}{tier}_{ISO8601}` e.g., `zvolta_hourly_2026-03-05T14-00-00Z` (D9)
- **Logging:** stdout only via `log/slog`. systemd journal captures output. (D13)
- **Scheduling:** Clock-aligned with configurable offset, 1-minute minimum tick. (D4, D12)
- **Retention:** Count-based per tier. (D3)
- **No SANoid compatibility.** Clean room replacement. (D7)

## CLI Subcommands

- `zvolta daemon` — run the snapshot management daemon
- `zvolta status` — show managed datasets and their policies
- `zvolta snap <dataset>` — manually trigger a snapshot
- `zvolta list <dataset>` — list snapshots
- `zvolta prune <dataset>` — manually trigger pruning

## ZFS Custom Properties

Properties use the `org.zvolta:` namespace and inherit down the dataset tree.

| Property | Example |
|---|---|
| `org.zvolta:autosnap` | `on` / `off` |
| `org.zvolta:autoprune` | `on` / `off` |
| `org.zvolta:snapshot-hourly` | `24` |
| `org.zvolta:snapshot-daily` | `30` |
| `org.zvolta:snapshot-frequent` | `12` |
| `org.zvolta:snapshot-frequent-period` | `5` (minutes) |
