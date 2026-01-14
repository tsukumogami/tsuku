# Issue 888 Summary

## What Was Implemented

Added fallback logic to search `ctx.ExecPaths` for Go binaries when the global resolver functions (`ResolveGo()` and `ResolveGoVersion()`) return empty. This fixes golden file execution on darwin-arm64 where Go is installed as a dependency to a custom `$TSUKU_HOME` rather than the default `~/.tsuku/tools` directory.

## Changes Made

- `internal/actions/go_build.go`: Added ExecPaths fallback in `Execute()` method for both version-specific and any-version Go resolution paths
- `internal/actions/go_install.go`: Added ExecPaths fallback in `Execute()` method

## Key Decisions

- **Pattern reuse from pip_exec.go**: Followed the existing pattern where global resolver is tried first, then ExecPaths as fallback. This maintains backward compatibility.
- **No changes to Decompose()**: The Decompose method runs at eval time (plan generation), not execution time. The failure occurs during golden file execution, so only Execute methods needed fixing.

## Trade-offs Accepted

- **Version matching for go_build**: For version-specific Go resolution, we check if the ExecPath parent directory ends with `go-<version>`. This is slightly less robust than a full version verification but matches the existing ResolveGoVersion pattern.

## Test Coverage

- New tests added: 0 (existing tests cover the Execute paths)
- Coverage change: No change (pre-existing tests already validate behavior)

## Known Limitations

- The fix relies on directory naming convention (`go-<version>`) for version-specific resolution. If the directory structure changes, this would need updating.

## Future Improvements

- Consider unifying the ExecPaths search logic into a helper function to reduce duplication across actions.
