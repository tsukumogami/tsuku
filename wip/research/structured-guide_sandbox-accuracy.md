# Sandbox Executor Integration Accuracy Assessment

This assessment reviews the DESIGN-structured-install-guide.md for accuracy against the current sandbox executor implementation, focusing on DeriveContainerSpec() integration and container building feasibility.

## 1. Current Sandbox Architecture

### Design's Description vs Reality

The design document describes the sandbox executor at a high level and proposes changes. Here is how the description maps to reality:

**Accurate descriptions:**

1. **Base image selection**: The design correctly identifies that the current system uses `debian:bookworm-slim` (line 65, 231) and `ubuntu:22.04` (line 65). The actual code in `requirements.go` confirms this:
   - `DefaultSandboxImage = "debian:bookworm-slim"` (line 15)
   - `SourceBuildSandboxImage = "ubuntu:22.04"` (line 19)

2. **Dynamic apt-get install**: The design mentions (line 65) that packages are installed dynamically. This is accurate - `executor.go:buildSandboxScript()` (lines 278-291) generates apt-get commands:
   ```go
   sb.WriteString("apt-get update -qq\n")
   sb.WriteString(fmt.Sprintf("apt-get install -qq -y %s >/dev/null 2>&1\n\n", strings.Join(packages, " ")))
   ```

3. **Network detection via RequiresNetwork()**: The design correctly references the `NetworkValidator` interface. The actual implementation in `requirements.go:ComputeSandboxRequirements()` (lines 97-103) does query this interface.

**Outdated or incomplete descriptions:**

1. **Package determination logic**: The design implies packages come from `require_system` steps. Currently, packages are derived from:
   - `RequiresNetwork` flag (adds `ca-certificates`, `curl`) - executor.go:281
   - `hasBuildActions()` (adds `build-essential`) - executor.go:283-285

   The current system does NOT read packages from `require_system` steps - this is the gap being addressed.

2. **ComputeSandboxRequirements scope**: The design says "Extract primitives: Parse `require_system` steps from the plan." The current `ComputeSandboxRequirements()` does NOT parse `require_system` steps at all. It only checks:
   - Actions implementing `NetworkValidator`
   - Presence of build actions (configure_make, cmake_build, etc.)

3. **Container building**: The design describes building derived containers (lines 598-599, 607-610). The current implementation does NOT build containers - it uses pre-built images (`debian:bookworm-slim` or `ubuntu:22.04`) and runs `apt-get install` at container startup.

### Current Architecture Summary

```
┌─────────────────────────────────────────────────────────────────┐
│                    Current Sandbox Flow                          │
├─────────────────────────────────────────────────────────────────┤
│ 1. ComputeSandboxRequirements(plan)                             │
│    └─ Returns: Image, RequiresNetwork, Resources                │
│                                                                  │
│ 2. Executor.Sandbox(ctx, plan, reqs)                            │
│    └─ Creates workspace, writes plan.json                       │
│    └─ buildSandboxScript() generates install script             │
│    └─ Mounts: tsuku binary, workspace, download cache           │
│                                                                  │
│ 3. Runtime.Run(ctx, opts)                                       │
│    └─ podman/docker run --rm with mounts                        │
│    └─ Script runs apt-get install, then tsuku install --plan    │
└─────────────────────────────────────────────────────────────────┘
```

Key observation: The sandbox executor **runs containers**, it does not **build containers**. All package installation happens via `apt-get install` in the startup script.

## 2. DeriveContainerSpec Integration

### Where Would DeriveContainerSpec Live?

The design places `DeriveContainerSpec` at lines 613-665, showing it operating on `*executor.InstallationPlan`. Based on the current architecture:

