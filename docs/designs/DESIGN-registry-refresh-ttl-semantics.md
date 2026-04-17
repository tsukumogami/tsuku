---
status: Proposed
problem: |
  `tsuku update-registry` applies a 24-hour TTL to skip recently-cached recipes, so a
  recipe fix merged upstream does not reach users who run the command explicitly if they
  cached that recipe within the past day. The TTL was introduced for implicit cache
  access (during `tsuku install`, `tsuku info`) and background automation, where it
  prevents unnecessary network traffic. Applied to an explicit user invocation, it
  violates the command's contract: users asking to refresh get a silent no-op instead.
decision: |
  Make plain `tsuku update-registry` (no flags) always force-fetch every cached recipe
  from the registry, bypassing the TTL entirely. Remove the `--all` flag and the
  `forceRegistryRefreshAll` private helper, which become redundant. Inline the
  force-refresh loop directly in `runRegistryRefreshAll`. `CachedRegistry.RefreshAll`
  (TTL-aware) and `CachedRegistry.GetRecipe` (TTL-aware) are untouched, preserving
  correct TTL semantics for implicit cache access and any future automated callers.
rationale: |
  The command name is the contract. An explicit `update-registry` invocation is an
  intent to fetch, not a cache freshness query. The `--all` flag already proved the
  force path was needed; this change makes it the default. Removing `--all` and its
  private helper rather than keeping them alongside the new default avoids dead code
  and a confusing flag that does what the default already does. The TTL remains intact
  for `GetRecipe`, which protects implicit cache hits during install and info operations.
---

# DESIGN: Registry Refresh TTL Semantics

## Status

Proposed

## Context and Problem Statement

`tsuku update-registry` is documented as "Refresh the recipe cache." When a recipe fix is merged upstream, users who run this command expect to receive the updated recipe. Instead, `RefreshAll()` in `internal/registry/cached_registry.go` applies the same time-based TTL check used for implicit cache hits: if a recipe was cached within the past 24 hours, it is skipped and reported "already fresh." No network request is made and no content is compared.

This means the users most likely to be affected — those who recently cached a broken recipe — are exactly the ones who get no update. The `--all` flag bypasses this correctly via `forceRegistryRefreshAll()`, but it is never surfaced to users who see "already fresh."

The TTL was introduced alongside the auto-update feature to prevent background refresh invocations from hammering the registry. That constraint is valid and must be preserved: implicit fetches (during `tsuku install`, `tsuku info`, etc.) and automated background checks should continue to respect the TTL.

The fix must differentiate between explicit user invocations and implicit/automated ones.

**Invocation site inventory:**

| Call site | Type | Current TTL behavior |
|---|---|---|
| `cmd/tsuku/update_registry.go:168` — `cachedReg.RefreshAll()` | explicit user command | skips fresh recipes (bug) |
| `internal/registry/cached_registry.go:GetRecipe()` | implicit during install/info/etc. | respects TTL (correct) |
| `cmd/tsuku/cmd_check_updates.go` — `check-updates` background | automated | does not call registry refresh at all |

## Decision Drivers

- Explicit user invocations of `update-registry` must fetch current data from the registry, even within the TTL window
- Implicit cache access (during `install`, `info`, `search`) must continue to respect the TTL to avoid unnecessary network traffic
- The background auto-update process (`check-updates`) does not invoke registry refresh; no change needed there
- Minimal API surface: prefer adjusting the command layer over restructuring `CachedRegistry` internals when the trade-offs are equivalent
- The fix must be testable: a test can verify that explicit refresh bypasses TTL, while implicit access respects it

## Considered Options

### Decision 1: Default behavior of `tsuku update-registry`

`update-registry` is the one command whose documented purpose is refreshing the recipe cache. Users invoke it when they believe their cache is stale — typically after seeing a broken tool or hearing about an upstream fix. The TTL's function is to limit unsolicited network traffic, not to throttle requests from users who explicitly asked for a refresh.

The existing `--all` flag already proves the force path works. It was added as an escape hatch precisely because the TTL-based default was insufficient for users who needed fresh recipes. The question is whether the escape hatch should be the default or a hidden flag.

#### Chosen: Force-refresh by default (Option A)

Plain `tsuku update-registry` always fetches all cached recipes from the network, regardless of TTL. This matches the current `--all` behavior. TTL remains in `GetRecipe()` to protect implicit cache access during install, info, and search operations.

