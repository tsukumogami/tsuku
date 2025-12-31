# Implementation Plan: Package Installation Action Structs (#755)

## Summary

Define Go structs for package installation actions (`apt_install`, `apt_repo`, `apt_ppa`, `brew_install`, `brew_cask`, `dnf_install`, `dnf_repo`, `pacman_install`, `apk_install`, `zypper_install`) that replace the polymorphic `install_guide` field with explicit, validatable structures. All structs implement the `SystemAction` interface with `Validate()` and support `fallback` and `unless_command` fields.

## Approach

1. **Define `SystemAction` interface** - A new interface extending `Action` with `Validate()` method and `ImplicitConstraint()` for target matching
2. **Create typed action structs** - One struct per package manager operation following the design document's vocabulary
3. **Implement validation logic** - Each struct validates its required/optional fields
4. **Add TOML parsing support** - Parse `action = "apt_install"` into the correct typed struct
5. **Write comprehensive unit tests** - Test each struct's validation, name, and constraint methods

## Alternatives Considered

### A1: Single polymorphic action with manager field
- **Pros**: Fewer files, centralized logic
- **Cons**: Complex validation, polymorphic schemas (rejected by design doc D1)
- **Decision**: Rejected - design document explicitly chose Option A (one action per operation)

### A2: Interface embedding for shared fields
- **Pros**: DRY for `fallback`, `unless_command` fields
- **Cons**: Go embedding doesn't work well with interface methods, adds complexity
- **Decision**: Use composition with a `BaseSystemAction` struct instead

### A3: Separate file per action
- **Pros**: Clean separation, easy to find
- **Cons**: Many small files
- **Decision**: Group related actions (e.g., all apt actions, all dnf actions) in logical files

## Files to Modify

| File | Change |
|------|--------|
| `/internal/actions/action.go` | Register new actions in `init()` |
| `/internal/actions/system_packages.go` | Update existing stubs, possibly rename |

## Files to Create

| File | Purpose |
|------|---------|
| `/internal/actions/system_action.go` | `SystemAction` interface, `Constraint` struct, `BaseSystemAction` helper |
| `/internal/actions/apt_actions.go` | `AptInstallAction`, `AptRepoAction`, `AptPPAAction` |
| `/internal/actions/brew_actions.go` | `BrewCaskAction` (expand existing `BrewInstallAction`) |
| `/internal/actions/dnf_actions.go` | `DnfInstallAction`, `DnfRepoAction` |
| `/internal/actions/linux_pm_actions.go` | `PacmanInstallAction`, `ApkInstallAction`, `ZypperInstallAction` |
| `/internal/actions/apt_actions_test.go` | Tests for apt actions |
| `/internal/actions/brew_actions_test.go` | Tests for brew actions |
| `/internal/actions/dnf_actions_test.go` | Tests for dnf actions |
| `/internal/actions/linux_pm_actions_test.go` | Tests for pacman/apk/zypper actions |
| `/internal/actions/system_action_test.go` | Tests for interface and constraint logic |

## Implementation Steps

### Step 1: Define SystemAction interface and Constraint struct

Create `system_action.go` with:
```go
// Constraint represents a platform/family requirement
type Constraint struct {
    OS          string // e.g., "darwin", "linux"
    LinuxFamily string // e.g., "debian", "rhel" (only when OS == "linux")
}

// SystemAction extends Action with system dependency capabilities
type SystemAction interface {
    Action

    // Validate checks that parameters are valid for this action
    Validate() error

    // ImplicitConstraint returns the built-in platform constraint
    // Returns nil if no constraint (action works everywhere)
    ImplicitConstraint() *Constraint
}

// BaseSystemAction provides shared fields for system actions
type BaseSystemAction struct {
    BaseAction
    Fallback      string // fallback text if install fails
    UnlessCommand string // skip if this command exists
}
```

### Step 2: Implement apt actions

