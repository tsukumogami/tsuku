# Architecture Review: Sandbox Implicit Dependencies Design

**Date**: 2026-01-04
**Reviewer**: Architecture Analysis Agent
**Context**: Solution architecture for making implicit action dependencies available during sandbox execution

## Executive Summary

The proposed architecture is **implementable and correctly designed**, with clear separation of concerns between host-side dependency installation and container-side execution. However, there are **several critical gaps** that need addressing:

1. **Missing function signature changes** - The design shows new parameters but doesn't update all call sites
2. **Path conversion logic is incomplete** - Host paths to container paths needs more detail
3. **Alternative approaches were insufficiently explored** - Simpler options exist
4. **Security implications need documentation** - Installing to host has attack surface

**Recommendation**: Proceed with implementation after addressing the gaps documented below.

---

## Question 1: Is the architecture clear enough to implement?

**Answer**: Mostly yes, but with important gaps.

### What's Clear

1. **Data flow is well-documented**: The 4-phase flow (dependency discovery → installation → container setup → plan execution) is logical and complete.

2. **Component responsibilities are separated**:
   - Host: `runSandboxInstall()` calls `ensurePackageManagersForRecipe()`
   - Executor: `Sandbox()` adds mount and builds script
   - Container: No changes needed, uses PATH lookup

3. **Mount strategy is sound**: Read-only mount of `~/.tsuku/tools/` prevents container from modifying host state.

4. **Existing infrastructure is reused**: The design correctly leverages `ensurePackageManagersForRecipe()` and `actions.ResolveDependencies()`.

### What Needs Clarification

#### 1. Function Signature Changes Are Incomplete

The design shows:
```go
func (m *Manager) Sandbox(recipe *Recipe, plan *InstallationPlan, opts *Options) error
```

