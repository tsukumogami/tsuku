# Issue 563 Introspection

## Context Reviewed

- **Design doc**: `docs/DESIGN-dependency-provisioning.md` (Phase 9, step 3: "Add `tsuku check-deps <recipe>` command")
- **Sibling issues reviewed**:
  - #560 (CLOSED): `require_system` action with detection - establishes detection/validation patterns
  - #561 (CLOSED): docker recipe using require_system
  - #562 (CLOSED): cuda recipe using require_system
  - #643 (CLOSED): platform-conditional dependencies - adds LinuxInstallTime/DarwinInstallTime to ActionDeps
  - #644 (OPEN): aggregate primitive action deps - related but not blocking
- **Prior patterns identified**:
  - Dependency resolution via `actions.ResolveDependencies()` and `actions.ResolveTransitive()`
  - CLI commands use cobra patterns in `cmd/tsuku/`
  - `info.go` already shows dependencies using the resolver (good reference)
  - `require_system` returns `SystemDepMissingError` or `SystemDepVersionError` with install guides
  - Pattern for distinguishing system-required recipes: `isSystemDependencyPlan()` checks if all steps are `require_system`

## Gap Analysis

### Minor Gaps

1. **File location is clear**: Should be `cmd/tsuku/check_deps.go` following kebab-case to underscore convention (like `install_deps.go`, `update_registry.go`)

2. **Dependency resolution patterns established**: The `info.go` command already demonstrates how to resolve dependencies:
   ```go
   directDeps := actions.ResolveDependencies(r)
   resolvedDeps, err := actions.ResolveTransitive(context.Background(), loader, directDeps, toolName)
   ```

3. **System dependency detection patterns established**: The `require_system.go` action provides:
   - `SystemDepMissingError` struct with `Command` and `InstallGuide` fields
   - `SystemDepVersionError` struct with `Command`, `Found`, `Required`, and `InstallGuide` fields
   - These should be leveraged for status reporting

4. **Platform-conditional deps now supported**: Issue #643 added `LinuxInstallTime` and `DarwinInstallTime` to `ActionDeps`. The resolver in `resolver.go` handles this via `getPlatformInstallDeps()` - the check-deps command should use `ResolveDependencies()` which handles platform filtering automatically.

5. **Colorized output**: Other commands use `fmt.Printf` with manual formatting. For colorized output, could use ANSI escape codes or a lightweight library (check existing patterns in codebase).

### Moderate Gaps

None identified. The acceptance criteria are clear and the patterns from prior work provide sufficient guidance.

### Major Gaps

None identified. The blocking dependency (#560) is closed and implemented. All prerequisite work is complete.

## Recommendation

**Proceed**

The issue specification is complete and implementation patterns are well-established from:
1. `require_system` action provides the core detection/error types
2. `info.go` demonstrates dependency resolution with transitive support
3. Recipe loading pattern via global `loader` variable
4. CLI command registration pattern in `main.go`

## Implementation Notes

Based on patterns from prior work:

1. **Dependency classification**: Load each dependency's recipe and check if all its steps are `require_system` actions to classify as "system-required" vs "provisionable" (pattern from `isSystemDependencyPlan()`)

2. **Status detection**: For system-required deps, execute the `require_system` action's detection logic. For provisionable deps, check state via `mgr.GetState()`.

3. **Exit codes**: Use `ExitGeneral` (1) from `exitcodes.go` when system deps are missing per acceptance criteria.

4. **Colorized output**: Consider using ANSI codes directly (green for installed, red for missing, yellow for version mismatch) since no color library is currently in use.

5. **Recipe not found handling**: When a dependency's recipe doesn't exist, treat as "unknown" with appropriate messaging.

## Proposed Implementation Structure

```go
// cmd/tsuku/check_deps.go
var checkDepsCmd = &cobra.Command{
    Use:   "check-deps <recipe>",
    Short: "Check dependency status for a recipe",
    Args:  cobra.ExactArgs(1),
    Run:   runCheckDeps,
}

type DepStatus struct {
    Name        string
    Type        string  // "provisionable" or "system-required"
    Status      string  // "installed", "missing", "version_mismatch"
    Version     string  // Installed version if any
    Required    string  // Required version if specified
    InstallGuide string // For system-required deps
}
```
