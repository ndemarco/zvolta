# Zvolta Phase 1 — Core Snapshot Management

## Goal
Replace SANoid: daemon that creates and prunes ZFS snapshots based on per-dataset policies stored as ZFS custom properties, with a management CLI.

## Project Structure

```
zvolta/
  cmd/
    zvolta/           # main entry point (daemon + CLI subcommands)
  internal/
    config/           # TOML server config parsing
    policy/           # ZFS property reading, inheritance, policy resolution
    zfs/              # ZFS CLI wrapper (exec-based)
    scheduler/        # Clock-aligned snapshot scheduling
    snapshot/         # Snapshot creation, listing, pruning
    daemon/           # Daemon lifecycle, signal handling
  _reference/
    sanoid/           # SANoid source (git submodule, not compiled)
  configs/
    zvolta.toml.example
  DECISIONS.md
  PLAN_PHASE1.md
  PROJECT_INITIAL_DESCRIPTION.md
  go.mod
  go.sum
```

## Implementation Steps

### Step 1: Project Scaffolding
- `go mod init github.com/nickydoes/zvolta`
- Directory structure
- Basic `main.go` with cobra CLI skeleton
- Subcommands: `daemon`, `status`, `snap`, `list`, `version`

### Step 2: ZFS CLI Wrapper (`internal/zfs/`)
- Functions: `ListDatasets()`, `GetProperty()`, `GetProperties()`, `SetProperty()`, `CreateSnapshot()`, `DestroySnapshot()`, `ListSnapshots()`
- Structured output parsing (ZFS `-H -o` tab-separated format)
- Error handling for common failure modes (pool not imported, permission denied, dataset not found)
- **Reference:** Study SANoid's `_reference/sanoid/sanoid` for which ZFS commands and flags it uses

### Step 3: Server Config (`internal/config/`)
- TOML config struct with sane defaults
- Fields: log level, schedule offset, snapshot name prefix, ZFS binary path, managed dataset roots
- Example config file in `configs/zvolta.toml.example`
- Config file path from CLI flag or default `/etc/zvolta/zvolta.toml`

### Step 4: Policy Engine (`internal/policy/`)
- Read `org.zvolta:*` properties from datasets
- Property inheritance: walk up the dataset tree, child overrides parent
- Resolve a complete policy for each managed dataset
- Default policy when no properties are set (autosnap=off — explicit opt-in)
- Validation: reject invalid combinations (e.g., frequent-period < 5 min)

### Step 5: Snapshot Naming (Phase 1)
- Format: `zvolta_{tier}_{ISO8601}` (e.g., `zvolta_hourly_2026-03-05T14-00-00Z`)
- Encodes: ownership prefix + tier + sortable timestamp (D9)
- Phase 2 adds a deterministic translation layer to produce Samba-compatible names from these

### Step 6: Scheduler (`internal/scheduler/`)
- Tick-based loop evaluating each dataset's policy
- Clock-aligned scheduling with configurable offset (D4)
- Determines which tiers are due for each dataset at each tick
- Handles catch-up after downtime (D5): one snapshot per overdue tier
- **Reference:** Study SANoid's scheduling logic closely — there are likely edge cases around DST, month boundaries, etc.

### Step 7: Snapshot Manager (`internal/snapshot/`)
- `Create(dataset, tier)` — creates a snapshot with the Phase 1 naming scheme
- `List(dataset)` — lists Zvolta-managed snapshots, parsed into structured data
- `Prune(dataset, tier, keep)` — enforces retention count per tier, destroys oldest beyond `keep`
- Dry-run mode for pruning (log what would be deleted)

### Step 8: Daemon Lifecycle (`internal/daemon/`)
- Main loop: wake on schedule tick, evaluate all managed datasets, create/prune
- Signal handling: SIGHUP reloads config, SIGTERM graceful shutdown
- Logging: structured logging (slog) with configurable level
- Lock file to prevent multiple daemon instances

### Step 9: CLI Commands (`cmd/zvolta/`)
- `zvolta daemon` — run the daemon (foreground; systemd manages backgrounding)
- `zvolta status` — show all managed datasets, their policies, and last snapshot times
- `zvolta snap <dataset> [--tier hourly]` — manually trigger a snapshot
- `zvolta list <dataset>` — list snapshots with tier, age, and name
- `zvolta prune <dataset> [--dry-run]` — manually trigger pruning

### Step 10: systemd Integration
- Unit file: `zvolta.service`
- Type=simple (daemon runs in foreground)
- Restart=on-failure
- After=zfs.target

### Step 11: Testing
- **Unit tests:** Mock ZFS CLI output for policy, scheduler, and snapshot logic
- **Integration tests:** File-backed ZFS pool, test real create/list/destroy cycles
- **Build pipeline:** Script to create/destroy test zpool

## Key SANoid Files to Study

| SANoid File | What to Learn |
|---|---|
| `sanoid` (main script) | Scheduling logic, retention algorithm, edge cases |
| `sanoid.conf` | Default values and which options matter in practice |
| `sanoid.defaults.conf` | Full list of configurable options and their defaults |
| GitHub Issues/PRs | Bugs and edge cases the community has found |

## Resolved
- **Minimum tick interval:** 1 minute (D12)
- **Logging:** stdout only, captured by systemd journal (D13). Loki-compatible output is a future enhancement.
- **Templates:** Deferred to Phase 2 (D14)

## Open Items for Phase 1
- (none)
