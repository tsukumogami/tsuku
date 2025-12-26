# DESIGN: Recipe Validation Unification

## Status

**Proposed**

## Context and Problem Statement

Tsuku's recipe validation exists in two disconnected layers that have diverged from the runtime systems they're meant to verify.

**The Current Situation:**

The validator (`internal/recipe/validator.go`) performs static analysis of recipe TOML files by maintaining hardcoded copies of what should be authoritative registries:

1. **Action Registry Duplication**: `validator.go` defines `knownActions` (29 actions) separately from `actions/action.go` which is the authoritative registry populated via `init()`. Adding a new action requires updates in both places.

2. **Parameter Validation Duplication**: `validateActionParams()` in `validator.go` spans 140+ lines reimplementing parameter requirements for each action. This logic is separate from the actual parameter handling in each action's `Execute()` method. When an action adds a new required parameter, both must be updated.

3. **Version Source Duplication**: `validator.go` defines `validSources` (14 sources) separately from `provider_factory.go` which registers 16 strategies. The validator also reimplements `canInferVersionFromActions()` logic that mirrors provider factory inference.

4. **Two Validation Layers**:
   - `loader.go:validate()` provides minimal 5-check validation at parse time
   - `validator.go:ValidateBytes()` provides comprehensive validation via CLI
   - No shared code path between these layers

**The Core Problem:**

This design is a form of "synthetic validation" - the validator constructs its own model of what a valid recipe looks like rather than delegating to the runtime systems. This pattern:

- **Guarantees Drift**: Every new action or version source requires parallel updates
- **Breaks the DRY Principle**: Business logic is duplicated, not reused
- **Inverts the Dependency**: The validator knows about actions/versions when those systems should validate themselves
- **Provides False Confidence**: A recipe can pass validation but fail at runtime due to drift

**Why This Matters Now:**

As tsuku grows, the action count continues to increase (now at 29+ actions). Each addition compounds the maintenance burden. Recent work on action dependencies and build actions has highlighted how easily validator and runtime can diverge.

### Scope

**In scope:**
- Unifying the validation logic with authoritative registries
- Eliminating duplicate action/version source definitions
- Designing a validation architecture that prevents future drift

**Out of scope:**
- Changing the TOML recipe format
- Modifying how actions execute at runtime
- Adding new validation checks (beyond unification)

## Decision Drivers

- **Single Source of Truth**: Each piece of knowledge should exist in exactly one place
- **Validation by Construction**: Validation should happen through the same code path as execution, not via parallel data structures (the "parse, don't validate" philosophy)
- **Go Idiomatic Design**: Solutions should follow established Go patterns and idioms (interfaces, constructor validation, registry-as-validator)
- **OSS Maintainability**: Design for long-term clarity over short-term velocity; demonstrate proper coding skills
- **Fail Fast**: Catch validation errors at the earliest possible point (parse-time preferred over execution-time)
- **Separation of Concerns**: Each package should have a clear responsibility boundary
- **Testability**: The solution should be easy to unit test without complex setup
- **Backward Compatibility**: Existing recipes and CLI behavior should not break (though this is less important than architectural correctness for an evolving project)

## Implementation Context

### Research Summary

This design draws on extensive research into Go validation patterns, codebase precedents, and multiple design alternatives. Full research documents are available in `wip/research/`:

- `explore_validation_go-patterns.md` - Kubernetes, Terraform, and Go standard library patterns
- `explore_validation_action-self-validation.md` - Action interface extension analysis
- `explore_validation_version-provider-validation.md` - Version source unification approaches
- `explore_validation_codebase-precedents.md` - Existing patterns in tsuku
- `explore_validation_dry-patterns.md` - Five pattern alternatives with trade-off analysis

**Key findings:**

1. The executor's `ValidatePlan()` already correctly uses `actions.Get()` to validate actions against the authoritative registry - this is the pattern the recipe validator should follow.

2. The "parse, don't validate" philosophy from functional programming applies well here: validation should happen by actually constructing objects (or a lightweight validation equivalent), not by maintaining parallel lists.

