# C Systems Engineer Review: dlopen Load Testing Design

**Reviewer perspective**: C systems engineer with expertise in dynamic linking, dlopen/dlclose, and minimal binary development.

**Document reviewed**: `docs/designs/DESIGN-library-verify-dlopen.md`

---

## Executive Summary

The design is technically sound and demonstrates good understanding of dlopen semantics. However, there are several nuances around dlerror thread safety, dlclose behavior, and platform-specific quirks that should be addressed. The Go+cgo choice is acceptable for MVP, but C would produce a cleaner, smaller binary. The 5-second timeout is reasonable but the design should document what happens when libraries with slow initializers legitimately need more time.

---

## 1. Language Choice: Go+cgo vs C

### Assessment

The design correctly notes that C would produce a ~10KB binary vs Go+cgo's ~5MB. For a helper that does essentially this:

```c
for (int i = 1; i < argc; i++) {
    void *h = dlopen(argv[i], RTLD_NOW | RTLD_LOCAL);
    if (h) dlclose(h);
    else fprintf(stderr, "%s\n", dlerror());
}
```

Go+cgo is massive overkill. The entire helper in C is approximately 80 lines including JSON output.

### Recommendation

**For MVP, Go+cgo is acceptable** to avoid adding a C build requirement. However, the design should note:

1. **C is the natural choice** for this task - direct API access, no runtime overhead, minimal attack surface
2. **Binary size matters for trust**: A 10KB C binary is trivially auditable; a 5MB Go binary is not
3. **Cross-compilation with musl-gcc or zig cc** is straightforward for C and produces fully static binaries

If CGO cross-compilation proves problematic (likely), pivot to C rather than Rust. The code is too simple to justify Rust's toolchain dependency.

---

## 2. dlopen/dlerror Nuances

### 2.1 dlerror() Thread Safety

**Issue**: The design's code calls `C.dlerror()` after `C.dlopen()`, which is correct, but there's a subtle issue.

```go
if handle == nil {
    result.OK = false
    result.Error = C.GoString(C.dlerror())  // Problem here
    exitCode = 1
}
```

**Problem**: `dlerror()` returns a pointer to a static buffer that may be overwritten by subsequent calls. If the Go runtime schedules a GC or cgo callback between `dlopen` failing and `dlerror` being called, another thread could clobber the error string.

**Mitigation**: This is actually fine in this design because:
- The helper is single-threaded (sequential loop)
- No goroutines are spawned
- The `C.GoString()` copies the string immediately

However, the design should explicitly document: **"The helper MUST remain single-threaded to preserve dlerror() semantics."**

### 2.2 dlerror() Must Be Called After Every dlopen

**Issue**: Per POSIX, `dlerror()` returns the error from the most recent `dlopen/dlsym/dlclose` call, but calling `dlerror()` also **clears** the error state.

The design's code is correct but should be explicit:

```c
// Correct pattern: always call dlerror() after dlopen(), even on success
void *handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
char *error = dlerror();  // Clear error state
if (handle == NULL) {
    // Use error string
}
```

The Go code doesn't call `dlerror()` on success, which could leave stale error state. While not a bug in the current sequential implementation, it's fragile.

**Recommendation**: Call `dlerror()` unconditionally after each `dlopen()`, discarding the result on success.

### 2.3 dlclose() Can Fail

**Issue**: The design assumes `dlclose()` always succeeds:

```go
} else {
    C.dlclose(handle)
}
```

**Reality**: `dlclose()` returns 0 on success, non-zero on failure. Failure typically means:
- The handle was invalid
- The library's destructor (`DT_FINI`) threw an exception or called `exit()`

**Recommendation**: Check `dlclose()` return value and call `dlerror()` on failure. A library that fails to unload is a red flag worth reporting.

---

## 3. RTLD_NOW | RTLD_LOCAL Flag Analysis

### Assessment

**RTLD_NOW**: Correct choice. Forces immediate symbol resolution, catching missing symbols that RTLD_LAZY would defer.

**RTLD_LOCAL**: Correct choice. Symbols are not made available for subsequent `dlsym(RTLD_DEFAULT, ...)` lookups, which prevents loaded libraries from polluting the global symbol namespace.

### Missing Consideration: RTLD_NOLOAD

For verification purposes, consider a two-phase approach:

1. First call with `RTLD_NOW | RTLD_NOLOAD` - checks if already loaded (should return NULL for unloaded library)
2. Then call with `RTLD_NOW | RTLD_LOCAL` - actual load test

This isn't strictly necessary but could detect cases where a library is already loaded by some other mechanism.

### Platform Note: RTLD_DEEPBIND (Linux-only)

On Linux, `RTLD_DEEPBIND` (since glibc 2.3.4) causes the library to prefer its own symbols over global ones. The design doesn't use this, which is correct - it would change the semantics of how the library resolves its own dependencies.

---

## 4. Platform-Specific Behaviors Not Addressed

### 4.1 macOS System Integrity Protection (SIP)

**Issue**: On macOS with SIP enabled (default since El Capitan), `DYLD_LIBRARY_PATH` and `DYLD_INSERT_LIBRARIES` are stripped for system binaries.

The design's environment sanitization is correct, but the helper binary itself needs to be in a user-writable location (which it is: `$TSUKU_HOME/tools/`). The design should note:

**macOS SIP does not affect the helper because**:
1. Helper is not a system binary
2. Helper is not in `/usr/` or `/System/`
3. Helper is not code-signed with library-validation entitlement

### 4.2 Linux: ld.so.cache Interaction

