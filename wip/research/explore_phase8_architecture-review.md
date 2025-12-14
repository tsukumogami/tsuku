# Architecture Review: Centralize Validation Logic

## Executive Summary

The proposed solution is **architecturally sound and ready for implementation** with minor clarifications. The three-layer design (metadata registry, requirements computation, unified validator) effectively addresses the core problem of scattered validation logic while following established codebase patterns.

**Recommendation**: Proceed with implementation as proposed, with the clarifications noted below.

## 1. Clarity Assessment

### What's Clear

The architecture provides clear separation of concerns:

1. **Action Metadata Layer** (`internal/actions/validation_metadata.go`)
   - Static registry mapping action names to requirements
   - Follows proven `ActionDependencies` pattern
   - Single function `GetActionValidationMetadata()` for lookups

2. **Requirements Computation Layer** (`internal/validate/requirements.go`)
   - Pure function: `InstallationPlan` → `ValidationRequirements`
   - Aggregates metadata across all plan steps
   - Derives container configuration (image, resources, network)

3. **Unified Validator** (`internal/validate/validator.go`)
   - Single entry point `Validate(ctx, plan, reqs)`
   - Consumes `ValidationRequirements` to configure container
   - Replaces both `Validate()` and `ValidateSourceBuild()`

The data flow is well-documented and easy to follow.

### Areas Needing Clarification

**1. Script generation function signature**

The design shows:
```go
func (e *Executor) buildValidationScript(plan *executor.InstallationPlan, reqs *ValidationRequirements) string
```

But the existing code has two methods:
- `buildSourceBuildPlanScript(r *recipe.Recipe)` - uses plan
- `buildSourceBuildScript(r *recipe.Recipe)` - legacy method

**Question**: Should the new unified method be:
- `buildValidationScript(plan, reqs)` as shown in the design?
- Does it still need the recipe for verification lookup?

**Recommendation**: The design document should specify:
```go
// buildValidationScript creates a shell script for container validation.
// It uses the plan for installation steps and requirements for build tool setup.
// The recipe is still needed for verification command lookup.
func (e *Executor) buildValidationScript(
    r *recipe.Recipe,
    plan *executor.InstallationPlan,
    reqs *ValidationRequirements,
) string
```

**2. Transition from `ValidateSourceBuild()` to unified `Validate()`**

Current code:
- `Validate()` - bottles, no network
- `ValidateSourceBuild()` - source builds, network enabled

Proposed:
- Single `Validate(plan, reqs)` - network determined by requirements

**Question**: How does the signature change map to call sites?

**Current**:
```go
func (e *Executor) Validate(ctx context.Context, r *recipe.Recipe) (*ValidationResult, error)
func (e *Executor) ValidateSourceBuild(ctx context.Context, r *recipe.Recipe) (*ValidationResult, error)
```

**Proposed**:
```go
func (e *Executor) Validate(
    ctx context.Context,
    plan *executor.InstallationPlan,
    reqs *ValidationRequirements,
) (*ValidationResult, error)
```

**Observation**: The signature changes from `recipe` to `plan + reqs`. This is correct since validation should operate on plans (not recipes), but the design document doesn't explicitly show this transition.

**Recommendation**: Add explicit signature comparison to Phase 3 documentation.

**3. Existing container validation entry point**

The current `Validate()` method signature shows it takes a recipe. But validation should operate on plans (as per eval+plan architecture).

**Question**: Does the current `Validate()` already generate a plan internally, or does it execute recipes directly?

Looking at `ValidateSourceBuild()` code (lines 97-114), I see:
```go
exec, err := planexec.New(r)
// ...
plan, err := exec.GeneratePlan(ctx, planexec.PlanConfig{...})
```

**Answer**: Yes, validators already generate plans internally. The refactor just makes this explicit in the signature.

**Recommendation**: Clarify in Phase 3 that the signature change surfaces what was already happening internally.

