# Recent PR Review: Library Verification

**Date:** 2026-01-17
**Reviewer:** Agent
**Focus:** PRs merged in last week related to library verification and their implications for Tier 2 design

## PRs Reviewed

### PR #963: feat(install): compute and store library checksums at install time
**Merged:** 2026-01-17T12:44:30Z

**Summary:**
- Added `ComputeLibraryChecksums()` function that walks library directories and computes SHA256 checksums for all regular files (skipping symlinks)
- Added `VerifyLibraryChecksums()` function for integrity verification
- Integrated checksum computation into `InstallLibrary()` flow
- Added `--integrity` flag to `tsuku verify` command for library verification
- Files changed: `internal/install/checksum.go`, `cmd/tsuku/verify.go`, `internal/install/library.go`

**Key Technical Details:**
- Uses `filepath.Walk` to traverse library directories
- Symlinks are explicitly skipped (uses `os.Lstat` to detect symlinks)
- Checksums stored as `map[string]string` keyed by relative path
- Checksum format: SHA256 hex-encoded

**Implications for Tier 2:**
1. The checksum infrastructure could be leveraged for dependency file existence checks
2. The symlink handling pattern (following symlinks to real files only) should be consistent with Tier 2's dependency resolution

---

### PR #962: feat(verify): add header validation for library verification (Tier 1)
**Merged:** 2026-01-17T01:25:14Z

**Summary:**
- Implemented complete Tier 1 header validation module
- Created `internal/verify/types.go` with `HeaderInfo`, `ValidationError`, `ErrorCategory`
- Created `internal/verify/header.go` with `ValidateHeader()` function
- Added support for ELF (Linux) and Mach-O (macOS) parsing
- Added fat/universal binary support with architecture extraction
- Added static library (.a archive) detection with clear error message

**Key Technical Details:**
- `HeaderInfo.Dependencies []string` already extracts `DT_NEEDED` (ELF) and `LC_LOAD_DYLIB` (Mach-O) entries
- Six error categories defined: `ErrUnreadable`, `ErrInvalidFormat`, `ErrNotSharedLib`, `ErrWrongArch`, `ErrTruncated`, `ErrCorrupted`
- Panic recovery implemented for robustness against malicious input
- Early magic detection (8-byte read) for efficient format dispatch

**Implications for Tier 2:**
1. **Critical:** Dependencies are already extracted by Tier 1 - Tier 2 design should reference `HeaderInfo.Dependencies` directly
2. The design's proposed `ValidateDependencies(libPath string, deps []string, libsDir string)` signature aligns with this
3. Error categories should be extended with `ErrDepMissing` and `ErrDepInvalid` as proposed

---

### PR #959: feat(verify): add library type detection and flag routing
**Merged:** 2026-01-16T23:46:54Z

**Summary:**
- Added library detection in verify command via `Recipe.IsLibrary()`
- Added `--integrity` and `--skip-dlopen` flags for library verification
- Created `LibraryVerifyOptions` struct for passing verification options
- Implemented routing to `verifyLibrary()` function for library recipes

**Key Technical Details:**
- Libraries use `state.Libs` for lookup (not `state.Installed`)
- Library version iteration: `for v, ls := range libVersions` (typically one version)
- Library directory path: `cfg.LibDir(name, version)`
- Added `findLibraryFiles()` function that walks directories looking for `.so` and `.dylib` files

**Implications for Tier 2:**
1. The `verifyLibrary()` function in `cmd/tsuku/verify.go` is the integration point
2. Current code logs "Tier 2: Dependency checking (not yet implemented)" - this is where `ValidateDependencies()` should be called
3. The `findLibraryFiles()` function already identifies shared library files - Tier 2 should iterate these for dependency validation

---

### PR #958: feat(state): add checksums field to library version state
**Merged:** 2026-01-16T23:30:40Z

**Summary:**
- Added `Checksums map[string]string` field to `LibraryVersionState`
- Used `json:"checksums,omitempty"` tag for backward compatibility

