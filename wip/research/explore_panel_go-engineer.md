# Go Systems Engineer Review: dlopen Load Testing Design

**Reviewer Role:** Go systems engineer with expertise in cgo, cross-compilation, and production Go deployments
**Design Document:** `docs/designs/DESIGN-library-verify-dlopen.md`

---

## Executive Summary

The design is well-considered and follows tsuku's established patterns. However, it underestimates the complexity of CGO cross-compilation for production releases. The Go+cgo choice for MVP is reasonable, but the implementation approach in goreleaser needs significant revision. The cgo code is mostly correct but has minor issues. I recommend either committing to platform-specific CI runners from the start, or choosing Rust/C to avoid cross-compilation pain entirely.

---

## 1. Is Go+cgo the Right Choice?

### Real-World CGO Cross-Compilation Pain Points

The design's "Language Choice" table is accurate but undersells the CGO cross-compilation problems:

**Problem 1: CGO cross-compilation doesn't "just work"**

```bash
# This fails without a C cross-compiler toolchain:
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build ./cmd/tsuku-dltest
# Error: cgo: C compiler "gcc" not found
```

Unlike pure Go, CGO requires a C compiler for the target platform. On Ubuntu building for linux/arm64, you need `aarch64-linux-gnu-gcc`. On macOS building for Linux, you need a Linux cross-toolchain.

**Problem 2: glibc version binding**

When you build with `CGO_ENABLED=1`, the resulting binary is linked against the build system's glibc. Running on an older system fails:

```
./tsuku-dltest: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.34' not found
```

The design mentions this for the main tsuku binary but doesn't address it for tsuku-dltest. Since the helper calls `dlopen()`, it must link against libc--there's no static linking escape hatch here.

**Problem 3: macOS notarization requirements**

The design mentions code signing as a "future consideration" but unsigned binaries on macOS 10.15+ trigger Gatekeeper warnings that break automated workflows:

```
"tsuku-dltest" cannot be opened because the developer cannot be verified.
```

Users must manually allow the binary in System Preferences, which is unacceptable UX.

### My Assessment

| Approach | Build Complexity | Distribution Complexity | Recommendation |
|----------|------------------|-------------------------|----------------|
| Go+cgo with platform runners | Medium (4 runners) | Medium (glibc versions) | MVP acceptable |
| Go+cgo with zig cc | High (new toolchain) | Low (portable binaries) | Better long-term |
| Rust with cross-rs | Medium (cargo install) | Low (musl/static) | Best if adding toolchain is OK |
| C with zig cc | Low (simple code) | Low (tiny binaries) | Worth considering |

**My recommendation:** For MVP, use platform-specific CI runners (ubuntu-latest for linux, macos-latest for darwin). This avoids cross-compilation entirely at the cost of 4 parallel jobs. The design's goreleaser approach needs revision (see Section 4).

---

## 2. Is the CGO Code Correct and Idiomatic?

### Issues in the Design's Implementation

The cgo code in "Step 1: Helper Binary" has several issues:

**Issue 1: Missing dlerror() clearing**

```go
// Current code:
handle := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
if handle == nil {
    result.Error = C.GoString(C.dlerror())
}
```

The `dlerror()` function returns the error from the last dlopen/dlsym/dlclose call. If a previous successful dlopen left an error message (which can happen), this code might report a stale error. The fix:

```go
// Correct pattern:
C.dlerror() // Clear any stale error
handle := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
if handle == nil {
    errMsg := C.dlerror()
    if errMsg != nil {
        result.Error = C.GoString(errMsg)
    } else {
        result.Error = "dlopen failed with unknown error"
    }
}
```

**Issue 2: LDFLAGS platform differences**

```go
// Current code:
/*
#cgo LDFLAGS: -ldl
*/
```

On macOS, `-ldl` is unnecessary and may cause warnings--dlopen is part of libSystem. The portable approach:

```go
/*
#cgo linux LDFLAGS: -ldl
#cgo darwin LDFLAGS: -framework CoreFoundation
*/
```

Actually, macOS doesn't even need the CoreFoundation framework for basic dlopen. This is simpler:

```go
/*
#cgo linux LDFLAGS: -ldl
*/
```

**Issue 3: Missing RTLD_NODELETE consideration**

On Linux, `RTLD_NODELETE` prevents the library from being unloaded via dlclose. Some libraries rely on this (notably NSS modules). For verification purposes, we want clean unloads, but we should handle cases where dlclose "fails" gracefully. The current code ignores dlclose errors, which is actually fine for verification purposes.

**Issue 4: No SIGBUS/SIGSEGV protection**

If a library's initialization code causes a segfault, the helper process crashes without producing JSON output. The invoker handles this gracefully (retry logic), but we could add signal handling for better error messages:

