# Issue 860 Implementation Plan

## Summary

Enable static linking with `CGO_ENABLED=0` for all CI workflow builds so the tsuku binary runs on Alpine Linux (musl) containers. This is a minimal change affecting only the `go build` commands in workflow files.

## Approach

Use Option 1 (Static build) from the issue. Setting `CGO_ENABLED=0` produces a fully static binary that works on any Linux distribution regardless of libc implementation. This approach is:

1. **Already proven**: `.goreleaser.yaml` uses `CGO_ENABLED=0` for production releases
2. **Already used**: `integration_test.go` uses `CGO_ENABLED=0` for container testing
3. **Zero code changes**: Only CI workflow YAML files need modification
4. **No functionality loss**: The project has no CGO dependencies (`import "C"` not used anywhere)

### Alternatives Considered

- **Musl cross-compile**: Build separate Alpine binary using musl toolchain - rejected because it adds complexity (separate build matrix entries) when static linking solves the problem universally
- **Container-based solution**: Build tsuku inside Alpine container - rejected because it duplicates build effort, slows CI, and doesn't leverage existing Go cross-compilation

## Files to Modify

- `.github/workflows/sandbox-tests.yml` - Add `CGO_ENABLED=0` to build command (line 57)
- `.github/workflows/build-essentials.yml` - Add `CGO_ENABLED=0` to 9 build commands (test-sandbox-multifamily job uses sandbox)
- `.github/workflows/validate-golden-execution.yml` - Add `CGO_ENABLED=0` to build commands (lines 154, 269)
- `.github/workflows/validate-golden-recipes.yml` - Add `CGO_ENABLED=0` to build command (line 109)
- `.github/workflows/generate-golden-files.yml` - Add `CGO_ENABLED=0` to build command (line 83)
- `.github/workflows/test-changed-recipes.yml` - Add `CGO_ENABLED=0` to build commands (lines 117, 158)
- `.github/workflows/test.yml` - Add `CGO_ENABLED=0` to build commands (lines 174, 230, 261)

## Files to Create

None.

## Implementation Steps

- [ ] Update `.github/workflows/sandbox-tests.yml` build command to use `CGO_ENABLED=0 go build -o tsuku ./cmd/tsuku`
- [ ] Update `.github/workflows/build-essentials.yml` build commands to use `CGO_ENABLED=0 go build -o tsuku ./cmd/tsuku`
- [ ] Update `.github/workflows/validate-golden-execution.yml` build commands to use static linking
- [ ] Update `.github/workflows/validate-golden-recipes.yml` build command to use static linking
- [ ] Update `.github/workflows/generate-golden-files.yml` build command to use static linking
- [ ] Update `.github/workflows/test-changed-recipes.yml` build commands to use static linking
- [ ] Update `.github/workflows/test.yml` build commands to use static linking
- [ ] Run `go test ./...` to verify no test regressions

## Testing Strategy

- **Unit tests**: Run `go test ./...` to ensure no regressions from static linking
- **Manual verification**: Build with `CGO_ENABLED=0 go build -o tsuku ./cmd/tsuku` and verify binary is statically linked using `file tsuku` (should show "statically linked")
- **CI validation**: The `test-sandbox-multifamily` job in `build-essentials.yml` already tests alpine family - once the fix is applied, this job will pass

## Risks and Mitigations

- **Risk**: Some Go features behave differently with CGO disabled (e.g., DNS resolution uses pure Go resolver)
  - **Mitigation**: The project already uses `CGO_ENABLED=0` in `.goreleaser.yaml` for production releases without issues. Integration tests also use it successfully.

- **Risk**: Slightly larger binary size with static linking
  - **Mitigation**: Size increase is negligible and acceptable for the cross-distro compatibility benefit.

## Success Criteria

- [ ] `test-sandbox-multifamily` job in `build-essentials.yml` passes for all families including alpine
- [ ] All existing CI tests continue to pass
- [ ] Binary built with `CGO_ENABLED=0` shows as "statically linked" via `file` command
- [ ] Alpine golden files can be validated in sandbox tests

## Open Questions

None - the approach is well-established in the existing codebase.
