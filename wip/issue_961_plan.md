# Implementation Plan: Issue #961

**Title**: fix(pipx): preserve hashes in pip constraint pinning
**Type**: Bug fix
**Branch**: fix/961-preserve-pip-hashes

## Problem Statement

When using constrained evaluation (`--pin-from`) with pipx_install recipes, the locked_requirements hashes are not preserved. The constraint extraction only stores package versions, not the full locked_requirements string including hashes.

**Expected**: `--hash=sha256:8fb482cf11667cd271a900cef5e58648c4511f3e25902cf1d35b4294c6964c99`
**Actual**: `--hash=sha256:0`

**Impact**: All pipx_install recipes fail golden file code validation even with dependency pinning enabled. Affects: black, httpie, meson, poetry, ruff.

## Root Cause Analysis

The bug is in the constraint extraction and reconstruction flow:

1. **Extraction** (`constraints.go:extractPipConstraintsFromSteps`): When extracting constraints from a golden file, `ParsePipRequirements` is called which only extracts `package==version` pairs. The hashes are discarded.

2. **Reconstruction** (`pipx_install.go:decomposeWithConstraints`): When reconstructing `locked_requirements` from constraints, `generateLockedRequirementsFromConstraints` generates placeholder hashes (`--hash=sha256:0`) because the real hashes weren't stored.

The current pip constraint system differs from other ecosystems:
- **Go/Cargo/npm/gem/cpan**: Store full lockfile content as a single string
- **pip**: Parses content into a `map[string]string` of versions, losing hashes

## Solution Design

### Approach: Follow Existing Pattern (Store Full String)

Add a `PipRequirements string` field to `EvalConstraints` to store the complete `locked_requirements` string, matching the pattern used by Go (`GoSum`), Cargo (`CargoLock`), npm (`NpmLock`), gem (`GemLock`), and CPAN (`CpanMeta`).

**Why this approach:**
1. Consistent with how all other ecosystems handle lockfile constraints
2. Minimal code changes - follows established patterns
3. Preserves all data (hashes, comments, ordering) without custom parsing
4. Simple to implement and test

**Alternative considered but rejected:**
- Store hashes alongside versions in a `map[string]PackageConstraint` struct
- Rejected because: More complex, doesn't match existing patterns, and the `PipConstraints map[string]string` is still needed for version lookups in other code paths

### Implementation Details

#### 1. Add `PipRequirements` field to `EvalConstraints` struct

**File**: `internal/actions/decomposable.go`

```go
type EvalConstraints struct {
    // PipConstraints maps package names to pinned versions.
    // Extracted from locked_requirements in pip_exec steps.
    // Keys are normalized package names (lowercase, hyphens).
    // Used for version lookups by GetPipConstraint.
    PipConstraints map[string]string

    // PipRequirements contains the full locked_requirements string.
    // Extracted from pip_exec steps in golden files.
    // Used during constrained evaluation to preserve hashes.
    PipRequirements string

    // ... existing fields ...
}
```

#### 2. Store full `locked_requirements` during extraction

**File**: `internal/executor/constraints.go`

Modify `extractPipConstraintsFromSteps` to also store the full requirements string:

```go
func extractPipConstraintsFromSteps(steps []ResolvedStep, constraints *actions.EvalConstraints) {
    for _, step := range steps {
        if step.Action != "pip_exec" {
            continue
        }

        lockedReqs, ok := step.Params["locked_requirements"].(string)
        if !ok || lockedReqs == "" {
            continue
        }

        // Store full requirements string (first one wins)
        if constraints.PipRequirements == "" {
            constraints.PipRequirements = lockedReqs
        }

        // Also parse and store versions for lookup
        parsed := ParsePipRequirements(lockedReqs)
        for pkg, ver := range parsed {
            constraints.PipConstraints[pkg] = ver
        }
    }
}
```

#### 3. Add helper function for checking PipRequirements

**File**: `internal/executor/constraints.go`

```go
// HasPipRequirementsConstraint returns true if the constraints contain pip requirements.
func HasPipRequirementsConstraint(constraints *actions.EvalConstraints) bool {
    return constraints != nil && constraints.PipRequirements != ""
}
```

#### 4. Update `decomposeWithConstraints` to use stored requirements

**File**: `internal/actions/pipx_install.go`

Modify `decomposeWithConstraints` to use the stored `PipRequirements` string directly instead of reconstructing from the version map:

```go
func (a *PipxInstallAction) decomposeWithConstraints(ctx *EvalContext, packageName, version string, executables []string) ([]Step, error) {
    // Use the full locked_requirements string from constraints
    lockedRequirements := ctx.Constraints.PipRequirements

    // Detect native addons based on the stored requirements
    hasNativeAddons := detectPythonNativeAddons(lockedRequirements)

    // ... rest of method unchanged ...
}
```

#### 5. Remove or deprecate `generateLockedRequirementsFromConstraints`

The function `generateLockedRequirementsFromConstraints` that generates placeholder hashes will no longer be needed for this code path. It can either be:
- Removed entirely if no other callers exist
- Kept but documented as deprecated

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/actions/decomposable.go` | Modify | Add `PipRequirements string` field to `EvalConstraints` |
| `internal/executor/constraints.go` | Modify | Store full requirements in `extractPipConstraintsFromSteps`, add `HasPipRequirementsConstraint` |
| `internal/actions/pipx_install.go` | Modify | Use `ctx.Constraints.PipRequirements` instead of reconstructing |
| `internal/executor/constraints_test.go` | Modify | Add tests for `PipRequirements` extraction and helpers |

## Testing Strategy

### Unit Tests

1. **Constraint extraction with hashes** - Verify `PipRequirements` contains complete string including hashes
2. **Constraint extraction from dependencies** - Verify extraction works for nested dependency plans
3. **First-wins semantics** - Verify first `pip_exec` step's requirements are stored
4. **HasPipRequirementsConstraint helper** - Test nil, empty, and populated cases

### Integration Test

Run the code-validation workflow to verify pipx recipes pass:

```bash
# Build and validate affected recipes
go build -o tsuku ./cmd/tsuku
./tsuku validate-plan black --pin-from testdata/golden/plans/b/black/v26.1a1-linux-amd64.json
```

### Golden File Validation

Verify all affected recipes now pass:
- black
- httpie
- meson
- poetry
- ruff

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Existing callers of `PipConstraints` break | Low | Medium | Keep `PipConstraints` map intact, only add `PipRequirements` |
| Multiple pip_exec steps with different requirements | Low | Low | First-wins semantics (consistent with other ecosystems) |
| Golden file format changes | None | N/A | Only internal constraint handling changes, golden file format unchanged |

## Acceptance Criteria

1. `EvalConstraints.PipRequirements` contains the full `locked_requirements` string from golden files
2. `pipx_install.decomposeWithConstraints` uses the stored requirements directly
3. Constrained evaluation preserves SHA256 hashes in the output
4. All existing tests pass
5. New tests cover `PipRequirements` extraction and usage
6. Golden file validation passes for black, httpie, meson, poetry, ruff

## Implementation Order

1. Add `PipRequirements` field to `EvalConstraints` struct
2. Modify `extractPipConstraintsFromSteps` to store full string
3. Add `HasPipRequirementsConstraint` helper function
4. Update `decomposeWithConstraints` to use stored requirements
5. Add unit tests for new behavior
6. Run `go test ./...` and verify all tests pass
7. Run `go vet ./...` and `golangci-lint run --timeout=5m ./...`
8. Verify golden file validation for affected recipes
