# Zvolta Design Decisions

Decisions made during planning. Each entry records the choice, rationale, and alternatives considered.

---

## D1: ZFS Interaction — Shell out to `zfs` CLI

**Decision:** Call `/sbin/zfs` (or `zfs` on PATH) via `os/exec` rather than using a Go ZFS library like `go-libzfs`.

**Rationale:**
- Avoids CGO dependency, preserving the single static binary goal
- Matches what SANoid does — proven approach
- More portable across ZFS versions and distributions
- Easier to test (mock command output vs mock C bindings)

**Alternatives considered:**
- `go-libzfs` — type-safe but requires CGO, libzfs headers at build time, and ties the binary to a specific ZFS version

---

## D2: Server Config Format — TOML

**Decision:** Use TOML for the daemon/server configuration file.

**Rationale:**
- Lighter than YAML, no ambiguous type coercion (the "Norway problem")
- Idiomatic for Go infrastructure tools
- Excellent Go library support (`BurntSushi/toml`, `pelletier/go-toml`)

---

## D3: Retention Semantics — Count-Based

**Decision:** Retention values (e.g., `snapshot-hourly=24`) mean "keep the N most recent snapshots of this tier." Not time-based.

**Rationale:**
- Matches SANoid behavior — proven, well-understood model
- Simpler to implement and reason about
- No ambiguity about what happens during downtime

---

## D4: Scheduling — Clock-Aligned with Configurable Offset

**Decision:** Snapshot schedules align to clock boundaries by default:
- Frequent: minutes from top of the hour
- Hourly: top of the hour
- Daily: midnight
- Weekly: Sunday midnight
- Monthly: 1st of month midnight
- Yearly: Jan 1 midnight

An instance-wide offset (positive or negative) is configurable in the server config to shift all boundaries.

**Rationale:**
- Predictable snapshot times that users can reason about
- Offset handles cases where midnight snapshots conflict with other scheduled work

**Phase 2 consideration:** Per-dataset schedule offset via `org.zvolta:schedule-offset` ZFS property, inheritable down the dataset tree. This would allow different dataset subtrees to snapshot at different offsets while keeping the global offset as the default.

---

## D5: Catch-Up After Downtime — Single Snapshot per Tier

**Decision:** If the daemon starts and snapshots are overdue for a tier, take one catch-up snapshot. Do not attempt to backfill the gap.

**Rationale:**
- Backfilling would create snapshots of the current state with past timestamps — misleading
- One catch-up snapshot ensures the dataset is protected going forward
- Matches practical expectations

---

## D6: Privilege Model — User-Configurable

**Decision:** Zvolta does not assume it runs as root. The daemon runs as whatever user is configured. ZFS permissions (either root or `zfs allow` delegation) are the user's responsibility to set up.

**Rationale:**
- Different environments have different security requirements
- `zfs allow` is the proper way to delegate snapshot permissions without root
- Documentation will cover both approaches

---

## D7: SANoid Compatibility — None (Clean Room)

**Decision:** Zvolta does not attempt to read, manage, or coexist with SANoid-named snapshots. It is a clean replacement.

**Rationale:**
- Avoids complexity of parsing SANoid naming conventions
- Users migrating from SANoid should decommission it first
- Clean separation prevents accidental pruning of SANoid snapshots

---

## D8: MVP Scope

**Decision:** Phase 1 (core snapshot management) is the MVP. Samba integration is Phase 2.

**Phase 1 delivers:**
- ZFS property-driven policy
- Scheduled snapshot creation across all tiers
- Count-based retention and pruning
- Management CLI (`zvolta status`, `zvolta snap`, `zvolta list`, etc.)
- Daemon with systemd integration

---

## D9: Snapshot Naming — Two-Layer Architecture

**Decision:** ZFS snapshots use a Zvolta-native naming scheme. Samba-compatible names are produced by a deterministic translation layer. These are two separate systems with two separate purposes, connected by a dynamic, definable bridge.

**ZFS snapshot name format (Phase 1):**
`zvolta_{tier}_{ISO8601}` e.g., `zvolta_hourly_2026-03-05T14-00-00Z`

The name encodes three things:
1. **Ownership prefix** (default `zvolta_`, overridable in server config) — namespace boundary so Zvolta never touches non-Zvolta snapshots and vice versa
2. **Tier** (`hourly`, `daily`, etc.) — required for per-tier retention/pruning
3. **Timestamp** (ISO 8601) — deterministic, lexicographically sortable, unique at 1-minute resolution

**Samba name translation (Phase 2):**
A per-dataset translation spec (the `org.zvolta:naming-algorithm` property) defines how to deterministically convert the Zvolta-native name into a Samba-compatible shadow copy name (e.g., `@GMT-2026.03.05-14.00.00`). The translation is:
- Algorithmic and stateless — no mapping database
- Reversible — given a Samba name, you can recover the ZFS snapshot name
- Per-dataset configurable — different datasets can use different translation specs

**Design of the translation spec is deferred to Phase 2.**

---

## D10: Samba `.zfs/snapshot` vs Clone-Mount — Deferred

**Decision:** Whether Samba reads from `.zfs/snapshot/<name>` directly or requires clone-mounted snapshots is deferred for investigation during Phase 2.

---

## D12: Minimum Tick Interval — 1 Minute

**Decision:** The scheduler's minimum tick interval is 1 minute. Sub-minute scheduling is not supported.

**Rationale:**
- ZFS snapshot operations have non-trivial overhead
- 1-minute granularity is sufficient even for the most frequent snapshot tiers (frequent-period minimum is 5 minutes)
- Keeps the daemon lightweight

---

## D13: Logging — stdout for systemd Journal Capture

**Decision:** Phases 1 and 2 log to stdout only. systemd journal captures and manages log persistence. Loki-compatible structured log output is a future enhancement.

**Rationale:**
- Avoids duplicating what systemd journal already does well
- Simplifies daemon — no log file rotation, path config, or file handling
- `journalctl -u zvolta` is the standard way to view logs
- Loki integration can be added later without changing the logging interface (structured slog output is already Loki-friendly)

---

## D14: Templates — Deferred to Phase 2

**Decision:** The `org.zvolta:template` property (named/shared policy templates across datasets) is a Phase 2 feature. Phase 1 requires explicit per-dataset properties or inheritance.

**Rationale:**
- Inheritance already reduces repetition for common cases
- Templates add configuration complexity that isn't needed for the MVP
- Phase 2 can design templates alongside the naming algorithm

---

## D11: Testing — ZFS Test Pool in Build Pipeline

**Decision:** Integration tests use a small ZFS pool created as part of the build pipeline (e.g., file-backed zpool). Unit tests mock ZFS CLI output.

**Rationale:**
- Real ZFS operations catch issues that mocks miss
- File-backed pool is cheap and disposable
- Keeps unit tests fast and CI-friendly
