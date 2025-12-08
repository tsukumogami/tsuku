# Issue 277 Implementation Plan

## Summary

Extend the Builder interface to accept a `BuildRequest` struct instead of separate `packageName` and `version` parameters. This enables builder-specific arguments like `SourceArg` for the LLM GitHub Release Builder.

## Approach

Update the existing interface to use `BuildRequest`, update all existing builders to match, and update callers. The `CanBuild` method remains on the interface as existing builders still use it for ecosystem validation.

### Alternatives Considered

- **Separate LLMBuilder interface**: Create a new interface just for LLM builders. Rejected because it fragments the builder abstraction and complicates the registry.
- **Add SourceArg as optional parameter**: Add to existing signature. Rejected because it doesn't scale for future builder-specific args.

## Files to Modify

- `internal/builders/builder.go` - Add `BuildRequest` struct, update interface
- `internal/builders/cargo.go` - Update `Build` method signature
- `internal/builders/cargo_test.go` - Update test calls
- `internal/builders/gem.go` - Update `Build` method signature
- `internal/builders/gem_test.go` - Update test calls
- `internal/builders/pypi.go` - Update `Build` method signature
- `internal/builders/pypi_test.go` - Update test calls
- `internal/builders/npm.go` - Update `Build` method signature
- `internal/builders/npm_test.go` - Update test calls
- `internal/builders/go.go` - Update `Build` method signature
- `internal/builders/go_test.go` - Update test calls
- `internal/builders/cpan.go` - Update `Build` method signature
- `internal/builders/cpan_test.go` - Update test calls
- `cmd/tsuku/create.go` - Update caller to use BuildRequest

## Files to Create

None

## Implementation Steps

- [x] Step 1: Add `BuildRequest` struct to `internal/builders/builder.go`
- [x] Step 2: Update `Builder` interface to use `BuildRequest`
- [x] Step 3: Update CargoBuilder to accept BuildRequest
- [x] Step 4: Update GemBuilder to accept BuildRequest
- [x] Step 5: Update PyPIBuilder to accept BuildRequest
- [x] Step 6: Update NpmBuilder to accept BuildRequest
- [x] Step 7: Update GoBuilder to accept BuildRequest
- [x] Step 8: Update CpanBuilder to accept BuildRequest
- [x] Step 9: Update `cmd/tsuku/create.go` to use BuildRequest
- [x] Step 10: Run tests and fix any issues

## Testing Strategy

- Unit tests: All existing builder tests updated to use new signature
- Integration tests: `go test ./...` must pass
- Manual verification: N/A (existing tests provide coverage)

## Risks and Mitigations

- **Breaking existing builders**: Mitigate by updating all builders in single PR
- **Missing callers**: Mitigate by compiler errors (interface change)

## Success Criteria

- [ ] `BuildRequest` struct defined with `Package`, `Version`, `SourceArg` fields
- [ ] `Builder` interface uses `BuildRequest`
- [ ] All existing builders updated
- [ ] All tests pass
- [ ] `go vet` passes
- [ ] Build succeeds

## Open Questions

None - implementation is straightforward.