3. Circular import prevention is the main constraint: the `recipe` package cannot import `version` (which already imports `recipe`), so version validation unification requires interface-based dependency inversion or a shared metadata package.

4. Go idioms favor registry-as-validator (query the registry to check validity) over schema-based validation (maintain separate lists of valid values).

## Considered Options

### Decision 1: Action Validation Strategy

How should the validator determine if an action name is valid and its parameters are correct?

#### Option 1A: Registry-as-Validator (Query actions.Get)

Replace the hardcoded `knownActions` map with direct registry queries:

```go
func validateSteps(result *ValidationResult, r *Recipe) {
    for i, step := range r.Steps {
        action := actions.Get(step.Action)
        if action == nil {
            result.addError(fmt.Sprintf("steps[%d].action", i),
                fmt.Sprintf("unknown action '%s'", step.Action))
            continue
        }
        // Action exists, continue with param validation
    }
}
```

**Pros:**
- Eliminates `knownActions` duplication entirely
- Single source of truth (the registry)
- Cannot drift - uses the same code path as execution

**Cons:**
- Only solves action *name* validation, not parameter validation (the larger problem)
- Requires validator to import actions package
- Similar action suggestions (for typos) would need the registry to expose `RegisteredNames()`
- Still leaves 140+ lines of `validateActionParams()` duplicated logic

#### Option 1B: Preflight Interface with Shared Parameter Extraction (Recommended)

This is the "parse, don't validate" pattern adapted for Go. Actions implement a `Preflight` interface that validates parameters without side effects. The key architectural insight is sharing parameter extraction code between `Preflight()` and `Execute()`, making drift impossible:

```go
// In actions package
type Preflight interface {
    Preflight(params map[string]interface{}) error
}

// Shared extraction function - SINGLE SOURCE OF TRUTH
func downloadParams(params map[string]interface{}) (url, dest string, err error) {
    url, ok := GetString(params, "url")
    if !ok {
        return "", "", fmt.Errorf("download action requires 'url' parameter")
    }
    // ... additional extraction
    return url, dest, nil
}

func (a *DownloadAction) Preflight(params map[string]interface{}) error {
    _, _, err := downloadParams(params)
    return err
}

func (a *DownloadAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    url, dest, err := downloadParams(params)
    if err != nil {
        return err
    }
    // ... execution with actual side effects
}
```

**Pros:**
- True single source of truth for parameter validation
- Cannot drift because Execute uses same extraction code
- Rich validation (semantic checks, not just required params)
- Gradual adoption - actions can opt-in

**Cons:**
- Requires adding interface + shared extraction to each action
- More upfront work than Option 1A
- Must ensure Preflight has no side effects

#### Option 1C: ParamSpec Metadata Interface

Actions declare their parameters via a metadata interface:

```go
type ParamSpec struct {
    Name     string
    Type     ParamType
    Required bool
    Default  interface{}
}

type ParamValidator interface {
    ParamSpecs() []ParamSpec
}
```

**Pros:**
- Explicit parameter documentation
- Can generate CLI help and documentation
- Compile-time verification that interface is implemented

**Cons:**
- Still two representations (metadata + actual Execute logic)
- Metadata can drift from actual behavior
- Verbose for actions with many parameters

### Decision 2: Version Source Validation Strategy

How should the validator check if a version source is valid?

#### Option 2A: Shared Metadata Package

Extract known sources to a new `internal/versionmeta` package that both `recipe` and `version` can import:

```go
// internal/versionmeta/sources.go
package versionmeta

var KnownSources = map[string]bool{
    "pypi": true, "crates_io": true, "npm": true, ...
}

var InferableActions = map[string]bool{
    "npm_install": true, "pipx_install": true, ...
}
```

**Pros:**
- Breaks circular dependency
- Single source of truth
- Simple data, easy to maintain

**Cons:**
- New package just for metadata
- Still manual synchronization with factory strategies

#### Option 2B: Interface-Based Dependency Inversion

Define a `VersionValidator` interface in `recipe` that `version` implements:

```go
// In recipe package
type VersionValidator interface {
    CanResolveVersion(r *Recipe) bool
    KnownSources() []string
}

var versionValidator VersionValidator

func SetVersionValidator(v VersionValidator) {
    versionValidator = v
}
```

The version package registers at init time.

**Pros:**
- Clean separation of concerns
- No circular dependency
- Validator uses actual factory logic

**Cons:**
- Runtime registration complexity
- Testing requires mocking
- More indirection

#### Option 2C: Accept Current Design with Sync Tests

Keep the current architecture but add consistency tests:

```go
func TestValidatorSourcesMatchFactory(t *testing.T) {
    // Verify validSources matches factory strategies
}
```

**Pros:**
- No code changes needed
- CI catches drift

**Cons:**
- Doesn't eliminate duplication
- Reactive rather than proactive
- Tests can be ignored or deleted

### Decision 3: loader.validate() vs validator.ValidateBytes() Unification

Should the two validation functions be unified?

#### Option 3A: Keep Separate (Status Quo with Documentation)

Keep both functions with clear documentation of their purposes:
- `loader.validate()`: Fast, minimal, parse-time checks
- `validator.ValidateBytes()`: Comprehensive CLI validation

**Pros:**
- No refactoring needed
- Fast parse-time path preserved

**Cons:**
- Continued code duplication
- Two places to update for new validation rules

#### Option 3B: Unified Validator with Mode Parameter

Single validation function with a mode parameter:

```go
type ValidationMode int

const (
    ValidationModeFast ValidationMode = iota  // Parse-time, fail-fast
    ValidationModeFull                         // CLI, accumulate all errors
)

func Validate(r *Recipe, mode ValidationMode) *ValidationResult
```

**Pros:**
- Single validation code path
- Clear modes for different use cases
- Easier to maintain

**Cons:**
- More complex function signature
- Need to ensure fast mode is actually fast

#### Option 3C: Layered Validation (Structural + Semantic)

Separate structural (TOML parsing) from semantic (registry queries) validation:

```go
func ValidateStructural(r *Recipe) []ValidationError  // No external deps
func ValidateSemantic(r *Recipe) []ValidationError    // Queries registries
```

**Pros:**
- Clear separation of concerns
- Structural validation can run without importing action/version packages
- Matches Kubernetes pattern

**Cons:**
- Two functions to call
- Need to decide which errors go where

## Decision Outcome

**Chosen: 1B + 2B + 3C**

### Summary

The solution combines Preflight interfaces for action validation (1B), dependency-inverted version validation (2B), and layered structural/semantic validation (3C). This approach eliminates all duplication by having validation flow through the same code paths as execution, aligns with the "parse, don't validate" philosophy from functional programming, and demonstrates idiomatic Go patterns.

### Rationale

**Why Option 1B (Preflight Interface)?**

This directly addresses the core problem: validation and execution currently use separate code paths. By introducing shared parameter extraction functions that both `Preflight()` and `Execute()` call, we make drift impossible at the source. This matches the user's stated preference for "dry-run instantiation" - validation happens by constructing/parsing, not by maintaining parallel validation rules.

The `Preflight` interface also provides a natural migration path: actions can opt-in gradually, with legacy validation falling back for unmigrated actions. Once all actions implement `Preflight`, the hardcoded `validateActionParams()` can be removed entirely.

**Why Option 2B (Interface-Based Dependency Inversion)?**

The circular import constraint (`version` imports `recipe`, so `recipe` cannot import `version`) is real but not insurmountable. Defining a `VersionValidator` interface in the `recipe` package that `version` implements at init-time is idiomatic Go and cleanly separates concerns. This is the same pattern used throughout the Go ecosystem (e.g., `database/sql` drivers).

Option 2A (shared metadata package) was considered but rejected because it still maintains separate data structures from the actual factory strategies, leaving room for drift.

**Why Option 3C (Layered Validation)?**

Separating structural validation (TOML parsing, field types) from semantic validation (registry queries, cross-field constraints) follows the Kubernetes model and enables:
- Fast structural validation at parse-time without action/version package imports
- Rich semantic validation for CLI that queries registries
- Clear separation of concerns (what's valid syntax vs what's valid meaning)

