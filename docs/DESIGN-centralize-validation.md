# Design: Centralize Validation Logic

## Status

Proposed

## Context and Problem Statement

Tsuku's container-based validation ensures recipes work correctly by executing them in isolated containers. The eval+plan architecture (PR #530) enables offline validation by caching downloads during plan generation on the host, then running `tsuku install --plan` in a container with cached assets.

However, validation behavior is currently scattered across individual builders rather than being a unified, recipe-driven operation:

- **GitHub Release builder** calls `Validate()` with `Network: none`
- **Homebrew bottle builder** calls `Validate()` with `Network: none`
- **Homebrew source builder** calls `ValidateSourceBuild()` with `Network: host`

Each builder independently decides:
1. Whether to validate (checks if executor is present)
2. Which validation method to call
3. What container image to use
4. Whether network access is needed
5. What resource limits to apply
6. How to handle failures (fail build vs log warning)

This architecture creates several problems:

**1. Validation cannot be invoked independently**

There is no way to validate an existing recipe outside the `tsuku create` flow. A user who writes or modifies a recipe cannot run `tsuku validate <recipe>` to test it.

**2. Validation depends on transient builder context**

The decision of which validation method to use relies on information the builder has at runtime (e.g., "I'm generating a source recipe") rather than information derivable from the recipe itself.

**3. Duplicated knowledge about action requirements**

`detectRequiredBuildTools()` in `source_build.go` contains a switch statement mapping action names to apt packages:

```go
switch step.Action {
case "configure_make":
    toolsNeeded["autoconf"] = true
case "cargo_build":
    toolsNeeded["curl"] = true
// ...
}
```

This knowledge should live with the actions themselves, not in the validator. When new actions are added, this switch must be manually updated.

**4. Network requirements are implicit**

Some actions require network access (e.g., `cargo_build` fetches crates, `go_build` fetches modules, `apt_install` needs package repositories). This is currently handled by using different validation methods, but the requirement isn't surfaced in the plan or derivable from recipe analysis.

**5. Inconsistent failure semantics**

Bottle validation failures trigger the LLM repair loop, but source validation failures are logged as warnings. This inconsistency is builder-specific rather than policy-driven.

### Scope

**In scope:**
- Centralizing validation logic into a single entry point
- Deriving validation requirements (image, network, resources, build tools) from recipe/plan content
- Surfacing action metadata (network requirements, build dependencies) for validation decisions
- Adding `tsuku validate` command for standalone validation
- Refactoring builders to use the centralized validation

**Out of scope:**
- Removing network access from ecosystem actions (cargo_build, go_build, etc.)
- Caching ecosystem dependencies (crates, go modules, npm packages)
- Changes to the eval+plan architecture itself
- Repair loop behavior changes

## Decision Drivers

- **Single source of truth**: Validation requirements should be derivable from recipe/plan content alone
- **Action encapsulation**: Each action should declare its own requirements (network, build tools)
- **Independent invocation**: Validation must work outside the `tsuku create` flow
- **Backwards compatibility**: Existing recipes and plans must continue to work
- **Minimal duplication**: Knowledge about actions shouldn't be duplicated between action implementations and validators
- **Plan completeness**: The installation plan should contain enough information to determine validation requirements
- **Testing simplicity**: Solutions should be easy to test without complex mocking
- **Migration path**: Gradual adoption preferred over all-or-nothing changes

## Success Criteria

A successful solution will:
1. Enable `tsuku validate recipe.toml` to work without any builder context
2. Derive validation requirements (network, build tools, image) from plan content alone
3. Require updating only one location when adding a new action's requirements
4. Surface network requirements explicitly so validation can configure containers appropriately
5. Work with existing plans (no forced regeneration)

## Implementation Context

### Existing Patterns

The action system uses **static registry maps** for metadata rather than instance methods:

```go
// Determinism classification
var deterministicActions = map[string]bool{
    "download": true, "extract": true, // ...
    "cargo_build": false, "go_build": false, // ...
}

// Runtime dependencies
var ActionDependencies = map[string]ActionDeps{
    "npm_install":  {InstallTime: []string{"nodejs"}, Runtime: []string{"nodejs"}},
    "cpan_install": {InstallTime: []string{"perl"}, Runtime: []string{"perl"}},
}
```

This pattern is extensible - new metadata dimensions can be added as new maps.

