---
issue: 2
title: "feat(state): add source tracking to ToolState"
reviewer: architect-reviewer
focus: completeness
---

## Architecture Review: Issue 2 Source Tracking

### Summary

The implementation adds a `Source` field to `ToolState` with lazy migration and a `recipeSourceFromProvider()` mapping function. The structural approach is sound: the field lives in the state contract (`internal/install/state.go`), migration runs alongside the existing `migrateToMultiVersion()` in `Load()`, and the mapping logic sits in `cmd/tsuku/helpers.go` at the CLI layer. No new packages, no dependency direction violations, no parallel patterns.

### Findings

**Finding 1: ToolState.Source is never set directly on new installs -- Advisory**

`internal/install/manager.go:169-195` -- The `UpdateTool` callback in `InstallWithOptions` sets `ActiveVersion`, `Versions`, `Binaries`, etc., but never sets `ts.Source`. Instead, `recipeSourceFromProvider()` writes to `Plan.RecipeSource`, and `ToolState.Source` is populated indirectly by `migrateSourceTracking()` on the next `Load()`.

This works because:
1. No current consumer reads `ToolState.Source` (Issues 7-9 are the intended consumers)
2. `Load()` always runs before any read, so migration fires before first use
3. The `omitempty` JSON tag means empty Source doesn't produce junk in state.json

However, the indirection is surprising and the mapping's evidence claim ("helpers.go recipeSourceFromProvider()") is misleading -- that function populates `Plan.RecipeSource`, not `ToolState.Source`. The migration is the actual mechanism. This is contained (no other code copies the pattern), so advisory, not blocking.

If Issue 7 intends to set `ToolState.Source` explicitly during install (which would be the cleaner path), it should also set it for the existing central/embedded/local cases to eliminate the migration dependency for new installs.

**Finding 2: Dependency installs hardcode RecipeSource to "registry" -- Advisory**

`cmd/tsuku/install_deps.go:125` -- `RecipeSource: "registry"` is hardcoded for all dependency installations. For the current system this is correct (all deps come from central). But when distributed sources arrive (Issues 5-7), a dependency pulled from a distributed registry will be incorrectly tagged as "registry" -> "central". This is outside Issue 2's scope but worth noting for Issue 7's implementer.

**Finding 3: install_lib.go also hardcodes RecipeSource -- Advisory**

`cmd/tsuku/install_lib.go:116` -- Same pattern as Finding 2 for library installs. Again, correct now, flagging for downstream awareness.

### Mapping Verification

| AC | Mapping Claim | Verified | Notes |
|----|--------------|----------|-------|
| ToolState has Source field | state.go line 87 | Correct | Line 86, off by one, field matches spec exactly |
| Lazy migration in Load() | state_tool.go migrateSourceTracking() | Correct | Called at state.go:197 inside Load() |
| Migration idempotent | TestMigrateSourceTracking_Idempotent | Correct | Test verifies double-run stability |
| New installs populate Source | helpers.go recipeSourceFromProvider() | Indirect | Populates Plan.RecipeSource, not ToolState.Source; migration bridges the gap |
| Existing state.json loads OK | TestSourceField_AbsentInJSON | Correct | Full round-trip through file I/O |
| Source persisted on Save | TestSourceField_RoundTrip | Correct | Three-value round-trip test |
| Unit tests | "12 tests in 3 files" | Mostly correct | ~12 test cases across 2 files (state_test.go, helpers_test.go), not 3 |
| go test passes | exit 0 | Not independently verified | Trusting CI |
| go vet passes | exit 0 | Not independently verified | Trusting CI |

### State Contract Assessment

The `Source` field currently has no consumer in the codebase. Under normal circumstances this would be a **blocking** state contract violation (field with no reader = schema drift). However, Issues 7-9 are the declared consumers and depend on this issue. The field is also consumed by the migration itself (write + read cycle) and tested. This is acceptable as a foundation issue in a multi-issue plan, not an orphaned field.

### Downstream Impact for Issues 7-9

- **Issue 7** should explicitly set `ts.Source` in `UpdateTool` callbacks during install, rather than relying on migration. The current indirect path works but adds unnecessary coupling to the migration.
- **Issue 8** (update/outdated/verify) will read `ToolState.Source` to route to the correct provider. The migration ensures all existing entries have a value.
- **Issue 9** (display in info/list) just reads the field. No architectural concern.