## 2. Missing Components Analysis

### What's Present

All essential components are specified:

- ✅ `ActionValidationMetadata` struct with network and build tools fields
- ✅ Static registry map `actionValidationMetadata`
- ✅ Getter function `GetActionValidationMetadata()`
- ✅ `ValidationRequirements` struct with network, tools, image, resources
- ✅ Computation function `ComputeValidationRequirements()`
- ✅ Unified `Validate()` method
- ✅ Script builder using requirements
- ✅ CLI command `tsuku validate`

### Missing or Underspecified Components

**1. Error Handling Strategy**

The design doesn't specify what happens when:
- An action in the plan has no metadata entry
- A plan contains unknown actions
- Requirements computation encounters invalid data

**Current behavior** (from `GetActionValidationMetadata`):
```go
// Returns zero value if action is not found (defaults to no network, no build tools).
func GetActionValidationMetadata(action string) ActionValidationMetadata {
    return actionValidationMetadata[action]
}
```

This fails silently for unknown actions. Is this acceptable?

**Recommendation**: Add explicit error handling specification:
```go
// Option A: Log warning and return zero value (safe defaults)
func GetActionValidationMetadata(action string) ActionValidationMetadata {
    meta, ok := actionValidationMetadata[action]
    if !ok {
        log.Warn("Unknown action in plan", "action", action, "using_defaults", "no network, no build tools")
    }
    return meta
}

// Option B: Return error to force metadata completeness
func GetActionValidationMetadata(action string) (ActionValidationMetadata, error) {
    meta, ok := actionValidationMetadata[action]
    if !ok {
        return ActionValidationMetadata{}, fmt.Errorf("no validation metadata for action: %s", action)
    }
    return meta, nil
}
```

**Preferred**: Option A (safe defaults) since actions without special requirements genuinely need no metadata.

**2. Backwards Compatibility Path for Existing Validators**

The design doesn't specify a migration strategy for existing code that calls:
- `executor.Validate()` with a recipe
- `executor.ValidateSourceBuild()` with a recipe

**Question**: Do we need transitional wrapper methods?

```go
// Deprecated: Use Validate(plan, reqs) directly
func (e *Executor) ValidateRecipe(ctx context.Context, r *recipe.Recipe) (*ValidationResult, error) {
    plan, err := e.GeneratePlan(...)
    reqs := validate.ComputeValidationRequirements(plan)
    return e.Validate(ctx, plan, reqs)
}
```

**Recommendation**: Since this is an internal API (not used by external callers), Phase 4 can update all call sites directly without transitional wrappers. Document this assumption explicitly.

**3. Testing Strategy**

The design mentions "tests for registry completeness" but doesn't specify what "completeness" means.

**Recommendation**: Add explicit test requirements:

```go
// Test: All registered actions have metadata entries
func TestActionValidationMetadata_AllActionsHaveEntries(t *testing.T) {
    allActions := actions.RegisteredActionNames()
    for _, action := range allActions {
        meta := actions.GetActionValidationMetadata(action)
        // This test just ensures no panics; zero values are acceptable
        _ = meta
    }
}

// Test: All known composite actions are NOT in metadata
// (they should decompose to primitives)
func TestActionValidationMetadata_NoCompositeActions(t *testing.T) {
    compositeActions := []string{
        "download_archive",
        "github_archive",
        "github_file",
        "hashicorp_release",
    }
    for _, action := range compositeActions {
        _, exists := actionValidationMetadata[action]
        if exists {
            t.Errorf("Composite action %s should not have metadata (decomposes to primitives)", action)
        }
    }
}
```

**4. CLI Command Specification Details**

The design mentions `tsuku validate <recipe|plan.json>` but doesn't specify:
- How to distinguish recipe vs plan input (file extension? content sniffing?)
- Error messages for invalid inputs
- Exit codes for success/failure

**Recommendation**: Add CLI specification:

