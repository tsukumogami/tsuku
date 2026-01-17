# Issue 979 Implementation Plan

## Goal

Add `IsExternallyManaged() bool` method to the `SystemAction` interface to distinguish package manager actions from other system actions.

## Approach

The implementation is simple interface extension with method implementation across all `SystemAction` implementers:

1. Add method signature to `SystemAction` interface in `system_action.go`
2. Implement returning `true` on 10 package manager actions:
   - `apt_actions.go`: AptInstallAction, AptRepoAction, AptPPAAction
   - `brew_actions.go`: BrewInstallAction, BrewCaskAction
   - `dnf_actions.go`: DnfInstallAction, DnfRepoAction
   - `linux_pm_actions.go`: PacmanInstallAction, ApkInstallAction, ZypperInstallAction
3. Implement returning `false` on 5 other system actions in `system_config.go`:
   - GroupAddAction, ServiceEnableAction, ServiceStartAction, RequireCommandAction, ManualAction
4. Add tests in `system_action_test.go`

## Files to Modify

| File | Change |
|------|--------|
| `internal/actions/system_action.go` | Add `IsExternallyManaged() bool` to interface |
| `internal/actions/apt_actions.go` | Add method on 3 actions (return true) |
| `internal/actions/brew_actions.go` | Add method on 2 actions (return true) |
| `internal/actions/dnf_actions.go` | Add method on 2 actions (return true) |
| `internal/actions/linux_pm_actions.go` | Add method on 3 actions (return true) |
| `internal/actions/system_config.go` | Add method on 5 actions (return false) |
| `internal/actions/system_action_test.go` | Add test for IsExternallyManaged |

## Testing Strategy

Add table-driven test to `system_action_test.go` verifying:
- All 10 package manager actions return `true`
- All 5 other system actions return `false`
- Ensure interface compliance

## Risk Assessment

Low risk:
- Simple interface extension
- No behavior change to existing code
- All implementations are trivial one-liner methods
