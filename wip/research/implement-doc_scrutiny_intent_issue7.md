# Intent Alignment Scrutiny: Issue 7

## Overview

Issue 7 ("feat(install): integrate distributed sources into install flow") integrates distributed recipe sources into `tsuku install`. This review evaluates whether Issue 7's implementation provides the correct API surface, state recording format, and dynamic provider registration for Issues 8-13 to build on.

**Files reviewed:**
- `cmd/tsuku/install.go` (install command entry point)
- `cmd/tsuku/install_distributed.go` (distributed install logic)
- `cmd/tsuku/install_distributed_test.go` (tests)
- `internal/install/state.go` (ToolState struct, migration)
- `internal/install/state_tool.go` (UpdateTool, GetToolState)
- `internal/recipe/loader.go` (AddProvider, GetFromSource, GetWithContext)
- `cmd/tsuku/helpers.go` (recipeSourceFromProvider, generateInstallPlan)
- `cmd/tsuku/update.go`, `outdated.go`, `verify.go`, `list.go`, `update_registry.go`
- `internal/distributed/provider.go` (DistributedProvider)
- `internal/install/list.go` (InstalledTool struct)

---

## Issue 8: Source-directed loading for update, outdated, verify

**Status: ALIGNED** (no blocking issues)

### ToolState.Source availability

Issue 8 needs to read `ToolState.Source` to route recipe fetches through `GetFromSource`. The implementation records this correctly:

- `ToolState.Source` is a `string` field with `json:"source,omitempty"` tag (state.go:89)
- `recordDistributedSource()` writes `"owner/repo"` as the source value (install_distributed.go:224)
- `migrateSourceTracking()` handles legacy entries by defaulting to `"central"` (state_tool.go:133-155)
- `recipeSourceFromProvider()` normalizes provider sources: `SourceRegistry` and `SourceEmbedded` both map to `"central"`, distributed sources pass through as `"owner/repo"` (helpers.go:159-169)

### GetFromSource compatibility

`Loader.GetFromSource(ctx, name, source)` (loader.go:185-248) handles all three source categories:
- `"central"` -> tries registry then embedded providers
- `"local"` -> tries local provider
- `"owner/repo"` (contains `/`) -> matches against distributed providers by `p.Source()`

This is the exact API Issue 8 needs. The fallback path for empty/missing source (default to `"central"`) is handled by the migration, so Issue 8 won't need special-case code.

### ADVISORY: update.go doesn't read ToolState.Source yet

The current `update.go` (line 24-50) loads installed tools via `mgr.List()` and uses the bare tool name with the normal loader chain. Issue 8 will need to:
1. Look up `ToolState.Source` for the tool being updated
2. Call `loader.GetFromSource(ctx, toolName, source)` instead of the normal `loader.Get()`
3. Handle the case where the distributed provider isn't registered in the current session (need to call `addDistributedProvider()` or equivalent)

This is expected work for Issue 8, not a gap in Issue 7.

### ADVISORY: Dynamic provider may not persist across sessions for update/outdated

When a user runs `tsuku update sometool` for a distributed tool, the `DistributedProvider` for that source won't be in the loader's provider chain unless:
(a) It was added during initial `main()` setup from `config.toml` registries, OR
(b) Issue 8 dynamically adds it before calling `GetFromSource`

Issue 7's `addDistributedProvider()` is in `cmd/tsuku/install_distributed.go` and is only called from the install flow. Issue 8 will need either:
- A shared helper that both install and update/outdated/verify can use, OR
- Loader initialization that pre-registers providers for all configured registries at startup

Neither approach requires changes to Issue 7's code -- the `AddProvider()` method on Loader and the `addDistributedProvider()` helper function are both available. But if providers aren't pre-registered at startup from `config.toml`, every command that uses distributed sources will need its own `ensureDistributedSource()` call.

**Recommendation:** Consider extracting provider pre-registration from `config.toml` registries into CLI initialization (e.g., `main.go` or a shared `initDistributedProviders()` function). This would benefit Issues 8, 9, and 10. However, this is optimization, not a blocker.

---

## Issue 9: Source display in list, info, recipes

**Status: ALIGNED** (no blocking issues)

### ToolState.Source is readable

Issue 9 needs `ToolState.Source` for display. It's available via `GetToolState()` (state_tool.go:86-98) which returns the full `ToolState` including `Source`.

### ADVISORY: InstalledTool struct lacks Source field

The `InstalledTool` struct (list.go:10-15) currently has `Name`, `Version`, `Path`, and `IsActive`. Issue 9 will need to add a `Source string` field to `InstalledTool` and populate it in `ListWithOptions()`. This is expected Issue 9 work.

### RecipeInfo.Source for recipes command

`RecipeInfo` (loader.go:601-605) already has a `Source RecipeSource` field. The `ListAllWithSource()` method (loader.go:516-534) populates it from each provider. Distributed providers return `RecipeSource("owner/repo")` from `Source()` (provider.go:70-72). This is ready for Issue 9 to consume.

---

## Issue 10: update-registry for distributed sources

**Status: ALIGNED** (no blocking issues)

### RefreshableProvider interface

