# Issue #980 Implementation Plan

## Goal
Create a system library registry with 47 patterns that identifies inherently OS-provided libraries on Linux and macOS.

## File to Create
`internal/verify/system_libs.go` and `internal/verify/system_libs_test.go`

## Implementation Design

### Struct: SystemLibraryRegistry

```go
type SystemLibraryRegistry struct {
    linuxSonamePatterns  []string  // 18 patterns
    darwinSonamePatterns []string  // 10 patterns
    linuxPathPatterns    []string  // 12 patterns
    darwinPathPatterns   []string  // 2 patterns
    pathVariablePrefixes []string  // 5 patterns
}
```

### Pattern Categories

**1. Linux Soname Patterns (18):**
- vDSO/gate: `linux-vdso.so`, `linux-gate.so`
- Loaders: `ld-linux`, `ld-musl`
- glibc core: `libc.so`, `libm.so`, `libdl.so`, `libpthread.so`, `librt.so`
- glibc extras: `libresolv.so`, `libnsl.so`, `libcrypt.so`, `libutil.so`
- GCC runtime: `libgcc_s.so`, `libstdc++.so`, `libatomic.so`, `libgomp.so`
- glibc additional: `libmvec.so` (vector math)

**2. Darwin Soname/Path Patterns (10):**
- `/usr/lib/libSystem.B.dylib`
- `/usr/lib/libc++.1.dylib`
- `/usr/lib/libc++abi.dylib`
- `/usr/lib/libobjc.A.dylib`
- `/usr/lib/libresolv.9.dylib`
- `/usr/lib/libz.1.dylib` (system zlib - macOS provides)
- `/usr/lib/libiconv.2.dylib`
- `/usr/lib/libcharset.1.dylib`
- `/System/Library/Frameworks/`
- `/System/Library/PrivateFrameworks/`

**3. Linux Path Patterns (12):**
- `/lib/x86_64-linux-gnu/`
- `/lib/aarch64-linux-gnu/`
- `/lib/i386-linux-gnu/`
- `/lib/arm-linux-gnueabihf/`
- `/lib64/`
- `/lib32/`
- `/lib/`
- `/usr/lib/x86_64-linux-gnu/`
- `/usr/lib/aarch64-linux-gnu/`
- `/usr/lib/i386-linux-gnu/`
- `/usr/lib64/`
- `/usr/lib/`

**4. macOS Path Patterns (2):**
- `/usr/lib/`
- `/System/Library/`

**5. Path Variable Prefixes (5):**
- `$ORIGIN`
- `${ORIGIN}`
- `@rpath`
- `@loader_path`
- `@executable_path`

### Method: IsSystemLibrary

```go
func (r *SystemLibraryRegistry) IsSystemLibrary(name string, targetOS string) bool
```

Logic:
1. Check if name starts with path variable prefix → return true (handled separately)
2. For targetOS == "linux":
   - Check if name matches any Linux soname pattern (prefix match)
   - Check if name starts with any Linux path pattern
3. For targetOS == "darwin":
   - Check if name matches any Darwin soname/path pattern (prefix match)
   - Check if name starts with any Darwin path pattern
4. Return false if no match

### Package-level Function

```go
func IsSystemLibrary(name string, targetOS string) bool {
    return DefaultRegistry.IsSystemLibrary(name, targetOS)
}
```

## Test Plan

1. Test Linux soname patterns:
   - `libc.so.6` → true
   - `ld-linux-x86-64.so.2` → true
   - `libgcc_s.so.1` → true
   - `linux-vdso.so.1` → true

2. Test Darwin patterns:
   - `/usr/lib/libSystem.B.dylib` → true
   - `/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation` → true

3. Test path variable prefixes:
   - `$ORIGIN/../lib/libfoo.so` → true
   - `@rpath/libfoo.dylib` → true

4. Test non-system libraries:
   - `libssl.so.3` → false
   - `libyaml.so.0` → false
   - `/home/user/.tsuku/lib/libfoo.so` → false

5. Test cross-platform correctness:
   - `libc.so.6` on darwin → false
   - `/usr/lib/libSystem.B.dylib` on linux → false

## Validation Commands

```bash
go build ./...
go test -v ./internal/verify/... -run TestSystemLibrary
```
