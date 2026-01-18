# Rust Systems Engineer Review: dlopen Load Testing Design

**Reviewer Role:** Rust systems engineer with expertise in FFI, cross-compilation, and safe systems programming

**Document Reviewed:** `docs/designs/DESIGN-library-verify-dlopen.md`

---

## Executive Summary

The design is sound and the architecture decisions are well-reasoned. However, the document undervalues Rust as the implementation choice and overcomplicates the Go+cgo cross-compilation story. From a systems programming perspective, Rust offers meaningful advantages for this specific use case that warrant serious consideration.

---

## 1. Would Rust Be a Better Choice for This Helper Binary?

**Yes, Rust would be a better choice.** Here's why:

### Technical Fit

The helper binary is a textbook Rust use case:
- **FFI to libc function (dlopen)**: Rust's `libc` crate provides zero-cost bindings
- **Minimal runtime requirements**: No garbage collector, predictable startup
- **Error handling with dlerror()**: Rust's `Result` type maps perfectly to success/failure semantics
- **JSON serialization**: `serde_json` is mature and compile-time optimized

### Memory Safety Concerns in the Go+cgo Version

Looking at the proposed Go+cgo implementation:

```go
cpath := C.CString(path)
handle := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
C.free(unsafe.Pointer(cpath))
```

This code has subtle issues:
1. **Manual memory management**: `C.CString` allocates, `C.free` deallocates. Easy to get wrong during refactoring.
2. **Null pointer handling**: If `path` is empty, behavior is undefined.
3. **Error state is global**: `dlerror()` returns a pointer to a statically-allocated string that may be overwritten by subsequent calls. The Go code doesn't protect against concurrent access.

In Rust, these become compile-time errors or are handled by RAII:

```rust
let c_path = CString::new(path)?;  // Returns Err if path contains null
let handle = unsafe { dlopen(c_path.as_ptr(), RTLD_NOW | RTLD_LOCAL) };
// c_path automatically freed when it goes out of scope
```

### Runtime Overhead

| Aspect | Go+cgo | Rust |
|--------|--------|------|
| Binary contains GC | Yes | No |
| Startup time | ~2-5ms (GC init) | ~0.1-0.5ms |
| Memory footprint | ~10MB baseline | ~1MB baseline |
| Deterministic cleanup | No (GC) | Yes (RAII) |

For a one-shot process these differences are minor, but they're measurable. More importantly, Rust's deterministic behavior makes debugging easier when things go wrong.

### Verdict

The document says "binary size is irrelevant for a one-time download." This is true. But the document conflates binary size with overall suitability. The real advantages of Rust are:

1. **Better FFI ergonomics** (no unsafe pointer juggling)
2. **No global error state race conditions** (dlerror wrapper)
3. **Simpler cross-compilation** (see next section)
4. **Smaller attack surface** (no Go runtime with complex internals)

---

## 2. Cross-Compilation Comparison: Rust (cross-rs) vs Go+cgo

### Go+cgo Cross-Compilation Reality

The document says "CGO cross-compilation is finicky (needs platform CC)" but understates the problem:

**What Go+cgo cross-compilation actually requires:**

For Linux targets:
- `x86_64-linux-gnu-gcc` or `aarch64-linux-gnu-gcc` toolchains
- Matching libc headers and libraries
- Setting `CC`, `CGO_ENABLED`, `GOOS`, `GOARCH` correctly

For macOS targets:
- macOS SDK (legally obtained only on macOS)
- Cross-compiling to macOS from Linux requires osxcross or similar
- Apple's license technically prohibits running macOS tools on non-Apple hardware

**Practical implication:** You need 4 CI runners (Linux x64, Linux ARM64, macOS x64, macOS ARM64) or complex cross-compilation setups.

### Rust with cross-rs

```bash
# Install cross once
cargo install cross

# Build for all 4 targets from any platform
cross build --release --target x86_64-unknown-linux-gnu
cross build --release --target aarch64-unknown-linux-gnu
cross build --release --target x86_64-apple-darwin
cross build --release --target aarch64-apple-darwin
```

**What cross-rs provides:**
- Pre-built Docker containers with all toolchains
- Automatic sysroot management
- Works from any host OS (Linux, macOS, Windows)
- Single CI runner can build all Linux targets

**Limitation:** macOS targets still require macOS runner (Apple SDK licensing). But you can build both macOS architectures from a single macOS runner, and all Linux architectures from a single Linux runner.

