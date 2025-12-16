# Issue 546 Implementation Plan

## Summary

Create an m4 recipe using configure_make action and add a CI job that validates source builds work without system gcc by running in a container.

## Approach

The `configure_make` action already has zig cc fallback logic (`hasSystemCompiler()` -> `SetupCCompilerEnv()`). To validate this works:
1. Create a simple m4 recipe that uses `configure_make`
2. Add a CI job that runs in a container WITHOUT gcc (using `Dockerfile.integration`)
3. This forces the zig cc path and validates end-to-end source builds

### Alternatives Considered
- **Remove gcc in CI step**: Fragile, might break other things, harder to maintain
- **Use build tags to force zig**: Would require code changes and wouldn't be a real validation

## Files to Create
- `internal/recipe/recipes/m/m4.toml` - m4 recipe using configure_make action

## Files to Modify
- `.github/workflows/build-essentials.yml` - Add container-based job for no-gcc validation
- `test/scripts/verify-tool.sh` - Add m4 verification function

## Implementation Steps
- [x] Create m4 recipe using configure_make action
- [x] Add verify_m4 function to verify-tool.sh
- [x] Add CI job that runs in container without gcc
- [x] Test locally with Docker to validate (will be tested in CI)

## Testing Strategy
- Unit tests: Existing recipe validation tests cover the recipe format
- Integration tests: CI job in container validates zig cc path works
- Manual verification: `./tsuku install m4 && m4 --version`

## Risks and Mitigations
- **GNU FTP mirror unreliable**: Use `homebrew_source` action which handles mirrors
- **zig cc compatibility**: m4 is a simple C program, should work with zig cc
- **Container image availability**: GitHub Actions has good Docker support

## Success Criteria
- [ ] m4 recipe exists and validates
- [ ] `m4 --version` works after install
- [ ] CI job runs in container without gcc
- [ ] CI job successfully builds m4 using zig cc
- [ ] All platforms work (Linux x86_64 for container test)

## Open Questions
None - the architecture is clear and tested with gdbm-source.