But the current signature (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/executor.go:117`) is:
```go
func (e *Executor) Sandbox(
    ctx context.Context,
    plan *executor.InstallationPlan,
    target platform.Target,
    reqs *SandboxRequirements,
) (*SandboxResult, error)
```

**Issues:**
- Design refers to `Manager.Sandbox()` but actual implementation is `Executor.Sandbox()`
- Design doesn't show how `execPaths` integrates with existing parameters (`ctx`, `target`, `reqs`)
- Missing details on where/how to add `ExecPaths` field to existing structures

**Resolution needed:**
```go
// Option 1: Add to SandboxRequirements (logical grouping)
type SandboxRequirements struct {
    RequiresNetwork bool
    Image           string
    Resources       ResourceLimits
    ExecPaths       []string  // NEW: Bin paths for implicit deps
}

// Option 2: Add to Executor options (passed via WithExecPaths)
func WithExecPaths(paths []string) ExecutorOption {
    return func(e *Executor) {
        e.execPaths = paths
    }
}

// Option 3: Pass directly to Sandbox() method
func (e *Executor) Sandbox(
    ctx context.Context,
    plan *executor.InstallationPlan,
    target platform.Target,
    reqs *SandboxRequirements,
    execPaths []string,  // NEW
) (*SandboxResult, error)
```

**Recommendation**: Option 3 (direct parameter) is clearest, matches the design intent, and avoids coupling to requirements or executor state.

#### 2. Path Conversion Logic Is Under-Specified

The design shows:
```go
// Convert ~/.tsuku/tools/foo-1.0/bin → /workspace/tsuku/tools/foo-1.0/bin
relPath := strings.TrimPrefix(p, tsukuHome)
containerPath := filepath.Join("/workspace/tsuku", relPath)
```

**Problems:**
- What if `tsukuHome` is not `~/.tsuku`? (User can set `$TSUKU_HOME`)
- What if path doesn't start with `tsukuHome`? (Error? Warning? Skip?)
- How to get `tsukuHome` value in `buildSandboxScript()`? (Not passed in design)

**Current code context:**
- From `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_deps.go:214`:
  ```go
  toolDir := cfg.ToolDir(depName, toolState.Version)
  binDir := filepath.Join(toolDir, "bin")
  ```
- `cfg.ToolsDir` is the base, but individual tool dirs include version

**Resolution needed:**
```go
func (e *Executor) buildSandboxScript(
    plan *executor.InstallationPlan,
    reqs *SandboxRequirements,
    execPaths []string,
    tsukuHome string,  // NEW: needed for path conversion
) string {
    var containerPaths []string
    for _, hostPath := range execPaths {
        // Verify path is under tsukuHome/tools/
        toolsDir := filepath.Join(tsukuHome, "tools")
        if !strings.HasPrefix(hostPath, toolsDir) {
            // Log warning and skip - this shouldn't happen
            continue
        }

        // Convert: ~/.tsuku/tools/cmake-3.28.1/bin → /workspace/tsuku/tools/cmake-3.28.1/bin
        relPath := strings.TrimPrefix(hostPath, toolsDir)
        containerPath := filepath.Join("/workspace/tsuku/tools", relPath)
        containerPaths = append(containerPaths, containerPath)
    }

    if len(containerPaths) > 0 {
        script.WriteString("# Add implicit dependency bins to PATH\n")
        script.WriteString("export PATH=\"")
        script.WriteString(strings.Join(containerPaths, ":"))
        script.WriteString(":$PATH\"\n\n")
    }
}
```

#### 3. Call Chain Is Incomplete

The design shows Phase 1 as:
```go
func runSandboxInstall(...) error {
    execPaths, err := ensurePackageManagersForRecipe(mgr, recipe, visited, telemetryClient)
    // ...
    err = mgr.Sandbox(recipe, plan, &sandbox.Options{ExecPaths: execPaths})
}
```

But from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_sandbox.go:24-151`, the actual flow is:
```go
func runSandboxInstall(toolName, planPath, recipePath string) error {
    // No manager instance created
    // No recipe loaded directly (loaded inside switch)
    // Calls sandboxExec.Sandbox(globalCtx, plan, target, reqs)
}
```

**Missing steps in design:**
1. Where does `mgr *install.Manager` come from in `runSandboxInstall()`?
2. How to get `recipe *recipe.Recipe` before plan generation?
3. How to pass `execPaths` from host to sandbox executor?

**Current actual call chain:**
```
runSandboxInstall()
  → loadLocalRecipe() OR loader.Get()
  → generatePlanFromRecipe()
  → sandbox.ComputeSandboxRequirements(plan)
  → sandboxExec.Sandbox(globalCtx, plan, target, reqs)
```

**Resolution needed:**
```go
func runSandboxInstall(toolName, planPath, recipePath string) error {
    cfg, err := config.DefaultConfig()
    // ... existing config setup ...

    // NEW: Create manager for dependency installation
    mgr := install.New(cfg)

    var plan *executor.InstallationPlan
    var r *recipe.Recipe
    var execPaths []string

    switch {
    case recipePath != "":
        r, err = loadLocalRecipe(recipePath)
        if err != nil {
            return fmt.Errorf("failed to load recipe: %w", err)
        }

        // NEW: Install implicit dependencies on host
        execPaths, err = ensurePackageManagersForRecipe(mgr, r, make(map[string]bool), nil)
        if err != nil {
            return fmt.Errorf("failed to ensure dependencies: %w", err)
        }

        plan, err = generatePlanFromRecipe(r, toolName, cfg)
        // ... rest of plan generation

    // ... other cases similarly updated
    }

    // ... existing sandbox setup ...

    // NEW: Pass execPaths to sandbox executor
    result, err := sandboxExec.Sandbox(globalCtx, plan, target, reqs, execPaths)
}
```

#### 4. Mount Specification Conflicts with Current Code

The design shows:
```go
mounts = append(mounts, Mount{
    Source:   filepath.Join(m.tsukuHome, "tools"),
    Target:   "/workspace/tsuku/tools",
    ReadOnly: true,
})
```

But from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/executor.go:267-278`, the current mount structure is:
```go
Mounts: []validate.Mount{
    {
        Source:   workspaceDir,
        Target:   "/workspace",
        ReadOnly: false,
    },
    {
        Source:   cacheDir,
        Target:   "/workspace/tsuku/cache/downloads",
        ReadOnly: true,
    },
},
```

**Issues:**
- Design uses `Mount` type, actual code uses `validate.Mount`
- Design doesn't show where `m.tsukuHome` comes from (Executor doesn't have this field)
- Mounting `/workspace/tsuku/tools` when `/workspace` is already mounted may cause conflicts

**Resolution needed:**
```go
// In Executor.Sandbox(), add after existing mounts:
tsukuHome := os.Getenv("TSUKU_HOME")
if tsukuHome == "" {
    homeDir, _ := os.UserHomeDir()
    tsukuHome = filepath.Join(homeDir, ".tsuku")
}

opts.Mounts = append(opts.Mounts, validate.Mount{
    Source:   filepath.Join(tsukuHome, "tools"),
    Target:   "/workspace/tsuku/tools",
    ReadOnly: true,
})
```

---

## Question 2: Are there missing components or interfaces?

**Answer**: Yes, several critical components are missing or under-specified.

### Missing Components

#### 1. Error Handling for Dependency Installation Failures

The design doesn't address:
- What happens if `ensurePackageManagersForRecipe()` fails?
- Should sandbox abort or proceed without deps?
- How to communicate which deps failed to install?

**Current behavior** (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_deps.go:181-183`):
```go
if err := installWithDependencies(depName, "", "", false, r.Metadata.Name, depVisited, telemetryClient); err != nil {
    return nil, fmt.Errorf("failed to install dependency '%s': %w", depName, err)
}
```

