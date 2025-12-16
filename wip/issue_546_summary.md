# Issue 546 Summary

## What Was Implemented

Added m4 recipe to validate end-to-end source builds without system gcc. The CI now includes a container-based job that explicitly verifies the zig cc fallback works when no system compiler exists.

## Changes Made
- `internal/recipe/recipes/m/m4.toml`: New m4 recipe using configure_make with zig/make dependencies
- `test/scripts/verify-tool.sh`: Added verify_m4 function for functional testing
- `.github/workflows/build-essentials.yml`: Added test-no-gcc job running in ubuntu:22.04 container without gcc

## Key Decisions
- **Container-based CI**: Using GitHub Actions `container:` directive with ubuntu:22.04 provides a clean environment without gcc, forcing the zig cc path
- **m4 as validation target**: m4 is a simple C program ideal for compiler validation - if it builds with zig cc, more complex programs should too

## Trade-offs Accepted
- **Linux-only no-gcc test**: The container test only runs on Linux x86_64. macOS runners don't support containers. This is acceptable since the zig cc fallback logic is platform-independent.

## Test Coverage
- No new unit tests needed - recipe validation covered by existing tests
- Integration test coverage via CI job that builds m4 in no-gcc environment

## Known Limitations
- Container test only validates Linux x86_64, not macOS or other architectures
- m4 has minimal dependencies, so doesn't fully exercise complex dependency chains

## Future Improvements
- Could add more complex source builds to no-gcc validation (e.g., gdbm with readline)
- Could test on other platforms if container support expands