```go
// Detect input type by file extension and content
func detectInputType(path string) (InputType, error) {
    if strings.HasSuffix(path, ".json") {
        return InputTypePlan, nil
    }
    if strings.HasSuffix(path, ".toml") {
        return InputTypeRecipe, nil
    }
    // Try to load as recipe first (more common)
    if _, err := recipe.LoadFile(path); err == nil {
        return InputTypeRecipe, nil
    }
    // Try as plan
    if _, err := executor.LoadPlan(path); err == nil {
        return InputTypePlan, nil
    }
    return InputTypeUnknown, fmt.Errorf("input must be .toml recipe or .json plan")
}
```

Exit codes:
- 0: Validation passed
- 1: Validation failed
- 2: Input error (file not found, parse error)

## 3. Implementation Phase Sequencing

### Current Phases (from design doc)

1. Add Action Metadata Registry
2. Add ValidationRequirements Computation
3. Refactor Executor to Use Requirements
4. Update Builders
5. Add CLI Command

### Analysis

**Phase 1-2 ordering is correct**: Metadata and computation can be built independently without touching existing validation.

**Phase 3-4 dependency is correct**: Executor must be refactored before builders can use it.

**Phase 5 placement is debatable**: The CLI command could be implemented in Phase 3 (after unified validator exists) to enable testing without builder changes.

**Suggested adjustment**: Split Phase 3 into two sub-phases:

**Phase 3A: Create Unified Validate() Method (Non-Breaking)**
- Add new `Validate(plan, reqs)` method alongside existing methods
- Don't remove `ValidateSourceBuild()` yet
- Implement using requirements-based script generation

**Phase 3B: Remove Legacy Methods (Breaking Change)**
- Remove `ValidateSourceBuild()`
- Remove `detectRequiredBuildTools()`
- Update internal tests

**Phase 4: Update Builders** (unchanged)

**Phase 5: Add CLI Command** (unchanged - could move earlier but not critical)

**Rationale**: Phase 3A allows testing the new unified validator in isolation before making breaking changes. This reduces risk.

### Missing Phase: Integration Testing

The design doesn't mention an integration testing phase. Validation is a critical operation that should be tested end-to-end.

**Recommendation**: Add Phase 6:

**Phase 6: Integration Testing**
- Test `tsuku validate` with real recipes from registry
- Verify bottle recipes validate offline (network=none)
- Verify source recipes validate with network (network=host)
- Test various build tool combinations (cmake, cargo, go, etc.)
- Verify repair loop still works with unified validation

## 4. Simpler Alternatives

### Alternative 1: Embed Metadata in Plan Steps

Instead of computing requirements from action names, embed them in the plan during generation:

```go
type ResolvedStep struct {
    Action string
    Params map[string]interface{}
    // ... existing fields ...

    // Validation metadata (new)
    RequiresNetwork bool     `json:"requires_network,omitempty"`
    BuildTools      []string `json:"build_tools,omitempty"`
}
```

**Pros**:
- No need for `ComputeValidationRequirements()` function
- Plan is self-describing
- Validation logic doesn't need action registry access

**Cons**:
- Plan format version bump required (breaks existing plans)
- Larger plan JSON size
- Duplicates information derivable from action names
- Forces plan regeneration for all existing recipes

**Verdict**: Rejected. The design document already considered this as "Option 2B" and correctly rejected it for requiring plan format changes.

### Alternative 2: Just Use Recipe Actions Directly

Instead of using plans at all, have `ComputeValidationRequirements()` operate on recipes:

```go
func ComputeValidationRequirements(r *recipe.Recipe) *ValidationRequirements {
    // Iterate r.Steps instead of plan.Steps
    // Same aggregation logic
}
```

**Pros**:
- Simpler - no plan generation needed for validation
- Faster - skip plan generation step
- Recipe is the source of truth

