---
title: "Scrutiny: Justification - Issue 2 (source tracking)"
phase: scrutiny
role: pragmatic-reviewer
---

# Justification Scrutiny - Issue 2

## Mapping Verification

### AC: "ToolState has Source field" - CONFIRMED
`internal/install/state.go:86` -- `Source string` with `json:"source,omitempty"`. Matches AC exactly.

### AC: "Lazy migration in Load()" - CONFIRMED
`internal/install/state.go:197` calls `migrateSourceTracking()` after `migrateToMultiVersion()`. `state_tool.go:133-157` implements the migration.

### AC: "Migration idempotent" - CONFIRMED
`TestMigrateSourceTracking_Idempotent` runs migration twice, compares results. Also `TestMigrateSourceTracking_SkipsExisting` verifies entries with Source are untouched.

### AC: "New installs populate Source" - PARTIALLY IMPLEMENTED
**Finding 1 (Advisory).** `recipeSourceFromProvider()` in `helpers.go:159` maps provider source to a string, but that string goes into `Plan.RecipeSource` (line 240), not `ToolState.Source`. `InstallWithOptions` in `manager.go:169-195` never sets `ts.Source`. New installs get Source populated only on the next `Load()` via the lazy migration. This is functionally correct (migration covers the gap) but the mapping claim "implemented via helpers.go recipeSourceFromProvider()" overstates what the code does -- it populates Plan.RecipeSource, not ToolState.Source directly.

### AC: "Existing state.json loads OK" - CONFIRMED
`TestSourceField_AbsentInJSON` loads JSON without Source, verifies no error and migration to "central".

### AC: "Source persisted on Save" - CONFIRMED
`TestSourceField_RoundTrip` does Save/Load cycle, verifies Source survives.

### AC: "Unit tests" - INFLATED COUNT
Mapping claims "12 tests in 3 files". Actual new test functions: 7 in `state_test.go` + 1 in `helpers_test.go` = 8 test functions across 2 files. The subtests within `TestMigrateSourceTracking_InfersFromPlan` (4 cases) and `TestRecipeSourceFromProvider` (4 cases) bring the total table-driven sub-test count higher, but the claim is misleading.

### AC: "go test passes" / "go vet passes" - NOT VERIFIED BY REVIEWER
These are runtime claims. Assumed correct since the code compiles and tests are structurally sound.

## Findings

### Finding 1: Embedded source inconsistency between migration and new installs (Advisory)

Migration (`state_tool.go:145`) maps Plan.RecipeSource "embedded" -> Source "embedded".
New installs (`helpers.go:163`) map SourceEmbedded -> Plan.RecipeSource "central".

Result: pre-existing tools installed from embedded recipes get Source="embedded", but new tools installed from embedded recipes get Source="central". Same origin, different tracking value depending on install timing.

The AC at line 69 lists "embedded" as a valid Source value for new installs, but `recipeSourceFromProvider` never produces it. The ToolState comment (line 84) also lists "embedded" as a value.

This is advisory because the migration path will shrink over time and embedded is functionally equivalent to central for downstream consumers. But it's a data consistency issue that could confuse future code that switches on Source values.

**Fix:** Either make migration also map "embedded" -> "central" (simpler, consistent), or make `recipeSourceFromProvider` preserve "embedded" (matches AC literally). The former is better since the comment on `recipeSourceFromProvider` explains the rationale for collapsing embedded into central.

### Finding 2: ToolState.Source not set inline during install (Advisory)

As noted above, `InstallWithOptions` never sets `ts.Source`. It relies entirely on the lazy migration in `Load()` to backfill from Plan.RecipeSource. Within the same process that installs a tool, `ToolState.Source` is empty until the state is reloaded.

This is unlikely to cause bugs today (no code reads Source in the install path), but it's a latent issue if future code (e.g., post-install telemetry) reads Source before the next Load().

**Fix:** Add `ts.Source = recipeSourceFromProvider(...)` in the install path, or accept the lazy-migration-only approach and update the AC wording.

### Finding 3: Test count claim is inflated (Advisory)

"12 tests in 3 files" should be "8 test functions across 2 files". Not blocking, but the mapping should be accurate.
