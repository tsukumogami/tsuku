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

**Root cause**: The `generatePlanFromRecipe()` function in `cmd/tsuku/install_sandbox.go` does not pass `RecipeLoader` to the plan generation step. Without RecipeLoader, the dependency embedding infrastructure (format v3) cannot resolve and include dependency installation steps.

This is the same issue that was just fixed in PR #808 for `tsuku install` (issue #805), which addressed *implicit* action dependencies (cmake, make, etc.). Issue #703 addresses *explicit* recipe dependencies declared in the `dependencies` field.

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

### Option 1: Enable RecipeLoader in generatePlanFromRecipe()

Apply the same fix from PR #808 to the sandbox code path by passing `RecipeLoader` to `GeneratePlan()` in `cmd/tsuku/install_sandbox.go`.

**Changes required:**
```go
plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
    OS:                 runtime.GOOS,
    Arch:               runtime.GOARCH,
    RecipeSource:       "local",
    Downloader:         downloader,
    DownloadCache:      downloadCache,
    RecipeLoader:       loader,  // ← Add this
    AutoAcceptEvalDeps: true,    // ← Add this
    OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
        return installEvalDeps(deps, autoAccept)  // ← Add this
    },
})
```

**Pros:**
- Minimal code change (~5 lines added)
- Proven pattern from PR #808 (already tested and working)
- Reuses existing dependency embedding infrastructure
- Consistent with normal install mode
- Self-contained plans work in isolated containers
- Supports transitive dependencies automatically

**Cons:**
- `installEvalDeps()` is in install_deps.go; need to extract to shared location or adapt
- Requires eval-time dependency installation on host during plan generation (e.g., python-standalone for pipx_install actions)
- Plan generation time increases slightly (same as normal install mode, typically <1s per dependency)

### Option 2: Pre-install dependencies on host, then copy to container

Install dependencies on the host system before plan generation, then include dependency artifacts in the plan for mounting into the container.

**Changes required:**
- Add dependency resolution step before `generatePlanFromRecipe()`
- Install each dependency to host $TSUKU_HOME/tools
- Modify plan to include dependency binaries/libraries
- Update sandbox executor to mount dependency directories

**Pros:**
- No need for eval-time dependency installation in sandbox
- Dependencies pre-verified on host before sandbox test
- Could potentially cache dependency installations

**Cons:**
- Violates plan portability (plans depend on host state)
- Requires host system to have all dependencies installed
- Creates different code paths for sandbox vs normal install
- Breaks the architecture principle established in PR #808
- More complex implementation (~50+ lines of new code)
- Dependency versions might not match between host and container OS

### Option 3: Generate separate plans for each dependency

Generate individual plans for each dependency, then execute them sequentially in the sandbox before executing the main tool's plan.

**Changes required:**
- Resolve dependencies before sandbox execution
- Generate plan for each dependency using existing plan generator
- Execute dependency plans in dependency order
- Execute main tool plan with dependencies available

**Pros:**
- Reuses plan generation for dependencies
- Dependencies are explicitly installed in correct order
- Could cache dependency plans for reuse

**Cons:**
- Requires orchestration logic to handle multiple plans
- More complex than unified plan approach (~40+ lines)
- Doesn't leverage existing dependency embedding in format v3
- Reinvents functionality that already exists in GeneratePlan()
- More network calls and plan generation overhead

## Evaluation Against Decision Drivers

| Option | Consistency | Self-contained | Reuse Infrastructure | Minimal Changes | CI Unblocking |
|--------|-------------|----------------|----------------------|-----------------|---------------|
| **Option 1** | ✅ Excellent | ✅ Excellent | ✅ Excellent | ✅ Excellent | ✅ Yes |
| **Option 2** | ❌ Poor | ❌ Poor | ⚠️  Partial | ⚠️  Fair | ✅ Yes |
| **Option 3** | ⚠️  Fair | ✅ Good | ❌ Poor | ❌ Poor | ✅ Yes |

**Option 1** best satisfies all decision drivers:
- **Consistency**: Identical to normal install mode
- **Self-contained**: Plans include all dependencies (no host state)
- **Reuse infrastructure**: Uses proven PR #808 pattern
- **Minimal changes**: ~5 lines added
- **CI unblocking**: Enables issue #806 immediately