**Cons**:
- Loses conditional step handling (when.os, when.arch)
- Can't validate hand-written plans
- Doesn't work with `tsuku validate plan.json` use case
- Inconsistent with eval+plan architecture

**Verdict**: Rejected. Validation should validate what will actually execute (the plan), not what might execute (the recipe). The design correctly operates on plans.

### Alternative 3: Inline Metadata in Action Registrations

Instead of a separate metadata map, embed metadata in the existing action registration:

```go
// In each action file
func init() {
    actions.RegisterWithMetadata("cargo_build", &CargoBuildAction{}, actions.Metadata{
        RequiresNetwork: true,
        BuildTools:      []string{"curl", "build-essential"},
    })
}
```

**Pros**:
- Metadata co-located with action implementation
- Enforced at registration time
- Single registration call per action

**Cons**:
- Requires modifying 33+ action files for initial implementation
- Requires changing action registration API
- Requires migrating all existing `Register()` calls
- Larger initial changeset

**Verdict**: Rejected for initial implementation, but worth considering as future refactor. The design document already considered this as "Option 1C" and correctly prioritized gradual migration over all-or-nothing changes.

### Alternative 4: No Centralization - Keep Builder-Specific Logic

The status quo: each builder decides how to validate.

**Pros**:
- No refactoring needed
- Builders can customize validation
- No shared state

**Cons**:
- No standalone `tsuku validate` command
- Duplicated `detectRequiredBuildTools()` logic
- Inconsistent validation behavior
- Cannot validate recipes outside `tsuku create` flow

**Verdict**: Rejected. This is the problem the design is solving.

### Conclusion on Alternatives

**The proposed design is the simplest solution that meets all requirements**:
- Enables standalone validation (requirement)
- Works with existing plans (backwards compatibility)
- Follows established patterns (minimal cognitive overhead)
- Allows gradual migration (low risk)

No simpler alternative meets these constraints.

## 5. Critical Design Decisions Review

### Decision 1: Static Registry vs Interface Methods

**Chosen**: Static registry map (Option 1D)

**Assessment**: Correct choice because:
- Matches existing `ActionDependencies` pattern (proven in production)
- No breaking changes to Action interface
- Easy to test (simple map lookups)
- Gradual migration path

**Potential issue**: Metadata can get out of sync with action implementation.

**Mitigation**: The design mentions "Add tests verifying all known actions have entries" but doesn't specify enforcement mechanism.

**Recommendation**: Add linter or test that fails if:
- An action is registered without metadata entry
- Metadata references non-existent actions

```go
// Test that enforces metadata completeness
func TestActionValidationMetadata_Completeness(t *testing.T) {
    registered := actions.RegisteredActions()
    for name := range registered {
        // Composite actions decompose, so skip them
        if actions.IsCompositeAction(name) {
            continue
        }

        meta := actions.GetActionValidationMetadata(name)
        // Just accessing it ensures it exists (even if zero-valued)
        t.Logf("Action %s: network=%v tools=%v", name, meta.RequiresNetwork, meta.BuildTools)
    }
}
```

### Decision 2: Separate Requirements Struct vs Plan Fields

**Chosen**: Separate `ValidationRequirements` struct (Option 2C)

**Assessment**: Correct choice because:
- No plan format version bump
- Works with existing cached plans
- Clean separation of concerns
- Validation logic independent of plan internals

**Potential issue**: Requirements must be recomputed each validation.

**Performance impact**: For typical recipes with <10 steps:
- Map lookups: O(10) = negligible
- String allocations: ~1KB for build tools list
- Total overhead: <1ms

**Verdict**: Performance impact is acceptable for one-time validation operation.

### Decision 3: apt Package Names in BuildTools

**Assumption**: Validation runs in Debian/Ubuntu containers, so apt package names are appropriate.

**Assessment**: Reasonable for current scope, but limits future extensibility.

