---
status: Superseded
problem: Installation lacks determinism—tsuku install and tsuku eval can produce different results, cached plans are not reused, and checksum mismatches are not detected during execution.
decision: Implement two-phase plan generation (version resolution + artifact verification) with plan caching by resolution output, hard-failure checksum verification, and ExecutePlan as the sole execution path.
rationale: This design ensures all installations are deterministic by default, detects supply chain changes via checksums, aligns with Nix's evaluation/realization model, and provides a clean architecture that naturally enables the --plan flag feature.
---

# Design: Deterministic Execution (Plan-Based Installation)

- **Status**: Superseded by [DESIGN-deterministic-resolution.md](../current/DESIGN-deterministic-resolution.md)
- **Milestone**: Deterministic Recipe Execution
- **Author**: @dangazineu
- **Created**: 2025-12-13
- **Scope**: Tactical
- **Archived**: 2025-12-19
- **See Also**: docs/GUIDE-plan-based-installation.md (user guide)

## Implementation Issues

### Milestone: [Deterministic Recipe Execution](https://github.com/tsukumogami/tsuku/milestone/15)

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#470](https://github.com/tsukumogami/tsuku/issues/470) | feat(executor): add plan cache infrastructure | None |
| [#471](https://github.com/tsukumogami/tsuku/issues/471) | feat(install): add GetCachedPlan to StateManager | None |
| [#472](https://github.com/tsukumogami/tsuku/issues/472) | feat(executor): expose ResolveVersion public method | None |
| [#473](https://github.com/tsukumogami/tsuku/issues/473) | feat(executor): add ExecutePlan with checksum verification | [#470](https://github.com/tsukumogami/tsuku/issues/470) |
| [#474](https://github.com/tsukumogami/tsuku/issues/474) | feat(cli): add --fresh flag to install command | None |
| [#475](https://github.com/tsukumogami/tsuku/issues/475) | feat(executor): add plan conversion helpers | [#470](https://github.com/tsukumogami/tsuku/issues/470) |
| [#477](https://github.com/tsukumogami/tsuku/issues/477) | feat(cli): implement getOrGeneratePlan orchestration | [#470](https://github.com/tsukumogami/tsuku/issues/470), [#471](https://github.com/tsukumogami/tsuku/issues/471), [#472](https://github.com/tsukumogami/tsuku/issues/472), [#474](https://github.com/tsukumogami/tsuku/issues/474), [#475](https://github.com/tsukumogami/tsuku/issues/475) |
| [#478](https://github.com/tsukumogami/tsuku/issues/478) | feat(cli): wire up plan-based installation flow | [#473](https://github.com/tsukumogami/tsuku/issues/473), [#477](https://github.com/tsukumogami/tsuku/issues/477) |
| [#479](https://github.com/tsukumogami/tsuku/issues/479) | refactor(executor): remove legacy Execute method | [#478](https://github.com/tsukumogami/tsuku/issues/478) |

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
- `--fresh` flag to force plan regeneration
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
6. **Simplicity**: Prefer clean architecture over backward compatibility (pre-1.0, no users yet)

## Considered Options

### Decision 1: Two-Phase Evaluation Model

Plan generation has two distinct phases:
1. **Version Resolution**: Maps user input → resolved version + artifact URLs
2. **Artifact Verification**: Downloads artifacts and computes checksums

The question is: which phases can be cached, and what should the cache key be?

#### Option 1A: Cache by User Input (Version Constraint)

Cache key based on what the user typed (e.g., "ripgrep@14.1.0" vs "ripgrep").

**Pros:**
- Simple mental model: exact version = cached, dynamic = fresh

**Cons:**
- Conflates user input with resolved state
- `ripgrep@14.1.0` and `ripgrep` might resolve to the same artifacts but have different cache behavior
- Requires complex version constraint classification logic

#### Option 1B: Cache by Resolution Output

Always run version resolution (Phase 1). Cache artifact verification (Phase 2) based on what resolution produces.

Cache key = hash of (tool, resolved_version, resolved_URLs, platform, recipe_hash)

**Pros:**
- Clean separation: resolution always runs, verification is cached
- Same resolved artifacts → same cache key, regardless of user input
- Simpler logic: no version constraint classification needed
- Aligns with Nix model: evaluation always runs, realization is cached
- `--fresh` flag has clear semantics: bypass artifact cache, re-verify checksums

**Cons:**
- Version resolution always incurs network cost (unless resolution itself is cached separately)

#### Option 1C: Cache Both Phases Separately

Cache version resolution results and artifact verification separately.

**Pros:**
- Maximum performance: can skip both network calls if cached

**Cons:**
- Complex cache invalidation (when does resolution cache expire?)
- Version resolution caching has different TTL requirements than artifact caching
- Over-engineered for current needs

### Decision 2: Plan Cache Invalidation

When should a cached plan be considered invalid?

#### Option 2A: Recipe Hash Only

Invalidate cached plan when recipe file hash changes.

**Pros:**
- Simple to implement
- Catches recipe modifications
- Clear invalidation signal

**Cons:**
- Doesn't detect format version changes
- Doesn't detect platform mismatches (if checking wrong cached plan)

#### Option 2B: Multi-Factor Validation

Validate recipe hash, plan format version, and platform before reusing cached plan.

**Pros:**
- Comprehensive validation
- Catches format evolution
- Prevents cross-platform confusion

**Cons:**
- More complex validation logic
- More fields to track and compare

#### Option 2C: Multi-Factor with Staleness Warning

Same as 2B, but also warn (don't fail) when plan is stale but still usable.

**Pros:**
- All benefits of 2B
- Users informed about staleness without blocking
- Graceful degradation for old state files

**Cons:**
- Most complex implementation
- Users may ignore warnings

### Decision 3: Executor Refactoring Approach

How should the executor be refactored to support plan-based execution?

#### Option 3A: Add ExecutePlan Method

Add a new `ExecutePlan(ctx, plan)` method alongside existing `Execute(ctx)`.

**Pros:**
- Non-breaking change
- Can migrate incrementally
- Clear separation of concerns

**Cons:**
- Two code paths to maintain
- Risk of divergence between Execute and ExecutePlan

#### Option 3B: Replace Execute with Plan-Based Flow

Remove `Execute(ctx)`, replace with `ExecutePlan(ctx, plan)` as the only execution method.

**Pros:**
- Single code path (all execution goes through plans)
- Guarantees architectural equivalence
- Clean architecture, no legacy path
- Enables Milestone 3 (`--plan` flag) naturally

**Cons:**
- All internal callers must provide plans (larger refactoring)

### Decision 4: Checksum Mismatch Behavior

What happens when a download's checksum doesn't match the plan?

#### Option 4A: Hard Failure

Checksum mismatch is an installation failure with clear error message.

**Pros:**
- Security-first approach
- Aligns with upstream design ("mismatch = failure")
- Forces user to investigate upstream changes
- Prevents silent installation of modified binaries

**Cons:**
- Blocks installation until user takes action
- May frustrate users who just want latest version

#### Option 4B: Warning with Proceed Option

Warn about mismatch, ask user whether to proceed.

**Pros:**
- User has choice
- Less disruptive for non-security-critical tools

**Cons:**
- Undermines determinism guarantees
- Interactive prompts don't work in CI
- Security risk if users habitually accept

#### Option 4C: Hard Failure with Recovery Path

Fail on mismatch, but provide clear recovery command (`--fresh` to re-verify artifacts).

**Pros:**
- Security-first (doesn't proceed with mismatched binary)
- Non-blocking (user knows how to recover)
- Works in CI (fail fast, fix forward)
- Aligns with Cargo's `--locked` behavior

**Cons:**
- Requires good error messaging

## Decision Outcome

**Chosen: 1B + 2B + 3B + 4C**

### Summary

Plan generation is split into two phases: version resolution (always runs) and artifact verification (cached by resolution output). Cache key is based on what resolution produces, not what the user typed (1B). Plans are validated against recipe hash, format version, and platform (2B). The executor's `Execute()` method is replaced with `ExecutePlan()` as the only execution path (3B). Checksum mismatches are hard failures with clear recovery via `--fresh` (4C).

### Rationale

**Cache by resolution output (1B)** provides the cleanest model. Version resolution always runs to map user input to concrete artifacts. Artifact verification (downloads, checksums) is cached based on what resolution produces. This means:
- `tsuku eval ripgrep` and `tsuku eval ripgrep@14.1.0` that resolve to the same version hit the same cache
- No complex version constraint classification logic needed
- `--fresh` has clear semantics: bypass artifact cache, re-verify checksums
- Aligns with Nix: evaluation always runs, realization is cached

**Multi-factor validation (2B)** prevents subtle bugs. Recipe hash alone misses format version evolution (plan format v1 vs v2) and platform mismatches. Full validation ensures cached plans are actually compatible.

**Replace Execute with ExecutePlan (3B)** provides the cleanest architecture. Since we're pre-1.0 with no users, backward compatibility isn't a concern. Removing the old `Execute()` method ensures all execution goes through plans by construction—there's no way to accidentally bypass the plan-based flow. This naturally supports Milestone 3 (`tsuku install --plan <file>`) since `ExecutePlan` is the only entry point.

**Hard failure with recovery (4C)** balances security with usability. Checksum mismatches indicate upstream changes—potentially malicious re-tagging. Hard failure prevents silent installation of modified binaries. The `--fresh` flag provides a clear, intentional path forward. This aligns with Cargo's strict `--locked` mode and npm's `npm ci` behavior.

### Trade-offs Accepted

By choosing cache by resolution output (1B), we accept:
- Version resolution always incurs network cost (GitHub API calls, etc.)
- This is acceptable because resolution is fast and the real cost is in downloads

By choosing to replace Execute (3B), we accept:
- All internal callers must generate plans before execution
- Larger initial refactoring effort

By choosing hard failure on checksum mismatch (4C), we accept:
- Blocked installations when upstream changes
- Users must explicitly opt-in to accepting changes via `--fresh`

These trade-offs favor simplicity, security, and determinism, which aligns with the project's philosophy.

## Solution Architecture

### Overview

The solution refactors the installation flow into two distinct phases:

1. **Phase 1 - Version Resolution**: Always runs. Maps user input to a resolved version.
2. **Phase 2 - Artifact Verification**: Cached by resolution output. Downloads and computes checksums.

The cache key is based on what Phase 1 produces, not what the user typed. This means `tsuku eval ripgrep` and `tsuku eval ripgrep@14.1.0` that resolve to the same version share the same cache.

**Download cache reuse**: Phase 2 downloads artifacts to `$TSUKU_HOME/cache/downloads/` to compute checksums. When `ExecutePlan()` runs, the download action checks this cache first—if the file exists and the checksum matches the plan, the download is skipped. This avoids re-downloading artifacts that were just verified during plan generation.

### Component Architecture

```
                           ┌──────────────────┐
                           │   User Command   │
                           │ (any constraint) │
                           └────────┬─────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Phase 1: Version Resolution  │
                    │         (always runs)         │
                    │                               │
                    │  "ripgrep" → "14.1.0"         │
                    │  "ripgrep@14.1.0" → "14.1.0"  │
                    │  "ripgrep@latest" → "14.1.0"  │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                         ┌──────────────────┐
                         │   Cache Lookup   │
                         │ key = (tool,     │
                         │  resolved_ver,   │
                         │  platform,       │
                         │  recipe_hash)    │
                         └────────┬─────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
                    ▼                           ▼
          ┌─────────────────┐         ┌─────────────────┐
          │   Cache Hit     │         │  Cache Miss     │
          │                 │         │  (or --fresh)   │
          └────────┬────────┘         └────────┬────────┘
                   │                           │
                   │                           ▼
                   │              ┌─────────────────────────┐
                   │              │ Phase 2: Artifact       │
                   │              │ Verification            │
                   │              │ - Expand URL templates  │
                   │              │ - Download artifacts    │
                   │              │ - Compute checksums     │
                   │              └────────────┬────────────┘
                   │                           │
                   └─────────────┬─────────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │ InstallationPlan│
                        │ (URLs+checksums)│
                        └────────┬────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  ExecutePlan()  │
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

The cache key is based on the OUTPUT of version resolution, not the user's input:

```go
// PlanCacheKey uniquely identifies a cached plan
// Key insight: based on resolution output, not user input
type PlanCacheKey struct {
    Tool       string `json:"tool"`
    Version    string `json:"version"`     // RESOLVED version (e.g., "14.1.0")
    Platform   string `json:"platform"`    // e.g., "linux-amd64"
    RecipeHash string `json:"recipe_hash"` // SHA256 of recipe TOML
}

// CacheKeyFor generates a cache key AFTER version resolution
func CacheKeyFor(tool, resolvedVersion, os, arch string, recipe *recipe.Recipe) PlanCacheKey {
    return PlanCacheKey{
        Tool:       tool,
        Version:    resolvedVersion,
        Platform:   fmt.Sprintf("%s-%s", os, arch),
        RecipeHash: computeRecipeHash(recipe),
    }
}
```

### Two-Phase Evaluation Flow

```
User: tsuku install ripgrep@14.1.0
      tsuku install ripgrep          (both may hit same cache!)
      tsuku eval ripgrep

┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: Version Resolution (ALWAYS runs)                       │
│                                                                 │
│ Input:  User constraint ("ripgrep", "ripgrep@14.1.0", etc.)     │
│ Output: VersionInfo { Version: "14.1.0", Tag: "v14.1.0" }       │
│                                                                 │
│ - Queries GitHub/PyPI/npm API                                   │
│ - Resolves "latest", version ranges, exact versions             │
│ - Fast (API call only, no downloads)                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Cache Lookup                                                    │
│                                                                 │
│ Key = (tool: "ripgrep", version: "14.1.0",                      │
│        platform: "linux-amd64", recipe_hash: "abc123")          │
│                                                                 │
│ - Same key regardless of user input ("ripgrep" or "@14.1.0")    │
│ - Lookup in state.Installed[tool].Versions[version].Plan       │
│ - Validate: recipe_hash, format_version, platform match         │
│ - If --fresh flag: skip cache, force Phase 2                    │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              │                               │
         Cache Hit                       Cache Miss
              │                               │
              │                               ▼
              │         ┌─────────────────────────────────────────┐
              │         │ Phase 2: Artifact Verification          │
              │         │                                         │
              │         │ - Expand URL templates with version     │
              │         │ - Download artifacts to compute SHA256  │
              │         │ - Build InstallationPlan with checksums │
              │         │                                         │
              │         │ Slow (network downloads)                │
              │         └─────────────────────────────────────────┘
              │                               │
              └───────────────┬───────────────┘
                              │
                              ▼
                      InstallationPlan
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
// getOrGeneratePlan implements the two-phase model
// Phase 1 (resolution) always runs; Phase 2 (verification) is cached
func getOrGeneratePlan(
    ctx context.Context,
    exec *executor.Executor,
    stateMgr *install.StateManager,
    cfg planRetrievalConfig,
) (*executor.InstallationPlan, error) {
    // Phase 1: Version Resolution (ALWAYS runs)
    version, err := exec.ResolveVersion(ctx, cfg.VersionConstraint)
    if err != nil {
        return nil, err
    }

    // Generate cache key from resolution output
    cacheKey := CacheKeyFor(cfg.Tool, version, cfg.OS, cfg.Arch, cfg.RecipeHash)

    // Check cache (unless --fresh)
    if !cfg.Fresh {
        if cachedPlan, err := stateMgr.GetCachedPlan(cfg.Tool, version); err == nil {
            if err := validateCachedPlan(cachedPlan, cacheKey); err == nil {
                printInfof("Using cached plan for %s@%s\n", cfg.Tool, version)
                return convertStoredPlan(cachedPlan), nil
            }
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
    tsuku install <tool> --fresh

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
tsuku install <tool> --fresh          # Force fresh plan generation
```

## Implementation Approach

The implementation is organized into parallel tracks that minimize file conflicts. Each issue is independently testable. The old `Execute()` method remains functional until the new flow is fully wired up, then removed in a cleanup ticket.

### File Hotpath Analysis

| File | Modifications | Track |
|------|---------------|-------|
| `internal/executor/plan_cache.go` (new) | PlanCacheKey, validateCachedPlan, ChecksumMismatchError | A |
| `internal/install/state.go` | GetCachedPlan() | B |
| `internal/executor/executor.go` | ResolveVersion(), ExecutePlan() | C |
| `cmd/tsuku/install.go` | --fresh flag | D |
| `cmd/tsuku/install_deps.go` | getOrGeneratePlan(), wire up flow | D |

### Track A: Plan Cache Types (new file, no conflicts)

**A1: Add plan cache infrastructure**
- Create `internal/executor/plan_cache.go` with:
  - `PlanCacheKey` structure
  - `CacheKeyFor()` helper function
  - `validateCachedPlan()` function
  - `ChecksumMismatchError` type with helpful error message
- Unit tests in `plan_cache_test.go`
- **Dependencies**: None
- **Testable**: Yes (pure functions, no integration needed)

### Track B: State Manager (isolated file)

**B1: Add GetCachedPlan to StateManager**
- Add `GetCachedPlan(tool, version)` method to `internal/install/state.go`
- Returns stored plan from `state.Installed[tool].Versions[version].Plan`
- Unit tests for cache hit/miss scenarios
- **Dependencies**: None
- **Testable**: Yes (mock state file)

### Track C: Executor Methods (sequential within track)

**C1: Add ResolveVersion public method**
- Expose version resolution as `ResolveVersion(ctx, constraint) (string, error)`
- Wraps existing internal resolution logic
- Unit tests
- **Dependencies**: None
- **Testable**: Yes (existing version provider mocks)

**C2: Add ExecutePlan method**
- Add `ExecutePlan(ctx, plan) error` method
- Implement checksum verification for download steps
- Use `ChecksumMismatchError` from Track A
- Keep existing `Execute()` working (no removal yet)
- Unit tests with mock plans
- **Dependencies**: A1 (ChecksumMismatchError type)
- **Testable**: Yes (can test ExecutePlan independently)

### Track D: Install Command (sequential within track)

**D1: Add --fresh flag**
- Add `--fresh` flag to `cmd/tsuku/install.go`
- Pass through to install options (no behavior change yet)
- **Dependencies**: None
- **Testable**: Yes (flag parsing tests)

**D2: Implement getOrGeneratePlan**
- Add `getOrGeneratePlan()` function to `cmd/tsuku/install_deps.go`
- Implements two-phase flow: resolve → cache lookup → generate if needed
- Uses Track A, B, C components
- Unit tests with mocked dependencies
- **Dependencies**: A1, B1, C1
- **Testable**: Yes (mock executor and state manager)

**D3: Wire up new installation flow**
- Update `installWithDependencies()` to use `getOrGeneratePlan()` + `ExecutePlan()`
- Handle `ChecksumMismatchError` with user-friendly output
- Integration tests for full flow
- **Dependencies**: C2, D1, D2
- **Testable**: Yes (integration tests)

### Track E: Cleanup (after D3 verified)

**E1: Remove legacy Execute method**
- Remove `Execute(ctx)` from executor
- Remove any code paths that bypass plan generation
- Update any remaining callers
- **Dependencies**: D3 fully tested and deployed
- **Testable**: Yes (ensure no regressions)

### Dependency Graph

```
A1 ─────────────────────────────┐
                                │
B1 ─────────────────────────────┼──→ D2 ──→ D3 ──→ E1
                                │         ↗
C1 ─────────────────────────────┤        /
                                │       /
C2 (depends on A1) ─────────────┘──────/

D1 ─────────────────────────────────→ D3
```

### Parallelization Strategy

**Can run in parallel (no file conflicts):**
- A1, B1, C1, D1 (all touch different files)

**Must be sequential:**
- C2 after A1 (needs ChecksumMismatchError)
- D2 after A1, B1, C1 (needs all infrastructure)
- D3 after C2, D1, D2 (integration point)
- E1 after D3 (cleanup after verification)

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

**Analysis**: Cached plans create a window where upstream changes are not immediately reflected. This is intentional (determinism) but means users must explicitly opt-in to changes via `--fresh`. The checksum mismatch error explicitly mentions supply chain attacks as a possibility.

**Mitigations**:
- Clear error messaging when checksums mismatch
- `--fresh` flag is explicit, requiring user intent
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
- **Single execution path**: All installations go through `ExecutePlan`, ensuring consistency by construction
- **Clean architecture**: No legacy code path to maintain; plan-based flow is the only way to install
- **Performance improvement**: Cached plans avoid redundant downloads and checksum computation
- **Foundation for Milestone 3**: `ExecutePlan` enables `tsuku install --plan <file>`

### Negative

- **Increased complexity**: Cache validation and two-phase evaluation add code paths
- **Blocked installations on upstream changes**: Users must explicitly acknowledge changes via `--fresh`
- **Internal refactoring**: All code paths through the executor must go through plan generation first

### Neutral

- **eval behavior unchanged**: `tsuku eval` continues to generate fresh plans
- **Existing state files without plans**: Treated as cache miss, triggers fresh plan generation
