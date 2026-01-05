# Sandbox Testing Doesn't Include Dependency Installation

## Status

Proposed

## Context and Problem Statement

When users run `tsuku install <tool> --sandbox` for tools with declared recipe dependencies, the sandbox test fails because dependencies aren't installed in the container. This prevents validation of tools that require library dependencies like openssl or zlib.

For example, installing curl (which depends on openssl and zlib) in sandbox mode fails:

```
$ tsuku install curl --sandbox

Running sandbox test for curl...
  Container image: ubuntu:22.04
  Network access: enabled (ecosystem build)
  Resource limits: 4g memory, 4 CPUs, 15m0s timeout

Sandbox test FAILED
...
configure: error: --with-openssl was given but OpenSSL could not be detected
```

The generated plan shows:
```
Total steps: 6 (including 0 from dependencies)
```

**Root cause**: `cmd/tsuku/install_sandbox.go` has its own `generatePlanFromRecipe()` function (lines 159-192) that **duplicates** the plan generation logic from `install_deps.go`. This duplicate implementation doesn't pass `RecipeLoader`, so dependencies aren't embedded in plans.

**Architectural issue**: This violates the principle established in PR #808 that plan generation should use a unified code path. The sandbox executor already runs `tsuku install --plan plan.json` inside the container (executor.go:354), which uses the same execution code as normal install. There's no reason to duplicate plan generation on the host side.

PR #808 fixed plan generation in `install_deps.go` (normal install mode), but the duplicated code in `install_sandbox.go` was left unfixed, creating two divergent implementations.

### Current State

**Working scenarios:**
- `tsuku install curl` (normal mode) - handles dependencies correctly
- `tsuku install biome --sandbox` - simple binary installations work
- Source builds without dependencies work in sandbox mode

**Broken scenario:**
- `tsuku install curl --sandbox` - tools with declared dependencies fail

### Why This Matters Now

- PR #808 enabled implicit dependencies (build tools), unblocking issue #806
- Issue #806 expands multi-family sandbox CI testing to all 5 Linux families
- **Issue #806 is blocked** by #703 because complex tools need dependencies
- Without fixing this, we cannot validate the multi-family infrastructure with real-world tools

### Scope

**In scope:**
- Include declared recipe dependencies in sandbox plans (from recipe `dependencies` field)
- Support transitive dependencies (dependencies of dependencies, resolved recursively)
- Generate self-contained plans that work in isolated containers
- Unify sandbox plan generation with normal install path (use same RecipeLoader mechanism)
- Handle eval-time dependencies (installed on host during plan generation, e.g., python-standalone for pipx_install)