**Potential issues**:
- What if we want to validate on Alpine (apk)?
- What if we want to validate on macOS (brew)?

**Recommendation**: Document assumption explicitly:

```go
type ActionValidationMetadata struct {
    RequiresNetwork bool

    // BuildTools lists apt package names required by this action.
    // NOTE: This assumes Debian/Ubuntu validation containers.
    // For multi-distro support, this would need to become a map[string][]string
    // keyed by package manager type.
    BuildTools []string
}
```

**Future extensibility path** (if needed):
```go
type ActionValidationMetadata struct {
    RequiresNetwork bool
    BuildTools      map[string][]string // "apt" -> ["cmake"], "apk" -> ["cmake"], etc.
}
```

For now, single-distro assumption is fine. Document it clearly.

### Decision 4: Zero-Value Defaults for Missing Actions

The `GetActionValidationMetadata()` returns zero values for unknown actions.

**Assessment**: Appropriate default behavior because:
- Most actions have no special requirements
- Failing hard would require every action to have explicit entry
- Safe defaults (no network, no build tools) are conservative

**Recommendation**: Add logging to catch typos:

```go
func GetActionValidationMetadata(action string) ActionValidationMetadata {
    meta, ok := actionValidationMetadata[action]
    if !ok {
        // Log warning in case this is a typo
        log.Debug("No validation metadata for action (using safe defaults)",
            "action", action,
            "network", false,
            "build_tools", "none")
    }
    return meta
}
```

## 6. Risks and Mitigations

### Risk 1: Metadata Incompleteness

**Risk**: New actions are added without updating metadata registry.

**Impact**: Validation may fail due to missing build tools or network access.

**Likelihood**: Medium (manual process, easy to forget)

**Mitigation**:
1. Add test that verifies all registered actions are covered
2. Document in action development guide
3. Consider pre-commit hook that checks for metadata

### Risk 2: Incorrect Build Tool Mappings

**Risk**: Metadata specifies wrong apt packages or misses required tools.

**Impact**: Container validation fails with missing dependency errors.

**Likelihood**: Low (common tools like cmake, autoconf are well-known)

**Detection**: Integration tests catch this immediately.

**Mitigation**: Phase 6 integration testing verifies real recipes work.

### Risk 3: Network Access Misconfiguration

**Risk**: Action requires network but metadata says `RequiresNetwork: false`.

**Impact**: Container validation hangs or fails trying to access network.

**Likelihood**: Low (only ~5 actions need network: cargo_build, go_build, cpan_install, npm_install, pip_install)

**Detection**: Integration tests catch this.

**Mitigation**: Explicit list of network-requiring actions is small and well-documented.

### Risk 4: Breaking Changes During Refactor

**Risk**: Removing `ValidateSourceBuild()` breaks existing call sites.

**Impact**: Build failures, broken tests.

**Likelihood**: High if we don't update all call sites.

**Mitigation**:
1. Grep for all call sites before removal
2. Update in single atomic commit
3. Split Phase 3 into 3A (additive) and 3B (removal)

### Risk 5: CLI Command Confusion

**Risk**: Users don't understand difference between validating recipe vs plan.

**Impact**: Confusion, support burden.

**Likelihood**: Medium (this is a new concept for users)

**Mitigation**:
1. Clear help text explaining difference
2. Auto-detect input type by extension
3. Error messages explain what was expected

## 7. Recommendations

### Must Have (Blocking Issues)

**None identified**. The architecture is sound and ready for implementation.

### Should Have (Important Improvements)

1. **Clarify script generation signature** - Include recipe parameter if needed for verification lookup

2. **Add error handling specification** - Document behavior for unknown actions

3. **Split Phase 3** - Separate additive changes (3A) from breaking changes (3B)

4. **Add Phase 6** - Integration testing with real recipes

5. **Document apt package assumption** - Make distro dependency explicit

### Nice to Have (Future Enhancements)

1. **Metadata completeness linter** - Automated check for missing entries

