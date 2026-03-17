---
focus: architect
issue: 7
blocking_count: 1
advisory_count: 2
---

# Architect Review: Issue 7 -- Integrate Distributed Sources into Install Flow

## Summary

The implementation follows the existing architecture well. Distributed providers register through the standard `RecipeProvider` interface and are appended to the loader's provider chain via `AddProvider`. The qualified-name routing in the loader (`owner/repo:recipe` -> `getFromDistributed`) keeps the existing resolution chain intact for bare names. State fields (`Source`, `RecipeHash`) have documented consumers (collision detection now, verify in Issue 8).

## Findings

### 1. State written outside the install transaction -- race with `runInstallWithTelemetry`

**File:** `cmd/tsuku/install.go:234-245`, `cmd/tsuku/install_distributed.go:217-228`

`recordDistributedSource` runs *after* `runInstallWithTelemetry` returns. Inside `runInstallWithTelemetry`, the normal install flow already calls `UpdateTool` to set `Source` via `recipeSourceFromProvider`. The distributed path then calls `recordDistributedSource` to overwrite `Source` and add `RecipeHash`. Two problems:

1. **Double write.** The normal flow sets `Source` from the provider tag (which for distributed providers returns `"owner/repo"` -- correct). Then `recordDistributedSource` overwrites it with the same value. This is redundant but not harmful on its own.

2. **RecipeHash set in a separate UpdateTool call.** If the process crashes between the install's `UpdateTool` and `recordDistributedSource`, the state has `Source` but no `RecipeHash`. More importantly, any future path that records state during install (e.g., a post-install hook) could interleave between these two writes. The hash should be set in the same state mutation as the install.

The existing `--recipe` path (`runRecipeBasedInstall`) avoids this by caching the recipe under the bare name before calling `runInstallWithTelemetry`, letting the normal flow handle state. The distributed path should follow that pattern: cache the recipe, let the normal install set `Source` from the provider's `Source()` return, and pass the recipe hash into the install flow rather than applying it after the fact.

**Severity: Advisory.** The double-write is benign because both writes produce the same `Source` value, and `RecipeHash` is write-only until Issue 8. This doesn't compound -- no other code path will copy the "write state after install" pattern because it's specific to distributed installs. But it should be cleaned up when Issue 8 adds the verify consumer.

### 2. `CacheRecipe` under bare name creates implicit aliasing

**File:** `cmd/tsuku/install.go:230`, `internal/recipe/loader.go:436-438`

After loading via `loader.GetWithContext(qualifiedName, ...)`, the code calls `loader.CacheRecipe(dArgs.RecipeName, r)` to alias the recipe under its bare name. This is necessary because `installWithDependencies` resolves recipes by bare name.

The problem: if the central registry also has a recipe named `dArgs.RecipeName`, the in-memory cache now shadows it for the remainder of the process. The `--recipe` path has the same pattern (line 360), so this isn't a new architectural divergence -- but the distributed path makes it more likely to collide with real registry names (a local recipe file is unlikely to share a name with a registry recipe; a distributed recipe very well might).

**Severity: Advisory.** This matches the existing `--recipe` pattern, so it's not a parallel approach. The scope is limited to the current process (the cache isn't persisted). Worth noting for future work: if multi-tool distributed installs become common (`tsuku install org/repo:toolA org/repo:toolB`), the aliasing could cause subtle resolution bugs when toolA depends on something named toolB in the central registry.

### 3. `addDistributedProvider` bypasses loader initialization path

**File:** `cmd/tsuku/install_distributed.go:149-171`

`addDistributedProvider` constructs a `DistributedProvider` inline (creating `CacheManager`, `GitHubClient`) and appends it to the global `loader` via `AddProvider`. This is a runtime mutation of the provider chain that was initialized at startup.

This is structurally sound -- `AddProvider` is the documented way to extend the chain, and the distributed provider implements `RecipeProvider`. The dependency direction is correct: `cmd/tsuku` imports `internal/distributed` and `internal/recipe`, both lower-level packages.

However, `config.DefaultConfig()` is called every time this function runs and again inside `checkSourceCollision` and `recordDistributedSource` -- three separate config loads for a single distributed install. The config is immutable within a process, so this creates unnecessary I/O. More importantly, it means the config loading pattern could diverge if `DefaultConfig()` ever gains caching or mutable state.

**Severity: Blocking.** The repeated `config.DefaultConfig()` calls aren't a structural violation on their own, but the real issue is that `addDistributedProvider` constructs infrastructure objects (`CacheManager`, `GitHubClient`) at the call site rather than using any shared construction path. If the `GitHubClient` gains configuration (auth tokens, proxy settings, rate limit config), every construction site needs updating. The existing providers are all constructed in a single initialization path (`cmd/tsuku/main.go` or equivalent). `addDistributedProvider` should receive the config and any shared clients as parameters, constructed once during the distributed install setup, not re-derived from `DefaultConfig()` per provider.

## Architecture Fit

The overall approach fits well:

- **Provider interface:** `DistributedProvider` implements `RecipeProvider` correctly. `Source()` returns `"owner/repo"`, which integrates with `recipeSourceFromProvider`'s default case and `GetFromSource`'s distributed branch. No interface bypass.

- **State contract:** `Source` field already existed and has consumers (collision detection, `GetFromSource` routing). `RecipeHash` is new but has a planned consumer in Issue 8 (verify). The `omitempty` tag ensures backward compatibility.

- **CLI surface:** The `--yes` flag is additive and doesn't overlap with existing flags. The `owner/repo` syntax is parsed in the install command's argument loop, not in a separate subcommand.

- **Dependency direction:** `cmd/tsuku` -> `internal/distributed` -> `internal/recipe`. Clean downward flow. No circular imports.

- **Loader extension:** `AddProvider` and `CacheRecipe` are clean public methods on `Loader` that don't break the existing provider chain semantics. `getFromDistributed` in the loader routes qualified names without affecting bare-name resolution.
