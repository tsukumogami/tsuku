# Issue #771 Investigation Report

## Executive Summary

**What #770 delivered:**
- Complete container building infrastructure for pre-installing system dependencies
- Plan filtering to generate platform-specific installation plans
- Container spec derivation from package requirements
- Container image caching with deterministic naming
- Integration with sandbox executor to build custom containers

**What #771 needs to implement:**
- The `ExecuteInSandbox()` method itself does NOT exist yet
- System actions currently have stub `Execute()` methods that just print messages
- Need to create `SandboxContext` type and add execution capability to actions
- Need to implement GPG key verification for apt_repo
- Container building works; action execution inside containers does not

**Current state of system actions:**
- They are STUBS, not verification actions
- `apt_install.Execute()` prints "Would install via apt: [packages]" and returns nil
- `apt_repo.Execute()` prints "Would add APT repository" and returns nil
- NO command verification happens; NO actual execution happens

## Investigation Results

### 1. What does ExtractPackages() do?

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/packages.go`

**Purpose:** Collects all package requirements from a filtered installation plan.

**Input:** An `*executor.InstallationPlan` that has already been filtered for the target platform and linux_family.

**Output:** `map[string][]string` where:
- Keys are package manager names: `"apt"`, `"brew"`, `"dnf"`, `"pacman"`, `"apk"`, `"zypper"`
- Values are lists of package names for that manager
- Returns `nil` if no system dependency actions found (distinguishes "no packages" from "empty list")

**What it extracts:**
```go
switch step.Action {
case "apt_install":
    packages["apt"] = append(packages["apt"], pkgs...)
case "apt_repo", "apt_ppa":
    // Signals apt usage but no packages directly
    hasSystemDeps = true
case "brew_install", "brew_cask":
    packages["brew"] = append(packages["brew"], pkgs...)
// ... etc for dnf, pacman, apk, zypper
}
```

**Example:**
```go
// Input plan with steps:
// - apt_install with packages=["curl", "jq"]
// - apt_install with packages=["git"]
// Output:
map[string][]string{"apt": ["curl", "jq", "git"]}
```

### 2. What does DeriveContainerSpec() do?

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/container_spec.go`

**Purpose:** Creates a container specification from extracted packages.

**Input:** The `map[string][]string` from `ExtractPackages()`

**Output:** `*ContainerSpec` containing:
```go
type ContainerSpec struct {
    BaseImage     string              // e.g., "debian:bookworm-slim"
    LinuxFamily   string              // e.g., "debian"
    Packages      map[string][]string // Original package map
    BuildCommands []string            // Docker RUN commands
}
```

**What it does:**
1. **Infers linux_family** from package managers (e.g., "apt" → "debian")
2. **Validates compatibility** (error if mixing apt + dnf)
3. **Selects base image** from `familyToBaseImage` map:
   - `debian` → `debian:bookworm-slim`
   - `rhel` → `fedora:41`
   - `arch` → `archlinux:base`
   - `alpine` → `alpine:3.19`
   - `suse` → `opensuse/leap:15`
4. **Generates Docker RUN commands** via `generateBuildCommands()`:
   - Debian: `"RUN apt-get update && apt-get install -y curl jq"`
   - Fedora: `"RUN dnf install -y <packages>"`
   - Arch: `"RUN pacman -Sy --noconfirm <packages>"`
   - Alpine: `"RUN apk add --no-cache <packages>"`
   - SUSE: `"RUN zypper install -y <packages>"`

**Example:**
```go
// Input:
packages := map[string][]string{"apt": ["curl", "jq"]}

// Output:
&ContainerSpec{
    BaseImage:     "debian:bookworm-slim",
    LinuxFamily:   "debian",
    Packages:      {"apt": ["curl", "jq"]},
    BuildCommands: ["RUN apt-get update && apt-get install -y curl jq"],
}
```

**YES - it converts apt_install actions to Docker RUN commands.**

