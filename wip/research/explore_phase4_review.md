# Phase 4 Review: Problem and Options Analysis

## Problem Statement Assessment

**Clarity: Strong with one significant ambiguity**

The problem statement clearly articulates:
- What works: Lockfile pinning via `EvalConstraints` (pip requirements, go.sum, Cargo.lock)
- What fails: Toolchain/recipe dependency versions (python-standalone, go, nodejs)
- Concrete failure scenario with specific version examples
- Current impact: `validate-golden-code.yml` workflow is disabled

**Ambiguity: "Direct dependencies only" scope limitation is underspecified**

The scope states "supporting direct dependencies only (no transitive toolchain dependencies)" but the implementation context shows recursive dependency traversal in `generateDependencyPlans()` and `extractConstraintsFromDependency()`. The design proposes recursive extraction:

```go
func extractDependencyVersions(plan *InstallationPlan, versions map[string]string) {
    for _, dep := range plan.Dependencies {
        versions[dep.Tool] = dep.Version
        // Recursively extract from nested dependencies
        extractDependencyVersionsFromDep(&dep, versions)
    }
}
```

This is actually correct behavior (plans already contain nested dependencies with versions), but the scope statement creates confusion. **Recommendation**: Clarify that "direct dependencies" refers to dependencies declared by the recipe, which may themselves have transitive dependencies that are also pinned because they appear in the golden file's dependency tree.

**Missing Context: Why is the workflow disabled?**

The design references the disabled workflow but doesn't quantify the impact. How many golden files are affected? How often do toolchain updates occur? This would help prioritize the work.

## Missing Alternatives Analysis

### Alternative A: Golden File Regeneration Cadence (Partial Solution)

Instead of pinning, establish a systematic regeneration cadence aligned with toolchain release schedules. For example:
- Regenerate python-standalone golden files weekly (high update frequency)
- Regenerate go golden files on minor releases

**Why not considered**: This doesn't solve CI stability for the time between regeneration runs. However, it should be mentioned as a complementary strategy, not an alternative.

### Alternative B: Dual-Mode Validation

Keep both constrained (pinned) and unconstrained (live) validation:
- Constrained mode catches code regressions (deterministic)
- Unconstrained mode catches incompatibilities with new toolchain versions

**Why valuable**: The current design pins everything, which means CI won't catch cases where a new toolchain version breaks the recipe. A secondary "smoke test" job could run unconstrained to catch these proactively.

### Alternative C: Version Range Constraints

Instead of exact version pins, allow semver range constraints:
- `python-standalone@^20260113` (any 2026 version)
- `go@1.25.x` (any 1.25 patch)

**Why not considered**: More complex to implement and validate. The current approach of exact pinning with graceful fallback achieves similar goals with less complexity.

### Alternative D: Toolchain Version in Recipe Metadata

Some recipes may want to pin their toolchain version explicitly in the recipe (not just golden files):
```toml
[metadata]
toolchain_versions = { python = "3.13", go = "1.25" }
```

This is similar to Option 2 but specifically for toolchain major/minor versions rather than exact versions.

**Assessment**: This is a different problem (recipe portability across toolchain versions) and correctly out of scope.

## Option Analysis Fairness

### Option 1: Extend EvalConstraints (Recommended)

**Pros assessment: Fair and complete**
- Correctly identifies minimal code changes
- Correctly notes data already exists in golden files
- Correctly identifies pattern consistency

**Cons assessment: Incomplete**

Missing cons:
1. **Version availability timing**: When a pinned version is no longer available (removed from registry), the fallback behavior changes the plan's determinism. The plan becomes "whatever is available" rather than "what was specified".
2. **Dependency version conflicts**: If two dependencies pin different versions of the same transitive toolchain (e.g., recipe A pins `go@1.25.5`, dependency B was generated with `go@1.25.6`), which wins? The design doesn't address this.
3. **Recursive extraction complexity**: The "recursive extraction needed" con is understated. The current `extractConstraintsFromDependency()` already recurses, so this is solved, but it should note this.

### Option 2: Recipe-Level Version Locks

**Assessment: Fairly presented as high-effort**

The pros/cons are accurate. This is correctly rejected for the stated reasons.

**Additional context**: This option would be valuable for a different use case: users who want reproducible builds of specific recipes, not just CI validation. It could be future work, not an alternative to Option 1.

### Option 3: Version Provider Constraint Mode

**Assessment: Fairly presented as invasive**

The cons correctly identify this as overkill. However, the analysis could note that this approach would be needed if we wanted to support partial constraints (pin some dependencies, resolve others fresh). Option 1's binary approach (all pinned or all resolved) is simpler but less flexible.

