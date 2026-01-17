---
status: Proposed
problem: Code validation fails for go_install recipes because decomposeWithConstraints() uses the installed Go version instead of the golden file's go_version parameter.
decision: Add a GoVersion field to EvalConstraints, extracted from go_build steps, and use it in decomposeWithConstraints().
rationale: This follows established constraint patterns exactly, requires minimal code changes (~15 lines), and provides explicit semantics for the source of truth.
---

# DESIGN: Go Version Constraint for Constrained Evaluation

## Status

Proposed

## Upstream Design Reference

This design extends [DESIGN-toolchain-dependency-pinning.md](./DESIGN-toolchain-dependency-pinning.md), which added `DependencyVersions` to `EvalConstraints` for pinning toolchain dependency versions. That design noted in the PR description:

> The go_version constraint gap: `decomposeWithConstraints` in go_install.go uses the installed Go version instead of the golden file version. Requires extracting go_version from go_build steps into constraints.

## Context and Problem Statement

The toolchain dependency pinning from #953/#960 successfully pins which Go recipe version to install as a dependency. For example, if a golden file was generated with `go@1.25.5`, constrained evaluation now correctly uses `go@1.25.5` instead of resolving to the latest `go@1.25.6`.

However, the `go_build` action step also captures a `go_version` parameter that records which Go binary was used during decomposition. This parameter serves two purposes:

1. **Code validation**: During `--pin-from` constrained evaluation, `go_install.decomposeWithConstraints()` should emit the same `go_version` as the golden file. Currently it uses `GetGoVersion(ResolveGo())` which returns the installed Go version, not the golden file's version.

2. **Execution validation**: During `install --plan`, `go_build.Execute()` looks for a Go binary matching the `go_version` parameter. If CI has Go 1.25.6 but the golden file says `go_version: "1.25.5"`, execution fails with:
   ```
   go 1.25.5 not found: install it first (tsuku install go@1.25.5)
   ```

**Example failure scenario:**

1. Golden file `dlv-v1.9.0.json` contains `go_version: "1.25.5"` in the `go_build` step
2. Go 1.25.6 is released; CI runners update
3. Code validation: `tsuku eval dlv@v1.9.0 --pin-from golden.json` generates `go_version: "1.25.6"` (mismatch)
4. Execution validation: `tsuku install --plan golden.json` fails looking for Go 1.25.5

**Current exclusions:** The code-validation-exclusions.json file lists 5 go_install recipes excluded due to this issue: cobra-cli, dlv, gofumpt, goimports, gopls.

### Scope

**In scope:**
- Adding a `GoVersion` field to `EvalConstraints` for pinning the `go_version` parameter
- Extracting `go_version` from `go_build` steps during constraint extraction
- Using the pinned `go_version` in `go_install.decomposeWithConstraints()`
- Documenting fallback behavior when pinned Go version is unavailable

**Out of scope:**
- Pinning other action-specific version parameters (python_version, node_version, etc.) - these can be added later following the same pattern
- Changes to non-constrained evaluation (normal `tsuku eval` without `--pin-from`)
- Changes to the execution validation workflow (`install --plan`)

### Assumptions

1. **Execution validation uses pinned Go dependency**: The `DependencyVersions` constraint ensures `go@1.25.5` is installed before `go_build` runs. The installed Go will match `go_version` in the golden file.

2. **go_version is consistent with dependencies[].version**: If the golden file has `go@1.25.5` in dependencies and `go_version: "1.25.5"` in go_build, they should match. We extract from go_build to be explicit, but there's no expected divergence.

3. **Version format**: The `go_version` parameter uses the same format as Go reports (e.g., "1.25.5" without "go" prefix or "v" prefix). This matches what `GetGoVersion()` returns.

4. **Go is always a dependency for go_install recipes**: The `GoInstallAction.Dependencies()` method always returns Go as a required dependency. A recipe using `go_build` without Go as a dependency would be a recipe bug.

## Decision Drivers

1. **Consistency with existing patterns**: Follow the same constraint extraction and application patterns used by `DependencyVersions`, `GoSum`, and other constraints.

2. **Minimal changes**: Add one field to `EvalConstraints` and modify one code path in `go_install.decomposeWithConstraints()`.

3. **Complete code validation fix**: After this change, the 5 excluded go_install recipes should pass code validation.

4. **Execution validation unchanged**: The execution side already works correctly when `DependencyVersions` is applied (Go 1.25.5 is installed as a dependency, then go_build finds it).

5. **No format changes**: Both recipe and golden file formats remain unchanged.

## Implementation Context

### Existing Patterns

**Constraint extraction** (`internal/executor/constraints.go`):

The `ExtractConstraintsFromPlan()` function extracts constraints from golden files by iterating through steps:

