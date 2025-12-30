# System Dependency Actions: Implementation Feasibility Assessment

## Executive Summary

The proposed system dependency actions design is feasible but represents significant implementation complexity. The design document outlines approximately 13-15 new action types across four categories. With strategic code reuse and phased delivery, the work can be decomposed into manageable increments.

## 1. Action Type Inventory and Complexity Estimates

### Package Installation Actions (Medium Complexity)

| Action | Complexity | Estimate | Notes |
|--------|------------|----------|-------|
| `apt_install` | Medium | 2-3 days | Stub exists; needs actual execution |
| `apt_repo` | Medium | 2-3 days | GPG key handling, sources.list management |
| `apt_ppa` | Low | 1 day | Wrapper around apt_repo for Ubuntu PPAs |
| `dnf_install` | Low | 1 day | Structurally identical to apt_install |
| `dnf_repo` | Low | 1 day | Structurally identical to apt_repo |
| `brew_install` | Low | 1 day | Stub exists; straightforward |
| `brew_cask` | Low | 1 day | Minor variant of brew_install |
| `pacman_install` | Low | 1 day | Structurally identical to apt_install |

### System Configuration Actions (Medium Complexity)

| Action | Complexity | Estimate | Notes |
|--------|------------|----------|-------|
| `group_add` | Medium | 2 days | Requires usermod, session handling |
| `service_enable` | Medium | 2 days | systemd vs other init systems |
| `service_start` | Low | 1 day | Structurally similar to service_enable |

### Verification Actions (Low Complexity)

| Action | Complexity | Estimate | Notes |
|--------|------------|----------|-------|
| `require_command` | Low | 0.5 days | Extract from existing require_system |

### Fallback Actions (Trivial)

| Action | Complexity | Estimate | Notes |
|--------|------------|----------|-------|
| `manual` | Trivial | 0.5 days | Display text, return error |

**Total: 13 action types, approximately 15-18 development days**

## 2. Code Sharing Opportunities

### Recommended Action Hierarchy

```go
// Base for all system package managers
type SystemPackageAction struct {
    BaseAction
}

func (a *SystemPackageAction) validatePackages(params map[string]interface{}) ([]string, error) {
    // Common package list validation
}

func (a *SystemPackageAction) requiresSudo() bool {
    return true // All system package actions need sudo
}

// Apt-family actions
type AptBaseAction struct {
    SystemPackageAction
}

func (a *AptBaseAction) detectDistro() (string, error) {
    // Read /etc/os-release, extract ID field
}

// Similarly for DnfBaseAction, BrewBaseAction
```

### Shared Utility Functions

Several operations repeat across actions and should be extracted:

1. **Distro detection** (`internal/platform/distro.go`):
   ```go
   func DetectDistro() (distro string, version string, err error)
   func IsDerivativeOf(distro, family string) bool  // e.g., Mint -> Ubuntu -> Debian
   ```

2. **Privilege escalation** (`internal/actions/sudo.go`):
   ```go
   func RunWithSudo(ctx context.Context, cmd []string) error
   func CanSudo() bool  // Check if user can sudo without password
   ```

3. **Package manager detection** (`internal/platform/pkgmanager.go`):
   ```go
   func DetectPackageManager() string  // apt, dnf, pacman, brew, etc.
   func IsPackageManagerAvailable(pm string) bool
   ```

### Apt/Dnf Code Sharing

These two families share nearly identical structure. Recommend a generic `LinuxPackageManager` interface:

```go
type LinuxPackageManager interface {
    InstallCmd(packages []string) []string   // apt-get install vs dnf install
    UpdateCmd() []string                      // apt-get update vs dnf makecache
    AddRepoCmd(repo RepoSpec) []string       // add-apt-repository vs dnf config-manager
}
```

Implementation effort saved: ~3-4 days by avoiding duplication.

## 3. Testing Strategy Recommendations

### Unit Testing (All Actions)

1. **Parameter validation**: Test Preflight() with valid/invalid params
2. **Command construction**: Mock exec.Command, verify generated commands
3. **Error handling**: Verify graceful failures for missing dependencies

### Integration Testing Challenges

System package actions present unique testing difficulties:

1. **Root requirement**: Most actions need sudo
2. **State mutation**: Installing packages changes system state
3. **Platform diversity**: apt vs dnf vs pacman vs brew

### Recommended Testing Approach

**Level 1: Command Generation Tests (No Root)**
```go
func TestAptInstallAction_CommandGeneration(t *testing.T) {
    action := &AptInstallAction{}
    cmd := action.buildCommand([]string{"docker.io"})
    expected := []string{"apt-get", "install", "-y", "docker.io"}
    assert.Equal(t, expected, cmd)
}
```

**Level 2: Container-Based Integration Tests**

Extend existing sandbox executor for system package testing:

```go
// Use different base images per distro family
var testImages = map[string]string{
    "ubuntu": "ubuntu:22.04",
    "debian": "debian:12",
    "fedora": "fedora:40",
    "arch":   "archlinux:base",
}

func TestAptInstall_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Integration tests disabled in short mode")
    }
    // Run in container with apt available
}
```

**Level 3: Mock Package Manager Protocol**

For CI without containers, implement mock package managers:

```go
// Inject via environment variable
// TSUKU_MOCK_APT=/path/to/mock-apt
func getAptPath() string {
    if mock := os.Getenv("TSUKU_MOCK_APT"); mock != "" {
        return mock
    }
    return "apt-get"
}
```

