---
summary:
  constraints:
    - Tests should not reference RecipeHash field (removed in #1585)
    - Content hash tests already exist in plan_cache_test.go (added in #1586)
    - Focus on removing stale RecipeHash references and adding portability test
  integration_points:
    - internal/executor/plan_test.go
    - internal/executor/plan_cache_test.go (already has content hash tests)
    - internal/executor/plan_generator_test.go
    - internal/executor/plan_conversion_test.go
    - internal/install/state_test.go
  risks:
    - Some RecipeHash references may already be removed in #1585/#1586
    - Need to verify what test updates are actually still needed
  approach_notes: |
    Issue #1586 already added comprehensive content hash tests. This issue focuses on:
    1. Cleaning up any remaining RecipeHash references in test files
    2. Adding the portability test (two different recipes -> identical plans -> same hash)
    3. Ensuring all tests pass with the new content-based caching

    Key insight: Most acceptance criteria may already be satisfied from #1585 and #1586.
    Need to audit current state and add only what's missing (likely just portability test).
---

# Implementation Context: Issue #1587

**Source**: docs/designs/DESIGN-plan-hash-removal.md (Phase 3: Test Updates)

## Design Excerpt

This issue covers Phase 3 of the plan hash removal design:

1. Update plan struct tests (`plan_test.go`)
2. Update cache validation tests (`plan_cache_test.go`)
3. Update plan generator tests (`plan_generator_test.go`)
4. Update plan conversion tests (`plan_conversion_test.go`)
5. Update state tests (`state_test.go`)
6. Update install deps tests (`install_deps_test.go`)
7. Add portability test: verify two different recipes producing identical plans have same content hash

## Key Test: Portability

The portability test demonstrates the key benefit of content-based hashing:

```go
func TestContentHashPortability(t *testing.T) {
    // Two plans with identical functional content but different metadata
    plan1 := &InstallationPlan{
        // ... generated from recipe A
        RecipeSource: "homebrew",
    }
    plan2 := &InstallationPlan{
        // ... identical steps, URLs, checksums
        RecipeSource: "/local/recipe.toml",
    }

    // Should produce identical content hashes
    hash1 := ComputePlanContentHash(plan1)
    hash2 := ComputePlanContentHash(plan2)
    if hash1 != hash2 {
        t.Error("Plans with identical content should have same hash")
    }
}
```