```go
import "os/signal"

func init() {
    c := make(chan os.Signal, 1)
    signal.Notify(c, syscall.SIGBUS, syscall.SIGSEGV)
    go func() {
        sig := <-c
        fmt.Fprintf(os.Stderr, "caught signal: %v\n", sig)
        os.Exit(3)  // Distinct exit code for crashes
    }()
}
```

However, this adds complexity and the current crash handling is acceptable for MVP.

### Corrected CGO Code

```go
package main

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
*/
import "C"
import (
    "encoding/json"
    "fmt"
    "os"
    "unsafe"
)

type Result struct {
    Path  string `json:"path"`
    OK    bool   `json:"ok"`
    Error string `json:"error,omitempty"`
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, "usage: tsuku-dltest <path>...")
        os.Exit(2)
    }

    if os.Args[1] == "--version" {
        fmt.Fprintln(os.Stderr, "tsuku-dltest v1.0.0")
        os.Exit(0)
    }

    results := make([]Result, 0, len(os.Args)-1)
    exitCode := 0

    for _, path := range os.Args[1:] {
        result := Result{Path: path, OK: true}

        cpath := C.CString(path)

        // Clear any stale error before calling dlopen
        C.dlerror()

        handle := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
        C.free(unsafe.Pointer(cpath))

        if handle == nil {
            result.OK = false
            errMsg := C.dlerror()
            if errMsg != nil {
                result.Error = C.GoString(errMsg)
            } else {
                result.Error = "dlopen failed with unknown error"
            }
            exitCode = 1
        } else {
            C.dlclose(handle)
        }

        results = append(results, result)
    }

    if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
        fmt.Fprintf(os.Stderr, "failed to encode results: %v\n", err)
        os.Exit(4)
    }
    os.Exit(exitCode)
}
```

---

## 3. Go-Specific Patterns to Improve the Helper

### Pattern 1: Structured Exit Codes

The design uses exit codes 0, 1, 2. I recommend a complete enumeration:

```go
const (
    ExitSuccess      = 0 // All libraries loaded successfully
    ExitLoadFailure  = 1 // At least one library failed to load
    ExitUsageError   = 2 // Invalid arguments
    ExitCrash        = 3 // Signal caught (SIGBUS, SIGSEGV)
    ExitInternalErr  = 4 // JSON encoding failed or other internal error
)
```

### Pattern 2: Build Tags for Platform-Specific Code

If the helper grows, use build tags to separate platform-specific implementations:

```
cmd/tsuku-dltest/
  main.go           // Shared entry point and JSON handling
  dlopen_linux.go   // Linux-specific dlopen wrapper
  dlopen_darwin.go  // macOS-specific dlopen wrapper
```

For the current minimal implementation, this is overkill.

### Pattern 3: Error Wrapping with %w

The invoker's error handling in the design is good, but ensure consistent use of `%w` for error wrapping throughout:

```go
return nil, fmt.Errorf("failed to parse helper output: %w (raw: %q)", err, stdout.Bytes())
```

### Pattern 4: Context-Aware Cancellation

The design's `invokeHelper` function uses `exec.CommandContext` correctly. One enhancement: check for context cancellation before starting:

```go
func invokeHelper(ctx context.Context, paths []string, tsukuHome string) ([]DlopenResult, error) {
    // Fast path: already cancelled
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... rest of implementation
}
```

### Pattern 5: Streaming JSON for Large Batches

For very large batches, streaming JSON (one object per line, NDJSON) would reduce memory pressure:

```go
// Each result on its own line:
{"path":"/path/to/lib1.so","ok":true}
{"path":"/path/to/lib2.so","ok":false,"error":"missing symbol"}
```

This is a future optimization; the current JSON array is fine for batches of 50.

---

## 4. Goreleaser Configuration for CGO Binaries

### The Problem

The design's goreleaser config **will not work** as written:

```yaml
builds:
  - id: tsuku-dltest
    env:
      - CGO_ENABLED=1
    goos: [linux, darwin]
    goarch: [amd64, arm64]
```

Goreleaser runs on a single host (ubuntu-latest in the current workflow). With `CGO_ENABLED=1`:
- linux/amd64: Works (native)
- linux/arm64: Fails (needs aarch64 cross-compiler)
- darwin/amd64: Fails (needs osxcross)
- darwin/arm64: Fails (needs osxcross + arm64 target)

### Solution A: Matrix Build with Platform Runners

Split the helper build into a matrix job with native compilation:

```yaml
# .github/workflows/release.yml
jobs:
  build-dltest:
    strategy:
      matrix:
        include:
          - os: ubuntu-latest
            goos: linux
            goarch: amd64
          - os: ubuntu-24.04-arm64
            goos: linux
            goarch: arm64
          - os: macos-13
            goos: darwin
            goarch: amd64
          - os: macos-latest  # M1/M2
            goos: darwin
            goarch: arm64
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Build helper
        run: |
          CGO_ENABLED=1 go build -o tsuku-dltest-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/tsuku-dltest
      - uses: actions/upload-artifact@v4
        with:
          name: dltest-${{ matrix.goos }}-${{ matrix.goarch }}
          path: tsuku-dltest-*

  release:
    needs: build-dltest
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          pattern: dltest-*
          merge-multiple: true
      - name: Run GoReleaser (main binary only)
        # ... existing goreleaser for CGO_ENABLED=0 main binary
      - name: Upload helper binaries
        # ... attach downloaded artifacts to release
```

This is the most reliable approach but adds workflow complexity.

### Solution B: Zig as C Compiler

Use Zig for hermetic cross-compilation:

```yaml
# .goreleaser.yaml
builds:
  - id: tsuku-dltest
    main: ./cmd/tsuku-dltest
    binary: tsuku-dltest-{{ .Os }}-{{ .Arch }}
    env:
      - CGO_ENABLED=1
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    overrides:
      - goos: linux
        goarch: amd64
        env:
          - CC=zig cc -target x86_64-linux-gnu
      - goos: linux
        goarch: arm64
        env:
          - CC=zig cc -target aarch64-linux-gnu
      - goos: darwin
        goarch: amd64
        env:
          - CC=zig cc -target x86_64-macos
      - goos: darwin
        goarch: arm64
        env:
          - CC=zig cc -target aarch64-macos
```

Workflow addition:
```yaml
- name: Install zig
  uses: goto-bus-stop/setup-zig@v2
  with:
    version: 0.11.0
```

This is elegant but I've seen edge cases with zig+darwin targets. Test thoroughly before relying on it.

### Solution C: Separate Repository for Helper

Build the helper in a separate repo with its own release workflow. The main tsuku repo references helper releases via checksums.

Pros:
- Clean separation of concerns
- Helper can have independent release cadence
- No CGO in main repo

Cons:
- Two repos to manage
- Version coordination complexity

### My Recommendation

**For MVP:** Solution A (matrix build with platform runners). It's verbose but reliable.

**For future:** Consider Solution B (Zig) if the matrix approach becomes a maintenance burden. The tsuku repo already has some Zig exploration in `scripts/test-zig-cc.sh`.

---

## 5. Testing Strategies for CGO Code

### Unit Tests

CGO code is testable with `go test` as long as `CGO_ENABLED=1`. Structure tests to avoid actual library loading where possible:

```go
// dltest_test.go
package main

import (
    "bytes"
    "encoding/json"
    "os/exec"
    "testing"
)

func TestHelperParseArgs(t *testing.T) {
    // Test argument parsing without invoking dlopen
    // This requires refactoring main() to be testable
}

func TestJSONOutput(t *testing.T) {
    // Build and run the helper with a known-good library
    cmd := exec.Command("./tsuku-dltest", "/lib/x86_64-linux-gnu/libc.so.6")
    out, err := cmd.Output()
    if err != nil {
        t.Fatalf("helper failed: %v", err)
    }

    var results []Result
    if err := json.Unmarshal(out, &results); err != nil {
        t.Fatalf("failed to parse JSON: %v", err)
    }

    if len(results) != 1 || !results[0].OK {
        t.Errorf("unexpected result: %+v", results)
    }
}
```

### Integration Tests

Test the full verification flow with real libraries. Use `testdata/` fixtures:

```
testdata/libraries/
  valid/
    libtest.so          # Minimal valid ELF shared object
  invalid/
    missing-symbol.so   # Depends on undefined symbol
    corrupt-header.so   # Invalid ELF header (for Level 1)
```

Creating test libraries requires a C compiler. Include a Makefile:

```makefile
# testdata/libraries/Makefile
all: valid/libtest.so invalid/missing-symbol.so

valid/libtest.so: valid/libtest.c
    $(CC) -shared -fPIC -o $@ $<

invalid/missing-symbol.so: invalid/missing-symbol.c
    $(CC) -shared -fPIC -o $@ $< || true  # May fail, that's OK
```

### CI Test Configuration

```yaml
# In .github/workflows/test.yml
dltest-tests:
  runs-on: ${{ matrix.os }}
  strategy:
    matrix:
      os: [ubuntu-latest, macos-latest]
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
    - name: Build helper
      run: CGO_ENABLED=1 go build -o tsuku-dltest ./cmd/tsuku-dltest
    - name: Build test libraries
      run: make -C testdata/libraries
    - name: Run helper tests
      run: go test -v ./cmd/tsuku-dltest/...
    - name: Run integration tests
      run: |
        export TSUKU_DLTEST_PATH=./tsuku-dltest
        go test -v -tags=integration ./internal/verify/...
```

### Fuzzing

