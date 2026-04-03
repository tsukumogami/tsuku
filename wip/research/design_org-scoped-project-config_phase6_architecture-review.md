# Architecture Review: DESIGN-org-scoped-project-config

**Reviewer**: Architect  
**Date**: 2026-04-03  
**Scope**: Structural fit of the proposed design against existing codebase patterns

---

## 1. Is the architecture clear enough to implement?

**Yes, with one gap.**

The design specifies exact function names, data flows, file locations, and code diffs for the resolver change. The data flow diagrams cover both the install path and the shell integration path. An implementer can work from this without ambiguity on the resolver side (Phase 1-2).

**Gap: Phase 3 install loop detail is underspecified for the `runInstallWithTelemetry` call.** The CLI distributed path (install.go:213-280) does significantly more than just calling `runInstallWithTelemetry` -- it also calls `loader.GetWithContext` with the qualified name, `loader.CacheRecipe` under the bare name, and `fetchRecipeBytes` + `recordDistributedSource` after install. The design's "Components" section (lines 129-133) lists these steps, but the prose summary (lines 87-89) says "passes them through the existing loader path" which could be read as implying `runInstallWithTelemetry` handles qualified names natively. It doesn't -- the qualified-name loading, cache aliasing, and post-install recording are all done by the caller in install.go. The implementer needs to replicate this sequence in install_project.go.

**Recommendation**: No design change needed; the Components section is sufficiently detailed. But an implementer should treat lines 119-133 of the design as the specification, not the prose summary.

## 2. Are there missing components or interfaces?

### 2a. `splitOrgKey` placement -- **Advisory**

The design places `splitOrgKey` in `internal/project/orgkey.go` and says it "delegates to the existing `parseDistributedName` parsing logic." But `parseDistributedName` lives in `cmd/tsuku/install_distributed.go` -- a `main` package file. `internal/project` cannot import `cmd/tsuku`. The design acknowledges this ("lives in `internal/project/` to avoid a dependency from `internal/project` on `cmd/tsuku`") but the phrasing "delegates to" is misleading -- it must reimplement the splitting logic, not call `parseDistributedName`.

This creates a **parallel pattern**: two functions that parse `owner/repo:recipe` strings with slightly different interfaces. The design is aware of the tradeoff (the "extract shared helper" alternative was rejected because error handling differs). The risk is real but contained -- `splitOrgKey` is a pure string splitter that doesn't need the `@version` parsing or `..` rejection that `parseDistributedName` handles, so divergence is limited.

**Recommendation**: Acceptable as designed. The functions serve different layers (config key parsing vs. CLI argument parsing) and the overlap is small. If both grow, extract the shared logic to `internal/project/` and have `install_distributed.go` call down.

### 2b. No interface change to `ProjectVersionResolver` -- **Correct**

The design modifies the concrete `Resolver` struct without changing the `ProjectVersionResolver` interface. This is architecturally correct -- the interface contract (command -> version) doesn't change, only the internal lookup strategy does. Callers in `autoinstall` are unaffected.

### 2c. Missing: how `runProjectInstall` accesses `loader` and `globalCtx` -- **Not a gap**

The design's pre-scan calls `ensureDistributedSource`, which internally calls `addDistributedProvider`, which uses `globalCtx` and `loader` (package-level vars in `cmd/tsuku`). Since `install_project.go` is in the same package, this works without any new wiring. Confirmed by reading the existing code.

### 2d. Missing: recipe hash computation in project install path

The CLI path (install.go:254) calls `fetchRecipeBytes` before install and `recordDistributedSource` after. The design's Components section (line 133) mentions `recordDistributedSource` but doesn't mention `fetchRecipeBytes` or `computeRecipeHash`. The recipe hash is part of the audit trail for distributed installs.

**Recommendation**: Phase 3 implementation should include `fetchRecipeBytes` + `computeRecipeHash` for org-scoped tools, matching the CLI path. This is an omission in the design's component list, not an architectural issue.

## 3. Are the implementation phases correctly sequenced?

**Yes. The sequencing is sound.**

- Phase 1 (splitOrgKey) has no dependencies -- pure function.
- Phase 2 (resolver) depends on Phase 1 only. Can be tested in isolation with mock configs.
- Phase 3 (project install) depends on Phase 1 for key detection. It reuses existing functions from `install_distributed.go` that are already tested. This is the riskiest phase since it touches the batch install loop.
- Phase 4 (functional tests) depends on all prior phases.

