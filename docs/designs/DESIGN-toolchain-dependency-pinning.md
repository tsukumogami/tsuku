# DESIGN: Toolchain Dependency Pinning for Constrained Evaluation

## Status

Accepted

## Upstream Design Reference

This design extends [DESIGN-non-deterministic-validation.md](./DESIGN-non-deterministic-validation.md), which acknowledges this limitation in the Negative Tradeoffs section:

> Toolchain version sensitivity: If Go/Python/Rust toolchain versions change, constraints may not apply identically

And suggests future work in the Mitigations section:

> Toolchain versions: Golden files can include toolchain version metadata; validation can warn on mismatch

## Context and Problem Statement

The constrained evaluation feature (#921-#927) successfully pins lockfile content (pip requirements, go.sum, Cargo.lock, etc.) during golden file validation. However, it does NOT pin recipe dependencies (toolchains like python-standalone, go, nodejs).

This causes golden file validation failures when toolchain versions change upstream:
- python-standalone 20260113 -> 20260114
- go 1.25.5 -> 1.25.6
- nodejs 24.12.0 -> 24.13.0

When a toolchain version changes, the generated plan differs from the golden file even though the recipe and tsuku code are unchanged. This defeats the purpose of constrained evaluation: catching regressions in eval code while tolerating expected drift.

**Example failure scenario:**

1. Golden file `black-26.1a1.json` was generated with `python-standalone@20260113`
2. `python-standalone` releases version `20260114`
3. CI runs `tsuku eval black@26.1a1 --pin-from golden.json`
4. Pip constraints are applied correctly (lockfile pinned)
5. But dependency resolution picks `python-standalone@20260114` (latest)
6. Golden file comparison fails because dependency versions differ

**Current state:** The validate-golden-code.yml workflow is disabled due to these failures. The constrained evaluation infrastructure from #921-#927 handles lockfile pinning but not toolchain pinning.

### Scope

**In scope:**
- Extracting toolchain dependency versions from golden files (including nested dependencies in the dependency tree)
- Pinning toolchain versions during `--pin-from` constrained evaluation
- Handling version mismatch warnings when pinned versions are unavailable
- First-encountered-wins semantics for version conflicts (depth-first traversal)

**Out of scope:**
- Pinning system dependencies (git, curl, tar, cmake)
- Pinning compiler versions for native builds (zig-cc, clang)
- Changes to the `--pin-from` CLI interface (reuse existing flag)

### Assumptions

1. **Toolchain versions remain available**: Version providers keep historical versions available. If providers aggressively prune old versions, fallback will become the common path rather than the exception.
2. **Single version per toolchain is sufficient**: Each toolchain (python-standalone, go, nodejs) needs only one pinned version. Diamond dependencies with different toolchain versions are not expected in the current recipe set.
3. **Golden file format stability**: The `dependencies[]` array structure remains stable. Future format changes would require updating extraction logic.

## Decision Drivers

1. **Complementary to existing infrastructure**: Solution should extend the existing `EvalConstraints` and `--pin-from` mechanism, not introduce parallel systems
2. **Minimal disruption**: Recipe format and golden file format should require minimal changes
3. **Graceful degradation**: If a pinned version is unavailable, warn but continue with available version
4. **Deterministic output**: Given the same recipe and constraints, eval should produce identical plans
5. **No network calls during constrained eval**: Constrained evaluation should not depend on remote version providers

## Implementation Context

### Existing Patterns

**Constraint application in decompose methods** (`internal/actions/pipx_install.go:311-312`):
```go
if ctx.Constraints != nil && len(ctx.Constraints.PipConstraints) > 0 {
    return a.decomposeWithConstraints(ctx, ...)
}
```

This pattern is consistent across `go_install.go`, `cargo_install.go`, and `npm_install.go`. Toolchain pinning should follow the same conditional pattern.

**Dependency resolution in plan generation** (`internal/executor/plan_generator.go`):
```go
func generateSingleDependencyPlan(depName string, cfg PlanConfig, ...) {
    depVersion := resolvedDeps.InstallTime[depName]  // Currently "latest" or from recipe
    exec, err := executor.NewWithVersion(depRecipe, depVersion)
    ...
}
```

The version comes from `resolvedDeps.InstallTime[depName]`, which is populated by `ResolveDependencies()`. This is where version constraints should be injected.

**Constraint extraction** (`internal/executor/constraints.go`):
```go
type EvalConstraints struct {
    PipConstraints map[string]string
    GoSum          string
    CargoLock      string
    NpmLock        string
    GemLock        string
    CpanMeta       string
}
```

A new field `DependencyVersions map[string]string` would fit naturally alongside these ecosystem-specific constraints.

### Conventions to Follow

- Constraint checking pattern: `if ctx.Constraints != nil && ctx.Constraints.Field != nil`
- Logging with structured fields: `ctx.Logger.Debug("using pinned version", "tool", depName, "version", pinnedVersion)`
- Graceful fallback with warnings, not errors

### Anti-patterns to Avoid

- Hard failures on version mismatch (toolchain versions change frequently)
- Modifying recipe struct for constraint data (constraints are eval-time, not authoring-time)
- Network calls during constrained evaluation (defeats determinism)

## Considered Options

### Option 1: Extend EvalConstraints with DependencyVersions Map

Add a `DependencyVersions map[string]string` field to `EvalConstraints` that maps dependency tool names to pinned versions.

**How it works:**
1. During constraint extraction, parse `dependencies[]` from golden file
2. Build `DependencyVersions` map from `{dep.Tool: dep.Version}` for all dependencies (recursively)
3. In `generateSingleDependencyPlan()`, check `cfg.Constraints.DependencyVersions[depName]`
4. If found, use pinned version instead of resolved "latest"
5. If pinned version unavailable, warn and fall back to available version

**Pros:**
- Natural extension of existing `EvalConstraints` struct
- Follows established constraint application patterns
- No changes to golden file format (already contains dependency versions)
- Minimal code changes (extraction + one conditional in plan_generator)

**Cons:**
- Recursive extraction needed for nested dependencies
- Must handle case where pinned version is no longer published
- Version conflict resolution needed when different dependencies use different toolchain versions (solved with first-encountered-wins semantics)

### Option 2: Recipe-Level Version Locks

Add an optional `[dependencies.locks]` section to recipes that pins toolchain versions.

**How it works:**
1. Recipe author adds `[dependencies.locks]` with explicit versions
2. During eval, locked versions override version resolution
3. Golden files become deterministic as a side effect

**Pros:**
- Makes version locking explicit in the recipe
- No runtime constraint extraction needed
- Clear authoring intent

**Cons:**
- Significant recipe format change
- Authoring burden on recipe maintainers
- Must keep locks updated when toolchains change
- Doesn't help with existing golden files

### Option 3: Version Provider Constraint Mode

Modify version providers to accept constraints and return constrained results.

**How it works:**
1. Pass constraints to `version.Resolver` during plan generation
2. Version providers check constraints before querying registries
3. Constrained provider returns exact version if specified

**Pros:**
- Integrates with existing version resolution architecture
- Could support partial constraints (pin some, resolve others)

**Cons:**
- Requires changes across all version providers (GitHub, PyPI, crates.io, npm, etc.)
- More invasive changes to version resolution subsystem
- Overkill for the specific problem of toolchain pinning

### Option 4: Plan-Level Dependency Snapshot

Store a complete snapshot of resolved dependency versions in the plan metadata.

**How it works:**
1. Golden files include `resolved_dependencies: {"python-standalone": "20260113", ...}`
2. During `--pin-from`, restore this snapshot to `PlanConfig`
3. Plan generator uses snapshot versions for all dependencies

**Pros:**
- Explicit snapshot makes intent clear
- Could include additional metadata (hashes, sources)

**Cons:**
- Changes plan format (new top-level field)
- Requires regenerating all golden files
- Redundant with existing `dependencies[]` array which already has versions

## Decision Outcome

**Chosen: Option 1 - Extend EvalConstraints with DependencyVersions Map**

### Summary

Add a `DependencyVersions map[string]string` field to the existing `EvalConstraints` struct. During constraint extraction from golden files, recursively collect all dependency tool names and versions. During plan generation, check this map before resolving dependency versions.

### Rationale

Option 1 is the natural evolution of the existing constrained evaluation architecture:

1. **Minimal changes**: Only adds one field to `EvalConstraints` and one conditional in `generateSingleDependencyPlan()`. No format changes, no new files, no provider changes.

2. **Data already exists**: Golden files already contain `dependencies[].tool` and `dependencies[].version`. We're extracting what's there, not adding new data.

3. **Consistent patterns**: Follows the same `if ctx.Constraints != nil` pattern used by pip, go, cargo, and npm constraint application.

4. **Graceful degradation**: If a pinned version is unavailable (removed from registry), we can warn and fall back to the latest available version, preserving CI stability.

### Why Not Other Options

- **Option 2 (Recipe locks)**: High authoring burden, requires format changes, doesn't help existing golden files.
- **Option 3 (Provider constraints)**: Too invasive for this specific problem. Version providers shouldn't need to know about golden file constraints.
- **Option 4 (Plan snapshot)**: Redundant with existing `dependencies[]` structure. Adding a parallel data structure creates maintenance burden.

## Solution Architecture

### Data Flow

```
Golden File (contains dependencies[])
         │
         ▼
ExtractConstraints() ─────────────────────────────────────┐
         │                                                │
         ├─ PipConstraints (existing)                     │
         ├─ GoSum (existing)                              │
         ├─ CargoLock (existing)                          │
         └─ DependencyVersions (NEW) ◄────────────────────┘
                  │                    Extracted from:
                  │                    dep.Tool → dep.Version
                  ▼
         PlanConfig.Constraints
                  │
                  ▼
    generateSingleDependencyPlan()
                  │
         ┌───────┴───────┐
         │               │
    Constraint?     No Constraint
         │               │
         ▼               ▼
   Use pinned      Resolve "latest"
     version         from recipe
```

### Constraint Extraction

Extend `ExtractConstraintsFromPlan()` to collect dependency versions:

```go
// EvalConstraints - add new field
type EvalConstraints struct {
    PipConstraints     map[string]string
    GoSum              string
    CargoLock          string
    NpmLock            string
    GemLock            string
    CpanMeta           string
    DependencyVersions map[string]string // NEW: tool name -> version
}

// Extract dependency versions recursively using depth-first traversal.
// First-encountered version wins in case of conflicts (preserves original generation order).
func extractDependencyVersions(plan *InstallationPlan, versions map[string]string) {
    for _, dep := range plan.Dependencies {
        // First-encountered wins: only set if not already present
        if _, exists := versions[dep.Tool]; !exists {
            versions[dep.Tool] = dep.Version
        }
        // Recursively extract from nested dependencies
        extractDependencyVersionsFromDep(&dep, versions)
    }
}

func extractDependencyVersionsFromDep(dep *DependencyPlan, versions map[string]string) {
    for _, nested := range dep.Dependencies {
        // First-encountered wins: only set if not already present
        if _, exists := versions[nested.Tool]; !exists {
            versions[nested.Tool] = nested.Version
        }
        extractDependencyVersionsFromDep(&nested, versions)
    }
}
```

### Version Pinning in Plan Generation

Modify `generateSingleDependencyPlan()` to use constraints:

```go
func (e *Executor) generateSingleDependencyPlan(
    depName string,
    cfg PlanConfig,
    processed map[string]bool,
) (*DependencyPlan, error) {
    // Check for pinned version from constraints
    var depVersion string
    if cfg.Constraints != nil && cfg.Constraints.DependencyVersions != nil {
        if pinnedVersion, ok := cfg.Constraints.DependencyVersions[depName]; ok {
            depVersion = pinnedVersion
            log.Debug("using pinned dependency version",
                "tool", depName,
                "version", pinnedVersion)
        }
    }

    // Fall back to recipe-specified or latest version
    if depVersion == "" {
        depVersion = resolvedDeps.InstallTime[depName]
    }

    // Create executor with specific version
    depExec, err := NewWithVersion(depRecipe, depVersion)
    if err != nil {
        // Handle unavailable version
        if errors.Is(err, ErrVersionNotFound) && cfg.Constraints != nil {
            log.Warn("pinned dependency version unavailable, using latest",
                "tool", depName,
                "pinned", depVersion)
            depExec, err = New(depRecipe) // Fall back to latest
        }
        if err != nil {
            return nil, err
        }
    }

    // IMPORTANT: Propagate constraints to nested dependency resolution
    depCfg := PlanConfig{
        RecipeLoader: cfg.RecipeLoader,
        Constraints:  cfg.Constraints,  // Pass constraints through
        // ... other fields
    }

    // Continue with plan generation using depCfg...
}
```

**Critical implementation detail:** The `cfg.Constraints` must be passed to the `depCfg` when generating nested dependency plans. Without this, constraints would only apply to direct dependencies, not their transitive dependencies.

### Version Availability Check

When a pinned version is unavailable:

1. **Log warning**: Indicate which dependency couldn't use the pinned version
2. **Fall back to available**: Use latest available version to maintain CI stability
3. **Mark in plan**: Optionally note that a fallback occurred (for debugging)

This ensures CI doesn't fail when an old toolchain version is removed from the registry.

### Integration with Existing Constraints

The new `DependencyVersions` field integrates naturally:

| Constraint Type | Source in Golden File | Applied During |
|-----------------|----------------------|----------------|
| PipConstraints | `pip_exec.locked_requirements` | Decompose() |
| GoSum | `go_build.go_sum` | Decompose() |
| CargoLock | `cargo_build.lock_data` | Decompose() |
| NpmLock | `npm_exec.package_lock` | Decompose() |
| **DependencyVersions** | `dependencies[].version` | generateSingleDependencyPlan() |

## Implementation Approach

### Phase 1: Constraint Field Addition

Add the `DependencyVersions` field and extraction logic:

1. Add field to `EvalConstraints` struct in `internal/executor/constraints.go`
2. Add `extractDependencyVersions()` function
3. Call from `ExtractConstraintsFromPlan()`
4. Unit tests for extraction from nested dependency structures

**Files to modify:**
- `internal/executor/constraints.go`

### Phase 2: Plan Generator Integration

Use constraints during dependency plan generation:

1. Modify `generateSingleDependencyPlan()` to check `DependencyVersions`
2. Add logging for pinned version usage
3. Add graceful fallback for unavailable versions
4. Integration tests with mock recipes

**Files to modify:**
- `internal/executor/plan_generator.go`

### Phase 3: Validation and Testing

Verify the feature works end-to-end:

1. Add golden file tests that exercise toolchain pinning
2. Test with recipes that have toolchain dependencies (black, httpie, gh)
3. Test fallback behavior when pinned version doesn't exist
4. Update `scripts/validate-golden.sh` if needed (likely no changes)

**Files to modify:**
- `internal/executor/constraints_test.go`
- `internal/executor/plan_generator_test.go`
- Potentially `scripts/validate-golden.sh` (for validation mode)

### Phase 4: Documentation

Document the behavior:

1. Update constrained evaluation documentation
2. Add troubleshooting for version mismatch warnings
3. Document the DependencyVersions constraint field

**Files to modify:**
- Relevant documentation files

## Security Considerations

### Download Verification

**Not affected.** Toolchain pinning only affects version selection during plan generation. All download verification (checksums, signatures) happens during plan execution, which is unchanged.

### Execution Isolation

**Not affected.** This feature changes which version is selected, not how the selected version is executed. Execution permissions and isolation remain unchanged.

### Supply Chain Risks

**Minimal impact with mitigation.** The change allows using older toolchain versions that are still in the registry:

- **Risk**: An older version might have known vulnerabilities
- **Mitigation**: Version pinning only occurs during `--pin-from` constrained evaluation (CI validation), not during normal user installations
- **Mitigation**: Golden files should be regenerated periodically to pick up security updates
- **Impact**: Users running `tsuku install` (without `--pin-from`) are unaffected

### User Data Exposure

**Not applicable.** This feature operates on golden file JSON and recipe TOML. No user data is accessed, transmitted, or stored.

## Consequences

### Positive

- **CI stability restored**: Golden file validation works even when toolchain versions change upstream
- **Full determinism**: Given the same golden file, `--pin-from` produces identical plans
- **Minimal changes**: Extends existing infrastructure rather than creating parallel systems
- **Graceful degradation**: Old versions that are removed don't cause hard failures
- **No format changes**: Both recipe and golden file formats remain unchanged

### Negative

- **Version lag**: Pinned golden files may use older toolchain versions than latest
- **Fallback noise**: Warnings when pinned versions unavailable may cause confusion
- **Testing complexity**: Need to test with actual version unavailability scenarios

### Mitigations

- **Version lag**: Periodic golden file regeneration (existing process) updates toolchain versions
- **Fallback noise**: Clear warning messages explaining the fallback and its cause
- **Testing**: Mock version providers for unavailability testing
