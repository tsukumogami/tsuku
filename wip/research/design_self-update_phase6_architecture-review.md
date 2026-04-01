# Architecture Review: DESIGN-self-update

**Reviewer**: architect-reviewer
**Date**: 2026-03-31
**Design**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/designs/DESIGN-self-update.md`
**Related**: `docs/designs/current/DESIGN-background-update-checks.md`

## Summary

The self-update design proposes three components: (1) asset resolution and download via direct URL construction, (2) binary replacement via two-rename-with-backup, and (3) cache integration via a well-known `SelfToolName` constant appended to `RunUpdateCheck`. The design fits the existing architecture well. Two structural concerns warrant attention before implementation.

---

## Question 1: Is the architecture clear enough to implement?

**Yes, with one gap.**

The design specifies file placement, function signatures, data flow, and edge cases in enough detail to implement directly. The cobra command registration pattern matches `cmd_check_updates.go`. The `internal/updates/self.go` placement follows the existing package structure.

**Gap: `checkSelf` parameter list vs existing `RunUpdateCheck` signature.** The design says `checkSelf(ctx, cacheDir, res)` takes a `*version.Resolver`, but `RunUpdateCheck` currently constructs both `version.New()` (the Resolver) and `version.NewProviderFactory()` (the factory). The `GitHubProvider` constructor requires a `*Resolver`, not a `*ProviderFactory`. The design's signature is correct -- `checkSelf` needs `version.New()` to call `NewGitHubProvider(res, SelfRepo).ResolveLatest(ctx)`. This works but the design should note that `checkSelf` creates its own `GitHubProvider` inline rather than going through `ProviderFactory.ProviderFromRecipe`. This is architecturally correct per PRD D5 (no recipe for tsuku), but an implementer might hesitate. **Advisory.**

## Question 2: Are there missing components or interfaces?

**Two items.**

### 2a. Version comparison logic (missing detail)

The design says "Compare against `buildinfo.Version()` -- if equal, report 'already up to date'." But `buildinfo.Version()` returns strings like `"v0.5.0"` (release) or `"dev-abc123def012"` (dev builds). `GitHubProvider.ResolveLatest()` returns a `VersionInfo` where `Version` is the normalized version (e.g., `"0.5.0"` without `v` prefix -- the resolver strips `v` prefixes).

The design needs to specify how the comparison works. Options:
- Strip `v` prefix from `buildinfo.Version()` before comparing
- Use semver parsing for both sides
- String equality on the `Tag` field instead of `Version`

Without this, implementers will either get false "update available" on every run (if `"v0.5.0" != "0.5.0"`) or silently skip updates. The `checkSelf` function in the cache integration has the same issue since it writes `ActiveVersion: buildinfo.Version()` and `LatestOverall: resolved.Version` -- consumers comparing these fields will hit the same mismatch.

**Blocking.** This affects correctness of both the command and the cache entry.

### 2b. `MaybeAutoApply` skip guard (design says modify `apply.go`, but the existing code has no `SelfToolName` awareness)

The design specifies: `MaybeAutoApply gains a one-line skip: if entry.Tool == SelfToolName { continue }`. Looking at the existing `apply.go:46`, the filter loop checks `e.LatestWithinPin != ""`. Since `checkSelf` writes an entry with empty `LatestWithinPin` (the design says it sets empty `Requested` and `LatestWithinPin`), the existing filter already excludes tsuku from auto-apply -- the `LatestWithinPin` field will be empty because there's no pin boundary for tsuku.

However, the design should still add the explicit guard as defense-in-depth. If a future change to `checkSelf` starts populating `LatestWithinPin`, the implicit exclusion breaks silently. The design's approach is correct. **Advisory -- note the implicit exclusion exists but the explicit guard is still the right call.**

## Question 3: Are the implementation phases correctly sequenced?

**Yes.** The sequencing is sound:

1. **Phase 1 (cache integration)** adds the constant and `checkSelf` to the background checker. This is the correct foundation -- `tsuku outdated` gets self-update awareness immediately.
2. **Phase 2 (self-update command)** adds the user-facing action. Depends on Phase 1 only for the `SelfToolName` constant (and could even inline it temporarily).
3. **Phase 3 (outdated display)** formats the cache entry differently. Naturally last since it consumes what Phase 1 produces.

One note: the `MaybeAutoApply` skip guard is listed in Phase 1 but `apply.go` was introduced by Feature 3. If Feature 3 hasn't landed yet, Phase 1 should note this dependency explicitly so the implementer knows to either wait or skip that deliverable. Looking at the existing code, `apply.go` already exists with the full `MaybeAutoApply` implementation, so this dependency is satisfied. **No issue.**

## Question 4: Are there simpler alternatives we overlooked?

**One alternative worth noting, but the design's choice is defensible.**

### Alternative: Use `gh` release download conventions instead of manual HTTP

The design constructs download URLs manually and parses `checksums.txt` with custom code. An alternative: use the existing `Resolver.ResolveGitHub` infrastructure to get the tag, then use `version.Resolver`'s HTTP client (which already handles auth tokens, timeouts, and retries) for downloads instead of presumably creating a new `http.Client`.

The design doesn't specify which HTTP client the download uses. The `version.Resolver` has a configured `httpClient` field with security hardening (`httputil` package, timeouts, compression settings). If `cmd_self_update.go` creates its own `http.Client` or uses `http.DefaultClient`, it would bypass this hardening. The design should specify that downloads use the same HTTP infrastructure.

**Advisory.** Not a structural violation, but a missed reuse opportunity that could introduce a parallel HTTP client pattern.

### The design's core choices are the right ones

- Direct URL construction over API-based asset discovery: correct. The naming convention is controlled and deterministic.
- Two-rename over direct overwrite: correct. Industry standard, well-understood failure modes.
- Well-known constant over struct field: correct. Avoids schema change for a single entry.
- Separate code path over recipe pipeline: correct per PRD D5. No bootstrap risk.

---

## Structural Findings

### Finding 1: Version string normalization mismatch -- Blocking

**Location**: Design Decision 1 step 1, and Decision 3 `checkSelf` output.

`buildinfo.Version()` returns `"v0.5.0"` (with `v` prefix, set by goreleaser ldflags). `GitHubProvider.ResolveLatest()` returns `VersionInfo.Version` as `"0.5.0"` (stripped). The design says "compare against `buildinfo.Version()` -- if equal" but doesn't address the prefix mismatch. Both the command's early-exit check and the cache entry's `ActiveVersion` vs `LatestOverall` comparison will produce incorrect results.

**Fix**: Specify that `runSelfUpdate` and `checkSelf` strip the `v` prefix from `buildinfo.Version()` before comparison (e.g., `strings.TrimPrefix(buildinfo.Version(), "v")`). Or use the `Tag` field from `VersionInfo` which preserves the `v` prefix. Either way, document the normalization.

### Finding 2: HTTP client for binary download unspecified -- Advisory

**Location**: Design Decision 1 steps 4-5.

The design specifies what to download but not how. The existing codebase has `httputil.NewClient()` with security hardening (timeouts, compression bomb prevention). If the self-update command creates a bare `http.Client`, it introduces a parallel HTTP pattern. The download path should use the same client infrastructure.

### Finding 3: `GitHubProvider` constructed without recipe -- pattern note, not a violation

**Location**: Design Decision 3, `checkSelf` implementation.

The design has `checkSelf` creating `NewGitHubProvider(res, SelfRepo)` directly, bypassing `ProviderFactory.ProviderFromRecipe`. This is architecturally correct -- tsuku has no recipe (PRD D5), so the factory can't be used. The `GitHubProvider` constructor is a public API designed for direct use. This is the right call and doesn't violate the provider registration pattern since `ProviderFactory` is a convenience for recipe-driven resolution, not an exclusive gateway.

### Finding 4: Design references `version.Resolver` as `res` but `New()` constructor name may confuse -- Advisory

**Location**: Architecture section, `checkSelf` signature.

The `version.New()` function returns a `*Resolver`. The design's `checkSelf(ctx, cacheDir, res *version.Resolver)` is correct but the parameter name `res` is ambiguous in a codebase where `res` commonly means "result." Consider `resolver` in the implementation. Minor naming concern, not structural.

---

## Compatibility with Existing Code

### `internal/updates/checker.go`

The design's Phase 1 modifies `RunUpdateCheck` to call `checkSelf()` after the tool loop (line 86-88 in current code, before `TouchSentinel`). The insertion point is clear. The `checkSelf` function needs access to `ctx`, `cacheDir`, and a `*version.Resolver` -- all of which are already in scope inside `RunUpdateCheck`. The `userCfg` parameter is also available for the `UpdatesSelfUpdate()` gate. Clean integration.

### `internal/updates/cache.go`

No modifications needed. `WriteEntry` and `ReadAllEntries` work with any tool name including `"tsuku"`. The `SelfToolName` constant goes in the new `self.go` file. No schema changes.

### `internal/updates/apply.go`

The skip guard `if entry.Tool == SelfToolName { continue }` goes in the `for _, e := range entries` loop at line 46. The import of the constant is package-internal (same package). Clean.

### `internal/updates/trigger.go`

No changes needed. The trigger spawns `check-updates`, which calls `RunUpdateCheck`, which will now include the self-check.

### `cmd/tsuku/main.go`

Adding `rootCmd.AddCommand(selfUpdateCmd)` follows the existing pattern (see `cmd_check_updates.go:init()`). The `self-update` command name needs to be added to the PersistentPreRun skip list since it shouldn't trigger background checks while performing a self-update. **The design doesn't mention this.** Advisory -- the consequence is minor (a redundant background spawn during self-update) but it's a completeness gap.

---

## Verdict

The design is well-structured and fits the existing architecture. The core decisions (direct URL construction, two-rename replacement, well-known constant) are the simplest correct choices for each concern. One blocking issue (version string normalization) must be resolved before implementation. The remaining items are advisory and can be addressed during implementation without design revision.