One improvement: Phase 2 and Phase 1 could be a single PR since `splitOrgKey` is only a few lines and its only consumer in Phase 2 is the resolver. Shipping Phase 1 alone creates a dead-code window. Minor point.

## 4. Are there simpler alternatives overlooked?

### 4a. Reuse `parseDistributedName` directly instead of `splitOrgKey`

`parseDistributedName` already handles `owner/repo` and `owner/repo:recipe`. The resolver could call it directly if it were moved from `cmd/tsuku` to `internal/project` or a shared package. This eliminates the parallel-pattern concern from 2a.

**Why this was likely rejected**: Moving `parseDistributedName` would also move `distributedInstallArgs` and create a dependency from a low-level package on parsing logic that includes version handling (`@version`) irrelevant to the resolver. The design's tradeoff is reasonable.

### 4b. Normalize keys at config load time

Instead of dual-key lookup in the resolver, `LoadProjectConfig` could detect org-scoped keys and store both the original key and a `bareKey` field in `ToolRequirement`. The resolver would then match on `ToolRequirement.BareKey` or the map key interchangeably.

**Why this is worse**: It changes the `ToolRequirement` struct (a config contract), adds implicit state to the config object, and makes iteration over `config.Tools` confusing. The design correctly rejected this class of approach.

### 4c. Don't build `bareToOrg` at construction -- do linear scan in `ProjectVersionFor`

For small tool counts (typical `.tsuku.toml` has 5-20 tools), a linear scan of `config.Tools` keys checking `strings.HasSuffix(key, "/"+m.Recipe)` would be simpler than maintaining a reverse map. The map is premature optimization for a hot path that runs once per shim invocation.

**Tradeoff**: The map is O(1) vs O(n) per lookup, but n is small and the map construction is O(n) anyway. The map approach is fine -- it's not over-engineered, just slightly more structured than necessary. **Not worth changing.**

## Structural Findings

### Finding 1: Parallel name-parsing logic -- **Advisory**

`splitOrgKey` (new, `internal/project/`) and `parseDistributedName` (existing, `cmd/tsuku/`) both parse `owner/repo` strings. They serve different layers and have different return types, so this is a contained duplication, not a compounding pattern. No other code needs to parse this format.

**Impact if left as-is**: Low. If a third consumer appears, the duplication should be extracted.

### Finding 2: Project install reuses distributed functions correctly -- **No issue**

The design routes through `ensureDistributedSource`, `checkSourceCollision`, and `recordDistributedSource` -- the same functions the CLI path uses. No dispatch bypass. No parallel pattern for source management.

### Finding 3: Resolver change preserves interface contract -- **No issue**

`ProjectVersionResolver` interface is unchanged. The `bareToOrg` map is internal to the concrete `Resolver` struct. No state contract violation.

### Finding 4: No state.json schema change -- **No issue**

The design explicitly avoids changes to state.json, config structs, or the binary index schema. Existing consumers are unaffected.

### Finding 5: Dependency direction is correct -- **No issue**

`internal/project` depends on `internal/autoinstall` (for `LookupFunc`). The new `splitOrgKey` in `internal/project` has no new imports. `cmd/tsuku` depends on `internal/project`. No inversion.

### Finding 6: Name collision resolution is nondeterministic -- **Advisory**

The design acknowledges that when two orgs provide a tool with the same binary name, resolver picks "first match" which depends on Go map iteration order. This is documented in the Consequences section. The scenario is unlikely (requires two org-scoped tools with identical binary names in the same `.tsuku.toml`), and the `owner/repo:recipe` qualified syntax provides an escape hatch.

**Impact if left as-is**: Nondeterministic behavior for an edge case. If it matters, sort `bareToOrg` values deterministically (e.g., alphabetically) so behavior is at least reproducible.

## Summary

The design fits the existing architecture well. It reuses the distributed-source machinery without bypass, preserves all existing contracts, and introduces no new packages or interfaces beyond a small utility function. The two advisory findings (parallel name parsing, nondeterministic collision resolution) are contained and documented.

**Verdict**: No blocking findings. Ready to implement.
