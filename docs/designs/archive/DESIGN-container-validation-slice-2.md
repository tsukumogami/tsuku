# Design Document: Container Validation (Slice 2)

**Status**: Superseded by [DESIGN-install-sandbox.md](../current/DESIGN-install-sandbox.md)

**Parent Issue**: [#268 - Slice 2: Container Validation](https://github.com/tsukumogami/tsuku/issues/268)

**Parent Design**: [DESIGN-llm-builder-infrastructure.md](DESIGN-llm-builder-infrastructure.md)

<a id="implementation-issues"></a>
**Implementation Issues**:

| Issue | Title | Design Section | Dependencies |
|-------|-------|----------------|--------------|
| [#302](https://github.com/tsukumogami/tsuku/issues/302) | Implement container runtime abstraction and detection | [Runtime Detector](#runtime-detector-internalvalidateruntimego) | None |
| [#303](https://github.com/tsukumogami/tsuku/issues/303) | Implement asset pre-download with checksum capture | [Pre-Download](#pre-download-internalvalidatepredownloadgo) | None |
| [#304](https://github.com/tsukumogami/tsuku/issues/304) | Implement parallel safety with lock files | [Parallel Safety](#parallel-safety-internalvalidatelockgo) | None |
| [#305](https://github.com/tsukumogami/tsuku/issues/305) | Implement Podman runtime | [Components](#components) | #302 |
| [#306](https://github.com/tsukumogami/tsuku/issues/306) | Implement Docker runtime | [Components](#components) | #302 |
| [#307](https://github.com/tsukumogami/tsuku/issues/307) | Implement startup cleanup for orphaned containers | [Startup Cleanup](#startup-cleanup) | #304 |
| [#308](https://github.com/tsukumogami/tsuku/issues/308) | Implement container executor for recipe validation | [Container Executor](#container-executor-internalvalidatecontainergo) | #303, #305, #306 |

```
Dependency Graph:

#302 (Runtime abstraction) ──┬──> #305 (Podman runtime) ───┐
                             │                              │
                             └──> #306 (Docker runtime) ────┼──> #308 (Container executor)
                                                            │
#303 (Pre-download) ────────────────────────────────────────┘

#304 (Lock manager) ──> #307 (Startup cleanup)
```

## Context and Problem Statement

Slice 1 proved that LLMs can generate working recipes from GitHub release data. Now we need to validate these recipes before presenting them to users. Container-based validation provides isolation to safely test untrusted recipes.

### The Critical Requirement

**Running containers without sudo** is essential for tsuku's philosophy of being a self-contained package manager that doesn't require system dependencies or elevated privileges.

### The Technical Challenge

Rootless containers on Linux require system-level configuration that cannot be installed without root:

1. **Subordinate UID/GID mappings** (`/etc/subuid`, `/etc/subgid`) - Required for multi-UID containers
2. **SETUID binaries** (`newuidmap`, `newgidmap`) - Required to map UIDs in user namespaces
3. **Kernel configuration** (`kernel.unprivileged_userns_clone=1`) - Required on some distributions

This creates a chicken-and-egg problem: tsuku wants to avoid requiring sudo, but rootless containers require one-time system configuration with sudo.

### Scope

**In scope:**
- Container runtime abstraction (Docker + Podman)
- Auto-detection of available container runtimes
- Asset pre-download with checksum capture
- Container isolation (`--network=none`, resource limits)
- Parallel execution safety (lock files, cleanup)
- Graceful degradation when containers unavailable

**Out of scope:**
- Cross-platform validation (validating on multiple architectures)
- Automatic container runtime installation

## External Research

### Rootless Container Options

#### Option A: Podman Rootless

[Podman](https://podman.io/) supports rootless operation natively but requires:
- `/etc/subuid` and `/etc/subgid` entries (requires root to configure)
- `newuidmap` and `newgidmap` binaries with SETUID capability
- Kernel with `kernel.unprivileged_userns_clone=1`

**Static binary option**: [podman-static](https://github.com/mgoltzsche/podman-static) provides portable binaries including all dependencies (crun, conmon, fuse-overlayfs, netavark, pasta). Can be installed in `$HOME` without root, but still requires system-level UID mapping configuration.

#### Option B: Docker Rootless

[Docker rootless mode](https://docs.docker.com/engine/security/rootless/) has similar requirements:
- `/etc/subuid` and `/etc/subgid` entries
- `newuidmap` and `newgidmap` binaries
- `docker-ce-rootless-extras` package

Docker rootless is more complex to set up than Podman and requires the daemon to run as a user service.

#### Option C: Bubblewrap (bwrap)

[Bubblewrap](https://github.com/containers/bubblewrap) is a low-level sandboxing tool used by Flatpak. It can work in two modes:
- **User namespace mode**: Requires unprivileged user namespaces (no SETUID needed)
- **SETUID mode**: Works on systems without user namespaces but requires SETUID installation

Bubblewrap provides namespace isolation but is not a full container runtime - no image management, no standardized container format.

#### Option D: Single-UID Containers

Some container runtimes support "single-UID" mode where only UID 0 inside the container maps to the user's UID outside. This works without `/etc/subuid` but limits what can run inside the container.

### Key Finding: Root is Unavoidable for First-Time Setup

All production-ready rootless container solutions require one-time system configuration with root privileges:

| Requirement | Can tsuku install without root? |
|-------------|--------------------------------|
| Container runtime binaries | Yes (podman-static) |
| `/etc/subuid` and `/etc/subgid` | No |
| `newuidmap` / `newgidmap` | No |
| Kernel parameters | No |

### Research Summary

1. **Podman is the best choice** for rootless containers due to native support and static binary availability
2. **One-time sudo is unavoidable** for proper rootless setup on most systems
3. **Graceful degradation** is essential - validation should work when possible, skip gracefully when not
4. **Detection before installation** - tsuku should detect existing container support rather than trying to install it

## Considered Options

### Decision 1: Container Runtime Strategy

#### Option 1A: Auto-Detect with Preference Order

Detect available container runtimes in preference order: Podman (rootless) > Docker (rootless) > Docker (with group) > None.

```go
type RuntimeDetector interface {
    Detect() (Runtime, error)
    AvailableRuntimes() []Runtime
}

// Detection order
// 1. podman (if available and rootless works)
// 2. docker (if rootless mode configured)
// 3. docker (if user in docker group)
// 4. none (skip validation with warning)
```

**Pros:**
- Uses existing system configuration
- Prefers more secure options (rootless)
- No sudo required by tsuku
- Works with whatever the user has

**Cons:**
- Users without containers get degraded experience
- Complex detection logic

#### Option 1B: Require Container Runtime as Prerequisite

Document that container validation requires Docker or Podman. Fail if not available.

**Pros:**
- Simple implementation
- Clear requirements

**Cons:**
- Poor user experience
- Violates tsuku's self-contained philosophy

#### Option 1C: Bundle Podman-Static with Auto-Setup

Download and install podman-static to `$TSUKU_HOME`, prompt for sudo to configure `/etc/subuid`.

**Pros:**
- Self-contained installation
- One-time setup

**Cons:**
- Requires sudo (even if just once)
- Complex setup process
- Maintenance burden for bundled binaries
- Architecture-specific downloads

### Decision 2: Handling Missing Container Runtime

#### Option 2A: Skip Validation with Warning

If no container runtime is available, skip validation and warn the user.

```
Warning: Container runtime not available. Skipping recipe validation.
  To enable validation, install Podman or Docker.
  Generated recipes may not work correctly.
```

**Pros:**
- Doesn't block users without containers
- Clear communication of risk
- Matches `--skip-sandbox` flag behavior

**Cons:**
- Users may ignore warnings
- Inconsistent experience

#### Option 2B: Require Explicit Opt-Out

Require `--skip-sandbox` flag when no runtime is available.

**Pros:**
- Forces user acknowledgment
- Explicit consent for risk

**Cons:**
- Frustrating for users who can't install containers
- Requires extra typing every time

#### Option 2C: Static Validation Only

Fall back to enhanced static validation (schema checks, URL validation, pattern matching).

**Pros:**
- Always provides some validation
- No external dependencies

**Cons:**
- Cannot catch runtime failures
- False confidence in validation

### Decision 3: Rootless Detection Strategy

How should we detect if rootless containers actually work?

#### Option 3A: Try Running a Simple Container

```go
func (d *Detector) checkRootless(runtime string) bool {
    // Try to run: podman run --rm alpine echo ok
    cmd := exec.Command(runtime, "run", "--rm", "alpine", "echo", "ok")
    return cmd.Run() == nil
}
```

**Pros:**
- Definitive test
- Catches all configuration issues

**Cons:**
- Requires network to pull alpine image
- Slow (~5-10 seconds)
- May fail for unrelated reasons

#### Option 3B: Check Configuration Files

```go
func (d *Detector) checkRootless() bool {
    // Check /etc/subuid has entry for current user
    // Check newuidmap exists and has capabilities
    return hasSubuidEntry(uid) && hasNewuidmap()
}
```

**Pros:**
- Fast
- No network required
- No container pull

**Cons:**
- May miss some configuration issues
- False positives possible

#### Option 3C: Hybrid Approach

Check configuration first, then verify with a simple container run if config looks good.

**Pros:**
- Fast path for known-bad configurations
- Definitive verification when needed

**Cons:**
- More complex implementation

### Decision 4: Container Base Image

What image should validation containers use?

#### Option 4A: Alpine-Based Minimal Image

Use `alpine:latest` or a custom minimal image.

**Pros:**
- Small (~5MB)
- Fast to pull
- Sufficient for binary validation

**Cons:**
- musl libc may cause compatibility issues
- Missing common tools

#### Option 4B: Distroless or Scratch

Use distroless base or build from scratch with only needed binaries.

**Pros:**
- Minimal attack surface
- Tiny image size

**Cons:**
- Hard to debug
- May lack needed tools

#### Option 4C: Match Host Distribution

Detect host distribution and use matching container image.

**Pros:**
- Best compatibility
- glibc matching

**Cons:**
- Large images (Ubuntu ~77MB, Fedora ~180MB)
- Complex detection
- Slower pulls

### Evaluation Summary

| Decision | Option A | Option B | Option C |
|----------|----------|----------|----------|
| **D1: Runtime Strategy** | Best balance | Too restrictive | Too complex |
| **D2: Missing Runtime** | User-friendly | Explicit but frustrating | Insufficient |
| **D3: Rootless Detection** | Simple but slow | Fast but incomplete | Best balance |
| **D4: Base Image** | Good default | Too minimal | Overkill |

## Decision Outcome

**Chosen: 1A (Auto-Detect) + 2A (Skip with Warning) + 3C (Hybrid Detection) + 4A (Alpine)**

### Summary

Tsuku will auto-detect available container runtimes in preference order (Podman rootless > Docker rootless > Docker group). When no runtime is available, validation is skipped with a clear warning. Detection uses a hybrid approach: check configuration first, verify with a quick container run. Alpine serves as the base image for simplicity and speed.

### Rationale

1. **Auto-detection (1A)** respects tsuku's philosophy of working with existing system configuration rather than requiring installation.

2. **Skip with warning (2A)** provides the best user experience - users aren't blocked, but are informed of the risk. This matches the existing `--skip-sandbox` flag semantics.

3. **Hybrid detection (3C)** balances speed with accuracy. Fast rejection of obviously-broken configurations, definitive verification when config looks good.

4. **Alpine base (4A)** provides the right balance of size, speed, and functionality for binary validation.

### Trade-offs Accepted

1. **No automatic container installation**: Users must install and configure containers themselves. This is documented in setup instructions.

2. **Degraded experience without containers**: Users without container runtimes get warnings but can still use the feature.

3. **Alpine compatibility risk**: Some binaries linked against specific glibc versions may fail. This is acceptable for validation purposes.

## Solution Architecture

### Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Container Validation Pipeline                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │   Runtime    │───▶│  Pre-        │───▶│  Container   │      │
│  │   Detector   │    │  Download    │    │  Executor    │      │
│  └──────────────┘    └──────────────┘    └──────────────┘      │
│         │                   │                   │               │
│         ▼                   ▼                   ▼               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │   Podman     │    │   Assets     │    │   Result     │      │
│  │   Docker     │    │   Checksums  │    │   Parser     │      │
│  │   None       │    │   Temp Dir   │    │   Cleanup    │      │
│  └──────────────┘    └──────────────┘    └──────────────┘      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Components

#### Runtime Detector (`internal/validate/runtime.go`)

```go
type Runtime interface {
    Name() string
    IsRootless() bool
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}

type RuntimeDetector struct {
    // Cached detection result
    detected Runtime
    checked  bool
}

func (d *RuntimeDetector) Detect() (Runtime, error) {
    // 1. Try Podman
    if r := d.tryPodman(); r != nil {
        return r, nil
    }

    // 2. Try Docker rootless
    if r := d.tryDockerRootless(); r != nil {
        return r, nil
    }

    // 3. Try Docker with group
    if r := d.tryDockerGroup(); r != nil {
        return r, nil
    }

    // 4. No runtime available
    return nil, ErrNoRuntime
}
```

#### Pre-Download (`internal/validate/predownload.go`)

```go
type PreDownloader struct {
    httpClient *http.Client
    tempDir    string
}

type DownloadResult struct {
    AssetPath  string
    Checksum   string // SHA256
    Size       int64
}

func (p *PreDownloader) Download(ctx context.Context, recipe *recipe.Recipe) (*DownloadResult, error) {
    // 1. Parse download URL from recipe
    // 2. Download to temp directory
    // 3. Compute SHA256 checksum
    // 4. Return result with checksum for embedding
}
```

#### Container Executor (`internal/validate/container.go`)

```go
type Executor struct {
    runtime Runtime
    limits  ResourceLimits
}

type ResourceLimits struct {
    Memory  string        // "2g"
    CPUs    string        // "2"
    Timeout time.Duration // 5 * time.Minute
}

type RunOptions struct {
    Image      string
    Command    []string
    Mounts     []Mount
    Network    string // "none" for isolation
    WorkDir    string
    Env        []string
}

func (e *Executor) Validate(ctx context.Context, recipe *recipe.Recipe, assets string) (*ValidationResult, error) {
    // 1. Create container with isolation
    // 2. Mount pre-downloaded assets
    // 3. Run recipe steps
    // 4. Check verification command
    // 5. Collect output for error parsing
    // 6. Cleanup
}
```

#### Parallel Safety (`internal/validate/lock.go`)

```go
type LockManager struct {
    lockDir string // $TSUKU_HOME/validate/locks
}

func (m *LockManager) Acquire(containerID string) (*Lock, error) {
    // Create lock file with flock
    // Write container ID
}

func (m *LockManager) Cleanup() error {
    // Find orphaned containers (exited/dead state)
    // Only cleanup if we can acquire lock
}
```

### Data Flow

```
1. User: tsuku create mytool --from github

2. LLM generates recipe (Slice 1)

3. RuntimeDetector.Detect()
   ├─ Podman available? → Use Podman
   ├─ Docker rootless? → Use Docker
   ├─ Docker group? → Use Docker (warn about security)
   └─ None → Skip validation (warn user)

4. PreDownloader.Download(recipe)
   ├─ Parse URL from recipe steps
   ├─ Download to $TMPDIR/tsuku-validate-{pid}-{ts}/assets/
   └─ Compute SHA256, return checksum

5. Executor.Validate(recipe, assets)
   ├─ Create container:
   │   podman run --rm \
   │     --network=none \
   │     --memory=2g \
   │     --cpus=2 \
   │     --read-only \
   │     -v $assets:/assets:ro \
   │     -v $workspace:/workspace \
   │     alpine:latest \
   │     /workspace/install.sh
   ├─ Run recipe steps in container
   ├─ Run verification command
   └─ Return result (pass/fail + output)

6. Cleanup
   ├─ Remove container
   └─ Remove temp directories
```

### Container Isolation

```bash
# Podman/Docker run command template
podman run --rm \
  --network=none \           # No network access
  --ipc=none \               # No IPC namespace sharing
  --memory=2g \              # Memory limit
  --cpus=2 \                 # CPU limit
  --pids-limit=100 \         # Process limit
  --read-only \              # Read-only root filesystem
  --tmpfs /tmp:rw,size=1g \  # Writable tmp with size limit
  -v /assets:/assets:ro \    # Pre-downloaded assets (read-only)
  -v /workspace:/workspace \ # Working directory
  -e TSUKU_VALIDATION=1 \    # Flag for recipe steps
  alpine:latest \
  /bin/sh -c "..."
```

### Startup Cleanup

On tsuku startup, clean orphaned validation artifacts:

```go
func (v *Validator) StartupCleanup() {
    // 1. List containers with tsuku-validate prefix
    // 2. For each in exited/dead state:
    //    a. Try to acquire lock
    //    b. If acquired, remove container
    // 3. List temp directories matching tsuku-validate-*
    // 4. Remove directories older than 1 hour
}
```

## Security Considerations

### Container Escape Risks

**Mitigations:**
- `--network=none`: No network access
- `--ipc=none`: No IPC namespace sharing
- `--read-only`: Read-only root filesystem
- Resource limits: Prevent resource exhaustion
- No privileged mode: Never use `--privileged`

**Residual risk:** Container runtime vulnerabilities could enable escape. This is accepted as containers are industry-standard isolation.

### Pre-Downloaded Asset Integrity

**Mitigations:**
- SHA256 checksums computed at download time
- Checksums embedded in generated recipe
- Assets mounted read-only in container

**Residual risk:** TOCTOU between validation and installation (deferred to M9).

### Untrusted Recipe Execution

**Mitigations:**
- All recipe output captured and sanitized before repair loop
- No host paths exposed except explicitly mounted directories
- Container environment isolated from host secrets

### Docker Group Security Warning

If Docker is available only via group membership (not rootless), warn users:

```
Warning: Using Docker with docker group membership.
  This grants root-equivalent access on this machine.
  Consider configuring Docker rootless mode for better security.
  See: https://docs.docker.com/engine/security/rootless/
```

## Implementation Plan

### Files to Create

1. `internal/validate/runtime.go` - Runtime abstraction and detection
2. `internal/validate/podman.go` - Podman runtime implementation
3. `internal/validate/docker.go` - Docker runtime implementation
4. `internal/validate/predownload.go` - Asset pre-download with checksums
5. `internal/validate/container.go` - Container execution and validation
6. `internal/validate/lock.go` - Lock file management for parallel safety
7. `internal/validate/cleanup.go` - Orphan cleanup on startup

### Exit Criteria

- [ ] Runtime auto-detection works for Podman and Docker
- [ ] Rootless detection correctly identifies configuration issues
- [ ] Pre-download captures checksums for embedding
- [ ] Container validation identifies broken recipes
- [ ] Container validation passes working recipes
- [ ] Parallel tsuku instances don't interfere
- [ ] Orphaned containers are cleaned on startup
- [ ] Clear warnings when validation is skipped

## Consequences

### Positive

1. **No sudo required by tsuku**: Detection and execution work with existing configuration
2. **Graceful degradation**: Users without containers still get LLM recipes (with warnings)
3. **Security by default**: Prefers rootless, warns about docker group
4. **Portable**: Works with both Podman and Docker

### Negative

1. **System dependency for full functionality**: Container validation requires pre-installed runtime
2. **Inconsistent experience**: Varies based on available container support
3. **Detection complexity**: Multiple code paths for different runtime configurations

### Mitigations

1. **Clear documentation**: Setup guide explains container options
2. **Helpful warnings**: Messages guide users to configure containers
3. **Static validation fallback**: Some validation always runs
