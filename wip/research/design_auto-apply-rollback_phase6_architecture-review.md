# Architecture Review: DESIGN-auto-apply-rollback.md

## Summary Verdict

The design is implementable and fits the existing architecture well. It reuses the install flow, mirrors the cache pattern for notices, and extends state.json with a consumer-backed field. Two structural issues need resolution before implementation: the locking coupling is real and underspecified, and `runInstallWithTelemetry` lives in `cmd/tsuku` which creates a dependency direction problem for `internal/updates/apply.go`.

---

## Question 1: Is the architecture clear enough to implement?

**Yes, with one gap.** The data flow diagram, component list, and interface signatures give an implementer enough to start. The gap is the `MaybeAutoApply` signature:

```go
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config, loader RecipeLoader) error
```

This function needs to call `runInstallWithTelemetry`, which is defined in `cmd/tsuku/install_deps.go` (package `main`). The design places `MaybeAutoApply` in `internal/updates/apply.go`. An `internal/` package cannot import `cmd/tsuku` -- this is a dependency direction violation. The design must specify how the install flow is made callable from `internal/updates/`. Options:

1. **Callback injection**: `MaybeAutoApply` accepts a `func(tool, version, constraint string) error` that `cmd/tsuku` provides, wrapping `runInstallWithTelemetry`. This is the lightest touch.
2. **Extract install logic to `internal/`**: Move the core of `installWithDependencies` out of `cmd/tsuku` into an internal package. Correct long-term but high blast radius for this feature.
3. **Keep `MaybeAutoApply` in `cmd/tsuku`**: Sidesteps the problem entirely but breaks the stated architecture ("new file in `internal/updates/`").

**Recommendation**: Option 1. The callback pattern is already used elsewhere (e.g., `OnEvalDepsNeeded` in `install_deps.go:131`). The design should specify the callback signature and that wiring happens in `main.go`'s `PersistentPreRun`.

**Severity: Blocking.** Without resolving this, the implementer will either put business logic in `cmd/` or create a circular import.

---

## Question 2: Are there missing components or interfaces?

### 2a. `Requested` field location (Advisory)

The design's data flow says "installs each pending update via `runInstallWithTelemetry` with the tool's `Requested` constraint." But `Requested` lives on `VersionState`, not `ToolState`. The cache entry already carries `Requested` (populated by the checker from `Versions[ActiveVersion].Requested`). The apply function should read `Requested` from the cache entry, not from state. The design implicitly assumes this but doesn't state it -- minor clarification needed.

### 2b. Cache entry consumption (Advisory)

The data flow shows "Remove consumed cache entry" on success. This means calling `updates.RemoveEntry()`. The design lists this in the flow but doesn't mention it in the component descriptions or the `applyUpdate` signature. Should be explicit.

### 2c. `Activate` also acquires `state.json.lock` (Blocking)

The design identifies the locking coupling between `MaybeAutoApply` and `UpdateTool`, but `Activate()` (used for auto-rollback) also calls `UpdateTool` internally (line 274 of `manager.go`). So the rollback path has the same deadlock risk. The design's locking resolution must cover both the install path AND the rollback path.

### 2d. Missing `RecipeLoader` type in `internal/updates` (Advisory)

The `MaybeAutoApply` signature uses `RecipeLoader` but `internal/updates` doesn't define or import this type. If using the callback approach from Q1, this parameter may not be needed at all (the callback closure captures it).

---

## Question 3: Are the implementation phases correctly sequenced?

**Yes.** The phasing is sound:

- **Phase 1** (notices + `PreviousVersion`) has no dependencies on the other phases and is independently testable.
- **Phase 2** (rollback + notices commands) depends on Phase 1's state changes and notice package. Correct ordering.
- **Phase 3** (auto-apply) depends on both Phase 1 (`PreviousVersion` for rollback) and Phase 2's patterns (notice writing). Correct that it's last.

One observation: Phase 2 commands (`tsuku rollback`, `tsuku notices`) can ship and be useful independently of auto-apply. Users can manually trigger scenarios that populate `PreviousVersion` (install v1, install v2, rollback to v1). This is a clean incremental delivery.

---

## Question 4: Does the locking coupling have a workable resolution?

**Yes, but the design underspecifies the chosen approach.** The design acknowledges the problem and lists three options without choosing one. Here's the analysis:

### Current locking architecture

`UpdateTool` creates a new `FileLock` instance each time (line 12 of `state_tool.go`):
```go
lock := NewFileLock(sm.lockPath())
if err := lock.LockExclusive(); err != nil { ... }
```

