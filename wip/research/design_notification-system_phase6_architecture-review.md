# Architecture Review: Notification System Design

**Design:** `docs/designs/DESIGN-notification-system.md`
**Reviewer role:** Architect (structural fit)
**Date:** 2026-04-01

## Summary Verdict

The design fits the existing architecture well. It extends `internal/updates/` rather than introducing a parallel package, reuses existing data sources (`notices/` files, cache entries), and threads the cmd/internal boundary correctly via parameter injection. Four findings below, two advisory, one blocking, one structural note.

---

## Finding 1: displayUnshownNotices is private but design references it as public

**Severity: Advisory**

The design document repeatedly references `DisplayUnshownNotices` (capitalized, exported) as the function being replaced. The actual function in `internal/updates/apply.go:127` is `displayUnshownNotices` (unexported). This is a documentation inaccuracy, not a code problem. The replacement `DisplayNotifications` will be correctly exported since it's called from `cmd/tsuku/main.go`.

The design's plan to extract the display call from inside `MaybeAutoApply` to the cmd layer is structurally correct -- it separates "apply updates" from "show what happened," which are distinct responsibilities that currently happen to be co-located.

**Action:** Fix naming in design doc to match the actual unexported function. No code impact.

---

## Finding 2: MaybeAutoApply gains a suppression dependency it shouldn't need

**Severity: Blocking**

The design says (Phase 2 deliverables): "Add `ShouldSuppressNotifications(quietFlag)` guard around the inline 'Updated X' lines in `MaybeAutoApply`."

This introduces a display concern into an apply function. `MaybeAutoApply` currently has no output responsibility -- `displayUnshownNotices` handles failure display, and success is silent. The design proposes making `MaybeAutoApply` emit "Updated tool X old -> new" lines, gated by suppression.

This is a structural problem because:
1. `MaybeAutoApply` doesn't currently receive `quiet` and shouldn't need to. Its signature is `(cfg, userCfg, installFn)` -- pure infrastructure.
2. Adding output to `MaybeAutoApply` means callers need to reason about whether it prints. The existing pattern is clean: apply writes state/files, display reads state/files.
3. If `DisplayNotifications` already runs after `MaybeAutoApply`, it can detect and render applied updates by comparing cache state before/after, or by having `MaybeAutoApply` return a result slice.

**Recommended alternative:** Have `MaybeAutoApply` return `[]ApplyResult` (tool, oldVersion, newVersion, error). `DisplayNotifications` renders both successes and failures from the result set. This keeps the apply/display separation clean and avoids threading `quiet` into `MaybeAutoApply`.

The result type already exists in embryonic form (`applyResult` at apply.go:112) -- it just needs to be extended with tool/version fields and returned to the caller.

---

## Finding 3: Sentinel file in ReadAllEntries directory needs filter

**Severity: Advisory**

The `.notified` sentinel file lives in `$TSUKU_HOME/cache/updates/`, the same directory that `ReadAllEntries` scans. `ReadAllEntries` in `cache.go:84-89` already skips directories and non-JSON files (it filters for `.json` suffix based on the skip logic at line 89). So the sentinel won't be parsed as a cache entry.

However, the design should explicitly note this interaction. If a future implementer changes `ReadAllEntries` to be less strict about filtering, the sentinel would cause a parse error. The existing filter is sufficient -- this is an observation, not a required change.

---

## Finding 4: PostRun supplement duplicates PreRun "available updates" logic

**Severity: Advisory**

`DisplayAvailableSummary` (PostRun) and the available-updates portion of `DisplayNotifications` (PreRun) both need to: read cache entries, count tools with updates, check the sentinel, call the suppression gate. The design describes these as two separate functions with overlapping logic.

This isn't a parallel-pattern problem in the architectural sense -- both functions live in `internal/updates/` and share the same data sources. But the implementation should extract the "count available updates and check sentinel" logic into a shared helper to avoid the duplication. The design's component diagram shows this as two separate call paths, which is fine at the design level. The implementer should recognize the shared core.

---

## Structural Fit Assessment

### What fits well

- **No new package.** The design explicitly keeps changes in `internal/updates/`, extending existing infrastructure. This avoids the parallel-package antipattern.
- **cmd/internal boundary.** The `quiet bool` parameter injection is the right pattern. The codebase already uses this approach (e.g., `installFn` callback injection in `MaybeAutoApply`).
- **Data source reuse.** All four notification types use existing on-disk formats: `Notice` structs in `$TSUKU_HOME/notices/`, `UpdateCheckEntry` in cache. No new serialization format.
- **Suppression gate placement.** Putting `ShouldSuppressNotifications` in `internal/updates/` rather than `userconfig` is correct. The userconfig methods answer "should this subsystem run?" while the gate answers "should output be visible?" Different questions, same signals, different locations. The design explains this trade-off well.
- **`progress.IsTerminalFunc` reuse.** Using the existing TTY detection variable maintains the single pattern for terminal checks.

### Dependency direction

The new code in `internal/updates/` will import `internal/notices/` (already happens), `internal/config/` (already happens), and `internal/progress/` (new, but same layer). No upward dependencies into `cmd/`. Clean.

### Phase sequencing

The three phases are correctly ordered:
1. Suppression gate (no external dependencies, fully testable in isolation)
2. Unified renderer (depends on phase 1, requires editing existing call sites)
3. PostRun supplement and integration tests (depends on phase 2, adds optional behavior)

Phase 3 is explicitly marked as droppable if PostRun adds complexity. This is good defensive design.

---

## Answers to Specific Questions

### 1. Is the architecture clear enough to implement?

Yes. The component diagram, function signatures, data flow, and phase breakdown are sufficient. One gap: the design doesn't specify what happens to the existing `displayUnshownNotices` call inside `MaybeAutoApply` (apply.go:108). The Phase 2 deliverables say "remove `DisplayUnshownNotices`" but should specify: remove the call at line 108, delete the function at lines 127-138, and replace both with the new `DisplayNotifications` called from main.go after `MaybeAutoApply` returns.

### 2. Are there missing components or interfaces?

One missing interface: `MaybeAutoApply` return type. Per Finding 2, the function should return apply results so that `DisplayNotifications` can render success messages without `MaybeAutoApply` having output responsibility. The existing `applyResult` struct needs tool/version fields and the function needs to return `[]ApplyResult`.

### 3. Are the implementation phases correctly sequenced?

Yes. Each phase depends only on the previous one. Phase 1 is independently testable. Phase 3 is explicitly optional. No circular dependencies between phases.

### 4. Are there simpler alternatives we overlooked?

The design is already the simple option. The considered alternatives (notification context struct, per-entry notified field, all-PostRun rendering) were correctly rejected for adding complexity without proportional benefit.

One minor simplification: the "available updates" PostRun supplement (Phase 3) could be deferred entirely to a future iteration. The PreRun path handles everything. The PostRun supplement exists only to show the summary "after command output" on happy paths -- a polish item, not a functional gap. Deferring it would reduce the implementation to two phases.
