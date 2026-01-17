# Architecture Review: Toolchain Dependency Pinning

## Executive Summary

The proposed solution architecture for Toolchain Dependency Pinning is **sound and implementable**. The design follows established patterns in the codebase, makes minimal changes, and integrates naturally with the existing constrained evaluation infrastructure. This review identifies no critical gaps but suggests refinements to improve clarity and robustness.

**Overall Assessment: APPROVED with minor clarifications**

---

## Question 1: Is the architecture clear enough to implement?

**Answer: Yes, with one clarification needed.**

### Strengths

1. **Data flow diagram is clear**: The flow from golden file through `ExtractConstraints()` to `generateSingleDependencyPlan()` is well-documented.

2. **Code examples are concrete**: The design provides actual function signatures and implementation patterns that match the existing codebase style.

3. **Integration points are precise**: The design identifies exactly where changes occur:
   - `EvalConstraints` struct in `internal/actions/decomposable.go` (line 60-85)
   - `ExtractConstraintsFromPlan()` in `internal/executor/constraints.go` (line 32-53)
   - `generateSingleDependencyPlan()` in `internal/executor/plan_generator.go` (line 678-741)

### Clarification Needed

**Constraint propagation to nested dependency plans**: The current `generateSingleDependencyPlan()` function does NOT pass `cfg.Constraints` when recursively generating nested plans:

```go
// Current code at plan_generator.go:706-716
depCfg := PlanConfig{
    OS:                 cfg.OS,
    Arch:               cfg.Arch,
    RecipeSource:       "dependency",
    OnWarning:          cfg.OnWarning,
    Downloader:         cfg.Downloader,
    DownloadCache:      cfg.DownloadCache,
    AutoAcceptEvalDeps: cfg.AutoAcceptEvalDeps,
    OnEvalDepsNeeded:   cfg.OnEvalDepsNeeded,
    RecipeLoader:       nil, // Don't recurse here - we handle it above
    // MISSING: Constraints field is not passed!
}
```

**Recommendation**: The design should explicitly note that `cfg.Constraints` must be passed to `depCfg` so that:
1. The `DependencyVersions` map is available during nested dependency resolution
2. Ecosystem constraints (PipConstraints, GoSum, etc.) propagate correctly to nested builds

---

## Question 2: Are there missing components or interfaces?

**Answer: One minor addition needed.**

### Current Interface Coverage

The design correctly leverages existing interfaces:
- `EvalConstraints` struct: Already exists, just needs new field
- `ExtractConstraintsFromPlan()`: Already exists, needs extended logic
- `PlanConfig.Constraints`: Already exists and is passed to plan generator

### Missing Component: Helper Function

The design proposes `extractDependencyVersions()` and `extractDependencyVersionsFromDep()` functions but should also add a convenience function parallel to existing helpers:

```go
// Existing pattern (constraints.go):
func HasPipConstraints(constraints *EvalConstraints) bool
func HasGoSumConstraint(constraints *EvalConstraints) bool
func HasCargoLockConstraint(constraints *EvalConstraints) bool
// ... etc

// Should add for consistency:
func HasDependencyVersionConstraints(constraints *EvalConstraints) bool {
    return constraints != nil && len(constraints.DependencyVersions) > 0
}

func GetDependencyVersionConstraint(constraints *EvalConstraints, toolName string) (string, bool) {
    if constraints == nil || constraints.DependencyVersions == nil {
        return "", false
    }
    ver, ok := constraints.DependencyVersions[toolName]
    return ver, ok
}
```

This follows the established pattern and improves testability.

---

## Question 3: Are the implementation phases correctly sequenced?

**Answer: Yes, the phasing is logical and correctly ordered.**

### Phase Dependency Analysis

| Phase | Dependencies | Rationale |
|-------|--------------|-----------|
| Phase 1: Field + Extraction | None | Foundation - struct change enables everything else |
| Phase 2: Plan Generator | Phase 1 | Requires `DependencyVersions` field to exist |
| Phase 3: Validation + Testing | Phase 1-2 | Requires implementation to test |
| Phase 4: Documentation | Phase 1-3 | Must reflect final implementation |

### Sequencing Correctness

The phases correctly follow the dependency chain. The design avoids the anti-pattern of parallel implementation that could lead to integration issues.

### Suggested Refinement

Consider splitting Phase 2 into two sub-phases for smaller, more reviewable PRs:

- **Phase 2a**: Add constraint check in `generateSingleDependencyPlan()` (core change)
- **Phase 2b**: Add graceful fallback for unavailable versions (error handling)

This allows the core functionality to land first, with error handling as a follow-up.

---

## Question 4: Are there simpler alternatives we overlooked?

**Answer: No fundamentally simpler alternatives exist, but one variant is worth considering.**

### Evaluated Alternatives

#### Alternative A: Inline constraint check (rejected)

