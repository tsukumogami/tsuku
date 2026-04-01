# Decision 3: Remove and Update Lifecycle Awareness

## Question

How should the remove and update flows gain lifecycle awareness? Specifically: hook ordering, failure handling, multi-version behavior, and interaction with the install flow.

## Context

The current `RemoveVersion` and `RemoveAllVersions` methods in `internal/install/remove.go` delete tool directories and symlinks. They don't run any cleanup beyond that -- no shell.d file removal, no completions cleanup, no pre-remove actions. The `update` command in `cmd/tsuku/update.go` calls `runInstallWithTelemetry` which does a full reinstall; it has no concept of cleaning up old version artifacts before installing.

The exploration already decided:
- Cleanup instructions are stored in state at install time
- Hooks fail gracefully (warn, don't block)
- Declarative hooks only (limited action vocabulary)
- Remove flow currently doesn't load recipes

Key code observations:
- `RemoveVersion` reads `ToolState` / `VersionState` from `state.json` but never consults recipes
- `RemoveAllVersions` iterates all versions and calls `removeToolEntirely`, which removes symlinks and state
- The `removeCmd` in `cmd/tsuku/remove.go` loads the recipe only *after* removal succeeds, for dependency cleanup
- `VersionState` already stores a `Plan` with the full list of `PlanStep` actions executed during install
- Update is essentially "install over the top" -- old version directories get overwritten or a new version directory is created alongside

## Options Evaluated

### Option A: State-driven cleanup (no recipe at remove time)

Install records cleanup actions as a new field on `VersionState` (e.g., `CleanupActions []CleanupAction`). Each cleanup action is a declarative instruction: `{action: "delete_file", path: "$TSUKU_HOME/share/shell.d/niwa.bash"}`. Remove reads these from state and executes them before deleting the tool directory.

**1. Tools installed before lifecycle hooks existed:**
No `CleanupActions` entries in state. Remove proceeds as today -- directory + symlink deletion. No regression. Tools installed after the feature get full cleanup automatically. Migration path: `tsuku reinstall` populates cleanup state.

**2. Multi-version (v1 and v2, removing v1):**
Each `VersionState` carries its own `CleanupActions`. When removing v1, only v1's cleanup actions execute. If v1 and v2 both wrote `shell.d/niwa.bash`, the cleanup action includes a guard: skip deletion if another installed version references the same path. Implementation: before deleting a shared file, scan other versions of the same tool for a matching cleanup path. This is cheap since `state.json` is already loaded.

**3. Update flow ordering:**
Update becomes: (1) install new version, (2) run new version's post-install hooks, (3) run old version's pre-remove cleanup filtered to exclude paths the new version also claims, (4) delete old version directory. The new version installs first so there's no gap where shell.d files are missing. Old cleanup runs after to remove stale paths only.

**4. Failure handling:**
Each cleanup action executes independently. If one fails (file doesn't exist, permission denied), log a warning and continue with remaining actions. Cleanup errors don't block removal. After all cleanup attempts, the tool directory is deleted regardless.

**5. Concrete example -- niwa with shell.d/niwa.bash:**
At install time, the `write_file` action that creates `$TSUKU_HOME/share/shell.d/niwa.bash` records a corresponding `CleanupAction{Action: "delete_file", Path: "$TSUKU_HOME/share/shell.d/niwa.bash"}` in `VersionState`. On `tsuku remove niwa`: state is loaded, cleanup action executes (`os.Remove` on the shell.d file), then `removeToolEntirely` deletes directory + symlinks + state.

### Option B: Recipe-consulted cleanup

Remove loads the recipe from the registry cache, filters steps by a `phase = "pre-remove"` qualifier on the WhenClause, and executes them through the existing executor.

**1. Tools installed before lifecycle hooks existed:**
Works if the recipe has been updated to include pre-remove steps. But the recipe version at remove time may differ from what was installed. If registry is unavailable or recipe deleted, no cleanup runs -- silent data loss.

**2. Multi-version:**
The recipe defines cleanup for the tool generically, not per-version. A single recipe can't express "only clean up if no other version still needs this file." Would need to add version-awareness to recipe steps, which adds complexity.

**3. Update flow ordering:**
Update loads recipe, runs pre-remove phase for old version, then installs new version with post-install phase. But the recipe may have changed between the old and new version. Running updated recipe's pre-remove against an old installation could produce incorrect results.

**4. Failure handling:**
If recipe is unavailable (offline, cache stale, recipe removed), no cleanup runs at all. This violates the exploration decision that cleanup should be reliable. Fallback to "no cleanup" defeats the purpose.

**5. Concrete example:**
On `tsuku remove niwa`, the loader fetches the niwa recipe, finds steps with `phase = "pre-remove"`, runs them. If the recipe was updated to add different shell.d paths since installation, the wrong files could be targeted.

### Option C: Hybrid (state for common, recipe for complex)

State records cleanup for standard patterns (file deletions, directory removal). For complex cleanup that can't be expressed declaratively, remove loads the recipe and executes special pre-remove steps. If recipe is unavailable, state-only cleanup still runs.

**1. Tools installed before lifecycle hooks existed:**
State-tracked cleanup absent, so falls through to recipe lookup. If recipe has pre-remove hooks, those run. If neither exists, behaves as today. Slightly better coverage than A alone.

**2. Multi-version:**
State-tracked actions are per-version (same as A). Recipe-based actions are generic and have the same version-awareness problem as B.

**3. Update flow ordering:**
State cleanup runs reliably. Recipe cleanup runs if available. Two cleanup sources create ordering questions: which runs first? What if they conflict (both try to delete the same file)?

**4. Failure handling:**
More complex failure surface. State cleanup can fail independently from recipe cleanup. Need to define precedence when both specify actions for the same path.

**5. Concrete example:**
niwa's shell.d file deletion is handled by state (common pattern). If niwa had a complex cleanup action (e.g., deregistering from a service), that would come from the recipe. Two code paths for cleanup in a single remove operation.

## Recommendation

**Option A: State-driven cleanup.**

Rationale:

**Aligns with exploration decisions.** The exploration explicitly decided "store cleanup instructions in state at install time" and noted "remove flow doesn't load recipes today." Option A is the direct implementation of these decisions. Options B and C walk back this decision by reintroducing recipe loading at remove time.

**Reliability over flexibility.** State-driven cleanup is deterministic: what was recorded at install time is what gets cleaned up. Recipe-based cleanup (B, C) introduces version skew between the recipe that installed the tool and the recipe available at remove time. For a package manager, predictable cleanup matters more than flexible cleanup.

**Simplicity.** Option A adds one new field to `VersionState` and one new loop to the remove flow. Option C adds that plus recipe loading, executor invocation, and conflict resolution. The hybrid's added complexity doesn't pay for itself -- the declarative action vocabulary already covers the use cases identified in the exploration (file/directory creation, shell.d scripts, completions).

**Multi-version handling is natural.** Per-version cleanup actions in `VersionState` make the multi-version case straightforward. Cross-referencing other versions' cleanup paths before deletion handles shared resources without recipe involvement.

**Graceful degradation for legacy installs.** No cleanup state means no cleanup actions -- same as today. Users can `tsuku reinstall` to populate cleanup state. No silent misbehavior, no broken remove for existing installs.

The one advantage of B/C -- handling cleanup patterns that can't be expressed declaratively -- isn't needed yet. The exploration scoped lifecycle hooks to Level 1 (declarative only). If Level 2 cleanup actions become necessary in the future, the state-driven approach can be extended: store more expressive cleanup instructions in state rather than falling back to recipes.

## Implementation Sketch

**State schema change:**
```go
type CleanupAction struct {
    Action string `json:"action"`           // "delete_file", "delete_dir"
    Path   string `json:"path"`             // Absolute path or $TSUKU_HOME-relative
}

type VersionState struct {
    // ... existing fields ...
    CleanupActions []CleanupAction `json:"cleanup_actions,omitempty"`
}
```

**Install flow change:**
When executing a post-install action that creates files outside the tool directory (shell.d scripts, completions), record a corresponding `CleanupAction` in the `VersionState` being built.

**Remove flow change (in `RemoveVersion` / `RemoveAllVersions`):**
Before deleting tool directories, iterate `CleanupActions` for the version(s) being removed. For each action:
1. Check if any *other* installed version of the same tool has a matching cleanup path. If so, skip (shared resource still in use).
2. Execute the cleanup action (e.g., `os.Remove` for `delete_file`).
3. On failure, log a warning and continue.

**Update flow change:**
1. Install new version (creates new `VersionState` with fresh `CleanupActions`).
2. Compute stale cleanup: actions in old version's `CleanupActions` that don't appear in new version's `CleanupActions`.
3. Execute stale cleanup actions (removes files the new version no longer creates).
4. Remove old version directory.

This ordering ensures no gap in shell.d coverage during update.
