# Dependency Resolution for Library Verification (Tier 2)

**Status:** Proposed

**Upstream Design Reference:** This design implements Tier 2 of [DESIGN-library-verification.md](./DESIGN-library-verification.md).

## Context and Problem Statement

Tsuku's `verify` command performs post-installation validation to ensure libraries were installed correctly. Tier 1 (header validation) confirms that shared library files have valid ELF or Mach-O headers. However, a library can have valid headers yet still fail at runtime if its dependencies are missing or unresolvable.

When tsuku installs a library like `libyaml`, that library may link against other libraries (e.g., `libc.so.6`). If a required dependency is missing, tools that depend on the library will fail at runtime with errors like "cannot open shared object file" or "library not loaded". Without dependency validation, `tsuku verify` cannot detect these issues proactively.

The challenge is distinguishing between:
1. **System libraries** (e.g., `libc`, `libm`, `libpthread`) - provided by the OS, expected to exist
2. **Tsuku-managed libraries** - installed to `$TSUKU_HOME/libs/`, must be validated
3. **Missing dependencies** - neither system nor tsuku-provided, indicates installation failure

Additionally, library paths use platform-specific conventions:
- Linux: `$ORIGIN`, `$LIB`, `ld.so.conf` search paths, multiarch directories
- macOS: `@rpath`, `@loader_path`, `@executable_path`, dyld shared cache

