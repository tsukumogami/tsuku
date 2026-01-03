# Issue #771 Implementation Plan

## Overview

**Goal:** Extend sandbox container building to handle repository actions (apt_repo, dnf_repo, etc.) with GPG key verification at BUILD time.

**Architecture Clarification:** The issue description is INCORRECT. Based on user clarification:
- Everything happens at BUILD time (Docker RUN commands during image creation)
- NO ExecuteInSandbox() method needed
- NO SandboxContext type needed
- Actions Execute() methods should VERIFY prerequisites exist, not install them

**Context from #802:** Test scripts need to declare dependencies in recipes and have tsuku build containers automatically, replacing hardcoded apt-get calls in Dockerfiles.

## Current State (from #770)

**What works:**
- `ExtractPackages()` extracts apt_install, brew_install, etc. from plans
- `DeriveContainerSpec()` generates Docker RUN commands for package installation
- Container images are built and cached
- Simple packages (curl, jq, git) are pre-installed at build time

**What's missing:**
- Repository actions (apt_repo, apt_ppa, dnf_repo) are ignored
- No GPG key verification
- Action Execute() methods are stubs (don't verify prerequisites)
- Design docs incorrectly describe ExecuteInSandbox

## Implementation Approach

### Phase 1: Extract Repository Metadata ✅

**Goal:** Extend package extraction to include repository configurations.

**Files:**
- `internal/sandbox/packages.go`

**Changes:**

1. Create `SystemRequirements` struct:
```go
type SystemRequirements struct {
    Packages     map[string][]string    // e.g., {"apt": ["curl", "jq"]}
    Repositories []RepositoryConfig      // Repository configurations
}

type RepositoryConfig struct {
    Manager   string  // "apt", "dnf", etc.
    Type      string  // "repo", "ppa"
    URL       string  // Repository URL
    KeyURL    string  // GPG key URL (if applicable)
    KeySHA256 string  // Expected SHA256 hash of key
    PPA       string  // PPA name (for apt_ppa)
}
```

2. Rename `ExtractPackages()` to `ExtractSystemRequirements()`:
   - Return `*SystemRequirements` instead of `map[string][]string`
   - Extract apt_repo, apt_ppa, brew_tap, dnf_repo actions
   - Populate Repositories slice with config data
   - Maintain backward compatibility for packages

3. Handle action types:
   - `apt_repo`: Extract url, key_url, key_sha256
   - `apt_ppa`: Extract ppa name
   - `dnf_repo`: Extract url, gpgkey
   - `brew_tap`: Extract tap name

**Test cases:**
- Extract mixed packages + repositories
- Handle missing GPG keys (some repos don't need them)
- Validate required fields present

### Phase 2: Generate Repository Docker RUN Commands

**Goal:** Generate Docker RUN commands for repository setup with GPG verification.

**Files:**
- `internal/sandbox/container_spec.go`

**Changes:**

1. Update `DeriveContainerSpec()` signature:
```go
func DeriveContainerSpec(reqs *SystemRequirements) (*ContainerSpec, error)
```

2. Update `generateBuildCommands()` to emit repository commands BEFORE package installation:

**For apt_repo with GPG key:**
```dockerfile
RUN apt-get update && apt-get install -y wget ca-certificates
RUN wget -O /tmp/repo.gpg https://example.com/key.gpg
RUN echo "abc123...  /tmp/repo.gpg" | sha256sum -c || (echo "GPG key hash mismatch" && exit 1)
RUN apt-key add /tmp/repo.gpg
RUN echo "deb https://example.com/ubuntu focal main" > /etc/apt/sources.list.d/custom.list
RUN apt-get update
RUN apt-get install -y <packages>
```

**For apt_ppa:**
```dockerfile
RUN apt-get update && apt-get install -y software-properties-common
RUN add-apt-repository -y ppa:user/repo
RUN apt-get update
RUN apt-get install -y <packages>
```

**For dnf_repo with GPG key:**
```dockerfile
RUN dnf install -y wget
RUN wget -O /tmp/repo.gpg https://example.com/key.gpg
RUN echo "abc123...  /tmp/repo.gpg" | sha256sum -c || exit 1
RUN rpm --import /tmp/repo.gpg
RUN echo -e "[repo]\nname=Custom\nbaseurl=https://example.com\ngpgcheck=1\ngpgkey=file:///tmp/repo.gpg" > /etc/yum.repos.d/custom.repo
RUN dnf install -y <packages>
```

3. Command ordering logic:
   - Install prerequisites (wget, ca-certificates, software-properties-common)
   - Download and verify GPG keys
   - Import keys
   - Add repositories
   - Update package cache (apt-get update, dnf makecache)
   - Install packages

**Test cases:**
- Repository with GPG key verification
- Repository without GPG key
- Multiple repositories
- PPA addition
- GPG hash mismatch (should fail build)
- Mixed repositories and packages

### Phase 3: Update Container Image Caching

**Goal:** Include repository configurations in cache keys to invalidate when repos change.

**Files:**
- `internal/sandbox/container_spec.go`

**Changes:**

1. Update `ContainerImageName()` to hash repository configs:
```go
func ContainerImageName(spec *ContainerSpec) string {
    // Current: hashes base image + packages
    // New: also hash repository URLs, key URLs, key hashes

    parts := []string{spec.BaseImage}

    // Hash packages (existing)
    for mgr, pkgs := range spec.Packages {
        parts = append(parts, fmt.Sprintf("%s:%s", mgr, strings.Join(pkgs, ",")))
    }

    // Hash repositories (NEW)
    for _, repo := range spec.Repositories {
        parts = append(parts, fmt.Sprintf("repo:%s:%s:%s:%s",
            repo.Manager, repo.Type, repo.URL, repo.KeySHA256))
    }

    sort.Strings(parts)
    hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
    return fmt.Sprintf("tsuku-sandbox-%s", hex.EncodeToString(hash[:8]))
}
```

**Test cases:**
- Same packages, different repo → different cache key
- Same repo, different GPG key hash → different cache key
- Repository order doesn't affect cache key (sorted)

### Phase 4: Update Action Execute() Methods

**Goal:** Make Execute() methods verify prerequisites instead of being stubs.

**Files:**
- `internal/actions/apt_actions.go`
- `internal/actions/brew_actions.go`
- `internal/actions/dnf_actions.go`
- `internal/actions/pacman_actions.go`
- `internal/actions/apk_actions.go`
- `internal/actions/zypper_actions.go`

**Changes:**

1. Update `apt_install.Execute()`:
```go
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    packages, _ := GetStringSlice(params, "packages")

    // Verify each package command exists
    for _, pkg := range packages {
        if !commandExists(pkg) {
            return fmt.Errorf("command %s not found\nInstall with: apt-get install %s", pkg, pkg)
        }
    }

    return nil  // All packages verified
}

func commandExists(cmd string) bool {
    _, err := exec.LookPath(cmd)
    return err == nil
}
```

2. Update `apt_repo.Execute()`:
```go
func (a *AptRepoAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    url := params["url"].(string)

    // Check if repository is in sources.list.d
    pattern := filepath.Join("/etc/apt/sources.list.d", "*.list")
    files, _ := filepath.Glob(pattern)

    for _, file := range files {
        content, _ := os.ReadFile(file)
        if strings.Contains(string(content), url) {
            return nil  // Repository found
        }
    }

    return fmt.Errorf("repository not found: %s\nAdd with: apt-add-repository '%s'", url, url)
}
```

3. Update similar methods for other package managers (brew, dnf, etc.)

**Test cases:**
- Command exists → success
- Command missing → error with instructions
- Repository exists → success
- Repository missing → error with instructions

### Phase 5: Wire Up Sandbox Executor Integration

**Goal:** Update sandbox executor to use new SystemRequirements.

**Files:**
- `internal/sandbox/executor.go`

**Changes:**

1. Update `Sandbox()` method:
```go
// OLD:
packages := ExtractPackages(plan)
if packages != nil {
    spec, err := DeriveContainerSpec(packages)
    // ...
}

// NEW:
sysReqs := ExtractSystemRequirements(plan)
if sysReqs != nil && (len(sysReqs.Packages) > 0 || len(sysReqs.Repositories) > 0) {
    spec, err := DeriveContainerSpec(sysReqs)
    // ...
}
```

**Test cases:**
- Plan with packages only → builds container
- Plan with repositories only → builds container
- Plan with packages + repositories → builds container with both
- Plan with no system deps → uses default image

### Phase 6: Fix Design Documentation

**Goal:** Correct design docs that incorrectly describe ExecuteInSandbox.

**Files:**
- `docs/DESIGN-structured-install-guide.md`

**Changes:**

1. Search for references to `ExecuteInSandbox` and `SandboxContext`
2. Update to describe BUILD-time architecture:
   - System actions generate Docker RUN commands
   - GPG verification happens during image build
   - Execute() methods verify prerequisites exist

3. Add section on "Build-Time vs Run-Time Execution":
   - Explain why everything happens at build time
   - Describe caching benefits
   - Note that Execute() just verifies environment

**Review:**
- Ensure consistency with actual implementation
- Remove misleading examples

### Phase 7: Add Tests for GPG Verification

**Goal:** Test GPG key verification failure paths.

**Files:**
- `internal/sandbox/container_spec_test.go`

**Test cases:**

1. **TestGenerateBuildCommands_AptRepoWithGPG:**
   - Verify wget command generated
   - Verify sha256sum verification command
   - Verify apt-key add command
   - Verify repository file creation

2. **TestGenerateBuildCommands_GPGHashMismatch:**
   - Mock container build
   - Provide mismatched GPG hash
   - Verify build fails with hash mismatch error
   - Verify error message is clear

3. **TestGenerateBuildCommands_MultipleRepos:**
   - Multiple apt_repo actions
   - Verify all repos added
   - Verify all GPG keys verified
   - Verify command ordering

4. **TestContainerImageName_IncludesRepoConfig:**
   - Same packages, different repo → different cache key
   - Same repo, different key hash → different cache key
   - Repository order doesn't affect key (sorted)

5. **TestExecute_VerifyCommand:**
   - Command exists → returns nil
   - Command missing → returns error with install instructions

## Files to Modify

| File | Lines Changed | Description |
|------|---------------|-------------|
| internal/sandbox/packages.go | ~150 | Add SystemRequirements struct, rename ExtractPackages |
| internal/sandbox/container_spec.go | ~250 | Generate Docker RUN commands for repos + GPG verification |
| internal/sandbox/container_spec_test.go | ~150 | Test GPG verification, cache keys, command generation |
| internal/sandbox/executor.go | ~50 | Use SystemRequirements instead of packages |
| internal/actions/apt_actions.go | ~100 | Update Execute() to verify prerequisites |
| internal/actions/brew_actions.go | ~50 | Update Execute() to verify prerequisites |
| internal/actions/dnf_actions.go | ~50 | Update Execute() to verify prerequisites |
| internal/actions/pacman_actions.go | ~20 | Update Execute() to verify prerequisites |
| internal/actions/apk_actions.go | ~20 | Update Execute() to verify prerequisites |
| internal/actions/zypper_actions.go | ~20 | Update Execute() to verify prerequisites |
| docs/DESIGN-structured-install-guide.md | ~100 | Fix ExecuteInSandbox references, document BUILD-time architecture |

**Total:** ~960 lines across 11 files

## Testing Strategy

**Unit tests:**
- SystemRequirements extraction
- Docker RUN command generation
- GPG verification commands
- Cache key computation
- Execute() verification logic

**Integration tests:**
- Build container with apt_repo + GPG key
- Verify GPG hash mismatch fails build
- Build container with PPA
- Build container with dnf_repo
- Run plan executor (Execute() verifies packages exist)

**Edge cases:**
- Repository without GPG key
- Multiple repositories with same package manager
- Mixed repositories (apt + brew in different platform filters)
- Invalid GPG hash format
- Missing key_url parameter

## Risks and Mitigations

**Risk 1:** Build-time GPG verification makes builds slower
- **Mitigation:** Container images are cached; verification only happens once per config

**Risk 2:** sha256sum command might not be available in minimal base images
- **Mitigation:** Install ca-certificates/wget as prerequisites before key download

**Risk 3:** apt-key is deprecated in newer Debian versions
- **Mitigation:** Use modern GPG key management (`/etc/apt/keyrings/`) for newer Debian, legacy apt-key for older

**Risk 4:** Different package managers have different GPG import mechanisms
- **Mitigation:** Abstract per-manager logic in generateBuildCommands()

## Acceptance Criteria Mapping

Original issue acceptance criteria (which were incorrect):
- ~~Each install action implements ExecuteInSandbox~~ → REPLACED: DeriveContainerSpec generates Docker RUN commands
- GPG key verification: download key, compute SHA256, verify against key_sha256 → IMPLEMENTED in Docker RUN commands
- Execution MUST fail if hash mismatch → IMPLEMENTED as `sha256sum -c || exit 1`
- Errors captured and returned with context → IMPLEMENTED as Execute() verification errors
- Unit tests with container mocking → IMPLEMENTED as generateBuildCommands tests
- Unit test for GPG key hash verification failure path → IMPLEMENTED as TestGPGHashMismatch

## Dependencies on #802

This implementation enables #802 by:
1. Test recipes can declare system dependencies (apt_install, apt_repo)
2. Sandbox executor builds containers with those dependencies pre-installed
3. Test scripts run in containers where prerequisites are verified to exist
4. Replaces hardcoded `apt-get install` in Dockerfiles with recipe declarations

Example test recipe for #802:
```toml
[[steps]]
action = "apt_install"
packages = ["curl", "ca-certificates", "patchelf", "wget"]
```

Sandbox builds container with these packages, test script runs inside, apt_install.Execute() verifies all commands exist.

## Implementation Order

1. Phase 1: SystemRequirements struct (enables phases 2-5)
2. Phase 2: Docker RUN generation (core functionality)
3. Phase 3: Cache key updates (correctness)
4. Phase 5: Sandbox executor wiring (integration)
5. Phase 4: Execute() verification (can be done in parallel)
6. Phase 7: Tests (verify correctness)
7. Phase 6: Design docs (documentation)

## Success Criteria

- [ ] Container builds include repository setup with GPG verification
- [ ] GPG hash mismatch fails container build with clear error
- [ ] Container images are cached by (packages + repositories + GPG hashes)
- [ ] Execute() methods verify prerequisites exist and provide install instructions if missing
- [ ] All tests pass (unit + integration)
- [ ] Design docs accurately reflect BUILD-time architecture
- [ ] Ready for #802 to use recipe-based dependencies in test scripts
