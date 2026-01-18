# Architecture Review: Embedded Recipe List Design

## Reviewer Role: Architecture Review

## Executive Summary

The design is **clear and implementable** with minor clarifications needed. The chosen approach (1D: Resolver Wrapper + 2A: Markdown Only + 3A: Regenerate and Compare) is sound and leverages existing, proven infrastructure.

**Verdict**: Ready for implementation with 3 recommended improvements.

---

## Analysis Scope

This review focuses on the Solution Architecture and Implementation Approach sections per request. Evaluated against:
- Architectural clarity and completeness
- Interface definitions
- Phase sequencing
- Alternative approaches

---

## Question 1: Is the Architecture Clear Enough to Implement?

**Answer: Yes, with clarifications needed.**

### Strengths

1. **Component diagram is clear**: The three-stage pipeline (Extraction -> Closure -> Generation) maps directly to implementation functions.

2. **Interfaces are well-defined**: The ActionDeps struct is documented with all fields (lines 429-441):
   ```go
   type ActionDeps struct {
       InstallTime []string
       Runtime     []string
       EvalTime    []string
       LinuxInstallTime  []string
       DarwinInstallTime []string
       LinuxRuntime      []string
       DarwinRuntime     []string
   }
   ```

3. **Output format is specified**: The markdown table structure and mermaid graph format are clearly defined (lines 443-469).

4. **Data flow is explicit**: The 7-step data flow (lines 473-479) provides a clear algorithm.

### Clarifications Needed

1. **EvalTime dependency handling is ambiguous**. The design mentions EvalTime deps in the "Uncertainties" section but doesn't specify whether they should be included in the embedded list.

   **Recommendation**: Include EvalTime deps. The go_install action declares `EvalTime: []string{"go"}` because Decompose() runs `go get`. If go is missing at eval time, plan generation fails. This makes EvalTime deps critical for bootstrap.

2. **Recipe dependency field ambiguity**. The design shows using `recipe.Load()` for transitive closure but doesn't specify which recipe field provides the dependency list. Looking at resolver.go:396, it's `ResolveDependenciesForPlatform(depRecipe, targetOS)` which processes recipe steps, not a simple field.

   **Recommendation**: Add clarification that recipe-level transitive closure uses the same `ResolveDependencies()` function used for action dependency resolution, not a separate mechanism.

3. **Platform combination handling**. The design mentions outputting "all", "linux", or "darwin" but not how to handle recipes that are needed on BOTH Linux and Darwin but from different actions.

   **Recommendation**: If a recipe appears in both LinuxInstallTime AND DarwinInstallTime (or equivalent), mark it as "all" since it's needed everywhere.

---

## Question 2: Are There Missing Components or Interfaces?

**Answer: One component is underspecified.**

### Properly Defined Components

1. **Action Registry Access**: Uses existing `actions.Registry()` - verified in action.go:119-136
2. **Dependency Extraction**: Uses existing `action.Dependencies()` interface - verified in action.go:84
3. **Recipe Loading**: Uses existing `recipe.Load()` mechanism
4. **Transitive Resolution**: Uses existing `ResolveTransitive()` from resolver.go

### Missing Specification

**Recipe loading for transitive closure**: The design shows:
```go
// 4. Recipe loading: For each tool, load the recipe and extract its dependencies field
```

But doesn't specify HOW to load recipes. Options:
- Use embedded recipe FS directly (`embeddedRecipes` from embedded.go)
- Use the Loader interface with a RecipeLoader

**Recommendation**: Use embedded recipe FS directly to avoid circular dependency concerns. The tool should only analyze recipes that are candidates for embedding, which are currently in `internal/recipe/recipes/`.

### Interface Completeness

The required interfaces are complete:

| Interface | Source | Purpose | Status |
|-----------|--------|---------|--------|
| `Action` | action.go:71-85 | Get Dependencies() | Complete |
| `ActionDeps` | action.go:54-68 | Structured dep data | Complete |
| `Recipe` | recipe/recipe.go | Load recipe for transitives | Complete |
| `ResolvedDeps` | resolver.go:29-34 | Closure output | Complete |

---

## Question 3: Are Implementation Phases Correctly Sequenced?

**Answer: Yes, the sequencing is correct.**

### Phase Analysis

| Phase | Dependency | Status |
|-------|------------|--------|
| Step 1: Analysis tool | None | Correct |
| Step 2: Validation script | Step 1 | Correct |
| Step 3: CI workflow | Step 2 | Correct |
| Step 4: Initial generation | Steps 1-3 | Correct |

### Critical Path Verification

The design correctly identifies that this work (Stage 0) MUST complete before Stage 1 (Recipe Migration) of DESIGN-recipe-registry-separation.md. The dependency is:

```
DESIGN-embedded-recipe-list.md
    └── Generates EMBEDDED_RECIPES.md
        └── Used by issue #1033 (Migrate registry recipes)
```

This sequencing is correct because:
1. You cannot move recipes to registry until you know which must stay embedded
2. CI validation must exist before changes are made to prevent drift
3. Documentation (EMBEDDED_RECIPES.md) serves as the migration contract

### Potential Parallelization

Steps 2-3 could be developed in parallel:
- Validation script (Step 2) doesn't require CI integration to function
- CI workflow (Step 3) can be drafted while Step 2 is in progress

Not critical, but could save time.

---

## Question 4: Are There Simpler Alternatives We Overlooked?

**Answer: The chosen approach is appropriate. Two alternatives were considered but correctly rejected.**

### Alternative Considered: Static Analysis of Go Code

Instead of running Go code to extract Dependencies(), we could parse the Go source files with `go/ast`.