2. **Multi-distro support** - Extensibility for non-apt package managers

3. **Transitional wrappers** - If external code uses validation API (investigate first)

## 8. Final Verdict

**Architecture Quality: 9/10**

The design is well-thought-out, follows established patterns, and addresses all stated requirements. The three-layer separation of concerns is clean and testable.

**Minor deductions for**:
- Incomplete error handling specification
- Missing integration testing phase
- Unclear script generation signature transition

**Implementation Readiness: 8/10**

The design provides sufficient detail for implementation to begin. All major components are specified with example code.

**Gaps**:
- Error handling details
- Testing completeness criteria
- CLI command detailed specification

**Recommendation**: **PROCEED WITH IMPLEMENTATION**

The architecture is sound. Address the clarifications noted above during Phase 1-2 implementation. The issues identified are minor and don't invalidate the overall design.

## Appendix: Code Impact Analysis

### Files to Create

1. `internal/actions/validation_metadata.go` (~150 lines)
2. `internal/validate/requirements.go` (~100 lines)
3. `cmd/tsuku/validate_container.go` (~200 lines) - CLI command

### Files to Modify

1. `internal/validate/executor.go`
   - Remove `ValidateSourceBuild()` (~60 lines deleted)
   - Modify `Validate()` signature (~20 lines changed)
   - Remove `detectRequiredBuildTools()` (~50 lines deleted)

2. `internal/validate/source_build.go`
   - Remove `buildSourceBuildScript()` and `buildSourceBuildPlanScript()` (~100 lines deleted)
   - Add `buildValidationScript(plan, reqs)` (~80 lines added)

3. `internal/builders/github_release.go`
   - Update validation call sites (~10 lines changed)

4. `internal/builders/homebrew.go`
   - Update validation call sites (~20 lines changed)

### Tests to Create

1. `internal/actions/validation_metadata_test.go`
2. `internal/validate/requirements_test.go`
3. `cmd/tsuku/validate_container_test.go`

### Tests to Update

1. `internal/validate/executor_test.go` - signature changes
2. `internal/builders/github_release_test.go` - new validation behavior
3. `internal/builders/homebrew_test.go` - new validation behavior

### Net Change Estimate

- **Added**: ~630 lines (metadata, requirements, CLI, tests)
- **Removed**: ~230 lines (legacy methods)
- **Modified**: ~50 lines (call sites)
- **Net**: +400 lines

**Impact**: Medium-sized refactor, well-scoped to validation subsystem.

## Appendix: Action Metadata Coverage

Based on grep output showing 33+ action types, the metadata registry must cover:

**Core primitives** (no special requirements):
- download, extract, chmod, install_binaries, install_libraries
- set_env, set_rpath, link_dependencies
- text_replace, apply_patch, apply_patch_file

**Build actions** (need build tools, no network):
- configure_make → autoconf, automake, libtool, pkg-config
- cmake_build → cmake, ninja-build
- go_build → curl (for Go installer)
- cargo_build → curl (for rustup)

**Ecosystem actions** (need network + tools):
- cpan_install → perl, cpanminus + network
- npm_install → network (node already provided)
- npm_exec → no network (modules pre-installed)
- pip_install → network
- pipx_install → network
- gem_install → network
- gem_exec → network
- cargo_install → curl + network
- go_install → curl + network

**Homebrew actions** (need build tools on source path):
- homebrew_source → configure_make tools
- homebrew_bottle → no special requirements

**Nix actions** (need nix runtime):
- nix_install → nix-portable (special case)
- nix_realize → nix-portable (special case)

**System package managers** (need network):
- apt_install → network
- yum_install → network
- brew_install → network

**Composite actions** (decompose, no metadata needed):
- download_archive, github_archive, github_file, hashicorp_release

**Total metadata entries needed**: ~25-30 actions

This is a manageable scope for Phase 1 implementation.