### 3. How are containers currently built?

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/executor.go` (lines 156-192)

**Process:**
1. Extract packages from the already-filtered plan: `packages := ExtractPackages(plan)`
2. If packages exist:
   - Derive container spec: `spec, err := DeriveContainerSpec(packages)`
   - Generate cache image name: `imageName := ContainerImageName(spec)`
   - Check if cached: `exists, err := runtime.ImageExists(ctx, imageName)`
   - Build if needed: `runtime.Build(ctx, imageName, spec.BaseImage, spec.BuildCommands)`
3. If no packages, use default image from `reqs.Image`

**Runtime.Build() implementation** (`internal/validate/runtime.go`):
```go
func (r *podmanRuntime) Build(ctx, imageName, baseImage string, buildCommands []string) error {
    // Generate Dockerfile content
    dockerfile := generateDockerfile(baseImage, buildCommands)
    // dockerfile = "FROM debian:bookworm-slim\nRUN apt-get update && apt-get install -y curl jq\n"

    // Build using stdin Dockerfile
    cmd := exec.CommandContext(ctx, r.path, "build", "-t", imageName, "-f", "-", ".")
    cmd.Stdin = strings.NewReader(dockerfile)
    return cmd.Run()
}
```

**What gets passed as buildCommands:**
- Actual Docker RUN commands like `"RUN apt-get update && apt-get install -y curl jq"`
- NOT shell scripts or action invocations
- These are literal Dockerfile instructions

**So #770 already converts system actions to Docker RUN commands? YES.**

### 4. System Action Implementations

**Location:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/apt_actions.go`

#### apt_install.Execute() (lines 47-56)

```go
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    packages, ok := GetStringSlice(params, "packages")
    if !ok {
        return fmt.Errorf("apt_install action requires 'packages' parameter")
    }

    fmt.Printf("   Would install via apt: %v\n", packages)
    fmt.Printf("   (Skipped - requires sudo and system modification)\n")
    return nil  // <-- STUB: always returns nil
}
```

**Does it check if commands exist?** NO.
**Does it verify anything?** NO.
**Is it a stub?** YES - just prints and returns nil.

#### apt_repo.Execute() (lines 132-137)

```go
func (a *AptRepoAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    url := params["url"].(string)
    fmt.Printf("   Would add APT repository: %s\n", url)
    fmt.Printf("   (Skipped - requires sudo and system modification)\n")
    return nil  // <-- STUB: always returns nil
}
```

**Same story - it's a stub.**

### 5. What's Missing for #771?

Based on the investigation:

#### Missing Component #1: SandboxContext Type

**Does NOT exist in codebase.** The design shows:
```go
type SandboxContext struct {
    runtime Runtime
    imageName string
}

func (ctx *SandboxContext) Run(command string, args ...string) error {
    // Execute command inside the running container
}
```

But this type is not implemented.

#### Missing Component #2: ExecuteInSandbox() Method

**Does NOT exist anywhere.** Neither `Action` nor `SystemAction` interfaces have this method.

Current interfaces:
```go
type Action interface {
    Name() string
    Preflight(params) *PreflightResult
    Execute(ctx *ExecutionContext, params) error  // <-- Stub implementation
}

type SystemAction interface {
    Action
    Validate(params) error
    ImplicitConstraint() *Constraint
    Describe(params) string
    // ExecuteInSandbox MISSING
}
```

#### Missing Component #3: GPG Key Verification

The `apt_repo` action requires:
- Download GPG key from `key_url`
- Compute SHA256 hash
- Compare to `key_sha256` parameter
- Fail if mismatch
- Import key if match

**None of this is implemented.**

#### Missing Component #4: Actual Execution Logic

All system actions need ExecuteInSandbox implementations:
- `apt_install`: Run `apt-get update && apt-get install -y <packages>` inside container
- `apt_repo`: Download key, verify hash, import, add repo inside container
- `apt_ppa`: Run `add-apt-repository ppa:<ppa>` inside container
- `brew_install`, `dnf_install`, etc.

### 6. apt_repo and GPG Verification

