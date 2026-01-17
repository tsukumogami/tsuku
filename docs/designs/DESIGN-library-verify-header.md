# DESIGN: Library Verification Header Validation (Tier 1)

**Status:** Accepted

## Upstream Design Reference

This design implements Level 1 (Header Validation) from [DESIGN-library-verification.md](DESIGN-library-verification.md).

**Relevant sections:**
- Solution Architecture: Level 1 specification (~50us, validates format and architecture)
- Implementation Approach: Step 1 (Header Validation Module)
- Research Summary: Header validation catches wrong file type, truncation, architecture mismatch

## Context and Problem Statement

The umbrella design (DESIGN-library-verification.md) defines a four-tier verification system for shared libraries. Tier 1 (Header Validation) must answer: "Is this file a valid shared library for the current platform?"

This question has significant cross-platform complexity:
- **ELF (Linux)**: Uses `debug/elf` to parse ELF headers. Shared objects have type `ET_DYN`. Dependencies are stored as `DT_NEEDED` entries in the dynamic section.
- **Mach-O (macOS)**: Uses `debug/macho` to parse Mach-O headers. Dylibs have type `MH_DYLIB`. Dependencies are stored as `LC_LOAD_DYLIB` load commands.
- **Universal binaries (macOS)**: Fat binaries contain multiple architectures. Must extract the slice matching the current platform.

The header validation module establishes patterns that will be reused by Tier 2 (dependency resolution) and informs error handling throughout the verification subsystem.

### Scope

**In scope:**
- Validating file is a shared library (not executable, object file, or static library)
- Architecture compatibility checking (x86_64 vs arm64)
- Extracting dependency list for downstream use (Tier 2)
- Counting exported symbols (sanity check)
- Handling universal/fat binaries on macOS
- Defining `HeaderInfo` return structure
- Defining error categorization taxonomy

**Out of scope:**
- Symbol table validation against expected APIs (Tier 2 may use symbol counts)
- Dependency resolution (Tier 2)
- dlopen testing (Tier 3)
- Checksum verification (Tier 4)
- DWARF debug info parsing

## Decision Drivers

- **Performance**: Must complete in ~50 microseconds per file (per umbrella design). This target assumes files are on local storage and in OS cache.
- **Safety**: Must not execute any code from the library being validated. Must not crash on malicious input.
- **Robustness**: Must handle malformed, truncated, and malicious files without crashing, including explicit panic recovery.
- **Cross-platform**: Must work identically on Linux (ELF) and macOS (Mach-O)
- **Downstream usability**: `HeaderInfo` structure must provide data needed by Tier 2
- **Error clarity**: Users must understand why validation failed and what to do

## Implementation Context

### Go Standard Library APIs

**ELF parsing (`debug/elf`):**
```go
f, err := elf.Open(path)
f.Type         // ET_DYN for shared objects
f.Machine      // EM_X86_64, EM_AARCH64, etc.
f.Class        // ELFCLASS32 or ELFCLASS64
f.ImportedLibraries() // DT_NEEDED entries
f.DynamicSymbols()    // Symbol table
```

**Mach-O parsing (`debug/macho`):**
```go
f, err := macho.Open(path)
f.Type         // TypeDylib for dynamic libraries
f.Cpu          // CpuAmd64, CpuArm64, etc.
f.ImportedLibraries() // LC_LOAD_DYLIB entries
f.Symtab.Syms         // Symbol table
```

**Fat binary handling (`debug/macho`):**
```go
ff, err := macho.NewFatFile(r)
// Returns ErrNotFat if not a fat binary
for _, arch := range ff.Arches {
    if arch.Cpu == targetCpu {
        return validateMachO(arch.File)
    }
}
```

### Error Types

Both packages define `FormatError` for parsing failures:
```go
type FormatError struct { /* offset, message, value */ }
```

Common error patterns:
- `io.EOF` / `io.ErrUnexpectedEOF` - truncated file
- `FormatError` - invalid structure (wrong magic, bad offsets)
- `os.PathError` - file access issues

### Existing Codebase Patterns