**Implications for Tier 2:**
- The state structure is established; no changes needed for Tier 2

---

### PR #954: docs(design): add implementation plan for library verification
**Merged:** 2026-01-16T21:01:05Z

**Summary:**
- Updated DESIGN-library-verification.md status from Proposed to Planned
- Added implementation issues table and Mermaid dependency diagram
- Created issues #942-#950 for the implementation roadmap

**Implications for Tier 2:**
- Issue #948 is the design issue for Tier 2 dependency resolution
- Dependencies: #948 depends on #947 (Tier 1) which is now complete

---

### PR #937: docs: add design for library verification
**Merged:** 2026-01-16T20:54:27Z

**Summary:**
- Created comprehensive DESIGN-library-verification.md
- Established four-tier verification system
- Defined architecture and component changes

**Implications for Tier 2:**
- This is the umbrella design that Tier 2 implements

---

## Summary of Current Implementation State

| Component | Status | Location |
|-----------|--------|----------|
| HeaderInfo with Dependencies | Implemented | `internal/verify/types.go` |
| ValidateHeader() | Implemented | `internal/verify/header.go` |
| Dependency extraction | Implemented | Lines 141 (ELF), 199 (Mach-O) in header.go |
| ErrorCategory | Implemented | `internal/verify/types.go` (6 categories) |
| verifyLibrary() integration | Implemented (Tier 1 only) | `cmd/tsuku/verify.go` |
| ValidateDependencies() | Not implemented | Proposed in DESIGN-library-verify-deps.md |
| System library patterns | Not implemented | Referenced in shell script |

---

## Proposed Changes to Tier 2 Design

Based on the recent PRs, the following modifications should be made to `docs/designs/DESIGN-library-verify-deps.md`:

### 1. Update Implementation Context to Reference Implemented Code

**Current (outdated):**
```markdown
**Dependency extraction (Tier 1):**
The header validation module (`internal/verify/header.go`) already extracts dependencies using Go's standard library:
- ELF: `f.ImportedLibraries()` returns `DT_NEEDED` entries (line 141)
- Mach-O: `f.ImportedLibraries()` returns `LC_LOAD_DYLIB` entries (line 199)
- Dependencies stored in `HeaderInfo.Dependencies []string`
```

**Proposed update:**
Add note that this is now implemented and tested:
```markdown
**Dependency extraction (Tier 1) - IMPLEMENTED in PR #962:**
The header validation module (`internal/verify/header.go`) extracts dependencies:
- ELF: `f.ImportedLibraries()` returns `DT_NEEDED` entries (header.go line 141)
- Mach-O: `f.ImportedLibraries()` returns `LC_LOAD_DYLIB` entries (header.go line 199)
- Dependencies stored in `HeaderInfo.Dependencies []string` (types.go)
- Error categorization via `ValidationError` with 6 categories (types.go)
```

### 2. Update Data Flow Section

**Current:**
```markdown
1. `verify.go` calls `ValidateHeader(libPath)` -> returns `HeaderInfo` with `Dependencies []string`
```

**Proposed update (reflect actual implementation):**
```markdown
1. `verifyLibrary()` in `cmd/tsuku/verify.go` iterates library files via `findLibraryFiles(libDir)`
2. For each file, calls `verify.ValidateHeader(libFile)` -> returns `HeaderInfo` with `Dependencies []string`
3. For each library with dependencies, call `ValidateDependencies(libFile, info.Dependencies, libsDir)`
```

### 3. Update Integration Point

**Add new section showing exact integration point:**

