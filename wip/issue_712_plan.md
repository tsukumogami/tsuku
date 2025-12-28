# Issue 712 Implementation Plan

## Problem Statement

The `tsuku eval` command supports generating installation plans for non-native platforms via `--os` and `--arch` flags. However, the platform support check at `cmd/tsuku/eval.go:161-164` uses `SupportsPlatformRuntime()` which validates against `runtime.GOOS`/`runtime.GOARCH` instead of the target platform specified by the user.

This causes `tsuku eval --os darwin --arch arm64` to fail on Linux if the recipe only supports darwin, even though the target platform (darwin/arm64) is valid.

## Current Behavior

```go
// cmd/tsuku/eval.go:160-164
// Check platform support before installation
if !r.SupportsPlatformRuntime() {
    printError(r.NewUnsupportedPlatformError())
    exitWithCode(ExitGeneral)
}
```

`SupportsPlatformRuntime()` is defined as:
```go
// internal/recipe/platform.go:115-117
func (r *Recipe) SupportsPlatformRuntime() bool {
    return r.SupportsPlatform(runtime.GOOS, runtime.GOARCH)
}
```

And `NewUnsupportedPlatformError()` also hardcodes runtime values:
```go
// internal/recipe/platform.go:119-129
func (r *Recipe) NewUnsupportedPlatformError() *UnsupportedPlatformError {
    return &UnsupportedPlatformError{
        RecipeName:           r.Metadata.Name,
        CurrentOS:            runtime.GOOS,      // Should be target OS
        CurrentArch:          runtime.GOARCH,    // Should be target arch
        // ...
    }
}
```

## Design Alternatives

### Alternative A: Inline target platform resolution in eval.go (CHOSEN)

Change the platform check in `eval.go` to use the target platform directly:

```go
// Resolve target platform (use flags or fall back to runtime)
targetOS := evalOS
if targetOS == "" {
    targetOS = runtime.GOOS
}
targetArch := evalArch
if targetArch == "" {
    targetArch = runtime.GOARCH
}

// Check platform support for target platform
if !r.SupportsPlatform(targetOS, targetArch) {
    printError(&recipe.UnsupportedPlatformError{
        RecipeName:           r.Metadata.Name,
        CurrentOS:            targetOS,
        CurrentArch:          targetArch,
        SupportedOS:          r.Metadata.SupportedOS,
        SupportedArch:        r.Metadata.SupportedArch,
        UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
    })
    exitWithCode(ExitGeneral)
}
```

**Pros:**
- Single file change
- Explicit control over error message content
- No API changes to recipe package

**Cons:**
- Manually constructs error struct instead of using helper method

### Alternative B: Add NewUnsupportedPlatformErrorForPlatform method

Add a new method to `platform.go`:

```go
func (r *Recipe) NewUnsupportedPlatformErrorForPlatform(os, arch string) *UnsupportedPlatformError {
    return &UnsupportedPlatformError{
        RecipeName:           r.Metadata.Name,
        CurrentOS:            os,
        CurrentArch:          arch,
        SupportedOS:          r.Metadata.SupportedOS,
        SupportedArch:        r.Metadata.SupportedArch,
        UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
    }
}
```

**Pros:**
- Cleaner API for callers that need custom platform
- Existing `NewUnsupportedPlatformError()` could delegate to it

**Cons:**
- Adds API surface to recipe package
- Only one caller needs this flexibility currently

### Decision: Alternative A

The inline approach is simpler for this isolated fix. The explicit struct construction is clear and matches the existing pattern for how `UnsupportedPlatformError` is tested (see `platform_test.go:412-493`). If more callers need platform-parameterized errors in the future, we can refactor to Alternative B.

## Implementation Steps

### Step 1: Update platform check in eval.go

**File:** `cmd/tsuku/eval.go`

**Change:** Replace lines 160-164 with target platform resolution and validation.

**Before:**
```go
// Check platform support before installation
if !r.SupportsPlatformRuntime() {
    printError(r.NewUnsupportedPlatformError())
    exitWithCode(ExitGeneral)
}
```

