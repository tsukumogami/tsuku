# Issue 277 Summary

## What Was Implemented

Extended the Builder interface to accept a `BuildRequest` struct instead of separate `packageName` and `version` parameters. This enables builder-specific arguments like `SourceArg` for the upcoming LLM GitHub Release Builder.

## Changes Made

- `internal/builders/builder.go`: Added `BuildRequest` struct with `Package`, `Version`, and `SourceArg` fields; updated `Builder.Build` interface signature
- `internal/builders/cargo.go`: Updated `Build` method to accept `BuildRequest`
- `internal/builders/gem.go`: Updated `Build` method to accept `BuildRequest`
- `internal/builders/pypi.go`: Updated `Build` method to accept `BuildRequest`
- `internal/builders/npm.go`: Updated `Build` method to accept `BuildRequest`
- `internal/builders/go.go`: Updated `Build` method to accept `BuildRequest`
- `internal/builders/cpan.go`: Updated `Build` method to accept `BuildRequest`
- `cmd/tsuku/create.go`: Updated caller to use `BuildRequest`
- `internal/builders/*_test.go`: Updated all test files to use new signature

## Key Decisions

- **Keep CanBuild method**: Existing builders still use `CanBuild` for ecosystem validation. The interface retains it.
- **Use struct instead of additional parameters**: `BuildRequest` struct is more extensible than adding parameters, and allows builder-specific fields to be added without changing signatures.

## Trade-offs Accepted

- **All callers must change**: Unavoidable for interface change. All tests and CLI updated in same PR.

## Test Coverage

- New tests added: 0 (existing tests updated)
- Coverage change: No change (all existing tests pass)

## Known Limitations

- `SourceArg` is unused by existing builders (crates.io, rubygems, pypi, npm, go, cpan) - it's for the upcoming GitHub Release Builder.

## Future Improvements

- GitHub Release Builder will use `SourceArg` for `owner/repo` specification
- Future builders may need additional fields in `BuildRequest`
