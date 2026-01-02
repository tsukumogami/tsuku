# Issue 794 Introspection

## Context Reviewed
- Design doc: `docs/DESIGN-system-dependency-actions.md`
- Sibling issues reviewed: #793 (closed 2026-01-02)
- Prior patterns identified:
  - Test fixtures use `*-system.toml` naming convention
  - Tests live in `internal/executor/system_deps_test.go`
  - Tests use `FilterStepsByTarget()` for platform filtering
  - Tests validate both filtering and `Describe()` output
  - Tests use `testdata/recipes/` directory for fixtures

## Gap Analysis

### Minor Gaps

**Test location pattern established:**
- PR #796 created `internal/executor/system_deps_test.go` for M30 system dependency tests
- Issue #794 doesn't specify where integration tests should go
- Pattern: CLI integration tests live in `cmd/tsuku/` (see `install_test.go`, `install_deps_test.go`)
- Resolution: CLI instruction display tests should go in `cmd/tsuku/sysdeps_test.go` (new file) to test the CLI-layer functions

**Existing test fixtures available:**
- PR #796 created three testdata recipes: `build-tools-system.toml`, `ssl-libs-system.toml`, `ca-certs-system.toml`
- Issue #794 says "Tests use testdata fixtures (not production recipes)" - fixtures already exist
- Resolution: Reuse the existing `*-system.toml` fixtures created in #793

**CLI implementation details discovered:**
- Issue #794 acceptance criteria mention testing `tsuku install <sysdep-recipe>` displays instructions
- Implementation (from `install_deps.go` lines 343-356):
  - `displaySystemDeps()` function called when `isExplicit && !quietFlag && hasSystemDeps(r)`
  - Function returns `true` if deps were displayed, causing early exit from install flow
  - `resolveTarget()` handles `--target-family` override
- Resolution: Tests need to validate the `displaySystemDeps()` and `resolveTarget()` helper functions

**Output format details:**
- Issue #794 doesn't specify what the instruction output should look like
- Implementation (from `sysdeps.go` lines 83-168):
  - Groups steps into categories: package, config, verify, manual
  - Displays numbered steps
  - Shows verification commands separately
  - Ends with "After completing these steps, run the install command again."
- Resolution: Tests should verify numbered output format, category grouping, and verification section

**Flag behavior details:**
- Issue #794 mentions `--quiet` flag but doesn't specify interaction with system deps
- Implementation: `quietFlag` is checked before calling `displaySystemDeps()` (line 345)
- Resolution: Test that `--quiet` suppresses all system dependency output

### Moderate Gaps

None identified. All implementation patterns and conventions are established by prior work.

### Major Gaps

None identified. The issue spec is complete and implementable given the established patterns.

## Recommendation

**Proceed** - Issue is ready for implementation.

## Implementation Notes

Based on review of completed work, the implementation should:

1. **Create `cmd/tsuku/sysdeps_test.go`** following the CLI test pattern
   - Test `displaySystemDeps()` function directly (unit test style)
   - Test `resolveTarget()` with and without `--target-family` override
   - Test `getSystemDepsForTarget()` filtering logic
   - Test `describeSystemStep()` output generation

2. **Reuse existing testdata fixtures** from #793:
   - `build-tools-system.toml` - multi-platform package managers
   - `ssl-libs-system.toml` - platform-specific package naming
   - `ca-certs-system.toml` - fallback field handling

3. **Test matrix coverage**:
   - Per acceptance criteria:
     - Current platform display (requires mocking `platform.DetectTarget()`)
     - `--target-family debian` shows apt commands
     - `--target-family rhel` shows dnf commands
     - `--quiet` suppresses output
   - Additional coverage from implementation:
     - Category grouping (packages, config, verify)
     - Numbered step output
     - Family display names ("Ubuntu/Debian", "Fedora/RHEL/CentOS")
     - Verification section formatting

4. **Testing approach**:
   - **Unit tests** for helper functions (`displaySystemDeps`, `resolveTarget`, `describeSystemStep`)
   - **Table-driven tests** for different target families
   - **Output capture** using `bytes.Buffer` or similar to validate printed instructions
   - **No actual CLI execution** - test the functions directly, not via subprocess

5. **Pattern consistency**:
   - Follow existing test style in `cmd/tsuku/install_test.go`
   - Use table-driven test pattern
   - Clear test names describing what's being validated
   - Parallel test execution where safe (`t.Parallel()`)

## Prior Work Dependencies

Issue #794 was blocked by #793, which:
- Created testdata recipes exercising all 15 M30 action types
- Established the `*-system.toml` fixture naming pattern
- Created `internal/executor/system_deps_test.go` for executor-level tests
- Validated that `FilterStepsByTarget()` and `Describe()` work correctly

This unblocks #794 to focus purely on CLI integration tests without needing to create fixtures.
