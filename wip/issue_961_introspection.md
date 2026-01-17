# Issue 961 Introspection

## Staleness Signals

- Referenced file `internal/executor/constraints.go` was modified since issue creation
- Change: feat(eval): add GoVersion constraint for constrained evaluation (#993)

## Assessment

The recent change to `constraints.go` added `GoVersion` field and extraction logic for go_build steps. This is orthogonal to the pip hash issue.

The issue's root cause analysis remains accurate:
- `extractPipConstraintsFromSteps` (lines 74-91) still only extracts versions, not hashes
- `PipConstraints map[string]string` cannot store hashes
- The fix approach (add `PipRequirements string` field) follows the pattern used by GoSum, CargoLock, etc.

## Recommendation

**Proceed** - The issue specification is valid and the implementation approach is sound. The recent changes to constraints.go don't affect the pip hash preservation logic.