```go
// For go.sum (similar pattern for cargo, npm, gem, cpan)
func extractGoConstraintsFromSteps(steps []ResolvedStep, constraints *actions.EvalConstraints) {
    for _, step := range steps {
        if step.Action != "go_build" {
            continue
        }
        goSum, ok := step.Params["go_sum"].(string)
        if !ok || goSum == "" {
            continue
        }
        if constraints.GoSum == "" {
            constraints.GoSum = goSum  // First one wins
        }
    }
}
```

**Constraint application** (`internal/actions/go_install.go:451-500`):

The `decomposeWithConstraints()` method checks for constraints and uses them:

```go
func (a *GoInstallAction) decomposeWithConstraints(ctx *EvalContext, ...) ([]Step, error) {
    // Use the constrained go.sum
    goSum := ctx.Constraints.GoSum

    // BUG: Gets Go version from installed Go, not from constraints
    goPath := ResolveGo()
    goVersion := ""
    if goPath != "" {
        goVersion = GetGoVersion(goPath)  // Should check constraints first
    }
    ...
}
```

### Conventions to Follow

1. **First-encountered-wins**: When extracting from multiple steps, the first value wins
2. **Nil-safe checks**: Always check `constraints != nil` before accessing fields
3. **Helper functions**: Provide `Has*Constraint()` and `Get*Constraint()` functions
4. **Fallback behavior**: If constraint unavailable, fall back to runtime value with warning

### Anti-patterns to Avoid

1. **Hard failures**: Don't fail if a constrained value is unavailable
2. **Network calls**: Don't make network calls during constrained evaluation
3. **Modifying golden file format**: Keep changes to Go code only

## Considered Options

### Option 1: Add GoVersion Field to EvalConstraints

Add a `GoVersion string` field to `EvalConstraints`, extracted from `go_build` steps alongside `GoSum`. The `decomposeWithConstraints()` method checks this field first before falling back to the installed Go version.

**How it works:**

1. `extractGoConstraintsFromSteps()` extracts both `go_sum` and `go_version` from `go_build` steps
2. `EvalConstraints.GoVersion` stores the pinned version
3. `decomposeWithConstraints()` uses `ctx.Constraints.GoVersion` if available
4. Falls back to `GetGoVersion(ResolveGo())` if no constraint

**Pros:**
- Follows existing patterns exactly (mirrors `GoSum` extraction)
- Single field addition to `EvalConstraints`
- Clear separation: `GoSum` for dependency resolution, `GoVersion` for toolchain version
- Minimal code changes (~15 lines)

**Cons:**
- Adds another field to `EvalConstraints` (currently has 7 fields)
- Redundant with `DependencyVersions["go"]` in most cases

**Enhancement**: During extraction, log a warning if `GoVersion` differs from `DependencyVersions["go"]` to detect potential golden file corruption.

### Option 2: Use DependencyVersions["go"] Instead of New Field

Instead of adding a new field, derive the Go version from the existing `DependencyVersions["go"]` constraint.

**How it works:**

1. `DependencyVersions` already contains `"go": "1.25.5"` from dependency extraction
2. `decomposeWithConstraints()` checks `ctx.Constraints.DependencyVersions["go"]`
3. Uses that version if available, falls back to installed Go version

**Pros:**
- No new fields in `EvalConstraints`
- Uses existing data that's already extracted
- Conceptually simpler: "pin Go version" means "use the Go dependency version"

**Cons:**
- Conflates two different concepts: which Go recipe to install vs which Go binary was used
- `DependencyVersions["go"]` is the recipe version (e.g., "1.25.5"), while `go_version` in go_build is the binary version - these should always match but are semantically different
- Breaks if a recipe doesn't have Go as a dependency but uses go_build (edge case, but possible)
- Less explicit about where the constraint comes from

### Option 3: Extract go_version During Decomposition from Golden File Steps

Instead of storing `GoVersion` as a constraint, read the golden file's `go_build` step directly during constrained decomposition.

**How it works:**

1. `decomposeWithConstraints()` receives the golden file path or parsed plan
2. It reads the `go_build` step from the golden file
3. Extracts `go_version` directly and uses it

**Pros:**
- No changes to `EvalConstraints` structure
- Always gets the exact value from the golden file

**Cons:**
- Changes the `decomposeWithConstraints()` signature (needs access to golden file)
- Doesn't follow the established constraint extraction pattern
- More complex plumbing to pass golden file data to decomposition
- Violates separation of concerns: decomposition shouldn't know about golden files

### Evaluation Against Decision Drivers

| Option | Consistency with patterns | Minimal changes | Complete fix | No format changes |
|--------|--------------------------|-----------------|--------------|-------------------|
| Option 1: GoVersion field | Excellent | Good (1 field, ~15 lines) | Yes | Yes |
| Option 2: Use DependencyVersions | Fair (different semantic) | Best (0 fields) | Yes* | Yes |
| Option 3: Direct extraction | Poor (breaks pattern) | Poor (signature changes) | Yes | Yes |

