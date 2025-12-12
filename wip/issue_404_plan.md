# Issue 404 Implementation Plan

## Summary

Add `Plan` field to `VersionState` and `InstallOptions`, generate installation plan during `tsuku install`, and store it in state.json after successful installation.

## Approach

Extend the existing state management with a Plan field that stores the same structure output by `tsuku eval`. The plan is generated during installation (after recipe execution succeeds) and stored alongside version metadata. This enables future plan inspection and deterministic re-installation.

### Alternatives Considered

- **Separate plan files**: Rejected per design doc - inline storage is simpler and uses existing file locking
- **Pre-execution plan generation**: Rejected - plan should reflect actual installation, generated post-execution

## Files to Modify

- `internal/install/state.go` - Add Plan field to VersionState
- `internal/install/manager.go` - Add Plan field to InstallOptions, store in VersionState
- `cmd/tsuku/install_deps.go` - Generate plan during installation and pass to InstallWithOptions

## Implementation Steps

- [ ] Add `Plan` field to `VersionState` struct in `state.go`
- [ ] Add `Plan` field to `InstallOptions` struct in `manager.go`
- [ ] Update `InstallWithOptions` to store plan in version state
- [ ] Update `installWithDependencies` to generate plan and pass to InstallWithOptions
- [ ] Add unit tests for plan storage in state
- [ ] Run `go vet`, `go test`, and `go build` to verify

## Testing Strategy

- Unit tests: Verify plan is stored and retrieved from state.json
- Unit tests: Verify backward compatibility (state without plans loads correctly)
- Manual verification: `tsuku install gh && cat ~/.tsuku/state.json | jq '.installed.gh.versions'`

## Risks and Mitigations

- **State file size increase**: Acceptable per design doc (1-5KB per tool typically)
- **Backward compatibility**: Use `omitempty` JSON tag, nil Plan for existing state

## Success Criteria

- [ ] `VersionState` includes optional `Plan` field
- [ ] Plan stored after successful installation
- [ ] Existing state.json files without plans load correctly
- [ ] Plan structure matches `tsuku eval` output
- [ ] All tests pass, no lint errors

## Open Questions

None - design document provides clear guidance.
