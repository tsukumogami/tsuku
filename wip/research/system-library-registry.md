# System Library Registry Design

**Date:** 2026-01-17
**Purpose:** Define format and patterns for system library detection in Tier 2

## Summary

The system library registry encodes our knowledge of libraries that are **inherently OS-provided**. These libraries exist as part of the operating system itself and are expected to be present on any conforming system.

We skip these libraries during dependency validation because they ARE OS-provided by nature, not because tsuku lacks recipes for them. The absence of tsuku recipes for these libraries is a CONSEQUENCE of their OS-provided nature, not the definition of why they are system libraries.

This document defines the registry format and complete pattern lists.

## Recommended Data Structure

```go
// internal/verify/system_libs.go

// SystemLibraryRegistry contains patterns for identifying inherently OS-provided libraries.
// These patterns encode our knowledge of what libraries the operating system provides as part
// of its core functionality. Pattern matching detects libraries that are OS-provided by nature.
type SystemLibraryRegistry struct {
    // Soname patterns (prefix match)
    LinuxPatterns  []string
    DarwinPatterns []string

    // Path patterns (prefix match)
    LinuxPaths  []string
    DarwinPaths []string

    // Path variable prefixes (special handling)
    PathVariables []string
}

var DefaultRegistry = SystemLibraryRegistry{
    LinuxPatterns:  linuxSystemPatterns,
    DarwinPatterns: darwinSystemPatterns,
    LinuxPaths:     linuxSystemPaths,
    DarwinPaths:    darwinSystemPaths,
    PathVariables:  pathVariablePrefixes,
}
```

## Linux System Library Patterns

### Core glibc Libraries

```go
var linuxSystemPatterns = []string{
    // Virtual DSOs (kernel-provided, no file on disk)
    "linux-vdso.so",
    "linux-gate.so",

    // Dynamic linkers
    "ld-linux",          // glibc: ld-linux-x86-64.so.2, ld-linux-aarch64.so.1
    "ld-musl",           // musl: ld-musl-x86_64.so.1

    // Core C library
    "libc.so",           // libc.so.6
    "libm.so",           // Math library
    "libdl.so",          // Dynamic loading (legacy, merged into libc in glibc 2.34)
    "libpthread.so",     // POSIX threads (legacy, merged into libc)
    "librt.so",          // Realtime extensions

    // Network/DNS
    "libresolv.so",      // DNS resolver
    "libnsl.so",         // Network services (NIS)
    "libnss_",           // Name service switch modules

    // Security/Crypto (system-provided)
    "libcrypt.so",       // Password hashing

    // Utility
    "libutil.so",        // Login utilities
    "libmvec.so",        // Vector math (glibc)

    // GCC runtime
    "libgcc_s.so",       // GCC support library
    "libstdc++.so",      // C++ standard library
    "libatomic.so",      // Atomic operations
    "libgomp.so",        // OpenMP runtime
    "libquadmath.so",    // Quad-precision math
}
```

### Linux System Paths

```go
var linuxSystemPaths = []string{
    // Standard library paths
    "/lib64/",
    "/lib/",
    "/usr/lib64/",
    "/usr/lib/",

    // Multiarch paths (Debian/Ubuntu)
    "/lib/x86_64-linux-gnu/",
    "/lib/aarch64-linux-gnu/",
    "/lib/arm-linux-gnueabihf/",
    "/usr/lib/x86_64-linux-gnu/",
    "/usr/lib/aarch64-linux-gnu/",
    "/usr/lib/arm-linux-gnueabihf/",

    // RHEL/Fedora multilib
    "/lib/i686/",
    "/usr/lib/i686/",
}
```

## macOS System Library Patterns

```go
var darwinSystemPatterns = []string{
    // Core system library (contains libc, libm, libpthread, etc.)
    "/usr/lib/libSystem",

    // C++ runtime
    "/usr/lib/libc++",
    "/usr/lib/libc++abi",

    // Objective-C runtime
    "/usr/lib/libobjc",

    // System frameworks (all are system-provided)
    "/System/Library/",

    // Common system libraries
    "/usr/lib/libz",           // zlib (system copy)
    "/usr/lib/libiconv",       // iconv
    "/usr/lib/libcurl",        // curl (system copy)
    "/usr/lib/libsqlite3",     // SQLite
    "/usr/lib/libxml2",        // libxml2
    "/usr/lib/libxslt",        // libxslt
}

var darwinSystemPaths = []string{
    "/usr/lib/",
    "/System/Library/",
}
```

## Path Variable Prefixes