Instead of adding a field to `EvalConstraints`, pass `DependencyVersions` directly in `PlanConfig`:

```go
type PlanConfig struct {
    // ... existing fields ...
    DependencyVersions map[string]string // NEW: direct field
}
```

**Why rejected**: This breaks the established pattern where all constraints go through `EvalConstraints`. It would create two parallel constraint systems.

#### Alternative B: Recipe-level locks (rejected in design)

The design correctly rejected this as it requires format changes and doesn't help existing golden files.

#### Alternative C: Version provider constraints (rejected in design)

The design correctly rejected this as too invasive for the specific problem.

### Variant Worth Considering: Lazy extraction

Instead of extracting `DependencyVersions` upfront in `ExtractConstraintsFromPlan()`, extract lazily when first needed:

```go
type EvalConstraints struct {
    // ... existing fields ...
    dependencyVersions     map[string]string
    dependencyVersionsOnce sync.Once
    goldenPlan             *InstallationPlan // Cached for lazy extraction
}

func (c *EvalConstraints) GetDependencyVersion(tool string) (string, bool) {
    c.dependencyVersionsOnce.Do(func() {
        c.dependencyVersions = extractDependencyVersionsFromPlan(c.goldenPlan)
    })
    return c.dependencyVersions[tool]
}
```

**Pros**: Avoids upfront work if dependencies aren't needed
**Cons**: Adds complexity, requires storing the full plan

**Recommendation**: The design's eager extraction approach is simpler and appropriate since:
1. Golden file validation typically needs all constraints
2. The extraction is cheap (just iterating nested structs)
3. Lazy extraction adds complexity without significant benefit

---

## Implementation Risk Assessment

### Low Risk Items

1. **Field addition to EvalConstraints**: Backward compatible, existing code unaffected
2. **Extraction logic**: Follows existing patterns (see constraints.go lines 56-67)
3. **Constraint check in plan generator**: Single conditional, isolated change

### Medium Risk Items

1. **First-encountered-wins semantics**: Need to verify the golden file's dependency order matches depth-first traversal expectations
2. **Fallback behavior**: Version resolution errors may manifest differently across providers

### Mitigation Strategies

1. **Ordering verification**: Add tests with diamond dependencies to verify first-wins behavior
2. **Error handling**: Log specific warning for each constraint application attempt

---

## Codebase Consistency Check

### Pattern Adherence

The design follows these established patterns:

| Pattern | Location in Codebase | Design Follows |
|---------|---------------------|----------------|
| Constraint struct fields | `decomposable.go:60-85` | Yes |
| Extraction from steps | `constraints.go:56-67` | Yes |
| First-wins semantics | `constraints.go:148-152` | Yes |
| Has* helper functions | `constraints.go:122-134` | Yes |
| Get* helper functions | `constraints.go:128-134` | Yes |
| Warning on constraint issues | `plan_generator.go:102-108` | Yes |

### Naming Conventions

The proposed names follow codebase conventions:
- `DependencyVersions` (field) - matches `PipConstraints` style
- `extractDependencyVersions()` - matches `extractPipConstraintsFromSteps()` style
- `HasDependencyVersionConstraints()` - matches `HasPipConstraints()` style

---

## Testing Strategy Validation

The design proposes:

1. **Unit tests for extraction**: Test nested dependency traversal
2. **Integration tests with mock recipes**: Test constraint application
3. **Fallback tests**: Test unavailable version handling

### Additional Tests Recommended

1. **Diamond dependency test**: When A depends on B and C, and both B and C depend on D (same tool), verify first-encountered-wins
2. **Empty constraints test**: Verify behavior when golden file has no dependencies
3. **Mixed constraints test**: Verify DependencyVersions + PipConstraints work together

---

## Final Recommendations

1. **Proceed with implementation** as designed - architecture is sound
2. **Add constraint propagation** to `depCfg` in `generateSingleDependencyPlan()`
3. **Add helper functions** for consistency (`HasDependencyVersionConstraints`, `GetDependencyVersionConstraint`)
4. **Consider PR splitting** Phase 2 into core functionality and error handling
5. **Add diamond dependency test** to validate first-wins semantics

---

## Appendix: Code Location Quick Reference

| Component | File | Line |
|-----------|------|------|
| EvalConstraints struct | `internal/actions/decomposable.go` | 60-85 |
| ExtractConstraintsFromPlan | `internal/executor/constraints.go` | 32-53 |
| extractConstraintsFromDependency | `internal/executor/constraints.go` | 56-67 |
| generateSingleDependencyPlan | `internal/executor/plan_generator.go` | 678-741 |
| DependencyPlan struct | `internal/executor/plan.go` | 64-79 |
| PlanConfig struct | `internal/executor/plan_generator.go` | 19-55 |
| --pin-from flag handling | `cmd/tsuku/eval.go` | 258-267 |
