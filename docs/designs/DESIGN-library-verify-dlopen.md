---
status: Proposed
problem: Levels 1-2 of library verification can't confirm a library will actually load; only dlopen() can test this, but it requires cgo which conflicts with tsuku's static build.
decision: Use a dedicated helper binary (tsuku-dltest) with JSON protocol and batched invocation, following the existing nix-portable pattern.
rationale: This preserves tsuku's simple distribution model while providing isolated code execution, embedded checksum verification, and good performance through batching.
---

# DESIGN: dlopen Load Testing for Library Verification (Level 3)

**Status:** Proposed

## Upstream Design Reference

This design implements Level 3 (Load Test) from [DESIGN-library-verification.md](DESIGN-library-verification.md).

**Relevant sections:**
- Solution Architecture: Level 3 specification (~1ms, dlopen via helper binary)
- Helper Binary Design: Batched verification, JSON output, embedded checksums
- Security Considerations: Execution Isolation (dlopen executes library initialization code)
- Fallback Behavior: Graceful degradation when helper unavailable

## Context and Problem Statement

Levels 1-2 of tsuku's library verification can identify structural problems (corrupt headers, missing dependencies) but can't definitively answer "will this library actually load?" A library might pass header validation and have all dependencies resolved, yet still fail when the dynamic linker attempts to load it:

- Linker version incompatibilities not caught by PT_INTERP validation
- Corrupted code sections (header parsing doesn't read code)
- Runtime-only link failures (symbol versioning, TLS issues)

The only way to confirm a library will work is to ask the dynamic linker to load it. The `dlopen()` system call does exactly this—it performs all the same resolution and relocation that happens when a process starts.

**Why a helper binary?**

Tsuku is built with `CGO_ENABLED=0` to avoid C library dependencies and simplify distribution. But `dlopen()` is a C library function that requires cgo. Rather than compromise the main binary, we use a small helper (`tsuku-dltest`) that's installed on first use.

**Security implications:**

When `dlopen()` loads a library, it executes initialization code:
- ELF: `.init` sections, `DT_INIT` function
- Mach-O: `__mod_init_func` sections
- Both: C++ global constructors, `__attribute__((constructor))` functions

This means Level 3 verification executes code from the libraries being verified. The design must address this through process isolation, user opt-out, and trust chain verification.

### Scope

**In scope:**
- Helper binary architecture and JSON protocol
- Trust chain verification (embedded checksums)
- Invocation protocol with timeout handling
- Fallback behavior when helper is unavailable
- User opt-out via `--skip-dlopen`
- Error handling and reporting

**Out of scope:**
- Levels 1, 2, or 4 of verification (covered by their own designs)
- Verification output formatting (covered by umbrella design)
- Installation flow for the helper (follows existing tsuku patterns)
- macOS code signing (documented as future consideration)

## Decision Drivers

- **Security**: Code execution during verification must be isolated and opt-out-able
- **Trust chain**: Helper binary must be verifiable without external network requests
- **Performance**: Batching must reduce overhead for libraries with many files
- **Graceful degradation**: Missing helper shouldn't block verification entirely
- **Cross-platform**: Must work on Linux (glibc/musl) and macOS (arm64/x86_64)
- **Debuggability**: Errors from dlopen must surface clearly to users

## Implementation Context

### Why Not CGO_ENABLED=1 for Main Tsuku?

An alternative to a helper binary is enabling cgo in the main tsuku build. This would simplify the design by eliminating the need for a separate binary. However, it would have significant distribution consequences:

| Concern | CGO_ENABLED=0 (current) | CGO_ENABLED=1 |
|---------|-------------------------|---------------|
| **Glibc version** | N/A (static) | Binary tied to build system's glibc |
| **Alpine/musl support** | Works out of box | Requires separate musl build |
| **Cross-compilation** | Simple (`GOOS=linux GOARCH=amd64`) | Requires C cross-compiler toolchain |
| **Binary portability** | Runs anywhere | "GLIBC_2.XX not found" errors on older systems |
| **Build infrastructure** | Standard Go | Needs musl-gcc or zig for portable builds |

The current tsuku binary is built with `CGO_ENABLED=0` and runs on any Linux system regardless of libc version. Switching to cgo would either limit portability or require complex build infrastructure (musl static linking, zig cross-compiler).

**Verdict**: The helper binary approach keeps the main tsuku distribution simple while allowing cgo-dependent functionality in a separate, platform-specific binary.

### Existing Helper Binary Pattern

Tsuku already uses the helper binary pattern for nix-portable (`internal/actions/nix_portable.go`). Key patterns to follow:

**Trust chain:**
```go
var nixPortableChecksums = map[string]string{
    "amd64": "b409c55904c909ac3aeda3fb1253319f86a89ddd1ba31a5dec33d4a06414c72a",
    "arm64": "af41d8defdb9fa17ee361220ee05a0c758d3e6231384a3f969a314f9133744ea",
}
```

Checksums are embedded in source code, requiring a code change (and review) to update the helper.

**Installation flow:**
1. Check if helper exists at expected path
2. If not, download from GitHub releases
3. Verify SHA256 checksum before execution
4. Atomic rename to prevent partial writes
5. File locking prevents concurrent download races

**Version tracking:**
```go
versionPath := filepath.Join(internalDir, "version")
if string(versionData) == nixPortableVersion {
    return path, nil  // Already have correct version
}
```

### dlopen Semantics

The `dlopen()` function performs these steps:
1. Locate the library file (or use provided path)
2. Map the library into memory
3. **Execute initialization code** (`.init`, constructors)
4. Return handle for `dlsym()` lookups

Step 3 is the security concern—initialization code runs with the calling process's privileges. For verification, we only need to know if step 3 succeeds, then immediately `dlclose()`.

**RTLD_LAZY vs RTLD_NOW:**
- `RTLD_LAZY`: Defer symbol resolution until used (faster, but may hide missing symbols)
- `RTLD_NOW`: Resolve all symbols immediately (catches more errors)

For verification, `RTLD_NOW` is preferred to catch symbol resolution failures.

### Platform Differences

**Linux:**
- `dlopen()` via `-ldl` linkage
- Error messages from `dlerror()`
- Supports `RTLD_LAZY`, `RTLD_NOW`, `RTLD_GLOBAL`, `RTLD_LOCAL`

**macOS:**
- `dlopen()` built into libSystem (no separate libdl)
- Same API as Linux
- Handles dyld shared cache transparently

### Anti-patterns to Avoid

- **Shelling out to ldd**: Security risk—ldd executes the binary's `.init` sections
- **Trying dlopen in tsuku directly**: Would require CGO_ENABLED=1
- **Unbounded batching**: Process memory grows with loaded libraries; need batch size limits
- **Ignoring dlerror()**: The error string contains the actual failure reason

## Considered Options

### Decision 1: How to Invoke dlopen

#### Option 1A: Dedicated Helper Binary (tsuku-dltest)

Build a minimal Go+cgo binary that accepts library paths and returns dlopen results. Distributed as GitHub release assets with checksums embedded in tsuku source.

**Pros:**
- Full control over behavior, error messages, and output format
- Follows existing nix-portable pattern
- Can be versioned and updated with tsuku releases
- No external dependencies at runtime

**Cons:**
- Requires building and distributing 4 binaries (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64)
- Users must download helper on first use
- Helper binary updates require tsuku release

#### Option 1B: Python ctypes Fallback

Use Python's ctypes module to call dlopen if available, falling back to skipping Level 3 if Python isn't installed.

**Pros:**
- No binary distribution needed
- Python is commonly available
- Already has cross-platform dlopen bindings

**Cons:**
- Python version differences (2 vs 3, ctypes API changes)
- Python may not be installed in minimal environments
- Slower startup time (~100ms vs ~5ms)
- Less control over error handling

#### Option 1C: CGO_ENABLED=1 for Main Tsuku

Enable cgo in the main tsuku build and call dlopen directly.

**Pros:**
- Simplest implementation (no IPC, no helper binary)
- No additional downloads or trust chain concerns
- Verification always available

**Cons:**
- Would require musl static linking or per-glibc-version builds
- Cross-compilation becomes complex (needs C cross-compiler)
- Alpine/older Linux support would need separate builds
- Increases main binary size and build complexity

### Decision 2: Communication Protocol

#### Option 2A: JSON over stdout

Helper accepts paths as arguments, outputs JSON array of results to stdout.

```json
[
  {"path": "/path/to/lib.so", "ok": true},
  {"path": "/path/to/broken.so", "ok": false, "error": "undefined symbol: foo"}
]
```

**Pros:**
- Human-readable for debugging
- Easy to parse in Go (`encoding/json`)
- Extensible (can add fields without breaking compatibility)
- Standard pattern for CLI tools

**Cons:**
- Slightly larger output than binary format
- JSON parsing adds ~100us overhead (negligible)

#### Option 2B: Exit Codes Only

Exit 0 if all libraries load, non-zero otherwise. No detailed output.

**Pros:**
- Simplest possible implementation
- No parsing needed

**Cons:**
- Can't identify which library failed in a batch
- Can't report error messages
- Not debuggable

#### Option 2C: Line-Based Text

One line per library: `OK /path/to/lib.so` or `FAIL /path/to/lib.so: error message`

**Pros:**
- Human-readable
- Streamable (can process as output arrives)

**Cons:**
- Fragile parsing (what if error message contains newlines?)
- Less structured than JSON
- Harder to extend

### Decision 3: Batching Strategy

#### Option 3A: Multiple Paths Per Invocation

Accept multiple library paths as command-line arguments, verify all in one process.

**Pros:**
- Reduces process spawn overhead (2-5ms per invocation)
- Much faster for libraries with many files (Qt: ~50 dylibs)
- Single IPC round-trip

**Cons:**
- If one library crashes the helper, partial results are lost
- Memory usage grows with batch size (libraries stay loaded until dlclose)
- Need to decide batch size limit

#### Option 3B: One Path Per Invocation

Spawn helper once per library file.

**Pros:**
- Maximum isolation (crash affects only one result)
- Simple implementation
- Predictable memory usage

**Cons:**
- Slow for many files (50 files × 5ms = 250ms vs ~10ms batched)
- More process spawning overhead

### Evaluation Against Decision Drivers

| Option | Security | Trust Chain | Performance | Graceful Degradation | Cross-Platform | Debuggability |
|--------|----------|-------------|-------------|----------------------|----------------|---------------|
| **1A: Helper binary** | Good (isolated) | Good (embedded checksums) | Good | Good (skip if unavailable) | Good (4 builds) | Good |
| **1B: Python ctypes** | Fair (Python risk) | N/A | Poor (~100ms startup) | Fair (Python may not exist) | Good | Fair |
| **1C: CGO in tsuku** | Good | N/A | Excellent | Excellent (always available) | Poor (build complexity) | Good |
| **2A: JSON stdout** | N/A | N/A | Good | N/A | Good | Excellent |
| **2B: Exit codes** | N/A | N/A | Excellent | N/A | Good | Poor |
| **2C: Line-based** | N/A | N/A | Good | N/A | Good | Fair |
| **3A: Batched** | Fair (batch crash) | N/A | Excellent | N/A | Good | Good |
| **3B: Per-file** | Excellent | N/A | Poor | N/A | Good | Good |

### Uncertainties

- **Batch size limits**: Need to respect ARG_MAX (~128KB-2MB depending on OS). With 256-byte average paths, that's 500-8000 libraries per batch. Will use conservative default (50 libraries) with option to tune.
- **macOS Gatekeeper**: Unsigned binaries may trigger security warnings on macOS. Code signing may be needed for good UX.
- **Helper binary size**: Unknown final size. Cgo adds overhead but should be <5MB.
- **dlopen memory behavior**: Some libraries may not fully unload on dlclose, causing memory growth across batches.
- **Environment inheritance**: Unclear whether helper should inherit tsuku's environment or run in a clean environment. Libraries with `$ORIGIN` dependencies need the former.
- **Timeout edge cases**: A library taking 4.9 seconds to initialize will succeed, but is that good? Users may want tighter timeouts for faster feedback.

## Decision Outcome

**Chosen: 1A (Helper Binary) + 2A (JSON Protocol) + 3A (Batched)**

A dedicated helper binary with JSON output and batched invocation provides the best balance of security, performance, and debuggability while maintaining tsuku's simple distribution model.

### Rationale

**Helper binary (1A) chosen because:**
- Preserves tsuku's `CGO_ENABLED=0` build simplicity and portability
- Follows proven nix-portable pattern already in codebase
- Trust chain verification through embedded checksums
- Process isolation comes free (separate process means crashes don't affect tsuku)

**Python ctypes (1B) rejected because:**
- Python availability varies (minimal containers, CI environments)
- ~100ms startup time adds up for many libraries
- Version differences create support burden

**CGO in main tsuku (1C) rejected because:**
- Would require musl static linking or zig cross-compiler for portability
- Complicates the main binary build for a single feature
- Existing CGO_ENABLED=0 policy serves the project well

**JSON protocol (2A) chosen because:**
- Extensible without breaking compatibility (can add fields)
- Human-readable for debugging failed verifications
- Standard Go parsing with `encoding/json`

**Batching (3A) chosen because:**
- 50 libraries × 5ms = 250ms unbatched vs ~10ms batched
- Crash risk acceptable with reasonable batch limits
- Memory issues mitigated by dlclose after each library

### Trade-offs Accepted

- **Helper download on first use**: Users need network access to download ~1-5MB binary. Acceptable because verification is not a critical path and tsuku already downloads tools.
- **Helper binary maintenance**: Requires building 4 platform-specific binaries. Acceptable because the helper is small and changes rarely.
- **Batch crash risk**: If a library crashes the helper, partial results are lost. Acceptable because crashes are rare and can be retried with smaller batches.
- **5-second timeout**: Some legitimate libraries may take longer to initialize. Acceptable because users can retry with `--skip-dlopen` if needed.

## Solution Architecture

### Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    tsuku verify <library>                        │
│                    [--skip-dlopen] [--integrity]                 │
├─────────────────────────────────────────────────────────────────┤
│  Level 1: Header Validation (Tier 1 design)                      │
│  Level 2: Dependency Check (Tier 2 design)                       │
├─────────────────────────────────────────────────────────────────┤
│  Level 3: Load Test [this design]                                │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ 1. Check if helper available                             │    │
│  │    └─ If not: download, verify checksum, install         │    │
│  │ 2. Batch library paths (max 50 per invocation)           │    │
│  │ 3. Invoke helper with timeout (5 seconds)                │    │
│  │ 4. Parse JSON results, report per-library status         │    │
│  └─────────────────────────────────────────────────────────┘    │
├─────────────────────────────────────────────────────────────────┤
│  Level 4: Integrity Check (Tier 4 design)                        │
└─────────────────────────────────────────────────────────────────┘

                              │
                              │ exec with timeout
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    tsuku-dltest helper                           │
│                    (separate CGO_ENABLED=1 binary)               │
├─────────────────────────────────────────────────────────────────┤
│  Input: library paths as command-line arguments                  │
│  Output: JSON array to stdout                                    │
│                                                                 │
│  For each path:                                                  │
│    1. dlopen(path, RTLD_NOW | RTLD_LOCAL)                       │
│    2. If success: dlclose(handle)                               │
│    3. If failure: capture dlerror() message                     │
│    4. Append result to output array                             │
│                                                                 │
│  Exit codes:                                                     │
│    0: All libraries loaded successfully                          │
│    1: At least one library failed                               │
│    2: Usage error (no paths provided)                           │
└─────────────────────────────────────────────────────────────────┘
```

### Helper Binary Protocol

**Input**: Library paths as command-line arguments

```bash
tsuku-dltest /path/to/lib1.so /path/to/lib2.so /path/to/lib3.so
```

**Output**: JSON array to stdout

```json
[
  {"path": "/path/to/lib1.so", "ok": true},
  {"path": "/path/to/lib2.so", "ok": true},
  {"path": "/path/to/lib3.so", "ok": false, "error": "libfoo.so: cannot open shared object file"}
]
```

**JSON Schema**:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "array",
  "items": {
    "type": "object",
    "required": ["path", "ok"],
    "properties": {
      "path": {"type": "string", "description": "Absolute path to library"},
      "ok": {"type": "boolean", "description": "True if dlopen succeeded"},
      "error": {"type": "string", "description": "Error message from dlerror() if ok=false"}
    }
  }
}
```

**Exit codes**:
- `0`: All libraries loaded successfully (every result has `ok: true`)
- `1`: At least one library failed to load (at least one `ok: false`)
- `2`: Usage error (e.g., no paths provided, invalid arguments)

**Version protocol**: Helper outputs version on stderr when run with `--version`:

```bash
$ tsuku-dltest --version
tsuku-dltest v1.0.0
```

### Trust Chain Verification

Checksums are embedded in tsuku source code:

```go
// internal/verify/dltest.go
var dltestVersion = "v1.0.0"

var dltestChecksums = map[string]string{
    "linux-amd64":  "sha256:abc123...",
    "linux-arm64":  "sha256:def456...",
    "darwin-amd64": "sha256:789abc...",
    "darwin-arm64": "sha256:cde012...",
}
```

**Verification flow**:

1. Acquire exclusive file lock on `$TSUKU_HOME/.dltest/.lock` (prevents race conditions)
2. Check if `$TSUKU_HOME/.dltest/tsuku-dltest` exists
3. If exists, verify version file matches expected version
4. If version mismatch or missing: download from GitHub releases
5. Verify SHA256 checksum against embedded value
6. If checksum mismatch: error, do not execute
7. Atomic rename to install location (prevents partial writes)
8. Release file lock
9. Execute helper

**File locking**: Following the nix-portable pattern, file locking prevents race conditions when multiple tsuku processes attempt to download the helper simultaneously.

**Why embedded checksums**: Updating the helper requires changing tsuku source code, which goes through code review. This prevents a compromised release server from pushing malicious helpers without review.

### Invocation Protocol

```go
func invokeHelper(ctx context.Context, paths []string, tsukuHome string) ([]DlopenResult, error) {
    // Validate paths are within $TSUKU_HOME/libs/
    for _, p := range paths {
        canonical, err := filepath.EvalSymlinks(p)
        if err != nil || !strings.HasPrefix(canonical, filepath.Join(tsukuHome, "libs")) {
            return nil, fmt.Errorf("invalid library path: %s", p)
        }
    }

    helperPath, err := ensureHelper()
    if err != nil {
        return nil, fmt.Errorf("helper unavailable: %w", err)
    }

    // Create context with timeout
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, helperPath, paths...)

    // Build sanitized environment
    cmd.Env = sanitizeEnvForHelper(tsukuHome)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err = cmd.Run()

    if ctx.Err() == context.DeadlineExceeded {
        return nil, fmt.Errorf("helper timed out after 5 seconds")
    }

    // Parse JSON even on non-zero exit (may have partial results)
    var results []DlopenResult
    if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
        return nil, fmt.Errorf("failed to parse helper output: %w", err)
    }

    return results, nil
}

// sanitizeEnvForHelper creates a safe environment for the helper.
// Strips dangerous loader variables while preserving necessary paths.
func sanitizeEnvForHelper(tsukuHome string) []string {
    // Variables that allow code injection - MUST be stripped
    dangerous := map[string]bool{
        "LD_PRELOAD": true, "LD_AUDIT": true, "LD_DEBUG": true,
        "DYLD_INSERT_LIBRARIES": true, "DYLD_FORCE_FLAT_NAMESPACE": true,
    }

    var env []string
    for _, e := range os.Environ() {
        key := strings.SplitN(e, "=", 2)[0]
        if !dangerous[key] {
            env = append(env, e)
        }
    }

    // Ensure tsuku libs are in search path
    libsDir := filepath.Join(tsukuHome, "libs")
    env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s:%s",
        libsDir, os.Getenv("LD_LIBRARY_PATH")))
    env = append(env, fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s",
        libsDir, os.Getenv("DYLD_LIBRARY_PATH")))

    return env
}
```

**Environment sanitization**: The helper runs with a sanitized environment:
- **Stripped**: `LD_PRELOAD`, `LD_AUDIT`, `LD_DEBUG`, `DYLD_INSERT_LIBRARIES`, `DYLD_FORCE_FLAT_NAMESPACE` (code injection vectors)
- **Preserved**: `LD_LIBRARY_PATH`, `DYLD_LIBRARY_PATH` (needed for dependency resolution)
- **Added**: `$TSUKU_HOME/libs` prepended to library paths

**Path validation**: All library paths are canonicalized via `filepath.EvalSymlinks()` and verified to be within `$TSUKU_HOME/libs/`. This prevents:
- Path traversal attacks (`../../../etc/malicious.so`)
- Symlink escapes pointing outside the install directory

### Batch Size Limits

Default batch size: 50 libraries per invocation

**Rationale**:
- ARG_MAX is typically 128KB-2MB
- 50 paths × 256 bytes average = 12.8KB (well under limit)
- 50 libraries × ~200KB average memory = 10MB (acceptable)
- If batch crashes, max loss is 50 results (retryable)

For libraries with >50 files, split into multiple batches and aggregate results.

### Fallback Behavior

| Scenario | Behavior |
|----------|----------|
| Helper not installed | Attempt download; if fails, skip Level 3 with warning |
| Checksum mismatch | Error: "helper binary checksum verification failed, refusing to execute" |
| Helper times out | Error: "load test timed out" for affected batch |
| Helper crashes | Retry batch in smaller chunks; if still fails, report crash |
| `--skip-dlopen` flag | Skip Level 3 entirely, no warning |
| Network unavailable | Skip Level 3 with warning: "helper unavailable, skipping load test" |

**Warning message format**:

```
Warning: tsuku-dltest helper not available, skipping load test
  Run 'tsuku install tsuku-dltest' to enable full verification
```

## Implementation Approach

### Step 1: Helper Binary

Create `cmd/tsuku-dltest/main.go`:

```go
package main

/*
#cgo LDFLAGS: -ldl
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
        handle := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
        C.free(unsafe.Pointer(cpath))

        if handle == nil {
            result.OK = false
            result.Error = C.GoString(C.dlerror())
            exitCode = 1
        } else {
            C.dlclose(handle)
        }

        results = append(results, result)
    }

    json.NewEncoder(os.Stdout).Encode(results)
    os.Exit(exitCode)
}
```

**Build**: `CGO_ENABLED=1 go build -o tsuku-dltest ./cmd/tsuku-dltest`

### Step 2: Trust Chain Module

Create `internal/verify/dltest.go`:

- `EnsureDltest()` - download and verify helper if needed
- `ResolveDltest()` - return path if installed
- Embedded checksums and version constant

### Step 3: Invocation Module

Add to `internal/verify/dltest.go`:

- `InvokeDltest(paths []string) ([]DlopenResult, error)`
- Batch splitting logic
- Timeout handling
- JSON parsing

### Step 4: Integration

Modify `internal/verify/library.go`:

- Call `InvokeDltest()` in Level 3 phase
- Handle `--skip-dlopen` flag
- Handle fallback when helper unavailable

### Step 5: Build Infrastructure

Add to `.goreleaser.yaml`:

```yaml
builds:
  - id: tsuku-dltest
    main: ./cmd/tsuku-dltest
    binary: tsuku-dltest-{{ .Os }}-{{ .Arch }}
    env:
      - CGO_ENABLED=1
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
```

### Step 6: Tests

- Unit tests for JSON parsing
- Integration tests with real dlopen (requires CGO in test)
- Timeout behavior tests
- Fallback behavior tests

## Consequences

### Positive

- **Definitive "does it load?" answer**: dlopen is what the OS uses; if it works here, it'll work at runtime
- **Process isolation for free**: Crashes in library initialization don't affect tsuku
- **Preserves distribution simplicity**: Main tsuku binary stays CGO_ENABLED=0
- **Batched performance**: ~10ms for 50 libraries vs ~250ms unbatched
- **Debuggable errors**: JSON output includes actual dlerror() messages
- **User opt-out**: `--skip-dlopen` for paranoid users or CI pipelines
- **Graceful degradation**: Verification works (Levels 1-2) even without helper

### Negative

- **First-use download**: Users must download ~1-5MB helper on first verification
- **4 platform builds**: Increases release complexity slightly
- **Library code execution**: Initialization code runs during verification (mitigated by isolation and opt-out)
- **5-second timeout**: May be too short for some legitimate libraries

### Mitigations

| Negative | Mitigation |
|----------|------------|
| First-use download | Cache in `$TSUKU_HOME/.dltest/`; only needed once per version |
| 4 platform builds | Add to existing goreleaser config; automated |
| Code execution | Process isolation, user opt-out, timeout limit |
| Timeout too short | Users can skip with `--skip-dlopen` and verify manually |

## Security Considerations

This section addresses the security checklist from issue #949. Level 3 verification is security-sensitive because it executes code during `dlopen()`.

### Download Verification (Helper Binary Trust Chain)

The `tsuku-dltest` helper is downloaded from GitHub releases and verified before execution.

**Verification mechanism:**
1. SHA256 checksums are embedded in tsuku source code (not fetched from URLs)
2. Checksum verification runs before any execution
3. Verification failure aborts with error (fail closed)
4. Atomic rename prevents execution of partially downloaded binaries

**Why embedded checksums:**
- Updating checksums requires a PR to tsuku (code review)
- No dependency on external checksum files that could be modified
- Users can audit exactly which helper binary their tsuku version uses
- Mirrors Go's `go.sum` pattern for module verification

**Failure behavior:**
```
Error: tsuku-dltest checksum verification failed
  Expected: sha256:abc123...
  Got: sha256:def456...
  The helper binary may have been tampered with.
  Please report this at https://github.com/tsukumogami/tsuku/issues
```

### Execution Isolation

**What code runs during dlopen():**

When `dlopen()` is called, the dynamic linker executes initialization code before returning:

| Platform | Initialization Code |
|----------|---------------------|
| ELF (Linux) | `.init` section, `DT_INIT` function, `DT_INIT_ARRAY` functions |
| Mach-O (macOS) | `__mod_init_func` section entries |
| Both | C++ global constructors, `__attribute__((constructor))` functions |

This code runs with the helper process's privileges (same as tsuku user).

**Mitigations:**

1. **Process isolation**: The helper runs as a separate process. If library initialization crashes, hangs, or behaves maliciously, only the helper process is affected. Tsuku receives an error and can report it without being compromised.

2. **Timeout protection**: 5-second timeout prevents infinite loops or hangs from blocking verification indefinitely.

3. **Only tsuku-installed libraries**: Level 3 verification only tests libraries in `$TSUKU_HOME/libs/`. Users must have already chosen to install these libraries via tsuku.

4. **User opt-out**: `--skip-dlopen` flag allows users to skip Level 3 entirely if they don't want initialization code to execute.

5. **Environment sanitization**: Dangerous environment variables (`LD_PRELOAD`, `LD_AUDIT`, `DYLD_INSERT_LIBRARIES`) are stripped before invoking the helper, preventing code injection via environment.

6. **Path validation**: Library paths are canonicalized and verified to be within `$TSUKU_HOME/libs/`, preventing path traversal attacks.

**User expectation warning**: Level 3 verification is a functional test, not a security scan. "Verification" means "the library loads correctly," not "the library is safe." Users should understand that installed libraries' initialization code will execute during verification.

**What the helper cannot do:**
- Affect tsuku's memory or state (separate process)
- Persist beyond the verification (process exits)
- Access files that tsuku can't already access (same user)

**What the helper can do (residual risk):**
- Execute arbitrary code from the library's initialization sections
- Make network requests, write files, etc. during the ~5 second window
- Crash in ways that might leave partial state

### Supply Chain Risks

**Helper binary source:**
- Built by tsuku's CI (GitHub Actions)
- Published as GitHub release assets
- Same trust model as tsuku itself

**Threat model:**

| Threat | Mitigation |
|--------|------------|
| Compromised GitHub release | Checksums embedded in source; attacker would need to compromise tsuku repo |
| MITM during download | HTTPS only; checksum verification catches modifications |
| Compromised build environment | Same risk as main tsuku binary; use reproducible builds |

**Residual risk**: If an attacker compromises the tsuku repository, they could update both the helper binary and its checksum. This is the same trust level as the main tsuku binary—users already trust the repository.

**Future consideration**: macOS code signing would add an additional verification layer (Apple notarization). Not implemented in this design but documented for future work.

### User Data Exposure

**What the helper accesses:**
- Library files in `$TSUKU_HOME/libs/` (read-only, via dlopen)
- Standard library search paths (read-only)

**What the helper does NOT access:**
- User documents, credentials, or personal data
- Network resources (other than what library initialization might do)
- Tsuku's configuration or state

**Data transmission:**
- No telemetry or external network requests from the helper itself
- Library initialization code may make network requests (out of our control)
- Results returned only via stdout to parent tsuku process

### Security Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious helper binary | Embedded checksums, fail-closed verification | Compromised tsuku repo |
| Library runs malicious .init code | Process isolation, timeout, opt-out | 5-second window for code execution |
| Helper crashes tsuku | Separate process, error handling | None (crashes isolated) |
| Helper hangs forever | 5-second timeout | Legitimate slow libraries may fail |
| MITM during helper download | HTTPS, checksum verification | None |
| Privilege escalation | Helper runs as same user as tsuku | None (no escalation) |
| Environment injection (LD_PRELOAD) | Environment sanitization strips dangerous vars | None |
| Path traversal attack | Path canonicalization, $TSUKU_HOME/libs/ validation | None |
| Concurrent download race | File locking during helper install | None |

### Security Checklist Resolution

Per issue #949 requirements:

- [x] **dlopen code execution documented**: See "Execution Isolation" above
- [x] **Helper binary trust chain defined**: Embedded checksums, fail-closed verification
- [x] **Process isolation ensured**: Helper is separate process; crashes don't affect tsuku
- [x] **Timeout handling defined**: 5-second timeout with context cancellation
- [x] **User opt-out mechanism**: `--skip-dlopen` flag documented
