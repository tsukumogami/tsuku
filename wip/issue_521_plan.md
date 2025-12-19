# Issue 521 Implementation Plan

## Summary
Implement a `meson_build` action following the same pattern as `cmake_build` and `configure_make`, supporting the three-phase Meson workflow: setup → compile → install.

## Approach
The implementation will closely mirror the `cmake_build` action structure, as both CMake and Meson are modern build systems with similar workflows. Key similarities:
- Both create isolated build directories
- Both use three-phase builds (configure/setup, build/compile, install)
- Both require compiler environment setup
- Both need security validation of arguments

### Alternatives Considered
1. **Simpler shell-based approach**: Use `run_command` action directly - Rejected because we need proper parameter validation, executable verification, and consistent error handling that matches other build system actions.
2. **Combine with configure_make**: Create a generic "build_system" action - Rejected because each build system has unique characteristics (Meson's `-D` options vs configure's `--` flags, different file markers, etc.).

## Files to Modify
- `internal/actions/action.go` - Register `MesonBuildAction` in init function (line ~150)
- `internal/actions/decomposable.go` - Add "meson_build" to primitives map (line ~92)

## Files to Create
- `internal/actions/meson_build.go` - Main implementation
- `internal/actions/meson_build_test.go` - Unit tests

## Implementation Steps
- [x] Create `internal/actions/meson_build.go` with MesonBuildAction struct and Execute method
- [x] Implement parameter validation (source_dir, executables, meson_args, buildtype, wrap_mode)
- [x] Implement security validation for meson_args (block shell metacharacters)
- [x] Implement three-phase build: meson setup, meson compile, meson install
- [x] Implement executable verification after install
- [x] Create `internal/actions/meson_build_test.go` with all unit tests from issue requirements
- [x] Register action in `internal/actions/action.go` init function
- [x] Add "meson_build" to primitives map in `internal/actions/decomposable.go`
- [ ] Add integration test entries to test-matrix.json for json-glib (simple Meson project)
- [ ] Create recipe for json-glib in recipes/ directory
- [ ] Run full test suite and verify all tests pass
- [ ] Run integration tests to verify real-world builds work

## Testing Strategy

### Unit Tests (meson_build_test.go)
All test cases from issue 521:
- MB-1: Basic meson project with default options
- MB-2: Custom meson_args with `-Dfeature=enabled`
- MB-3: Invalid meson_args with shell metacharacters (should fail)
- MB-4: Missing meson.build file (should fail)
- MB-5: Invalid buildtype value (should fail)
- MB-6: Build succeeds but executable not found (should fail)
- MB-7: Multiple executables in single build
- MB-8: Custom wrap_mode parameter
- Plus: Name(), Registered(), NotDeterministic(), and validation helper tests

### Integration Tests
Add to test-matrix.json:
- `meson_json-glib_simple`: Simple Meson project with minimal dependencies
  - Tool: json-glib
  - Tier: 5 (ecosystem primitive)
  - Features: ["action:meson_build", "build_system:meson"]

### Manual Verification
After implementation:
1. Build json-glib using meson_build action
2. Verify executable is installed and functional
3. Test with different meson_args to ensure flexibility

## Risks and Mitigations

### Risk 1: Meson not available in test environment
**Mitigation**: Integration tests will run in Docker with Meson pre-installed. Unit tests use testdata fixtures and don't require actual Meson binary.

### Risk 2: Build directory conflicts with source directory
**Mitigation**: Follow cmake_build pattern of creating isolated `build/` directory under WorkDir, separate from source.

### Risk 3: Meson-specific options not properly validated
**Mitigation**: Use same security validation pattern as cmake_build, blocking shell metacharacters. Validate buildtype against known values (release, debug, plain, debugoptimized).

## Success Criteria
- [x] All unit tests pass (go test ./internal/actions)
- [ ] Integration test builds json-glib successfully
- [x] meson_build action is registered as primitive
- [x] meson_build action returns IsDeterministic() = false
- [x] Security validation blocks malicious meson_args
- [x] Executables are verified after installation
- [x] No regressions in existing tests

## Open Questions
None - implementation path is clear based on existing cmake_build and configure_make patterns.