**Does apt_repo action exist?** YES - at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/apt_actions.go` (lines 67-151)

**Does it handle GPG keys?** Partially:
- It validates that `key_url` and `key_sha256` parameters exist
- It validates that both URLs use HTTPS
- It has a stub `Execute()` method that just prints
- It does NOT download keys, verify hashes, or import keys

**Is this part of what #771 needs to implement?** YES - issue #771 specifically says:
> "GPG key verification: download key, compute SHA256, verify against key_sha256 before import"

**Implementation needed:**
```go
func (a *AptRepoAction) ExecuteInSandbox(ctx *SandboxContext, params map[string]interface{}) error {
    keyURL := params["key_url"].(string)
    expectedHash := params["key_sha256"].(string)

    // Download key
    keyData, err := ctx.Download(keyURL)
    if err != nil {
        return fmt.Errorf("failed to download GPG key: %w", err)
    }

    // Compute SHA256
    hash := sha256.Sum256(keyData)
    actualHash := hex.EncodeToString(hash[:])

    // Verify hash
    if actualHash != expectedHash {
        return fmt.Errorf("GPG key hash mismatch: expected %s, got %s", expectedHash, actualHash)
    }

    // Import key and add repo
    // ... actual apt-key / gpg commands
}
```

## Detailed Comparison: #770 vs #771

### What #770 Delivered (Container Building Infrastructure)

**Problem it solved:** "How do we build containers with system dependencies pre-installed?"

**Components added:**

1. **ExtractPackages()** - Extracts package requirements from filtered plans
2. **DeriveContainerSpec()** - Converts packages to container specifications with Docker RUN commands
3. **ContainerImageName()** - Generates deterministic cache keys
4. **Runtime.Build()** - Builds container images from Dockerfiles
5. **Runtime.ImageExists()** - Checks for cached images
6. **Plan filtering** - Plans are filtered during generation for target platform/family
7. **Sandbox executor integration** - Executor builds custom containers when packages present

**Result:** The sandbox can now:
- Detect that a recipe needs apt packages
- Build a Debian container with those packages installed
- Cache the built container
- Use the custom container for sandbox execution

**Example flow:**
```
Recipe has: apt_install with packages=["curl", "jq"]
    ↓
ExtractPackages() returns: {"apt": ["curl", "jq"]}
    ↓
DeriveContainerSpec() returns:
    BaseImage: "debian:bookworm-slim"
    BuildCommands: ["RUN apt-get update && apt-get install -y curl jq"]
    ↓
Runtime.Build() creates container with curl and jq installed
    ↓
Sandbox runs in that container (curl and jq are available)
```

**What #770 did NOT do:**
- It doesn't execute apt_install actions inside containers
- It doesn't verify GPG keys
- It doesn't implement ExecuteInSandbox()
- Actions are still stubs

### What #771 Needs to Implement (Action Execution in Sandbox)

**Problem to solve:** "How do actions execute their operations inside the built containers?"

**Components to add:**

1. **SandboxContext type** - Wrapper around Runtime with execution primitives
2. **ExecuteInSandbox() method** - New interface method for sandbox execution
3. **Actual execution logic** - Replace stub Execute() with real ExecuteInSandbox()
4. **GPG key verification** - Download, hash, verify, import
5. **Error handling** - Capture and wrap errors with context
6. **Container mocking** - Test infrastructure for unit tests

**Result:** Actions will be able to:
- Run apt-get commands inside containers
- Download and verify GPG keys
- Add repositories
- Install packages
- Return errors if operations fail

**Example flow:**
```
Container built by #770 (has Debian base, no packages yet)
    ↓
Plan has: apt_repo + apt_install steps
    ↓
apt_repo.ExecuteInSandbox():
    - Downloads GPG key from key_url
    - Computes SHA256 hash
    - Verifies against key_sha256
    - Fails if mismatch
    - Imports key if match
    - Adds repository
    ↓
apt_install.ExecuteInSandbox():
    - Runs "apt-get update"
    - Runs "apt-get install -y <packages>"
    - Returns error if install fails
    ↓
