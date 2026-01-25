# Issue 1112 Implementation Plan

## Summary

Create `system_dependency` action that checks for system packages and guides users to install them on musl systems.

## Approach

The `system_dependency` action follows the established action pattern but with key differences:
1. **Read-only**: Only checks if packages are installed, never runs privileged commands
2. **Structured errors**: Returns `DependencyMissingError` for CLI aggregation
3. **Root detection**: Includes sudo/doas prefix in suggested command based on current user

## Files to Create/Modify

### 1. Create `internal/actions/system_dependency.go` (New File)

Components:
- `SystemDependencyAction` struct (embeds BaseAction)
- `DependencyMissingError` struct with Library, Package, Command, Family fields
- `IsDependencyMissing()` helper function
- `isPackageInstalled()` - checks if package is installed (Alpine: `apk info -e`)
- `getInstallCommand()` - builds install command with root detection

Structure:
```go
type SystemDependencyAction struct{ BaseAction }

// IsDeterministic returns true - package presence is deterministic
func (SystemDependencyAction) IsDeterministic() bool { return true }

func (a *SystemDependencyAction) Name() string { return "system_dependency" }

func (a *SystemDependencyAction) Preflight(params map[string]interface{}) *PreflightResult
- Validates 'name' parameter is present
- Validates 'packages' parameter is present and is a map

func (a *SystemDependencyAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error
- Get family from ctx (needs LinuxFamily from somewhere)
- Look up package for family
- Check if installed
- Return DependencyMissingError if missing, nil if installed
```

### 2. Modify `internal/actions/action.go`

- Register the new action in `init()`: `Register(&SystemDependencyAction{})`

### 3. Create `internal/actions/system_dependency_test.go` (New File)

Test cases:
- `TestSystemDependency_Name` - action name is "system_dependency"
- `TestSystemDependency_Preflight_Valid` - valid params pass
- `TestSystemDependency_Preflight_MissingName` - error without name
- `TestSystemDependency_Preflight_MissingPackages` - error without packages
- `TestDependencyMissingError` - error message format
- `TestIsDependencyMissing` - helper function works
- `TestGetInstallCommand_Root` - no prefix when root
- `TestGetInstallCommand_NotRoot` - sudo prefix when not root
- `TestIsPackageInstalled_Alpine` - apk info check (may need mocking)

## Implementation Details

### DependencyMissingError Structure

```go
type DependencyMissingError struct {
    Library string // Display name (e.g., "zlib")
    Package string // Package name (e.g., "zlib-dev")
    Command string // Install command (e.g., "sudo apk add zlib-dev")
    Family  string // Linux family (e.g., "alpine")
}

func (e *DependencyMissingError) Error() string {
    return fmt.Sprintf("missing system dependency: %s (install with: %s)", e.Library, e.Command)
}

func IsDependencyMissing(err error) bool {
    var depErr *DependencyMissingError
    return errors.As(err, &depErr)
}
```

### Package Detection

```go
func isPackageInstalled(pkg string, family string) bool {
    switch family {
    case "alpine":
        cmd := exec.Command("apk", "info", "-e", pkg)
        return cmd.Run() == nil
    // Future: debian, rhel, arch, suse
    }
    return false
}
```

### Root Detection and Install Command

```go
func getInstallCommand(pkg string, family string) string {
    prefix := ""
    if os.Getuid() != 0 {
        if _, err := exec.LookPath("doas"); err == nil {
            prefix = "doas "
        } else {
            prefix = "sudo "
        }
    }

    switch family {
    case "alpine":
        return prefix + "apk add " + pkg
    // Future: debian, rhel, arch, suse
    }
    return ""
}
```

### Getting Linux Family

The action needs access to the Linux family. Looking at `ExecutionContext`, it has `Recipe` which we can use, but family detection is in `internal/platform`. Options:
1. Add `LinuxFamily` field to `ExecutionContext`
2. Have the action call `platform.DetectFamily()` directly
3. Pass family through params

The cleanest approach is to have the caller pass family through params, since recipes already know the target family via WhenClause filtering. But for now, we can call `platform.DetectFamily()` directly.

## Scope Clarifications

Based on introspection, these items are **out of scope** for this issue:
- Plan generator aggregation logic (handled in #1114)
- CLI formatting of aggregated errors (handled in #1114)
- Debian/RHEL/Arch/SUSE detection (future work)

This issue focuses on:
- Creating the action with Alpine support
- Structured error type for downstream use
- Unit tests for the action

## Test Strategy

- Unit tests mock exec.Command for `apk info -e` checks
- Tests verify error types and messages
- Tests verify root detection logic
