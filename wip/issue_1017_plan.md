# Issue 1017 Implementation Plan

## Overview

Add environment sanitization and path validation to the dlopen helper invocation flow.

## Changes

### 1. Add `sanitizeEnvForHelper` function

Location: `internal/verify/dltest.go`

```go
// sanitizeEnvForHelper creates a safe environment for the helper process.
// It strips dangerous loader variables that could enable code injection.
func sanitizeEnvForHelper(tsukuHome string) []string {
    dangerous := map[string]bool{
        // Linux ld.so injection vectors
        "LD_PRELOAD": true, "LD_AUDIT": true, "LD_DEBUG": true,
        "LD_DEBUG_OUTPUT": true, "LD_PROFILE": true, "LD_PROFILE_OUTPUT": true,
        // macOS dyld injection vectors
        "DYLD_INSERT_LIBRARIES": true, "DYLD_FORCE_FLAT_NAMESPACE": true,
        "DYLD_PRINT_LIBRARIES": true, "DYLD_PRINT_LIBRARIES_POST_LAUNCH": true,
    }

    var env []string
    for _, e := range os.Environ() {
        key := strings.SplitN(e, "=", 2)[0]
        if !dangerous[key] {
            env = append(env, e)
        }
    }

    // Prepend tsuku libs to search paths
    libsDir := filepath.Join(tsukuHome, "libs")
    env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libsDir, os.Getenv("LD_LIBRARY_PATH")))
    env = append(env, fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s", libsDir, os.Getenv("DYLD_LIBRARY_PATH")))

    return env
}
```

### 2. Add `validateLibraryPaths` function

Location: `internal/verify/dltest.go`

```go
// validateLibraryPaths ensures all paths are within the allowed directory.
// It canonicalizes paths via EvalSymlinks before checking the prefix.
func validateLibraryPaths(paths []string, libsDir string) error {
    // Canonicalize the libs dir itself for consistent prefix checking
    canonicalLibsDir, err := filepath.EvalSymlinks(libsDir)
    if err != nil {
        // If libsDir doesn't exist yet, that's an error
        return fmt.Errorf("libs directory not accessible: %w", err)
    }
    // Ensure canonical path has trailing separator for prefix check
    if !strings.HasSuffix(canonicalLibsDir, string(filepath.Separator)) {
        canonicalLibsDir += string(filepath.Separator)
    }

    for _, p := range paths {
        canonical, err := filepath.EvalSymlinks(p)
        if err != nil {
            return fmt.Errorf("invalid library path %q: %w", p, err)
        }
        // Check if path is within libs directory
        if !strings.HasPrefix(canonical, canonicalLibsDir) && canonical != strings.TrimSuffix(canonicalLibsDir, string(filepath.Separator)) {
            return fmt.Errorf("library path %q is outside %s", p, libsDir)
        }
    }
    return nil
}
```

### 3. Modify function signatures

Update signatures to pass `tsukuHome`:

- `InvokeDltest(ctx, helperPath, paths, tsukuHome)`
- `invokeBatchWithRetry(ctx, helperPath, batch, tsukuHome)`
- `invokeBatch(ctx, helperPath, batch, tsukuHome)`

### 4. Integrate into invokeBatch

Add `cmd.Env = sanitizeEnvForHelper(tsukuHome)` after creating the command:

```go
func invokeBatch(ctx context.Context, helperPath string, batch []string, tsukuHome string) ([]DlopenResult, error) {
    batchCtx, cancel := context.WithTimeout(ctx, BatchTimeout)
    defer cancel()

    cmd := exec.CommandContext(batchCtx, helperPath, batch...)
    cmd.Env = sanitizeEnvForHelper(tsukuHome)  // NEW

    var stdout, stderr bytes.Buffer
    // ... rest unchanged
}
```

### 5. Add path validation to InvokeDltest

```go
func InvokeDltest(ctx context.Context, helperPath string, paths []string, tsukuHome string) ([]DlopenResult, error) {
    if len(paths) == 0 {
        return nil, nil
    }

    // Validate all paths before processing
    libsDir := filepath.Join(tsukuHome, "libs")
    if err := validateLibraryPaths(paths, libsDir); err != nil {
        return nil, err
    }

    batches := splitIntoBatches(paths, DefaultBatchSize)
    // ... rest unchanged but with tsukuHome passed through
}
```

### 6. Update tests

Update all existing tests to pass the new `tsukuHome` parameter. Add new tests:

1. `TestSanitizeEnvForHelper` - verify dangerous vars stripped
2. `TestSanitizeEnvForHelper_PreservesNormalVars`
3. `TestSanitizeEnvForHelper_AddsLibPaths`
4. `TestValidateLibraryPaths_ValidPath`
5. `TestValidateLibraryPaths_PathTraversal`
6. `TestValidateLibraryPaths_SymlinkEscape`
7. `TestValidateLibraryPaths_NonexistentPath`
8. `TestInvokeDltest_PathValidation_Rejects`

## Test Plan

1. Run existing tests (should fail until signatures updated)
2. Update existing tests with tsukuHome parameter
3. Add new unit tests for sanitization and validation
4. Run full test suite
5. Run go vet and golangci-lint