**Recommendation**: Fail-fast is correct for sandbox mode. User should fix dependency issues before sandbox testing.

#### 2. Verification That Dependencies Are Actually Available

The design assumes dependencies installed to `~/.tsuku/tools/cmake-3.28.1/bin/cmake` will work in the container, but doesn't verify:
- Binary is executable
- Binary is compatible with container OS/arch
- Binary doesn't depend on host-specific libraries

**Missing component**: Pre-flight check in `ensurePackageManagersForRecipe()` return path:
```go
func ensurePackageManagersForRecipe(...) ([]string, error) {
    // ... existing installation logic ...

    // NEW: Verify each installed dependency
    for _, binPath := range execPaths {
        if err := verifyDependencyBinary(binPath); err != nil {
            return nil, fmt.Errorf("dependency at %s failed verification: %w", binPath, err)
        }
    }

    return execPaths, nil
}

func verifyDependencyBinary(binPath string) error {
    // Check directory exists and is readable
    if _, err := os.Stat(binPath); err != nil {
        return fmt.Errorf("bin directory not accessible: %w", err)
    }

    // Check at least one executable exists
    entries, err := os.ReadDir(binPath)
    if err != nil {
        return err
    }
    for _, entry := range entries {
        if !entry.IsDir() {
            info, _ := entry.Info()
            if info.Mode()&0111 != 0 { // Has execute bit
                return nil // Found at least one executable
            }
        }
    }
    return fmt.Errorf("no executable files found in %s", binPath)
}
```

#### 3. Logging/Telemetry for Sandbox Dependency Installation

The design shows calling `ensurePackageManagersForRecipe()` but doesn't address:
- Should sandbox dependency installation count toward telemetry?
- How to distinguish "installed for sandbox" vs "installed for normal use"?
- Should user see progress output for dependency installation?

