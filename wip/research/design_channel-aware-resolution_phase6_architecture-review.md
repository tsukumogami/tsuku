# Architecture Review: DESIGN-channel-aware-resolution

## Review Scope

Full architecture review of the channel-aware version resolution design document.
Validated against current source code in the tsuku codebase.

## Question 1: Is the architecture clear enough to implement?

**Verdict: Yes, with one correction needed.**

The design is well-structured and implementable. The three-decision stack (pin level derivation, boundary enforcement, cache-backed resolution) is cleanly decomposed. Each phase has clear deliverables and the data flow diagram accurately describes the intended behavior.

**Correction needed:** The design document's interface listing (line 148) shows `ListVersions` returning `[]*VersionInfo`, but the actual `VersionLister` interface in `internal/version/provider.go:33` returns `[]string`. The `ResolveWithinBoundary` helper must account for this: after filtering the `[]string` list, it needs to construct a `*VersionInfo` from the matched version string. This means the helper either:

1. Calls `provider.ResolveVersion(ctx, matchedVersion)` to get the full `VersionInfo` (including Tag and Metadata), or
2. Constructs a minimal `VersionInfo{Version: matchedVersion}` and hopes callers don't need the Tag.

Option 1 is correct because downstream code (like the install flow) needs the `Tag` field for download URL construction. This is a non-trivial detail the design should call out.

## Question 2: Are there missing components or interfaces?

**Verdict: One gap in the `update.go` integration path.**

The design says `update.go` should read `VersionState.Requested` and pass it to `ResolveWithinBoundary`. However, the current `update.go` (line 82) calls `runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient)` -- passing empty strings for both `reqVersion` and `versionConstraint`. The version resolution happens deep inside the install flow, not in `update.go` directly.

This means the design's integration approach has two options:

- **Option A (design's approach):** Add pin-aware resolution in `update.go` before calling the install flow, then pass the resolved version. This requires understanding how `runInstallWithTelemetry` uses its `reqVersion` parameter.
- **Option B (deeper integration):** Modify the install flow itself to be pin-aware when it detects an update scenario (tool already installed).

The design should specify which path, because the current `update.go` doesn't do any version resolution itself -- it delegates everything to the install pipeline.

**No missing interfaces.** The existing `VersionResolver` and `VersionLister` interfaces are sufficient. The `ProviderFactory` already exists and works correctly. No new interfaces are needed.

## Question 3: Are the implementation phases correctly sequenced?

**Verdict: Yes, the sequencing is correct.**

- Phase 1 (pin level model) has zero dependencies on existing code. Pure functions with pure tests.
- Phase 2 (cache-backed resolution) depends on Phase 1 for `VersionMatchesPin` and depends on the existing `CachedVersionLister`. The cache modification (deriving `ResolveLatest` from the list) is a localized change.
- Phase 3 (command integration) depends on Phase 2. Wiring into `update.go` and `outdated.go` requires the helper to exist.

One suggestion: Phase 2 could be split into 2a (add `ResolveWithinBoundary` helper) and 2b (modify `CachedVersionLister.ResolveLatest`). The cache change is independent and carries its own risk (changing behavior of existing callers of `ResolveLatest`). Separating them makes each PR easier to review and roll back.

## Question 4: Does ResolveWithinBoundary correctly handle VersionLister vs VersionResolver?

**Verdict: Mostly correct, with a subtle issue.**

The design's branching logic is:
1. Provider implements `VersionLister`? -> Use cached list + filter
2. Provider is `VersionResolver` only? -> Call `ResolveVersion(ctx, requested)`
3. Empty `requested`? -> Call `ResolveLatest(ctx)`

**Issue with FossilTimelineProvider:** The design says FossilTimelineProvider is a "VersionResolver-only provider" (line 83, listed alongside CustomProvider). But checking the actual code, `FossilTimelineProvider` **does** implement `ListVersions` (fossil_provider.go:57). It implements the full `VersionLister` interface. The design's claim that it's VersionResolver-only is wrong.

After code review, the **only** VersionResolver-only provider is `CustomProvider`. The design mentions two such providers; in reality there's one. This doesn't break the design -- it actually makes it simpler. The VersionResolver-only fallback path has a smaller blast radius than anticipated.

**Second issue: prefix matching semantics.** The design assumes that filtering a version list by string prefix is equivalent to what `ResolveVersion` does. For most providers this is true. However, `GitHubProvider.ResolveVersion` (via `ResolveGitHubVersion`) does more than prefix matching -- it also checks with/without "v" prefix and normalizes versions before comparison. The list-filter path in `ResolveWithinBoundary` needs to replicate this normalization, or use `VersionMatchesPin` which should handle it. The design's `VersionMatchesPin` function should explicitly account for version normalization (v-prefix stripping).

## Question 5: Are there simpler alternatives we overlooked?

**Verdict: The design is already close to the simplest viable approach. Two minor simplifications exist.**

**Simplification 1: Skip the `PinLevel` enum entirely.** The `ResolveWithinBoundary` helper doesn't actually need the enum. It can work directly with the `requested` string:
- Empty string -> `ResolveLatest`
- Non-empty string -> filter list by prefix (for VersionLister) or call `ResolveVersion` (for VersionResolver-only)

The `PinLevel` enum is useful for display purposes (`tsuku outdated` showing "pinned to major 18") and for the future auto-update system to make policy decisions. If those use cases are confirmed, keep it. If the only consumer is `ResolveWithinBoundary`, the enum adds complexity without value.

**Simplification 2: Don't change `CachedVersionLister.ResolveLatest` yet.** Decision 3 (derive ResolveLatest from cached list) is an optimization. The `ResolveWithinBoundary` helper can call `ListVersions` directly through the cached wrapper (getting cache benefits) and pick the first entry. This avoids modifying `ResolveLatest`'s behavior for existing callers, reducing risk. The optimization can be added later if profiling shows it matters.

**Why not just use ResolveVersion everywhere?** One alternative the design briefly considers but correctly rejects: just calling `ResolveVersion(ctx, requested)` on all providers. This won't work because:
- Not all providers' `ResolveVersion` does prefix matching (some require exact match).
- `ResolveVersion` bypasses the cache, so you lose the performance benefit.
- For empty `requested`, you still need to call `ResolveLatest`.

## Additional Findings

### The `outdated.go` rewrite is more involved than described

The current `outdated.go` does more than just call `res.ResolveGitHub()`. It:
1. Loads recipes using `loadRecipeForTool` (source-directed loading with distributed source support)
2. Extracts the `repo` field from step params
3. Compares versions via simple string inequality

The rewrite needs to:
1. Keep the `loadRecipeForTool` integration (it's already there and correct)
2. Replace the repo extraction + `ResolveGitHub` call with `ProviderFactory.ProviderFromRecipe(recipe)` + `ResolveWithinBoundary`
3. Handle the case where `ProviderFactory` can't create a provider (recipe has no version source)
4. Read `Requested` from the active version's `VersionState`, not from `ToolState` directly

Point 4 is important: `Requested` lives at `state.Installed[name].Versions[activeVersion].Requested`, not at `state.Installed[name].Requested`. The design doesn't specify this navigation path.

### CachedVersionLister receives a VersionLister, not a VersionResolver

`NewCachedVersionLister` takes a `VersionLister` parameter. The `ResolveWithinBoundary` helper needs to decide: does it create a `CachedVersionLister` wrapper, or does it expect the caller to pass one? If the helper creates it, it needs the `cacheDir` and `ttl` parameters, which couples it to config. If the caller creates it, the helper's signature should accept `VersionLister` directly rather than `VersionResolver`.

The design's signature is `ResolveWithinBoundary(ctx, provider VersionResolver, requested)`. This accepts any provider. Internally, the helper does a type assertion `provider.(VersionLister)` to check if listing is available. But the cached wrapper should be applied *before* passing to the helper, not inside it. The helper should not be responsible for constructing cache wrappers.

Recommendation: callers should wrap providers in `CachedVersionLister` before calling `ResolveWithinBoundary`. The helper accepts `VersionResolver` and type-asserts to `VersionLister` when available. This keeps the helper simple and cache-agnostic.

### Version comparison in outdated

The current `outdated.go` uses simple string inequality (`latest.Version != tool.Version`). This will flag a tool as "outdated" even when the installed version is *newer* than what the pin allows. For example, if a user installed `node@22.1.0` (PinExact) and the latest 22.x is `22.5.0`, the tool shows as outdated even though it shouldn't auto-update.

The design should clarify: `outdated` shows tools where a newer version exists *within the pin boundary*. For PinExact, nothing should ever show as outdated. The `PinLevel` enum would be useful here to suppress PinExact tools from the outdated list.

## Summary of Recommendations

1. **Fix the `ListVersions` return type** in the design doc (`[]string`, not `[]*VersionInfo`). Document the strategy for converting a matched version string back to `*VersionInfo`.

2. **Clarify the `update.go` integration point.** The current update command doesn't do version resolution; it delegates to the install pipeline. Specify where pin-aware resolution hooks in.

3. **Correct the FossilTimelineProvider claim.** It implements `VersionLister`. Only `CustomProvider` is VersionResolver-only.

4. **Document the `Requested` field navigation path.** It's `state.Installed[name].Versions[activeVersion].Requested`, which requires knowing the active version first.

5. **Consider deferring the `CachedVersionLister.ResolveLatest` modification** (Decision 3) to reduce risk. The helper can use `ListVersions` through the cached wrapper directly.

6. **Add PinExact suppression to `outdated`.** Exactly-pinned tools should not appear in the outdated list.

7. **Account for version normalization in `VersionMatchesPin`.** Providers handle v-prefixes differently. The pin matching logic should normalize before comparing.