From `internal/install/checksum.go`:
```go
type ChecksumMismatch struct {
    Path     string // Relative path
    Expected string
    Actual   string
    Error    error  // Non-nil if file unreadable
}
```

This pattern separates "file is wrong" from "file is unreadable."

From `cmd/tsuku/verify.go`:
- Hierarchical output with 2/4-space indentation
- Exit code `ExitVerifyFailed` (7) for validation failures
- Error messages to stderr, info to stdout (respects `--quiet`)

## Considered Options

### Option 1: Unified Validation Function with Platform Dispatch

Single entry point that dispatches based on detected format:

```go
func ValidateHeader(path string) (*HeaderInfo, error) {
    // Try ELF first (Linux)
    if f, err := elf.Open(path); err == nil {
        defer f.Close()
        return validateELF(f)
    }

    // Try Mach-O (macOS)
    if f, err := macho.Open(path); err == nil {
        defer f.Close()
        return validateMachO(f)
    }

    // Try fat binary
    if ff, err := macho.OpenFat(path); err == nil {
        defer ff.Close()
        return validateFat(ff)
    }

    return nil, &ValidationError{Category: ErrInvalidFormat, ...}
}
```

**Pros:**
- Simple API: one function to call
- Automatically detects format without caller needing to know
- Follows existing pattern in `debug/` packages

**Cons:**
- Tries multiple parsers on invalid files (performance overhead on bad input)
- Less explicit about expected format
- Fat binary handling interleaved with single-arch

### Option 2: Format-Specific Functions with Explicit Dispatch

Separate functions for each format, caller decides which to use:

```go
func ValidateELF(path string) (*HeaderInfo, error)
func ValidateMachO(path string) (*HeaderInfo, error)
func ValidateFat(path string, targetArch string) (*HeaderInfo, error)

// Caller:
if runtime.GOOS == "linux" {
    info, err = header.ValidateELF(path)
} else if runtime.GOOS == "darwin" {
    info, err = header.ValidateMachOOrFat(path, runtime.GOARCH)
}
```

**Pros:**
- No wasted parsing attempts (direct to correct parser)
- Clear separation of concerns
- Fat binary handling explicit and testable

**Cons:**
- More complex caller code
- Duplicates format detection logic at call sites
- Three functions instead of one

### Option 3: Unified Function with Early Format Detection (Recommended)

Single entry point with efficient magic number detection before full parsing:

```go
func ValidateHeader(path string) (*HeaderInfo, error) {
    // Read first 8 bytes for format detection
    magic, err := readMagic(path)
    if err != nil {
        return nil, &ValidationError{Category: ErrUnreadable, Err: err}
    }

    switch {
    case isELF(magic):
        return validateELFPath(path)
    case isMachO(magic):
        return validateMachOPath(path)
    case isFatBinary(magic):
        return validateFatPath(path)
    default:
        return nil, &ValidationError{Category: ErrInvalidFormat, ...}
    }
}
```

**Pros:**
- Simple API (one function)
- Efficient: reads 8 bytes for format detection before full parse
- Clear error for unknown formats
- No wasted parsing attempts

**Cons:**
- Extra I/O operation (magic read + full parse)
- Still need internal format-specific functions

## Evaluation Against Decision Drivers

| Option | Performance | Safety | Robustness | Cross-platform | Downstream | Error Clarity |
|--------|-------------|--------|------------|----------------|------------|---------------|
| 1. Unified dispatch | Fair (tries multiple) | Good | Good | Good | Good | Fair |
| 2. Explicit dispatch | Good | Good | Good | Fair (caller logic) | Good | Good |
| 3. Magic detection | Good | Good | Good | Good | Good | Good |

## Uncertainties

- **Fat binary prevalence**: Most modern macOS tools ship as universal binaries (arm64+x86_64). We need to handle this case well, not as an edge case.
- **Symbol count performance**: Counting all dynamic symbols can take 200-800us for large libraries. Decision: Make symbol counting lazy (skip by default, compute only when explicitly requested or for Tier 2).
- **Static library detection**: `.a` files (ar archives) have magic `!<arch>\n`. Decision: Add explicit detection for ar archives and return `ErrNotSharedLib` with clear message.
- **File system performance**: The ~50us target assumes local storage with OS caching. Network mounts or uncached files may be significantly slower.