### Current Validation Flow

```
Builder (knows recipe type)
   |
   +-- Bottle recipe --> Validate() --> Network: none, Debian
   |
   +-- Source recipe --> ValidateSourceBuild() --> Network: host, Ubuntu
                              |
                              v
                    detectRequiredBuildTools() [duplicates action knowledge]
```

### Information Available in Plan

The `ResolvedStep` struct contains:
- Action name and parameters
- Evaluable flag (can be reproduced from URLs/checksums)
- Deterministic flag (produces identical results)
- URL, checksum, size for downloads

### Information Missing from Plan

- Network requirements per step
- Build tool requirements per step
- Aggregate validation configuration (image, resources)

### Conventions to Follow

- Use static registry maps for action metadata (like `ActionDependencies`)
- Keep validation logic in `internal/validate/` package
- Plan generation in `internal/executor/` package
- Actions define their own metadata, validator consumes it

## Considered Options

This design addresses two independent decisions:
1. Where to store action metadata (network requirements, build tools)
2. How to surface validation requirements for use by the validator

### Decision 1: Action Metadata Storage

#### Option 1A: Static Registry Maps

Extend the existing pattern of static maps in `decomposable.go`:

```go
var ActionNetworkRequirements = map[string]bool{
    "download": false,      // uses cached files
    "cargo_build": true,    // fetches crates
    "go_build": true,       // fetches modules
    "configure_make": false, // source already extracted
}

var ActionBuildTools = map[string][]string{
    "configure_make": {"autoconf", "automake", "libtool", "pkg-config"},
    "cmake_build": {"cmake", "ninja-build"},
    "cpan_install": {"perl", "cpanminus"},
}
```

**Pros:**
- Follows existing pattern (ActionDependencies, deterministicActions)
- Simple to implement
- Easy to query during plan generation
- No interface changes to existing actions

**Cons:**
- Metadata lives separately from action implementation
- Adding a new action requires updating multiple files
- Can become out of sync if action behavior changes

#### Option 1B: Interface Methods on Actions

Add methods to the Action interface:

```go
type ActionMetadata interface {
    RequiresNetwork() bool
    BuildTools() []string
}

// Actions implement this interface
func (a *CargoAction) RequiresNetwork() bool { return true }
func (a *CargoAction) BuildTools() []string { return nil }
```

**Pros:**
- Metadata co-located with action implementation
- Harder to forget when adding new actions (compiler enforces interface)
- Self-documenting

**Cons:**
- Requires modifying all 49 existing actions
- Breaking interface change
- Most actions would return default values (false, nil)

#### Option 1C: Action Struct with Metadata Fields

Embed metadata in the action registration:

```go
type ActionInfo struct {
    Action         Action
    Network        bool
    BuildTools     []string
    Deterministic  bool
}

func RegisterWithMetadata(info ActionInfo) { ... }
```

**Pros:**
- All metadata in one place per action
- Registration-time validation possible
- Consolidates existing scattered maps

