# Intent Scrutiny: Issue 2 (feat(state): add source tracking to ToolState)

## Finding 1: `loadWithoutLock` skips source migration (Divergent twins)

**Severity: Blocking**

`state.go` has two load paths:
- `loadWithLock()` (line 159) calls both `migrateToMultiVersion()` and `migrateSourceTracking()`
- `loadWithoutLock()` (line 244) calls `migrateToMultiVersion()` but NOT `migrateSourceTracking()`

`loadWithoutLock` is used by `UpdateTool`, `RemoveTool`, `AddRequiredBy`, and `RemoveRequiredBy` -- all read-modify-write operations. These paths will load state with empty `Source` fields and write it back without migrating. The migration only runs when `Load()` is called (e.g., by `list`, `info`, `outdated`).

**Impact on downstream issues:** Issue 7 (install integration) will set `ToolState.Source` inside an `UpdateTool` callback. The tool being installed will get its Source, but all other tools in the state file will be read and written back with empty Source because `loadWithoutLock` doesn't migrate. This doesn't cause data loss (the migration is idempotent and will run on next `Load()`), but it means Source tracking is intermittently present in the state file depending on which code path last touched it. The next developer working on Issue 8 (`update`, `outdated`, `verify`) will find some tools have Source and some don't, depending on access patterns, and will need to understand this two-path behavior.

**Fix:** Add `state.migrateSourceTracking()` to `loadWithoutLock()`, matching the pattern already established for `migrateToMultiVersion()`.

## Finding 2: New installs don't populate `ToolState.Source` at install time

**Severity: Blocking**

The AC says "New installs populate Source during plan generation in `cmd/tsuku/helpers.go` based on the provider that resolved the recipe." The mapping claims this is implemented via `recipeSourceFromProvider()`.

What actually happens: `recipeSourceFromProvider()` maps the provider source to a string that flows into `Plan.RecipeSource`, NOT `ToolState.Source`. No `UpdateTool` callback in the install flow (`plan_install.go:106`, `install_deps.go:174`, `install_deps.go:412`, `install_deps.go:469`) sets `ts.Source`. New installs leave `ToolState.Source` empty; it only gets filled when `Load()` runs later and the migration infers from `Plan.RecipeSource`.

This is the intended design (lazy migration), but the AC and mapping are misleading. The mapping says `recipeSourceFromProvider()` is evidence for "New installs populate Source" -- it isn't. The function populates `Plan.RecipeSource`, and the migration infers from that.

**Impact on downstream issues:** Issue 7 says it "needs Source to be set during install." If the implementer reads the mapping and trusts that Issue 2 already handles this, they'll skip adding `ts.Source = ...` in the install flow. But Issue 2 only provides the migration fallback, not the direct-set path. Issue 7 must add `ts.Source = recipeSourceFromProvider(source)` inside the `UpdateTool` callback. The current Issue 2 implementation supports this (the field exists, the migration fills it if Issue 7 doesn't), but the mapping overstates what Issue 2 delivers.

**Fix:** The mapping should say "deferred to lazy migration" not "implemented via recipeSourceFromProvider()." Issue 7 should explicitly set `ToolState.Source` during install.

## Finding 3: Migration handles "embedded" Plan.RecipeSource but nothing produces it

**Severity: Advisory**

`migrateSourceTracking()` (state_tool.go:144) has a `case "embedded"` that maps `Plan.RecipeSource == "embedded"` to `ToolState.Source = "embedded"`. But:

1. The old install code (`helpers.go:generateInstallPlan`, `eval.go:205`) only ever set `RecipeSource` to `"registry"`, `"local"`, or a file path. Never `"embedded"`.
2. The new `recipeSourceFromProvider(SourceEmbedded)` returns `"central"`, so future Plan.RecipeSource will also never be `"embedded"`.

The `case "embedded"` branch is dead code. It doesn't cause a bug, but the next developer maintaining the migration will wonder when this case fires. A comment explaining "this handles a theoretical value for completeness" would prevent investigation.

## Finding 4: `recipeSourceFromProvider` maps embedded to "central" but ToolState.Source comment says "embedded" is valid

**Severity: Advisory**

`state.go:84` documents `ToolState.Source` values as: `"central"`, `"embedded"`, `"local"`, or `"owner/repo"`. But `recipeSourceFromProvider` maps `SourceEmbedded` to `"central"`, meaning `"embedded"` will only appear if the migration's dead branch fires (which it won't per Finding 3).

If a future developer adds code that checks `ToolState.Source == "embedded"` (trusting the comment), it will never match. The comment should either remove `"embedded"` from the valid values list, or the migration/mapping should be updated to produce it.

## Requirements Mapping Corrections

| AC | Mapping Status | Actual Status | Notes |
|----|---------------|---------------|-------|
| ToolState has Source field | implemented | **correct** | state.go:86 |
| Lazy migration in Load() | implemented | **correct** | state_tool.go migrateSourceTracking() |
| Migration idempotent | implemented | **correct** | TestMigrateSourceTracking_Idempotent |
| New installs populate Source | implemented | **overstated** | recipeSourceFromProvider populates Plan.RecipeSource, not ToolState.Source directly. Lazy migration fills in Source from Plan. Issue 7 must add direct-set. |
| Existing state.json loads OK | implemented | **correct** | TestSourceField_AbsentInJSON |
| Source persisted on Save | implemented | **correct** | TestSourceField_RoundTrip |
| Unit tests | implemented | **correct** | Coverage is solid |
| go test passes | implemented | **correct** | |
| go vet passes | implemented | **correct** | |

## Additional: Missing migration in `loadWithoutLock`

Not in the original mapping. `loadWithoutLock()` (used by all `UpdateTool`/`RemoveTool` operations) does not call `migrateSourceTracking()`, creating a divergent load path. See Finding 1.
