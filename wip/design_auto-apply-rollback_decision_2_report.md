# Decision 2: Rollback mechanism and state tracking

## Question

How does `tsuku rollback <tool>` switch to the previous version, how is the previous version tracked in state, and how does the temporary nature (doesn't change Requested) interact with auto-update re-applying?

## Options Considered

### Option A: Add a PreviousVersion field to ToolState

Add a `PreviousVersion string` field to `ToolState` in state.json. When `InstallWithOptions` sets a new `ActiveVersion`, it snapshots the old `ActiveVersion` into `PreviousVersion`. Rollback reads `PreviousVersion`, calls `Activate()` to switch symlinks, and sets `ActiveVersion` back without touching `Requested`.

**State tracking:** Single field on ToolState. Updated atomically during install. Cleared on explicit user install (since the user chose this version deliberately). One level deep by design -- matches R9 requirement.

**Symlink switching:** Reuses the existing `Activate()` method, which already handles atomic symlink creation via `AtomicSymlink()` and updates `ActiveVersion` in state.

**Re-apply interaction:** Rollback sets `ActiveVersion` back to the old version but leaves `Requested` and `Versions` map unchanged. The new version's entry stays in the `Versions` map (its directory remains on disk). On the next auto-update cycle, the system resolves the latest version matching `Requested`, sees it differs from `ActiveVersion`, and re-applies the update. If the update succeeds verification, it sticks. If it fails again, auto-rollback (R10) reverts to the now-previous version.

**Edge cases:**
- Previous version directory deleted (garbage collected): Rollback checks `os.Stat` on the tool directory. If missing, returns an error telling the user the previous version is no longer available. This is a clean failure -- no partial state.
- PreviousVersion is empty (first install, no prior version): Rollback returns an error: "no previous version to roll back to."
- Multiple rapid updates: Only the immediately prior version is tracked. This is intentional per R9 (one level deep).

### Option B: Derive previous version from the Versions map sorted by InstalledAt

No new field. When rollback is requested, iterate `Versions`, sort by `InstalledAt`, find the entry installed just before the current `ActiveVersion`, and activate it.

**State tracking:** No state changes needed. The `Versions` map already stores `InstalledAt` timestamps. The helper `getMostRecentVersion()` in remove.go already does similar logic.

**Symlink switching:** Same as Option A -- call `Activate()`.

**Re-apply interaction:** Same as Option A -- `Requested` is untouched, auto-update will re-apply.

**Problems:**
- Ambiguous "previous": If a user installs v1, then v2, then explicitly installs v1 again (reinstall), `InstalledAt` for v1 is now newer than v2. "Previous" by timestamp would point to v2, which may not be what the user expects. The semantics of "previous" become "second most recently installed" rather than "the version that was active before the current one."
- Reinstalls update `InstalledAt`: The current `InstallWithOptions` code always writes a new `VersionState` with `InstalledAt: time.Now()`, even for reinstalls. This means timestamps don't reliably reflect the activation order.
- Garbage collection interaction: If intermediate versions are removed, the "second most recent" may jump to an unexpected version with no way to know it wasn't the actual predecessor.

### Option C: Keep a separate rollback log file

Maintain a file like `$TSUKU_HOME/rollback.json` mapping tool names to their rollback target version. Updated whenever ActiveVersion changes.

**State tracking:** Separate file, separate locking. Written during install/update.

**Symlink switching:** Same as Option A -- call `Activate()`.

**Re-apply interaction:** Same as Option A.

**Problems:**
- Adds a second source of truth that can drift from state.json.
- Requires coordinating file locks across two files during install.
- The rollback.json file needs its own migration and cleanup logic.
- Overkill for a single field per tool.

### Option D: Add a VersionHistory list to ToolState

Track a `VersionHistory []string` recording the sequence of activated versions. Rollback pops the stack. Richer than Option A -- supports multi-level rollback in the future.

**State tracking:** Append to the list on every activation. Rollback pops.

**Symlink switching:** Same as Options A/B/C.

**Re-apply interaction:** Same as Option A.

**Problems:**
- R9 explicitly requires one-level-deep rollback. A history list invites feature creep and adds unbounded state growth.
- The list needs pruning logic to avoid growing forever.
- Adds complexity with no current requirement to justify it.

## Chosen

**Option A: Add a PreviousVersion field to ToolState.**

## Rationale

Option A is the simplest approach that directly satisfies R9 (one-level rollback) and D7 (rollback doesn't change Requested). The field is explicit about its purpose, trivial to implement, and its behavior is obvious to anyone reading the code or inspecting state.json.

Option B (deriving from timestamps) fails in the reinstall case. When a user reinstalls the same version or when auto-apply reinstalls after a rollback, `InstalledAt` gets overwritten, making timestamp-based derivation unreliable. Fixing this would require either making `InstalledAt` immutable (breaking reinstall semantics) or tracking a separate `ActivatedAt` timestamp -- at which point you've reinvented Option A with more complexity.

Option C (separate file) adds coordination overhead for no benefit over a single field in the existing state structure.

Option D (version history) violates YAGNI. R9 is explicit about one-level-deep rollback. If multi-level rollback becomes a requirement later, migrating from PreviousVersion to a history list is straightforward.

The concrete implementation:

1. Add `PreviousVersion string json:"previous_version,omitempty"` to `ToolState`.
2. In `InstallWithOptions`, before setting `ts.ActiveVersion = version`, snapshot: `ts.PreviousVersion = ts.ActiveVersion` (only if `ts.ActiveVersion != ""` and `ts.ActiveVersion != version`).
3. In the `Activate()` method (used by rollback), similarly update PreviousVersion.
4. The `rollback` command calls `Activate(name, toolState.PreviousVersion)` after verifying the directory exists.
5. On explicit `tsuku install <tool>@<version>`, clear PreviousVersion (the user made a deliberate choice; rollback from an intentional pin doesn't make sense).

The re-apply loop works naturally: rollback switches `ActiveVersion` back to the old version without touching `Requested`. The next auto-update resolves the latest version for that `Requested` constraint, sees it differs from `ActiveVersion`, installs and verifies. If verification passes, `ActiveVersion` moves forward and `PreviousVersion` is set to the rolled-back version. If verification fails (R10), auto-rollback uses the same mechanism to revert.

## Assumptions

- R9's "one level deep" means we only need to track the single immediately preceding active version, not a full history.
- Garbage collection of old version directories will check `PreviousVersion` before removing a directory (or rollback will gracefully error if the directory is missing).
- Auto-apply updates go through the same `InstallWithOptions` path, so `PreviousVersion` is set consistently whether the update was manual or automatic.
- Clearing `PreviousVersion` on explicit install is acceptable because explicit installs represent intentional user decisions, not something to roll back from automatically.