### Option 4: Plan-Level Dependency Snapshot

**Assessment: Correctly identified as redundant**

The key insight is correct: `dependencies[].version` already provides this data. A separate `resolved_dependencies` field would duplicate information.

**Minor correction**: The con states "Requires regenerating all golden files" but this is only true if the format change is breaking. A non-breaking addition of the field wouldn't require regeneration.

## Unstated Assumptions

### Assumption 1: Toolchain versions are available indefinitely

The design's fallback behavior assumes removed versions are rare. If toolchain providers aggressively prune old versions (e.g., python-standalone), the fallback path becomes the common path, undermining the pinning benefit.

**Should be explicit**: Document the expected availability window for major toolchains and adjust fallback strategy accordingly.

### Assumption 2: Version resolution order is deterministic

`generateDependencyPlans()` processes dependencies in alphabetical order (`sortStrings(depNames)`). The design assumes this produces consistent results, but if two dependencies have the same name with different sources, order could matter.

**Risk: Low** - This is already solved in the current implementation.

### Assumption 3: Golden file format is stable

The extraction logic parses `dependencies[].tool` and `dependencies[].version`. If the plan format changes (e.g., format_version bump), extraction may break.

**Should be explicit**: Add version checking in `ExtractConstraintsFromPlan()` to handle format migrations.

### Assumption 4: Single version per toolchain is sufficient

The design maps `toolName -> version`, implying one version per toolchain. If a complex recipe uses multiple Go versions (unlikely but possible), this would collapse to the last encountered version.

**Risk: Very low** - Recipes don't typically mix toolchain versions.

### Assumption 5: Dependency plan generation uses the same code path as golden file generation

The integration point is `generateSingleDependencyPlan()`. The design assumes this function is called during both golden file generation and constrained evaluation. If there are alternative code paths, pinning might not apply.

**Verified**: Looking at `plan_generator.go`, `generateDependencyPlans()` is called from `GeneratePlan()`, which is the same path for both modes.

## Strawman Detection

**No strawmen detected.**

All four options are legitimate approaches with real tradeoffs:

- **Option 1** is clearly the best fit for the specific problem (lowest effort, highest compatibility)
- **Option 2** solves a different (broader) problem and is correctly rejected for this scope
- **Option 3** is architecturally cleaner but invasive for the current need
- **Option 4** is redundant given existing data structures

The analysis fairly weighs each option against the decision drivers. Option 1's selection is well-justified, not predetermined.

## Recommendations

### Critical: Address version conflict semantics

Add a section specifying how version conflicts are resolved when:
1. Parent recipe pins `go@1.25.5`
2. Nested dependency golden file was generated with `go@1.25.6`

Recommendation: First-encountered wins (depth-first), matching the existing recursive extraction pattern. Document this explicitly.

### Important: Add version availability validation

During constraint extraction, validate that pinned versions exist before using them:
```go
// During plan generation (not extraction)
if pinnedVersion, ok := cfg.Constraints.DependencyVersions[depName]; ok {
    if !isVersionAvailable(depName, pinnedVersion) {
        log.Warn("pinned version unavailable", ...)
        // Continue with latest
    }
}
```

This provides clear feedback rather than silent fallback.

### Minor: Rename for clarity

Consider renaming `DependencyVersions` to `ToolchainVersions` or `PinnedDependencyVersions` to distinguish from lockfile constraints (PipConstraints, GoSum, etc.).

### Enhancement: Diagnostic mode

Add a `--diagnose-constraints` flag to `tsuku eval` that reports:
- Which dependency versions were pinned
- Which fell back to latest (and why)
- Version mismatch warnings

This aids debugging when golden file validation fails after this feature is implemented.

### Documentation: Add complementary strategy

The design should recommend regenerating golden files on a regular cadence (weekly or on toolchain releases) to keep pinned versions current and available. This complements pinning by ensuring versions don't become unavailable.

### Format versioning

Add a comment in `EvalConstraints` noting that extraction depends on plan format version 3. If format version changes, extraction logic may need updates.

## Summary

The design document is well-structured with a clear problem statement and reasonable option analysis. Option 1 is the correct choice. The main gaps are:

1. Version conflict resolution semantics (critical)
2. Scope clarification for "direct dependencies" (minor wording)
3. Unstated assumptions about version availability (should be documented)
4. Missing complementary strategy for keeping golden files fresh

The implementation approach is sound and builds naturally on the existing `EvalConstraints` infrastructure.