```go
var pathVariablePrefixes = []string{
    // ELF (Linux)
    "$ORIGIN",       // Directory containing the binary
    "${ORIGIN}",     // Alternative syntax

    // Mach-O (macOS)
    "@rpath",        // Runtime search path
    "@loader_path",  // Directory of loading binary
    "@executable_path", // Directory of main executable
}
```

## Classification Function

```go
// ClassifyDependency determines if a dependency is inherently OS-provided or tsuku-managed.
// Libraries are classified as system libraries because they ARE part of the OS, not because
// tsuku lacks a recipe for them. The registry encodes our knowledge of OS-provided libraries.
func (r *SystemLibraryRegistry) ClassifyDependency(dep string, targetOS string) DepClassification {
    // 1. Check path variable prefixes (needs resolution)
    for _, prefix := range r.PathVariables {
        if strings.HasPrefix(dep, prefix) {
            return DepNeedsResolution
        }
    }

    // 2. Check OS-specific patterns
    patterns := r.LinuxPatterns
    paths := r.LinuxPaths
    if targetOS == "darwin" {
        patterns = r.DarwinPatterns
        paths = r.DarwinPaths
    }

    // Check soname patterns (basename matching)
    baseName := filepath.Base(dep)
    for _, pattern := range patterns {
        if strings.HasPrefix(baseName, pattern) {
            return DepSystem
        }
    }

    // Check path patterns (for absolute paths)
    if filepath.IsAbs(dep) {
        for _, pathPrefix := range paths {
            if strings.HasPrefix(dep, pathPrefix) {
                return DepSystem
            }
        }
    }

    // 3. Not a known system library
    return DepTsukuManaged
}

type DepClassification int

const (
    DepSystem          DepClassification = iota // Skip - inherently OS-provided, expected on any system
    DepTsukuManaged                             // Validate - library tsuku can/should manage
    DepNeedsResolution                          // Expand path variable first
)
```

## Handling Ambiguous Libraries

Some libraries (libssl, libz, libcurl) exist as both system and tsuku versions. These are NOT inherently system libraries - they are third-party libraries that the OS happens to bundle. When tsuku provides its own version, we want to use that; when a binary links to the system copy, we accept it.

**Resolution strategy:**

```go
func (r *SystemLibraryRegistry) ResolveAmbiguous(dep string, tsukuLibsDir string) DepClassification {
    // If absolute path in system location → system
    if filepath.IsAbs(dep) {
        for _, sysPath := range r.allSystemPaths() {
            if strings.HasPrefix(dep, sysPath) {
                return DepSystem
            }
        }
    }

    // If we can find it in tsuku libs → tsuku-managed
    if r.existsInTsukuLibs(dep, tsukuLibsDir) {
        return DepTsukuManaged
    }

    // If soname matches system pattern → assume system
    // (e.g., "libssl.so.3" without path)
    baseName := filepath.Base(dep)
    for _, pattern := range r.allPatterns() {
        if strings.HasPrefix(baseName, pattern) {
            return DepSystem
        }
    }

    // Unknown - report as warning
    return DepUnknown
}
```

## macOS dyld Shared Cache Note

Since macOS Big Sur, many system libraries don't exist as files on disk. They're in the dyld shared cache.

**Implications:**
- Cannot check file existence for system libraries
- Must rely on pattern matching only
- `dlopen()` still works (dynamic linker handles cache)

**Our approach:**
- Pattern-based detection skips file existence checks for system paths
- Libraries in `/usr/lib/` or `/System/Library/` are assumed system
- No false negatives for system libraries

## Complete Pattern Count

| Category | Count |
|----------|-------|
| Linux soname patterns | 18 |
| Linux path patterns | 12 |
| macOS soname patterns | 10 |
| macOS path patterns | 2 |
| Path variables | 5 |
| **Total** | **47** |

## Integration with Tier 2

```go
func ValidateDependencies(binaryPath string, deps []string, cfg *Config) []DepResult {
    registry := DefaultRegistry
    targetOS := runtime.GOOS

    var results []DepResult

    for _, dep := range deps {
        class := registry.ClassifyDependency(dep, targetOS)

        switch class {
        case DepSystem:
            // Skip validation - this library is inherently OS-provided.
            // We know it will be available on any conforming system.
            results = append(results, DepResult{
                Name:   dep,
                Status: DepStatusSystem,
            })

        case DepNeedsResolution:
            // Expand path variable, then re-classify
            resolved := expandPathVariable(dep, binaryPath, cfg)
            // ... recursive classification

        case DepTsukuManaged:
            // This is a library that tsuku can/should manage.
            // Look up in soname index to find which tsuku package provides it.
            // ...
        }
    }

    return results
}
```