**Option 2** violates key principles:
- Breaks plan portability
- Creates divergent code paths
- Depends on host state (violates self-contained requirement)

**Option 3** reinvents existing infrastructure:
- Doesn't leverage format v3 dependency embedding
- More complex than necessary
- Higher maintenance burden

## Decision Outcome

**Chosen option: Option 1 - Enable RecipeLoader in generatePlanFromRecipe()**

This option reuses the proven pattern from PR #808, requires minimal code changes (~5 lines), and fully unifies the code path between normal install and sandbox mode. It ensures plans are self-contained and portable across environments.

### Rationale

This option was chosen because:

- **Consistency** (decision driver): Applies the exact same fix that worked in PR #808, making sandbox mode identical to normal install mode for dependency handling
- **Reuse infrastructure** (decision driver): Leverages the existing format v3 dependency embedding infrastructure without reinventing any logic
- **Minimal changes** (decision driver): Only 5 lines of code added to pass RecipeLoader and callbacks to GeneratePlan()
- **Self-contained plans** (decision driver): Plans include all dependencies in the dependency tree, making them executable without host state
- **Proven pattern**: PR #808 already validated this approach with comprehensive testing across multiple Linux families

Alternatives were rejected because:

- **Option 2**: Violates the architectural principle that plans must be portable and self-contained. Depending on host-side dependency installation creates divergent code paths and breaks plan portability.
- **Option 3**: Reinvents functionality that already exists in GeneratePlan() with dependency embedding. More complex implementation (~40+ lines) with higher maintenance burden and no architectural benefits over Option 1.

### Trade-offs Accepted

By choosing this option, we accept:

- **Eval-time dependency installation on host**: During plan generation, some dependencies (like python-standalone for pipx_install) must be installed on the host system. This is the same behavior as normal install mode.
- **Slight increase in plan generation time**: Adding dependency resolution increases plan generation time by <1 second per dependency. This is identical to the overhead in normal install mode and acceptable for the portability benefits.
- **Code location decision**: `installEvalDeps()` needs to be either extracted to a shared location or adapted for sandbox use. This is a minor refactoring task.

These trade-offs are acceptable because:

- Eval-time deps are required for plan generation to work correctly (can't decompose pipx_install without python-standalone)
- Plan generation time increase is negligible and only happens once (plans can be cached and reused)
- The code refactoring is straightforward and improves code organization by removing duplication

## Solution Architecture

### Overview

The solution adds dependency embedding to sandbox plan generation by passing `RecipeLoader` to `GeneratePlan()` in `cmd/tsuku/install_sandbox.go`. This enables the existing format v3 dependency infrastructure to include all dependencies (explicit and implicit, transitive and direct) in the generated plan.

When a user runs `tsuku install <tool> --sandbox`, the flow becomes:

1. **Plan Generation** (host): generatePlanFromRecipe() calls GeneratePlan() with RecipeLoader enabled
2. **Dependency Resolution** (host): GeneratePlan() recursively resolves dependencies and embeds them in the plan's dependency tree
3. **Eval-time Installation** (host): OnEvalDepsNeeded callback installs tools needed for plan generation (e.g., python-standalone for pipx_install)
4. **Plan Execution** (container): Sandbox executor runs the plan with all dependencies included

### Components

**Modified file**: `cmd/tsuku/install_sandbox.go`

```
generatePlanFromRecipe()
  ├─> executor.GeneratePlan() ← Add RecipeLoader parameter
  │   ├─> PlanConfig
  │   │   ├─> RecipeLoader: loader          ← Add this field
  │   │   ├─> AutoAcceptEvalDeps: true      ← Add this field
  │   │   └─> OnEvalDepsNeeded: callback    ← Add this field
  │   └─> Embeds dependencies in plan.Dependencies []Dependency
  └─> Returns self-contained plan
```

**New helper** (either extracted or adapted):

```go
func installEvalDepsForSandbox(deps []string, autoAccept bool) error {
    // Install eval-time dependencies on host
    // Reused logic from install_deps.go or extracted to shared location
}
```

### Key Interfaces

**PlanConfig struct** (already exists in `internal/executor/plan_generator.go`):

```go
type PlanConfig struct {
    OS                 string
    Arch               string
    RecipeSource       string
    Downloader         validate.Downloader
    DownloadCache      *actions.DownloadCache
    RecipeLoader       actions.RecipeLoader       // ← Use this
    AutoAcceptEvalDeps bool                       // ← Add this
    OnEvalDepsNeeded   func([]string, bool) error // ← Add this
}
```

**Generated plan** (format v3):

```json
{
  "tool": "curl",
  "version": "8.5.0",
  "steps": [...],
  "dependencies": [
    {
      "tool": "openssl",
      "version": "3.1.4",
      "recipe_type": "library",
      "steps": [...]
    },
    {
      "tool": "zlib",
      "version": "1.3",
      "recipe_type": "library",
      "steps": [...]
    }
  ]
}
```

### Data Flow

```
User: tsuku install curl --sandbox
  ↓
runSandboxInstall()
  ↓
generatePlanFromRecipe(curl recipe)
  ├─> GeneratePlan() with RecipeLoader
  │   ├─> Resolve curl recipe dependencies: [openssl, zlib]
  │   ├─> Recursively resolve transitive dependencies
  │   ├─> For each dependency:
  │   │   ├─> Load dependency recipe via RecipeLoader
  │   │   ├─> Generate dependency plan
  │   │   └─> Add to plan.Dependencies
  │   ├─> OnEvalDepsNeeded fires if pipx_install/etc found
  │   │   └─> installEvalDepsForSandbox([python-standalone])
  │   └─> Returns plan with embedded dependencies
  ↓
sandbox.Executor.Sandbox(plan)
  ├─> Execute dependency steps in container
  │   ├─> Install openssl in container
  │   └─> Install zlib in container
  ├─> Execute main tool steps in container
  │   └─> Build curl (has openssl and zlib available)
  └─> Verify installation
```

## Implementation Approach

### Step 1: Extract or adapt installEvalDeps() helper

**Option A - Extract to shared location:**
- Move `installEvalDeps()` from `cmd/tsuku/install_deps.go` to a shared file like `cmd/tsuku/helpers.go`
- Update both install_deps.go and install_sandbox.go to call the shared function

**Option B - Adapt inline:**
- Create `installEvalDepsForSandbox()` in install_sandbox.go that replicates the logic
- Accept minor duplication for code locality

**Recommendation**: Option A (extract to shared location) to follow DRY principle.

**Files modified**: `cmd/tsuku/helpers.go` (new function), `cmd/tsuku/install_deps.go` (call shared function)

### Step 2: Add RecipeLoader and callbacks to generatePlanFromRecipe()

Modify `cmd/tsuku/install_sandbox.go` line 180-186:

**Before:**
```go
plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
    OS:            runtime.GOOS,
    Arch:          runtime.GOARCH,
    RecipeSource:  "local",
    Downloader:    downloader,
    DownloadCache: downloadCache,
})
```

**After:**
```go
plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
    OS:                 runtime.GOOS,
    Arch:               runtime.GOARCH,
    RecipeSource:       "local",
    Downloader:         downloader,
    DownloadCache:      downloadCache,
    RecipeLoader:       loader,  // ← Add
    AutoAcceptEvalDeps: true,    // ← Add (non-interactive in sandbox)
    OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
        return installEvalDeps(deps, autoAccept)  // ← Add (shared helper)
    },
})
```

**Files modified**: `cmd/tsuku/install_sandbox.go`

**Lines changed**: 5 lines added

### Step 3: Test with tools that have dependencies

Verify the fix with integration tests:

```bash
# Test explicit library dependencies
go build -o tsuku ./cmd/tsuku
./tsuku install curl --sandbox

# Test tools with transitive dependencies
./tsuku install sqlite --sandbox

# Test tools with eval-time dependencies
./tsuku install pipx --sandbox
```

Expected behavior:
- Plans include all dependencies in the dependency tree
- Sandbox execution installs dependencies before main tool
- Build steps succeed with dependencies available

### Step 4: Verify CI compatibility

Run existing CI tests to ensure no regressions:

```bash
go test ./...
golangci-lint run --timeout=5m ./...
```

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