The command name is the contract. Users who run `tsuku update-registry` are not asking whether their cache is still within a freshness window — they are asking for fresh data. "Already fresh" after skipping the network violates that expectation without informing the user what happened. Additionally, the network cost of a full registry refresh is low (a few hundred HTTPS requests to GitHub's raw content CDN for ~150 recipes at ~3KB each) and is a cost the user has already implicitly accepted by running a command named "update."

#### Alternatives Considered

**Option B — Keep TTL, surface `--all` hint**: Append "to force refresh, run: tsuku update-registry --all" to the summary line. Rejected because it doesn't fix the bug. Users who see "already fresh" have no reason to believe their cache contains a broken recipe — the hint must be noticed and acted on at exactly the moment when the user is already confused. It also adds ongoing display noise for every normal run when the cache is genuinely fresh. It inverts the semantic by implying `--all` is unusual when it should be the default.

**Option C — Remove TTL from `RefreshAll` entirely**: Make `RefreshAll` always fetch, affecting any caller. Rejected because it violates the constraint that TTL must be preserved for implicit/automated callers. `CachedRegistry.RefreshAll` is a shared type method; removing its TTL guard would affect any future automated caller that expects TTL semantics.

### Decision 2: Where the TTL-bypass logic lives

Given that plain `update-registry` will force-refresh, the implementation must express this somewhere. The command layer already has `forceRegistryRefreshAll()` (a private function that calls `Refresh()` per recipe) alongside `cachedReg.RefreshAll()` (TTL-aware). The choice is whether to reroute through the existing private function, add API to `CachedRegistry`, or clean up both.

#### Chosen: Inline the force loop; delete `forceRegistryRefreshAll` and `--all`

Once the default is force-refresh, `forceRegistryRefreshAll` and `--all` are redundant. The right outcome is to inline the force-refresh loop directly into `runRegistryRefreshAll` (it is short — a loop over cached recipe names calling `Refresh` per entry with error accumulation) and remove the flag and the private helper.

This makes `runRegistryRefreshAll` unconditionally force-fetch, keeps `CachedRegistry.RefreshAll` (TTL-aware) available for any future automated caller that needs it, and shrinks the command layer by one private function and one flag. The explicit/implicit distinction is clear at each call site: the command calls `Refresh` per recipe; any automated caller uses `RefreshAll`.

#### Alternatives Considered

**Option A — Command layer only, keep `forceRegistryRefreshAll`**: Reroute `runRegistryRefreshAll` to always call `forceRegistryRefreshAll`, leave `--all` in place. Rejected because once the default is force-refresh, the `--all` flag does what the default already does. Keeping it adds noise to help text and tests without providing value. Option D is strictly cleaner.

**Option B — Add `force bool` to `RefreshAll`**: Change signature to `RefreshAll(ctx, force bool)`. Rejected because boolean flag parameters on methods are harder to read at the call site (`RefreshAll(ctx, true)` vs `RefreshAll(ctx, false)`) and mutate the `CachedRegistry` API without a genuine need — the command layer already owns this distinction.

**Option C — New `RefreshAllForce()` method on `CachedRegistry`**: Add a dedicated force-refresh method. Rejected because it duplicates logic already present in `forceRegistryRefreshAll` without a second caller to justify the abstraction.

## Decision Outcome

**Chosen: 1A + 2D**

### Summary

`runRegistryRefreshAll` in `cmd/tsuku/update_registry.go` is rewritten to unconditionally call `cachedReg.Refresh(ctx, name)` for each cached recipe, assembling `RefreshStats` inline. The `forceRegistryRefreshAll` private function, the `registryRefreshAll` variable, and the `--all` flag are all removed. The `Long` help text on `updateRegistryCmd` is updated to remove the `--all` mention.

`CachedRegistry.RefreshAll` and `CachedRegistry.GetRecipe` are untouched. `RefreshAll` retains its TTL-aware behavior and remains available for any future automated caller. `GetRecipe`'s TTL check continues to protect implicit cache access during `tsuku install`, `tsuku info`, and `tsuku search`.

The result is that `tsuku update-registry` always fetches current recipes. Users who ran `--all` previously get the same behavior from the plain command. The "already fresh" status code disappears from explicit refresh output — every cached recipe is fetched on each explicit invocation.

### Rationale

The two decisions reinforce each other. Deciding that the default should be force-refresh makes the `--all` flag and its private helper redundant; deciding to remove them rather than keep them simplifies the code while preserving the `CachedRegistry` API for future callers. Neither decision requires a new API or new behavior — only a routing change and a cleanup. The constraint to preserve TTL for implicit access is satisfied by leaving `GetRecipe` unchanged.

## Solution Architecture

### Overview

The change is entirely within the command layer (`cmd/tsuku/update_registry.go`). The `CachedRegistry` package is read-only for this fix. The explicit/implicit TTL distinction is encoded at the call site: the command calls `Refresh` per recipe (force); implicit callers go through `GetRecipe` (TTL-aware).

### Components

```
cmd/tsuku/update_registry.go
  └── runRegistryRefreshAll(ctx, cachedReg)
        Calls: cachedReg.Refresh(ctx, name) for each cached recipe
        Assembles: RefreshStats inline (no forceRegistryRefreshAll helper)

internal/registry/cached_registry.go  [unchanged]
  ├── GetRecipe(ctx, name)        -- TTL-aware, used by install/info/search
  ├── Refresh(ctx, name)          -- always fetches, used by command layer
  └── RefreshAll(ctx)             -- TTL-aware bulk refresh, retained for future automated callers
```

`CachedRegistry.RefreshAll` is kept intentionally — it is not dead code. It provides the TTL-aware bulk path for any future automated caller that should not bypass the cache. The command layer simply no longer uses it for explicit user invocations.

### Key Interfaces

`CachedRegistry.Refresh(ctx context.Context, name string) (*RefreshDetail, error)` — already exists, always fetches from network regardless of cache age. This is the force path.

`RefreshStats` — assembled inline by `runRegistryRefreshAll`. Fields: `Total`, `Refreshed`, `Errors`, `Details []RefreshDetail`.

### Data Flow

```
tsuku update-registry
  └── runRegistryRefreshAll(ctx, cachedReg)
        └── cachedReg.Registry().ListCached()       -- get names of all cached recipes
              For each name:
                cachedReg.Refresh(ctx, name)         -- fetch from network, update cache
                  └── cachedReg.fetchAndCache(ctx, name)
                        └── registry.FetchRecipe(ctx, name)  -- HTTPS to registry
                              registry.CacheRecipe(name, data) -- write to disk
```

## Implementation Approach

### Phase 1: Rewrite `runRegistryRefreshAll` and remove dead code

Replace the `if registryRefreshAll { ... } else { ... }` branch in `runRegistryRefreshAll` with an unconditional force loop (the body of the former `forceRegistryRefreshAll`). Remove `forceRegistryRefreshAll`, the `registryRefreshAll` variable, the `--all` flag registration, and its mention in `updateRegistryCmd.Long`. Help text and flag removal belong in the same commit so the command is never in an inconsistent state.

Deliverables:
- `cmd/tsuku/update_registry.go` — rewritten `runRegistryRefreshAll`, removed helper, variable, flag, and help text

### Phase 2: Tests

Add tests for `runRegistryRefreshAll` covering: empty cache (no-op), all-success, partial-error, and cache-clear side effects. No existing tests cover this function, so new test cases are needed rather than updates to existing ones.

Deliverables:
- `cmd/tsuku/update_registry_test.go` — new tests for the rewritten refresh path

## Security Considerations

**Supply chain (positive impact).** Making explicit `update-registry` invocations force-fetch all cached recipes closes the propagation gap identified during review. Before this change, a recipe security fix (corrected download URL, fixed checksum) would not reach users who cached that recipe within the TTL window, even if they explicitly ran `update-registry`. After this change, any explicit invocation delivers the current registry state.

**Transport security (unchanged).** Recipe fetches use TLS with certificate verification, a 1 MiB response body cap, and `DisableCompression` to block decompression attacks. These controls apply equally to the new force-fetch path.

**Rate limiting.** Force-fetching all cached recipes sends one request per cached TOML. At the current registry size (~1,400 recipes), a full cache refresh generates ~1,400 sequential requests to `raw.githubusercontent.com`. Users should use `--recipe <name>` for targeted refreshes and `--dry-run` to preview what would be fetched. Scripts and CI pipelines can set `TSUKU_REGISTRY_URL` to a trusted local directory path to avoid CDN requests entirely; this variable must point to a controlled location and should not be set from untrusted input. HTTP 429 responses are surfaced as per-recipe errors; the refresh loop does not abort on rate limiting, which may produce repeated errors if the CDN rate-limits the session. A follow-on could add early-exit after consecutive 429 responses.

**TOCTOU / concurrent access (pre-existing, low severity).** The cache write path uses `os.WriteFile`, which is not atomic with respect to the metadata sidecar. A concurrent `tsuku install` reading the cache during a refresh may observe a stale TOML until the write completes. This is a pre-existing condition not worsened by the change; worst case is the behavior that existed before the fix.

**Disk integrity (pre-existing gap).** `CacheMetadata.ContentHash` (SHA-256 of the cached TOML) is computed on write but not verified on read. Silent disk corruption within `$TSUKU_HOME/registry/` is not detected. A future improvement could verify the hash on every cache read. This is out of scope for this change.

## Consequences

### Positive

- `tsuku update-registry` reliably delivers updated recipes after upstream fixes, including for users who ran it within the previous 24 hours
- Recipe security fixes (corrected URLs, updated checksums) propagate immediately to users who explicitly request a refresh
- The `--all` flag, `registryRefreshAll` variable, and `forceRegistryRefreshAll` function are removed — the command layer is simpler
- `CachedRegistry.RefreshAll` is unchanged and remains available for future automated callers that need TTL semantics
- TTL still protects implicit cache access: `GetRecipe` in `CachedRegistry` is untouched

### Negative

- Plain `tsuku update-registry` always makes network requests, even if the cache was refreshed minutes ago. Users treating the command as a no-op health check will always get a real refresh.
- Any future automated internal caller that uses `runRegistryRefreshAll` directly will incur full network cost. Such a caller should use `cachedReg.RefreshAll()` instead.

### Mitigations

- The network cost of a full registry refresh is low: a few hundred HTTPS requests to GitHub's CDN for ~150 recipes. On any reasonable connection this completes in seconds.
- The `CachedRegistry.RefreshAll` method (TTL-aware) is preserved and available for callers that need it. The command layer simply no longer uses it for explicit invocations.