### Distro Detection Testing

Use test fixtures for `/etc/os-release` parsing:

```go
// testdata/os-release/ubuntu-22.04
// testdata/os-release/fedora-40
// testdata/os-release/arch

func TestDetectDistro(t *testing.T) {
    for _, fixture := range testFixtures {
        distro, version, err := detectDistroFromFile(fixture.path)
        // Verify against expected
    }
}
```

## 4. Phased Delivery Plan

### Phase 1: Infrastructure (Week 1)

**Goal**: Lay groundwork without changing existing behavior

1. Add `distro` field to `WhenClause` in `internal/recipe/types.go`
2. Implement distro detection in new `internal/platform/distro.go`
3. Update `WhenClause.Matches()` to handle distro filtering
4. Add unit tests for distro detection

**Deliverables**:
- `when = { distro = ["ubuntu", "debian"] }` works in recipes
- Distro detection reads `/etc/os-release` correctly
- Derivative distro resolution (Mint -> Ubuntu -> Debian)

### Phase 2: Core Package Actions (Week 2)

**Goal**: Implement primary package installation actions

1. Convert `apt_install` stub to real implementation
2. Implement `dnf_install` using shared base
3. Implement `pacman_install` using shared base
4. Convert `brew_install` stub to real implementation

**Deliverables**:
- Basic package installation works across major distros
- All actions share common validation and error handling

### Phase 3: Repository Management (Week 3)

**Goal**: Enable third-party repository configuration

1. Implement `apt_repo` with GPG key handling
2. Implement `apt_ppa` as convenience wrapper
3. Implement `dnf_repo` using shared pattern
4. Add `brew_cask` as variant of `brew_install`

**Deliverables**:
- Complete Docker installation recipe works end-to-end
- Repository key verification via SHA256

### Phase 4: System Configuration (Week 4)

**Goal**: Post-installation configuration actions

1. Implement `group_add` action
2. Implement `service_enable` and `service_start`
3. Extract `require_command` from `require_system`
4. Implement `manual` action

**Deliverables**:
- Full Docker recipe including group membership and service enablement
- Clear separation between `require_system` (legacy) and new actions

### Phase 5: Sandbox Integration (Week 5)

**Goal**: Enable sandbox testing for system dependency recipes

1. Update `SandboxRequirements` to handle distro-specific base images
2. Add privilege escalation support to sandbox executor
3. Create container variants for each distro family
4. Document consent flow for privileged operations

**Deliverables**:
- `tsuku validate --sandbox` works for Docker recipe
- User consent prompt before system modification

## 5. Technical Risks and Mitigations

### Risk 1: Sudo/Privilege Handling

**Risk**: Actions require root but tsuku runs as user.

**Mitigation**:
- Require explicit `requires_sudo = true` in recipe metadata
- Prompt user for consent before invoking sudo
- Support `TSUKU_ALLOW_SUDO` env var for CI
- Never store credentials; always prompt via TTY

### Risk 2: Distro Detection Fragmentation

**Risk**: Derivative distros (Mint, Pop!_OS) may not be detected correctly.

**Mitigation**:
- Use `ID_LIKE` field from `/etc/os-release` for family detection
- Maintain explicit mapping table for common derivatives
- Allow recipes to specify `distro_family` as fallback

### Risk 3: Package Name Variations

**Risk**: Same software has different package names across distros.

**Mitigation**: This is a recipe authoring problem, not an action problem. Recipes must specify platform-specific packages:

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu"] }

[[steps]]
action = "dnf_install"
packages = ["docker"]
when = { distro = ["fedora"] }
```

### Risk 4: Sandbox Isolation

**Risk**: System package actions inside containers may behave differently than on host.

**Mitigation**:
- Container tests verify command generation, not actual installation
- Host-level testing requires user consent flow
- Document that sandbox tests are indicative, not definitive

### Risk 5: Service Management Portability

**Risk**: systemd vs sysvinit vs launchd across platforms.

**Mitigation**:
- Initially target only systemd (Linux) and launchd (macOS)
- Detect init system at runtime via `/sbin/init --version` or similar
- Return clear error on unsupported init systems

## 6. Integration with Existing Executor

The current sandbox executor in `internal/sandbox/executor.go` already handles:
- Container runtime detection (Podman/Docker)
- Network mode configuration
- Resource limits
- Plan-based installation

Required modifications:

1. **Base image selection**: Currently hardcoded. Needs to select based on `SandboxRequirements.Image` which should vary by distro:
   ```go
   reqs.Image = selectBaseImage(recipe.DistroRequirements)
   ```

2. **Privilege mode**: Add support for running containers with `--privileged` or `--cap-add` for systemd testing

3. **Init system in container**: For service testing, containers may need to run systemd as PID 1

## 7. Conclusion

The system dependency actions design is implementable with moderate effort (4-5 weeks for a single developer). Key success factors:

1. **Invest in shared infrastructure first**: Distro detection and privilege handling are foundational
2. **Leverage existing patterns**: The stub implementations in `system_packages.go` provide clear templates
3. **Phase delivery by capability**: Core package actions first, then repos, then services
4. **Accept testing limitations**: Full integration testing requires containers; unit tests cover command generation

The design's emphasis on composability aligns well with tsuku's action-based architecture. The main architectural decision to resolve is Option A vs B vs C for action granularity, with Option A (one action per operation) recommended for maximum composability and testability.
