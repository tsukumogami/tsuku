# Issue #980 Summary

## Implementation

Created `internal/verify/system_libs.go` with a `SystemLibraryRegistry` that identifies OS-provided libraries using 47 patterns.

### Pattern Categories

| Category | Count | Examples |
|----------|-------|----------|
| Linux soname patterns | 18 | `libc.so`, `libm.so`, `ld-linux`, `libgcc_s.so` |
| Darwin soname patterns | 10 | `/usr/lib/libSystem.B.dylib`, `/System/Library/Frameworks/` |
| Linux path patterns | 12 | `/lib/x86_64-linux-gnu/`, `/lib64/`, `/usr/lib/` |
| Darwin path patterns | 2 | `/usr/lib/`, `/System/Library/` |
| Path variable prefixes | 5 | `$ORIGIN`, `@rpath`, `@loader_path` |

### API

```go
// Check if a library is system-provided
verify.IsSystemLibrary("libc.so.6", "linux")     // true
verify.IsSystemLibrary("/usr/lib/libSystem.B.dylib", "darwin") // true
verify.IsSystemLibrary("libssl.so.3", "linux")   // false

// Check if path contains runtime variables
verify.IsPathVariable("$ORIGIN/../lib/foo.so")   // true
```

### Key Design Decisions

1. **Prefix matching**: Patterns use `strings.HasPrefix` to handle versioned sonames (e.g., `libc.so.6` matches `libc.so` pattern)

2. **Path variables return true**: Runtime path variables (`$ORIGIN`, `@rpath`, etc.) are recognized so callers know to expand them before final validation

3. **Platform isolation**: Linux patterns only match on Linux, Darwin patterns only on Darwin. Path variables are cross-platform.

4. **Order of precedence**: Path variables checked first, then OS-specific soname patterns, then OS-specific path patterns

## Test Coverage

- `TestSystemLibraryRegistry_IsSystemLibrary_LinuxSonames` - 19 test cases
- `TestSystemLibraryRegistry_IsSystemLibrary_LinuxPaths` - 9 test cases
- `TestSystemLibraryRegistry_IsSystemLibrary_DarwinSonames` - 12 test cases
- `TestSystemLibraryRegistry_IsSystemLibrary_PathVariables` - 7 test cases
- `TestSystemLibraryRegistry_IsSystemLibrary_CrossPlatform` - 5 test cases
- `TestSystemLibraryRegistry_IsSystemLibrary_UnknownOS` - 3 test cases
- `TestIsSystemLibrary_ConvenienceFunction` - 3 test cases
- `TestIsPathVariable` - 7 test cases
- `TestDefaultRegistry_PatternCount` - 6 test cases (verifies 47 total)

## Files Changed

- `internal/verify/system_libs.go` (NEW) - 196 lines
- `internal/verify/system_libs_test.go` (NEW) - 316 lines
