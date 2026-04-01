---
status: Planned
upstream: docs/prds/PRD-auto-update.md
spawned_from:
  issue: 2184
  repo: tsukumogami/tsuku
problem: |
  Feature 2 produces per-tool cache files showing available updates, but nothing
  acts on them. Users still need to run tsuku update manually for each tool. When
  auto-apply is added, a failed update could leave a tool broken with no fast
  recovery path. Rollback must ship alongside auto-apply since auto-apply is the
  default behavior (PRD D6).
decision: |
  Auto-apply runs in PersistentPreRun with a non-blocking TryLock gate on
  state.json.lock. It reads cached check entries, installs updates via the
  existing runInstallWithTelemetry flow, and on failure auto-rollbacks via a
  PreviousVersion field in ToolState and the existing Activate() method. Failed
  updates write per-tool notice files to $TSUKU_HOME/notices/ with a shown flag
  for one-time stderr display.
rationale: |
  The TryLock gate prevents blocking when concurrent processes run (skip and
  retry on next command). PreviousVersion is the simplest one-level-deep tracking
  that correctly handles reinstalls. Per-tool notice files mirror the cache pattern
  and avoid cross-tool lock contention.
---

# DESIGN: Auto-apply with rollback

## Status

Proposed

## Context and Problem Statement

Feature 2 (background update checks) writes per-tool cache files to `$TSUKU_HOME/cache/updates/<toolname>.json` showing what's available within pin boundaries. But nothing reads those results and acts on them. Users must still manually run `tsuku update <tool>` for each tool.

The auto-update system's core value proposition is that tools stay current without manual intervention. This feature closes the loop: during any tsuku command, if cached check results show a newer version within pin boundaries, tsuku downloads and installs it automatically. The apply phase runs during tsuku commands only (not shell hooks or shim invocations) to avoid adding latency to tool execution or prompt rendering (PRD R3).

Two companion features must ship together:

1. **Auto-rollback on failure** (R10): If an auto-update fails at any point (download, extraction, verification, symlink creation), the previous version remains active with no user intervention.

2. **Manual rollback** (R9): `tsuku rollback <tool>` switches to the immediately preceding version for runtime breakage (tool installs fine but crashes when used). Rollback is one level deep and temporary -- it doesn't change the `Requested` field, so auto-update may re-apply on the next cycle (PRD D7).

3. **Basic failure notices** (R11a): Failed auto-updates write a notice to `$TSUKU_HOME/notices/`. `tsuku notices` displays details. Consecutive-failure suppression (R11) ships in Feature 7.

## Decision Drivers

- **Safety as the default**: Auto-apply is the default behavior (PRD D1). A failed update must never leave the user without a working tool. Auto-rollback is mandatory.
- **Existing install infrastructure**: The install flow (`runInstallWithTelemetry`) already handles downloading, extracting, verifying, and symlinking. Auto-apply should reuse this, not reinvent it.
- **Multi-version directory model**: Tsuku installs each version to `$TSUKU_HOME/tools/<name>-<version>/`. Old version directories remain on disk. This makes rollback cheap (symlink switch, no re-download).
- **Concurrency safety**: Auto-apply mutates state.json and tool directories. It must not run concurrently with other state-mutating commands (install, update, remove).
- **Feature 2 cache as input**: The `UpdateCheckEntry.LatestWithinPin` field from per-tool cache files is the signal for what to install. If the field is empty or matches the active version, no update is needed.
- **Downstream consumers**: Feature 5 (notifications) needs to know what was applied or failed. Feature 7 (resilience) adds consecutive-failure tracking on top of the basic notice system.

## Considered Options

### Decision 1: Apply lifecycle and concurrency

Auto-apply needs to run during tsuku commands, read cached check results, install pending updates, and avoid interfering with concurrent tsuku processes. The key tension: where does auto-apply hook in, and how does it handle the case where another tsuku process is already running?

Key assumptions:
- `runInstallWithTelemetry` can be called from PersistentPreRun (config and loader are initialized in `init()`).
- The install flow's internal state locking needs adjustment to work within an outer TryLock scope. The auto-apply function holds the exclusive lock, and internal `UpdateTool`/`Save` calls must reuse the same lock scope (implementation detail for coding phase).
- PRD R19's "zero added latency" refers to background checks, not the apply step. Apply inherently takes time when updates exist.

#### Chosen: PersistentPreRun with non-blocking TryLock gate