On Linux, `flock` operates on the open file description. Two `NewFileLock` calls on the same path create two separate file descriptions. If the outer `TryLockExclusive` succeeds, the inner `LockExclusive` in `UpdateTool` will also succeed because flock is per-process reentrant on Linux (same process, different fd = succeeds). **Wait -- this is wrong.** `flock` on Linux is per-open-file-description, and two separate `open()` calls on the same file create two descriptions. A process CAN hold multiple flocks on the same file via different fds. So the inner lock would actually succeed on Linux.

However, this creates a subtle bug: the inner `Unlock` (in `UpdateTool`'s defer) would release the lock *while `MaybeAutoApply` still expects to hold it*. The outer lock's fd is different, so the outer lock remains. Actually, since each `FileLock` opens its own fd, each `Unlock` closes its own fd. The outer lock persists until its own `Unlock`.

**Net assessment**: On Linux/macOS, the current code would not deadlock because flock is per-fd and the same process can hold multiple flocks. But the semantic is sloppy -- the outer TryLock claims exclusivity, then inner operations acquire and release their own exclusive locks on separate fds. Between the inner unlock and the next inner lock, a third process could briefly acquire the lock.

**Recommended resolution**: The `WithoutLock` variants already exist (`loadWithoutLock`, `saveWithoutLock`). Add `UpdateToolWithoutLock` that skips flock acquisition (caller is responsible for holding the lock). `MaybeAutoApply` acquires the lock once via `TryLockExclusive` and passes the `FileLock` (or a flag) to the install flow. This is the cleanest approach and matches the existing `WithoutLock` pattern in `state.go`.

**Severity: Blocking.** The design must pick an approach. The `WithoutLock` variant path is the architecturally consistent choice.

---

## Question 5: Does the design compose with existing patterns?

### Install flow reuse (Good)

The design reuses `runInstallWithTelemetry` rather than reimplementing download/extract/verify. This is correct. The function already handles dependency resolution, atomic staging, symlink creation, and state updates.

### State management (Good)

`PreviousVersion` on `ToolState` follows the existing pattern: a field on the struct, set during `UpdateTool` callbacks, serialized to `state.json`. The field has two consumers (auto-rollback reads it, `tsuku rollback` reads it), so it's not dead state. The `omitempty` annotation means existing state files remain valid.

### Feature 2 cache (Good)

The design correctly treats cache entries as the input signal. `ReadAllEntries` already exists and skips corrupt files. `RemoveEntry` exists for consumption. No new cache format needed.

### Notices pattern (Good)

`internal/notices/` mirrors `internal/updates/cache.go` exactly: per-tool JSON files with `ReadEntry`/`WriteEntry`/`ReadAllEntries`. Same atomic write pattern (temp + rename). Same directory scan with skip-on-error. This is pattern reuse, not a parallel pattern.

### PersistentPreRun integration (Good)

The existing `PersistentPreRun` already loads `config.DefaultConfig()` and `userconfig.Load()` for the update check trigger. Adding `MaybeAutoApply` after `CheckAndSpawnUpdateCheck` is a natural extension. The skip list is shared.

### CLI surface (Good)

Two new commands (`rollback`, `notices`) don't overlap with existing commands. `rollback` is distinct from `activate` (which switches to any installed version; rollback specifically targets the previous version and doesn't change `Requested`). `notices` is new functionality with no existing equivalent.

---

## Findings Summary

| # | Finding | Severity | Location |
|---|---------|----------|----------|
| 1 | `MaybeAutoApply` in `internal/updates/` cannot call `runInstallWithTelemetry` in `cmd/tsuku` -- dependency direction violation. Use callback injection. | Blocking | Design: Solution Architecture, Components |
| 2 | Locking coupling is real but underspecified. `Activate` rollback path has the same coupling. Pick `WithoutLock` variants. | Blocking | Design: Decision Outcome, locking paragraph |
| 3 | `Requested` should be read from cache entry, not state. Design is ambiguous. | Advisory | Design: Data Flow |
| 4 | Cache entry removal on success should be explicit in component descriptions. | Advisory | Design: Components |
| 5 | `RecipeLoader` type undefined in `internal/updates`. Callback approach may eliminate it. | Advisory | Design: Key Interfaces |

## Recommendations

1. **Add a callback type for install execution.** Define something like `type InstallFunc func(tool, version, constraint string) error` in `internal/updates/apply.go`. `MaybeAutoApply` accepts it as a parameter. `cmd/tsuku/main.go` wires it by closing over `runInstallWithTelemetry`. This matches the `OnEvalDepsNeeded` precedent.

2. **Commit to `WithoutLock` variants for the locking resolution.** Add `UpdateToolWithoutLock` and document that `MaybeAutoApply` holds a single `FileLock` for the entire apply cycle. Both the install path and the rollback path must use the without-lock variants.

3. **Clarify that `Requested` comes from the cache entry.** The cache entry's `Requested` field was populated by the checker from `Versions[ActiveVersion].Requested`. The apply function should pass `entry.Requested` as the version constraint to the install callback.