*Option 2 works but relies on Go being in dependencies, which is true for go_install recipes.

### Uncertainties

- **Edge case**: Could a recipe use `go_build` without having Go as a dependency? Theoretically yes (if Go is assumed to be on PATH), but no current recipes do this. Additionally, `GoInstallAction.Dependencies()` always returns Go, so this would be a recipe bug.
- **Divergence**: Could `go_version` in go_build ever differ from `dependencies[].version` for Go? In practice no, since go_install runs Go to generate go.sum, but the data model allows it.

## Decision Outcome

**Chosen option: Option 1 - Add GoVersion Field to EvalConstraints**

This option best addresses the decision drivers by following established patterns exactly while requiring minimal code changes (~15 lines total).

### Rationale

This option was chosen because:

1. **Consistency with existing patterns**: Mirrors how `GoSum`, `CargoLock`, and other constraints are extracted and applied. Developers familiar with the codebase will immediately understand the implementation.

2. **Explicit semantics**: Clearly separates "which Go binary version was used" (`GoVersion`) from "which Go recipe version to install" (`DependencyVersions["go"]`). Even though they should match, extracting from `go_build` is explicit about the source of truth for this specific parameter.

3. **Minimal changes**: Single field addition plus ~10-15 lines of extraction/application code. No signature changes, no format changes.

4. **Complete fix**: Directly addresses the 5 excluded recipes with no edge cases.

Alternatives were rejected because:

- **Option 2 (Use DependencyVersions)**: Conflates two semantically different concepts. While it would work for current recipes, it establishes a precedent of deriving action parameters from dependency metadata, which could cause subtle bugs if assumptions change.

- **Option 3 (Direct extraction)**: Violates the established separation between constraint extraction (in `constraints.go`) and constraint application (in decompose methods). Would require significant plumbing changes.

### Trade-offs Accepted

By choosing this option, we accept:

1. **One more field in EvalConstraints**: The struct grows from 7 to 8 fields. This is acceptable because:
   - The field is conceptually distinct from existing fields
   - The struct is designed to grow as more constraints are needed
   - Clarity and explicitness are worth a small increase in struct size

2. **Redundancy with DependencyVersions["go"]**: Both values should always match for valid golden files. This is acceptable because:
   - Redundancy enables consistency validation (warn if they differ)
   - Explicit extraction is more maintainable than implicit derivation

## Solution Architecture

### Overview

Add a `GoVersion` field to `EvalConstraints` that stores the `go_version` parameter from `go_build` steps. During constrained evaluation, `go_install.decomposeWithConstraints()` uses this value instead of querying the installed Go binary.

### Data Flow

```
Golden File (go_build step with go_version param)
         │
         ▼
ExtractConstraintsFromPlan()
         │
         ├─ extractGoConstraintsFromSteps()
         │         │
         │         ├─ GoSum (existing)
         │         └─ GoVersion (NEW)
         │
         ▼
   EvalConstraints
         │
         ▼
   PlanConfig.Constraints
         │
         ▼
go_install.decomposeWithConstraints()
         │
    ┌────┴────┐
    │         │
GoVersion?  No constraint
    │         │
    ▼         ▼
 Use        Use GetGoVersion(ResolveGo())
 pinned     (installed Go version)
```

### Key Changes

**1. EvalConstraints struct** (`internal/actions/decomposable.go`):

```go
type EvalConstraints struct {
    PipConstraints     map[string]string
    GoSum              string
    GoVersion          string  // NEW: Go binary version from go_build
    CargoLock          string
    NpmLock            string
    GemLock            string
    CpanMeta           string
    DependencyVersions map[string]string
}
```

**2. Constraint extraction** (`internal/executor/constraints.go`):

```go
func extractGoConstraintsFromSteps(steps []ResolvedStep, constraints *actions.EvalConstraints) {
    for _, step := range steps {
        if step.Action != "go_build" {
            continue
        }

        // Extract go_sum (existing)
        if goSum, ok := step.Params["go_sum"].(string); ok && goSum != "" {
            if constraints.GoSum == "" {
                constraints.GoSum = goSum
            }
        }

        // Extract go_version (NEW)
        if goVersion, ok := step.Params["go_version"].(string); ok && goVersion != "" {
            if constraints.GoVersion == "" {
                constraints.GoVersion = goVersion
            }
        }
    }
}
```

**3. Constraint application** (`internal/actions/go_install.go`):

```go
func (a *GoInstallAction) decomposeWithConstraints(ctx *EvalContext, ...) ([]Step, error) {
    goSum := ctx.Constraints.GoSum

    // Use constrained go_version if available (NEW)
    goVersion := ""
    if ctx.Constraints.GoVersion != "" {
        goVersion = ctx.Constraints.GoVersion
    } else {
        // Fall back to installed Go version
        goPath := ResolveGo()
        if goPath != "" {
            goVersion = GetGoVersion(goPath)
        }
    }

    // ... rest of method unchanged
}
```