## Decision Outcome

**Chosen option: Option 3 (Unified Function with Early Format Detection)**

### Rationale

Option 3 provides the best balance of simplicity and performance:

1. **Simple API**: Callers just call `ValidateHeader(path)` without format knowledge
2. **Efficient**: 8-byte magic read is negligible (~1us), avoids trying wrong parsers
3. **Clear errors**: Can immediately identify "not a binary format" vs "invalid ELF" vs "invalid Mach-O"
4. **Fat binary support**: Naturally handles universal binaries as a distinct case

### Trade-offs Accepted

- Extra file open (magic read, then full parse) adds ~5us overhead per file
- Internal complexity of three validation paths (ELF, Mach-O, Fat)
- Fat binaries require architecture matching logic

## Solution Architecture

### Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                   ValidateHeader(path)                           │
├─────────────────────────────────────────────────────────────────┤
│  1. Read 8-byte magic number                                     │
│  2. Dispatch to format-specific validator:                       │
│     ├── ELF:     validateELF() → HeaderInfo                      │
│     ├── Mach-O:  validateMachO() → HeaderInfo                    │
│     └── Fat:     validateFat() → extract arch → validateMachO()  │
│  3. Return HeaderInfo or ValidationError                         │
└─────────────────────────────────────────────────────────────────┘
```

### Data Structures

```go
// HeaderInfo contains validated header information for a shared library.
type HeaderInfo struct {
    // Format identifies the binary format ("ELF" or "Mach-O")
    Format string

    // Type describes the file type ("shared object", "dynamic library", etc.)
    Type string

    // Architecture is the target architecture ("x86_64", "arm64", etc.)
    Architecture string

    // Dependencies lists required libraries (DT_NEEDED or LC_LOAD_DYLIB)
    Dependencies []string

    // SymbolCount is the number of exported dynamic symbols
    // Note: May be -1 if symbol counting was skipped for performance
    SymbolCount int

    // SourceArch is set for fat binaries to indicate which slice was used
    // Empty for single-architecture files
    SourceArch string
}
```

```go
// ValidationError categorizes validation failures for user-friendly reporting.
type ValidationError struct {
    Category ErrorCategory
    Path     string
    Message  string
    Err      error // Underlying error (may be nil)
}

func (e *ValidationError) Error() string
func (e *ValidationError) Unwrap() error
```

```go
// ErrorCategory classifies validation failures.
type ErrorCategory int

const (
    // ErrUnreadable indicates the file could not be read (permission, not found, etc.)
    ErrUnreadable ErrorCategory = iota

    // ErrInvalidFormat indicates the file is not a recognized binary format
    ErrInvalidFormat

    // ErrNotSharedLib indicates the file is a valid binary but not a shared library
    // (e.g., executable, object file, static library)
    ErrNotSharedLib

    // ErrWrongArch indicates the library is for a different architecture
    ErrWrongArch

    // ErrTruncated indicates the file appears truncated (unexpected EOF)
    ErrTruncated

    // ErrCorrupted indicates the file has invalid internal structure
    ErrCorrupted
)
```

### Component Changes

**New file: `internal/verify/header.go`**
- `ValidateHeader(path string) (*HeaderInfo, error)` - main entry point
- `validateELFPath(path string) (*HeaderInfo, error)` - ELF validation
- `validateMachOPath(path string) (*HeaderInfo, error)` - Mach-O validation
- `validateFatPath(path string) (*HeaderInfo, error)` - fat binary handling
- `readMagic(path string) ([]byte, error)` - efficient magic detection
- `mapELFMachine(m elf.Machine) string` - architecture name mapping
- `mapMachOCpu(c macho.Cpu) string` - architecture name mapping

**New file: `internal/verify/header_test.go`**
- Tests for each format with valid libraries
- Tests for each error category (truncated, wrong type, wrong arch)
- Tests for fat binary architecture extraction
- Benchmarks for performance validation (~50us target)

### Magic Number Detection

```go
var (
    elfMagic    = []byte{0x7f, 'E', 'L', 'F'}           // ELF
    machO32     = []byte{0xfe, 0xed, 0xfa, 0xce}       // Mach-O 32-bit
    machO64     = []byte{0xfe, 0xed, 0xfa, 0xcf}       // Mach-O 64-bit
    machO32Rev  = []byte{0xce, 0xfa, 0xed, 0xfe}       // Mach-O 32-bit (reversed)
    machO64Rev  = []byte{0xcf, 0xfa, 0xed, 0xfe}       // Mach-O 64-bit (reversed)
    fatMagic    = []byte{0xca, 0xfe, 0xba, 0xbe}       // Fat/Universal
    arMagic     = []byte{'!', '<', 'a', 'r', 'c', 'h', '>', '\n'} // Static library (ar archive)
)