**Why correctly rejected:**
1. Would duplicate logic that already exists in compiled form
2. Would need to handle type resolution for interface implementations
3. More fragile - changes to source structure would break parsing
4. The existing infrastructure is proven and tested

### Alternative Considered: Manual Maintenance

Document the embedded list manually without generation.

**Why correctly rejected:**
1. Action dependencies change over time (new actions, new deps)
2. Transitive recipe dependencies are non-obvious
3. No CI validation means drift goes undetected
4. The 15-20 recipe estimate could be wrong

### Simpler Alternative Worth Considering: go generate

Instead of `scripts/analyze-deps/main.go`, use `go generate`:

```go
//go:generate go run ./scripts/analyze-deps/main.go > EMBEDDED_RECIPES.md
```

**Pros:**
- Standard Go pattern
- Single command: `go generate ./...`
- Discoverable via `grep -r go:generate`

**Cons:**
- Slightly less explicit than dedicated script
- `go generate` might run other generators

**Recommendation:** Consider this minor improvement. The current approach (dedicated script) is also valid.

### Alternative Not in Design: Test-Based Validation

Instead of regenerate-and-compare, use a Go test:

```go
func TestEmbeddedRecipesUpToDate(t *testing.T) {
    expected := generateEmbeddedRecipes()
    actual := readEmbeddedRecipesFile()
    if diff := cmp.Diff(expected, actual); diff != "" {
        t.Errorf("EMBEDDED_RECIPES.md is stale (-want +got):\n%s", diff)
    }
}
```

**Pros:**
- Runs as part of `go test ./...`
- Integrated with existing test infrastructure
- Better diff formatting with cmp.Diff

**Cons:**
- Would need to parse markdown to compare (or compare raw strings)
- Test file is slightly unconventional location for generation

**Verdict:** The design's approach (shell script regeneration) is equivalent in correctness. The test approach is marginally better for integration but not significantly so.

---

## Detailed Findings

### Finding 1: Stale TODO Comments

The design correctly identifies (lines 32-34) that TODO comments in `homebrew.go` and `homebrew_relocate.go` reference #644 but #644 is now closed.

**Evidence from homebrew.go:32-33:**
```go
// TODO(#644): Remove this method once composite actions automatically aggregate primitive dependencies.
// This is a workaround because dependency resolution happens before decomposition.
```

**But**: Issue #644 is CLOSED and the `aggregatePrimitiveDeps()` function in resolver.go (lines 73-86) now handles this:
```go
// Aggregate dependencies from primitive actions if this is a composite action
aggregatedDeps := aggregatePrimitiveDeps(step.Action, step.Params)
```

**Recommendation:** Include removal of these stale TODOs in the implementation scope. They are misleading.

### Finding 2: EvalTime Dependencies Are Critical

The design's uncertainty about EvalTime deps should be resolved. From go_install.go:21-25:
```go
func (GoInstallAction) Dependencies() ActionDeps {
    return ActionDeps{
        InstallTime: []string{"go"},
        EvalTime:    []string{"go"},
    }
}
```

EvalTime deps are needed for `tsuku eval` which generates installation plans. If go is missing at EvalTime, `go_install.Decompose()` fails because it runs `go get` (line 403).

**Recommendation:** Include EvalTime deps in the embedded recipe list. They are equally critical for bootstrap.

### Finding 3: Recipe Transitive Closure Uses Existing Infrastructure

The design mentions "Use existing ResolveTransitive for recipe-level closure" but doesn't detail how. Looking at resolver.go:288-294:

```go
func ResolveTransitive(
    ctx context.Context,
    loader RecipeLoader,
    deps ResolvedDeps,
    rootName string,
) (ResolvedDeps, error) {
    return ResolveTransitiveForPlatform(ctx, loader, deps, rootName, runtime.GOOS)
}
```

This function handles both action->recipe and recipe->recipe transitives.

**Recommendation:** The implementation should:
1. Collect direct action dependencies (action -> recipe)
2. For each recipe, call `ResolveDependenciesForPlatform()` to get its deps
3. Use `ResolveTransitive()` to compute the closure
4. Union results across all actions

### Finding 4: Platform Handling Edge Case

The resolver handles platform-specific deps via `getPlatformInstallDeps()` (resolver.go:183-192). The analysis tool needs to call both:
- `ResolveDependenciesForPlatform(r, "linux")`
- `ResolveDependenciesForPlatform(r, "darwin")`

And union the results with appropriate annotations.

**Recommendation:** Run the analysis for each target platform separately, then merge results with platform annotations.

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Missing transitive dep | Low | Medium | Test against known examples (ruby->libyaml) |
| Platform annotation wrong | Low | Low | Manual verification of generated list |
| CI validation too slow | Low | Low | Module cache keeps it under 5s |
| Markdown parsing fragile | Low | Medium | Use structured table format |

---

## Recommendations Summary

### Critical (Must Address)

1. **Clarify EvalTime dependency inclusion**: Add explicit statement that EvalTime deps ARE included in embedded list.

2. **Specify recipe loading mechanism**: Use embedded recipe FS directly to avoid loader complexity.

### Recommended (Should Address)

3. **Remove stale TODO comments**: Include cleanup of #644 TODOs in implementation scope.

4. **Document platform merging logic**: Specify that recipes appearing in both platform-specific lists are marked "all".

### Optional (Could Address)

5. **Consider go generate pattern**: Minor improvement for discoverability.

6. **Consider test-based validation**: Alternative to shell script with better integration.

---

## Conclusion

The design is well-structured and leverages existing, proven infrastructure. The chosen options (1D, 2A, 3A) represent the simplest correct solution. With the clarifications noted above, implementation can proceed with confidence.

**Estimated implementation effort**: 2-3 days for a developer familiar with the codebase.