Auto-apply runs in `rootCmd.PersistentPreRun` alongside the existing update check trigger. It first calls `TryLockExclusive` on `state.json.lock`. If another tsuku process holds the lock, auto-apply silently skips. If acquired, it reads all cache entries via `ReadAllEntries`, installs updates for entries where `LatestWithinPin` is non-empty and `Error` is empty, then releases the lock before the user's command runs.

The skip-and-retry semantic is acceptable: cached entries persist until consumed, so a skipped auto-apply is picked up on the next command invocation. The same skip list as the update check trigger applies (`check-updates`, `hook-env`, `run`, `help`, `version`, `completion`).

#### Alternatives considered

- **PersistentPreRun with blocking lock (Option A)**: Same integration point but uses `LockExclusive` instead of `TryLockExclusive`. Rejected because it blocks every command when `tsuku install` is running in another terminal. Auto-apply is best-effort; blocking defeats the purpose.
- **Specific command allowlist (Option B)**: Calls `MaybeAutoApply` explicitly in `install`, `update`, `list`, `outdated`, `info`. Rejected because scattered call sites are fragile -- new commands must opt in manually, and there's no single source of truth for when apply fires.
- **Post-command hook (Option C)**: Runs in `PersistentPostRun` after the user's command. Rejected because tool versions used during the command would be stale (e.g., `tsuku list` shows old versions, then apply installs new ones). Also, cobra's PostRun doesn't fire when the command returns an error.

### Decision 2: Rollback mechanism and state tracking