This matters now because:
- Tier 1 header validation is complete (#947), establishing the verification framework
- Users need confidence that verified libraries will work at runtime
- Complex tool chains (Ruby, Python with native extensions) depend on library verification

### Scope

**In scope:**
- Extracting dependency lists from ELF (`DT_NEEDED`) and Mach-O (`LC_LOAD_DYLIB`) binaries
- Resolving path variables (`$ORIGIN`, `@rpath`, `@loader_path`)
- Identifying system libraries (skip verification)
- Verifying tsuku-managed dependencies exist in `$TSUKU_HOME/libs/`
- Handling macOS dyld shared cache (system libraries not on disk)

**Out of scope:**
- Symbol-level verification (Tier 3: dlopen test)
- Checksum/integrity verification (Tier 4)
- Recursive dependency resolution (only direct dependencies)
- Version compatibility checking (e.g., SONAME version matching)
- Runtime-loaded dependencies (dlopen) - only statically-linked dependencies are visible in headers
- Weak/optional dependencies - these don't cause failures if missing

### Verification Outcome Levels

Tier 2 verification produces one of three outcomes per dependency:

| Outcome | Meaning | User Action |
|---------|---------|-------------|
| **Pass** | System library (skipped) or tsuku-managed library exists | None |
| **Warning** | Unknown absolute path outside tsuku (may be build artifact) | Review if intentional |
| **Failure** | Tsuku-managed dependency missing | Re-install library |

## Decision Drivers

- **Cross-platform**: Must work on Linux (ELF) and macOS (Mach-O)
- **No false positives**: System libraries must not trigger verification failures
- **No external tools**: Use Go's `debug/elf` and `debug/macho` packages, no shelling out
- **Consistent with Tier 1**: Reuse patterns from header validation (data structures, error categories)
- **Performance**: Dependency extraction should be fast (no recursive resolution)
- **Maintainability**: System library lists should be easy to update as OS versions change

## Implementation Context

### Existing Patterns

**Dependency extraction (Tier 1):**
The header validation module (`internal/verify/header.go`) already extracts dependencies using Go's standard library:
- ELF: `f.ImportedLibraries()` returns `DT_NEEDED` entries (line 141)
- Mach-O: `f.ImportedLibraries()` returns `LC_LOAD_DYLIB` entries (line 199)
- Dependencies stored in `HeaderInfo.Dependencies []string`

**System library lists (shell script):**
`test/scripts/verify-no-system-deps.sh` defines allowed system libraries:
- Linux: `linux-vdso`, `ld-linux`, `libc.so`, `libm.so`, `libdl.so`, `libpthread.so`, `librt.so`, `libresolv.so`, `libgcc_s.so`, `libstdc++.so`, paths under `/lib64/`, `/lib/x86_64-linux-gnu/`, `/lib/aarch64-linux-gnu/`
- macOS: paths starting with `/usr/lib/lib`, `/System/Library/`, or `@rpath`, `@loader_path`, `@executable_path`

**RPATH handling (actions):**
`internal/actions/set_rpath.go` shows patterns for:
- Binary format detection (`detectBinaryFormat`)
- Path variable validation (`validateRpath`)
- Valid relative path prefixes: `$ORIGIN`, `@executable_path`, `@loader_path`, `@rpath`

**Data structures:**
Tier 1 established `ValidationError` with `ErrorCategory` for categorized failures. Tier 2 should add new categories for dependency errors.

### Applicable Specifications

**ELF Specification:**
- `DT_NEEDED` entries in `.dynamic` section list required libraries
- `DT_RPATH` and `DT_RUNPATH` define search paths (Go's `debug/elf` provides `DynString(elf.DT_RPATH)`)
- `$ORIGIN` expands to directory containing the binary

**Mach-O Specification:**
- `LC_LOAD_DYLIB` load commands list required libraries
- `LC_RPATH` load commands define `@rpath` search paths
- `@loader_path` expands to directory containing the loading binary
- `@executable_path` expands to main executable's directory

**macOS dyld Shared Cache:**
- Since macOS 11 Big Sur, many system libraries are in a shared cache at `/System/Library/dyld/`
- Individual `.dylib` files may not exist on disk
- Libraries like `/usr/lib/libSystem.B.dylib` are virtualized

## Considered Options

### Option 1: Pattern-Based System Library Detection

Classify dependencies as "system" or "tsuku-managed" using pattern matching on library names and paths. System libraries are skipped; tsuku-managed libraries are verified to exist.

**Approach:**
- Define lists of system library patterns (names like `libc.so*`, paths like `/usr/lib/*`)
- Match each dependency against patterns
- If matched: skip (system library)
- If not matched and starts with `$TSUKU_HOME`: verify file exists
- If not matched and absolute path: report as unexpected dependency

**Pros:**
- Simple to implement and understand
- Fast (no file I/O for system library checks)
- Works even when system libraries aren't on disk (macOS dyld cache)
- Pattern lists can be maintained and extended easily
- Already have reference implementation in `verify-no-system-deps.sh`

**Cons:**
- Pattern lists may become stale as OS versions evolve
- May miss edge cases (uncommon system libraries not in pattern list)
- Different Linux distributions have different library layouts
- Requires periodic updates to pattern lists

### Option 2: Pure Path Resolution (Why It Doesn't Work)

This option is included for completeness to explain why pure file existence checking is insufficient.

**Approach:**
- Extract RPATH/RUNPATH from the binary
- For each dependency, resolve variables and search RPATH
- Check if resolved path exists on disk
- Report missing files as errors

**Why this fails:**
- **macOS dyld cache**: On macOS 11+, system libraries like `/usr/lib/libSystem.B.dylib` don't exist as files on disk - they're in a shared cache. Pure resolution reports them as "missing."
- **Cross-distribution variance**: `/lib/x86_64-linux-gnu/libc.so.6` vs `/lib64/libc.so.6` varies by distribution, making "check if file exists" unreliable.

This establishes why some form of pattern-based system library detection is required for cross-platform support.

### Option 3: Hybrid Approach (Pattern + RPATH-Aware Resolution)

Combine pattern-based detection for known system libraries with RPATH-aware resolution for tsuku-managed dependencies.

**Approach:**

```
For each dependency in HeaderInfo.Dependencies:
  1. Classification Phase:
     - If matches system library pattern → PASS (skip)
     - If absolute path starts with /usr/lib or /System/Library → PASS (system)
     - If contains @rpath, @loader_path, $ORIGIN → go to resolution phase
     - If absolute path outside tsuku → WARNING (unknown)
     - If relative path referencing $TSUKU_HOME/libs → go to resolution phase

  2. Resolution Phase (tsuku-managed only):
     - Expand path variables ($ORIGIN → directory of library being verified)
     - Check file exists
     - Optionally: run Tier 1 header validation on resolved file
     - If missing → FAILURE
     - If exists → PASS
```

**Pros:**
- Handles macOS dyld cache correctly (pattern match skips them)
- Verifies tsuku-managed libraries actually exist
- RPATH-aware resolution is principled and testable
- Validates tsuku dependencies with header check, not just existence
- Balanced accuracy and performance

**Cons:**
- More complex implementation (two-phase)
- Still requires pattern list maintenance
- RPATH extraction adds parsing complexity
- Testing requires fixtures for various path variable combinations

### Evaluation Against Decision Drivers

| Driver | Option 1: Pattern-Only | Option 3: Hybrid |
|--------|------------------------|------------------|
| Cross-platform | Good | Good |
| No false positives | Fair (pattern coverage) | Good |
| No external tools | Good | Good |
| Consistent with Tier 1 | Good | Good (reuses HeaderInfo) |
| Performance | Good | Good |
| Maintainability | Fair (list updates) | Fair (list updates) |

Note: Option 2 (Pure Resolution) is non-viable due to macOS dyld cache and is excluded from comparison.

### Uncertainties

- We believe the pattern lists from `verify-no-system-deps.sh` cover most cases, but haven't exhaustively tested across all supported distributions
- Performance impact of file existence checks on slow filesystems (NFS, etc.) is unknown
- The exact behavior of `@rpath` resolution when multiple LC_RPATH entries exist needs verification
- Alpine Linux uses musl libc which has different library names (e.g., `ld-musl-x86_64.so.1` instead of `ld-linux-x86-64.so.2`) - pattern list must cover both

### Assumptions

- System library patterns are knowable and relatively stable across OS versions
- Header-declared dependencies (`DT_NEEDED`, `LC_LOAD_DYLIB`) are complete for Tier 2 purposes - dlopen-loaded libraries are explicitly out of scope
- RPATH/RUNPATH values in tsuku-installed libraries are well-formed (set by `set_rpath` action)
- File existence combined with Tier 1 header validation is sufficient - symbol-level checking is deferred to Tier 3

## Decision Outcome

**Chosen option: Option 3 - Hybrid Approach (Pattern + RPATH-Aware Resolution)**

The hybrid approach best addresses the cross-platform requirement while providing meaningful validation of tsuku-managed dependencies. Pattern-based detection handles the macOS dyld cache correctly, while RPATH-aware resolution ensures tsuku-installed libraries actually exist and are valid.

### Rationale

This option was chosen because:
- **Cross-platform support**: Pattern matching works regardless of whether system libraries exist on disk, handling macOS dyld cache correctly
- **No false positives**: Known system libraries are skipped; only tsuku-managed dependencies are resolved and validated
- **Validates tsuku dependencies**: Unlike pattern-only (Option 1), this actually verifies that referenced libraries exist and pass Tier 1 validation
- **Consistent with Tier 1**: Reuses `HeaderInfo.Dependencies` from existing validation; adds error categories for dependency failures

Alternatives were rejected because:
- **Option 1 (Pattern-Only)**: Doesn't verify tsuku-managed dependencies exist - a library could reference a missing `$ORIGIN/../lib/libfoo.so` and pass verification
- **Option 2 (Pure Resolution)**: Non-viable on macOS due to dyld shared cache; would produce false positives for every system library

### Trade-offs Accepted

By choosing this option, we accept:
- **Pattern list maintenance**: System library patterns must be updated as OS versions evolve
- **Implementation complexity**: Two-phase logic is more complex than pure pattern matching
- **RPATH parsing**: Must extract RPATH entries from binaries for proper variable expansion

These are acceptable because:
- Pattern lists change infrequently and the shell script provides a reference implementation
- The two-phase logic is well-defined with clear boundaries (classification → resolution)
- RPATH extraction uses the same `debug/elf` and `debug/macho` packages already in use

## Solution Architecture

### Overview

Tier 2 dependency validation extends the existing `internal/verify` package with a new `ValidateDependencies()` function. This function takes the dependencies already extracted by Tier 1's `ValidateHeader()` and classifies/resolves each one according to the hybrid approach.

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    cmd/tsuku/verify.go                          │
│                    (integration point)                          │
└───────────────────────────┬─────────────────────────────────────┘
                            │ calls
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                  internal/verify/deps.go                        │
│                                                                 │
│  ValidateDependencies(libPath string, deps []string) []DepResult│
│                                                                 │
│  ┌─────────────────┐    ┌─────────────────┐                    │
│  │ ClassifyDep()   │───▶│ ResolveTsukuDep │                    │
│  │ (pattern match) │    │ (RPATH expand)  │                    │
│  └─────────────────┘    └─────────────────┘                    │
│           │                      │                              │
│           ▼                      ▼                              │
│  ┌─────────────────┐    ┌─────────────────┐                    │
│  │ systemPatterns  │    │ ValidateHeader  │                    │
│  │ (linux/darwin)  │    │ (Tier 1 reuse)  │                    │
│  └─────────────────┘    └─────────────────┘                    │
└─────────────────────────────────────────────────────────────────┘
```

### Key Interfaces

**New types in `internal/verify/types.go`:**

```go
// DepResult represents the validation result for a single dependency
type DepResult struct {
    // Name is the dependency as listed in the binary (e.g., "libc.so.6", "@rpath/libfoo.dylib")
    Name string

    // Status is the validation outcome
    Status DepStatus

    // ResolvedPath is the expanded path (only set for tsuku-managed deps)
    ResolvedPath string

    // Error is set if Status is DepMissing or DepInvalid
    Error error
}

// DepStatus represents the classification/validation outcome
type DepStatus int

const (
    DepSystem   DepStatus = iota // System library, skipped
    DepValid                     // Tsuku-managed, validated
    DepMissing                   // Tsuku-managed, file not found
    DepInvalid                   // Tsuku-managed, Tier 1 validation failed
    DepWarning                   // Unknown absolute path (not system, not tsuku)
)

// Tier 2 error categories (extends Tier 1's 0-5 range)
const (
    ErrDepMissing  ErrorCategory = 10 // Dependency file not found
    ErrDepInvalid  ErrorCategory = 11 // Dependency exists but Tier 1 validation failed
    ErrDepWarning  ErrorCategory = 12 // Unknown absolute path outside tsuku
)
```

**New function in `internal/verify/deps.go`:**

```go
// ValidateDependencies checks that all dependencies can be resolved.
// libPath is the path to the library being validated (for $ORIGIN expansion).
// deps is the dependency list from HeaderInfo.Dependencies.
// libsDir is $TSUKU_HOME/libs/ for tsuku-managed dependency resolution.
func ValidateDependencies(libPath string, deps []string, libsDir string) []DepResult
```

### Data Flow

1. `verifyLibrary()` in `cmd/tsuku/verify.go` iterates library files via `findLibraryFiles(libDir)`
2. For each file, calls `verify.ValidateHeader(libFile)` → returns `HeaderInfo` with `Dependencies []string`
3. For each library with dependencies, calls `verify.ValidateDependencies(libFile, info.Dependencies, libsDir)`
4. For each dependency in `ValidateDependencies()`:
   - `classifyDep(dep)` → returns `DepSystem`, `DepWarning`, or "needs resolution"
   - If needs resolution: `resolveTsukuDep(dep, libPath, libsDir)` → expands path variables
   - If resolved: `ValidateHeader(resolvedPath)` → validates the dependency file
5. Results aggregated and returned to caller

**Integration point:** The current `verifyLibrary()` function logs "Tier 2: not yet implemented" at the point where `ValidateDependencies()` should be called.

### System Library Patterns

Patterns are organized by platform and stored as Go slices for O(n) matching:

```go
// Linux system library patterns (glibc + musl)
// Synced from test/scripts/verify-no-system-deps.sh
var linuxSystemPatterns = []string{
    "linux-vdso.so",       // Virtual DSO (kernel-provided)
    "linux-gate.so",       // 32-bit virtual DSO
    "ld-linux",            // Dynamic linker (glibc)
    "ld-musl",             // Dynamic linker (musl/Alpine)
    "libc.so",             // C library
    "libm.so",             // Math library
    "libdl.so",            // Dynamic loading
    "libpthread.so",       // POSIX threads
    "librt.so",            // Realtime extensions
    "libresolv.so",        // DNS resolver
    "libnsl.so",           // Network services library
    "libcrypt.so",         // Cryptographic library
    "libutil.so",          // Utility library
    "libgcc_s.so",         // GCC runtime
    "libstdc++.so",        // C++ standard library
    "libatomic.so",        // Atomic operations (GCC)
}

var linuxSystemPaths = []string{
    "/lib64/",
    "/lib/x86_64-linux-gnu/",
    "/lib/aarch64-linux-gnu/",
    "/usr/lib/x86_64-linux-gnu/",
    "/usr/lib/aarch64-linux-gnu/",
}

// macOS system library patterns
var darwinSystemPatterns = []string{
    "/usr/lib/libSystem",  // Core system library
    "/usr/lib/libc++",     // C++ standard library
    "/usr/lib/libobjc",    // Objective-C runtime
    "/System/Library/",    // System frameworks
}

// Path variable prefixes (resolve, don't skip)
var pathVariablePrefixes = []string{
    "$ORIGIN",
    "@rpath",
    "@loader_path",
    "@executable_path",
}
```

**RPATH extraction interface:**

```go
// ExtractRPaths returns the RPATH/RUNPATH entries from a binary.
// For ELF: extracts DT_RPATH and DT_RUNPATH from dynamic section
// For Mach-O: extracts LC_RPATH load commands
func ExtractRPaths(path string) ([]string, error)
```

**RPATH extraction implementation:**

For ELF binaries, use Go's `debug/elf` package:

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

For Mach-O binaries, iterate load commands:

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

Note: Go's `debug/macho.Rpath` type is available since Go 1.16.

## Implementation Approach

### Step 1: Add dependency types and error categories

Add `DepResult`, `DepStatus`, and new `ErrorCategory` values to `internal/verify/types.go`.

**Files:** `internal/verify/types.go`

### Step 2: Implement classification logic

Create `internal/verify/deps.go` with `classifyDep()` function that matches against system patterns and identifies path variables.

**Files:** `internal/verify/deps.go`

### Step 3: Implement path variable expansion

Add `expandPathVariables()` to handle `$ORIGIN`, `@rpath`, `@loader_path`, `@executable_path`. Extract RPATH entries from binaries for `@rpath` resolution.

**Files:** `internal/verify/deps.go`

**Symlink handling:** Follow the same pattern established for checksum computation (PR #963):
- Use `filepath.EvalSymlinks()` to resolve symlinks to real files before validation
- Only validate the real file, not each symlink pointing to it
- This is consistent with `findLibraryFiles()` in `cmd/tsuku/verify.go`

### Step 4: Implement ValidateDependencies

Wire together classification, expansion, and Tier 1 validation into the main entry point.

**Files:** `internal/verify/deps.go`

### Step 5: Integrate with verify command

Update `cmd/tsuku/verify.go` to call `ValidateDependencies()` after `ValidateHeader()` and report results.

**Files:** `cmd/tsuku/verify.go`

### Step 6: Add unit tests

Create `internal/verify/deps_test.go` with tests for pattern matching, path expansion, and integration scenarios.

**Files:** `internal/verify/deps_test.go`

### Step 7: Update CI integration test

Update `test/scripts/test-library-verify.sh` to verify that Tier 2 validation runs and catches dependency issues.

**Files:** `test/scripts/test-library-verify.sh`

## Consequences

### Positive

- **Catches missing dependencies**: Libraries with unresolvable tsuku-managed dependencies are flagged before runtime failures occur
- **Cross-platform**: Works correctly on both Linux and macOS, handling dyld shared cache
- **Reuses existing code**: Leverages Tier 1's `ValidateHeader()` for dependency validation
- **Clear error reporting**: Each dependency gets a specific status (system, valid, missing, invalid, warning)

### Negative

- **Pattern maintenance**: System library patterns must be updated when new OS versions add libraries
- **RPATH complexity**: `@rpath` resolution requires extracting LC_RPATH entries, adding parsing complexity
- **No symbol validation**: A library might exist but have wrong symbols - this is deferred to Tier 3

### Mitigations

- **Pattern maintenance**: Start with patterns from `verify-no-system-deps.sh` which covers common cases; document process for adding new patterns
- **RPATH complexity**: Use Go's standard library for RPATH extraction; keep implementation simple by only supporting the variable prefixes tsuku sets via `set_rpath`
- **No symbol validation**: Tier 2's scope is explicit about this limitation; document that Tier 3 will address symbol-level issues

## Security Considerations

### Download Verification

**Not applicable** - This feature does not download external artifacts. Tier 2 validation operates on already-installed libraries within `$TSUKU_HOME/libs/`. The libraries being validated were downloaded and verified by the installation process (which has its own download verification via checksums).

### Execution Isolation

**File system access scope:**
- Reads library files from `$TSUKU_HOME/libs/` (trusted tsuku-managed directory)
- Reads binary headers using `debug/elf` and `debug/macho` (no code execution)
- No writes to any files; purely read-only operation

**Network access requirements:**
- None. Dependency validation is entirely local.

**Privilege escalation risks:**
- None. Validation runs with the same privileges as the user invoking `tsuku verify`.
- No setuid operations or privilege elevation.

### Supply Chain Risks

**Source trust model:**
- Libraries being validated come from tsuku recipes that specify download sources
- Tier 2 validation does not alter the trust model; it only validates that dependencies are present
- Pattern lists for system libraries are compiled into the binary, not fetched externally

**What if upstream is compromised:**
- Tier 2 cannot detect compromised libraries (that's integrity verification, Tier 4)
- If a malicious library is installed, Tier 2 will validate its dependencies exist but cannot detect malicious content
- This is acceptable because Tier 2's scope is dependency resolution, not integrity

### User Data Exposure

**Local data accessed:**
- Library file paths and dependency lists (technical metadata, not user content)
- No access to user documents, credentials, or personal data

**Data sent externally:**
- None. Validation is entirely local.

**Privacy implications:**
- None. No user data is collected or transmitted.

### Security Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Path traversal via symlinks | Use `filepath.EvalSymlinks()` to resolve symlinks before path validation | Minimal - resolved path must still pass prefix check |
| Path traversal via normalization tricks | Apply `filepath.Clean()` to all dependency paths before pattern matching | Minimal - standard normalization handles `/usr//lib/`, `/usr/lib/../lib/` |
| Path traversal in dependency paths | Validate resolved paths stay within `$TSUKU_HOME/libs/` or system paths after symlink resolution | Bounded by file existence check after validation |
| Parser vulnerabilities in ELF/Mach-O parsing | Use Go's standard library parsers with panic recovery (already in Tier 1) | Theoretical bugs in `debug/elf` or `debug/macho`; mitigated by Go's security practices |
| Dependency count exhaustion | Limit to 1000 dependencies per binary; warn if exceeded | Extremely malicious binaries could still cause slowdown |
| Unexpanded variable injection | Treat unexpanded variables (e.g., `$MALICIOUS_VAR`) after expansion as errors | None - unknown variables cause failure |
| False sense of security | Document clearly that Tier 2 validates dependency presence, not integrity or symbols | Users must understand verification tiers |

**Defense-in-Depth Note:** While Tier 2 does not download artifacts, it operates on binaries that could be malicious if installation verification was bypassed. The panic recovery in parsers provides a secondary defense layer against crafted malicious binaries.