This is preferred over Option 3B (mode parameter) because layering makes responsibilities explicit rather than hiding them behind a mode flag.

### Trade-offs Accepted

1. **More upfront work**: Migrating 29+ actions to implement `Preflight` is more work than simply querying the registry for action names (1A). However, this investment pays off by eliminating the much larger `validateActionParams()` duplication.

2. **Runtime registration for version validation**: Interface-based dependency inversion requires init-time registration, adding some indirection. This is acceptable because it follows established Go patterns and enables true single-source-of-truth validation.

3. **Two validation functions to call**: Layered validation means callers invoke both `ValidateStructural()` and `ValidateSemantic()`. This is acceptable because responsibilities are clear and composable.

## Solution Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Recipe Validation                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────────────────┐          ┌──────────────────────────────┐ │
│  │  ValidateStructural │          │     ValidateSemantic         │ │
│  │  (recipe package)   │          │     (recipe package)         │ │
│  ├─────────────────────┤          ├──────────────────────────────┤ │
│  │ • TOML syntax       │          │ • Action existence (1B)      │ │
│  │ • Required fields   │          │ • Action params (Preflight)  │ │
│  │ • Type coercion     │          │ • Version source (2B)        │ │
│  │ • URL/path formats  │          │ • Cross-field constraints    │ │
│  └─────────────────────┘          └──────────────────────────────┘ │
│           │                                    │                   │
│           │                       ┌────────────┴────────────┐      │
│           │                       ▼                         ▼      │
│           │            ┌──────────────────┐    ┌───────────────────┐
│           │            │ ActionValidator  │    │ VersionValidator  │
│           │            │ (interface)      │    │ (interface)       │
│           │            └────────┬─────────┘    └────────┬──────────┘
│           │                     │                       │          │
└───────────│─────────────────────│───────────────────────│──────────┘
            │                     │                       │
            ▼                     ▼                       ▼
     loader.validate()     actions package         version package
     (uses Structural)     (implements via         (implements via
                           Preflight interface)    ProviderFactory)
```

### 1. Preflight Interface (actions package)

**New interface definition:**

```go
// internal/actions/preflight.go

// Preflight is implemented by actions that can validate their parameters
// without executing side effects. This enables static validation of recipes.
type Preflight interface {
    // Preflight validates that the given parameters would produce a valid
    // action execution. Returns nil if valid, error describing the problem
    // if invalid.
    //
    // CONTRACT: Preflight MUST NOT have side effects (no filesystem, no network).
    // It validates parameter presence, types, and semantic correctness.
    Preflight(params map[string]interface{}) error
}

// ValidateAction checks if an action exists and validates its parameters.
// This is the entry point for semantic validation of action steps.
func ValidateAction(name string, params map[string]interface{}) error {
    action := Get(name)
    if action == nil {
        return fmt.Errorf("unknown action '%s'", name)
    }

    if pf, ok := action.(Preflight); ok {
        return pf.Preflight(params)
    }

    // Action exists but doesn't implement Preflight - pass validation
    return nil
}