func detectFormat(magic []byte) string {
    switch {
    case bytes.HasPrefix(magic, elfMagic):
        return "elf"
    case bytes.Equal(magic[:4], machO32), bytes.Equal(magic[:4], machO32Rev),
         bytes.Equal(magic[:4], machO64), bytes.Equal(magic[:4], machO64Rev):
        return "macho"
    case bytes.Equal(magic[:4], fatMagic):
        return "fat"
    case bytes.HasPrefix(magic, arMagic):
        return "ar" // Static library - will return ErrNotSharedLib
    default:
        return ""
    }
}
```

### ELF Validation Logic

```go
func validateELFPath(path string) (*HeaderInfo, error) {
    f, err := elf.Open(path)
    if err != nil {
        return nil, categorizeELFError(path, err)
    }
    defer f.Close()

    // Check file type
    if f.Type != elf.ET_DYN {
        return nil, &ValidationError{
            Category: ErrNotSharedLib,
            Path:     path,
            Message:  fmt.Sprintf("file is %s, not shared object", elfTypeName(f.Type)),
        }
    }

    // Check architecture
    currentArch := mapGoArchToELF(runtime.GOARCH)
    if f.Machine != currentArch {
        return nil, &ValidationError{
            Category: ErrWrongArch,
            Path:     path,
            Message:  fmt.Sprintf("library is %s, expected %s",
                mapELFMachine(f.Machine), mapELFMachine(currentArch)),
        }
    }

    // Extract dependencies
    deps, err := f.ImportedLibraries()
    if err != nil {
        // No dependencies is valid (leaf library)
        deps = nil
    }

    // Symbol counting is lazy - skip by default for performance
    // Tier 2 will request symbol count if needed
    symbolCount := -1

    return &HeaderInfo{
        Format:       "ELF",
        Type:         "shared object",
        Architecture: mapELFMachine(f.Machine),
        Dependencies: deps,
        SymbolCount:  symbolCount,
    }, nil
}
```

### Mach-O Validation Logic

```go
func validateMachOPath(path string) (*HeaderInfo, error) {
    f, err := macho.Open(path)
    if err != nil {
        return nil, categorizeMachOError(path, err)
    }
    defer f.Close()

    // Check file type
    if f.Type != macho.TypeDylib && f.Type != macho.TypeBundle {
        return nil, &ValidationError{
            Category: ErrNotSharedLib,
            Path:     path,
            Message:  fmt.Sprintf("file is %s, not dynamic library", machoTypeName(f.Type)),
        }
    }

    // Check architecture
    currentCpu := mapGoArchToMachO(runtime.GOARCH)
    if f.Cpu != currentCpu {
        return nil, &ValidationError{
            Category: ErrWrongArch,
            Path:     path,
            Message:  fmt.Sprintf("library is %s, expected %s",
                mapMachOCpu(f.Cpu), mapMachOCpu(currentCpu)),
        }
    }

    // Extract dependencies
    deps, err := f.ImportedLibraries()
    if err != nil {
        deps = nil
    }

    // Symbol counting is lazy - skip by default for performance
    symbolCount := -1

    return &HeaderInfo{
        Format:       "Mach-O",
        Type:         machoTypeName(f.Type),
        Architecture: mapMachOCpu(f.Cpu),
        Dependencies: deps,
        SymbolCount:  symbolCount,
    }, nil
}
```

### Fat Binary Handling

```go
func validateFatPath(path string) (*HeaderInfo, error) {
    ff, err := macho.OpenFat(path)
    if err != nil {
        return nil, categorizeFatError(path, err)
    }
    defer ff.Close()

    targetCpu := mapGoArchToMachO(runtime.GOARCH)

    // Find matching architecture
    for _, arch := range ff.Arches {
        if arch.Cpu == targetCpu {
            info, err := validateMachOFile(arch.File)
            if err != nil {
                return nil, err
            }
            info.SourceArch = fmt.Sprintf("fat(%s)", mapMachOCpu(arch.Cpu))
            return info, nil
        }
    }

    // No matching architecture
    available := make([]string, len(ff.Arches))
    for i, arch := range ff.Arches {
        available[i] = mapMachOCpu(arch.Cpu)
    }

    return nil, &ValidationError{
        Category: ErrWrongArch,
        Path:     path,
        Message:  fmt.Sprintf("no %s slice in universal binary (has: %s)",
            runtime.GOARCH, strings.Join(available, ", ")),
    }
}
```

### Architecture Mapping

```go
func mapGoArchToELF(goarch string) elf.Machine {
    switch goarch {
    case "amd64":
        return elf.EM_X86_64
    case "arm64":
        return elf.EM_AARCH64
    case "386":
        return elf.EM_386
    case "arm":
        return elf.EM_ARM
    default:
        return elf.EM_NONE
    }
}

