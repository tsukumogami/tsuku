# Issue #762 Introspection

## Context Reviewed

- Design doc: `docs/DESIGN-system-dependency-actions.md`
- Sibling issues reviewed: #754 (Target struct), #755 (package install structs), #756 (config/verify structs), #759 (linux_family detection)
- Prior patterns identified:
  - Configuration actions use `Preflight(params map[string]interface{}) *PreflightResult` interface
  - Package install actions use `Validate(params map[string]interface{}) error` from `SystemAction` interface
  - `ValidateAction()` in `preflight.go` dispatches to `Preflight()` if implemented

## Gap Analysis

### Minor Gaps

1. **Interface signature clarification**: Issue says `Preflight() error` but codebase uses `Preflight(params map[string]interface{}) *PreflightResult`. This is the established pattern from #756 implementation - use it consistently.

2. **File locations**: New preflight methods for PM actions should go in their respective files (`apt_actions.go`, `dnf_actions.go`, `brew_actions.go`, `linux_pm_actions.go`).

3. **Testing pattern**: Follow `system_config_test.go` pattern for preflight tests - table-driven tests with `wantErrors`/`wantErrMsg` structure.

### Moderate Gaps

1. **`Validate()` vs `Preflight()` duality**: PM actions currently have `Validate()` from `SystemAction` interface but need `Preflight()` for recipe validation. Two options:
   - Option A: Have `Preflight()` call `Validate()` internally (DRY)
   - Option B: Duplicate validation logic (less coupling)

   **Recommendation**: Option A - implement `Preflight()` that wraps `Validate()` and adds additional security checks (HTTPS validation).

2. **HTTPS enforcement not specified in detail**: Issue says "All external URLs must be HTTPS" but doesn't clarify:
   - Which actions have URLs? (`apt_repo`, `dnf_repo`, `apt_ppa` via PPA URL expansion?)
   - What about development/testing scenarios with local HTTP servers?

   **Recommendation**: Enforce HTTPS for `url` and `key_url` fields in `apt_repo` and `dnf_repo`. PPAs are Ubuntu-specific and don't have user-provided URLs.

3. **dnf_repo key_sha256 conditional requirement**: Issue says "key_sha256 REQUIRED when key_url is provided" but current `Validate()` requires all three (`url`, `key_url`, `key_sha256`). Design doc also shows `key_sha256` as always required for `dnf_repo`.

   **Clarification needed**: Is there a valid use case for `dnf_repo` without `key_url`? If not, current implementation is correct. The issue wording may have been imprecise.

### Major Gaps

None identified. The issue spec is implementable with the patterns established by sibling issues.

## Recommendation

**Proceed** - All gaps are minor or moderate and can be resolved during implementation using established patterns.

## Proposed Amendments

For moderate gaps:

1. **Preflight wrapping**: Implement `Preflight()` on all PM actions that:
   - Wraps the existing `Validate()` call
   - Adds HTTPS URL validation
   - Converts `error` to `*PreflightResult`

2. **HTTPS validation scope**: Apply to `apt_repo` and `dnf_repo` actions for `url` and `key_url` fields.

3. **dnf_repo requirement**: Keep current behavior (all three fields required) - this matches design doc. No change needed.

## Implementation Notes from Sibling Issues

### From #755 (Package install structs)
- PM actions embed `BaseAction` for default behavior
- `Validate()` uses `ValidatePackages()` helper for install actions
- `GetStringSlice()` helper for parameter extraction

### From #756 (Config/verify structs)
- `Preflight()` returns `*PreflightResult` with `AddError()`/`AddErrorf()` methods
- Validation helper functions: `isValidGroupName()`, `isValidServiceName()`, `isValidCommandName()`
- Test pattern: table-driven with `wantErrors int` and `wantErrMsg string`

### Pattern for URL validation
```go
func isValidHTTPS(url string) bool {
    return strings.HasPrefix(url, "https://")
}
```

Add this to validation in `Preflight()` methods for `apt_repo` and `dnf_repo`.