**Current behavior** (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_deps.go:176`):
```go
printInfof("Ensuring dependency '%s' for package manager action...\n", depName)
```

**Recommendation**: Keep existing logging (user needs to know what's being installed), but pass `nil` telemetry client to avoid counting sandbox deps as user installs:
```go
// In runSandboxInstall():
execPaths, err := ensurePackageManagersForRecipe(mgr, r, make(map[string]bool), nil) // nil telemetry
```

#### 4. Cleanup Mechanism for Sandbox-Installed Dependencies

The design doesn't address lifecycle:
- Are dependencies persistent across sandbox runs? (Yes, installed to `~/.tsuku/tools/`)
- Should there be a `--sandbox-clean` flag to remove them?
- What if user never uses those tools outside sandbox?

**Current state**: Dependencies are installed permanently. This is acceptable (they're useful tools), but should be documented.

**Future enhancement** (out of scope for this issue):
```go
// Potential --sandbox-isolation flag:
// Install deps to temporary directory instead of ~/.tsuku/tools/
tempToolsDir, _ := os.MkdirTemp("", "tsuku-sandbox-deps-")
defer os.RemoveAll(tempToolsDir)
```

### Missing Interfaces

#### 1. No Abstraction for Path Conversion

The design hardcodes path conversion logic in `buildSandboxScript()`. This should be extracted:

```go
// PathMapper converts host paths to container paths
type PathMapper interface {
    ToContainerPath(hostPath string) (string, error)
}

type TsukuPathMapper struct {
    tsukuHome string
}

func (m *TsukuPathMapper) ToContainerPath(hostPath string) (string, error) {
    toolsDir := filepath.Join(m.tsukuHome, "tools")
    if !strings.HasPrefix(hostPath, toolsDir) {
        return "", fmt.Errorf("path %s is not under tools directory %s", hostPath, toolsDir)
    }
    relPath := strings.TrimPrefix(hostPath, toolsDir)
    return filepath.Join("/workspace/tsuku/tools", relPath), nil
}
```

**Benefit**: Testable, reusable, and clear contract.

#### 2. No Validation Interface for ExecPaths

The design assumes `execPaths` are valid directory paths, but doesn't enforce:

```go
// ExecPathValidator validates that exec paths are safe to mount and use
type ExecPathValidator interface {
    Validate(paths []string) error
}

type DefaultExecPathValidator struct {
    tsukuHome string
}

func (v *DefaultExecPathValidator) Validate(paths []string) error {
    toolsDir := filepath.Join(v.tsukuHome, "tools")
    for _, p := range paths {
        // Must be under tsuku/tools/
        if !strings.HasPrefix(p, toolsDir) {
            return fmt.Errorf("exec path %s is outside tools directory", p)
        }
        // Must exist and be readable
        if _, err := os.Stat(p); err != nil {
            return fmt.Errorf("exec path %s is not accessible: %w", p, err)
        }
        // Must not contain path traversal
        if strings.Contains(p, "..") {
            return fmt.Errorf("exec path %s contains path traversal", p)
        }
    }
    return nil
}
```

---

## Question 3: Are the implementation phases correctly sequenced?

**Answer**: Phases are mostly correct, but dependencies between them need adjustment.

### Current Phasing

```
Phase 1: Host-side Dependency Installation
Phase 2: Container Mount Configuration
Phase 3: PATH Configuration in Sandbox Script
Phase 4: Integration Testing
```

### Analysis

#### Phase 1: Correct Foundation

**Strengths:**
- Starts with host-side changes (no container coupling)
- Can be tested independently (install deps, verify they exist)
- Reuses existing `ensurePackageManagersForRecipe()`

**Weaknesses:**
- Verification step is incomplete ("Verify cmake, zig, make, pkg-config installed to `~/.tsuku/tools/`")
- Missing: Verify bin paths are returned correctly
- Missing: Test with recipe that has no implicit deps (should return empty `execPaths`)

**Improved Phase 1 verification:**
```bash
# Build and run
go build -o tsuku ./cmd/tsuku
./tsuku install --recipe recipes/ninja.toml --sandbox --linux-family debian

# Verify dependencies installed
ls -la ~/.tsuku/tools/cmake-*/bin/cmake
ls -la ~/.tsuku/tools/zig-*/zig
ls -la ~/.tsuku/tools/make-*/bin/make
ls -la ~/.tsuku/tools/pkg-config-*/bin/pkg-config

# Verify execPaths returned (add debug logging)
# Should print: execPaths = [~/.tsuku/tools/cmake-3.28.1/bin, ~/.tsuku/tools/zig-0.11.0, ...]
```

#### Phase 2: Incomplete Dependency on Phase 1

**Problem:** Phase 2 says "Dependencies: Phase 1 (execPaths must be available to pass to Sandbox)"

But Phase 2's changes are:
```go
In `internal/sandbox/executor.go`:
  - Extend `Options` struct with `ExecPaths []string` field
  - In `Sandbox()` method, add mount specification
```

**Issue:** These changes don't actually depend on Phase 1 being complete. They just add the infrastructure to receive and use `execPaths`. Phase 2 could be done in parallel with Phase 1.

**Better sequencing:**
- Phase 1a: Host-side (`ensurePackageManagersForRecipe()` integration)
- Phase 1b: Executor infrastructure (add `execPaths` parameter, mount config)
- Phase 2: Script generation (uses output from 1a + 1b)
- Phase 3: Integration testing

#### Phase 3: Should Include Error Cases

**Current verification:**
```
- Examine generated sandbox script (`/tmp/tsuku-sandbox-*/script.sh`)
- Verify PATH export appears before `tsuku install --plan`
- Run `which cmake` inside container - should resolve to `/workspace/tsuku/tools/cmake-*/bin/cmake`
- Verify ninja recipe builds successfully in sandbox
```

**Missing:**
- What if `execPaths` is empty? (Recipe with no implicit deps)
- What if path conversion fails? (Invalid host path)
- What if mount fails? (Permission issues)

**Improved Phase 3 verification:**
```bash
# Test 1: Recipe with implicit deps (ninja)
./tsuku install --recipe recipes/ninja.toml --sandbox
# Verify: PATH includes /workspace/tsuku/tools/*/bin
# Verify: ninja builds successfully

# Test 2: Recipe without implicit deps (gh - pure download)
./tsuku install --recipe recipes/gh.toml --sandbox
# Verify: No PATH additions in script
# Verify: gh installs successfully

# Test 3: Error handling - corrupt dependency
rm -rf ~/.tsuku/tools/cmake-*
./tsuku install --recipe recipes/ninja.toml --sandbox
# Verify: Fails with clear error message about missing cmake

# Test 4: Container path resolution
# Add debug logging to buildSandboxScript() showing path conversions
./tsuku install --recipe recipes/ninja.toml --sandbox 2>&1 | grep "Container path:"
# Verify: Conversions look correct
```

#### Phase 4: Missing Cross-Platform Testing

**Current verification:**
```
- All test recipes build successfully in sandbox mode
- No regressions in normal install mode
- Tools correctly discovered (check build logs for cmake/meson/configure invocations)
```

**Missing:** The design mentions "Test across different Linux families: debian, rhel, arch, alpine, suse" but Phase 4 doesn't include this.

**Problem:** Different Linux families have different base images, which may have different default PATHs, different `/bin` vs `/usr/bin` layouts, etc. The PATH configuration needs to work across all of them.

**Improved Phase 4:**
```bash
for family in debian rhel arch alpine suse; do
    echo "Testing with $family"
    ./tsuku install ninja --sandbox --linux-family $family
    # Verify: cmake found in PATH
    # Verify: build succeeds
done
```

### Recommended Phasing Revision

```
Phase 1: Infrastructure Setup (parallel)
  1a: Host-side dependency installation (ensurePackageManagersForRecipe integration)
  1b: Executor parameter plumbing (execPaths parameter, mount config)

Phase 2: Script Generation
  - buildSandboxScript() path conversion and PATH setup
  - Unit tests for path conversion logic
  - Depends on: 1a (for execPaths data) + 1b (for mount)

Phase 3: Integration Testing (single recipe)
  - Test ninja recipe in sandbox mode
  - Verify implicit deps installed, mounted, and used
  - Test error cases (missing deps, invalid paths)
  - Depends on: Phase 2

Phase 4: Cross-Platform Validation
  - Test across all Linux families
  - Test multiple recipes (ninja, libsixel-source, gdbm-source)
  - Performance testing (cached deps across runs)
  - Depends on: Phase 3
```

---

## Question 4: Are there simpler alternatives we overlooked?

**Answer**: Yes, there are simpler alternatives worth considering.

### Alternative 1: Install Dependencies Inside Container (Original Option 2)

**Design rejected this because:** "Redundant installation in normal (non-sandbox) mode"

**Re-evaluation:** This concern may be overstated.

#### How It Would Work

```go
// In generatePlanFromRecipe():
func generatePlanFromRecipe(r *recipe.Recipe, toolName string, cfg *config.Config) (*executor.InstallationPlan, error) {
    exec, err := executor.New(r)

    // NEW: Resolve implicit dependencies
    deps := actions.ResolveDependencies(r)

    plan, err := exec.GeneratePlan(globalCtx, executor.PlanConfig{
        OS:               runtime.GOOS,
        Arch:            runtime.GOARCH,
        RecipeSource:    "local",
        Downloader:      downloader,
        DownloadCache:   downloadCache,
        ImplicitDeps:    deps.InstallTime,  // NEW: Include in plan
    })

    return plan, nil
}

// In plan execution (container):
func (e *Executor) ExecutePlan(ctx context.Context, plan *InstallationPlan) error {
    // NEW: Install implicit dependencies first
    for depName := range plan.ImplicitDeps {
        if err := e.installDependency(ctx, depName); err != nil {
            return fmt.Errorf("failed to install implicit dependency %s: %w", depName, err)
        }
    }

    // Existing step execution
    for _, step := range plan.Steps {
        // ...
    }
}
```

#### Advantages Over Proposed Design

1. **Simpler implementation**: No host/container path conversion, no mounts, no PATH manipulation
2. **Self-contained plans**: Plan includes everything needed to execute (true offline execution)
3. **No host state pollution**: Sandbox deps stay in container (cleaner for testing)
4. **Container-native**: Dependencies installed for container's OS/arch, not host's

#### Why Original Rejection Was Wrong

The design states: "Options 2-4 would need to replicate this logic" (referring to transitive dependency resolution).

**But:** The logic doesn't need replication. `actions.ResolveDependencies()` is already a pure function:
```go
// From /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/resolver.go:56
func ResolveDependencies(r *recipe.Recipe) ResolvedDeps {
    return ResolveDependenciesForPlatform(r, runtime.GOOS)
}
```

It can be called **once** during plan generation and the results embedded in the plan. No duplication needed.

#### Addressing the "Redundant Installation" Concern

**In normal mode:**
```
1. Load recipe
2. Call ensurePackageManagersForRecipe() → installs cmake (if needed)
3. GeneratePlan() → includes cmake in plan.ImplicitDeps
4. ExecutePlan() → sees cmake in ImplicitDeps → checks if installed → YES, skip
```

**In sandbox mode:**
```
1. Load recipe
2. GeneratePlan() → includes cmake in plan.ImplicitDeps
3. ExecutePlan() (in container) → sees cmake in ImplicitDeps → NOT installed → install
```

The "redundant installation" only happens if you generate a plan in normal mode then execute it in normal mode. But that's the same as current behavior (plan execution always checks `IsInstalled()`).

#### When This Alternative Is Better

- **Recipe testing workflows**: Author testing local recipe shouldn't pollute host with build deps
- **CI/CD**: Sandbox runs in ephemeral containers, no need to persist deps
- **Cross-compilation**: Container can install deps for different arch than host

#### When Proposed Design Is Better

- **Performance**: Amortize dependency installation across multiple sandbox runs
- **Offline execution**: Pre-install deps on host, then run sandbox without network
- **Consistency**: Same dependency resolution for normal and sandbox modes

### Alternative 2: Hybrid Approach

Install common build tools (cmake, make, gcc) in custom base image, only mount recipe-specific deps.

#### How It Would Work

```dockerfile
# docker/debian-build.Dockerfile
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y \
    cmake \
    make \
    gcc \
    g++ \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*
```

```go
// In sandbox/requirements.go:
const BuildToolsImage = "tsuku/debian-build:latest"

func ComputeSandboxRequirements(plan *executor.InstallationPlan) *SandboxRequirements {
    deps := extractImplicitDeps(plan)

    // Common build tools available in image
    commonTools := map[string]bool{
        "cmake": true, "make": true, "gcc": true, "pkg-config": true,
    }

    // Determine which deps need mounting
    var mountDeps []string
    for dep := range deps {
        if !commonTools[dep] {
            mountDeps = append(mountDeps, dep) // e.g., zig, custom tools
        }
    }

    reqs := &SandboxRequirements{
        Image: BuildToolsImage,
        MountDeps: mountDeps,
    }
    return reqs
}
```

#### Advantages

- **Smaller attack surface**: Common tools come from distro packages (trusted source)
- **Faster startup**: No dependency installation delay
- **Simpler PATH setup**: Common tools already in default PATH

#### Disadvantages

- **Image maintenance**: Need to rebuild/publish custom images
- **Version drift**: Image cmake version may not match what recipes expect
- **Less flexible**: Can't easily test different cmake versions

### Alternative 3: Nix-Based Dependency Provision

Use `nix-portable` (already in codebase) to install build tools hermetically.

#### How It Would Work

```go
// In internal/actions/cmake_build.go:
func (a *CMakeBuildAction) Dependencies() ActionDeps {
    return ActionDeps{
        InstallTime: []string{"nix-portable"},  // Single dep, not cmake+make+zig+pkg-config
    }
}

func (a *CMakeBuildAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // Use nix to get cmake
    nixCmd := exec.CommandContext(ctx.Context, "nix-portable", "nix", "shell",
        "nixpkgs#cmake",
        "nixpkgs#gnumake",
        "nixpkgs#zig",
        "nixpkgs#pkg-config",
        "--command", "bash", "-c", "cmake --version && make --version && zig version")

    // ... rest of build
}
```

#### Advantages

- **Hermetic**: Exact versions, reproducible across machines
- **Self-contained**: nix-portable bundles everything
- **Minimal host deps**: Only need nix-portable itself

#### Disadvantages

- **Performance**: Nix downloads are slow (large closure)
- **Complexity**: Adds Nix to the mental model
- **Not all tools available**: Some proprietary tools not in nixpkgs

### Recommendation Matrix

| Use Case | Recommended Alternative |
|----------|------------------------|
| Quick implementation, optimize for development velocity | **Proposed design** (install on host) |
| CI/CD, ephemeral containers, no host state | **Alternative 1** (install in container) |
| Production sandbox, frequent recipe tests | **Alternative 2** (hybrid with base image) |
| Maximum reproducibility, research projects | **Alternative 3** (Nix-based) |

**For this issue:** Stick with proposed design, but document Alternative 1 as future enhancement for `--sandbox-isolated` flag.

---

## Security Implications (Missing from Design)

The design doesn't address security considerations of installing dependencies to the host.

### Attack Vectors

#### 1. Malicious Recipe → Compromised Host Dependencies

**Scenario:**
```toml
# evil-recipe.toml
[[steps]]
action = "cmake_build"
params = {source_dir = ".", executables = ["evil"]}
```

Running `tsuku install --recipe evil-recipe.toml --sandbox` will:
1. Install cmake, make, zig, pkg-config to `~/.tsuku/tools/` (on host)
2. These tools persist after sandbox test completes
3. User later runs `tsuku install some-other-tool` which uses cmake
4. Compromised cmake from step 1 runs on host (not in sandbox)

**Mitigation in design:** None specified.

**Recommended mitigation:**
```go
// In runSandboxInstall(), before calling ensurePackageManagersForRecipe():
if recipePath != "" {
    // Installing deps from local recipe - require confirmation
    fmt.Fprintf(os.Stderr, "WARNING: Testing local recipe will install build dependencies to your system:\n")
    for dep := range actions.ResolveDependencies(r).InstallTime {
        fmt.Fprintf(os.Stderr, "  - %s (from registry)\n", dep)
    }
    fmt.Fprintf(os.Stderr, "\nThese dependencies will persist after sandbox testing.\n")
    if !installForce && !confirm("Proceed with installing dependencies?") {
        return fmt.Errorf("sandbox test canceled")
    }
}
```

#### 2. Dependency Confusion

**Scenario:**
- Recipe depends on "cmake"
- Attacker publishes recipe "cmake" to registry with malicious binary
- `ensurePackageManagersForRecipe()` installs attacker's cmake instead of legitimate one

**Current protection:** Recipe names are curated (pull request review).

**Additional protection needed:**
```go
// In ensurePackageManagersForRecipe():
for depName := range resolvedDeps.InstallTime {
    // Verify dependency is from trusted registry
    if !isFromTrustedRegistry(depName) {
        return nil, fmt.Errorf("dependency %s is not from trusted registry", depName)
    }
}
```

#### 3. Path Traversal in ExecPaths

**Scenario:**
- Compromised dependency returns `binPath = "/../../etc"`
- Sandbox script adds `/../../etc` to PATH
- Container executes malicious binaries from weird location

**Current protection:** None in design.

**Recommended protection:**
```go
// In buildSandboxScript():
for _, containerPath := range containerPaths {
    // Validate no path traversal
    if strings.Contains(containerPath, "..") {
        return "", fmt.Errorf("path traversal detected in container path: %s", containerPath)
    }
    // Validate starts with expected prefix
    if !strings.HasPrefix(containerPath, "/workspace/tsuku/tools/") {
        return "", fmt.Errorf("invalid container path (must be under /workspace/tsuku/tools/): %s", containerPath)
    }
}
```

---

## Performance Considerations (Missing from Design)

### Caching Across Sandbox Runs

**Scenario:** User runs `tsuku install ninja --sandbox` 10 times while debugging recipe.

**Current design behavior:**
1. First run: Installs cmake, make, zig, pkg-config to `~/.tsuku/tools/` (~200MB, ~2 minutes)
2. Subsequent runs: `ensurePackageManagersForRecipe()` sees tools already installed, skips (fast)

**Good!** But should be documented.

### Dependency Installation Time Budget

**Question not addressed:** How long should users wait for dependency installation?

**Data points:**
- cmake: ~30MB download, ~1 minute compile
- zig: ~50MB download, extract-only (fast)
- make: ~5MB, fast
- pkg-config: ~2MB, fast

**Total for ninja recipe:** ~2-3 minutes on first sandbox run, <1 second on subsequent runs.

**Recommendation:** Add progress indicator during dependency installation:
```go
printInfof("Installing build dependencies for sandbox test (this may take a few minutes on first run)...\n")
execPaths, err := ensurePackageManagersForRecipe(mgr, r, visited, nil)
printInfof("Dependencies ready. Starting sandbox test...\n")
```

---

## Concrete Implementation Gaps

### Gap 1: `ensurePackageManagersForRecipe()` Signature Mismatch

**Design shows:**
```go
execPaths, err := ensurePackageManagersForRecipe(mgr, recipe, visited, telemetryClient)
```

**Actual signature** (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_deps.go:148`):
```go
func ensurePackageManagersForRecipe(mgr *install.Manager, r *recipe.Recipe, visited map[string]bool, telemetryClient *telemetry.Client) ([]string, error)
```