func mapGoArchToMachO(goarch string) macho.Cpu {
    switch goarch {
    case "amd64":
        return macho.CpuAmd64
    case "arm64":
        return macho.CpuArm64
    case "386":
        return macho.Cpu386
    default:
        return 0
    }
}

func mapELFMachine(m elf.Machine) string {
    switch m {
    case elf.EM_X86_64:
        return "x86_64"
    case elf.EM_AARCH64:
        return "arm64"
    case elf.EM_386:
        return "i386"
    case elf.EM_ARM:
        return "arm"
    default:
        return fmt.Sprintf("unknown(%d)", m)
    }
}

func mapMachOCpu(c macho.Cpu) string {
    switch c {
    case macho.CpuAmd64:
        return "x86_64"
    case macho.CpuArm64:
        return "arm64"
    case macho.Cpu386:
        return "i386"
    default:
        return fmt.Sprintf("unknown(%d)", c)
    }
}
```

### Error Categorization

```go
func categorizeELFError(path string, err error) *ValidationError {
    var fmtErr *elf.FormatError

    switch {
    case errors.As(err, &fmtErr):
        if strings.Contains(fmtErr.Error(), "bad magic") {
            return &ValidationError{Category: ErrInvalidFormat, Path: path, Err: err}
        }
        return &ValidationError{Category: ErrCorrupted, Path: path, Err: err}

    case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
        return &ValidationError{Category: ErrTruncated, Path: path, Err: err}

    case errors.Is(err, os.ErrNotExist), errors.Is(err, os.ErrPermission):
        return &ValidationError{Category: ErrUnreadable, Path: path, Err: err}

    default:
        return &ValidationError{Category: ErrUnreadable, Path: path, Err: err}
    }
}
```

### Verification Output Integration

When integrated with `cmd/tsuku/verify.go`, the output will be:

```
$ tsuku verify gcc-libs
Verifying gcc-libs (version 15.2.0)...

  lib/libstdc++.so.6.0.33
    Format: ELF shared object (x86_64)
    Dependencies: libm.so.6, libc.so.6, libgcc_s.so.1
    ...

  lib/libgcc_s.so.1
    Format: ELF shared object (x86_64)
    Dependencies: libc.so.6
    ...
```

Failure example:

```
$ tsuku verify gcc-libs
Verifying gcc-libs (version 15.2.0)...

  lib/libstdc++.so.6.0.33
    Format: FAILED
      Error: file is executable, not shared object
