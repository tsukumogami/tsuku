# Sandbox Implicit Dependencies

## Status

Proposed

## Context and Problem Statement

Build actions in tsuku (cmake_build, configure_make, meson_build) declare implicit dependencies through their `Dependencies()` method. These are tools required to execute the action - for example, cmake_build needs cmake, make, zig, and pkg-config available at build time.

In normal install mode, tsuku discovers these implicit dependencies by calling `ensurePackageManagersForRecipe()`, which:
1. Calls `actions.ResolveDependencies()` to collect all implicit InstallTime dependencies from recipe steps
2. Installs each dependency if not already present
3. Collects bin paths from installed dependencies
4. Passes these paths to the executor via `SetExecPaths()`

When the executor runs, actions can find required tools in `execCtx.ExecPaths`.

**Sandbox mode breaks this mechanism** because it operates differently:
1. Generates a plan from the recipe
2. Writes plan.json to disk
3. Runs `tsuku install --plan plan.json` inside a container

When `tsuku install --plan` executes, it has **no recipe context** - only the plan JSON. The plan contains steps (download, cmake_build, install_binaries) but **does not include implicit action dependencies**.

Note that `plan.Dependencies` (the existing field) contains only explicit recipe-declared dependencies from `metadata.dependencies`. Implicit action dependencies are never included in plans - they're resolved and installed before plan generation in normal mode. This same issue affects non-sandbox plan-based installation, but it's masked because normal mode installs implicit deps before generating the plan.

Actions that need tools fail:

```bash
$ tsuku eval ninja --linux-family rhel > plan.json
$ tsuku install --plan plan.json --sandbox

Running sandbox test for ninja...
  Container image: fedora:41

Sandbox test FAILED
Exit code: 1
Error: exec: "cmake": executable file not found in $PATH
```

This affects any tool using source build actions:
- ninja (cmake_build → needs cmake, make, zig, pkg-config)
- libsixel-source (meson_build → needs meson, make, zig, patchelf)
- gdbm-source (configure_make → needs make, zig, pkg-config)

The gap is structural: normal install has recipe context throughout, sandbox install separates plan generation (has recipe) from plan execution (only has JSON).

### Scope

**In scope:**
- Making implicit action dependencies available during sandbox execution
- Ensuring build actions (cmake_build, configure_make, meson_build) work in sandbox mode
- Maintaining parity between normal and sandbox install modes

**Out of scope:**
- Declared recipe dependencies (metadata.dependencies) - tracked separately in #703
- Changing how implicit dependencies are declared in actions
- Modifying the ActionDeps system or dependency resolution algorithm

## Decision Drivers

- **Correctness**: Sandbox mode must work identically to normal mode for the same recipes
- **Security**: Dependencies installed outside sandbox must be mounted read-only to maintain container isolation
- **Plan portability**: Plans should remain portable and not require access to original recipe file
- **Minimal disruption**: Prefer solutions that reuse existing infrastructure (ensurePackageManagersForRecipe, ResolveDependencies)
- **Performance**: Avoid redundant dependency resolution or installation
- **Clarity**: The mechanism should be understandable - where dependencies come from and when they're installed

## Implementation Context

### Upstream Designs

This work builds on completed dependency provisioning architecture:

**DESIGN-dependency-provisioning.md** (Status: Current, Milestones 18-21)
- Established implicit dependency system via ActionDeps (#547)
- Build actions declare InstallTime dependencies (cmake, make, zig, pkg-config)
- Normal install uses `ensurePackageManagersForRecipe()` to resolve and install implicit deps
- `actions.ResolveDependencies()` collects both recipe-declared and action-implicit dependencies

**DESIGN-install-sandbox.md** (Status: Current, Milestone 22)
- Centralized sandbox testing with recipe-driven requirements computation
- Actions implement NetworkValidator interface to declare network needs
- Sandbox executor derives container requirements from plan analysis
- Supports `tsuku install --sandbox` for standalone testing

### Existing Patterns

**Normal install dependency flow** (`cmd/tsuku/install_deps.go:143-434`):
1. `ensurePackageManagersForRecipe()` discovers implicit deps via `ResolveDependencies()`
2. Installs each dependency if not present
3. Collects bin paths from installed tools
4. Passes paths to executor via `SetExecPaths(execPaths)`
5. Execution context makes tools available to actions

**Plan-based installation** (`cmd/tsuku/plan_install.go:16-117`):
- Takes pre-generated InstallationPlan as input
- No recipe context available during execution
- plan.Dependencies contains only explicit recipe-declared deps
- Creates minimal recipe stub for executor initialization

**Sandbox execution** (`internal/sandbox/executor.go:117-357`):
- Generates plan from recipe
- Writes plan.json to workspace
- Builds container with system dependencies pre-installed (from #771)
- Runs `tsuku install --plan plan.json` inside container
- Mounts download cache read-only for offline execution

### Key Constraint

The plan-based installation pattern means **recipe context is lost** at execution time. The plan JSON contains steps to execute but not the action metadata (implicit dependencies). This is fundamental to the eval → install --plan workflow that enables offline sandbox testing.

## Considered Options

### Option 1: Install Dependencies Before Sandbox Execution

Install implicit dependencies **on the host** before launching the sandbox container, then mount the tools directory into the container.

**Approach:**
1. In `runSandboxInstall()`, after loading the recipe but before generating the plan
2. Call `ensurePackageManagersForRecipe(mgr, recipe, visited, telemetryClient)`
3. This installs cmake, make, zig, pkg-config to `~/.tsuku/tools/`
4. Mount `~/.tsuku/tools/` read-only into container at `/workspace/tsuku/tools/`
5. Update sandbox script to add tool bin directories to PATH

**Pros:**
- Reuses existing `ensurePackageManagersForRecipe()` function - no new dependency resolution logic
- Dependencies installed once on host, reused across sandbox runs (performance)
- Read-only mount maintains container isolation (security)
- Matches normal install flow exactly - same dependencies available in same locations
- Minimal code changes - add mount + PATH setup in sandbox script

**Cons:**
- Dependencies installed for host platform (works because Homebrew bottles are relocatable and `--linux-family` targets match host)
- Requires recipe context in `runSandboxInstall()` (currently available for modes 1 and 2, but mode 3 with external plan would need recipe file path)
- Tools visible to container even if not needed for this specific recipe (minor)
- Security consideration: Mounting `~/.tsuku/tools/` exposes all installed tools to container, not just those needed for this recipe (mitigated by read-only mount)

### Option 2: Include Implicit Dependencies in Plan

Extend InstallationPlan to include implicit dependencies with their resolved versions, allowing plan-based installation to discover and install them.

**Approach:**
1. During plan generation, call `ResolveDependencies()` to get implicit deps
2. Add new field to InstallationPlan: `ImplicitDependencies map[string]string`
3. When executing plan, check if `plan.ImplicitDependencies` exists
4. For each implicit dep, install it (similar to how `plan.Dependencies` are installed)
5. Set ExecPaths from installed implicit dependencies

**Alternative approach (2B):** Merge implicit deps into existing `plan.Dependencies` field using the same DependencyPlan structure for consistency. This avoids having two different dependency models but makes all deps install during plan execution.

**Pros:**
- Plan becomes self-contained - no recipe context needed at execution time
- Works for all plan-based installation modes (normal, sandbox, external plans)
- Dependencies installed inside container, cleaner isolation
- Could enable truly portable plans that capture all requirements

**Cons:**
- Changes InstallationPlan schema - affects plan caching, golden files, JSON serialization
- Redundant with existing dependency installation in normal mode (installed twice - once before plan generation, once during plan execution)
- Breaks plan portability - plans become host-specific (include resolved versions for host platform)
- More complex: need to handle dependency resolution in both plan generation and execution
- Unclear how to handle dependency chains (implicit dep itself has implicit deps)
- Creates two different dependency models if using separate field (existing nested DependencyPlans vs flat ImplicitDependencies map)

### Option 3: Re-derive Dependencies from Plan Steps

At plan execution time, examine plan steps to determine which actions are present, then install their implicit dependencies.

**Approach:**
1. In plan execution, before running steps, iterate through `plan.Steps`
2. For each step, call `actions.Get(step.Action).Dependencies()`
3. Collect all unique InstallTime dependencies
4. Install each dependency and collect bin paths
5. Set ExecPaths before executing plan steps

**Pros:**
- No changes to InstallationPlan schema
- Works without recipe context
- Dependencies derived from plan content itself
- Could work for external plans from unknown sources

**Cons:**
- Duplicates dependency resolution logic (already done in `ResolveDependencies()`)
- **Critical correctness issue:** Misses recipe-level dependency overrides/extensions (extra_dependencies, dependencies field replacement)
- **Cannot handle platform-specific dependencies** - no way to know target platform at plan execution time to choose between LinuxInstallTime vs DarwinInstallTime
- Actions must be loaded/instantiated during plan execution just to query Dependencies()
- Less accurate than full recipe-based resolution (missing recipe-level dependency context)
- Re-implements complex resolution logic, likely incorrectly

### Option 4: Pass Recipe to Sandbox Executor

Provide the recipe alongside the plan to sandbox execution, enabling full dependency resolution at execution time.

**Approach:**
1. Extend `Sandbox()` method to accept optional recipe parameter
2. In `runSandboxInstall()`, pass recipe to sandbox executor
3. Inside container, make recipe available (mount or embed in script)
4. Before executing plan, run dependency resolution on recipe
5. Install implicit dependencies inside container

**Pros:**
- Full recipe context available for accurate dependency resolution
- Handles all recipe-level dependency features (overrides, platform-specific, etc.)
- No changes to InstallationPlan schema
- Clean separation: plan defines "what to do", recipe defines "what's needed"

**Cons:**
- Adds complexity to sandbox interface and container setup
- Recipe must be available (breaks mode 3: external plan without recipe file)
- Duplicates resolution work (already done on host before plan generation)
- Dependencies installed inside container (slower, can't reuse across runs)
- More container filesystem writes (more attack surface)

## Option Evaluation

| Decision Driver | Option 1 (Host Install) | Option 2 (In Plan) | Option 3 (Re-derive) | Option 4 (Recipe) |
|-----------------|-------------------------|--------------------|-----------------------|-------------------|
| Correctness | Excellent - reuses proven logic | Good - but needs careful impl | Fair - misses recipe features | Excellent - full context |
| Security | Good - read-only mount | Good - container isolation | Good - container isolation | Fair - more writes in container |
| Plan portability | Good - plans portable | Poor - plans host-specific | Good - plans portable | Fair - requires recipe file |
| Minimal disruption | Excellent - reuses existing fn | Poor - schema changes | Fair - new resolution path | Fair - new interfaces |
| Performance | Excellent - install once on host | Fair - install per sandbox run | Fair - install per sandbox run | Fair - install per sandbox run |
| Clarity | Excellent - matches normal mode | Good - self-contained plans | Fair - implicit re-resolution | Good - explicit context passing |

## Decision Outcome

**Chosen option: Option 1 - Install Dependencies Before Sandbox Execution**

This option reuses the existing `ensurePackageManagersForRecipe()` function to install implicit dependencies on the host before launching the sandbox container, then mounts the tools directory read-only into the container.

### Rationale

This option was chosen because:

- **Correctness**: Reuses the proven dependency resolution logic from `ensurePackageManagersForRecipe()` and `ResolveDependencies()` - no new code paths that could introduce bugs or miss edge cases
- **Minimal disruption**: Requires only adding the function call in `runSandboxInstall()`, mounting `~/.tsuku/tools/`, and updating the sandbox script PATH - under 50 lines of changes
- **Performance**: Dependencies installed once on host and reused across all sandbox runs, avoiding redundant container installations
- **Clarity**: Matches normal install flow exactly - same dependencies available in same locations via same mechanism (ExecPaths)
- **Plan portability**: Plans remain portable; dependency installation is orthogonal to plan execution

Alternatives were rejected because:

- **Option 2 (In Plan)**: Changes InstallationPlan schema (affects caching, serialization, golden files), creates redundant installation (once before plan generation, again during execution), and breaks plan portability by making plans host-specific
- **Option 3 (Re-derive)**: Has critical correctness issues - cannot handle platform-specific dependencies (LinuxInstallTime vs DarwinInstallTime) without recipe context, misses recipe-level dependency overrides, and duplicates complex resolution logic
- **Option 4 (Recipe)**: Adds interface complexity, duplicates resolution work, requires recipe file availability (breaks external plan mode), and installs deps inside container (slower, can't reuse)

### Trade-offs Accepted

By choosing this option, we accept:

- **Host platform dependency**: Implicit dependencies installed for host platform (mitigated: Homebrew bottles are relocatable and `--linux-family` targets match host)
- **Recipe context requirement**: Requires recipe available in `runSandboxInstall()` (currently true for modes 1 and 2; mode 3 with external plan would need recipe file path)
- **Broad tool exposure**: All installed tools in `~/.tsuku/tools/` visible to container, not just those needed for this recipe (mitigated: read-only mount maintains isolation)

These are acceptable because:

- Host platform assumption aligns with current tsuku architecture (Homebrew bottles work across platforms, --linux-family already assumes host matches target)
- Recipe context is already available in all current sandbox modes and is the source of truth for requirements
- Read-only mount prevents container from modifying tools, maintaining security boundary despite broader visibility

## Solution Architecture

### Overview

The solution installs implicit action dependencies on the host before launching the sandbox container, then makes them available inside the container via a read-only mount and PATH configuration.

```
┌─────────────────────────────────────────────────────────────┐
│ runSandboxInstall() - Host Process                         │
│                                                             │
│  1. Load recipe from file                                  │
│  2. ensurePackageManagersForRecipe() ──> Install to        │
│                                           ~/.tsuku/tools/   │
│  3. Generate plan from recipe                              │
│  4. Write plan.json to workspace                           │
│  5. Launch container with mounts:                          │
│     - ~/.tsuku/tools/ → /workspace/tsuku/tools/ (ro)       │
│     - ~/.tsuku/cache/ → /workspace/tsuku/cache/ (ro)       │
│     - workspace → /workspace (rw)                          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ docker run
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Container Process                                           │
│                                                             │
│  1. Sandbox script sets PATH:                              │
│     export PATH="/workspace/tsuku/tools/cmake-*/bin:..."   │
│                                                             │
│  2. tsuku install --plan plan.json                         │
│     ├─ Reads plan.json                                     │
│     ├─ Executes steps (download, cmake_build, etc.)        │
│     └─ cmake_build finds cmake in PATH                     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Components

**Host-side changes:**

1. **`cmd/tsuku/install_sandbox.go:runSandboxInstall()`**
   - Add call to `ensurePackageManagersForRecipe()` after loading recipe
   - Collect bin paths from installed dependencies
   - Pass bin paths to sandbox executor

2. **`internal/sandbox/executor.go:Sandbox()`**
   - Accept new parameter: `toolsBinPaths []string`
   - Add mount specification for `~/.tsuku/tools/` (read-only)

3. **`internal/sandbox/executor.go:buildSandboxScript()`**
   - Generate PATH setup using toolsBinPaths
   - Add export statements before `tsuku install --plan` invocation

**Container-side (no changes needed):**
- Plan execution remains unchanged - uses standard PATH lookup

### Key Interfaces

**Modified function signature:**

```go
// cmd/tsuku/install_sandbox.go
func runSandboxInstall(
    mgr *manager.Manager,
    installReq *InstallRequest,
    cliCtx *CLIContext,
    telemetryClient telemetry.Client,
    visited map[string]bool,
) error {
    // Add after recipe loading, before plan generation:
    execPaths, err := ensurePackageManagersForRecipe(mgr, recipe, visited, telemetryClient)
    if err != nil {
        return err
    }

    // Pass execPaths to sandbox executor:
    err = mgr.Sandbox(recipe, plan, &sandbox.Options{
        ExecPaths: execPaths,
        // ... other options
    })
}
```

**Modified Sandbox method:**

```go
// internal/sandbox/executor.go
type Options struct {
    LinuxFamily string
    ExecPaths   []string  // New field
}

func (m *Manager) Sandbox(recipe *Recipe, plan *InstallationPlan, opts *Options) error {
    // Add mount for tools directory
    mounts = append(mounts, Mount{
        Source:   filepath.Join(m.tsukuHome, "tools"),
        Target:   "/workspace/tsuku/tools",
        ReadOnly: true,
    })

    // Pass opts.ExecPaths to buildSandboxScript
    script := buildSandboxScript(plan, opts.ExecPaths)
}
```

### Data Flow

1. **Dependency Discovery (Host)**:
   - `runSandboxInstall()` loads recipe
   - Calls `ensurePackageManagersForRecipe(mgr, recipe, visited, telemetryClient)`
   - `ensurePackageManagersForRecipe()` calls `actions.ResolveDependencies(recipe)`
   - Collects all InstallTime dependencies from recipe steps
   - Returns list of dependency names: `["cmake", "make", "zig", "pkg-config"]`

2. **Dependency Installation (Host)**:
   - For each dependency, checks if installed (`mgr.IsInstalled(dep)`)
   - Installs missing dependencies to `~/.tsuku/tools/<dep>-<version>/`
   - Collects bin paths: `["~/.tsuku/tools/cmake-3.28.1/bin", "~/.tsuku/tools/zig-0.11.0/", ...]`
   - Returns `execPaths []string`

3. **Container Setup (Host)**:
   - Passes `execPaths` to `Sandbox()` method
   - Sandbox executor adds mount: `~/.tsuku/tools/ → /workspace/tsuku/tools/ (ro)`
   - Builds sandbox script with PATH setup:
     ```bash
     export PATH="/workspace/tsuku/tools/cmake-3.28.1/bin:/workspace/tsuku/tools/zig-0.11.0:$PATH"
     tsuku install --plan plan.json
     ```

4. **Plan Execution (Container)**:
   - Container starts with tools mounted and PATH configured
   - `tsuku install --plan` executes normally
   - When `cmake_build` action runs, it finds `cmake` via PATH
   - No changes needed to plan execution logic

## Implementation Approach

### Phase 1: Host-side Dependency Installation

**Goal**: Install implicit dependencies on host before sandbox execution

**Changes**:
1. In `cmd/tsuku/install_sandbox.go:runSandboxInstall()`:
   - After loading recipe (line ~70), add:
     ```go
     execPaths, err := ensurePackageManagersForRecipe(mgr, recipe, visited, telemetryClient)
     if err != nil {
         return fmt.Errorf("installing implicit dependencies: %w", err)
     }
     ```
   - Store `execPaths` for passing to sandbox executor

2. Update call to `mgr.Sandbox()` to pass `execPaths`

**Dependencies**: None - uses existing `ensurePackageManagersForRecipe()` function

**Verification**:
- Build and run: `tsuku install --sandbox ninja --linux-family debian`
- Verify cmake, zig, make, pkg-config installed to `~/.tsuku/tools/` before container launches
- Check container logs show tools mounted

### Phase 2: Container Mount Configuration

**Goal**: Mount `~/.tsuku/tools/` into container read-only

**Changes**:
1. In `internal/sandbox/executor.go`:
   - Extend `Options` struct with `ExecPaths []string` field
   - In `Sandbox()` method, add mount specification:
     ```go
     mounts = append(mounts, Mount{
         Source:   filepath.Join(m.tsukuHome, "tools"),
         Target:   "/workspace/tsuku/tools",
         ReadOnly: true,
     })
     ```

**Dependencies**: Phase 1 (execPaths must be available to pass to Sandbox)

**Verification**:
- Run sandbox test with verbose logging
- Verify mount appears in `docker run` command
- Check container can read `/workspace/tsuku/tools/cmake-*/bin/cmake`
- Verify mount is read-only (try writing to `/workspace/tsuku/tools/` in container - should fail)

### Phase 3: PATH Configuration in Sandbox Script

**Goal**: Make mounted tools available via PATH in container

**Changes**:
1. In `internal/sandbox/executor.go:buildSandboxScript()`:
   - Accept `execPaths []string` parameter
   - Convert host paths to container paths:
     ```go
     var containerPaths []string
     for _, p := range execPaths {
         // Convert ~/.tsuku/tools/foo-1.0/bin → /workspace/tsuku/tools/foo-1.0/bin
         relPath := strings.TrimPrefix(p, tsukuHome)
         containerPath := filepath.Join("/workspace/tsuku", relPath)
         containerPaths = append(containerPaths, containerPath)
     }
     ```
   - Generate PATH export in script:
     ```go
     script.WriteString("export PATH=\"")
     script.WriteString(strings.Join(containerPaths, ":"))
     script.WriteString(":$PATH\"\n")
     ```

**Dependencies**: Phase 2 (mount must exist for paths to be valid)

**Verification**:
- Examine generated sandbox script (`/tmp/tsuku-sandbox-*/script.sh`)
- Verify PATH export appears before `tsuku install --plan`
- Run `which cmake` inside container - should resolve to `/workspace/tsuku/tools/cmake-*/bin/cmake`
- Verify ninja recipe builds successfully in sandbox

### Phase 4: Integration Testing

**Goal**: Verify end-to-end functionality across all affected recipes

**Changes**:
1. Add sandbox tests for recipes using build actions:
   - `ninja` (cmake_build)
   - `libsixel-source` (meson_build)
   - `gdbm-source` (configure_make)

2. Test across different Linux families:
   - debian, rhel, arch, alpine, suse

**Dependencies**: Phases 1-3 complete

**Verification**:
- All test recipes build successfully in sandbox mode
- No regressions in normal install mode
- Tools correctly discovered (check build logs for cmake/meson/configure invocations)

## Consequences

### Positive

- **Correctness**: Reuses battle-tested dependency resolution logic - no new bugs from reimplementation
- **Performance**: Dependencies installed once on host, reused across all sandbox runs - significantly faster than per-run installation
- **Minimal risk**: Small change surface (~50 lines across 2 files) reduces likelihood of regressions
- **Parity**: Sandbox mode now works identically to normal mode for implicit dependencies
- **Debuggability**: Tools visible on host filesystem - easier to debug version/availability issues

### Negative

- **Disk usage**: Implicit dependencies remain on host after sandbox run (not cleaned up automatically)
- **Security boundary expansion**: All tools in `~/.tsuku/tools/` exposed to container, not just recipe-specific deps
- **Platform assumptions**: Assumes host platform matches target (currently true but limits future flexibility)
- **Recipe context coupling**: Sandbox mode now requires recipe file (mode 3 with external plan only would need adaptation)

### Mitigations

- **Disk usage**: Acceptable - tools are small (cmake ~50MB, zig ~200MB) and reused across runs; users can manually `tsuku remove` if needed
- **Security boundary**: Read-only mount prevents modification; container already trusts tsuku binary itself, so trusting tsuku-installed tools is consistent threat model
- **Platform assumptions**: Documented limitation; future work could extend `--linux-family` to install family-specific deps, but not needed for MVP
- **Recipe context**: Current sandbox modes (1: recipe file, 2: recipe name) already have recipe context; mode 3 (external plan) is rare and can require recipe via `--recipe` flag if implicit deps needed

## Security Considerations

### Download Verification

**Impact**: Implicit dependencies (cmake, make, zig, pkg-config) are downloaded and installed to `~/.tsuku/tools/` on the host before sandbox execution.

**Verification mechanism**:
- All implicit dependencies are themselves tsuku-managed tools with recipes
- Each recipe specifies SHA256 checksums for downloaded artifacts
- Download verification uses existing tsuku checksum validation (same as normal installs)
- If checksum verification fails, installation aborts before mounting to container

**Risk**: Compromised dependency mirrors could serve malicious binaries that match outdated checksums in recipes.

**Mitigation**: Recipe updates include checksum updates; users running `tsuku update-registry` get fresh checksums. Tsuku's existing checksum verification applies to implicit deps identically to explicit installs.

**Residual risk**: Users with stale registries could download compromised binaries if attacker compromises mirror and matches old checksums. Same risk as any tsuku install - not specific to this feature.

### Execution Isolation

**Impact**: This feature expands the sandbox isolation boundary by mounting host-installed tools into containers.

**Permissions required**:
- **Host**: Read/write access to `~/.tsuku/tools/` for installing implicit dependencies (same as normal tsuku installs)
- **Container**: Read-only access to `/workspace/tsuku/tools/` via bind mount
- **Container**: Execute permissions on mounted binaries (cmake, zig, make, etc.)

**Isolation changes**:
- **Before**: Container had no access to `~/.tsuku/tools/`
- **After**: Container can read all tools in `~/.tsuku/tools/`, not just recipe-specific deps
- Read-only mount prevents container from modifying tools or escalating privileges

**Risk**: Container compromise could read tools directory contents and discover what other tools are installed on host (information leak).

**Mitigation**:
- Read-only mount prevents modification or privilege escalation
- Container already has read access to other mounted directories (`~/.tsuku/cache/`)
- Tool discovery provides minimal value to attacker (tool names/versions already public info)

**Residual risk**: Minimal - container can enumerate installed tools but cannot modify them or gain additional privileges. This is an acceptable expansion of the trust boundary.

### Supply Chain Risks

**Impact**: Implicit dependencies are sourced from upstream providers (GitHub releases, official websites) and installed on the host system where they're shared across sandbox runs.

**Source trust model**:
- Implicit dependencies are standard tsuku recipes (cmake, zig, make, pkg-config)
- Sources: GitHub releases (cmake, zig), GNU mirrors (make), freedesktop.org (pkg-config)
- Trust chain: Recipe maintainer → upstream provider → tsuku checksum verification

**Compromise scenarios**:

1. **Upstream provider compromised**: Attacker compromises cmake GitHub releases
   - Mitigation: Checksum verification detects if binary doesn't match recipe
   - User must update registry to get compromised checksum before attack succeeds

2. **Recipe repository compromised**: Attacker modifies tsuku recipes with malicious checksums
   - Mitigation: Recipe repository is version-controlled and reviewed via PR process
   - Users pull recipes from trusted tsuku-registry repository

3. **Persistent host installation**: Compromised tool persists on host and affects all future sandbox runs
   - Mitigation: Tools installed to `~/.tsuku/tools/` are isolated per version; users can `tsuku remove <tool>` to purge
   - No different from normal tsuku install behavior

**Risk**: If both upstream provider AND recipe repository are compromised (or recipe is updated with compromised checksum), malicious binaries could be installed to host and used in all sandbox runs.

**Mitigation**: Same supply chain security as any tsuku install - recipe review process, checksum verification, user vigilance on registry updates.

**Residual risk**: Sophisticated attacker compromising both upstream and recipe repository could inject malicious tools. This is a fundamental tsuku risk, not specific to sandbox implicit deps. Future work: signature verification, reproducible builds.

### User Data Exposure

**Impact**: This feature does not change what data implicit dependencies can access - it only changes when/where they're installed (host before sandbox vs. container during execution).

**Data access by implicit dependencies**:
- **Build tools (cmake, make, meson)**: Read recipe source code, write build artifacts to workspace
- **Compiler (zig)**: Read source, write binaries/libraries to workspace
- **pkg-config**: Read system package metadata (none in container, minimal on host)

**No new data exposure**:
- Tools already had access to workspace in normal install mode
- Sandbox mode restricts data access (container isolation) compared to normal mode
- Read-only mounts for cache and tools prevent data exfiltration via cache modification

**Data transmission**:
- Implicit dependencies installed during host-side `ensurePackageManagersForRecipe()` call
- Network access only during installation (download phase) - same as normal installs
- No new network access during container execution (tools mounted from host)

**Privacy implications**:
- No change to individual tool capabilities - same data access, same network behavior as normal install
- Sandbox mode arguably improves privacy by isolating execution in container
- However, implicit dependencies are auto-installed and reported via telemetry without explicit user consent (see Known Limitations below)

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Compromised dependency binaries | SHA256 checksum verification; installation aborts on mismatch | Users with stale registries could use outdated checksums; same risk as any tsuku install |
| Container reads tools directory | Read-only mount prevents modification; information leak is minimal (tool names/versions public) | Container can enumerate installed tools but cannot modify or exploit them |
| Upstream provider compromise | Checksum verification detects tampering; recipe PR review process | Sophisticated attacker compromising both upstream and recipe repo could succeed |
| Persistent malicious tool on host | Version-isolated installation; users can remove tools; same as normal tsuku install | If compromised tool installed, affects all sandbox runs until removed |
| Data exfiltration via build tools | Sandbox container isolation; read-only mounts for cache/tools | Build tools can write to workspace (required for functionality) but cannot modify cache or tools |

### Known Limitations and Future Enhancements

**TOCTOU Race Condition (Time-of-Check-Time-of-Use)**:
Between checksum verification during installation and container mounting, an attacker with host filesystem access could swap verified binaries with malicious ones. This affects shared systems or compromised hosts.

**Future mitigation**: Implement integrity re-verification before mounting tools to containers. Before starting container, re-hash all binaries in `~/.tsuku/tools/` that will be mounted and verify they match expected checksums.

**Multi-User Scenarios**:
This design assumes single-user `~/.tsuku/` directory. Sharing `~/.tsuku/tools/` between users creates cross-user compromise risk.

**Mitigation**: Document `~/.tsuku` as unsupported for multi-user sharing. Add startup validation that `~/.tsuku/tools/` is owned by current user and not world-writable.

**Telemetry Behavior**:
Implicit dependency installations generate telemetry events. When users run `tsuku install neovim --sandbox`, cmake is silently installed and reported, exposing the user's implicit dependency graph.

**Transparency**: This behavior is documented. Future enhancement: allow opt-out for implicit dependency telemetry separately from explicit installations.