**Recommended location**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/sandbox/requirements.go`

This aligns with existing patterns:
- `ComputeSandboxRequirements()` already lives here
- Both functions analyze plans to determine container configuration
- The sandbox package is the right abstraction level

**Alternative**: A new file `container_spec.go` in the sandbox package if the implementation grows large.

### Integration with ComputeSandboxRequirements()

The design shows `DeriveContainerSpec()` returning a `ContainerSpec` with:
- `Base` - base image
- `Packages` - map[string][]string (manager -> packages)
- `Primitives` - []Primitive

**Integration approach 1: Replace ComputeSandboxRequirements**

`DeriveContainerSpec()` could subsume `ComputeSandboxRequirements()` entirely:

```go
func DeriveContainerSpec(plan *executor.InstallationPlan) (*ContainerSpec, error) {
    // Current ComputeSandboxRequirements logic for network/resources
    reqs := computeBaseRequirements(plan)

    // NEW: Parse require_system steps
    packages, primitives, err := extractSystemDeps(plan)
    if err != nil {
        return nil, err
    }

    return &ContainerSpec{
        Base:            reqs.Image,
        Packages:        packages,
        Primitives:      primitives,
        RequiresNetwork: reqs.RequiresNetwork,
        Resources:       reqs.Resources,
    }, nil
}
```

**Integration approach 2: Compose the functions**

Keep `ComputeSandboxRequirements()` for non-system-dep concerns, call `DeriveContainerSpec()` when `require_system` steps exist:

```go
func (e *Executor) Sandbox(ctx, plan, reqs) {
    // Check for system deps
    spec, err := DeriveContainerSpec(plan)
    if err != nil {
        // UnsupportedRecipeError - cannot sandbox test
        return &SandboxResult{Skipped: true}, nil
    }
    if spec != nil {
        // Use derived container with required packages
        reqs.Image = getOrBuildImage(spec)
    }
    // ... rest of existing flow
}
```

**Recommendation**: Approach 2 is cleaner. It keeps concerns separated and allows incremental adoption.

### Data Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                    Proposed Data Flow                             │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  InstallationPlan                                                 │
│        │                                                          │
│        ├───► ComputeSandboxRequirements()                        │
│        │            │                                             │
│        │            └─► SandboxRequirements (image, network, etc)│
│        │                                                          │
│        └───► DeriveContainerSpec()                               │
│                     │                                             │
│                     ├─► nil (no require_system steps)            │
│                     ├─► ContainerSpec (packages/primitives)      │
│                     └─► UnsupportedRecipeError (legacy recipe)   │
│                                                                   │
│  ContainerSpec + SandboxRequirements                              │
│        │                                                          │
│        └───► getOrBuildImage()                                   │
│                     │                                             │
│                     └─► Cached image name or build new           │
└──────────────────────────────────────────────────────────────────┘
```

## 3. Container Building Feasibility

### Current State: Runs Containers, Does Not Build

The current implementation uses the `Runtime` interface which only supports:
- `Run(ctx context.Context, opts RunOptions) (*RunResult, error)`

There is no `Build()` method. The `validate.Runtime` interface (runtime.go:18-27) and implementations (`podmanRuntime`, `dockerRuntime`) only support `run`.

### Changes Needed to Support Container Building

**Option A: Add Build() to Runtime interface**

```go
type Runtime interface {
    Name() string
    IsRootless() bool
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
    Build(ctx context.Context, opts BuildOptions) (*BuildResult, error)  // NEW
}

type BuildOptions struct {
    Tag        string            // Image tag (e.g., "tsuku/sandbox-cache:abc123")
    Dockerfile string            // Dockerfile content or path
    Context    string            // Build context directory
    NoCache    bool              // Skip build cache
}

type BuildResult struct {
    ImageID string
}
```

Implementation for podman/docker:
```go
func (r *podmanRuntime) Build(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
    args := []string{"build", "-t", opts.Tag, "-f", "-", opts.Context}
    cmd := exec.CommandContext(ctx, r.path, args...)
    cmd.Stdin = strings.NewReader(opts.Dockerfile)
    // ...
}
```

**Option B: Keep Run-time Package Installation (Current Approach)**

Instead of building containers, continue using the current pattern but enhance it:

```go
func (e *Executor) buildSandboxScript(plan, spec) string {
    // ...
    if spec != nil {
        for manager, pkgs := range spec.Packages {
            switch manager {
            case "apt":
                sb.WriteString(fmt.Sprintf("apt-get install -y %s\n", strings.Join(pkgs, " ")))
            // ... other managers
            }
        }
    }
    // ...
}
```

**Trade-offs:**

| Aspect | Option A (Build) | Option B (Run-time install) |
|--------|------------------|----------------------------|
| Caching | Image-level (fast reuse) | No caching (reinstall each run) |
| Complexity | Higher (Dockerfile generation) | Lower (script modification) |
| CI Integration | Requires image registry | Works immediately |
| Local Dev | Needs image management | Just works |
| Reproducibility | High (immutable images) | Lower (apt repos change) |