**Out of scope:**
- Changes to the dependency resolution logic (already works in normal install)
- New dependency types beyond `dependencies` field and eval-time deps
- Platform-specific dependency handling (already implemented in PR #808)
- Changes to the sandbox execution environment
- Changes to container resource limits or network policies

## Decision Drivers

- **Consistency**: Sandbox mode should behave like normal install mode for dependencies
- **Self-contained plans**: Plans must be executable without host-side dependency installation
- **Reuse existing infrastructure**: PR #808 already enabled RecipeLoader for install_deps.go
- **Minimal changes**: Apply the same proven pattern from PR #808
- **Backward compatibility**: Don't break existing sandbox tests for simple tools
- **CI unblocking**: Must enable issue #806 (multi-family sandbox testing)

## Implementation Context

### Upstream Design Reference

This design implements the same dependency embedding pattern from PR #808 (issue #805), which fixed implicit action dependencies in normal install mode. PR #808 enabled `RecipeLoader` in `cmd/tsuku/install_deps.go`, making plans self-contained.

Issue #703 extends this fix to sandbox mode by enabling `RecipeLoader` in `cmd/tsuku/install_sandbox.go`.

**Relationship**:
- PR #808: Fixed implicit dependencies (cmake, make) in `tsuku install`
- Issue #703: Fixes explicit dependencies (openssl, zlib) in `tsuku install --sandbox`
- Issue #806: Requires both fixes to test complex tools across 5 Linux families

### Existing Patterns

**Proven pattern from PR #808 (install_deps.go:363-373):**

```go
plan, err := generator.GeneratePlan(ctx, executor.PlanConfig{
    OS:                 targetOS,
    Arch:               targetArch,
    RecipeSource:       "registry",
    Downloader:         downloader,
    DownloadCache:      downloadCache,
    RecipeLoader:       cfg.RecipeLoader,  // ← KEY: Enables dependency embedding
    AutoAcceptEvalDeps: true,
    OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
        return installEvalDeps(deps, autoAccept)
    },
})
```

**Current broken pattern (install_sandbox.go:180-186):**

```go
plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
    OS:            runtime.GOOS,
    Arch:          runtime.GOARCH,
    RecipeSource:  "local",
    Downloader:    downloader,
    DownloadCache: downloadCache,
    // ← MISSING: RecipeLoader not passed
})
```

**Available resources:**
- Global `loader` variable (defined in `cmd/tsuku/helpers.go:13`) is available in all cmd/tsuku files
- `installEvalDeps()` function exists in install_deps.go and can be reused or adapted

**Conventions to follow:**
- Use `executor.PlanConfig` struct with all necessary fields
- Pass `RecipeLoader: loader` to enable dependency embedding
- Add `AutoAcceptEvalDeps: true` for automatic eval-time dependency installation
- Provide `OnEvalDepsNeeded` callback for dependency installation

**Anti-patterns to avoid:**
- Passing `RecipeLoader: nil` (disables dependency embedding)
- Installing dependencies on host before plan generation (creates non-portable plans)
- Using `SetResolvedDeps()` workaround (was removed in PR #808)

### Research Summary

**Upstream constraints:**
- Must follow the pattern established in PR #808
- Plans must be self-contained and portable
- Must support both explicit and implicit dependencies

**Patterns to follow:**
- Pass `RecipeLoader` to `GeneratePlan()` (proven in PR #808)
- Use `AutoAcceptEvalDeps: true` for sandbox mode (non-interactive)
- Provide `OnEvalDepsNeeded` callback for eval-time dependency installation

**Specifications to comply with:**
- Format v3 plan structure (dependency tree in JSON)
- Executor platform validation (from PR #808)

**Implementation approach:**
Apply the same pattern from install_deps.go to install_sandbox.go by adding the missing RecipeLoader field and callback. This is a minimal, proven fix that reuses existing infrastructure.

## Considered Options

### Option 1: Eliminate duplication - reuse install_deps.go plan generation

**Remove** the duplicated `generatePlanFromRecipe()` function from `install_sandbox.go` and reuse the plan generation code from `install_deps.go` instead.

**Changes required:**
1. Extract plan generation logic from `install_deps.go` into a shared function in `helpers.go`:
   ```go
   func generateInstallPlan(toolName, version string, cfg *config.Config) (*executor.InstallationPlan, error)
   ```
2. Update `install_deps.go` to call the shared function
3. Update `install_sandbox.go` to call the shared function
4. **Delete** `generatePlanFromRecipe()` from `install_sandbox.go` (lines 159-192)

**Pros:**
- **Eliminates code duplication entirely** (~50 lines deleted)
- True unification - sandbox and normal install use **identical** plan generation
- Bug fixes in plan generation automatically apply to both modes
- RecipeLoader fix from PR #808 automatically works for sandbox
- Simpler mental model: one code path, not two
- Reduces maintenance burden (changes only needed in one place)

**Cons:**
- Requires careful refactoring to extract shared logic
- Need to handle both tool-from-registry and recipe-from-file cases in shared function
- More invasive change than quick fix (~100 lines touched across 3 files)

### Option 2: Band-aid fix - add RecipeLoader to duplicated code

Keep the duplicated `generatePlanFromRecipe()` function and add `RecipeLoader` to it (similar to PR #808 fix for `install_deps.go`).

**Changes required:**
Add ~5 lines to `generatePlanFromRecipe()` in `install_sandbox.go`:
```go
plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
    // ... existing fields ...
    RecipeLoader:       loader,              // ← Add
    AutoAcceptEvalDeps: true,                // ← Add
    OnEvalDepsNeeded:   installEvalDeps,     // ← Add
})
```

**Pros:**
- Minimal immediate change (~5 lines added)
- Low risk - proven pattern from PR #808
- Quick fix to unblock issue #806

**Cons:**
- **Perpetuates code duplication** (doesn't fix architectural problem)
- Still two separate code paths that can drift
- Future bugs must be fixed in two places
- Violates DRY principle
- Contradicts the unified code path goal from PR #808

### Option 3: Extract to shared helper, keep wrapper functions

Extract the core plan generation logic to a shared helper, but keep thin wrapper functions in both files for their specific use cases.

**Changes required:**
1. Create `generatePlanFromRecipeInternal()` helper in `helpers.go`
2. Keep both `install_deps.go` and `install_sandbox.go` wrappers
3. Wrappers handle file-specific setup, call shared function

**Pros:**
- Shared core logic (reduces duplication)
- Allows file-specific customization in wrappers
- Lower risk than full extraction

**Cons:**
- Still maintains two entry points (partial duplication)
- Unclear where logic belongs (helper vs wrapper)
- More complex than Option 1 with marginal benefit

## Evaluation Against Decision Drivers

| Option | Consistency | Reuse Infrastructure | Minimal Changes | Code Simplification | Maintainability |
|--------|-------------|----------------------|-----------------|---------------------|-----------------|
| **Option 1** | ✅ Perfect | ✅ Perfect | ⚠️ Medium | ✅ Excellent | ✅ Excellent |
| **Option 2** | ✅ Good | ✅ Good | ✅ Minimal | ❌ Poor | ❌ Poor |
| **Option 3** | ✅ Good | ✅ Good | ⚠️ Medium | ⚠️ Fair | ⚠️ Fair |

**Option 1** best satisfies architectural goals:
- **Consistency**: Perfect - literally the same code path
- **Reuse infrastructure**: Perfect - eliminates all duplication
- **Code simplification**: Removes ~50 lines of duplicate code
- **Maintainability**: Future fixes only needed once

**Option 2** is a band-aid:
- Fixes immediate symptom but not root cause
- Perpetuates technical debt
- Contradicts architectural goals from PR #808

**Option 3** is a compromise:
- Better than Option 2 but not as clean as Option 1
- Adds complexity with wrappers and shared functions

## Decision Outcome

**Chosen option: Option 1 - Eliminate duplication by reusing install_deps.go plan generation**

This option removes the duplicated plan generation code entirely, achieving true unification of the install code paths. Sandbox and normal install modes will use **identical** plan generation logic, eliminating drift and reducing maintenance burden.

### Rationale

This option was chosen because:

- **Architectural correctness**: The sandbox executor already runs `tsuku install --plan` inside the container (using the same execution code path). There's no architectural reason to have different plan generation code paths on the host side.
- **Code simplification**: Removes ~50 lines of duplicate code. Net reduction in codebase complexity.
- **Automatic bug fixes**: The RecipeLoader fix from PR #808 automatically works for sandbox mode without any additional changes.
- **Future-proof**: Any improvements to plan generation (better dependency resolution, caching, etc.) automatically benefit both modes.
- **Maintainability**: Changes to plan generation logic only need to be made once, not twice.
- **Consistency with PR #808 goals**: PR #808 established that plan generation should use unified infrastructure. This completes that vision.

Alternatives were rejected because:

- **Option 2 (band-aid fix)**: Perpetuates code duplication and technical debt. While it's faster to implement (~5 lines), it violates the DRY principle and contradicts the architectural goals from PR #808. Future bugs must be fixed in two places.
- **Option 3 (partial extraction)**: Adds complexity with wrappers around shared functions without significant benefit over full unification. Unclear where logic should live (wrapper vs helper).

### Trade-offs Accepted

By choosing this option, we accept:

- **Refactoring complexity**: Requires extracting plan generation logic to a shared function and updating both call sites. More invasive than a quick fix (~100 lines touched across 3 files vs ~5 lines for band-aid).
- **Testing burden**: Need to verify both normal install and sandbox modes still work correctly after refactoring. Regression risk exists if extraction is done incorrectly.
- **Implementation time**: Takes longer to implement than Option 2 band-aid fix (estimated 2-3 hours vs 30 minutes).

These trade-offs are acceptable because:

- **One-time cost, permanent benefit**: Refactoring takes longer now but eliminates ongoing maintenance burden forever.
- **Reduced long-term risk**: Having one implementation reduces the chance of bugs compared to maintaining two divergent implementations.
- **Testing is necessary anyway**: Issue #806 requires comprehensive sandbox testing, which validates the refactoring.
- **Pays off technical debt**: Eliminates duplication that already exists, rather than adding more.

## Solution Architecture

### Overview

The solution eliminates code duplication by extracting plan generation logic into a shared function that both `install_deps.go` and `install_sandbox.go` can use. The duplicated `generatePlanFromRecipe()` function in `install_sandbox.go` is deleted entirely.

**Key insight**: The sandbox executor already runs `tsuku install --plan plan.json` inside the container (internal/sandbox/executor.go:354), which uses the same execution code path as normal install. There's no reason to have different plan generation code paths on the host side.

### Unified Architecture

**Before (current state - duplicated code paths):**
```
Normal install:           Sandbox install:
  install_deps.go           install_sandbox.go
  ↓                         ↓
  getOrGeneratePlan()       generatePlanFromRecipe()  ← DUPLICATE!
  ↓                         ↓
  GeneratePlan()            GeneratePlan()
    + RecipeLoader            - RecipeLoader (MISSING!)
```

**After (unified code path):**
```
Normal install:                Sandbox install:
  install_deps.go                install_sandbox.go
  ↓                              ↓
  generateInstallPlan() ────────→ generateInstallPlan()  ← SHARED!
    (in helpers.go)
  ↓
  GeneratePlan() with RecipeLoader (used by both)
```

### Components

**New shared function** in `cmd/tsuku/helpers.go`:

```go
// generateInstallPlan generates an installation plan for a tool.
// It handles both tool-from-registry and recipe-from-file cases.
func generateInstallPlan(
    ctx context.Context,
    toolName string,
    version string,  // empty string means latest
    recipePath string,  // empty string means load from registry
    cfg *config.Config,
) (*executor.InstallationPlan, error) {
    // 1. Load recipe (from registry or file)
    // 2. Create executor
    // 3. Set up downloader and cache
    // 4. Call GeneratePlan() with RecipeLoader
    // 5. Return plan
}
```

**Modified files**:
- `cmd/tsuku/helpers.go`: New shared function (`generateInstallPlan`)
- `cmd/tsuku/install_deps.go`: Replace inline logic with call to shared function
- `cmd/tsuku/install_sandbox.go`: **Delete** `generatePlanFromRecipe()`, call shared function instead

**Code reduction**: ~50 lines deleted (duplicate function), ~60 lines added (shared function), net +10 lines but eliminates duplication

### Data Flow

```
User: tsuku install curl --sandbox
  ↓
runSandboxInstall()
  ↓
generateInstallPlan("curl", "", "", cfg)  ← Shared function
  ├─> Load curl recipe from registry
  ├─> Create executor
  ├─> GeneratePlan() with RecipeLoader
  │   ├─> Resolve curl recipe dependencies: [openssl, zlib]
  │   ├─> Recursively resolve transitive dependencies
  │   ├─> For each dependency:
  │   │   ├─> Load dependency recipe via RecipeLoader
  │   │   ├─> Generate dependency plan
  │   │   └─> Add to plan.Dependencies
  │   ├─> OnEvalDepsNeeded fires if pipx_install/etc found
  │   │   └─> installEvalDeps([python-standalone])
  │   └─> Returns plan with embedded dependencies
  ↓
sandbox.Executor.Sandbox(plan)
  ├─> Write plan.json to workspace
  ├─> Create container with tsuku binary mounted
  ├─> Run: tsuku install --plan /workspace/plan.json
  │   ├─> Execute dependency steps (openssl, zlib)
  │   └─> Execute main tool steps (curl)
  └─> Return success/failure
```

### Benefits of Unified Architecture

1. **Bug fixes propagate automatically**: RecipeLoader fix from PR #808 works for sandbox without changes
2. **Single source of truth**: Plan generation logic exists in exactly one place
3. **Easier testing**: Test suite only needs to validate one implementation
4. **Simpler mental model**: Developers don't need to understand two different code paths
5. **Reduced cognitive load**: Changes to plan generation don't require thinking about two implementations

## Implementation Approach

This refactoring extracts plan generation logic to a shared function, eliminating the duplication between `install_deps.go` and `install_sandbox.go`.

### Step 1: Create shared plan generation function

Create `generateInstallPlan()` in `cmd/tsuku/helpers.go`:

```go
// generateInstallPlan generates an installation plan for a tool.
// Handles both tool-from-registry and recipe-from-file cases.
func generateInstallPlan(
    ctx context.Context,
    toolName string,
    version string,      // empty = latest
    recipePath string,   // empty = load from registry
    cfg *config.Config,
) (*executor.InstallationPlan, error) {
    var r *recipe.Recipe
    var err error

    // Load recipe from registry or file
    if recipePath != "" {
        r, err = recipe.ParseFile(recipePath)
    } else {
        r, err = loader.Get(toolName)
    }
    if err != nil {
        return nil, err
    }

    // Create executor
    exec, err := executor.New(r)
    if err != nil {
        return nil, err
    }
    defer exec.Cleanup()

    // Configure executor
    exec.SetToolsDir(cfg.ToolsDir)
    exec.SetDownloadCacheDir(cfg.DownloadCacheDir)
    exec.SetKeyCacheDir(cfg.KeyCacheDir)

    // Set up downloader and cache
    predownloader := validate.NewPreDownloader()
    downloader := validate.NewPreDownloaderAdapter(predownloader)
    downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)

    // Generate plan with RecipeLoader (enables dependencies)
    return exec.GeneratePlan(ctx, executor.PlanConfig{
        OS:                 runtime.GOOS,
        Arch:               runtime.GOARCH,
        RecipeSource:       "registry",  // or "local" based on recipePath
        Downloader:         downloader,
        DownloadCache:      downloadCache,
        RecipeLoader:       loader,
        AutoAcceptEvalDeps: true,
        OnEvalDepsNeeded:   installEvalDeps,  // Shared callback
    })
}
```

**Files modified**: `cmd/tsuku/helpers.go` (~60 lines added)

### Step 2: Extract installEvalDeps to helpers.go

Move `installEvalDeps()` from `install_deps.go` to `helpers.go` so it can be called from the shared function.

**Files modified**:
- `cmd/tsuku/helpers.go` (~30 lines added - moved from install_deps.go)
- `cmd/tsuku/install_deps.go` (~30 lines removed - function moved out)

### Step 3: Update install_deps.go to use shared function

Replace plan generation logic in `install_deps.go` with calls to shared function.

**Before** (lines ~335-390 in getOrGeneratePlanWith):
```go
// Complex logic to create executor, set up downloader, call GeneratePlan...
```

**After**:
```go
// Use shared plan generation
plan, err := generateInstallPlan(ctx, toolName, version, "", cfg)
```

**Files modified**: `cmd/tsuku/install_deps.go` (~50 lines removed, ~5 lines added)

### Step 4: Update install_sandbox.go to use shared function

**Delete** `generatePlanFromRecipe()` entirely and use shared function instead.

**Before** (lines 159-192):
```go
func generatePlanFromRecipe(...) (*executor.InstallationPlan, error) {
    // Duplicated logic...
}
```

**After** (deleted, replaced with call to shared function in runSandboxInstall):
```go
// Use shared plan generation
plan, err = generateInstallPlan(globalCtx, toolName, "", recipePath, cfg)
```

**Files modified**: `cmd/tsuku/install_sandbox.go` (~34 lines deleted from generatePlanFromRecipe, ~5 lines modified in runSandboxInstall)

### Step 5: Test both normal and sandbox install paths

Verify no regressions in either mode:

```bash
# Build
go build -o tsuku ./cmd/tsuku

# Test normal install (should still work)
./tsuku install curl
./tsuku list | grep curl

# Test sandbox install with dependencies (should now work!)
./tsuku install curl --sandbox

# Test tools with transitive dependencies
./tsuku install sqlite --sandbox

# Test tools with eval-time dependencies
./tsuku install pipx --sandbox
```

**Expected behavior**:
- Normal install works exactly as before
- Sandbox install now includes dependencies in plans
- Dependencies installed correctly in both modes

### Step 6: Run full test suite

Ensure no regressions:

```bash
go test ./...
golangci-lint run --timeout=5m ./...
```

### Implementation Summary

**Net code changes**:
- `helpers.go`: +90 lines (new shared function + moved installEvalDeps)
- `install_deps.go`: -75 lines (removed inline logic + moved installEvalDeps)
- `install_sandbox.go`: -29 lines (deleted duplicate function)
- **Total**: +90 -104 = **-14 lines** (net code reduction)

**Complexity**: Medium refactoring, but well-scoped and testable at each step.

## Consequences

### Positive

- **Unblocks issue #806**: Multi-family sandbox CI testing can now use complex tools with dependencies
- **Code simplification**: Removes need for any sandbox-specific dependency handling workarounds
- **Plan portability**: Sandbox plans are now truly self-contained and can be executed on different machines
- **Consistency**: Sandbox mode behaves identically to normal install mode for dependencies
- **Future-proof**: Any improvements to dependency resolution automatically apply to sandbox mode
- **Minimal maintenance**: Reuses existing, tested infrastructure from PR #808

### Negative

- **Eval-time dependency installation**: Tools like python-standalone may need to be installed on host during plan generation
- **Plan generation time**: Slight increase (<1s per dependency) due to dependency resolution
- **Code duplication**: Need to either extract `installEvalDeps()` to shared location or duplicate the logic

### Mitigations

- **Eval-time deps acceptable**: This is required for correct plan generation and matches normal install behavior
- **Plan caching**: Generated plans can be reused, so generation overhead is one-time
- **Extract to shared helper**: Move `installEvalDeps()` to `cmd/tsuku/helpers.go` to eliminate duplication and improve code organization

## Security Considerations

### Download Verification

**Impact**: This feature extends the existing download verification system to sandbox mode by enabling dependency resolution.

**Current verification mechanisms** (unchanged by this feature):
- All downloads use checksums from recipes (sha256)
- Downloads are cached and verified before use
- Verification failures abort installation
- Format v3 plans include checksums for all downloaded artifacts

**Changes from this design**:
- Dependencies are now included in sandbox plans, extending verification to dependency downloads
- Dependency downloads use the same verification mechanisms as the main tool
- No weakening of verification; actually improves security by ensuring dependencies are verified in sandbox mode

**Assessment**: This feature improves security posture by ensuring all dependencies go through the same verification process in sandbox mode that they already use in normal mode.

### Execution Isolation

**Impact**: This feature affects how dependencies are installed during sandbox plan generation and execution.

**⚠️ IMPORTANT SECURITY CAVEAT**: Sandbox mode is designed to test recipe correctness across platforms, not to provide security isolation from malicious code. Eval-time dependencies execute on the host with full user permissions during plan generation. **Only use sandbox mode with trusted recipes from the tsuku registry.**

**Execution contexts**:

1. **Plan generation (host)** - **Not isolated**:
   - Eval-time dependencies (e.g., python-standalone for pipx_install) installed to `$TSUKU_HOME/tools` on host
   - Runs with user permissions (no sudo required)
   - **Full access to user files, credentials, and network**
   - Same installation flow as normal install mode (not a new risk)
   - **Critical**: This is architectural limitation of tsuku's plan generation design

2. **Plan execution (container)** - **Isolated**:
   - All tool and library dependencies installed inside container
   - Container runs with limited privileges (no host access beyond mounted cache)
   - Existing sandbox isolation unchanged
   - Network access controlled by sandbox requirements

**Permission requirements**:
- No new permissions required
- No sudo or elevated privileges needed
- File system access limited to `$TSUKU_HOME` (host) and container filesystem (sandbox)

**Privilege escalation risks**: None introduced beyond existing eval-time dependency model. Dependencies install with same user permissions as main tool installation.

**Download cache security** (identified risk):
- Download cache directory is mounted read-write in container
- Malicious dependency could poison cached files for future installations
- **Mitigation**: Use separate cache directory for sandbox mode (future enhancement)
- **Current risk**: Acceptable for trusted recipes; document limitation

**Container runtime security** (identified gap):
- No specification of container security requirements (AppArmor, SELinux, user namespaces)
- Container breakout vulnerabilities could allow sandbox escape
- **Mitigation**: Document recommended container runtime security practices
- **Current risk**: Depends on user's container runtime configuration

**Assessment**: This feature inherits the existing limitation that plan generation is not isolated. Sandbox mode provides platform validation, not malicious code isolation. The security model requires trust in recipes being tested.

### Supply Chain Risks

**Impact**: This feature increases the number of artifacts downloaded when using sandbox mode with tools that have dependencies.

**Supply chain trust model**:
- All dependencies resolved from tsuku recipe registry (same trust model as main tools)
- Dependencies declared explicitly in recipe `dependencies` field (not dynamically resolved)
- Recipe maintainers control dependency versions and sources
- No transitive dependency resolution from external package managers

**Authenticity verification**:
- Dependencies use same recipe structure as main tools
- Download URLs and checksums specified in dependency recipes
- Recipes are human-reviewed before merging to registry (social trust model)
- **No cryptographic signing of recipes** (identified gap - out of scope for this design)
- Users trust registry maintainers to validate recipes before publication

**Compromise scenarios**:

| Scenario | Current Mitigation | This Feature's Impact |
|----------|-------------------|----------------------|
| Upstream source compromised | Checksum mismatch prevents installation | No change (same verification) |
| Dependency recipe malicious | Recipe review process, user trust in registry | Extends to dependencies (no new risk) |
| Dependency URL poisoned | Checksum verification fails | No change (same verification) |
| MITM attack during download | HTTPS + checksum verification | No change (same verification) |

**Assessment**: This feature extends the existing supply chain risk model to dependencies without introducing new attack vectors. Dependencies are subject to the same recipe review and verification as main tools.

### User Data Exposure

**Impact**: This feature changes what data is processed during plan generation and what is included in generated plans.

**Data accessed**:
- Recipe dependency declarations (public data from registry)
- Download cache directory (already accessed in normal mode)
- Installation state (`$TSUKU_HOME/state.json`) for deduplication

**Data included in plans**:
- Dependency names and versions
- Dependency installation steps
- Download URLs and checksums (already in plans for main tool)

**Data transmitted**:
- HTTP(S) requests to download dependency artifacts
- Same request patterns as normal install mode
- No telemetry or analytics (tsuku is offline-first)

**Privacy implications**:
- Plans now reveal full dependency tree (could indicate user intent to install specific tool stack)
- Plans with dependencies are larger files (more data if shared)
- No new external communication; all downloads were already happening in normal mode

**Assessment**: Minimal privacy impact. The dependency tree information was already being resolved and installed in normal mode; this feature just includes it in the plan structure. No new data is transmitted externally.

### Security Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious dependency binaries | Checksum verification on all downloads | Recipe maintainer could specify malicious URL + checksum |
| Compromised upstream source | HTTPS + checksum verification prevents binary substitution | Source compromise before checksum recorded in recipe |
| Dependency version confusion | Explicit version pinning in recipes | Recipe maintainer must keep versions current |
| Plan contains sensitive info | Plans are local files, user controls sharing | User could share plan with dependency tree revealing tool stack |
| **Eval-time deps execute on host** | **User trust model: only use with trusted recipes** | **Full host access during plan generation (architectural limitation)** |
| **Download cache poisoning** | **Document limitation, recommend separate cache** | **Container can write to shared cache (future fix needed)** |
| **Recipe signing absent** | **Social trust: human review before registry merge** | **Compromised registry could serve malicious recipes (out of scope)** |
| Container runtime security | User configures container runtime (Podman/Docker) | Container breakout depends on runtime security configuration |

**Residual risk acceptance**:

1. **Eval-time dependency host execution**: This is an **architectural limitation** of tsuku's plan generation design, not specific to this feature. Sandbox mode is for testing recipe platform compatibility, not isolating malicious code. Users must trust recipes they test.

2. **Cache poisoning**: Acknowledged limitation. Future enhancement could use separate cache for sandbox mode. Current risk is acceptable for trusted recipes from the registry.

3. **Recipe signing**: Out of scope for this design. The social trust model (human review + checksums) is the current tsuku security model. Recipe signing would be a separate strategic enhancement.

4. **Container security**: Users are responsible for configuring their container runtime securely. Tsuku provides the isolation mechanism but doesn't enforce container security policies.

All other residual risks are inherited from the existing tsuku trust model (trust recipe registry maintainers, verify checksums, no sudo). This feature extends the existing model to dependencies in sandbox mode without introducing fundamentally new attack vectors.
