# Issue 771 Introspection

## Context Reviewed

- Design doc: docs/DESIGN-structured-install-guide.md
- Sibling issues reviewed: #757, #767, #768, #769, #770 (all closed after #771 creation)
- Prior patterns identified:
  - Container building infrastructure added to Runtime interface (Build, ImageExists methods)
  - ContainerSpec struct defines container specifications with packages and build commands
  - Package extraction from plans already implemented
  - Container image caching with deterministic naming implemented
  - Sandbox executor integration with container building complete (#770)

## Gap Analysis

### Critical Finding: ExecuteInSandbox Does Not Exist

The issue spec says:
> Each install action implements `ExecuteInSandbox(ctx *SandboxContext) error`

However, reviewing the actual implementation reveals:

1. **No SandboxContext type exists** in the codebase
2. **No ExecuteInSandbox method** is defined on Action or SystemAction interfaces
3. **Actions only have Execute(ctx *ExecutionContext, params) method** which currently:
   - For apt_install: prints "Would install via apt: [packages]" and returns nil (stub)
   - For apt_repo: prints "Would add APT repository" and returns nil (stub)
   - Does NOT execute anything in containers

4. **The Runtime interface** (internal/validate/runtime.go) already has:
   - `Run(ctx, opts RunOptions) (*RunResult, error)` - runs commands in containers
   - `Build(ctx, imageName, baseImage, buildCommands)` - builds container images
   - `ImageExists(ctx, name) (bool, error)` - checks if image exists

5. **Container building is complete** (#770):
   - ExtractPackages() extracts packages from plans
   - DeriveContainerSpec() creates container specifications
   - ContainerImageName() generates deterministic cache keys
   - Runtime.Build() can build images with packages installed

### The Real Gap: Execution vs Building

The issue conflates two different concerns:

**Concern A: Container Building** (DONE in #770)
- Extract packages from recipe steps
- Build containers WITH those packages installed
- Cache built containers
- Use appropriate base image per linux_family

**Concern B: Action Execution in Sandbox** (NOT DONE - this issue)
- Run action operations INSIDE a built container
- For apt_repo: download GPG key, verify SHA256, import key
- For apt_install: run apt-get install inside container
- Capture and return errors

### What Actually Needs to Happen

Looking at the design doc section "Action Execution in Sandbox" (lines 570-604), the intended flow is:

```go
type Action interface {
    ExecuteInSandbox(ctx *SandboxContext) error  // <-- THIS METHOD DOES NOT EXIST
    Describe() string                             // <-- This exists (on SystemAction)
}
```

But the current implementation has:

```go
type Action interface {
    Execute(ctx *ExecutionContext, params) error  // <-- This exists, but is a stub
}

type SystemAction interface {
    Action
    Validate(params) error
    ImplicitConstraint() *Constraint
    Describe(params) string
}
```

### Major Gaps

1. **No SandboxContext type**: The design shows a SandboxContext with a Run() method, but this doesn't exist in the codebase

2. **No ExecuteInSandbox method**: Neither Action nor SystemAction has this method. The design shows it, but it's not implemented.

3. **Execute() is a stub**: Current Execute() methods just print messages. They don't actually run commands.

4. **GPG key verification not implemented**: The issue AC says "GPG key verification: download key, compute SHA256, verify against key_sha256 before import" but there's no code for this.

5. **Container mocking for unit tests**: Issue AC says "Unit tests with container mocking" but there's no mocking infrastructure.

### What the Issue SHOULD Say

Based on the completed work and the design, this issue needs to:

1. **Create SandboxContext type** that wraps Runtime and provides a Run() method for executing commands in containers

2. **Add ExecuteInSandbox to SystemAction interface** (or create a new SandboxExecutable interface)

3. **Implement ExecuteInSandbox for each system action**:
   - apt_install: Run apt-get update && apt-get install
   - apt_repo: Download GPG key, verify SHA256, import key, add repo
   - apt_ppa: Run add-apt-repository
   - brew_install, brew_cask, dnf_install, etc.

4. **GPG key verification logic**: Hash downloaded key, compare to key_sha256, fail if mismatch

5. **Container test infrastructure**: Mocks for Runtime interface to test ExecuteInSandbox without actual containers

### Why This Matters for #802

Issue #802 depends on this issue because it needs to:
- Run test scripts WITH sandbox-built containers
- Those containers must have recipe dependencies INSTALLED (not just declared)
- The "installing" part requires ExecuteInSandbox() to actually work

Currently:
- Container building works (#770) - can build a container with apt packages
- But actions don't execute inside those containers yet
- So #802 cannot run tests inside containers with dependencies installed

## Recommendation

**Clarify** - The issue spec needs significant amendments to reflect what actually needs to be done.

## Proposed Amendments

The issue should be amended to clarify:

### 1. Scope Definition

This issue is about **action execution inside sandbox containers**, not container building (which #770 completed). The goal is to make system dependency actions (apt_install, apt_repo, etc.) actually execute their operations inside containers.

### 2. Architecture Clarification

Need to create:
- `SandboxContext` type that wraps `Runtime` and provides execution primitives
- Either add `ExecuteInSandbox()` to `SystemAction` interface OR create new `SandboxExecutable` interface
- Update all system actions to implement the sandbox execution method

### 3. Updated Acceptance Criteria

- [ ] Create `SandboxContext` type with `Run(command string, args ...string) error` method
- [ ] Add `ExecuteInSandbox(ctx *SandboxContext) error` to SystemAction interface (or create SandboxExecutable)
- [ ] Implement ExecuteInSandbox for all system actions:
  - apt_install: Run `apt-get update && apt-get install -y <packages>`
  - apt_repo: Download key, compute SHA256, verify hash, import key, add repo
  - apt_ppa: Run `add-apt-repository ppa:<ppa>`
  - brew_install, brew_cask, dnf_install, pacman_install, apk_install, zypper_install
- [ ] GPG key verification: Download key from key_url, compute SHA256, compare to key_sha256, fail if mismatch
- [ ] Error context: Wrap errors with action name and parameters
- [ ] Unit tests: Mock Runtime interface to test ExecuteInSandbox without real containers
- [ ] Unit test: GPG key hash mismatch fails with clear error message

### 4. Key Design Decision

**Question for user**: Should ExecuteInSandbox be added to the existing SystemAction interface, or should we create a separate SandboxExecutable interface that system actions can optionally implement?

**Option A: Add to SystemAction**
```go
type SystemAction interface {
    Action
    Validate(params) error
    ImplicitConstraint() *Constraint
    Describe(params) string
    ExecuteInSandbox(ctx *SandboxContext, params map[string]interface{}) error
}
```

**Option B: Separate interface**
```go
type SandboxExecutable interface {
    ExecuteInSandbox(ctx *SandboxContext, params map[string]interface{}) error
}

// System actions that can run in sandbox implement both:
type AptInstallAction struct {
    BaseAction
}
func (a *AptInstallAction) ExecuteInSandbox(ctx *SandboxContext, params) error { ... }
```

### 5. Implementation Notes

The design document shows ExecuteInSandbox NOT taking params as an argument:
```go
func (a *AptInstallAction) ExecuteInSandbox(ctx *SandboxContext) error {
    args := append([]string{"install", "-y"}, a.Packages...)  // <-- uses struct fields
    return ctx.Run("apt-get", args...)
}
```

But the current action architecture uses map[string]interface{} params everywhere:
```go
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error
```

**Decision needed**: Should ExecuteInSandbox follow the design (no params, use struct fields) or the current pattern (params map)?

Current codebase pattern suggests params map is preferred, since actions are stateless and all data comes from params.

## Blocking Concerns

None - this is an architecture clarification, not a fundamental blocker. The work can proceed once the scope is clarified.

## Resolution (User Clarification)

**User Decision:** The architecture is BUILD time, not RUN time. GPG keys and repository setup should happen during container image building via Docker RUN commands, NOT during action execution.

**Correct Architecture:**

**BUILD time** (Container image creation):
1. Extract ALL system actions from plan (apt_install, apt_repo, brew_install, etc.)
2. Generate Docker RUN commands for:
   - GPG key download: `RUN wget -O /tmp/key.gpg <key_url>`
   - Hash verification: `RUN echo "<key_sha256> /tmp/key.gpg" | sha256sum -c || exit 1`
   - Key import: `RUN apt-key add /tmp/key.gpg`
   - Repository addition: `RUN echo "deb <url>" > /etc/apt/sources.list.d/...`
   - Package installation: `RUN apt-get update && apt-get install -y <packages>`
3. Build container image with everything installed and verified
4. Cache the image

**RUN time** (Action execution):
1. Actions Execute() methods do VERIFICATION only
2. `apt_install packages=["curl"]` → check if `curl` command exists
3. `apt_repo url=...` → check if repo exists in /etc/apt/sources.list.d/
4. If exists: return nil (success)
5. If missing: return error with install instructions

**What issue #771 should implement:**
1. Extend `DeriveContainerSpec()` to handle apt_repo, apt_ppa, dnf_repo, etc.
2. Generate GPG verification Docker RUN commands
3. Make action Execute() methods verify prerequisites instead of being stubs
4. Update design docs if they incorrectly describe ExecuteInSandbox
5. NO ExecuteInSandbox() method needed
6. NO SandboxContext type needed

**Key insight:** Everything happens at build time, gets baked into cached images. Actions just verify the environment is correct.