**Recommendation**: Start with Option B for MVP, add Option A for CI/performance optimization later. The design's phase 3 (lines 765-769) implies Option A is the target.

### Dockerfile Generation (If Building)

The design shows a minimal Dockerfile (lines 585-591). A more complete version:

```dockerfile
FROM tsuku/sandbox-base:latest

# Install required packages from primitives
ARG APT_PACKAGES=""
RUN if [ -n "$APT_PACKAGES" ]; then \
      apt-get update && apt-get install -y --no-install-recommends $APT_PACKAGES && \
      rm -rf /var/lib/apt/lists/*; \
    fi

# Content-addressed: hash of package list determines tag
```

The `ContainerImageName()` function (lines 675-689) provides deterministic naming via SHA256 hash of sorted packages.

## 4. Minimal Base Container Tradeoffs

### Design's Proposal: "tsuku + glibc only"

The design (lines 581-598) proposes a minimal base container:

```dockerfile
FROM scratch
COPY --from=builder /tsuku /usr/local/bin/tsuku
COPY --from=builder /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=builder /lib64/ld-linux-x86-64.so.2 /lib64/
```

### What Tsuku Actually Needs to Run

Analyzing the codebase and runtime requirements:

**Absolute minimum for tsuku binary:**
1. `glibc` (libc.so.6) - tsuku is dynamically linked
2. `ld-linux` (ld-linux-x86-64.so.2) - dynamic linker
3. `/etc/passwd`, `/etc/group` - for user lookup (Go runtime)
4. `/tmp` - for temporary files

**For typical installations:**
1. `tar`, `gzip`, `bzip2`, `xz-utils` - extraction actions
2. `ca-certificates` - HTTPS downloads (if network needed)
3. Shell (`/bin/sh` or `/bin/bash`) - for running scripts

**For build actions:**
- `build-essential`, `gcc`, `make` - source builds
- Ecosystem-specific: `nodejs`, `cargo`, `go`, etc.

### Hidden Dependencies in Current Base Images

`debian:bookworm-slim` includes (non-exhaustive):
- `coreutils` (cp, mv, rm, chmod, etc.)
- `tar`, `gzip`
- `bash`, `dash`
- `libc6`
- Basic `/etc` files

Recipes currently "accidentally" depend on these. A truly minimal base would expose:
1. Recipes using `chmod` action need coreutils
2. Recipes using `extract` action need tar/gzip/etc.
3. Recipes with `run_command` steps need a shell

### Recommendation: Tiered Base Images

Rather than a single minimal image, consider:

**Tier 1: tsuku-base-minimal**
- glibc, ld-linux, /etc basics
- For: recipes that only use `install_binaries` with pre-extracted binaries

**Tier 2: tsuku-base-standard** (default)
- Tier 1 + coreutils + tar + gzip + ca-certificates + shell
- For: typical download-extract-install recipes

**Tier 3: tsuku-base-build**
- Tier 2 + build-essential
- For: source build recipes

The design's approach (line 303: "Option 3A") is ambitious but practical concerns suggest Tier 2 as the default.

### Impact on Existing Recipes

Current recipes with `require_system` steps (docker.toml, cuda.toml, test-tuples.toml) will:
1. Need migration to new `packages`/`primitives` syntax
2. Continue to skip sandbox testing until structured primitives are provided
3. Work for host installation (unchanged behavior)

## 5. Specific Change Recommendations

### Section: Sandbox Executor Changes (Lines 600-665)

**Issue 1: DeriveContainerSpec signature**

The design shows:
```go
func DeriveContainerSpec(plan *executor.InstallationPlan) (*ContainerSpec, error)
```

Missing: Should accept `*SandboxRequirements` or compute it internally to avoid duplicate plan iteration.

**Recommendation**: Update to:
```go
func DeriveContainerSpec(plan *executor.InstallationPlan, baseReqs *SandboxRequirements) (*ContainerSpec, error)
```

**Issue 2: Unclear integration point**

The design describes "sandbox executor is modified" but doesn't show the actual modification to `Executor.Sandbox()`.