**Cons:**
- Larger refactor of registration system
- Need to migrate all existing Register() calls
- All-or-nothing migration (can't adopt gradually)

#### Option 1D: Structured Metadata Registry

Combine extensibility of static maps with structured organization:

```go
type ActionValidationMetadata struct {
    RequiresNetwork bool
    BuildTools      []string  // apt package names
}

var ActionValidationMetadata = map[string]ActionValidationMetadata{
    "configure_make": {
        RequiresNetwork: false,
        BuildTools:      []string{"autoconf", "automake", "libtool", "pkg-config"},
    },
    "cargo_build": {
        RequiresNetwork: true,
        BuildTools:      []string{"curl"},  // for rustup
    },
    "download": {
        RequiresNetwork: false,  // uses cached files
        BuildTools:      nil,
    },
}
```

**Pros:**
- Follows existing ActionDependencies pattern (proven in production)
- Consolidates network + build tools in single struct (vs separate maps)
- Easy to audit - single map shows all requirements
- No interface changes to actions
- Gradual migration possible (add entries as needed)
- Simple to extend (add fields to struct)

**Cons:**
- Metadata still separate from action implementation
- Adding new action requires updating map (but only one place)
- Can become out of sync if action behavior changes

### Decision 2: Surfacing Validation Requirements

#### Option 2A: Plan-Level Aggregate Fields

Add aggregate fields to `InstallationPlan`:

```go
type InstallationPlan struct {
    // ... existing fields ...

    // Validation requirements (computed from steps)
    RequiresNetwork bool     // true if any step needs network
    BuildTools      []string // union of all step build tools
}
```

**Pros:**
- Simple for validator to consume
- Pre-computed during plan generation
- Clear contract between plan generator and validator

**Cons:**
- Loses per-step granularity (can't skip network for specific steps)
- Plan must be regenerated if requirements change
- Duplicates information derivable from steps

#### Option 2B: Per-Step Metadata in ResolvedStep

Add metadata to each step:

```go
type ResolvedStep struct {
    // ... existing fields ...

    RequiresNetwork bool
    BuildTools      []string
}
```

**Pros:**
- Full granularity preserved
- Validator can make nuanced decisions
- Steps are self-describing

**Cons:**
- Larger plan JSON
- Validator must aggregate across steps
- More complex validation logic

#### Option 2C: Separate ValidationRequirements Struct

Compute requirements as a separate output alongside the plan:

```go
type ValidationRequirements struct {
    Network    bool
    BuildTools []string
    Image      string  // recommended container image
    Resources  ResourceLimits
}

func ComputeValidationRequirements(plan *InstallationPlan) *ValidationRequirements
```

**Pros:**
- Clean separation of concerns
- Requirements can be computed from any plan (including hand-written ones)
- Validator takes this struct directly

**Cons:**
- Another data structure to maintain
- Must be recomputed if plan changes
- Indirection between plan and validation

### Evaluation Against Decision Drivers

| Option | Single Source | Action Encapsulation | Backwards Compat | Min Duplication | Testing Simplicity | Migration Path |
|--------|---------------|---------------------|------------------|-----------------|-------------------|----------------|
| 1A: Static Maps | Good | Fair | Good | Fair | Good | Good |
| 1B: Interface Methods | Good | Good | Poor | Good | Fair | Poor |
| 1C: Action Struct | Good | Good | Fair | Good | Good | Poor |
| 1D: Structured Registry | Good | Fair | Good | Good | Good | Good |
| 2A: Plan Aggregate | Good | N/A | Fair | Fair | Good | Fair |
| 2B: Per-Step Metadata | Good | N/A | Fair | Good | Good | Fair |
| 2C: Separate Struct | Good | N/A | Good | Good | Good | Good |

Note: Options 2A/2B require plan format version bump; Option 2C works with existing plans.

### Assumptions

The following assumptions inform this design:

1. **Network requirements are action-intrinsic**: This design assumes network requirements can be determined from action name alone. In practice, some actions may have conditional requirements (e.g., `cargo_build` with vendored dependencies). For the initial implementation, we treat network as binary per action. Conditional requirements can be addressed in future iterations.

2. **Build tools are apt package names**: The `BuildTools` field contains Debian/Ubuntu apt package names. This is appropriate since validation currently uses `debian:bookworm-slim` and `ubuntu:22.04` images. Multi-platform support (brew packages, etc.) is out of scope.

3. **Build tools are action-intrinsic**: Most build tools are determined by action name. Parameter-dependent requirements (e.g., cmake generator affecting tool choice) are edge cases handled by including the superset of possibly-needed tools.

4. **Offline vs network validation coexist**: Some validation can be fully offline (binary installation), while source builds require network for ecosystem dependencies. The design accommodates both modes.

### Uncertainties

- **Plan size impact**: Adding metadata to every step increases plan JSON size by approximately 20-50 bytes per step. For typical recipes with <10 steps, this is negligible (~200-500 bytes total).
- **Future metadata dimensions**: May need additional metadata (resource hints, platform constraints) in the future. The structured registry (Option 1D) makes this easy to extend.
- **Composite action handling**: When a composite action decomposes to primitives, the primitive steps' metadata should be used (not the composite's).

## Decision Outcome

**Chosen: Option 1D (Structured Metadata Registry) + Option 2C (Separate ValidationRequirements Struct)**

### Summary

We use a structured metadata registry to store action validation requirements, combined with a computed `ValidationRequirements` struct that derives container configuration from the plan. This approach follows established codebase patterns, requires no plan format changes, and enables standalone validation through a simple `ComputeValidationRequirements(plan)` function.

### Rationale

**Option 1D (Structured Metadata Registry)** was chosen because:
- **Follows existing pattern**: The codebase already uses `ActionDependencies` map with a struct type. This is a proven, production-tested approach.
- **Good migration path**: Can add entries incrementally without touching all actions at once.
- **Single source of truth**: All validation metadata for an action lives in one place.
- **Easy to extend**: Future metadata dimensions just add fields to the struct.
- **Testing simplicity**: Simple map lookups are easy to test without mocking.

**Option 2C (Separate ValidationRequirements Struct)** was chosen because:
- **Backwards compatible**: Works with existing plans without regeneration.
- **Clean separation**: Validator doesn't need to understand plan internals beyond step actions.
- **Independent invocation**: Any code with a plan can compute requirements.
- **No plan format changes**: Avoids version bump and migration complexity.

**Alternatives rejected:**

- **Option 1A (Separate Static Maps)**: Would scatter metadata across multiple maps, making it harder to ensure completeness and consistency.
- **Option 1B (Interface Methods)**: Breaking change to Action interface, requires modifying 33+ action files, most returning default values.
- **Option 1C (Action Struct Registration)**: All-or-nothing migration, larger refactor than necessary.
- **Option 2A (Plan Aggregate Fields)**: Requires plan format version bump; loses ability to work with existing plans.
- **Option 2B (Per-Step Metadata)**: Requires plan format changes; adds complexity to plan generation for marginal benefit.

### Trade-offs Accepted

By choosing this approach, we accept:

1. **Metadata separation from action code**: The validation requirements for an action live in `decomposable.go` rather than the action's own file. This is the same trade-off already made for `ActionDependencies` and `deterministicActions`.

2. **Must remember to update registry**: Adding a new action requires updating the metadata map. However, this is one location rather than multiple scattered maps.

3. **Computation at validation time**: Requirements are computed from the plan each time rather than cached. This is acceptable because the computation is trivial (iterate steps, lookup metadata, aggregate).

These trade-offs are acceptable because:
- The existing codebase already uses this pattern successfully
- The alternative (interface methods) has worse trade-offs (breaking changes, boilerplate)
- Validation is not performance-critical (runs once per recipe)

## Solution Architecture

### Overview

The solution introduces a centralized validation system that derives container configuration from recipe/plan content. The system consists of three layers:

1. **Action Metadata Layer**: Static registry mapping action names to validation requirements
2. **Requirements Computation Layer**: Function that aggregates metadata from plan steps
3. **Unified Validator**: Single entry point that consumes requirements and executes validation

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Validation Flow                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Recipe ──► GeneratePlan() ──► InstallationPlan                     │
│                                       │                              │
│                                       ▼                              │
│                          ComputeValidationRequirements()             │
│                                       │                              │
│                    ┌──────────────────┴──────────────────┐          │
│                    │                                      │          │
│                    ▼                                      ▼          │
│         ActionValidationMetadata              ValidationRequirements │
│         (static registry)                     (computed struct)      │
│                                                      │               │
│                                                      ▼               │
│                                              Validate(plan, reqs)    │
│                                                      │               │
│                                                      ▼               │
│                                              Container Execution     │
└─────────────────────────────────────────────────────────────────────┘
```

### Components

#### 1. ActionValidationMetadata Registry

Location: `internal/actions/validation_metadata.go`

```go
// ActionValidationMetadata describes validation requirements for an action.
type ActionValidationMetadata struct {
    // RequiresNetwork indicates whether this action needs network access.
    // Actions that fetch external dependencies (cargo_build, go_build) need network.
    // Actions that work with cached/pre-downloaded content do not.
    RequiresNetwork bool

    // BuildTools lists apt packages required by this action.
    // These are installed via apt-get in the validation container.
    BuildTools []string
}

// actionValidationMetadata maps action names to their validation requirements.
var actionValidationMetadata = map[string]ActionValidationMetadata{
    // Core primitives - work offline with cached content
    "download":         {RequiresNetwork: false, BuildTools: nil},
    "extract":          {RequiresNetwork: false, BuildTools: nil},
    "chmod":            {RequiresNetwork: false, BuildTools: nil},
    "install_binaries": {RequiresNetwork: false, BuildTools: nil},
    "apply_patch_file": {RequiresNetwork: false, BuildTools: []string{"patch"}},

    // Build actions - source is cached, but need build tools
    "configure_make": {
        RequiresNetwork: false,
        BuildTools:      []string{"build-essential", "autoconf", "automake", "libtool", "pkg-config"},
    },
    "cmake_build": {
        RequiresNetwork: false,
        BuildTools:      []string{"build-essential", "cmake", "ninja-build"},
    },

    // Ecosystem primitives - need network for dependency resolution
    "cargo_build": {
        RequiresNetwork: true,
        BuildTools:      []string{"curl", "build-essential"},  // curl for rustup
    },
    "go_build": {
        RequiresNetwork: true,
        BuildTools:      []string{"curl", "build-essential"},  // curl for Go installer
    },
    "cpan_install": {
        RequiresNetwork: true,
        BuildTools:      []string{"perl", "cpanminus", "build-essential"},
    },
    "npm_exec": {
        RequiresNetwork: false,  // node_modules already installed
        BuildTools:      nil,
    },
}

// GetActionValidationMetadata returns validation metadata for an action.
// Returns zero value if action is not found (defaults to no network, no build tools).
// Unknown actions are treated safely: they don't request network and don't require build tools.
// This allows new actions to work without metadata entries, though they should be added.
func GetActionValidationMetadata(action string) ActionValidationMetadata {
    return actionValidationMetadata[action]
}

// AllActionValidationMetadata returns all registered action metadata.
// Used by tests to verify completeness.
func AllActionValidationMetadata() map[string]ActionValidationMetadata {
    // Return copy to prevent modification
    result := make(map[string]ActionValidationMetadata, len(actionValidationMetadata))
    for k, v := range actionValidationMetadata {
        result[k] = v
    }
    return result
}
```

#### 2. ValidationRequirements Struct

Location: `internal/validate/requirements.go`

```go
// ValidationRequirements describes what a validation container needs.
type ValidationRequirements struct {
    // RequiresNetwork is true if any step needs network access.
    RequiresNetwork bool

    // BuildTools is the deduplicated union of all steps' build tool requirements.
    BuildTools []string

    // Image is the recommended container image based on requirements.
    // Uses debian:bookworm-slim for binary-only, ubuntu:22.04 for builds.
    Image string

    // Resources are the recommended resource limits.
    Resources ResourceLimits
}

// ComputeValidationRequirements derives container requirements from a plan.
func ComputeValidationRequirements(plan *executor.InstallationPlan) *ValidationRequirements {
    reqs := &ValidationRequirements{
        RequiresNetwork: false,
        BuildTools:      nil,
        Image:           DefaultValidationImage,  // debian:bookworm-slim
        Resources: ResourceLimits{
            Memory:  "2g",
            CPUs:    "2",
            PidsMax: 100,
        },
    }

    toolSet := make(map[string]bool)

    for _, step := range plan.Steps {
        metadata := actions.GetActionValidationMetadata(step.Action)

        if metadata.RequiresNetwork {
            reqs.RequiresNetwork = true
        }

        for _, tool := range metadata.BuildTools {
            toolSet[tool] = true
        }
    }

    // Convert toolSet to sorted slice
    for tool := range toolSet {
        reqs.BuildTools = append(reqs.BuildTools, tool)
    }
    sort.Strings(reqs.BuildTools)

    // Upgrade image and resources if build tools are needed
    if len(reqs.BuildTools) > 0 {
        reqs.Image = SourceBuildValidationImage  // ubuntu:22.04
        reqs.Resources = SourceBuildLimits()     // 4g, 4 CPUs, 15min timeout
    }

    return reqs
}
```

#### 3. Unified Validator

Location: `internal/validate/validator.go`

```go
// Validate runs a plan in an isolated container using computed requirements.
// This is the single entry point for all validation.
func (e *Executor) Validate(
    ctx context.Context,
    plan *executor.InstallationPlan,
    reqs *ValidationRequirements,
) (*ValidationResult, error) {
    // Detect container runtime
    runtime, err := e.detector.Detect(ctx)
    if err != nil {
        // ... handle no runtime
    }

    // Build container options from requirements
    opts := RunOptions{
        Image:   reqs.Image,
        Command: []string{"/bin/bash", "/workspace/validate.sh"},
        Network: "none",
        Limits:  reqs.Resources,
        // ... mounts, env, etc.
    }

    // Enable network if required
    if reqs.RequiresNetwork {
        opts.Network = "host"
    }

    // Generate validation script with build tool installation
    script := e.buildValidationScript(plan, reqs)

    // ... write script, run container, check results
}

// buildValidationScript creates the shell script for validation.
func (e *Executor) buildValidationScript(
    plan *executor.InstallationPlan,
    reqs *ValidationRequirements,
) string {
    var sb strings.Builder

    sb.WriteString("#!/bin/bash\n")
    sb.WriteString("set -e\n\n")

    // Install build tools if needed
    if len(reqs.BuildTools) > 0 {
        sb.WriteString("# Install build tools\n")
        sb.WriteString("apt-get update -qq\n")
        sb.WriteString(fmt.Sprintf("apt-get install -qq -y %s >/dev/null 2>&1\n\n",
            strings.Join(reqs.BuildTools, " ")))
    }

    // Setup and install
    sb.WriteString("# Setup TSUKU_HOME\n")
    // ... mkdir, cp recipe, tsuku install --plan

    return sb.String()
}
```

### Key Interfaces

#### Public API

```go
// In internal/validate/requirements.go
func ComputeValidationRequirements(plan *executor.InstallationPlan) *ValidationRequirements

// In internal/validate/executor.go
func (e *Executor) Validate(ctx context.Context, plan *executor.InstallationPlan, reqs *ValidationRequirements) (*ValidationResult, error)

// In internal/actions/validation_metadata.go
func GetActionValidationMetadata(action string) ActionValidationMetadata
```

#### CLI Command

```
tsuku validate <recipe|plan.json> [--network] [--verbose]
```

The command:
1. Loads recipe (if .toml) or plan (if .json)
2. Generates plan (if recipe)
3. Computes validation requirements
4. Runs validation in container
5. Reports success/failure

### Data Flow

```
1. User invokes: tsuku validate recipe.toml

2. CLI loads recipe, creates Executor, generates plan:
   plan, err := exec.GeneratePlan(ctx, PlanConfig{...})

3. CLI computes requirements from plan:
   reqs := validate.ComputeValidationRequirements(plan)

4. CLI creates validator and runs:
   result, err := validator.Validate(ctx, plan, reqs)

5. Validator uses reqs to configure container:
   - reqs.Image → container image
   - reqs.RequiresNetwork → network mode
   - reqs.BuildTools → apt-get install in script
   - reqs.Resources → memory/CPU limits

6. Container executes:
   - Install build tools (if any)
   - Run tsuku install --plan
   - Execute verify command

7. Result returned to CLI
```

## Implementation Approach

### Phase 1: Add Action Metadata Registry

**Goal**: Create the structured metadata registry without changing existing behavior.

- Create `internal/actions/validation_metadata.go` with `ActionValidationMetadata` struct and map
- Populate metadata for all primitive and ecosystem actions
- Add `GetActionValidationMetadata()` function
- Add tests verifying all known actions have entries

**Deliverables**:
- New file with metadata registry
- Tests for registry completeness

### Phase 2: Add ValidationRequirements Computation

**Goal**: Implement requirements derivation from plans.

- Create `internal/validate/requirements.go` with `ValidationRequirements` struct
- Implement `ComputeValidationRequirements()` function
- Add tests with various plan configurations

**Deliverables**:
- ValidationRequirements struct and computation function
- Unit tests for aggregation logic

### Phase 3: Refactor Executor to Use Requirements

**Goal**: Unify `Validate()` and `ValidateSourceBuild()` into single method.

- Modify `Executor.Validate()` to accept `ValidationRequirements`
- Remove `ValidateSourceBuild()` (functionality absorbed into Validate)
- Remove `detectRequiredBuildTools()` (replaced by metadata registry)
- Update script generation to use requirements

**Deliverables**:
- Unified Validate() method
- Removal of duplicated logic
- Updated tests

### Phase 4: Update Builders to Use Centralized Validation

**Goal**: Refactor builders to use the new unified validation.

- Update `github_release.go` to compute requirements and call unified Validate()
- Update `homebrew.go` (both bottle and source paths) similarly
- Remove builder-specific validation decisions

**Deliverables**:
- Updated builders using centralized validation
- Consistent validation behavior across all builders

### Phase 5: Add CLI Command

**Goal**: Enable standalone validation via `tsuku validate`.

- Add `cmd/tsuku/validate.go` command
- Support recipe (.toml) and plan (.json) inputs
- Add `--verbose` flag for detailed output
- Document in help text

**Deliverables**:
- New `tsuku validate` command
- User documentation

## Consequences

### Positive

- **Single source of truth**: Validation requirements live in one place (metadata registry)
- **Independent validation**: Users can run `tsuku validate recipe.toml` standalone
- **Reduced duplication**: No more `detectRequiredBuildTools()` switch statement
- **Consistent behavior**: All builders use the same validation logic
- **Extensible**: Adding new metadata dimensions just adds fields to the struct
- **Backwards compatible**: Existing plans work without regeneration

### Negative

- **Metadata separation**: Action validation requirements don't live with action code
- **Manual updates**: New actions require updating the metadata registry
- **Runtime computation**: Requirements computed on each validation (not cached)

### Mitigations

- **Metadata separation**: This is the existing pattern (ActionDependencies). Accept as known trade-off.
- **Manual updates**: Add test that verifies all registered actions have metadata entries. CI will catch missing entries.
- **Runtime computation**: Computation is O(n) where n is number of steps. For typical recipes (<10 steps), this is negligible.

## Security Considerations

### Download Verification

This design does not change download verification behavior. The existing eval+plan architecture continues to:
- Compute SHA256 checksums during plan generation
- Cache downloaded files for offline container execution
- Verify checksums during `tsuku install --plan`

The centralization refactor preserves all existing checksum verification. No new download paths are introduced.

### Execution Isolation

**Container isolation is preserved and improved:**

1. **Network access is now explicit**: The `RequiresNetwork` field makes network requirements visible and auditable. Previously, network access was an implicit decision made by builders.

2. **Minimal privilege principle**: Containers run with:
   - `Network: none` when possible (binary installations)
   - `Network: host` only when required (ecosystem builds)
   - Read-only mounts for tsuku binary and download cache
   - Non-root user where possible

3. **Resource limits**: The design continues using resource limits (memory, CPU, pids) to prevent runaway processes.

**New risk introduced**: The `tsuku validate` command allows users to validate arbitrary recipes. A malicious recipe could:
- Consume resources up to the container limits
- Attempt network access if RequiresNetwork is true
- Execute arbitrary commands within the container

**Mitigation**: Container isolation contains these risks. The validation container has no access to the host filesystem beyond the mounted workspace, and resource limits prevent DoS.

### Supply Chain Risks

This design does not change supply chain trust model:

- **Recipe source trust**: Users must trust recipe sources (registry or local files)
- **Binary source trust**: Recipes specify upstream sources (GitHub releases, Homebrew, etc.)
- **Build tool trust**: apt packages are fetched from distribution repositories

**New exposure point**: The `ActionValidationMetadata` registry lists apt packages to install. If this registry is compromised:
- Malicious package names could be substituted
- Extra packages could be added to exfiltrate data

**Mitigation**: The metadata registry is code-reviewed like any other source file. The package names are well-known standard packages (autoconf, cmake, etc.). Typosquatting risk is low because packages are installed by name from official repositories.

### User Data Exposure

**No new user data exposure:**

- Validation runs in isolated containers with no access to user home directory
- The workspace contains only recipe, plan, and download cache
- No telemetry or external reporting is added by this design

**Existing exposure (unchanged):**
- Recipes are loaded from user-specified paths
- Plans may contain version numbers that could identify user system

### Additional Considerations

**Container Networking**: The design uses `--network=host` for ecosystem builds. A future improvement could use bridge networking with egress filtering to limit access to known package registries only. This is out of scope for the initial implementation but noted as a security hardening opportunity.

**Metadata Registry Integrity**: The package names in `ActionValidationMetadata` should be verified against known package lists. A CI check could validate that all referenced apt packages exist in the target distribution.

**User Awareness**: The `tsuku validate` command should display what network access will be granted before execution, especially when validating untrusted recipes.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious recipe execution | Container isolation, resource limits, user displays network requirements | Recipe could consume allowed resources |
| Network-enabled builds have broader attack surface | Network enabled only when RequiresNetwork=true; use bridge networking in future | Data exfiltration via network, compromised dependencies |
| Compromised metadata registry | Code review, well-known package names, CI validation | Insider threat, typosquatting of new packages |
| Arbitrary user-provided recipes | Container isolation, read-only mounts, clear warnings | Resource exhaustion within limits |