// RegisteredNames returns all registered action names (for error messages).
func RegisteredNames() []string {
    registryMu.RLock()
    defer registryMu.RUnlock()
    names := make([]string, 0, len(registry))
    for name := range registry {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}
```

**Shared parameter extraction pattern (example):**

```go
// internal/actions/download.go

// downloadParams extracts and validates download action parameters.
// This is the SINGLE SOURCE OF TRUTH for download parameter handling.
func downloadParams(params map[string]interface{}) (url, dest, checksum, algo string, err error) {
    url, ok := GetString(params, "url")
    if !ok {
        return "", "", "", "", fmt.Errorf("download action requires 'url' parameter")
    }

    dest, _ = GetString(params, "dest")
    if dest == "" {
        // Will be computed from URL at execution time
    }

    checksum, _ = GetString(params, "checksum")
    algo, _ = GetString(params, "checksum_algo")
    if algo == "" {
        algo = "sha256"
    }

    return url, dest, checksum, algo, nil
}

// Preflight validates download action parameters without side effects.
func (a *DownloadAction) Preflight(params map[string]interface{}) error {
    url, _, _, _, err := downloadParams(params)
    if err != nil {
        return err
    }

    // Additional validation that doesn't require execution
    if !strings.Contains(url, "{") {
        if _, err := neturl.Parse(url); err != nil {
            return fmt.Errorf("invalid URL format: %w", err)
        }
    }

    return nil
}

// Execute performs the download action.
func (a *DownloadAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    url, dest, checksum, algo, err := downloadParams(params)
    if err != nil {
        return err
    }

    // Proceed with actual download...
}
```

### 2. Version Validator Interface (recipe package)

**Interface definition:**

```go
// internal/recipe/version_validator.go

// VersionValidator validates version configuration for recipes.
// This interface is implemented by the version package and registered at init time.
type VersionValidator interface {
    // CanResolveVersion returns true if a version provider can be created for this recipe.
    CanResolveVersion(r *Recipe) bool

    // KnownSources returns the list of known version source values.
    KnownSources() []string

    // ValidateVersionConfig performs detailed validation of version configuration.
    // Returns nil if valid, error describing the problem if invalid.
    ValidateVersionConfig(r *Recipe) error
}

var versionValidator VersionValidator
var versionValidatorMu sync.RWMutex

// SetVersionValidator registers the version validator (called from version package init).
func SetVersionValidator(v VersionValidator) {
    versionValidatorMu.Lock()
    defer versionValidatorMu.Unlock()
    versionValidator = v
}

// getVersionValidator returns the registered validator or nil.
func getVersionValidator() VersionValidator {
    versionValidatorMu.RLock()
    defer versionValidatorMu.RUnlock()
    return versionValidator
}
```

**Implementation in version package:**

```go
// internal/version/validation.go

import "github.com/tsukumogami/tsuku/internal/recipe"

// FactoryValidator implements recipe.VersionValidator using the provider factory.
type FactoryValidator struct {
    factory *ProviderFactory
}

func (v *FactoryValidator) CanResolveVersion(r *recipe.Recipe) bool {
    for _, strategy := range v.factory.strategies {
        if strategy.CanHandle(r) {
            return true
        }
    }
    return false
}

func (v *FactoryValidator) KnownSources() []string {
    return []string{
        "pypi", "crates_io", "npm", "rubygems", "nixpkgs",
        "go_toolchain", "goproxy", "metacpan", "homebrew",
        "github_releases", "github_tags", "nodejs_dist",
        "hashicorp", "manual",
    }
}

func (v *FactoryValidator) ValidateVersionConfig(r *recipe.Recipe) error {
    // Use factory logic to validate
    if !v.CanResolveVersion(r) {
        return fmt.Errorf("no version source configured (add [version] section or use inferrable action)")
    }
    return nil
}

func init() {
    recipe.SetVersionValidator(&FactoryValidator{
        factory: NewProviderFactory(),
    })
}
```

### 3. Layered Validation Functions (recipe package)

**Refactored validation API:**

```go
// internal/recipe/validate.go

// ValidateStructural performs fast, structural validation without external dependencies.
// This is suitable for parse-time validation in the loader.
func ValidateStructural(r *Recipe) []ValidationError {
    var errors []ValidationError

    // Metadata validation
    if r.Metadata.Name == "" {
        errors = append(errors, ValidationError{Field: "metadata.name", Message: "name is required"})
    } else if strings.Contains(r.Metadata.Name, " ") {
        errors = append(errors, ValidationError{Field: "metadata.name", Message: "name should not contain spaces"})
    }

    // Type validation
    if r.Metadata.Type != "" && r.Metadata.Type != RecipeTypeTool && r.Metadata.Type != RecipeTypeLibrary {
        errors = append(errors, ValidationError{Field: "metadata.type", Message: "invalid type"})
    }

    // Steps existence
    if len(r.Steps) == 0 {
        errors = append(errors, ValidationError{Field: "steps", Message: "at least one step is required"})
    }

    // Verify command (for non-libraries)
    if r.Metadata.Type != RecipeTypeLibrary && r.Verify.Command == "" {
        errors = append(errors, ValidationError{Field: "verify.command", Message: "command is required"})
    }

    // URL format validation, path security checks, etc.
    // (No registry queries - pure structural checks)

    return errors
}

// ValidateSemantic performs deep validation that queries action and version registries.
// This is suitable for CLI validation where comprehensive checks are desired.
func ValidateSemantic(r *Recipe) []ValidationError {
    var errors []ValidationError

    // Action validation (using Preflight interface)
    for i, step := range r.Steps {
        if err := actions.ValidateAction(step.Action, step.Params); err != nil {
            errors = append(errors, ValidationError{
                Field:   fmt.Sprintf("steps[%d]", i),
                Message: err.Error(),
            })
        }
    }

    // Version validation (using registered validator)
    if vv := getVersionValidator(); vv != nil {
        if err := vv.ValidateVersionConfig(r); err != nil {
            errors = append(errors, ValidationError{
                Field:   "version",
                Message: err.Error(),
            })
        }
    }

    return errors
}

// ValidateFull performs both structural and semantic validation.
// This is the entry point for CLI validation (tsuku validate).
func ValidateFull(r *Recipe) *ValidationResult {
    result := &ValidationResult{Valid: true, Recipe: r}

    for _, err := range ValidateStructural(r) {
        result.Errors = append(result.Errors, err)
        result.Valid = false
    }

    for _, err := range ValidateSemantic(r) {
        result.Errors = append(result.Errors, err)
        result.Valid = false
    }

    return result
}
```

### 4. Updated loader.validate()

```go
// internal/recipe/loader.go

func validate(r *Recipe) error {
    errors := ValidateStructural(r)
    if len(errors) > 0 {
        // Return first error for fail-fast behavior
        return fmt.Errorf("%s: %s", errors[0].Field, errors[0].Message)
    }
    return nil
}
```

## Implementation Approach

### Migration Strategy

The implementation follows a phased approach to minimize risk:

**Phase 1: Infrastructure (1 issue)**
- Add `Preflight` interface to `actions/action.go`
- Add `ValidateAction()` and `RegisteredNames()` helper functions
- Add `VersionValidator` interface to `recipe` package
- Add layered validation functions (`ValidateStructural`, `ValidateSemantic`)

**Phase 2: Version Validation Unification (1 issue)**
- Implement `FactoryValidator` in version package
- Register at init time
- Update `validateVersion()` to use the interface
- Remove hardcoded `validSources` and `canInferVersionFromActions()`

**Phase 3: Action Migration (multiple issues, can be parallelized)**
- Migrate core actions first: `download`, `extract`, `install_binaries`
- Migrate ecosystem actions: `npm_install`, `cargo_install`, `pipx_install`, etc.
- Migrate build actions: `configure_make`, `cmake_build`, etc.

**Phase 4: Cleanup (1 issue)**
- Remove hardcoded `knownActions` map
- Remove `validateActionParams()` function
- Update `ValidateBytes()` to use `ValidateFull()`
- Update loader to use `ValidateStructural()`

### Backward Compatibility

- Existing recipes continue to work without modification
- CLI validation output format preserved
- Actions without `Preflight` implementation pass validation (gradual adoption)

## Security Considerations

This design affects how tsuku validates recipes before downloading and executing external binaries. Each security dimension is analyzed below.

### Download Verification

**Current state**: The validator checks URL format but does not validate checksums or verify download sources. Actual checksum verification happens at execution time in the `download` action.

**Impact of this design**: The `Preflight` interface enables validation of checksum-related parameters (checksum presence, algorithm validity) without performing the actual download. This catches configuration errors earlier, but does not change the fundamental security model - checksums are still verified at execution time.

**No regression**: This design does not weaken download verification. The same checksum logic in the `download` action's `Execute()` method continues to run. The shared `downloadParams()` extraction function validates that checksum parameters are well-formed, adding an earlier validation layer.

### Execution Isolation

**Current state**: Tsuku executes actions with the user's permissions. No sandboxing or isolation is applied. This is by design - tsuku is a user-space tool manager, not a system package manager.

**Impact of this design**: The `Preflight` interface contract explicitly forbids side effects (no filesystem, no network). This is enforced by code review, not by a sandbox. Implementers must ensure `Preflight()` methods only validate parameters without executing code.

**Mitigation**: The design documentation and interface comments clearly state the no-side-effects contract. Unit tests should verify that `Preflight()` methods do not touch the filesystem or network. If a `Preflight()` method violates this contract, it is a bug in that action's implementation, not in the validation architecture.

### Supply Chain Risks

**Current state**: Recipes specify download URLs (often GitHub releases, crates.io, PyPI, etc.). The registry is a Git repository; recipe provenance is tracked via commits.

**Impact of this design**: The version validation unification (Option 2B) routes version source validation through the same `ProviderFactory` that resolves versions at runtime. This means:

1. If a version source is valid enough for the factory to handle, it passes validation
2. Unknown version sources are rejected by both validation and execution

This tightens the coupling between validation and execution, reducing the risk of a recipe passing validation but failing at runtime due to source configuration issues.

**No new risks introduced**: This design does not change where binaries come from or how they are verified. It only unifies the validation code paths.

### User Data Exposure

**Current state**: The validator reads recipe files from disk. It does not transmit data or access user files beyond the recipe being validated.

**Impact of this design**:

1. **Structural validation** (`ValidateStructural`): Operates purely on in-memory recipe data. No external calls.

2. **Semantic validation** (`ValidateSemantic`): Queries the action registry and version validator interface. These are in-memory registries populated at init time. No network calls during validation.

3. **Version validator registration**: The `FactoryValidator` is registered at init time from the version package. This does not transmit data; it only makes the factory's logic available for validation queries.

**No data exposure**: This design does not introduce any new data transmission. Validation remains a local operation.

### Summary

| Dimension | Impact | Risk Level |
|-----------|--------|------------|
| Download verification | Earlier parameter validation, no weakening | No change |
| Execution isolation | Preflight contract forbids side effects | Relies on code review |
| Supply chain risks | Tighter validation-execution coupling | Slight improvement |
| User data exposure | No network calls added | No change |

This design is security-neutral-to-positive: it does not introduce new attack vectors while slightly improving validation coverage and reducing validation-execution drift.

## Consequences

### Positive

1. **Single Source of Truth**: Action parameter requirements are defined once in the shared extraction functions, not duplicated between validator and executor.

2. **Drift Impossible**: Validation and execution use the same code path for parameter extraction. If an action changes its parameters, both validation and execution automatically reflect the change.

3. **Reduced Maintenance Burden**: No need to update `knownActions`, `validSources`, or `validateActionParams()` when adding new actions or version sources.

4. **Better Error Messages**: Validation errors come from the same code that would fail at execution time, producing consistent and accurate error messages.

5. **Gradual Migration**: Actions can adopt `Preflight` incrementally. The system remains functional during migration.

6. **Testability**: `Preflight` methods are pure functions (no side effects) that are easy to unit test in isolation.

7. **Go Idiomatic**: The design follows established Go patterns (interfaces, init-time registration, registry-as-validator) rather than introducing non-standard patterns.

### Negative

1. **Migration Effort**: 29+ actions need to be updated to implement `Preflight` and refactor to shared extraction functions. This is significant upfront work.

2. **Interface Proliferation**: Two new interfaces (`Preflight`, `VersionValidator`) add to the codebase's interface count. However, both serve clear purposes and follow Go conventions.

3. **Init-Time Coupling**: The `VersionValidator` is registered at init time, creating an implicit dependency between packages. This is mitigated by the clear interface contract.

4. **Partial Validation During Migration**: Until all actions implement `Preflight`, some actions will pass validation without parameter checking. This is temporary and tracks toward full coverage.

### Neutral

1. **Validation Speed**: Structural validation remains fast. Semantic validation adds registry queries but these are in-memory lookups, not expensive operations.

2. **API Surface**: The public validation API changes from `ValidateBytes()` to layered functions, but backward compatibility is maintained through `ValidateFull()`.

