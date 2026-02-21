# Issue 1817 Implementation Plan

## Summary

Add homepage URL scheme validation to `validateMetadata()` in the Go recipe validator, matching the checks already present in the Python registry generator (`generate-registry.py`). This ensures invalid homepage URLs are caught by `tsuku validate` during PR review instead of failing at website deploy time.

## Approach

Extend the existing `validateMetadata()` function with homepage URL checks. The validation logic mirrors what the Python script does (lines 145-162 of `generate-registry.py`): reject URLs not starting with `https://` and reject dangerous schemes (`javascript:`, `data:`, `vbscript:`). The homepage field is optional in the Go validator (only 170 of 345 recipes set it), so validation only runs when the field is non-empty.

### Alternatives Considered

- **Separate `validateHomepage()` function**: Extracting homepage validation into its own function would be consistent with `validateURLParam()`. However, the homepage check is metadata-specific (not a step parameter), and the logic is only a few lines. Keeping it inline in `validateMetadata()` matches how other metadata fields (name format, type) are validated in the same function. If more URL fields are added to metadata later, extraction would make sense.
- **Reuse `validateURLParam()` for homepage**: The existing `validateURLParam()` accepts both HTTP and HTTPS for step URLs. Homepage validation is stricter (HTTPS only) and needs the dangerous scheme check. Reusing it would require adding parameters to control behavior, which adds unnecessary complexity for a different validation policy.

## Files to Modify

- `internal/recipe/validator.go` - Add homepage URL validation to `validateMetadata()` (after the type validation block, around line 139)
- `internal/recipe/validator_test.go` - Add test cases for homepage validation (valid HTTPS, HTTP rejection, dangerous schemes, empty/missing homepage)

## Files to Create

None.

## Implementation Steps

- [ ] Add homepage validation logic to `validateMetadata()` in `validator.go`: when `r.Metadata.Homepage` is non-empty, check it starts with `https://` (error if not), then check for dangerous schemes (`javascript:`, `data:`, `vbscript:`) via case-insensitive substring match (error if found)
- [ ] Add test `TestValidateBytes_HomepageValidHTTPS` - recipe with `homepage = "https://example.com"` passes validation
- [ ] Add test `TestValidateBytes_HomepageHTTPRejected` - recipe with `homepage = "http://example.com"` produces error on `metadata.homepage` mentioning `https://`
- [ ] Add test `TestValidateBytes_HomepageDangerousSchemes` - table-driven test with `javascript:alert(1)`, `data:text/html,...`, `vbscript:...` all produce errors mentioning "dangerous scheme"
- [ ] Add test `TestValidateBytes_HomepageEmpty` - recipe without homepage field passes validation (no regression)
- [ ] Run `go test ./internal/recipe/...` to confirm all tests pass
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` for lint compliance
- [ ] Run full test suite `go test ./...` to check for regressions

## Testing Strategy

- **Unit tests**: Table-driven tests in `validator_test.go` covering:
  - Valid HTTPS URL (passes)
  - HTTP URL (rejected with error)
  - Each dangerous scheme (rejected with error)
  - Missing/empty homepage (passes, no error)
  - Mixed case dangerous scheme like `JavaScript:` (still caught)
- **Regression**: Run `go test ./...` to confirm no existing tests break
- **Manual verification**: `go build -o tsuku ./cmd/tsuku && ./tsuku validate recipes/a/ansifilter.toml` should pass (homepage is now HTTPS)

## Risks and Mitigations

- **False positives on existing recipes**: All 170 recipes with homepage fields already use `https://`. Running `go test ./...` and checking a sample of real recipes confirms no regressions.
- **Edge cases in dangerous scheme detection**: The Python script uses `lower_homepage` with `in` for substring matching. Using `strings.Contains(strings.ToLower(homepage), scheme)` in Go provides equivalent behavior. The check runs after the `https://` prefix check, so a URL like `https://example.com/javascript:foo` would still be flagged -- matching the Python behavior. This is conservative and acceptable.

## Success Criteria

- [ ] `validateMetadata()` produces an error for `homepage = "http://example.com"`
- [ ] `validateMetadata()` produces an error for `homepage = "javascript:alert(1)"`
- [ ] `validateMetadata()` produces no error for `homepage = "https://example.com"`
- [ ] `validateMetadata()` produces no error when homepage is empty/missing
- [ ] All new tests pass: `go test ./internal/recipe/...`
- [ ] Full test suite passes: `go test ./...`
- [ ] Lint passes: `go vet ./...`

## Open Questions

None.
