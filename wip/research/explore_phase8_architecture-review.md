# Architecture Review: Tier 2 Dependency Resolution Design

**Date:** 2026-01-16
**Reviewer:** Architecture Review Agent
**Design Document:** `docs/designs/DESIGN-library-verify-deps.md`

## Executive Summary

The Tier 2 dependency resolution design is well-structured and builds appropriately on the existing Tier 1 implementation. The architecture is implementable with minor clarifications needed. This review identifies one significant gap (RPATH extraction mechanism), validates the implementation phases as correctly sequenced, and suggests no simpler alternatives that would meet all requirements.

---

## 1. Architecture Clarity Assessment

### 1.1 Strengths

**Clear separation of concerns:**
- Classification phase (pattern matching) is distinct from resolution phase (path expansion)
- The `DepResult` type cleanly captures all possible outcomes
- Data flow from `ValidateHeader()` -> `ValidateDependencies()` is intuitive

**Good reuse of existing code:**
- Leverages `HeaderInfo.Dependencies` already extracted by Tier 1
- Reuses `ValidateHeader()` for validating resolved dependencies
- Pattern lists derived from tested shell script (`verify-no-system-deps.sh`)

**Appropriate interface design:**
- `ValidateDependencies(libPath string, deps []string, libsDir string) []DepResult` is a clean API
- `DepStatus` enum with clear semantics (System, Valid, Missing, Invalid, Warning)
- Error categories follow existing `ErrorCategory` pattern

### 1.2 Areas Needing Clarification

**Classification flow ambiguity:**
The design describes classification logic in prose but the exact precedence is unclear. The implementation should follow this order:

1. Check system library patterns first (fast path for common case)
2. Check path variable prefixes (`$ORIGIN`, `@rpath`, etc.) - these go to resolution
3. Check absolute paths (system vs. unknown)
4. Everything else is a warning

**Return value semantics:**
The design shows `classifyDep()` returning `DepSystem`, `DepWarning`, or "needs resolution" but this isn't modeled in the type system. Consider:
- Making this explicit with a separate `ClassificationResult` type, OR
- Having `classifyDep()` return `(DepStatus, needsResolution bool)`

**libsDir parameter source:**
The function signature includes `libsDir` but the design doesn't specify how `verify.go` obtains this. Looking at the codebase, this should come from `cfg.LibsDir` (from `internal/config`).

---

## 2. Missing Components and Interfaces

### 2.1 Critical Gap: RPATH Extraction

**Problem:** The design mentions "Extract RPATH entries from binaries for `@rpath` resolution" but doesn't specify the interface.