**Recommendation**: Add pseudo-code showing where in the existing flow `DeriveContainerSpec` is called:
```go
func (e *Executor) Sandbox(ctx, plan, reqs) {
    // EXISTING: runtime detection, workspace creation

    // NEW: Check for system deps
    spec, err := DeriveContainerSpec(plan, reqs)
    if err != nil {
        if _, ok := err.(*UnsupportedRecipeError); ok {
            return &SandboxResult{Skipped: true, Error: err}, nil
        }
        return nil, err
    }

    // NEW: Get or build container image
    if spec != nil && len(spec.Packages) > 0 {
        image, err := e.getOrBuildImage(ctx, spec)
        if err != nil {
            return nil, err
        }
        reqs.Image = image
    }

    // EXISTING: build script, run container
}
```

### Section: Minimal Base Container (Lines 581-598)

**Issue**: The Dockerfile example is too minimal and will fail for most recipes.

**Recommendation**: Update to show the practical minimum:
```dockerfile
FROM debian:bookworm-slim AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=builder /lib64/ld-linux-x86-64.so.2 /lib64/
COPY --from=builder /etc/passwd /etc/
COPY --from=builder /etc/group /etc/
COPY tsuku /usr/local/bin/tsuku

# Add /bin/sh for script execution
COPY --from=builder /bin/dash /bin/sh
```

### Section: Container Image Caching (Lines 670-692)

**Issue**: No mention of cache location or lifecycle management.

**Recommendation**: Add details:
```go
// Cache images locally in podman/docker image store
// For CI, push to GHCR: ghcr.io/tsukumogami/sandbox-cache:<hash>

func (e *Executor) getOrBuildImage(ctx context.Context, spec *ContainerSpec) (string, error) {
    name := ContainerImageName(spec)

    // Check if image exists locally
    if e.imageExists(ctx, name) {
        return name, nil
    }

    // Check remote registry (CI mode)
    if e.registryURL != "" {
        remoteName := e.registryURL + "/" + name
        if err := e.pullImage(ctx, remoteName); err == nil {
            return remoteName, nil
        }
    }

    // Build locally
    return e.buildImage(ctx, spec, name)
}
```

### Missing Implementation Details

**1. Primitive execution in sandbox**

The design mentions "Primitive execution" (lines 486-516) but doesn't show how primitives are executed in the sandbox context. Add:

```go
// In sandbox context, primitives run as root (container user)
// No sudo needed - the container runs as root
func (p *AptPrimitive) ExecuteInSandbox(ctx *SandboxContext) error {
    args := append([]string{"install", "-y"}, p.Packages...)
    return ctx.Run("apt-get", args...)
}
```

**2. require_system action changes**

The design doesn't show how the existing `require_system.go` changes. The current implementation:
- Only checks `install_guide` parameter
- Uses `GetMapStringString(params, "install_guide")`

Add section showing:
```go
// NEW in require_system.go
func (a *RequireSystemAction) Preflight(params) *PreflightResult {
    // ... existing command check

    hasPackages := params["packages"] != nil
    hasPrimitives := params["primitives"] != nil

    if hasPackages && hasPrimitives {
        result.AddError("'packages' and 'primitives' are mutually exclusive")
    }

    // Deprecation warning for install_guide
    if params["install_guide"] != nil {
        result.AddWarning("'install_guide' is deprecated; migrate to 'packages' or 'primitives'")
    }
}
```

**3. Error handling for unsupported recipes**

Add explicit handling for recipes that cannot be sandbox-tested:

```go
type UnsupportedRecipeError struct {
    Recipe  string
    Command string
    Reason  string
}

func (e *UnsupportedRecipeError) Error() string {
    return fmt.Sprintf("recipe %s cannot be sandbox-tested: %s requires %s",
        e.Recipe, e.Command, e.Reason)
}

// Recipes with require_system but no packages/primitives are unsupported
// They can still be installed on host (current behavior)
```

## Summary

The design document is directionally correct but has several gaps:

1. **Current architecture description**: Partially accurate. Correctly describes base images but misses that packages are determined at runtime via script generation, not from `require_system` steps.

2. **DeriveContainerSpec**: Well-designed function but integration point unclear. Should live in `internal/sandbox/requirements.go` and compose with existing `ComputeSandboxRequirements()`.

3. **Container building**: Major architectural change. Current system runs containers, doesn't build them. Recommend starting with run-time installation (Option B) for MVP.

4. **Minimal base container**: Overly aggressive. A "standard" tier with coreutils, tar, ca-certificates is more practical than pure scratch+glibc.

5. **Missing details**: Primitive execution in sandbox, require_system.go changes, error handling, cache lifecycle.
