# Zvolta Priorities

Phase 1 is complete. All work below is Phase 2 and beyond.

---

## P1 — SIGHUP Config Reload

**Why it's P1:** The daemon currently ignores SIGHUP (default terminate). Operators expect `systemctl reload zvolta` to apply config changes without a full restart. This is a basic operational requirement before Samba work begins, since Samba config integration will require reloading state.

**Scope:**
- Re-read `/etc/zvolta/zvolta.toml` on SIGHUP
- Re-register `signal.Notify` for SIGHUP in daemon
- Validate new config before applying; log and keep old config on error
- No restart of the scheduler loop — just update `d.Config` in place

---

## P2 — Samba / Windows Previous Versions Integration

**Goal:** Windows clients on the same SMB share see ZFS snapshots as "Previous Versions" natively — no client software, no manual restore. User right-clicks a file → "Restore previous versions" → done.

**Open decision required before implementation: D10**

### D10: How does Samba access snapshot contents?

Two architecturally distinct approaches. Choose one before writing any P2 code.

---

#### Option A: `.zfs/snapshot` direct access via `vfs_shadow_copy2`

ZFS automatically exposes every snapshot at `.zfs/snapshot/<name>` inside the dataset root. Samba's `vfs_shadow_copy2` VFS module can be pointed at this directory to serve Previous Versions.

**How it works:**
1. Set `snapdir=visible` on each managed dataset (`zfs set snapdir=visible <dataset>`)
2. Configure Samba share with:
   ```ini
   vfs objects = shadow_copy2
   shadow:snapdir = .zfs/snapshot
   shadow:sort = desc
   shadow:format = zvolta_%S_%Y-%m-%dT%H-%M-%SZ
   shadow:snapprefix = zvolta_
   ```
3. `vfs_shadow_copy2` reads the directory listing of `.zfs/snapshot`, parses names matching `shadow:format`, and presents them as `@GMT-...` timestamps to Windows

**Pros:**
- No additional ZFS operations — snapshots appear in `.zfs/snapshot` the moment they're created
- No clone lifecycle management in Zvolta
- No extra datasets or mounts
- Proven approach used by TrueNAS and others

**Cons:**
- Requires `snapdir=visible` on every dataset — Zvolta would need to enforce/set this
- Samba's `shadow:format` strptime parsing must exactly match the zvolta name format; the format string is fiddly to get right
- All snapshots appear (no per-tier filtering unless `shadow:snapprefix` is used per share)
- The `.zfs` directory is hidden by default — Windows clients may see it if browsing

**Name translation note (D9):** With this option, `vfs_shadow_copy2` does the translation internally using `shadow:format`. Zvolta does not need to produce `@GMT-...` names. The `org.zvolta:naming-algorithm` property may be unnecessary for this path.

---

#### Option B: Clone-mount — explicit `@GMT-...` directories

For each snapshot, Zvolta creates a ZFS clone and mounts it at a path named `@GMT-YYYY.MM.DD-HH.MM.SS`. Samba's `vfs_shadow_copy2` (or `vfs_shadow_copy`) reads these mounted paths directly.

**How it works:**
1. On snapshot creation, Zvolta also runs:
   ```
   zfs clone <dataset>@<snapname> <shadowbase>/<dataset>/@GMT-2026.03.05-14.00.00
   zfs set readonly=on <clone>
   ```
2. On snapshot pruning, Zvolta destroys the corresponding clone
3. Samba share configured with:
   ```ini
   vfs objects = shadow_copy2
   shadow:snapdir = /srv/shadow/<dataset>
   shadow:sort = desc
   ```

**Pros:**
- `@GMT-...` names are produced explicitly — no Samba format string parsing
- Snapshots can be mounted anywhere, with any structure
- `snapdir=visible` not required
- More control over what's exposed (can expose only certain tiers)

**Cons:**
- Every snapshot creates a ZFS clone — 2x dataset operations (snapshot + clone)
- Every prune requires destroying the clone first, then the snapshot
- Zvolta must manage clone lifecycle, mount paths, and handle mount failures
- Failed clone creation leaves the snapshot without a shadow copy entry
- Clone proliferation adds ZFS metadata overhead on large snapshot counts
- Significantly more implementation complexity

**Name translation note (D9):** This option IS the translation layer — the clone mount path IS the `@GMT-...` name. `org.zvolta:naming-algorithm` would control the path format.

---

#### Option C: Symlink bridge (hybrid)

Keep snapshots in `.zfs/snapshot` (no clones). Create a managed directory of symlinks with `@GMT-...` names pointing into `.zfs/snapshot/<zvolta-name>`. Samba reads the symlinks.

**How it works:**
1. On snapshot creation, create:
   ```
   /srv/shadow/<dataset>/@GMT-2026.03.05-14.00.00 -> /srv/<dataset>/.zfs/snapshot/zvolta_hourly_2026-03-05T14-00-00Z
   ```
2. On pruning, remove the symlink
3. Samba reads through symlinks normally

**Pros:**
- No clone overhead
- Explicit `@GMT-...` names without Samba format string complexity
- Exposes only what Zvolta creates (no `.zfs` leakage)

**Cons:**
- Requires a writable directory for symlinks — adds a path to manage and configure
- Symlink lifecycle must be kept in sync with snapshot lifecycle; desync = broken Previous Versions
- `snapdir=visible` not required but the symlink target path must be accessible
- More moving parts than Option A for only marginal benefit over it

---

### Recommendation

**Start with Option A.** It's the lightest implementation, has production precedent (TrueNAS Core uses this approach), and requires no clone management. The main risk is the `shadow:format` parsing — prototype the Samba config against a manually-created zvolta snapshot before committing. If format parsing proves brittle across Samba versions, fall back to Option C. Option B (clones) adds complexity that only pays off if you need per-snapshot mount-level control, which isn't a stated requirement.

**Decision needed from Nick before P2 implementation begins.**

---

### P2 implementation order (after D10 is decided)

1. Decide D10 (above)
2. Implement `org.zvolta:samba-expose` property enforcement
3. Set `snapdir=visible` on managed datasets (Option A) or implement clone/symlink lifecycle (B/C)
4. Implement name translation layer (D9) — `internal/naming/` package
5. Add Samba config generation helper (`zvolta samba-config <share>`)
6. Integration tests with a real Samba instance

---

## P3 — Per-Dataset Schedule Offset

**Why P3:** The global `schedule_offset` in the server config shifts all snapshots uniformly. Some datasets (e.g., a busy database) may need their snapshots offset from the global schedule to avoid contention. Implemented via `org.zvolta:schedule-offset` ZFS property, inheritable.

**Scope:** Small change to the scheduler — read per-dataset offset from policy, apply before boundary alignment.

---

## P3 — Dataset Policy Templates

**Why P3:** ZFS property inheritance handles most repetition. Templates (`org.zvolta:template`) would let datasets reference a named policy preset. Adds config convenience at the cost of a new resolution layer. Defer until real-world usage shows inheritance isn't sufficient.

---

## Deferred / Out of Scope

- ZFS replication / remote snapshots (Syncoid territory)
- Web or GUI management interface
- Non-ZFS filesystem support
- Sub-minute snapshot scheduling
- Loki log integration (stdout is sufficient; Loki can consume slog output without changes)