**Issue**: On Linux, `dlopen()` without an absolute path consults `/etc/ld.so.cache` (built by `ldconfig`). The design uses absolute paths, so this is fine, but worth documenting.

If a library has a `DT_NEEDED` entry without a path (e.g., `libfoo.so.1`), the dynamic linker searches:
1. `DT_RPATH` (deprecated)
2. `LD_LIBRARY_PATH`
3. `DT_RUNPATH`
4. `/etc/ld.so.cache`
5. `/lib`, `/usr/lib`

The design correctly prepends `$TSUKU_HOME/libs` to `LD_LIBRARY_PATH`, ensuring tsuku-managed dependencies are found first.

### 4.3 Linux: glibc vs musl dlopen Differences

**Issue**: The design doesn't address glibc vs musl differences.

**glibc**:
- dlerror() returns thread-local static buffer
- Supports `RTLD_NODELETE`, `RTLD_NOLOAD`, `RTLD_DEEPBIND`
- Constructor order: dependencies first, then library

**musl**:
- dlerror() returns global static buffer (thread-safe via internal lock)
- Does NOT support `RTLD_DEEPBIND`
- Simpler implementation, fewer edge cases

The design should note: **"Helper behavior is consistent across glibc and musl for the subset of dlopen functionality used."**

### 4.4 macOS: dyld3 vs dyld4

**Issue**: macOS Ventura (13.0) introduced dyld4, which has performance improvements but the same API.

More importantly, **dyld interposing** (DYLD_INSERT_LIBRARIES) behavior changed slightly in recent macOS versions. The design correctly strips this variable, but should note that even if it weren't stripped, interposed libraries cannot affect signed Apple binaries.

### 4.5 Linux: DF_1_NODELETE and DF_1_NODEFLIB

**Issue**: Some libraries are marked with `DF_1_NODELETE` flag (in `DT_FLAGS_1`), which means `dlclose()` will not actually unload them.

This is fine for verification purposes - the library "loaded successfully" is what matters. However, the design should note that memory may not be fully reclaimed for such libraries, which affects batching memory estimates.

### 4.6 Constructor Hangs

**Issue**: The 5-second timeout handles infinite loops, but constructors can also:
- Spawn threads that outlive the timeout
- Register `atexit()` handlers that run on dlclose or process exit
- Open file descriptors or sockets that leak if the process is killed

**Recommendation**: Document that the helper uses `exec.CommandContext` which sends SIGKILL after timeout, ensuring cleanup regardless of constructor behavior. However, note that shared resources (semaphores, shared memory) may be left in inconsistent state.

---

## 5. Timeout Analysis

### 5-Second Timeout Assessment

**Reasonable for most cases**:
- Simple libraries: <100ms
- Libraries with disk I/O in constructors: 100ms-1s
- Libraries initializing GPU contexts: 1-3s
- Libraries connecting to services: highly variable

**Edge cases that may legitimately exceed 5 seconds**:
- OpenGL libraries probing available displays
- Database client libraries establishing connection pools (if poorly coded)
- Libraries loading large resource files from slow storage

### Recommendation

The timeout is reasonable. However, add documentation:

1. **Timeout is per-batch, not per-library** (currently unclear in design)
2. **If a single library hangs, the entire batch fails** - the design mentions "retry batch in smaller chunks" which is correct
3. **Consider an environment variable for override**: `TSUKU_DLTEST_TIMEOUT=10s` for users with legitimately slow libraries

---

## 6. Additional Technical Concerns

### 6.1 Symbol Visibility and -fvisibility=hidden

**Issue**: If the helper is built with `-fvisibility=hidden` (common for shared libraries but unusual for executables), it shouldn't affect dlopen behavior. Just noting this is a non-issue.

### 6.2 Position-Independent Executable (PIE)

**Issue**: The helper should be built as PIE for ASLR benefits. Go+cgo defaults to PIE on modern systems. A C implementation should explicitly use `-fPIE -pie`.

### 6.3 Stack Protector

**Issue**: For a security-sensitive helper, build with `-fstack-protector-strong`. Go includes this by default; a C implementation should specify it.

### 6.4 Fortify Source

**Issue**: A C implementation should use `-D_FORTIFY_SOURCE=2` for additional buffer overflow protection.

### 6.5 RELRO and Immediate Binding

**Issue**: A C implementation should use `-Wl,-z,relro,-z,now` for full RELRO, making the GOT read-only after startup. This mitigates GOT overwrite attacks from malicious libraries.

---

## 7. Recommendations Summary

### Critical (Must Address)

1. **Document single-threaded requirement** for dlerror() safety
2. **Check dlclose() return value** and report failures
3. **Call dlerror() after successful dlopen()** to clear error state

### Important (Should Address)

4. **Add glibc/musl compatibility note** to platform differences
5. **Document timeout is per-batch** behavior
6. **Consider TSUKU_DLTEST_TIMEOUT environment variable** for edge cases
7. **Note DF_1_NODELETE libraries** don't actually unload

### Minor (Nice to Have)

8. **If pivoting to C**: Use `-fPIE -pie -fstack-protector-strong -D_FORTIFY_SOURCE=2 -Wl,-z,relro,-z,now`
9. **Document SIP implications** for macOS
10. **Consider RTLD_NOLOAD** pre-check for diagnostics

---

## 8. Conclusion

The design is solid and demonstrates good understanding of the problem space. The Go+cgo choice is pragmatic for MVP velocity. The main gaps are around dlerror() edge cases, dlclose() error handling, and platform-specific documentation.

The 5-second timeout is reasonable, and the batching strategy is well thought out. The security considerations are thorough.

**Overall assessment**: Approve with minor modifications. The design is ready for implementation with the critical items addressed.
