# Issue 557 & 558 Implementation Plan

## Summary

Update the existing readline recipe to declare ncurses dependency, then create a new sqlite recipe that builds from source with readline support. This validates the complete dependency chain: sqlite → readline → ncurses.

## Approach

Following the established patterns from PR #659 (cmake + ninja), this implementation:

1. **Update readline recipe** - Add ncurses dependency declaration to the existing readline.toml
2. **Create sqlite recipe** - Build sqlite from source using configure_make action with readline dependency
3. **Add test script** - Create test-readline-provisioning.sh that validates readline isn't available in system and tsuku provisions it
4. **Update CI matrix** - Add sqlite to build-essentials.yml to validate on all 4 platforms
5. **Update design doc** - Mark issues #557 and #558 as done in DESIGN-dependency-provisioning.md mermaid diagrams

This approach reuses proven patterns:
- configure_make action with dependency declarations (like curl recipe)
- Test script structure from test-cmake-provisioning.sh
- CI matrix pattern from build-essentials.yml
- ExecPaths for finding installed dependencies

### Alternatives Considered

- **Use homebrew bottle for sqlite**: Rejected because the goal is to validate building from source with library dependencies. Using pre-built bottles wouldn't test the configure_make + dependency resolution workflow.
- **Test readline separately without consumer**: Rejected because readline is a library (no standalone binary to test). Need a real consumer like sqlite to validate it works.

## Files to Modify

- `internal/recipe/recipes/r/readline.toml` - Add ncurses dependency and binaries that include .so/.dylib files
- `docs/DESIGN-dependency-provisioning.md` - Update mermaid diagrams to mark #557, #558 as done
- `.github/workflows/build-essentials.yml` - Add sqlite to test matrix
- `test/scripts/verify-tool.sh` - Add verify_sqlite and verify_readline functions

## Files to Create

- `internal/recipe/recipes/s/sqlite.toml` - sqlite recipe using configure_make action
- `test/scripts/test-readline-provisioning.sh` - Test script validating readline provisioning in clean environment

## Implementation Steps

- [ ] Update readline.toml to declare ncurses dependency
- [ ] Add readline library binaries (.so/.dylib files) to readline.toml install_binaries
- [ ] Create sqlite.toml recipe with readline dependency
  - Use download action to fetch sqlite source from sqlite.org
  - Use extract action for tar.gz
  - Use setup_build_env action
  - Use configure_make action with readline dependency and --enable-readline flag
  - Use set_rpath action for sqlite3 binary with readline/ncurses lib paths
  - Use install_binaries action for sqlite3 executable
- [ ] Create test-readline-provisioning.sh following test-cmake-provisioning.sh pattern
  - Dockerfile creates Ubuntu 22.04 container without readline/ncurses
  - Verify readline/ncurses not in system
  - Build tsuku and install sqlite
  - Test sqlite3 --version works
  - Test interactive mode works (validates readline functional)
- [ ] Add verify_readline function to verify-tool.sh
  - Check that readline library files exist in tools directory
- [ ] Add verify_sqlite function to verify-tool.sh
  - Test sqlite3 --version
  - Test basic SQL operations (CREATE TABLE, INSERT, SELECT)
  - Verify readline support by checking for interactive mode functionality
- [ ] Update build-essentials.yml to add sqlite to test matrix
  - Add sqlite to test-homebrew matrix tool list
  - Exclude macOS if needed (check if readline bottles work on macOS)
- [ ] Update DESIGN-dependency-provisioning.md mermaid diagrams
  - Mark #557 (readline recipe) as done
  - Mark #558 (sqlite recipe) as done
  - Update class definitions to show them as green

## Testing Strategy

**Unit tests**: Not applicable - this is recipe-only change

**Integration tests**:
1. **CI matrix test** - sqlite installs on all 4 platforms (Linux x86_64, Linux arm64, macOS Intel, macOS Apple Silicon)
2. **Functionality test** - sqlite3 --version works and shows version
3. **Readline integration test** - Interactive mode works (validates readline linkage)
4. **Clean environment test** - test-readline-provisioning.sh validates readline provisioning without system readline/ncurses
5. **Dependency chain test** - Validates sqlite → readline → ncurses dependency resolution

**Manual verification**:
- Run tsuku install sqlite locally
- Test sqlite3 interactive mode with up/down arrow keys (readline history)
- Verify pkg-config reports correct readline paths
- Check ldd/otool shows tsuku-provided readline/ncurses libraries

## Risks and Mitigations

**Risk**: Readline recipe already exists but may not have correct dependency declarations
- **Mitigation**: Review and update existing readline.toml to add ncurses dependency

**Risk**: SQLite configure script may not find readline even with PKG_CONFIG_PATH set
- **Mitigation**: Use explicit configure flags (--with-readline) and CPPFLAGS/LDFLAGS from setup_build_env

**Risk**: RPATH setup for sqlite3 binary may be complex due to multiple library dependencies
- **Mitigation**: Follow curl recipe pattern which already handles openssl + zlib RPATH setup

**Risk**: Readline bottles on macOS may have unrelocated paths (similar to gdbm issue #605)
- **Mitigation**: Test on macOS early; add exclusion to CI matrix if needed; consider building readline from source as fallback

**Risk**: Interactive mode testing is difficult to automate
- **Mitigation**: Use expect/unbuffer or simple echo-based test; document manual verification steps

## Success Criteria

- [ ] readline recipe declares dependencies = ["ncurses"]
- [ ] readline installs successfully on all 4 platforms (or documented exclusions)
- [ ] sqlite recipe builds from source with readline dependency
- [ ] sqlite3 --version works and shows version number
- [ ] Interactive mode works (validates readline functional)
- [ ] CI validates sqlite on all 4 platforms
- [ ] test-readline-provisioning.sh passes in clean container
- [ ] verify-tool.sh has readline and sqlite verification functions
- [ ] DESIGN-dependency-provisioning.md mermaid diagrams updated
- [ ] No system readline/ncurses dependencies used (verified by verify-no-system-deps.sh)

## Open Questions

None - all patterns are established and documented.