The helper takes file paths as input, which limits fuzzing surface. However, the JSON parsing in the invoker is fuzzable:

```go
// internal/verify/dltest_fuzz_test.go
//go:build go1.18

package verify

import "testing"

func FuzzParseHelperOutput(f *testing.F) {
    f.Add([]byte(`[{"path":"/foo","ok":true}]`))
    f.Add([]byte(`[{"path":"/foo","ok":false,"error":"test"}]`))
    f.Add([]byte(`[]`))
    f.Add([]byte(`garbage`))

    f.Fuzz(func(t *testing.T, data []byte) {
        // Should not panic
        _, _ = parseHelperOutput(data)
    })
}
```

### Test with ASAN (Address Sanitizer)

For catching memory issues in the CGO code:

```bash
CGO_CFLAGS="-fsanitize=address" \
CGO_LDFLAGS="-fsanitize=address" \
go build -o tsuku-dltest ./cmd/tsuku-dltest

./tsuku-dltest /lib/x86_64-linux-gnu/libc.so.6
```

This catches buffer overflows, use-after-free, etc. Not critical for this simple code but good practice.

---

## Additional Observations

### Helper Binary Size

The design estimates "~5MB" for Go+cgo. In my experience:

- Pure Go with `CGO_ENABLED=0`: ~3MB (stripped)
- Go with `CGO_ENABLED=1` (dynamically linked): ~4MB
- Go with `CGO_ENABLED=1` + musl static: ~8MB

The 5MB estimate is reasonable for dynamic linking.

### Recipe-Based Installation

The design proposes using tsuku's recipe system to install the helper. This is elegant but has a bootstrap problem: what if the user runs `tsuku verify` before installing anything? The helper isn't installed yet, and installing it requires... tsuku.

The design handles this with `EnsureDltest()` which calls `install.InstallTool()`. This is fine as long as the install path doesn't itself require verification (which would cause a loop).

Consider adding a safeguard:

```go
func EnsureDltest() (string, error) {
    if ensureDltestInProgress {
        return "", fmt.Errorf("circular dependency: cannot install tsuku-dltest during helper lookup")
    }
    ensureDltestInProgress = true
    defer func() { ensureDltestInProgress = false }()
    // ... rest of implementation
}
```

### Environment Sanitization

The `sanitizeEnvForHelper()` function is good but incomplete. Additional dangerous variables:

```go
dangerous := map[string]bool{
    // Linux
    "LD_PRELOAD": true,
    "LD_AUDIT": true,
    "LD_DEBUG": true,
    "LD_DEBUG_OUTPUT": true,  // Can write to arbitrary files
    "LD_PROFILE": true,       // Can write to arbitrary files
    "LD_PROFILE_OUTPUT": true,
    // macOS
    "DYLD_INSERT_LIBRARIES": true,
    "DYLD_FORCE_FLAT_NAMESPACE": true,
    "DYLD_PRINT_LIBRARIES": true,  // Spams stderr, may interfere with JSON parsing
}
```

---

## Summary of Recommendations

### Critical (Must Fix)

1. **Revise goreleaser config:** The current approach won't cross-compile CGO binaries. Use platform-specific runners (Solution A).

2. **Clear dlerror() before dlopen:** Add `C.dlerror()` call before `C.dlopen()` to clear stale errors.

3. **Platform-specific LDFLAGS:** Use `#cgo linux LDFLAGS: -ldl` instead of unconditional `-ldl`.

### Important (Should Fix)

4. **Address macOS code signing:** At minimum, document the UX impact of unsigned binaries. Consider adding code signing to the release workflow.

5. **Add exit code for internal errors:** Distinguish between load failures (exit 1) and internal errors (exit 4) for better debugging.

6. **Expand environment sanitization:** Add `LD_DEBUG_OUTPUT`, `LD_PROFILE`, `DYLD_PRINT_LIBRARIES` to the dangerous variables list.

### Nice to Have

7. **Add ASAN to CI:** Run CGO tests with address sanitizer to catch memory issues early.

8. **Consider build tags:** If helper grows, split into `dlopen_linux.go` and `dlopen_darwin.go`.

9. **Add context cancellation fast path:** Check `ctx.Done()` before starting work in `invokeHelper()`.

---

## Conclusion

The design is solid and follows tsuku's established patterns. The main risk is underestimating CGO cross-compilation complexity--the proposed goreleaser config needs revision. The cgo code is close to correct but needs the dlerror() clearing fix and platform-specific LDFLAGS.

For MVP, I recommend Go+cgo with platform-specific CI runners. This avoids cross-compilation entirely and matches what the current release.yml workflow already does for testing. If the matrix build proves too slow or flaky, consider Zig as a cross-compiler or migrating to Rust.

The helper's simplicity is a strength. Keep it minimal--resist adding features beyond the core dlopen/dlclose/dlerror loop.
