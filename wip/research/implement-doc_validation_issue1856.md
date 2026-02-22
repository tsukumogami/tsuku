# Validation Report: Issue #1856

## Summary

All 4 scenarios passed. The implementation correctly adds subcategory support to the CLI install error JSON output.

## Environment

- Branch: `docs/structured-error-subcategories`
- Commit: `52f6b6fa` (feat(#1856): add subcategory to install error JSON output)
- Platform: Linux 6.17.0-14-generic
- Go: system go toolchain
- Isolation: `/tmp/qa-tsuku-*` via setup-env.sh (cleaned up)

---

## Scenario 1: classifyInstallError returns subcategory for timeout errors

**ID**: scenario-1
**Status**: PASSED

**Command**: `go test ./cmd/tsuku/... -run TestClassifyInstallError -v`

**Verification**:
- `TestClassifyInstallError/timeout_registry_error` passes
- Test case creates `&registry.RegistryError{Type: registry.ErrTypeTimeout, Message: "request timed out"}`
- Asserts `gotCode == ExitNetwork` and `gotSubcategory == "timeout"`
- Both assertions pass

---

## Scenario 2: classifyInstallError returns subcategory for DNS, TLS, and connection errors

**ID**: scenario-2
**Status**: PASSED

**Command**: `go test ./cmd/tsuku/... -run TestClassifyInstallError -v`

**Verification**:
- `TestClassifyInstallError/DNS_registry_error` passes: subcategory = "dns_error"
- `TestClassifyInstallError/TLS_registry_error` passes: subcategory = "tls_error"
- `TestClassifyInstallError/connection_registry_error` passes: subcategory = "connection_error"
- `TestClassifyInstallError/not_found_registry_error` passes: subcategory = "" (ErrTypeNotFound)
- `TestClassifyInstallError/network_registry_error` passes: subcategory = "" (generic ErrTypeNetwork)
- `TestClassifyInstallError/dependency_failure` passes: subcategory = "" (dependency wrapper)
- `TestClassifyInstallError/dependency_failure_wrapping_RegistryError_NotFound` passes: subcategory = "" (dependency wrapper takes precedence)
- `TestClassifyInstallError/generic_install_error` passes: subcategory = "" (catch-all)
- All 12 sub-test cases pass

---

## Scenario 3: installError JSON includes subcategory when non-empty

**ID**: scenario-3
**Status**: PASSED

**Command**: `go test ./cmd/tsuku/... -run TestInstallErrorJSON -v`

**Verification**:
- `TestInstallErrorJSON/with_subcategory` passes: marshaled JSON contains `"subcategory":"timeout"` when Subcategory is set
- `TestInstallErrorJSON/without_subcategory` passes: subcategory key is omitted from JSON when Subcategory is empty (omitempty behavior verified by asserting key does not exist in parsed map)

---

## Scenario 4: handleInstallError populates subcategory in JSON output

**ID**: scenario-4
**Status**: PASSED

**Commands**:
- `go build ./cmd/tsuku` -- builds successfully with no errors
- `go vet ./cmd/tsuku/...` -- passes with no issues

**Verification**:
- `handleInstallError()` at install.go:368 uses `code, subcategory := classifyInstallError(err)` (both return values)
- The `installError` struct literal at install.go:370-377 includes `Subcategory: subcategory`
- The `installError` struct has `Subcategory string` field with `json:"subcategory,omitempty"` tag at install.go:333
- Build and vet both succeed, confirming all call sites compile correctly