Container now has packages installed
```

## Architecture Clarification

### The Confusion

The issue #771 description says:
> "Actions need to execute their operations (apt-get install, etc.) inside the sandbox container."

But #770 already makes apt-get install commands run - as Docker RUN commands during container BUILD.

**Key distinction:**

**#770 approach (BUILD time):**
```dockerfile
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y curl jq
```
Packages are installed during container image build.

**#771 approach (RUN time - NOT IMPLEMENTED):**
```bash
# Inside running container:
apt-get update
apt-get install -y curl jq
```
Actions execute inside running container.

### Why Both?

**For simple packages (curl, jq, git):**
- #770 installs them at BUILD time
- Container image is cached
- Subsequent runs use cached image (fast)

**For complex operations (apt_repo with GPG keys):**
- Can't do GPG verification at BUILD time (no verification logic exists)
- Need #771 to run at RUN time with full verification
- GPG hash verification must happen before import

**For recipes that mix both:**
```toml
[[steps]]
action = "apt_repo"
url = "https://download.docker.com/linux/ubuntu"
key_url = "https://download.docker.com/linux/ubuntu/gpg"
key_sha256 = "1500c1f56fa9e26b9b8f42452a553675796ade0807cdaf10..."

[[steps]]
action = "apt_install"
packages = ["docker-ce"]
```

Currently with #770:
- Both steps are detected as "system dependencies"
- Container is built... but apt_repo doesn't add the repo
- apt_install can't find docker-ce package
- FAIL

With #771:
- apt_repo.ExecuteInSandbox() runs inside container
- Downloads GPG key, verifies hash, imports key, adds repo
- apt_install.ExecuteInSandbox() runs apt-get install
- SUCCESS

## Summary Answers

### 1. What #770 delivered?

**Container building infrastructure:**
- Package extraction from filtered plans
- Container spec derivation with Docker RUN commands
- Container image building and caching
- Integration with sandbox executor
- Plan filtering for platform/family awareness

**Result:** Sandbox can build custom containers with packages pre-installed via Docker RUN commands.

### 2. What #771 needs to implement?

**Action execution infrastructure:**
- Create `SandboxContext` type
- Add `ExecuteInSandbox()` method to system actions
- Implement actual execution logic (replace stubs)
- GPG key verification for apt_repo
- Error handling and context
- Container mocking for tests

**Result:** Actions can execute operations inside running containers with full verification.

### 3. Are system actions verification or stubs?

**STUBS.**

Current state:
```go
func (a *AptInstallAction) Execute(...) error {
    fmt.Printf("   Would install via apt: %v\n", packages)
    return nil  // <-- Always succeeds, does nothing
}
```

They do NOT:
- Check if commands exist
- Verify anything is installed
- Execute any operations
- Return errors

They ARE:
- Placeholder implementations
- Waiting for #771 to add ExecuteInSandbox()

## Implementation Implications

### For #771 Implementation

1. **Create SandboxContext** - Needs to wrap Runtime and provide Run() method
2. **Add ExecuteInSandbox to interface** - Either extend SystemAction or create new interface
3. **Implement for all actions**:
   - apt_install, apt_repo, apt_ppa
   - brew_install, brew_cask
   - dnf_install, dnf_repo
   - pacman_install, apk_install, zypper_install
4. **GPG verification logic** - Critical security requirement
5. **Unit tests with mocking** - Don't need real containers for unit tests

### Design Decision Needed

The design doc shows ExecuteInSandbox without params:
```go
func (a *AptInstallAction) ExecuteInSandbox(ctx *SandboxContext) error {
    args := append([]string{"install", "-y"}, a.Packages...)  // uses struct fields
}
```

But current architecture uses params everywhere:
```go
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error
```

**Recommendation:** Keep params pattern for consistency with existing architecture. Actions are stateless, data comes from params.

```go
func (a *AptInstallAction) ExecuteInSandbox(ctx *SandboxContext, params map[string]interface{}) error {
    packages, _ := GetStringSlice(params, "packages")
    return ctx.Run("apt-get", append([]string{"install", "-y"}, packages...)...)
}
```

## Files Referenced

- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/packages.go` - ExtractPackages()
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/container_spec.go` - DeriveContainerSpec(), ContainerImageName()
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/sandbox/executor.go` - Sandbox executor integration
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/validate/runtime.go` - Runtime interface with Build(), ImageExists()
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/apt_actions.go` - apt_install, apt_repo, apt_ppa actions
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/actions/system_action.go` - SystemAction interface
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/DESIGN-structured-install-guide.md` - Design specification