```markdown
### Integration Point in verify.go

The `verifyLibrary()` function (cmd/tsuku/verify.go lines 234-312) currently implements Tier 1 and logs placeholders for Tier 2/3:

```go
// Current code at line 307-308:
printInfo("  Tier 2 (deps): not yet implemented\n")
if !opts.SkipDlopen {
    printInfo("  Tier 3 (dlopen): not yet implemented\n")
}
```

**Tier 2 should be integrated as:**

```go
// Tier 2: Dependency validation
printInfo("  Tier 2: Dependency validation...\n")
for _, libFile := range libFiles {
    relPath, _ := filepath.Rel(libDir, libFile)
    info, _ := verify.ValidateHeader(libFile)
    if len(info.Dependencies) > 0 {
        results := verify.ValidateDependencies(libFile, info.Dependencies, cfg.LibsDir)
        // Report results...
    }
}
```

### 4. Add RPATH Extraction Details

**Current design proposes but doesn't detail RPATH extraction.**

**Proposed addition after line 386 (ExtractRPaths interface):**

```markdown
### RPATH Extraction Implementation

For ELF binaries, RPATH values are extracted using Go's `debug/elf` package:

```go
func extractELFRPaths(path string) ([]string, error) {
    f, err := elf.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    // Try DT_RUNPATH first (preferred), then DT_RPATH
    runpath, _ := f.DynString(elf.DT_RUNPATH)
    if len(runpath) > 0 {
        return strings.Split(runpath[0], ":"), nil
    }

    rpath, _ := f.DynString(elf.DT_RPATH)
    if len(rpath) > 0 {
        return strings.Split(rpath[0], ":"), nil
    }

    return nil, nil // No RPATH set
}
```

For Mach-O binaries, LC_RPATH is extracted via load command iteration:

```go
func extractMachORPaths(path string) ([]string, error) {
    f, err := macho.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var rpaths []string
    for _, load := range f.Loads {
        if rpath, ok := load.(*macho.Rpath); ok {
            rpaths = append(rpaths, rpath.Path)
        }
    }
    return rpaths, nil
}
```

Note: Go's `debug/macho` package provides the `Rpath` type since Go 1.16.
```

### 5. Update System Library Patterns to Match Shell Script

**The shell script `test/scripts/verify-no-system-deps.sh` is referenced but patterns may differ.**

**Proposed update to linuxSystemPatterns (line 338-353):**

Add explicit alignment note and ensure patterns match:
```go
// Linux system library patterns (glibc + musl)
// Aligned with test/scripts/verify-no-system-deps.sh
var linuxSystemPatterns = []string{
    "linux-vdso.so",       // Virtual DSO (kernel-provided)
    "linux-gate.so",       // 32-bit virtual DSO
    "ld-linux",            // Dynamic linker (glibc)
    "ld-musl",             // Dynamic linker (musl/Alpine)
    "libc.so",             // C library
    "libm.so",             // Math library
    "libdl.so",            // Dynamic loading (glibc)
    "libpthread.so",       // POSIX threads (glibc)
    "librt.so",            // Realtime extensions
    "libresolv.so",        // DNS resolver
    "libnsl.so",           // Network services library
    "libcrypt.so",         // Cryptographic library
    "libutil.so",          // Utility library
    "libgcc_s.so",         // GCC runtime
    "libstdc++.so",        // C++ standard library
    "libatomic.so",        // Atomic operations library (GCC)
}
```

### 6. Add Symlink Handling Consistency Note

**PR #963 established symlink handling for checksums. Tier 2 should be consistent.**

**Proposed addition to Implementation Approach section:**

```markdown
### Symlink Handling

