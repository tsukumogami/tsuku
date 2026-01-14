# Issue 888 Implementation Plan

## Summary

Fix the `go_build` and `go_install` actions to find Go installed as a dependency by checking `ctx.ExecPaths` in addition to the global `~/.tsuku/tools` location.

## Approach

The bug occurs because `ResolveGoVersion()` and `ResolveGo()` only look in the default `~/.tsuku/tools/` directory, but during golden file execution in CI, dependencies are installed to a custom `$TSUKU_HOME` per golden file. The fix follows the same pattern used by `pip_exec.go` - first try the global resolver, then fall back to searching `ctx.ExecPaths`.

### Alternatives Considered

- **Modify ResolveGoVersion to respect TSUKU_HOME**: This would require passing TSUKU_HOME through all code paths and modifying the utility function signature. More invasive than necessary.
- **Always use ctx.ExecPaths only**: This would break standalone `go_install` usage where Go is already globally installed. Need to preserve backward compatibility.

## Files to Modify

- `internal/actions/go_build.go` - Add fallback to search `ctx.ExecPaths` for Go binary when `ResolveGoVersion()` fails
- `internal/actions/go_install.go` - Add fallback to search `ctx.ExecPaths` for Go binary when `ResolveGo()` fails

## Files to Create

None.

## Implementation Steps

- [x] Modify `go_build.go` `Execute()` method to search `ctx.ExecPaths` when `ResolveGoVersion()` returns empty
- [x] Modify `go_install.go` `Execute()` method to search `ctx.ExecPaths` when `ResolveGo()` returns empty
- [x] ~~Modify `go_install.go` `Decompose()` method~~ (Not needed: Decompose runs at eval time, not execution time)
- [x] Run `go test ./internal/actions/...` to verify no regressions
- [x] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` for code quality

## Testing Strategy

- Unit tests: Existing tests in `internal/actions/go_build_test.go` and `internal/actions/go_install_test.go` should continue to pass
- Integration tests: The fix will be validated by CI when the PR runs the `validate-golden-execution.yml` workflow on darwin-arm64
- Manual verification: None required (CI provides full coverage)

## Risks and Mitigations

- **Risk**: Breaking existing go_install/go_build functionality for users with global Go installation
  - **Mitigation**: Keep `ResolveGoVersion()`/`ResolveGo()` as first attempt, only fall back to ExecPaths when they return empty

- **Risk**: ExecPaths search not finding Go binary with correct version
  - **Mitigation**: Search for exact version match when `go_version` param is specified; match the existing pattern from `pip_exec.go`

## Success Criteria

- [ ] Golden file execution passes for cobra-cli on darwin-arm64 in CI
- [ ] Golden file execution passes for dlv on darwin-arm64 in CI (same root cause)
- [ ] All existing tests pass (`go test ./...`)
- [ ] Code passes linting (`golangci-lint run --timeout=5m ./...`)

## Open Questions

None - root cause is clear and fix pattern is established in codebase.
