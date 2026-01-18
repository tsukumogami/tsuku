# Implementation Context for Issue #980

## Design Reference
Design: `docs/designs/DESIGN-library-verify-deps.md`
Section: Solution Architecture - System Library Registry

## Issue Summary
Create a system library registry with 47 patterns that identifies inherently OS-provided libraries on Linux and macOS.

## Key Design Decisions

### What This Enables
When validating binary dependencies, tsuku must distinguish between:
- **System libraries** (libc.so.6, /usr/lib/libSystem.B.dylib) - inherently OS-provided
- **Tsuku-managed libraries** - installed by tsuku recipes

System libraries should be skipped during dependency validation because:
1. They're expected on any conforming system
2. On macOS, dyld shared cache means libraries don't exist as files on disk

### Pattern Categories (47 total)

**Linux soname patterns (18):**
- vDSO: `linux-vdso.so`, `linux-gate.so`
- Loaders: `ld-linux`, `ld-musl`
- glibc core: `libc.so`, `libm.so`, `libdl.so`, `libpthread.so`, `librt.so`
- glibc extras: `libresolv.so`, `libnsl.so`, `libcrypt.so`, `libutil.so`
- GCC runtime: `libgcc_s.so`, `libstdc++.so`, `libatomic.so`, `libgomp.so`

**macOS soname patterns (10):**
- System: `/usr/lib/libSystem`, `/usr/lib/libc++`, `/usr/lib/libobjc`
- Frameworks: `/System/Library/`

**Linux path patterns (12):**
- Multiarch layouts: `/lib/`, `/lib64/`, `/lib/x86_64-linux-gnu/`, etc.

**macOS path patterns (2):**
- `/usr/lib/`, `/System/Library/`

**Path variable prefixes (5):**
- `$ORIGIN`, `${ORIGIN}`, `@rpath`, `@loader_path`, `@executable_path`

### Classification Flow
```
1. Is soname in our index? → YES → TSUKU dep
2. Matches system pattern? → YES → PURE SYSTEM
3. Otherwise → UNKNOWN → FAIL
```

**Order matters:** Check soname index FIRST. A soname like `libssl.so.3` should be TSUKU-managed when we have an installed recipe.

## API Design

```go
// internal/verify/system_libs.go

type SystemLibraryRegistry struct {
    linuxSonamePatterns  []string
    darwinSonamePatterns []string
    linuxPathPatterns    []string
    darwinPathPatterns   []string
    pathVariablePrefixes []string
}

var DefaultRegistry = &SystemLibraryRegistry{...}

func (r *SystemLibraryRegistry) IsSystemLibrary(name string, targetOS string) bool
```

## Dependencies
None

## Downstream Dependencies
- Issue #986 (SonameIndex) - Needs system library patterns as classification fallback
