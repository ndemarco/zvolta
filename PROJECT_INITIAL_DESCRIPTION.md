# Zvolta
### Automated ZFS snapshot management with Windows Previous Versions integration

## Project Identity
- **Project Name:** Zvolta
- **Author / Owner:** NickyDoes (Nick DeMarco)
- **Language:** Go
- **License:** TBD
- **Repository:** TBD

## Problem Statement
ZFS is a powerful filesystem with excellent snapshot capabilities, but there is no modern, unified tool that both manages ZFS snapshots with flexible per-dataset policies AND exposes those snapshots natively to Windows clients as Previous Versions via Samba. Existing tools like SANoid are written in Perl, use centralized configuration files disconnected from the datasets they manage, and have no native Samba shadow copy integration. The result is a fragile, multi-tool chain with naming incompatibilities and no clean path to Windows self-service file recovery.

## Solution
Zvolta is a Go daemon that manages the full ZFS snapshot lifecycle and exposes snapshots to Samba as Windows-compatible shadow copies. Configuration lives on the datasets themselves as ZFS custom properties, making policy intrinsic to the data rather than maintained in a separate file. A domain-wide server configuration file handles infrastructure-level concerns. A deterministic, algorithmic naming layer bridges ZFS snapshot names to Samba-compatible shadow copy names with no persistent state required.

## Goals
- Replace SANoid with a modern, maintainable Go implementation
- Enable per-dataset snapshot policy via ZFS custom properties
- Expose ZFS snapshots to Samba as Windows Previous Versions natively
- Eliminate naming incompatibilities between ZFS snapshots and Samba shadow copies via algorithmic translation
- Deliver a single self-contained binary with no runtime dependencies
- Support fine-grained snapshot frequency per dataset (as frequent as every 5 minutes)

## Non-Goals (v1)
- ZFS replication or remote snapshot transfer (Syncoid territory — out of scope)
- GUI or web management interface
- Support for non-ZFS filesystems
- Named/shared algorithm templates across datasets (considered for v2)
- Support for shadow copy protocols other than Samba/SMB

## Architecture Overview

Zvolta operates as a systemd daemon with four core subsystems:

### 1. Policy Engine
Reads ZFS custom properties on each dataset to determine snapshot frequency, retention policy, and naming algorithm. Policies inherit down the dataset tree. Children can override parent policies.

### 2. Snapshot Manager
Implements the snapshot lifecycle: creation on schedule, retention enforcement, and pruning of expired snapshots. Internal scheduler — no cron dependency.

### 3. Name Translator
Applies a deterministic, per-dataset naming algorithm to convert ZFS snapshot names into Samba-compatible shadow copy names on demand. No mapping database or state synchronization required.

### 4. Samba Interface
Presents translated snapshot names to Samba's vfs_shadow_copy2 module. Windows clients see clean, human-readable version timestamps in "Previous Versions" with no client software required.

## Configuration Model

### Domain-Wide Server Config (file)
- Shadow copy mount base path
- Daemon behavior (log level, polling interval)
- Pruning schedule and behavior
- Samba integration settings
- Service identity and permissions

### Per-Dataset Policy (ZFS custom properties)

| Property | Purpose |
|---|---|
| `org.zvolta:snapshot-frequent` | Number of sub-hourly snapshots to retain |
| `org.zvolta:snapshot-frequent-period` | Interval in minutes for sub-hourly snapshots |
| `org.zvolta:snapshot-hourly` | Number of hourly snapshots to retain |
| `org.zvolta:snapshot-daily` | Number of daily snapshots to retain |
| `org.zvolta:snapshot-weekly` | Number of weekly snapshots to retain |
| `org.zvolta:snapshot-monthly` | Number of monthly snapshots to retain |
| `org.zvolta:snapshot-yearly` | Number of yearly snapshots to retain |
| `org.zvolta:autosnap` | Enable/disable automatic snapshots |
| `org.zvolta:autoprune` | Enable/disable automatic pruning |
| `org.zvolta:naming-algorithm` | Algorithm for translating snapshot names to Samba shadow copy names |
| `org.zvolta:samba-expose` | Enable/disable Samba shadow copy exposure |

Properties inherit from parent datasets. Children override as needed.

## SANoid Feature Analysis

### Carried Forward
- Snapshot creation across frequency tiers (frequent, hourly, daily, weekly, monthly, yearly)
- Automatic pruning based on retention counts
- Recursive dataset handling with per-dataset overrides
- Sub-hourly snapshot support
- Health and status monitoring

### Rewritten / Modified
- Configuration model — ZFS properties replace sanoid.conf entirely
- Snapshot naming — deterministic algorithmic naming replaces autosnap_timestamp_tier convention
- Scheduling — internal daemon loop replaces cron dependency
- Implementation language — Go replaces Perl

### Dropped
- Syncoid (replication) — out of scope
- Two-file config model
- Perl runtime dependencies

### New (not in SANoid)
- ZFS property-driven configuration schema
- Algorithmic snapshot name translation layer
- Samba vfs_shadow_copy2 integration
- Domain-wide server config for infrastructure settings
- Self-contained Go binary

## Development Phases

### Phase 1 — Core Snapshot Management
- ZFS property schema definition
- Policy engine reading dataset properties
- Snapshot creation daemon with internal scheduler
- Retention and pruning logic
- Basic health monitoring and logging

### Phase 2 — Samba Integration
- Naming algorithm implementation and per-dataset configuration
- Samba vfs_shadow_copy2 interface layer
- Shadow copy exposure and Windows Previous Versions validation
- End-to-end testing with Windows 11 clients

### Phase 3 — Hardening and Distribution
- Comprehensive error handling and edge cases
- systemd unit file and packaging
- Documentation
- Named algorithm templates (v2 feature)
- Potential community release

## Technology Stack

| Component | Choice | Rationale |
|---|---|---|
| Language | Go | Single binary, no runtime deps, strong daemon support |
| Per-dataset config | ZFS custom properties | Policy intrinsic to the data |
| Server config | TOML or YAML | Infrastructure-level settings |
| Samba integration | vfs_shadow_copy2 | Native Windows Previous Versions |
| Packaging | systemd service | Standard Linux daemon lifecycle |

## Open Questions
- Exact naming algorithm syntax and format (define before Phase 2)
- Whether Samba requires clone-mounted snapshots or can consume .zfs/snapshot directly
- Packaging format (deb, rpm, binary tarball)
- Whether to expose a management CLI alongside the daemon

## References
- SANoid: https://github.com/jimsalterjrs/sanoid
- Samba vfs_shadow_copy2: https://www.samba.org/samba/docs/current/man-html/vfs_shadow_copy2.8.html
- OpenZFS: https://openzfs.github.io/openzfs-docs/