Create `apt_actions.go`:
- `AptInstallAction` with `packages []string`, `fallback`, `unless_command`
- `AptRepoAction` with `url`, `key_url`, `key_sha256`
- `AptPPAAction` with `ppa`

All return `&Constraint{OS: "linux", LinuxFamily: "debian"}` from `ImplicitConstraint()`.

### Step 3: Expand brew actions

Modify or create `brew_actions.go`:
- Update existing `BrewInstallAction` to implement `SystemAction`
- Add `BrewCaskAction` with same fields plus `tap` option

Both return `&Constraint{OS: "darwin"}` from `ImplicitConstraint()`.

### Step 4: Implement dnf actions

Create `dnf_actions.go`:
- `DnfInstallAction` with `packages []string`, `fallback`, `unless_command`
- `DnfRepoAction` with `url`, `key_url`, `key_sha256`

Both return `&Constraint{OS: "linux", LinuxFamily: "rhel"}` from `ImplicitConstraint()`.

### Step 5: Implement other Linux PM actions

Create `linux_pm_actions.go`:
- `PacmanInstallAction` -> `LinuxFamily: "arch"`
- `ApkInstallAction` -> `LinuxFamily: "alpine"`
- `ZypperInstallAction` -> `LinuxFamily: "suse"`

### Step 6: Register actions and update parsing

Update `action.go` to register all new actions.

### Step 7: Write unit tests

For each action type, test:
- `Name()` returns correct action name
- `Validate()` errors on missing required fields
- `Validate()` passes with valid params
- `ImplicitConstraint()` returns correct constraint
- `Preflight()` integration with validation

## Testing Strategy

### Unit Tests
- Test each action's `Name()`, `Validate()`, `ImplicitConstraint()` methods
- Test `Preflight()` returns errors for missing required parameters
- Test `Preflight()` returns warnings for missing optional parameters (like fallback)
- Test `Execute()` stub behavior (logs what would be installed)

### Table-Driven Tests
Use table-driven tests for validation across all actions:
```go
testCases := []struct {
    name     string
    action   SystemAction
    params   map[string]interface{}
    wantErr  bool
}{
    {"apt_install valid", &AptInstallAction{}, map[string]interface{}{"packages": []string{"curl"}}, false},
    {"apt_install missing packages", &AptInstallAction{}, map[string]interface{}{}, true},
    // ...
}
```

### Constraint Tests
Test that each action returns the correct implicit constraint:
```go
constraintTests := []struct {
    action     SystemAction
    wantOS     string
    wantFamily string
}{
    {&AptInstallAction{}, "linux", "debian"},
    {&BrewCaskAction{}, "darwin", ""},
    // ...
}
```

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Existing `BrewInstallAction` usage conflicts | Medium | Keep backward compatibility, add `SystemAction` interface on top |
| Action parsing not updated in executor | Low | Issue scope is struct definition; parsing is separate concern |
| Missing action from design vocabulary | Low | Cross-check against design doc table before merging |

## Success Criteria

- [ ] All 10 package installation actions defined with correct structs
- [ ] All structs implement `SystemAction` interface
- [ ] All structs have working `Validate()` method
- [ ] All structs have correct `ImplicitConstraint()` return values
- [ ] Support for `fallback` field on all install actions
- [ ] Support for `unless_command` field on all install actions
- [ ] Actions registered in action registry
- [ ] Unit tests pass for all actions
- [ ] `go test ./internal/actions/...` passes
- [ ] `go vet ./...` passes

## Open Questions

1. **Should `AptRepoAction` and `DnfRepoAction` support `unless_command`?**
   - Current design shows it on install actions only
   - Decision: Follow design doc - only on `*_install` actions

2. **Should existing `BrewInstallAction` be refactored or replaced?**
   - It already exists as a stub in `system_packages.go`
   - Decision: Refactor in place to implement `SystemAction`, move to `brew_actions.go`

3. **How to handle the `tap` field for brew actions?**
   - Design shows `tap?: string` as optional
   - Decision: Add as optional field, validate it's a string if present
