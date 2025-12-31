# Issue 762 Implementation Plan

## Summary

Implement `Preflight()` methods on all package manager actions to enable consistent validation across both configuration and PM actions, with security-focused checks for HTTPS URLs and required SHA256 hashes.

## Approach

Implement `Preflight()` on PM actions by wrapping the existing `Validate()` logic and adding security checks (HTTPS validation for URLs). This follows the established pattern in `system_config.go` where configuration actions already implement `Preflight()` returning `*PreflightResult`.

### Alternatives Considered

- **Option A: Replace Validate() with Preflight()**: Would break `SystemAction` interface contract and require updating all call sites. Rejected - too invasive.
- **Option B: Separate validation logic in Preflight() and Validate()**: Would duplicate logic and risk drift. Rejected - not DRY.
- **Option C (Chosen): Preflight() wraps Validate() + adds security checks**: Maintains backward compatibility, DRY, adds security validation. Selected because it builds on existing patterns with minimal disruption.

## Files to Modify

- `internal/actions/apt_actions.go` - Add `Preflight()` to `AptInstallAction`, `AptRepoAction`, `AptPPAAction`
- `internal/actions/dnf_actions.go` - Add `Preflight()` to `DnfInstallAction`, `DnfRepoAction`
- `internal/actions/brew_actions.go` - Add `Preflight()` to `BrewInstallAction`, `BrewCaskAction`
- `internal/actions/linux_pm_actions.go` - Add `Preflight()` to `PacmanInstallAction`, `ApkInstallAction`, `ZypperInstallAction`
- `internal/actions/system_action.go` - Add `ValidatePackagesPreflight()` helper, add HTTPS validation helper

## Files to Create

- `internal/actions/apt_actions_preflight_test.go` - Preflight tests for apt actions (or add to existing `apt_actions_test.go`)
- `internal/actions/dnf_actions_preflight_test.go` - Preflight tests for dnf actions (or add to existing)
- `internal/actions/brew_actions_preflight_test.go` - Preflight tests for brew actions (or add to existing)
- `internal/actions/linux_pm_actions_preflight_test.go` - Preflight tests for linux PM actions (or add to existing)

Note: Tests will be added to existing `*_test.go` files to follow existing patterns.

## Implementation Steps

- [ ] Add HTTPS validation helper to `system_action.go`
  - Create `isHTTPS(url string) bool` helper
  - Create `ValidatePackagesPreflight(params, actionName) *PreflightResult` helper that wraps `ValidatePackages` and converts to `PreflightResult`

- [ ] Add `Preflight()` to install actions (apt_install, dnf_install, brew_install, brew_cask, pacman_install, apk_install, zypper_install)
  - Wrap `ValidatePackages` validation
  - All install actions use the same pattern: packages required, no additional URL validation needed

- [ ] Add `Preflight()` to `AptRepoAction` with security checks
  - Validate `url`, `key_url`, `key_sha256` are present
  - Validate `url` and `key_url` use HTTPS scheme
  - Validate `key_sha256` is non-empty (required per acceptance criteria)

- [ ] Add `Preflight()` to `DnfRepoAction` with security checks
  - Validate `url`, `key_url`, `key_sha256` are present
  - Validate `url` and `key_url` use HTTPS scheme
  - Validate `key_sha256` is required when `key_url` is provided (per issue spec)

- [ ] Add `Preflight()` to `AptPPAAction`
  - Validate `ppa` parameter is present and non-empty
  - Validate PPA format (should be "owner/repo" style)

- [ ] Add unit tests for all Preflight() methods
  - Follow `system_config_test.go` pattern: table-driven with `wantErrors`/`wantErrMsg`
  - Test valid params, missing required params, invalid URL schemes, missing SHA256

- [ ] Verify recipe validation integration
  - Existing `ValidateAction()` in `preflight.go` already dispatches to `Preflight()` if implemented
  - Recipe validation in `validator.go` already calls `av.ValidateAction()` for each step
  - No additional integration code needed - just implementing the interface is sufficient

- [ ] Run tests and verify no regressions

## Testing Strategy

- **Unit tests**: Table-driven tests for each action's `Preflight()` method following `system_config_test.go` pattern
  - Valid parameters pass without errors
  - Missing required parameters produce specific error messages
  - Invalid URL schemes (HTTP instead of HTTPS) produce errors for repo actions
  - Missing SHA256 produces errors for repo actions
  - Empty package lists produce errors for install actions

- **Integration tests**: Recipe validation already exercises `Preflight()` through the `ActionValidator` interface
  - Existing recipe validation tests in `validator_test.go` provide coverage
  - May add specific test cases for PM actions if coverage gaps exist

- **Manual verification**:
  - `go test ./internal/actions/...` - verify all action tests pass
  - `go test ./internal/recipe/...` - verify recipe validation integration

## Risks and Mitigations

- **Risk**: Breaking existing `Validate()` behavior by changing return types
  - **Mitigation**: Keep `Validate()` unchanged; `Preflight()` is additive and wraps existing validation

- **Risk**: Inconsistent error messages between `Validate()` and `Preflight()`
  - **Mitigation**: `Preflight()` reuses validation logic from `Validate()`, just wraps in `PreflightResult`

- **Risk**: HTTPS enforcement may break existing recipes using HTTP URLs in test environments
  - **Mitigation**: This is intentional security hardening per design doc; HTTP for production external resources is not acceptable

## Success Criteria

- [ ] All package manager actions have `Preflight()` method
- [ ] `apt_repo`: requires `key_sha256` unconditionally
- [ ] `dnf_repo`: requires `key_sha256` when `key_url` is provided
- [ ] All repo actions validate HTTPS for `url` and `key_url` parameters
- [ ] Error messages are actionable (include field name, expected format)
- [ ] All unit tests pass
- [ ] Recipe validation correctly invokes `Preflight()` on PM actions (already works via `ValidateAction()`)
- [ ] No regressions in existing tests

## Open Questions

None - all gaps identified in introspection have been resolved:
1. Interface signature: Using `Preflight(params) *PreflightResult` per established pattern
2. Preflight wrapping Validate: Option A selected (wrap, don't duplicate)
3. HTTPS enforcement scope: Applied to `apt_repo` and `dnf_repo` for `url` and `key_url` fields
4. dnf_repo key_sha256: Keep current behavior (always required) per design doc
