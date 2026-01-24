---
summary:
  constraints:
    - Security-critical: must strip all loader injection variables (LD_PRELOAD, DYLD_INSERT_LIBRARIES, etc.)
    - Path validation must use filepath.EvalSymlinks() to resolve symlinks BEFORE prefix check
    - Fail closed: validation errors must be errors, not warnings
    - No TOCTOU race: validate paths immediately before exec
  integration_points:
    - internal/verify/dltest.go - add sanitizeEnvForHelper() and validatePaths()
    - invokeBatch() must use sanitized environment
    - InvokeDltest() or caller must validate paths before invocation
    - config.Config provides TsukuHome for path validation
  risks:
    - Missing an injection variable leaves a security hole
    - Validating before canonicalization allows path traversal
    - TOCTOU if paths validated too early in the call chain
  approach_notes: |
    1. Add sanitizeEnvForHelper(tsukuHome string) []string function
    2. Add validateLibraryPaths(paths []string, libsDir string) error function
    3. Integrate into invokeBatch - set cmd.Env to sanitized env
    4. Add path validation at InvokeDltest entry point
    5. Comprehensive tests for all dangerous env vars and path traversal scenarios
---

# Implementation Context: Issue #1017

**Source**: docs/designs/DESIGN-library-verify-dlopen.md

## Key Security Requirements (from design)

### Environment Sanitization

Strip these dangerous variables:
- **Linux**: `LD_PRELOAD`, `LD_AUDIT`, `LD_DEBUG`, `LD_DEBUG_OUTPUT`, `LD_PROFILE`, `LD_PROFILE_OUTPUT`
- **macOS**: `DYLD_INSERT_LIBRARIES`, `DYLD_FORCE_FLAT_NAMESPACE`, `DYLD_PRINT_LIBRARIES`, `DYLD_PRINT_LIBRARIES_POST_LAUNCH`

Preserve/add these:
- Prepend `$TSUKU_HOME/libs` to `LD_LIBRARY_PATH` and `DYLD_LIBRARY_PATH`

### Path Validation

```go
for _, p := range paths {
    canonical, err := filepath.EvalSymlinks(p)
    if err != nil || !strings.HasPrefix(canonical, filepath.Join(tsukuHome, "libs")) {
        return nil, fmt.Errorf("invalid library path: %s", p)
    }
}
```

Key: canonicalize THEN validate prefix (order matters for security).

## Existing Code Reference

The `invokeBatch` function in `internal/verify/dltest.go` already creates the command but doesn't set `cmd.Env`. Need to add:
- `cmd.Env = sanitizeEnvForHelper(tsukuHome)`
- Path validation before the batch loop