**Match!** But design doesn't show the return type `([]string, error)` explicitly in all places. Should clarify that `execPaths` is `[]string`, not `map[string]string` or custom type.

### Gap 2: Executor Field for tsukuHome

**Design shows** (in `buildSandboxScript`):
```go
relPath := strings.TrimPrefix(p, tsukuHome)
```

**Problem:** Where does `tsukuHome` variable come from?

**Current Executor struct** (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/executor.go:34-43`):
```go
type Executor struct {
    detector         *validate.RuntimeDetector
    logger           log.Logger
    tsukuBinary      string
    downloadCacheDir string
}
```

**Missing:** `tsukuHome string` field.

**Resolution:**
```go
// Option 1: Add field to Executor
type Executor struct {
    detector         *validate.RuntimeDetector
    logger           log.Logger
    tsukuBinary      string
    downloadCacheDir string
    tsukuHome        string  // NEW
}

// Option 2: Pass as parameter to buildSandboxScript
func (e *Executor) buildSandboxScript(
    plan *executor.InstallationPlan,
    reqs *SandboxRequirements,
    execPaths []string,
    tsukuHome string,  // NEW parameter
) string
```

**Recommendation:** Option 1 (add field) - it's needed in multiple places (mount config + script generation).

### Gap 3: Recipe Loading in runSandboxInstall

**Design assumes** recipe is loaded before calling `ensurePackageManagersForRecipe()`.

**Current code** (from `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_sandbox.go:48-74`):
```go
switch {
case recipePath != "":
    r, err = loadLocalRecipe(recipePath)
    // ...
    plan, err = generatePlanFromRecipe(r, toolName, cfg)

case default:
    r, err = loader.Get(toolName)
    // ...
    plan, err = generatePlanFromRecipe(r, toolName, cfg)
}
```

Recipe is loaded in all branches, **BUT** the variable `r` is only used for plan generation, then discarded.

**To call `ensurePackageManagersForRecipe(mgr, r, ...)`**, need to keep `r` alive:
```go
var plan *executor.InstallationPlan
var r *recipe.Recipe
var execPaths []string

switch {
case recipePath != "":
    r, err = loadLocalRecipe(recipePath)
    if err != nil {
        return fmt.Errorf("failed to load recipe: %w", err)
    }

    // NEW: Install dependencies BEFORE plan generation
    execPaths, err = ensurePackageManagersForRecipe(mgr, r, make(map[string]bool), nil)
    if err != nil {
        return fmt.Errorf("failed to ensure dependencies: %w", err)
    }

    plan, err = generatePlanFromRecipe(r, toolName, cfg)
    // ...
}
```

**This is correct**, but design should make it explicit that recipe loading must happen before dependency installation (obvious in retrospect, but worth documenting).

---

## Summary & Recommendations

### What's Good

1. **Conceptually sound**: Installing deps on host, mounting read-only, and setting PATH is a proven pattern
2. **Reuses existing code**: `ensurePackageManagersForRecipe()` and `ResolveDependencies()` are mature
3. **Clear separation of concerns**: Host handles installation, container handles execution
4. **Phased implementation**: Allows incremental progress with validation at each step

### Critical Gaps to Address Before Implementation

1. **Function signatures**: Update design to match actual `Executor.Sandbox()` signature, clarify where `execPaths` parameter goes
2. **Path conversion**: Add `tsukuHome` field to `Executor`, implement proper validation for path conversion
3. **Call chain**: Document full integration in `runSandboxInstall()` (manager creation, recipe loading, exec paths passing)
4. **Mount specification**: Use correct `validate.Mount` type, verify no conflicts with existing `/workspace` mount
5. **Security**: Add confirmation prompt for local recipe dep installation, validate paths for traversal
6. **Error handling**: Document what happens when dep installation fails, verify bins are executable
7. **Testing**: Expand Phase 3 to include error cases, expand Phase 4 to cover all Linux families

### Alternative Approaches Worth Considering

1. **Install in container** (Alternative 1): Simpler, no host state, better for CI/CD. Consider for future `--sandbox-isolated` flag.
2. **Hybrid with base image** (Alternative 2): Better performance for common deps, but requires image maintenance. Good for production.

### Recommended Next Steps

1. **Update design document** with:
   - Actual function signatures from codebase
   - Path conversion error handling
   - Security considerations section
   - Performance characteristics (caching, first-run time)

2. **Prototype Phase 1** with just `cmake` dependency:
   - Verify `ensurePackageManagersForRecipe()` integration
   - Test path collection and validation
   - Measure installation time

3. **Design review #2** after prototype:
   - Validate path conversion actually works
   - Verify no edge cases missed
   - Get feedback on Alternative 1 feasibility

4. **Proceed with full implementation** if prototype succeeds.

---

## Appendix: Code References

### Key Files

- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_sandbox.go` - Sandbox installation entry point
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_deps.go` - Dependency installation logic
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/executor.go` - Sandbox execution
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/cmake_build.go` - Example action with implicit deps
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/resolver.go` - Dependency resolution logic

### Key Functions

- `ensurePackageManagersForRecipe()` - Installs implicit deps, returns bin paths
- `actions.ResolveDependencies()` - Extracts deps from recipe
- `Executor.Sandbox()` - Runs plan in container
- `buildSandboxScript()` - Generates container setup script

### Example Dependency Declaration

From `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/cmake_build.go:24-26`:
```go
func (CMakeBuildAction) Dependencies() ActionDeps {
    return ActionDeps{InstallTime: []string{"cmake", "make", "zig", "pkg-config"}}
}
```