`DistributedProvider` implements `Refresh(ctx)` (provider.go:76-81) which calls `ForceListRecipes` to bypass cache freshness. The `update-registry` command can iterate providers and type-assert to `RefreshableProvider`:

```go
for _, p := range loader.providers {
    if rp, ok := p.(recipe.RefreshableProvider); ok {
        rp.Refresh(ctx)
    }
}
```

### ADVISORY: Current update-registry only handles central registry

The current `update_registry.go` (line 33-43) directly accesses `ProviderBySource(recipe.SourceRegistry)` and type-asserts to `*CentralRegistryProvider`. Issue 10 will add a loop over all providers that implement `RefreshableProvider`. The infrastructure is there -- `ProviderBySource` returns a single provider, but the `providers` slice on Loader isn't exported. Issue 10 has two options:
1. Add a `Loader.RefreshAll(ctx)` method that iterates internally
2. Add a `Loader.Providers()` accessor

Either works. No Issue 7 changes needed.

### ADVISORY: Dynamic registration timing matters

Same concern as Issue 8 -- if distributed providers aren't registered at startup, `update-registry` won't find them to refresh. The registries are in `config.toml`, so Issue 10 should either pre-register them or iterate `config.Registries` directly.

---

## Issue 12: Recipe migration from central to distributed

**Status: ALIGNED** (no blocking issues)

Issue 12 removes koto recipes from `recipes/` after distributed install is working. The only dependency on Issue 7 is that `tsuku install tsukumogami/koto` must work. The install flow (install.go:188-246) handles this correctly:
1. `parseDistributedName` parses the qualified name
2. `ensureDistributedSource` handles registration
3. Recipe fetched via qualified name routing
4. Source recorded as `"tsukumogami/koto"` in state

No gaps.

---

## Issue 13: End-to-end validation

**Status: ALIGNED** (no blocking issues)

Issue 13 is a validation issue, not an implementation issue. It tests the full flow that Issues 7-12 build. The install path records everything needed:
- `Source` in ToolState for `list`/`info`/`update`/`outdated`/`verify`
- Auto-registration in `config.toml` for subsequent runs
- `-y` flag support for non-interactive testing

---

## Cross-cutting Concerns

### Source collision detection for update scenarios

**Status: ALIGNED**

`checkSourceCollision()` (install_distributed.go:178-213) compares `ToolState.Source` against the new source. This correctly handles:
- Same-source reinstalls (no collision)
- Different-source replacements (prompt or `--force`)
- Not-installed tools (no collision)
- Empty source defaulting to `"central"` (line 191-193)

For `update` (Issue 8), collision detection isn't needed because you're updating from the *same* source. The `ToolState.Source` tells you which source to use, and you fetch from that source only. No cross-source collision possible.

### RecipeHash audit trail

**Status: ALIGNED**

`RecipeHash` is recorded via `recordDistributedSource()` after successful install (install.go:239-245). The hash is computed from raw TOML bytes via `computeRecipeHash()` (SHA256). This field is available for Issue 8's `verify` command to detect recipe tampering, and for audit purposes.

One minor note: `fetchRecipeBytes()` (install_distributed.go:239-241) calls `loader.GetFromSource()` which returns raw bytes. This is the same bytes the provider fetched, so the hash is consistent with what was used for installation.

### ADVISORY: Race between install and source recording

The distributed install flow (install.go:219-245) has a two-phase approach:
1. `runInstallWithTelemetry()` installs the tool (sets up ToolState via the normal flow)
2. `recordDistributedSource()` updates ToolState with source and hash *after* install

If the install succeeds but `recordDistributedSource()` fails, the tool is installed but has no distributed source recorded. The code handles this gracefully by printing a warning (install.go:244) rather than failing. On next `update`, the tool would be treated as `"central"` (via migration default).

This is acceptable behavior -- the warning tells the user, and a reinstall would fix it. Not blocking.

### Telemetry opaque tag

**Status: ALIGNED**

`distributedTelemetryTag()` returns `"distributed"` (install_distributed.go:246), never leaking `owner/repo` to telemetry. All downstream issues should maintain this pattern.

---

## Summary

| Downstream Issue | Status | Blocking Items | Advisory Items |
|-----------------|--------|----------------|----------------|
| Issue 8 (update/outdated/verify) | ALIGNED | None | Provider pre-registration from config.toml needed |
| Issue 9 (list/info/recipes display) | ALIGNED | None | InstalledTool needs Source field (expected) |
| Issue 10 (update-registry) | ALIGNED | None | Provider iteration API or Loader.Providers() accessor needed |
| Issue 12 (recipe migration) | ALIGNED | None | None |
| Issue 13 (E2E validation) | ALIGNED | None | None |

**No blocking issues found.** The API surface, state format, and provider registration model from Issue 7 are compatible with all downstream consumers.

The main advisory theme is **provider pre-registration at startup**: Issues 8, 9, and 10 all need distributed providers in the loader chain, but Issue 7 only registers them dynamically during `tsuku install`. A shared initialization step that reads `config.toml` registries and pre-registers providers would reduce duplicated boilerplate across downstream issues. This could be done as part of any of those issues or as a small preparatory change.