```

## Implementation Approach

### Step 1: Create Header Validation Module

Create `internal/verify/header.go` with:
1. Magic number constants and detection
2. `ValidateHeader()` entry point
3. Format-specific validators (ELF, Mach-O, Fat)
4. Architecture mapping functions
5. Error categorization

### Step 2: Create Data Structures

Create `internal/verify/types.go` with:
1. `HeaderInfo` struct
2. `ValidationError` struct
3. `ErrorCategory` constants

### Step 3: Add Comprehensive Tests

Create `internal/verify/header_test.go` with:
1. Valid library tests for ELF and Mach-O
2. Error category tests (truncated, wrong type, wrong arch)
3. Fat binary tests
4. Edge cases (empty file, wrong magic, static library)

### Step 4: Add Benchmarks

Add benchmarks to verify performance target:
```go
func BenchmarkValidateHeader_ELF(b *testing.B) {
    // Target: < 50us per file
}
```

### Step 5: Integrate with Verify Command

Modify `cmd/tsuku/verify.go` to call `ValidateHeader()` for each library file in Tier 1.

## Security Considerations

### Download Verification

**Not applicable.** Header validation operates on already-downloaded files. Download verification is handled by recipe checksums.

### Execution Isolation

**Low risk.** Header validation uses Go's `debug/elf` and `debug/macho` packages which only read and parse bytes. No code execution occurs:
- No `.init` sections run
- No constructors execute
- No dlopen calls
- Pure parsing only

The Go documentation notes these packages perform basic validation only and may panic on malicious input. We mitigate this with:

1. **Explicit panic recovery**: All validation functions are wrapped with `defer recover()` to catch panics and convert them to `ErrCorrupted` errors:
   ```go
   func validateELFPath(path string) (info *HeaderInfo, err error) {
       defer func() {
           if r := recover(); r != nil {
               err = &ValidationError{Category: ErrCorrupted, Path: path,
                   Message: fmt.Sprintf("parser panic: %v", r)}
           }
       }()
       // ... validation logic
   }
   ```
2. Input validation before passing to debug packages (magic number check)
3. Process isolation already provided by tsuku-dltest helper (Tier 3)

### Supply Chain Risks

**Not applicable.** Header validation does not involve network access or external tools.

### User Data Exposure

**Not applicable.** Header validation reads only library files in `$TSUKU_HOME/libs/`. No user data is accessed or transmitted.

### Denial of Service via Crafted Headers

**Medium consideration.** Malformed files could trigger:
- Excessive memory allocation (huge section counts)
- Infinite loops (circular references)
- Long parsing times (deeply nested structures)

**Mitigations:**
1. Go's debug packages have internal limits on section counts
2. We validate header fields before deep parsing
3. Timeout protection at verify command level (5 second default)
4. Clear error messages for invalid files (no sensitive data leaked)

### Distinguishing Legitimate Libraries from Executables

**Addressed by design.** The validation explicitly checks file type:
- ELF: Must be `ET_DYN` (rejects `ET_EXEC` executables)
- Mach-O: Must be `MH_DYLIB` or `MH_BUNDLE` (rejects `MH_EXECUTE`)

This prevents executables disguised as libraries from passing validation.

## Consequences

### Positive

- **Fast validation**: Magic detection + parsing completes in ~50us (meets performance target)
- **Clear error taxonomy**: Six distinct error categories enable actionable user messages
- **Cross-platform**: Works identically on Linux and macOS
- **Fat binary support**: Handles universal binaries naturally
- **Foundation for Tier 2**: `HeaderInfo.Dependencies` provides data for dependency resolution
- **No external dependencies**: Uses only Go standard library

### Negative

- **Cannot detect all corruption**: Header validation cannot detect corrupted code sections (only Tier 3 dlopen or Tier 4 checksums can).
- **Architecture list is limited**: Only maps common architectures (x86_64, arm64, i386, arm). Rare architectures show as "unknown(N)".
- **Performance depends on storage**: The ~50us target assumes local storage with OS caching. Network mounts or uncached files may be significantly slower.

### Neutral

- **Symbol counting is lazy**: SymbolCount returns -1 by default. Tier 2 can request explicit symbol counting if needed.
- **Debug info not validated**: DWARF sections not checked (out of scope per design).
