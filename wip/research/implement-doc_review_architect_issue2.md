---
issue: 2
title: "feat(state): add source tracking to ToolState"
reviewer: architect-reviewer
focus: structural fit, dependency direction, state contract, pattern consistency
---

# Architecture Review: Issue 2 (Source Tracking)

## Verdict: 0 blocking, 2 advisory

The implementation fits the existing architecture cleanly. The new field lives in the state contract (`internal/install/state.go`), migration follows the established pattern (`migrateToMultiVersion`), and the mapping function sits at the CLI layer where recipe-loading already happens. No dependency direction violations, no parallel patterns introduced.

## Findings

### Finding 1: State contract field with no current consumer -- Advisory

`internal/install/state.go:86` -- `Source string` is added to `ToolState` but nothing in the codebase currently reads it. Under normal circumstances this would be a blocking state contract violation (orphaned field = schema drift). However, this is explicitly a foundation issue in a declared multi-issue plan: Issues 7-9 are the consumers (install integration, update/outdated/verify routing, display). The field is exercised by its own migration and tested via round-trip serialization.

This is acceptable as a staged delivery. It would become blocking if Issues 7-9 were abandoned without removing the field.

### Finding 2: ToolState.Source comment lists "embedded" but nothing produces it -- Advisory

`internal/install/state.go:84` -- The comment reads `Values: "central" (default registry and embedded), "local", or "owner/repo" (distributed)`. This is accurate post-scrutiny-fix: the comment no longer lists "embedded" as a distinct value.

However, the migration comment at `internal/install/state_tool.go:125` still documents the `"embedded" -> "central"` mapping as a case the migration handles. In the actual switch statement (line 144), only `case "local"` exists -- everything else falls through to the default "central". The comment is correct documentation of the design intent, but a reader might wonder why "embedded" is mentioned in the comment but not in the switch. This is cosmetic -- the behavior is right and no code path depends on it.

## Structural Assessment

### Dependency direction: Correct

`internal/install/state_tool.go` imports only `fmt`. The mapping function `recipeSourceFromProvider()` lives in `cmd/tsuku/helpers.go`, which imports `internal/recipe` -- correct direction (CLI imports internal, never the reverse).

### Migration pattern: Consistent

`migrateSourceTracking()` follows the exact same pattern as `migrateToMultiVersion()`:
- Called in both `loadWithLock()` (line 197) and `loadWithoutLock()` (line 275)
- Idempotent (skips entries with non-empty Source)
- Mutates the in-memory State struct, persisted on next Save()
- Method on `*State`, not on `*StateManager`

The scrutiny fix correctly added the migration call to `loadWithoutLock()`, preventing the divergent-twins issue where `UpdateTool`/`RemoveTool` paths would skip migration.

### No parallel patterns

- No new packages introduced
- No new interfaces or abstractions
- No duplicate config parsing or error types
- The mapping function is a simple pure function, not a new dispatch mechanism

### State contract forward compatibility

The `omitempty` JSON tag means empty Source values don't appear in serialized state, preserving backward compatibility with older tsuku versions that don't know about the field. The migration handles the forward path (old state -> new state). Clean in both directions.

## Downstream Architectural Notes (for Issues 7-9 implementers)

1. **Issue 7 should set `ts.Source` explicitly in `UpdateTool` callbacks.** The current implementation relies entirely on lazy migration from `Plan.RecipeSource`. This works but adds coupling between install-time behavior and load-time migration. Setting the field directly during install is cleaner and eliminates the window where Source is empty in memory.

2. **Dependency installs hardcode `RecipeSource: "registry"` in `install_deps.go:125` and `install_lib.go:116`.** When distributed sources arrive, deps pulled from distributed registries will be incorrectly tagged. Issue 7's implementer should update these paths.

3. **The `default` branch in `recipeSourceFromProvider` (line 169) passes through the raw source string.** This is the intended extension point for distributed sources (`"owner/repo"`). The pattern is ready for Issue 5-6 without modification.
