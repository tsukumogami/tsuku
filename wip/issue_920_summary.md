# Issue 920 Summary

## What Was Implemented
Fixed the planner to use target OS instead of host OS when resolving platform-specific action dependencies in transitive dependency resolution. This ensures that plans generated on Linux CI for Darwin targets correctly exclude Linux-specific dependencies like patchelf.

## Changes Made
- `internal/actions/resolver.go`: Added `ResolveTransitiveForPlatform` function with target OS parameter; updated `resolveTransitiveSet` to use `ResolveDependenciesForPlatform` and properly merge recursive results back into the main deps map
- `internal/actions/resolver_test.go`: Added 4 new tests for cross-platform transitive dependency resolution
- `internal/executor/plan_generator.go`: Updated `generateDependencyPlans` to use target OS from config instead of `runtime.GOOS`
- `testdata/golden/exclusions.json`: Removed exclusions for libcurl and ncurses (issue #920)
- `testdata/golden/plans/l/libcurl/`: Generated golden files for all platforms
- `testdata/golden/plans/n/ncurses/`: Generated golden files for all platforms

## Key Decisions
- **Backward compatibility**: Kept `ResolveTransitive` as a thin wrapper that defaults to `runtime.GOOS`, ensuring existing callers continue to work
- **Merge recursive results**: Modified `resolveTransitiveSet` to merge nested dependencies back into the parent map, fixing an existing bug where deeply nested transitive deps weren't properly accumulated

## Trade-offs Accepted
- The `resolveTransitiveSet` function now has an additional parameter (targetOS), making it slightly more complex, but this is necessary for correct cross-platform plan generation

## Test Coverage
- New tests added: 4 (`TestResolveTransitiveForPlatform_*`)
- Tests verify: linux deps excluded on darwin target, linux deps included on linux target, nested deps handled correctly, runtime deps also respect platform filtering

## Known Limitations
- None

## Future Improvements
- None needed - the fix is complete and addresses the root cause
