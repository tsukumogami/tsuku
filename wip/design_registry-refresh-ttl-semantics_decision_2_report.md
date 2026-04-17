# Decision 2: Where should the TTL-bypass for explicit refresh live?

**Status:** decided
**Date:** 2026-04-16

---

## Context

`CachedRegistry` (in `internal/registry/cached_registry.go`) wraps `Registry` with TTL-based caching. It has two entry points:

- `RefreshAll(ctx)` — skips recipes whose cache is within TTL (24h default)
- `Refresh(ctx, name)` — always fetches a single named recipe, ignoring TTL

In `cmd/tsuku/update_registry.go`, the command layer has:

- `forceRegistryRefreshAll(ctx, cachedReg, cached)` — private function that calls `Refresh()` per recipe, bypassing TTL
- `runRegistryRefreshAll(ctx, cachedReg)` — routes to `RefreshAll` (TTL-aware) or `forceRegistryRefreshAll` based on the `--all` flag

If Decision 1 resolves to "force-refresh by default for explicit invocation," the implementation must express this somewhere. The question is whether the right place is the command layer (no new `CachedRegistry` API) or the `CachedRegistry` type itself (new method or modified signature).

This matters because the answer affects where the semantics of "explicit vs. implicit" are encoded: at the boundary closest to user intent (the command), or in the caching infrastructure that could be called from many contexts.

---

## Key Assumptions

1. Decision 1 resolves to "force-refresh by default" — that is, plain `tsuku update-registry` with no flags will bypass the TTL and always fetch fresh content. This decision is only meaningful under that assumption.
2. The only current caller of `RefreshAll` is `runRegistryRefreshAll` in the command layer. There are no callers in tests or other packages that depend on the TTL-aware behavior of `RefreshAll` for a force-refresh path.
3. The `--all` flag, if Decision 1 removes the distinction between "default" and "force," becomes redundant and can be eliminated.
4. No other package currently calls `forceRegistryRefreshAll` — it is private to the command layer.
5. Testability for the TTL-aware path (`RefreshAll`) is already covered by `TestCachedRegistry_RefreshAll` in `internal/registry/cached_registry_test.go`. The TTL-bypass path is covered implicitly through `Refresh` tests.

---

## Chosen Option: D — Delete `forceRegistryRefreshAll` and `--all`

**Rationale:** If the default is force-refresh, the `--all` flag and `forceRegistryRefreshAll` are redundant. Remove both, simplify `runRegistryRefreshAll` to always call `Refresh()` per recipe (the same logic `forceRegistryRefreshAll` uses today), and keep `RefreshAll` on `CachedRegistry` for any automated future callers that want TTL-aware behavior. The distinction between explicit and implicit callers is preserved — the command always forces, anything calling `RefreshAll` directly respects TTL — without adding API surface or boolean flag parameters.

**Detailed rationale:**

The core issue is that `--all` and `forceRegistryRefreshAll` exist solely to give users an escape hatch from the TTL when they need fresh recipes. Decision 1 collapses that escape hatch into the default. Once that happens:

- `--all` becomes a flag that does the same thing as the default. Keeping it adds noise to the help text and tests without providing value.
- `forceRegistryRefreshAll` becomes dead code — or rather, its logic moves directly into `runRegistryRefreshAll`.

The simplest, correct result is:

1. `runRegistryRefreshAll` calls `Refresh(ctx, name)` for each cached recipe, directly — the same logic `forceRegistryRefreshAll` uses today, but inlined (it's short enough).
2. `RefreshAll` stays on `CachedRegistry` as the TTL-aware bulk refresh, ready for any future automated context.
3. No new methods, no new parameters, no boolean flags.

This satisfies all constraints:

- **CachedRegistry coherence**: `RefreshAll` remains the TTL-aware path (correct for background/automated use); `Refresh` per-recipe remains the force path (correct for explicit use). The type's API is unchanged.
- **Minimal API surface**: no new method, no modified signature. The API actually shrinks (one less private command-layer function, one less flag).
- **Expressing the explicit/implicit distinction in code**: the command layer always calls `Refresh` per recipe. Any future automated caller uses `RefreshAll`. Intent is clear from the call site.
- **Testability**: `RefreshAll`'s TTL-aware behavior is tested in `cached_registry_test.go`. The force path through `Refresh` is already tested. Command-level tests can verify the flag is gone and the behavior is always-force.

The one risk is that a caller in the future might want TTL-aware bulk refresh from the command layer. If that need arises, `RefreshAll` is still there. The command simply doesn't use it for explicit user invocations.

---

## Rejected Alternatives

### A — Command layer only (change routing, no API change)

**Why rejected:** This option keeps `forceRegistryRefreshAll` and routes `runRegistryRefreshAll` to always call it, while leaving `--all` in place (or removing it separately). It doesn't go far enough. If the default is force-refresh, the `--all` flag is a flag that does what the default already does. Option A leaves the dead flag and dead function in place, which adds confusion. The "minimal API surface" constraint applies to the full system, not just `CachedRegistry` — command-layer dead code counts. Option D is strictly cleaner.

### B — Add `force bool` to `RefreshAll`

**Why rejected:** Boolean flags on methods are a code smell when there are only two callers and the distinction is already expressible through separate methods. `RefreshAll(ctx, true)` vs `RefreshAll(ctx, false)` at the call site is less readable than `Refresh(ctx, name)` in a loop vs `RefreshAll(ctx)`. It also mutates the `CachedRegistry` API without a genuine need — Option D leaves that API intact. The "minimal API surface preferred" constraint rules this out.

### C — New `RefreshAllForce()` method on `CachedRegistry`

**Why rejected:** Adds a method to `CachedRegistry` when the command layer already has equivalent logic (`forceRegistryRefreshAll` calls `Refresh` per recipe in a loop). A new `RefreshAllForce()` would duplicate that logic inside the registry package. There's no second caller of a force-bulk-refresh path that would justify the indirection. The constraint "minimal API surface preferred" and the observation that the command layer already owns this logic make Option C wasteful. Option D instead removes a layer of indirection rather than adding one.

---

## Consequences

**Immediate changes:**

- Delete `forceRegistryRefreshAll` from `cmd/tsuku/update_registry.go`.
- Delete the `--all` flag registration and the `registryRefreshAll` variable.
- In `runRegistryRefreshAll`, replace the conditional branch with a direct loop calling `cachedReg.Refresh(ctx, name)` for each cached recipe, assembling `RefreshStats` inline (the logic is short — a small loop with error accumulation).
- Update `Long` help text on `updateRegistryCmd` to remove the `--all` mention.
- Remove tests that cover `--all` behavior (if any exist beyond the distributed-sources tests already present in `update_registry_test.go`).

**`CachedRegistry` API:** unchanged. `RefreshAll` and `Refresh` remain as-is.

**Future automated callers:** can call `RefreshAll` (TTL-aware) or `Refresh` (force) per their needs. The TTL-aware path is not removed — it's just not used by the explicit command anymore.

**User-facing:** `tsuku update-registry` always force-refreshes. The `--all` flag disappears from the help text. Users who were using `--all` get the same behavior from the plain command.

**Test surface reduction:** one fewer code path to test in the command layer. The registry package tests are unaffected.