**Current state:**
- `internal/actions/set_rpath.go` has `parseRpathsFromOtool()` for macOS but shells out to `otool`
- There's no equivalent ELF RPATH extraction function
- The design requires "no external tools" (use Go's standard library)

**Recommended interface:**

```go
// ExtractRPaths returns RPATH/RUNPATH entries from a binary.
// For ELF: reads DT_RPATH and DT_RUNPATH from .dynamic section
// For Mach-O: reads LC_RPATH load commands
func ExtractRPaths(path string) ([]string, error)
```

**Implementation notes:**
- ELF: Use `f.DynString(elf.DT_RPATH)` and `f.DynString(elf.DT_RUNPATH)` from `debug/elf`
- Mach-O: Need to iterate `f.Loads` and check for `*macho.Rpath` (Go 1.21+)

### 2.2 Missing: Path Variable Expansion Interface

**Problem:** The design mentions `expandPathVariables()` but doesn't define it.

**Recommended interface:**

```go
// ExpandPathVariables resolves path variables in a dependency name.
// libPath is the library being validated (for $ORIGIN/@loader_path).
// rpaths is the list of RPATH entries from the binary (for @rpath).
// Returns empty string if resolution fails.
func ExpandPathVariables(dep string, libPath string, rpaths []string) string
```

**Edge cases to handle:**
- `@rpath` with multiple LC_RPATH entries (search order matters - first match wins)
- `@executable_path` vs `@loader_path` (different for nested dylib loading)
- `$ORIGIN` in symlinked binaries (should resolve to real path's directory)

### 2.3 Missing: Integration with verify.go

The design shows `verify.go` calling `ValidateDependencies()` but doesn't specify:
- How to aggregate results (all pass vs. any fail)
- Output formatting (how to display to user)
- Whether to continue after first failure

**Recommendation:** Add to the design:

```go
// In verifyLibrary():
for _, libFile := range libFiles {
    info, err := verify.ValidateHeader(libFile)
    if err != nil { ... }

    // Tier 2: Dependency resolution
    depResults := verify.ValidateDependencies(libFile, info.Dependencies, cfg.LibsDir)
    for _, dep := range depResults {
        switch dep.Status {
        case verify.DepMissing, verify.DepInvalid:
            // Report error, continue checking
        case verify.DepWarning:
            // Report warning, don't fail
        }
    }
}
```

---

## 3. Implementation Phase Sequencing

### 3.1 Current Sequence Analysis

| Step | Description | Dependencies | Assessment |
|------|-------------|--------------|------------|
| 1 | Add types to types.go | None | Correct first step |
| 2 | Implement classifyDep() | Step 1 | Correct - needs types |
| 3 | Implement path expansion | Step 2 | **Issue: needs RPATH extraction** |
| 4 | Implement ValidateDependencies | Steps 2, 3 | Correct |
| 5 | Integrate with verify.go | Step 4 | Correct |
| 6 | Add unit tests | Steps 2-4 | Could be earlier |
| 7 | Update CI integration test | Step 5 | Correct - last |

### 3.2 Recommended Sequence Adjustments

**Insert Step 2.5: Implement RPATH extraction**

The design jumps from classification to path expansion without implementing RPATH extraction. This should be a separate step:

1. Add dependency types and error categories
2. Implement classification logic
3. **Implement RPATH extraction (`ExtractRPaths`)** - NEW
4. Implement path variable expansion (uses RPATH extraction)
5. Implement ValidateDependencies
6. Integrate with verify command
7. Add unit tests
8. Update CI integration test

**Consider test-first approach:**

Unit tests for classification and path expansion are straightforward to write without integration. Moving Step 6 earlier enables TDD:

1. Add types
2. Add unit tests for classification (with stub implementation)
3. Implement classification
4. Add unit tests for path expansion (with stub)
5. Implement RPATH extraction
6. Implement path expansion
7. Implement ValidateDependencies
8. Integrate with verify command
9. Update CI integration test

---

## 4. Simpler Alternatives Evaluation

### 4.1 Alternative: Skip tsuku-managed dependency validation

**Approach:** Only classify dependencies as system or warning; don't validate tsuku-managed deps.

**Why rejected:**
- Doesn't catch the core failure mode (missing tsuku deps)
- Already rejected in the design as "Option 1"
- Provides false confidence to users

### 4.2 Alternative: Shell out to ldd/otool

**Approach:** Use subprocess to run `ldd` (Linux) or `otool -L` (macOS) instead of parsing headers.

**Why rejected:**
- Violates "no external tools" decision driver
- `ldd` actually loads the binary (security concern with untrusted binaries)
- Less portable (tool might not be installed)
- Go standard library provides all needed functionality

### 4.3 Alternative: Only validate file existence, skip Tier 1 on deps

**Approach:** For resolved tsuku deps, just check `os.Stat()` instead of full `ValidateHeader()`.

**Evaluation:**
- Simpler implementation
- Faster execution
- But misses corrupted dependency files
- Trade-off: Speed vs. completeness

**Recommendation:** This could be a valid simplification for Phase 1, with header validation added later. However, since `ValidateHeader()` is already available and fast (header-only, not full file read), the overhead is minimal. Keep the current design.

### 4.4 Alternative: Lazy pattern matching with compiled regex

**Approach:** Compile system patterns to regex for faster matching.

**Evaluation:**
- Current O(n) string matching is simple and correct
- Number of patterns is small (~15 per platform)
- Number of dependencies per library is typically <20
- Regex compilation overhead might exceed savings

**Recommendation:** Keep simple string matching. Optimize only if profiling shows this is a bottleneck.

---

## 5. Detailed Findings

### 5.1 System Pattern Completeness

**Current patterns (Linux):**
```go
var linuxSystemPatterns = []string{
    "linux-vdso.so",
    "ld-linux", "ld-musl",
    "libc.so", "libm.so", "libdl.so", "libpthread.so",
    "librt.so", "libresolv.so", "libgcc_s.so", "libstdc++.so",
}
```

**Missing from shell script:**
- `libnsl.so` - Network services library (in shell script, not design)
- `libcrypt.so` - Encryption library (in shell script, not design)
- `libutil.so` - Utility library (in shell script, not design)

**Recommendation:** Sync with `verify-no-system-deps.sh` to ensure consistency.

### 5.2 macOS Pattern Concerns

**Current patterns:**
```go
var darwinSystemPatterns = []string{
    "/usr/lib/lib",        // System libraries
    "/System/Library/",    // System frameworks
}
```

**Issue:** `/usr/lib/lib` is overly broad - it matches `/usr/lib/library_user_installed.dylib`.

**Recommendation:** Use more specific patterns:
- `/usr/lib/libSystem` - Main system library
- `/usr/lib/libc++` - C++ runtime
- `/usr/lib/libobjc` - Objective-C runtime
- Or check full paths against known system library list

### 5.3 Error Category Values

**Design proposes:**
```go
const (
    ErrDepMissing   ErrorCategory = iota + 100
    ErrDepInvalid
)
```

**Concern:** Using `iota + 100` creates a gap in enum values. This works but is unusual.

**Alternative:** Just continue the existing iota sequence:
```go
const (
    // Existing categories...
    ErrCorrupted ErrorCategory = iota

    // New Tier 2 categories
    ErrDepMissing
    ErrDepInvalid
)
```

**Recommendation:** Use continuous iota. The gap provides no benefit and could cause confusion in debugging.

### 5.4 Path Traversal Safety

**Design mentions:**
> "Validate resolved paths stay within `$TSUKU_HOME/libs/` or system paths"

**Current code reference:** `internal/actions/set_rpath.go` has `validatePathWithinDir()` which could be reused.

**Recommendation:** Move path validation to a shared location (e.g., `internal/verify/path.go` or `internal/util/path.go`) and reuse in both actions and verify packages.

### 5.5 Concurrency Considerations

The design doesn't mention concurrency, but `ValidateDependencies()` is a natural candidate for parallel processing (each dependency can be validated independently).

**Recommendation:** Document that the initial implementation is sequential for simplicity, with concurrent validation as a future optimization if needed.

---

## 6. Summary

### Questions Answered

| Question | Answer |
|----------|--------|
| Is the architecture clear enough to implement? | **Yes**, with minor clarifications on classification precedence and integration points |
| Are there missing components or interfaces? | **Yes**: RPATH extraction interface, path expansion interface, and verify.go integration details |
| Are implementation phases correctly sequenced? | **Mostly yes**, but needs RPATH extraction step inserted before path expansion |
| Are there simpler alternatives? | **No** viable alternatives that meet all decision drivers |

### Key Recommendations

1. **Add RPATH extraction interface** - Critical gap for `@rpath` resolution
2. **Sync system patterns** with existing shell script to avoid regression
3. **Refine macOS patterns** - current `/usr/lib/lib` prefix is too broad
4. **Insert RPATH extraction step** in implementation sequence
5. **Consider reusing** `validatePathWithinDir()` from actions package
6. **Document sequential execution** as initial approach, concurrent as future optimization

### Implementation Readiness

**Ready to implement:** Types, classification, basic path expansion
**Needs design clarification:** RPATH extraction mechanism, verify.go integration details
**Low risk areas:** System patterns, error categories
**Medium risk areas:** Path variable expansion edge cases (`@rpath` with multiple entries)

---

## Appendix: Code References

| Component | File | Lines |
|-----------|------|-------|
| Current Tier 1 implementation | `internal/verify/header.go` | Full file |
| Type definitions | `internal/verify/types.go` | Full file |
| Verify command | `cmd/tsuku/verify.go` | Lines 234-309 (verifyLibrary) |
| Shell script patterns | `test/scripts/verify-no-system-deps.sh` | Lines 38-63 |
| RPATH handling reference | `internal/actions/set_rpath.go` | Lines 269-297 (parseRpathsFromOtool) |
