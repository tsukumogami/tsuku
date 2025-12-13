# Design: Deterministic Execution (Plan-Based Installation)

- **Status**: Proposed
- **Milestone**: Deterministic Recipe Execution
- **Author**: @dangazineu
- **Created**: 2025-12-13
- **Scope**: Tactical

## Upstream Design Reference

This design implements Milestone 2 of [DESIGN-deterministic-resolution.md](DESIGN-deterministic-resolution.md).

**Relevant sections:**
- Vision: "A recipe is a program that produces a deterministic installation plan"
- Milestone 2: Deterministic Execution
- Core Insight: Split installation into evaluation (dynamic) and execution (deterministic) phases

**Prerequisite work completed:**
- Milestone 1 (#367): Installation plans and `tsuku eval` command
- Decomposable Actions (#436-#449): Plans contain only primitive operations

## Context and Problem Statement

The current `tsuku install` command executes recipes directly, generating plans as a post-execution artifact. This creates several problems:

1. **Two execution paths diverge**: `tsuku install foo` executes the recipe directly, while `tsuku eval foo` generates a plan. These can produce different results if upstream assets change between commands.

2. **Plans are not authoritative**: Plans are generated after successful installation, making them a record of what happened rather than a specification of what should happen.

3. **No plan reuse for re-installation**: When re-installing a tool with an exact version, the system generates a fresh plan each time rather than reusing the cached plan from the original installation.

4. **Checksum verification is reactive**: Checksums are computed during evaluation but not enforced during execution, missing the opportunity to detect upstream changes.

The upstream strategic design mandates that `tsuku install foo` must be functionally equivalent to `tsuku eval foo | tsuku install --plan -`. This requires refactoring the executor so all installations go through plan generation and execution.

### Scope

**In scope:**
- Refactor executor to use plan-first installation flow
- Plan caching and reuse based on version constraints
- `--refresh` flag to force plan regeneration
- Checksum verification during plan execution
- Clear error messaging for checksum mismatches

**Out of scope:**
- `tsuku install --plan <file>` (Milestone 3, issue #370)
- Lock files for team coordination (tracked separately)
- Plan signing or cryptographic verification (future security enhancement)
- Concurrent installation handling (existing file locking applies)
- Plan garbage collection / retention policies

## Decision Drivers

1. **Architectural equivalence**: `tsuku install foo` and `tsuku eval foo | tsuku install --plan -` must produce identical results
2. **Determinism by default**: Exact version constraints should produce identical installations without opt-in
3. **Performance**: Avoid redundant plan generation when cached plans are valid
4. **Consistency**: `tsuku eval` and `tsuku install` should have coherent, predictable caching behavior
5. **Industry alignment**: Follow established patterns from Nix, Terraform, Cargo, npm
6. **Backward compatibility**: Existing installations without plans should continue to work

## Considered Options

### Decision 1: Caching Behavior for `tsuku eval`

Should `tsuku eval` return cached plans or always generate fresh plans?

#### Option 1A: Always Fresh Evaluation

`tsuku eval` always generates a fresh plan, never using cached plans.

**Pros:**
- Matches user expectation: "evaluate" implies computing current state
- Essential for golden file testing: detects upstream asset changes
- Simple mental model: eval = fresh, install = may use cache
- Aligns with Terraform's `terraform plan` (always computes current state)

**Cons:**
- Slower for repeated evaluation (downloads required)
- Cannot inspect cached plan via eval output

#### Option 1B: Check Cache First

`tsuku eval` returns cached plan if inputs match, with `--fresh` flag to override.

**Pros:**
- Faster repeated evaluation
- Can inspect what's already stored

**Cons:**
- Confuses the purpose of eval (inspection vs computation)
- Recipe testing requires remembering to use `--fresh`
- Different semantics from other tools

### Decision 2: Caching Behavior for `tsuku install`

When should `tsuku install` reuse a cached plan vs generate a new one?

#### Option 2A: Always Generate Fresh Plans

`tsuku install` always generates a fresh plan before execution.

**Pros:**
- Simplest implementation
- Always gets latest upstream state
- No cache invalidation complexity

**Cons:**
- Violates determinism goal for pinned versions
- Wastes resources on redundant downloads
- Cannot detect upstream changes (no baseline to compare)

#### Option 2B: Cache by Exact Version Match

Reuse cached plan when version constraint exactly matches resolved version and recipe hash matches.

**Pros:**
- Deterministic re-installation for pinned versions
- Detects upstream changes via checksum mismatch
- Aligns with Cargo (`--locked`), npm (`npm ci`), Nix (derivation reuse)
- Performance benefit from cache reuse

**Cons:**
- Cache invalidation logic required
- Users must understand when cache applies
- Recipe changes require explicit handling

#### Option 2C: Cache by Version Constraint Type

Different caching rules based on constraint type:
- Exact (`foo@1.2.3`): Use cached plan
- Dynamic (`foo`, `foo@latest`, `foo@1.x`): Always regenerate

**Pros:**
- Intuitive: "pinned = deterministic, floating = latest"
- Matches user intent: explicit version = want reproducibility
- Aligns with upstream strategic design specification

**Cons:**
- More complex logic than 2A or 2B
- Edge cases around version constraint parsing

### Decision 3: Plan Cache Invalidation

When should a cached plan be considered invalid?

#### Option 3A: Recipe Hash Only

Invalidate cached plan when recipe file hash changes.

**Pros:**
- Simple to implement
- Catches recipe modifications
- Clear invalidation signal

**Cons:**
- Doesn't detect format version changes
- Doesn't detect platform mismatches (if checking wrong cached plan)

#### Option 3B: Multi-Factor Validation

Validate recipe hash, plan format version, and platform before reusing cached plan.

**Pros:**
- Comprehensive validation
- Catches format evolution
- Prevents cross-platform confusion

**Cons:**
- More complex validation logic
- More fields to track and compare

#### Option 3C: Multi-Factor with Staleness Warning

Same as 3B, but also warn (don't fail) when plan is stale but still usable.

**Pros:**
- All benefits of 3B
- Users informed about staleness without blocking
- Graceful degradation for old state files

**Cons:**
- Most complex implementation
- Users may ignore warnings

### Decision 4: Executor Refactoring Approach

How should the executor be refactored to support plan-based execution?

#### Option 4A: Add ExecutePlan Method

Add a new `ExecutePlan(ctx, plan)` method alongside existing `Execute(ctx)`.

**Pros:**
- Non-breaking change
- Can migrate incrementally
- Clear separation of concerns

**Cons:**
- Two code paths to maintain initially
- Risk of divergence between Execute and ExecutePlan

#### Option 4B: Rewrite Execute to Use Plans

Modify `Execute(ctx)` to internally generate a plan, then execute it.

**Pros:**
- Single code path
- Guarantees equivalence
- Cleaner long-term architecture

**Cons:**
- Larger change
- Requires all callers to have plans available
- Harder to review/test incrementally

#### Option 4C: Execute Delegates to ExecutePlan

`Execute(ctx)` generates plan and calls `ExecutePlan(ctx, plan)`. Both methods public.

**Pros:**
- Single execution path (ExecutePlan)
- Backward compatible (Execute still works)
- Enables Milestone 3 (`--plan` flag) naturally
- Incremental migration possible

**Cons:**
- Two entry points to document
- Plan generation always happens (even if Execute called)

### Decision 5: Checksum Mismatch Behavior

What happens when a download's checksum doesn't match the plan?

#### Option 5A: Hard Failure

Checksum mismatch is an installation failure with clear error message.

**Pros:**
- Security-first approach
- Aligns with upstream design ("mismatch = failure")
- Forces user to investigate upstream changes
- Prevents silent installation of modified binaries

**Cons:**
- Blocks installation until user takes action
- May frustrate users who just want latest version

#### Option 5B: Warning with Proceed Option

Warn about mismatch, ask user whether to proceed.

**Pros:**
- User has choice
- Less disruptive for non-security-critical tools

**Cons:**
- Undermines determinism guarantees
- Interactive prompts don't work in CI
- Security risk if users habitually accept

#### Option 5C: Hard Failure with Recovery Path

Fail on mismatch, but provide clear recovery command (`--refresh` to regenerate plan).

**Pros:**
- Security-first (doesn't proceed with mismatched binary)
- Non-blocking (user knows how to recover)
- Works in CI (fail fast, fix forward)
- Aligns with Cargo's `--locked` behavior

**Cons:**
- Requires good error messaging

## Decision Outcome

**Chosen: 1A + 2C + 3B + 4C + 5C**

### Summary

`tsuku eval` always generates fresh plans (1A) while `tsuku install` uses cached plans for exact versions and regenerates for dynamic constraints (2C). Plans are validated against recipe hash, format version, and platform (3B). The executor delegates to an `ExecutePlan` method that handles plan execution (4C). Checksum mismatches are hard failures with clear recovery instructions (5C).

### Rationale

**Fresh evaluation (1A)** is essential because `tsuku eval` is a diagnostic command. Users run eval to see what *would* happen now, not what happened before. Golden file testing depends on this to detect upstream changes. This aligns with Terraform's `terraform plan` which always computes current state.

**Version-constraint-based caching (2C)** matches user intent. When a user specifies `ripgrep@14.1.0`, they're expressing "I want exactly this version, reproducibly." When they say `ripgrep` or `ripgrep@latest`, they're saying "give me the current latest." The caching behavior should match this intent. This is explicitly specified in the upstream strategic design.

**Multi-factor validation (3B)** prevents subtle bugs. Recipe hash alone misses format version evolution (plan format v1 vs v2) and platform mismatches. Full validation ensures cached plans are actually compatible.

**Execute delegates to ExecutePlan (4C)** provides a clean architecture where all execution goes through one code path, while maintaining backward compatibility. This naturally supports Milestone 3 (`tsuku install --plan <file>`) since `ExecutePlan` already exists.

**Hard failure with recovery (5C)** balances security with usability. Checksum mismatches indicate upstream changes—potentially malicious re-tagging. Hard failure prevents silent installation of modified binaries. The `--refresh` flag provides a clear, intentional path forward. This aligns with Cargo's strict `--locked` mode and npm's `npm ci` behavior.

### Trade-offs Accepted

By choosing version-constraint-based caching (2C), we accept:
- More complex version parsing logic
- Users must understand the version constraint taxonomy

By choosing hard failure on checksum mismatch (5C), we accept:
- Blocked installations when upstream changes
- Users must explicitly opt-in to accepting changes via `--refresh`

These trade-offs favor security and determinism over convenience, which aligns with the project's philosophy.

## Solution Architecture

### Overview

The solution refactors the installation flow to be plan-first. Every installation generates or retrieves a plan, then executes that plan. The plan is the source of truth for what gets installed.

### Component Architecture

```
                           ┌──────────────────┐
                           │   User Command   │
                           └────────┬─────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
              ▼                     ▼                     ▼
    ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
    │   tsuku eval    │   │  tsuku install  │   │ tsuku install   │
    │                 │   │                 │   │   --plan <file> │
    │ (Always fresh)  │   │ (Cache-aware)   │   │   (Milestone 3) │
    └────────┬────────┘   └────────┬────────┘   └────────┬────────┘
             │                     │                     │
             ▼                     ▼                     │
    ┌─────────────────┐   ┌─────────────────┐            │
    │ GeneratePlan()  │   │ GetOrGenerate   │            │
    │                 │   │     Plan()      │            │
    └────────┬────────┘   └────────┬────────┘            │
             │                     │                     │
             │            ┌────────┴────────┐            │
             │            │                 │            │
             │            ▼                 ▼            │
             │   ┌─────────────┐   ┌─────────────┐       │
             │   │ Cache Hit   │   │ Cache Miss  │       │
             │   │ (Validate)  │   │ (Generate)  │       │
             │   └──────┬──────┘   └──────┬──────┘       │
             │          │                 │              │
             │          └────────┬────────┘              │
             │                   │                       │
             ▼                   ▼                       ▼
    ┌─────────────────────────────────────────────────────────┐
    │                    InstallationPlan                     │
    └─────────────────────────────┬───────────────────────────┘
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  ExecutePlan()  │
                         │                 │
                         │ - Download      │
                         │ - Verify chksum │
                         │ - Extract       │
                         │ - Install bins  │
                         └────────┬────────┘
                                  │
                                  ▼
                         ┌─────────────────┐
                         │ Installed Tool  │
                         └─────────────────┘
```

### Key Data Structures

#### Plan Cache Key

```go
// PlanCacheKey uniquely identifies a cached plan
type PlanCacheKey struct {
    Tool       string `json:"tool"`
    Version    string `json:"version"`     // Resolved version (e.g., "14.1.0")
    Platform   string `json:"platform"`    // e.g., "linux-amd64"
    RecipeHash string `json:"recipe_hash"` // SHA256 of recipe TOML
}

// CacheKeyFor generates a cache key for plan lookup
func CacheKeyFor(tool, version, os, arch string, recipe *recipe.Recipe) PlanCacheKey {
    return PlanCacheKey{
        Tool:       tool,
        Version:    version,
        Platform:   fmt.Sprintf("%s-%s", os, arch),
        RecipeHash: computeRecipeHash(recipe),
    }
}
```

#### Version Constraint Classification

```go
// VersionConstraintType classifies version constraints for caching decisions
type VersionConstraintType int

const (
    // DynamicConstraint: "", "latest", "1.x", ">=1.0" - always regenerate
    DynamicConstraint VersionConstraintType = iota
    // ExactConstraint: "1.2.3", "v1.2.3" - use cached plan
    ExactConstraint
)

// ClassifyConstraint determines the constraint type
func ClassifyConstraint(constraint string) VersionConstraintType {
    if constraint == "" || constraint == "latest" {
        return DynamicConstraint
    }
    // Check for version operators or wildcards
    if strings.ContainsAny(constraint, "^~*xX><|") {
        return DynamicConstraint
    }
    // Exact versions: 1.2.3, v1.2.3
    return ExactConstraint
}
```

### Version Resolution and Cache Lookup Sequence

```
User: tsuku install ripgrep@14.1.0

1. Parse command
   └─ Tool: "ripgrep", Constraint: "14.1.0"

2. Classify constraint
   └─ "14.1.0" → ExactConstraint (no wildcards, operators)

3. Resolve version (may require network for non-exact)
   └─ ResolveVersion("14.1.0") → "14.1.0"

4. Generate cache key
   └─ {tool: "ripgrep", version: "14.1.0", platform: "linux-amd64", recipe_hash: "abc123"}

5. Query state for cached plan
   └─ state.Installed["ripgrep"].Versions["14.1.0"].Plan

6a. Cache hit + valid → ExecutePlan(cachedPlan)
6b. Cache miss/invalid → GeneratePlan() → ExecutePlan(newPlan)

7. Store plan in state (if newly generated)
```

### Plan Retrieval from State

Plans are stored inline in `state.json` within `VersionState.Plan`. The `StateManager` provides access:

```go
// GetCachedPlan retrieves a plan from state by tool and version
func (sm *StateManager) GetCachedPlan(tool, version string) (*Plan, error) {
    state, err := sm.Load()
    if err != nil {
        return nil, err
    }

    toolState, ok := state.Installed[tool]
    if !ok {
        return nil, fmt.Errorf("tool %s not installed", tool)
    }

    versionState, ok := toolState.Versions[version]
    if !ok {
        return nil, fmt.Errorf("version %s not installed", version)
    }

    if versionState.Plan == nil {
        return nil, fmt.Errorf("no plan cached for %s@%s", tool, version)
    }

    return versionState.Plan, nil
}
```

### Plan Retrieval Flow

The orchestration logic lives in `install_deps.go`, not the executor, keeping the executor focused on execution:

```go
// GetOrGeneratePlan retrieves a cached plan or generates a new one
// Note: This is orchestration logic in install_deps.go, not an Executor method
func getOrGeneratePlan(
    ctx context.Context,
    exec *executor.Executor,
    stateMgr *install.StateManager,
    cfg planRetrievalConfig,
) (*executor.InstallationPlan, error) {
    // 1. Resolve version from constraint
    version, err := exec.ResolveVersion(ctx, cfg.VersionConstraint)
    if err != nil {
        return nil, err
    }

    // 2. Check if we should use cache
    constraintType := ClassifyConstraint(cfg.VersionConstraint)
    if constraintType == ExactConstraint && !cfg.ForceRefresh {
        // 3. Try to load cached plan from state
        if cachedPlan, err := stateMgr.GetCachedPlan(cfg.Tool, version); err == nil {
            // 4. Validate cached plan
            cacheKey := CacheKeyFor(cfg.Tool, version, cfg.OS, cfg.Arch, cfg.RecipeHash)
            if err := validateCachedPlan(cachedPlan, cacheKey); err == nil {
                // Cache hit - log for user visibility
                printInfof("Using cached plan for %s@%s\n", cfg.Tool, version)
                return convertStoredPlan(cachedPlan), nil
            }
            // Invalid cache, fall through to regenerate
            printInfof("Cached plan invalid, regenerating...\n")
        }
    }

    // 5. Generate fresh plan
    return exec.GeneratePlan(ctx, executor.PlanConfig{
        OS:   cfg.OS,
        Arch: cfg.Arch,
    })
}
```

### Plan Validation

```go
// validateCachedPlan checks if a cached plan is still valid
func (e *Executor) validateCachedPlan(plan *InstallationPlan, key PlanCacheKey) error {
    // Check format version
    if plan.FormatVersion != PlanFormatVersion {
        return fmt.Errorf("plan format version %d is outdated (current: %d)",
            plan.FormatVersion, PlanFormatVersion)
    }

    // Check platform
    if plan.Platform.OS != key.Platform[:strings.Index(key.Platform, "-")] ||
       plan.Platform.Arch != key.Platform[strings.Index(key.Platform, "-")+1:] {
        return fmt.Errorf("plan platform %s-%s does not match %s",
            plan.Platform.OS, plan.Platform.Arch, key.Platform)
    }

    // Check recipe hash
    if plan.RecipeHash != key.RecipeHash {
        return fmt.Errorf("recipe has changed since plan was generated")
    }

    return nil
}
```

### ExecutePlan Implementation

```go
// ExecutePlan executes an installation plan
func (e *Executor) ExecutePlan(ctx context.Context, plan *InstallationPlan) error {
    // Create execution context
    execCtx := &ExecutionContext{
        Tool:       plan.Tool,
        Version:    plan.Version,
        WorkDir:    e.workDir,
        InstallDir: e.installDir,
    }

    // Execute each step
    for i, step := range plan.Steps {
        if err := ctx.Err(); err != nil {
            return err
        }

        // Get the action
        action := actions.Get(step.Action)
        if action == nil {
            return fmt.Errorf("unknown action: %s", step.Action)
        }

        // For download steps, verify checksum after download
        if step.Action == "download" && step.Checksum != "" {
            if err := e.executeDownloadWithVerification(ctx, execCtx, step); err != nil {
                return err
            }
            continue
        }

        // Execute other steps
        if err := action.Execute(ctx, execCtx, step.Params); err != nil {
            return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)
        }
    }

    return nil
}

// executeDownloadWithVerification downloads and verifies checksum
func (e *Executor) executeDownloadWithVerification(
    ctx context.Context,
    execCtx *ExecutionContext,
    step ResolvedStep,
) error {
    // Download the file
    action := actions.Get("download")
    if err := action.Execute(ctx, execCtx, step.Params); err != nil {
        return err
    }

    // Compute checksum of downloaded file
    dest := step.Params["dest"].(string)
    actualChecksum, err := computeFileChecksum(filepath.Join(execCtx.WorkDir, dest))
    if err != nil {
        return fmt.Errorf("failed to compute checksum: %w", err)
    }

    // Verify checksum
    if actualChecksum != step.Checksum {
        return &ChecksumMismatchError{
            URL:              step.URL,
            ExpectedChecksum: step.Checksum,
            ActualChecksum:   actualChecksum,
        }
    }

    return nil
}
```

### Checksum Mismatch Error

```go
// ChecksumMismatchError indicates upstream asset has changed
type ChecksumMismatchError struct {
    URL              string
    ExpectedChecksum string
    ActualChecksum   string
}

func (e *ChecksumMismatchError) Error() string {
    return fmt.Sprintf(`checksum mismatch for %s

Expected: %s
Got:      %s

The upstream asset has changed since the installation plan was generated.
This could indicate:
- A legitimate release update (re-tagged release)
- A supply chain attack (malicious modification)

To proceed with the new asset, regenerate the plan:
    tsuku install <tool> --refresh

To investigate, compare with upstream release notes or checksums.`,
        e.URL, e.ExpectedChecksum, e.ActualChecksum)
}
```

### Installation Flow Changes

```go
// In cmd/tsuku/install_deps.go

func installWithDependencies(ctx context.Context, toolName string, opts installOptions) error {
    // ... dependency resolution unchanged ...

    // Create executor
    exec, err := executor.NewWithVersion(recipe, opts.version, cfg)
    if err != nil {
        return err
    }

    // NEW: Get or generate plan (replaces direct execution)
    planCfg := executor.PlanConfig{
        VersionConstraint: opts.requestedVersion,
        ForceRefresh:      opts.refresh,
        OS:                runtime.GOOS,
        Arch:              runtime.GOARCH,
    }

    plan, err := exec.GetOrGeneratePlan(ctx, planCfg)
    if err != nil {
        return fmt.Errorf("failed to generate plan: %w", err)
    }

    // NEW: Execute the plan (replaces exec.Execute(ctx))
    if err := exec.ExecutePlan(ctx, plan); err != nil {
        return err
    }

    // Store plan in state (unchanged, but now stores the authoritative plan)
    installOpts := install.InstallOptions{
        Plan: convertExecutorPlan(plan),
        // ... other options ...
    }

    return mgr.InstallWithOptions(toolName, plan.Version, exec.WorkDir(), installOpts)
}
```

### Command-Line Interface

```bash
# Existing commands, unchanged behavior:
tsuku eval <tool>[@version]           # Always fresh plan generation
tsuku plan show <tool>                # Show cached plan from state
tsuku plan export <tool>              # Export cached plan to file

# Install command with new flag:
tsuku install <tool>[@version]        # Uses plan caching based on constraint
tsuku install <tool>@1.2.3            # Exact: uses cached plan if valid
tsuku install <tool>                  # Dynamic: always regenerates
tsuku install <tool> --refresh        # Force fresh plan generation
```

## Implementation Approach

### Phase 1: Core Infrastructure

1. Add `VersionConstraintType` and `ClassifyConstraint()` function
2. Add `PlanCacheKey` structure
3. Implement `validateCachedPlan()` method
4. Add `ChecksumMismatchError` type with helpful error message

### Phase 2: Executor Refactoring

1. Add `ExecutePlan(ctx, plan)` method to executor
2. Implement checksum verification in `ExecutePlan`
3. Add `GetOrGeneratePlan(ctx, cfg)` method
4. Modify `Execute(ctx)` to delegate to `GetOrGeneratePlan` + `ExecutePlan`

### Phase 3: Install Command Updates

1. Add `--refresh` flag to install command
2. Update `installWithDependencies` to use `GetOrGeneratePlan`
3. Pass `ForceRefresh` from CLI flag through to executor
4. Update error handling for `ChecksumMismatchError`

### Phase 4: Testing and Documentation

1. Unit tests for version constraint classification
2. Unit tests for plan validation
3. Integration tests for plan caching behavior
4. Integration tests for checksum verification
5. Update CLI help text and documentation

## Security Considerations

### Download Verification

**Analysis**: All downloads during plan execution verify checksums computed during plan generation. The `executeDownloadWithVerification` function computes the SHA256 checksum of downloaded files and compares against the plan's recorded checksum. Mismatches are hard failures.

**Mitigation**: Checksum verification uses streaming SHA256 computation to detect tampering. Downloads use HTTPS with certificate verification (fail-closed on TLS errors). The download cache is validated against plan checksums before use.

### Execution Isolation

**Analysis**: Plan execution runs in user space with no elevated privileges. The executor operates within the `$TSUKU_HOME` directory structure. Downloaded files are written to a temporary work directory before being moved to the final location.

**Mitigations**:
- Work directories created with mode 0700 (user-only access)
- Work directory names include unpredictable suffix (UUID) to prevent targeting
- Installed binaries set to mode 0755, data files to mode 0644
- `install_binaries` action validates symlinks point within `$TSUKU_HOME`

### Supply Chain Risks

**Analysis**: Cached plans create a window where upstream changes are not immediately reflected. This is intentional (determinism) but means users must explicitly opt-in to changes via `--refresh`. The checksum mismatch error explicitly mentions supply chain attacks as a possibility.

**Mitigations**:
- Clear error messaging when checksums mismatch
- `--refresh` flag is explicit, requiring user intent
- Recipe hash validation ensures plans match current recipes
- Format version validation prevents use of outdated plan structures

**Residual risk**: Initial plan generation inherits any existing compromise. Plans created from compromised upstream sources will have "valid" checksums for malicious content. This risk is inherent to any download-and-install tool; tsuku's checksum verification detects changes but cannot validate initial trustworthiness.

### User Data Exposure

**Analysis**: Plan caching adds version constraint information to execution context. This doesn't change what data is transmitted (same download URLs, same telemetry if enabled).

**Mitigations**:
- state.json created with mode 0600 (user read/write only)
- URLs with embedded authentication tokens should be sanitized before storage (tokens re-injected at execution time)
- No additional data exposure beyond existing installation metadata

### Future Security Enhancements

The following security improvements are explicitly out of scope for this design but should be considered for future work:

1. **Plan cache integrity**: HMAC signing of cached plans to detect tampering (requires user-specific key management)
2. **Recipe signing**: Cryptographic signatures for recipes to prevent repository compromise
3. **Upstream checksum validation**: For recipes where upstream provides checksums (GitHub releases, SHASUMS files), validate plan checksums against upstream source

## Consequences

### Positive

- **Deterministic installations**: Exact version constraints produce identical results across time and machines
- **Detects upstream changes**: Checksum mismatches surface supply chain concerns
- **Single execution path**: All installations go through `ExecutePlan`, ensuring consistency
- **Performance improvement**: Cached plans avoid redundant downloads and checksum computation
- **Foundation for Milestone 3**: `ExecutePlan` enables `tsuku install --plan <file>`

### Negative

- **Breaking change for workflows expecting fresh plans**: Scripts that reinstall expecting latest must use `--refresh`
- **Increased complexity**: Version constraint classification and cache validation add code paths
- **Blocked installations on upstream changes**: Users must explicitly acknowledge changes

### Neutral

- **Backward compatibility maintained**: Existing `state.json` files without plans continue to work (treated as cache miss)
- **eval behavior unchanged**: `tsuku eval` continues to generate fresh plans
