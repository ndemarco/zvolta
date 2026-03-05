# Zvolta

**ZFS snapshot lifecycle management with Windows Previous Versions integration.**

Zvolta is a Go daemon that manages ZFS snapshots — creation, retention, and pruning — driven by policies stored directly on your datasets as ZFS custom properties. In Phase 2, it will expose those snapshots to Windows clients as Previous Versions through Samba, with no client software required.

The name is a nod to the Italian word *volta* — a turn, a time — because snapshots are moments you can return to. The Z is for ZFS.

## Why Zvolta?

Existing tools like [SANoid](https://github.com/jimsalterjrs/sanoid) work, but they rely on Perl, cron, and centralized config files that live separately from the data they manage. Zvolta takes a different approach:

- **Policy lives on the dataset.** Snapshot frequency and retention are set as ZFS custom properties (`org.zvolta:*`), inherited down the dataset tree. No config file to keep in sync.
- **Single binary, no dependencies.** Written in Go. No Perl, no cron, no runtime requirements.
- **Built-in scheduler.** Runs as a systemd daemon with clock-aligned snapshot scheduling.
- **Samba integration.** (Phase 2) Deterministic name translation exposes ZFS snapshots as Windows Previous Versions — no mapping database, no state to corrupt.

## Status

Under active development. Phase 1 (core snapshot management) is in progress.

## Quick Start

Set a snapshot policy on a dataset:

```bash
zfs set org.zvolta:autosnap=on pool/data
zfs set org.zvolta:snapshot-hourly=24 pool/data
zfs set org.zvolta:snapshot-daily=30 pool/data
```

Run the daemon:

```bash
zvolta daemon --config /etc/zvolta/zvolta.toml
```

Or manage snapshots directly:

```bash
zvolta status              # Show managed datasets and policies
zvolta snap pool/data      # Take a snapshot now
zvolta list pool/data      # List snapshots
zvolta prune pool/data     # Prune expired snapshots
```

## Configuration

### Server Config (TOML)

Infrastructure-level settings: log level, schedule offset, snapshot name prefix, managed dataset roots. See [`configs/zvolta.toml.example`](configs/zvolta.toml.example) for a complete example.

### Per-Dataset Policy (ZFS Properties)

| Property | Purpose | Example |
|---|---|---|
| `org.zvolta:autosnap` | Enable automatic snapshots | `on` |
| `org.zvolta:autoprune` | Enable automatic pruning | `on` |
| `org.zvolta:snapshot-frequent` | Sub-hourly snapshots to retain | `12` |
| `org.zvolta:snapshot-frequent-period` | Interval in minutes | `5` |
| `org.zvolta:snapshot-hourly` | Hourly snapshots to retain | `24` |
| `org.zvolta:snapshot-daily` | Daily snapshots to retain | `30` |
| `org.zvolta:snapshot-weekly` | Weekly snapshots to retain | `4` |
| `org.zvolta:snapshot-monthly` | Monthly snapshots to retain | `12` |
| `org.zvolta:snapshot-yearly` | Yearly snapshots to retain | `3` |

Properties inherit from parent datasets. Set a policy on `pool/data` and all child datasets pick it up. Override on any child as needed.

## How Zvolta Compares to SANoid

| | SANoid | Zvolta |
|---|---|---|
| **Language** | Perl | Go (single static binary) |
| **Scheduling** | Cron | Built-in daemon with clock-aligned ticks |
| **Configuration** | `sanoid.conf` file | ZFS custom properties on the datasets themselves |
| **Policy inheritance** | Manual per-dataset config sections | Native ZFS property inheritance |
| **Time handling** | Localtime (complex DST workarounds) | UTC throughout (no DST issues) |
| **Snapshot naming** | `autosnap_{date}_{time}_{tier}` | `{prefix}{tier}_{ISO8601}` (configurable prefix) |
| **Samba integration** | None | Deterministic name translation (Phase 2) |
| **Runtime dependencies** | Perl, cron, Config::IniFiles | None |
| **Retention model** | Dual-constraint (age AND count) | Count-based (keep newest N) |

Zvolta is a clean-room reimplementation informed by SANoid's years of community-tested logic, rebuilt with modern tooling and a simpler architecture.

## Building

```bash
go build -o zvolta ./cmd/zvolta
```

## License

GPLv3 — see [LICENSE](LICENSE).