**4. Helper functions** (`internal/executor/constraints.go`):

```go
// HasGoVersionConstraint returns true if the constraints contain a go_version.
func HasGoVersionConstraint(constraints *actions.EvalConstraints) bool {
    return constraints != nil && constraints.GoVersion != ""
}

// GetGoVersionConstraint returns the constrained go_version, if any.
func GetGoVersionConstraint(constraints *actions.EvalConstraints) (string, bool) {
    if constraints == nil || constraints.GoVersion == "" {
        return "", false
    }
    return constraints.GoVersion, true
}
```

## Implementation Approach

### Phase 1: Struct and Extraction

Add the field and extraction logic:

1. Add `GoVersion string` field to `EvalConstraints` in `internal/actions/decomposable.go`
2. Modify `extractGoConstraintsFromSteps()` in `internal/executor/constraints.go` to extract `go_version`
3. Add `HasGoVersionConstraint()` and `GetGoVersionConstraint()` helper functions
4. Add unit tests for extraction

**Files to modify:**
- `internal/actions/decomposable.go` (1 line)
- `internal/executor/constraints.go` (~15 lines)
- `internal/executor/constraints_test.go` (new test cases)

### Phase 2: Application

Use the constraint during decomposition:

1. Modify `decomposeWithConstraints()` in `internal/actions/go_install.go` to check `ctx.Constraints.GoVersion`
2. Fall back to `GetGoVersion(ResolveGo())` if no constraint
3. Add integration test for constrained evaluation with go_version

**Files to modify:**
- `internal/actions/go_install.go` (~8 lines)
- `internal/actions/go_install_test.go` (new test case)

### Phase 3: Validation and Cleanup

Enable previously excluded recipes:

1. Remove cobra-cli, dlv, gofumpt, goimports, gopls from `code-validation-exclusions.json`
2. Run code validation to verify fix
3. Update documentation if needed

**Files to modify:**
- `testdata/golden/code-validation-exclusions.json`

## Consequences

### Positive

- **CI stability**: The 5 excluded go_install recipes can be re-enabled for code validation
- **Pattern consistency**: Implementation follows established constraint patterns
- **Explicit semantics**: Clear separation between recipe dependency version and binary version
- **Minimal footprint**: ~25 lines of code total, well-localized changes

### Negative

- **Struct growth**: `EvalConstraints` grows by one field
- **Redundancy**: `GoVersion` will typically match `DependencyVersions["go"]`

### Mitigations

- **Struct growth**: The struct is designed for extensibility; one additional field is negligible
- **Redundancy**: Consider adding a consistency warning if values differ (enhancement, not required for initial implementation)

## Security Considerations

### Download Verification

**Not affected.** This feature operates entirely within the constraint extraction and plan generation phases. No downloads occur during these phases. The `go_version` field is extracted from an already-parsed golden file (which was previously validated) and used to set a parameter in the generated plan.

Download verification happens during plan execution (in `go_build.Execute()`), which is unchanged by this design. The existing checksum verification for Go binary downloads remains in place.

### Execution Isolation

**Not affected.** This feature does not change execution permissions or isolation:

- No new file system access: Only reads from the already-parsed golden file structure
- No network access: Constraint extraction is purely in-memory parsing
- No privilege changes: Plan generation runs with the same privileges as before

The `go_version` parameter affects which Go binary is used during execution, but the execution isolation model is unchanged - it still uses tsuku-managed Go installations in `$TSUKU_HOME/tools/`.

### Supply Chain Risks

**Minimal impact, mitigated by existing controls.**

The `go_version` parameter specifies which Go binary version to use. A compromised golden file could theoretically specify an old Go version with known vulnerabilities.

**Mitigations:**

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious go_version in golden file | Golden files are committed to version control and reviewed via PR | Attacker with commit access could inject bad values |
| Old Go version with CVEs | go_version should match DependencyVersions["go"], which is pinned to the Go recipe version in the golden file's dependency tree | If the Go dependency version is also compromised, the attack succeeds |
| Version downgrade attack | Version is extracted from a trusted golden file, not from user input or network | An attacker would need to compromise the golden file first |

**Residual risk**: This feature trusts golden file content. Golden file integrity is protected by version control and code review, which are existing trust boundaries for the project.

### User Data Exposure

**Not applicable.** This feature:

- Does not access any user data beyond what's already available in the golden file
- Does not transmit any data externally
- Does not persist any new data
- Does not change logging behavior

The `go_version` value is extracted from a golden file (which is committed to the repository and contains no user-specific data) and used only for plan generation.