**After:**
```go
// Resolve target platform (use flags or fall back to runtime)
targetOS := evalOS
if targetOS == "" {
    targetOS = runtime.GOOS
}
targetArch := evalArch
if targetArch == "" {
    targetArch = runtime.GOARCH
}

// Check platform support for target platform
if !r.SupportsPlatform(targetOS, targetArch) {
    printError(&recipe.UnsupportedPlatformError{
        RecipeName:           r.Metadata.Name,
        CurrentOS:            targetOS,
        CurrentArch:          targetArch,
        SupportedOS:          r.Metadata.SupportedOS,
        SupportedArch:        r.Metadata.SupportedArch,
        UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
    })
    exitWithCode(ExitGeneral)
}
```

**Import needed:** Add `"runtime"` to imports (already present).

### Step 2: Verify and run tests

1. Run unit tests: `go test ./cmd/tsuku/... ./internal/recipe/...`
2. Run full test suite: `go test ./...`
3. Build: `go build -o tsuku ./cmd/tsuku`

### Step 3: Manual verification

Test scenarios:
1. **Cross-platform success:** Run eval for darwin-only recipe from linux with `--os darwin`
2. **Same-platform success:** Run eval without flags (should use runtime platform)
3. **Cross-platform failure:** Run eval for linux-only recipe from linux with `--os darwin` (should fail with correct error message)
4. **Error message validation:** Verify error shows target platform, not runtime platform

## Files Modified

| File | Change Type | Description |
|------|-------------|-------------|
| `cmd/tsuku/eval.go` | Modify | Replace `SupportsPlatformRuntime()` with `SupportsPlatform(targetOS, targetArch)` and construct error with target platform |

**Total: 1 file modified**

## Testing Strategy

### Existing Tests

The existing tests in `cmd/tsuku/eval_test.go` test `ValidateOS` and `ValidateArch` functions. These do not need modification as they test flag validation, not platform support checking.

The platform support logic is thoroughly tested in `internal/recipe/platform_test.go` with:
- `TestSupportsPlatform` - 20 test cases covering all constraint combinations
- `TestUnsupportedPlatformError` - 3 test cases verifying error message format

### Manual Testing

Since the change involves command-line behavior with platform flags, manual testing is appropriate:

```bash
# Build
go build -o tsuku ./cmd/tsuku

# Create a linux-only test recipe
cat > /tmp/linux-only.toml << 'EOF'
[metadata]
name = "linux-only-test"
description = "Test recipe that only supports linux"
version_command = "echo 1.0.0"
supported_os = ["linux"]

[[steps]]
action = "download"
[steps.params]
url = "https://example.com/test-${os}-${arch}.tar.gz"
EOF

# Test 1: Should succeed on linux with --os linux
./tsuku eval --recipe /tmp/linux-only.toml --os linux --arch amd64

# Test 2: Should fail with target platform (darwin) in error message
./tsuku eval --recipe /tmp/linux-only.toml --os darwin --arch arm64

# Test 3: Default behavior (no flags) should use runtime platform
./tsuku eval --recipe /tmp/linux-only.toml
```

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing eval behavior | Low | High | Existing tests verify flag validation; manual testing confirms runtime fallback works |
| Error message format change | Low | Low | `UnsupportedPlatformError` struct unchanged; only values passed differ |

## Acceptance Criteria Verification

- [x] `tsuku eval --os <os> --arch <arch>` validates platform support against target platform
  - Implementation uses `r.SupportsPlatform(targetOS, targetArch)` with resolved target values
- [x] Cross-platform plan generation works when recipe supports target but not runtime platform
  - Target platform is resolved before check; runtime platform only used as fallback when flags empty
- [x] Error message indicates unsupported target platform when appropriate
  - Error struct populated with `targetOS` and `targetArch` instead of `runtime.GOOS/GOARCH`
- [x] Existing tests pass
  - No test changes required; existing coverage validates component behavior

## Commit Message

```
fix(eval): validate platform support against target platform

The eval command's --os and --arch flags allow generating installation
plans for non-native platforms. However, the platform support check
incorrectly validated against runtime.GOOS/GOARCH instead of the
target platform, causing valid cross-platform evaluations to fail.

Change the check to resolve target platform from flags (with runtime
fallback) and validate against that, with error messages reflecting
the actual target platform being evaluated.

Fixes #712
```