Rollback needs to track what the previous version was, switch symlinks atomically, and preserve the temporary nature (doesn't change `Requested`). The multi-version directory model means old version directories are already on disk.

Key assumptions:
- Garbage collection (Feature 7) will check `PreviousVersion` before removing a version directory.
- Auto-apply goes through `InstallWithOptions`, so `PreviousVersion` is set consistently for both manual and automatic updates.
- Clearing `PreviousVersion` on explicit install is acceptable -- explicit installs are intentional user decisions.

#### Chosen: PreviousVersion field in ToolState

Add `PreviousVersion string` to `ToolState` in state.json. When `InstallWithOptions` sets a new `ActiveVersion`, it snapshots the old value into `PreviousVersion` (only when `ActiveVersion` is non-empty and differs from the new version). `tsuku rollback <tool>` reads `PreviousVersion`, verifies the directory exists, and calls the existing `Activate()` method to switch symlinks atomically. Rollback doesn't touch `Requested`, so auto-update re-applies on the next cycle.

On explicit `tsuku install <tool>@<version>`, `PreviousVersion` is cleared (the user made a deliberate choice). Empty `PreviousVersion` means "no previous version to roll back to."

#### Alternatives considered

- **Derive from Versions map sorted by InstalledAt (Option B)**: No new field; find the second-most-recently-installed version. Rejected because `InstalledAt` gets overwritten on reinstalls, making the "previous" version ambiguous. If a user installs v1, then v2, then reinstalls v1, the timestamp-based "previous" points to v2, which may not be what they expect.
- **Separate rollback log file (Option C)**: A `$TSUKU_HOME/rollback.json` mapping tools to rollback targets. Rejected because it adds a second source of truth that can drift from state.json and requires coordinating locks across two files.
- **Version history list (Option D)**: A `VersionHistory []string` recording all activations. Rejected because R9 explicitly requires one-level-deep rollback. A history list invites feature creep and unbounded state growth with no current justification.

### Decision 3: Failure notice system

Failed auto-updates need persistent failure records that survive process exit, display once on stderr, and provide detail via `tsuku notices`. This feature implements basic notice writing only -- every failure writes a notice. Feature 7 adds consecutive-failure suppression later.

Key assumptions:
- Failures are rare events (tools that consistently build and verify correctly don't produce notices).
- One notice per tool is sufficient. Full audit trails come from telemetry (R22, Phase 2).
- The `shown` flag rewrite is safe without file locking because only the foreground process reads and marks notices; the background process only writes new ones.
- Feature 7 extends the per-tool file schema (adding `consecutive_failures`) rather than changing the storage model.

#### Chosen: Per-tool JSON files at `$TSUKU_HOME/notices/<toolname>.json`

Each tool gets one notice file. A new failure overwrites the previous notice for that tool. The file contains: tool name, attempted version, error message, timestamp, and a `shown` boolean. On first display (stderr during a tsuku command), the file is rewritten with `shown: true`. `tsuku notices` reads all files regardless of `shown` state, sorted by timestamp.

Storage is bounded (one file per tool). The pattern mirrors `cache/updates/` exactly: per-tool JSON files with `ReadEntry`/`WriteEntry`/`ReadAllEntries` functions.

#### Alternatives considered

- **Per-event JSON files (Option A)**: One file per failure event (`<toolname>-<timestamp>.json`). Rejected because it accumulates files rapidly for repeatedly failing tools until Feature 7 adds suppression. The most recent failure is what matters for debugging.
- **Single append-only notices.json (Option B)**: All notices in one file. Rejected because concurrent auto-updates for different tools must serialize on the same file lock, and the append-read-write cycle is not atomic without locking.
- **Per-tool files with history array (Option D)**: Each file contains the last N failures. Rejected because the read-modify-write cycle adds complexity for a capability not required by R11a. Feature 7 can add a `consecutive_failures` integer field without changing the storage model.

## Decision Outcome

The three decisions form a complete auto-apply lifecycle:

A `MaybeAutoApply` function runs in PersistentPreRun, after the existing update check trigger. It attempts a non-blocking `TryLockExclusive` on `state.json.lock` -- if another tsuku process is running, it silently skips and the cached entries persist for the next command. When the lock is acquired, it reads all per-tool cache entries from `$TSUKU_HOME/cache/updates/`, filters for entries where `LatestWithinPin` is non-empty (an update is available) and `Error` is empty (the check succeeded), then installs each pending update via `runInstallWithTelemetry` with the tool's `Requested` constraint.

Before each install, the current `ActiveVersion` is snapshot into a new `PreviousVersion` field on `ToolState`. If the install fails (download, extraction, verification, or symlink creation), the system calls `Activate(toolName, previousVersion)` to switch symlinks back atomically -- the old version directory is still on disk. A failure notice is written to `$TSUKU_HOME/notices/<toolname>.json` with the tool name, attempted version, error message, and timestamp. The notice displays on stderr during the next tsuku command (one-time via a `shown` flag), and `tsuku notices` shows all recent notices regardless of shown state.

For manual rollback (runtime breakage), `tsuku rollback <tool>` reads `PreviousVersion` from state, verifies the directory exists, and calls `Activate()`. Rollback doesn't change `Requested`, so auto-update will re-attempt the update on the next cycle. If the upstream release is still broken, the cycle repeats: install -> verify -> fail -> auto-rollback. Feature 7 later adds consecutive-failure suppression to break this loop.

The cross-validation between D1 and D2 identified a locking coupling: `MaybeAutoApply` holds `TryLockExclusive` on `state.json.lock`, but `runInstallWithTelemetry` internally calls `state.UpdateTool` which also locks the same file. Since flock operates per-open-file-description, the internal lock would deadlock against the outer lock. The resolution: add `UpdateToolWithoutLock` to the `StateManager` (following the existing `loadWithoutLock`/`saveWithoutLock` precedent at `internal/install/state.go:251-310`). `MaybeAutoApply` holds a single `FileLock` for the entire apply cycle and passes a flag or uses the `WithoutLock` variants for state mutations within that scope. The `Activate()` rollback path also calls `UpdateTool`, so the same fix applies there.

The architecture review also identified a dependency direction issue: `MaybeAutoApply` in `internal/updates/` can't import `cmd/tsuku/` to call `runInstallWithTelemetry`. The resolution: `MaybeAutoApply` accepts a callback `installFn func(toolName, version, constraint string) error` that `main.go` provides by wrapping `runInstallWithTelemetry`. This follows the existing `OnEvalDepsNeeded` callback precedent in `cmd/tsuku/install_deps.go`.

## Solution Architecture

### Overview

Four components compose the solution: an apply function in `internal/updates/`, a rollback command, a notices package, and modifications to state tracking. The apply function reuses the existing install flow; rollback reuses the existing `Activate()` method. No new packages beyond `internal/updates/` (extended) and `internal/notices/` are needed.

### Components

**`internal/updates/apply.go`** (new file)
- `InstallFunc func(toolName, version, constraint string) error` -- callback type for the install flow, injected by `cmd/tsuku/main.go` wrapping `runInstallWithTelemetry`. Avoids `internal/` -> `cmd/` import.
- `MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config, installFn InstallFunc) error` -- the PersistentPreRun entry point. TryLock, read cache, install pending updates, handle failures.
- `applyUpdate(tool string, entry *UpdateCheckEntry, cfg *config.Config, installFn InstallFunc) error` -- installs a single tool update, handles auto-rollback on failure, writes notice on failure.

**`internal/notices/notices.go`** (new file)
- `Notice` struct: Tool, AttemptedVersion, Error, Timestamp, Shown
- `WriteNotice(noticesDir string, notice *Notice) error` -- atomic per-tool write
- `ReadAllNotices(noticesDir string) ([]Notice, error)` -- directory scan
- `ReadUnshownNotices(noticesDir string) ([]Notice, error)` -- filter for `shown == false`
- `MarkShown(noticesDir, toolName string) error` -- rewrite with `shown: true`
- `RemoveNotice(noticesDir, toolName string) error` -- cleanup

**`cmd/tsuku/cmd_rollback.go`** (new file)
- `tsuku rollback <tool>` command. Reads `PreviousVersion` from state, verifies directory exists, calls `mgr.Activate()`.

**`cmd/tsuku/cmd_notices.go`** (new file)
- `tsuku notices` command. Reads all notice files, displays details sorted by timestamp.

**`internal/install/state.go`** (modified)
- Add `PreviousVersion string` to `ToolState`
- Set `PreviousVersion` in `UpdateTool` when `ActiveVersion` changes

**`internal/install/manager.go`** (modified)
- `InstallWithOptions` snapshots `PreviousVersion` before setting new `ActiveVersion`

**`cmd/tsuku/main.go`** (modified)
- PersistentPreRun calls `updates.MaybeAutoApply` after `CheckAndSpawnUpdateCheck`

### Key Interfaces

```go
// Notice represents a failed auto-update for a single tool.
type Notice struct {
    Tool             string    `json:"tool"`
    AttemptedVersion string    `json:"attempted_version"`
    Error            string    `json:"error"`
    Timestamp        time.Time `json:"timestamp"`
    Shown            bool      `json:"shown"`
}

// InstallFunc is the callback type for the install flow.
// Injected by cmd/tsuku/main.go wrapping runInstallWithTelemetry.
type InstallFunc func(toolName, version, constraint string) error

// MaybeAutoApply is the PersistentPreRun entry point.
// On success, consumed cache entries are removed via RemoveEntry.
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config, installFn InstallFunc) error

// Extended ToolState
type ToolState struct {
    // ... existing fields ...
    PreviousVersion string `json:"previous_version,omitempty"`
}
```

### Data Flow

```
PersistentPreRun fires
  |
  v
MaybeAutoApply(cfg, userCfg, loader)
  |
  v
Check UpdatesAutoApplyEnabled() -- bail if false
  |
  v
TryLockExclusive(state.json.lock) -- bail if held
  |
  v
ReadAllEntries(cache/updates/) -> filter for LatestWithinPin != ""
  |
  v
For each pending update:
  |
  +-- Snapshot PreviousVersion = ActiveVersion
  |
  +-- runInstallWithTelemetry(tool, latestWithinPin, requested, ...)
  |     |
  |     +-- SUCCESS: ActiveVersion updated, PreviousVersion set
  |     |            Remove consumed cache entry
  |     |
  |     +-- FAILURE:
  |           |
  |           +-- Activate(tool, previousVersion) -- auto-rollback
  |           |
  |           +-- WriteNotice(notices/, {tool, version, error, now, false})
  |
  v
Release lock
  |
  v
Show unshown notices on stderr (one-time)
  |
  v
User's command runs
```

## Implementation Approach

### Phase 1: Notices and state changes

Add the `Notice` struct and per-tool file operations in `internal/notices/`. Add `PreviousVersion` to `ToolState`. Update `InstallWithOptions` to snapshot `PreviousVersion`. Unit tests for both.

Deliverables:
- `internal/notices/notices.go`
- `internal/notices/notices_test.go`
- `internal/install/state.go` (modified)
- `internal/install/manager.go` (modified)

### Phase 2: Rollback command

Add `tsuku rollback <tool>` and `tsuku notices` commands. Rollback reads `PreviousVersion`, verifies directory, calls `Activate()`. Notices reads and displays all notice files.

Deliverables:
- `cmd/tsuku/cmd_rollback.go`
- `cmd/tsuku/cmd_notices.go`

### Phase 3: Auto-apply integration

Add `MaybeAutoApply` in `internal/updates/apply.go`. Wire into PersistentPreRun. Handle auto-rollback on failure and notice writing. Address the state locking coupling (pass lock or use WithoutLock variants).

Deliverables:
- `internal/updates/apply.go`
- `internal/updates/apply_test.go`
- `cmd/tsuku/main.go` (modified)

## Security Considerations

**State mutation from cached data.** Auto-apply reads version strings from cache files and passes them to `runInstallWithTelemetry`. A poisoned cache file could specify an arbitrary version string. The existing install flow validates versions through the recipe and provider systems (checksums for recipes that define them), so the attack surface is the same as a manual `tsuku update` -- no new escalation.

**Auto-rollback preserves the previous version directory.** If an attacker can place a malicious binary in an old version directory (via filesystem access to `$TSUKU_HOME/tools/`), rollback would activate it. This is bounded by the same-user permission model: an attacker with write access to `$TSUKU_HOME` already controls the active binaries.

**Notice files are informational only.** They contain error messages and version strings, not executable content. A poisoned notice file could display misleading error messages, but can't cause code execution.

**Recipes without checksums have zero integrity protection.** Auto-apply reuses the install flow, which verifies checksums only for recipes that define them. Recipes with dynamic URL templates and no per-version checksums pass through unchecked. This isn't new (manual `tsuku update` has the same gap), but auto-apply increases exposure by automating the install cadence. This is the same gap documented in DESIGN-background-update-checks.md.

**Activate() failure during auto-rollback.** If `Activate()` itself fails (e.g., symlink creation error, disk full), the tool is left in the newly-installed (possibly broken) state with no automatic recovery. The failure is logged in the notice file, and the user must intervene manually via `tsuku install <tool>@<version>`. This is a degraded state, not a security vulnerability, but should be documented for completeness.

**Reduced user-awareness window.** Manual updates give the user a moment to evaluate what's changing. Auto-apply removes that window -- updates install without confirmation. Users who want awareness without auto-apply can set `updates.auto_apply = false` in config.toml and rely on notifications (Feature 5) instead.

**TryLock skip means deferred application.** If an attacker can hold `state.json.lock` indefinitely, auto-apply never runs. This is the same flock DoS vector identified in DESIGN-background-update-checks.md, bounded by the same-user permission model.

## Consequences

### Positive

- Tools stay current without manual intervention once shell integration is enabled
- Failed updates never break the user's workflow -- auto-rollback preserves the previous version
- `tsuku rollback` provides one-command recovery for runtime breakage
- Failure notices give visibility into what went wrong without cluttering normal output
- The existing install flow is reused without reimplementation
- Multi-version directory model makes rollback O(1) -- no re-download

### Negative

- Auto-apply adds latency to command startup when updates are pending (download + install time)
- The TryLock gate means auto-apply can be deferred indefinitely if the user always has a concurrent tsuku process
- Only one level of rollback history is tracked; users who need to go back further must use `tsuku install tool@version`
- Per-tool notice files lose earlier failure history when a tool fails repeatedly

### Mitigations

- Apply latency is bounded by the number of pending updates and their download size. Most updates are incremental (same tool, newer version). Feature 7 adds concurrent failure suppression to limit repeated install attempts.
- The TryLock skip-and-retry is by design: cached entries persist. In practice, concurrent tsuku processes are short-lived (install/update), so the window is small.
- One-level rollback matches R9. `tsuku install tool@version` is the escape hatch for deeper rollback.
- Feature 7 adds a `consecutive_failures` counter to notice files, capturing "how long has this been broken" without needing full history.

### Resilience to corruption

The auto-apply system follows the same self-healing principles as Feature 2:

- **Cache files missing or corrupt**: `ReadAllEntries` skips invalid files. No pending updates means no apply attempt.
- **Notice files missing or corrupt**: `ReadAllNotices` skips invalid files. Lost notices mean the user doesn't see a failure report, but the tool is still functional (auto-rollback succeeded independently).
- **PreviousVersion pointing to a deleted directory**: Rollback checks `os.Stat` on the directory. If missing, returns a clear error. The tool remains on the current version.
- **state.json.lock stale**: Advisory locks are kernel-managed and auto-release on process death. No stale lock cleanup needed.
- **Auto-apply interrupted mid-install**: The install flow uses atomic staging directories. An interrupted install leaves either the old version active or a complete new version -- never a partial state.
