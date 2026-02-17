# Validation Report: Issue #1736 - Platform tokens migrated to secrets package

**Scenario**: scenario-11
**Date**: 2026-02-16
**Environment**: Docker (golang:1.25)

---

## Scenario 11: Platform tokens migrated to secrets package

**Category**: infrastructure
**Status**: PASSED

### Test Execution

**Command**: `go test -v -count=1 ./internal/discover/... ./internal/version/... ./internal/builders/... ./internal/search/...`

**Results**:
- All 4 packages compiled and passed: `ok` status
- 760 tests passed, 0 failed, 4 skipped (integration tests requiring external services)
- Execution time: discover 0.6s, version 32.4s, builders 1.7s, search 31.6s

Package-level results:
```
ok  github.com/tsukumogami/tsuku/internal/discover   0.636s
ok  github.com/tsukumogami/tsuku/internal/version    32.362s
ok  github.com/tsukumogami/tsuku/internal/builders    1.732s
ok  github.com/tsukumogami/tsuku/internal/search     31.604s
```

### Source Verification: No os.Getenv for platform tokens in non-test code

Searched for `os.Getenv("GITHUB_TOKEN")`, `os.Getenv("TAVILY_API_KEY")`, and `os.Getenv("BRAVE_API_KEY")` in the four packages.

**Results**:
- `GITHUB_TOKEN`: Only found in `discover/llm_discovery_test.go:153` (test file, acceptable)
- `TAVILY_API_KEY`: No matches anywhere
- `BRAVE_API_KEY`: No matches anywhere

All non-test source files have been fully migrated away from direct os.Getenv calls.

### Source Verification: secrets package usage confirmed

Production code now uses `secrets.Get()` and `secrets.IsSet()`:

- `discover/llm_discovery.go:798`: `token, _ := secrets.Get("github_token")`
- `discover/validate.go:77`: `token, _ := secrets.Get("github_token")`
- `version/resolver.go:80`: `if token, err := secrets.Get("github_token"); err == nil && token != "" {`
- `version/provider_tap.go:169`: `token, _ := secrets.Get("github_token")`
- `builders/github_release.go:695`: `Authenticated: secrets.IsSet("github_token"),`
- `builders/github_release.go:765`: `Authenticated: secrets.IsSet("github_token"),`
- `builders/github_release.go:830`: `if token, err := secrets.Get("github_token"); err == nil && token != "" {`
- `search/factory.go:18`: `key, err := secrets.Get("tavily_api_key")`
- `search/factory.go:25`: `key, err := secrets.Get("brave_api_key")`
- `search/factory.go:36`: `if key, err := secrets.Get("tavily_api_key"); err == nil {`
- `search/factory.go:39`: `if key, err := secrets.Get("brave_api_key"); err == nil {`

### Error message verification

The secrets package error messages reference both environment variables and config file options:
```go
// secrets.go:90
"%s not configured. Set the %s environment variable, or add %s to [secrets] in $TSUKU_HOME/config.toml"
```

### Skipped tests (4)

These are integration tests requiring external services, not related to the migration:
- `TestDDGProvider_Integration` - requires INTEGRATION_TESTS=1
- Other skips in version/discover packages for CI/API-dependent tests

---

## Summary

Scenario 11 **passed**. All 760 tests across the four target packages pass. No `os.Getenv` calls for GITHUB_TOKEN, TAVILY_API_KEY, or BRAVE_API_KEY remain in non-test source files. All platform token access now goes through `secrets.Get()` and `secrets.IsSet()`, and error messages reference both env var and config file options.
