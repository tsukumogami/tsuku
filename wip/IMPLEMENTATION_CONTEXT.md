---
summary:
  constraints:
    - Step-level deps resolved ONLY if step.When matches target
    - Recipe-level deps still resolved for ALL matching steps
    - Dependencies are additive (step inherits recipe + own deps)
    - Deduplication happens after aggregation
  integration_points:
    - internal/recipe/types.go - Add Dependencies field to Step struct
    - internal/recipe/types.go - Update UnmarshalTOML for Step to parse deps
    - internal/recipe/types.go - Update Step.ToMap() for serialization
    - Dependency resolver (need to locate) - Modify to check step.When before resolving
  risks:
    - Circular dependency at step level needs detection
    - Must not break existing recipe-level dependency behavior
    - Plan generator changes must handle both levels correctly
  approach_notes: |
    The design doc Component 3 specifies adding Dependencies []string to Step struct.
    Key semantic: step deps are only resolved if step.When.Matches(target).
    This prevents "phantom dependencies" where musl plans show unresolved Homebrew deps.
    Follow existing TOML parsing patterns for arrays (same as OS, Platform).
---

# Implementation Context: Issue #1111

**Source**: docs/designs/DESIGN-platform-compatibility-verification.md

## Key Design Points

### Step-Level Dependencies (Component 3 from design)

```go
type Step struct {
    Action       string
    When         *WhenClause
    Note         string
    Description  string
    Params       map[string]interface{}
    Dependencies []string  // New: only resolved if this step matches target
}
```

### Dependency Resolution Logic

```go
func (g *PlanGenerator) resolveStepDependencies(step *Step, target *Target) ([]Plan, error) {
    // Only resolve dependencies if step matches target
    if step.When != nil && !step.When.Matches(target) {
        return nil, nil  // Step doesn't match, skip its dependencies
    }

    var depPlans []Plan
    for _, depName := range step.Dependencies {
        depPlan, err := g.generatePlan(depName, target)
        if err != nil {
            return nil, err
        }
        depPlans = append(depPlans, depPlan)
    }
    return depPlans, nil
}
```

### Precedence Rules (Component 6)

1. Recipe-level dependencies (in [metadata]) resolved for ALL matching steps
2. Step-level dependencies (in [[steps]]) resolved ONLY if that step matches target
3. Dependencies are additive - step inherits recipe-level deps plus its own
4. Deduplication happens after aggregation

### Example Recipe

```toml
[[steps]]
action = "homebrew"
formula = "curl"
when = { os = ["linux"], libc = ["glibc"] }
dependencies = ["openssl", "zlib"]  # Only resolved if this step matches

[[steps]]
action = "system_dependency"
name = "curl"
packages = { alpine = "curl-dev" }
when = { os = ["linux"], libc = ["musl"] }
# No dependencies - apk handles it
```

## Acceptance Criteria from Issue

- [ ] `Dependencies []string` field added to `Step` struct in `types.go`
- [ ] TOML parsing handles step-level `dependencies = ["openssl", "zlib"]`
- [ ] Dependency resolver only resolves step deps if step.When matches target
- [ ] Plan generator handles step-level deps in addition to recipe-level deps
- [ ] Precedence rules implemented (additive, deduplication)
- [ ] Unit tests cover step-level dependency resolution
- [ ] Unit tests verify deps skipped when step doesn't match