Tier 2 dependency validation should follow the same symlink handling pattern established for checksum computation (PR #963):

1. When finding library files to validate, use `filepath.EvalSymlinks()` to resolve to real files
2. Only validate the real file, not each symlink pointing to it
3. When resolving dependency paths, follow symlinks to actual files

This is consistent with `findLibraryFiles()` in `cmd/tsuku/verify.go` which already implements this pattern:
```go
realPath, err := filepath.EvalSymlinks(path)
if err != nil {
    return nil // Skip broken symlinks
}
if realPath == path {
    files = append(files, path)
}
```
```

### 7. Update Error Categories to Use Explicit Constants

**Current design proposes new error categories starting at iota+100:**

```go
const (
    ErrDepMissing   ErrorCategory = iota + 100 // Dependency file not found
    ErrDepInvalid                              // Dependency exists but invalid
)
```

**Proposed change:**

Using `iota + 100` is fragile if new categories are added to Tier 1. Instead, use explicit values:

```go
const (
    // Tier 1 error categories (defined in types.go)
    // ErrUnreadable     = 0
    // ErrInvalidFormat  = 1
    // ErrNotSharedLib   = 2
    // ErrWrongArch      = 3
    // ErrTruncated      = 4
    // ErrCorrupted      = 5

    // Tier 2 error categories (dependency validation)
    ErrDepMissing  ErrorCategory = 10 // Dependency file not found
    ErrDepInvalid  ErrorCategory = 11 // Dependency exists but Tier 1 validation failed
    ErrDepWarning  ErrorCategory = 12 // Unknown absolute path outside tsuku
)
```

### 8. Add Test Data Considerations

**Proposed addition to Step 6 (Add unit tests):**

```markdown
### Test Data for Dependency Validation

Unit tests should cover:

1. **ELF with tsuku-managed dependencies:**
   - Create minimal ELF with DT_NEEDED pointing to `$ORIGIN/../lib/libtest.so`
   - Verify resolution against mock tsuku libs directory

2. **Mach-O with @rpath dependencies:**
   - Create minimal Mach-O with LC_LOAD_DYLIB using `@rpath/libtest.dylib`
   - Add LC_RPATH entries and verify resolution

3. **System library pattern matching:**
   - Test all patterns in linuxSystemPatterns and darwinSystemPatterns
   - Verify versioned libraries match (libc.so.6, libstdc++.so.6.0.33)

4. **Mixed dependencies:**
   - Library with both system and tsuku-managed dependencies
   - Verify system deps are skipped, tsuku deps are validated

The existing `testdata/` directory structure can be extended:
```
testdata/
  verify/
    deps/
      elf-with-origin/
        lib/
          libtest.so          # ELF with DT_NEEDED: $ORIGIN/libdep.so
          libdep.so           # Dependency
      macho-with-rpath/
        lib/
          libtest.dylib       # Mach-O with @rpath/libdep.dylib
          libdep.dylib        # Dependency
```
```

### 9. Update Status to Reflect Tier 1 Completion

**Current status:** Proposed

**Proposed status update:**

```markdown
**Status:** Accepted

**Prerequisite Completion:** Tier 1 header validation is fully implemented (PR #962, #965). The `HeaderInfo.Dependencies` field provides the dependency list needed for Tier 2 validation.
```

---

## Implementation Priority Recommendations

Based on the PR review, the following implementation order is recommended:

1. **Add error categories to types.go** (Step 1)
   - Low risk, foundational change
   - Use explicit constants (10, 11, 12) not iota+100

2. **Implement classifyDep() with system patterns** (Step 2)
   - Align patterns with verify-no-system-deps.sh
   - Add unit tests for pattern matching

3. **Implement expandPathVariables() for $ORIGIN/@rpath** (Step 3)
   - Add RPATH extraction using debug/elf and debug/macho
   - Test with real libraries from tsuku installations

4. **Wire up ValidateDependencies() in verifyLibrary()** (Step 5)
   - Replace placeholder log message with actual call
   - Integrate with existing Tier 1 validation flow

5. **Add integration tests** (Step 7)
   - Test with real installed libraries (gcc-libs, zlib)
   - Add CI job for Tier 2 validation

---

## Conclusion

The recent PRs have established a solid foundation for Tier 2:

- **HeaderInfo.Dependencies** is implemented and tested
- **Error categorization** framework exists and is extensible
- **verify.go integration point** is clearly defined
- **Symlink handling patterns** are established

The Tier 2 design in `DESIGN-library-verify-deps.md` is largely correct but needs updates to:
1. Reference implemented code locations
2. Add RPATH extraction implementation details
3. Use explicit error category constants
4. Add symlink handling consistency notes
5. Update status to Accepted