### CI Complexity Comparison

| Scenario | Go+cgo | Rust+cross |
|----------|--------|------------|
| Linux x64 from Linux x64 | Native | Native |
| Linux ARM64 from Linux x64 | Needs aarch64-gcc | `cross build --target aarch64-unknown-linux-gnu` |
| macOS ARM64 from Linux | Very difficult (osxcross) | Needs macOS runner |
| macOS ARM64 from macOS x64 | Needs Xcode | `cargo build --target aarch64-apple-darwin` |

**Bottom line:** Rust reduces the CI matrix from 4 native runners to 2 (one Linux, one macOS). Go+cgo effectively requires 4 runners or significant cross-compilation infrastructure.

---

## 3. Safety Concerns in the Current Design

### Thread Safety of dlerror()

The design doesn't address that `dlerror()` returns a pointer to thread-local storage that's overwritten on the next `dlerror()` or `dlopen()` call. The Go code:

```go
result.Error = C.GoString(C.dlerror())
```

This is safe *only* because the helper is single-threaded. But the design doesn't document this constraint. If someone adds goroutines later, this becomes a data race.

**Rust solution:**
```rust
// dlerror is wrapped to immediately copy the string
fn get_dlerror() -> Option<String> {
    let err = unsafe { dlerror() };
    if err.is_null() {
        None
    } else {
        Some(unsafe { CStr::from_ptr(err) }.to_string_lossy().into_owned())
    }
}
```

### Signal Handler Interaction

`dlopen()` can execute arbitrary code including installing signal handlers. If a library installs a `SIGSEGV` handler and then crashes, the behavior is undefined. The design mentions "process isolation" but doesn't note that:

1. The helper might not crash cleanly (signal handler infinite loop)
2. Timeout via `context.WithTimeout` sends `SIGKILL`, which can't be caught

This is actually well-handled by the design (SIGKILL is uncatchable), but could be documented more explicitly.

### Path Validation Edge Cases

```go
canonical, err := filepath.EvalSymlinks(p)
if err != nil || !strings.HasPrefix(canonical, filepath.Join(tsukuHome, "libs")) {
    return nil, fmt.Errorf("invalid library path: %s", p)
}
```

Edge cases:
1. What if `$TSUKU_HOME` itself is a symlink? Should resolve it first.
2. Race condition: path could be replaced between validation and dlopen.
3. Unicode normalization: paths like `/home/libs/../libs/evil.so` might bypass the check on some filesystems.

**Rust advantage:** The `std::path::Path` type has explicit canonicalization that handles these cases, and the type system prevents you from accidentally using a non-canonical path.

---

## 4. What Would a Rust Implementation Look Like?

### Project Structure

```
cmd/tsuku-dltest/
  Cargo.toml
  src/
    main.rs
    dlopen.rs
```

### Cargo.toml

```toml
[package]
name = "tsuku-dltest"
version = "1.0.0"
edition = "2021"

[dependencies]
libc = "0.2"
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"

[profile.release]
opt-level = "z"      # Optimize for size
lto = true           # Link-time optimization
strip = true         # Strip symbols
panic = "abort"      # Smaller panic handling
```

### src/main.rs

```rust
use std::env;
use std::ffi::CString;
use std::io::{self, Write};
use std::process::ExitCode;

mod dlopen;

use dlopen::DlopenResult;
use serde::Serialize;

#[derive(Serialize)]
struct Output {
    path: String,
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();

    if args.is_empty() {
        eprintln!("usage: tsuku-dltest <path>...");
        return ExitCode::from(2);
    }

    if args.len() == 1 && args[0] == "--version" {
        eprintln!("tsuku-dltest v1.0.0");
        return ExitCode::from(0);
    }

    let mut all_ok = true;
    let mut results = Vec::with_capacity(args.len());

    for path in &args {
        let result = match dlopen::try_load(path) {
            Ok(()) => Output {
                path: path.clone(),
                ok: true,
                error: None,
            },
            Err(e) => {
                all_ok = false;
                Output {
                    path: path.clone(),
                    ok: false,
                    error: Some(e),
                }
            }
        };
        results.push(result);
    }

    // Output JSON to stdout
    serde_json::to_writer(io::stdout(), &results).unwrap();
    io::stdout().flush().unwrap();

    if all_ok {
        ExitCode::from(0)
    } else {
        ExitCode::from(1)
    }
}
```

### src/dlopen.rs

```rust
use libc::{c_void, dlclose, dlerror, dlopen, RTLD_LOCAL, RTLD_NOW};
use std::ffi::{CStr, CString};
use std::ptr;

/// Attempt to load a library with dlopen, then immediately unload it.
/// Returns Ok(()) if the library loads successfully, Err(message) otherwise.
pub fn try_load(path: &str) -> Result<(), String> {
    // Clear any previous error
    unsafe { dlerror(); }

    let c_path = CString::new(path).map_err(|_| "path contains null byte".to_string())?;

    let handle = unsafe { dlopen(c_path.as_ptr(), RTLD_NOW | RTLD_LOCAL) };

    if handle.is_null() {
        // Get error message
        let err = unsafe { dlerror() };
        if err.is_null() {
            return Err("dlopen failed with unknown error".to_string());
        }
        let err_msg = unsafe { CStr::from_ptr(err) }
            .to_string_lossy()
            .into_owned();
        return Err(err_msg);
    }

    // Success - immediately close
    unsafe { dlclose(handle); }

    Ok(())
}
```

### Binary Size Expectations

With the profile settings above:
- Linux x86_64: ~400KB
- macOS ARM64: ~500KB

This is 10x smaller than Go+cgo (~5MB) and provides the same functionality.

---

## 5. Is the ~1MB vs ~5MB Binary Size Difference Meaningful?

**Not for download time, but yes for other reasons:**

### Download Impact: Negligible

- 5MB downloads in <1 second on any reasonable connection
- One-time download, cached forever
- Users are already downloading multi-MB tools

### But Size Indicates Complexity

A 5MB Go+cgo binary contains:
- Go runtime (~3MB)
- Garbage collector
- Goroutine scheduler
- Reflection metadata
- Large standard library portions

A 400KB Rust binary contains:
- Minimal startup code
- The actual dlopen logic
- serde_json (surprisingly small with LTO)

**Why this matters:**

1. **Attack surface**: More code = more potential vulnerabilities. A 5MB binary has more places for things to go wrong than a 400KB binary.

2. **Auditability**: A 400KB binary is feasible to disassemble and audit. A 5MB binary is not.

3. **Startup time**: While minor, deterministic startup makes debugging timing issues easier.

4. **Reproducibility**: Smaller binaries with fewer dependencies are easier to reproduce. Go's module system is good, but Cargo's lockfile + deterministic builds are excellent.

### Practical Recommendation

If the only goal is "make it work," Go+cgo is fine. If the goals include:
- Minimal attack surface for a security-relevant component
- Easy cross-compilation
- Long-term maintainability

Then Rust is worth the toolchain addition.

---

## Recommendations

### Strong Recommendations

1. **Consider Rust for implementation.** The cross-compilation story alone justifies it. The safety benefits are a bonus.

2. **Document the single-threaded constraint.** If using Go+cgo, explicitly document that the helper must remain single-threaded due to dlerror() thread-safety.

3. **Canonicalize $TSUKU_HOME before path validation.** The current validation has TOCTOU race conditions and symlink edge cases.

### Minor Recommendations

4. **Add a `--dry-run` flag** that validates paths exist without calling dlopen. Useful for debugging.

5. **Consider JSON Lines output** for streaming results during long batches, instead of collecting everything in memory.

6. **Pre-clear dlerror()** before each dlopen call. The Go code doesn't do this, which could cause stale errors to be reported.

### If Go+cgo Is Chosen

7. **Use `runtime.LockOSThread()`** in main() to ensure dlerror() calls are on the same OS thread as dlopen() calls.

8. **Add explicit null checks** for dlerror() return value.

9. **Consider using zig as the C compiler** for cross-compilation. `CGO_ENABLED=1 CC="zig cc"` works surprisingly well and handles cross-compilation cleanly.

---

## Conclusion

The design is architecturally sound. The helper binary pattern, JSON protocol, and batching strategy are all good choices. The security considerations are thorough.

However, the language choice deserves more weight. The document frames it as "defer to implementation time," but this understates the CI complexity difference between Go+cgo and Rust. For a security-relevant component that calls into native code, Rust's compile-time safety and superior cross-compilation story make it the better choice.

If adding Rust to the repository is truly unacceptable, Go+cgo will work. But the document should acknowledge the additional CI complexity and the dlerror() thread-safety constraint that comes with it.
