# Maintainer Review: #1939 (PyPI wheel-based executable discovery)

## Summary

0 blocking, 3 advisory findings. The code is well-structured and follows established patterns from the Cargo and npm BinaryNameProvider implementations. The new `artifact.go` helper is clean with good separation of concerns. The wheel parsing pipeline (download, unzip, parse entry_points.txt) is straightforward and thoroughly tested.

## Findings

### 1. Test name lies: `TestPyPIBuilder_Build_InvalidExecutableNamesFiltered` contains no invalid names

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/pypi_wheel_test.go:499-558`

**Severity:** Advisory

The test name says "invalid executable names filtered" but the entry_points.txt fixture only contains `good-tool` and `also-good` -- both valid names. The assertion at line 552 expects exactly 2 executables (all of them), meaning the test doesn't actually exercise the filtering path at all.

The next developer reading this test name will think "filtering of bad names is covered" and won't write a test with actually invalid names like `../escape` or `;rm -rf /`. Compare with cargo's `TestCargoBuilder_Build_InvalidBinaryNamesFiltered` which includes genuinely invalid entries alongside valid ones.

To fix: add invalid entries to the entry_points.txt fixture (e.g., `../bad = pkg:main`, empty name, or names with special chars) so the assertion `len(executables) != 2` actually proves filtering works.

### 2. `AuthoritativeBinaryNames()` returns the internal slice directly (aliasing)

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/pypi.go:734-739`

**Severity:** Advisory

```go
func (b *PyPIBuilder) AuthoritativeBinaryNames() []string {
    if len(b.cachedWheelExecutables) == 0 {
        return nil
    }
    return b.cachedWheelExecutables
}
```

`Build()` at line 171 carefully creates a defensive copy into `cachedWheelExecutables`, but `AuthoritativeBinaryNames()` returns that slice directly. The caller (`correctBinaryNames` in `binary_names.go`) doesn't mutate the slice today, so this is safe in practice. However, the Cargo implementation at `cargo.go:322-332` builds a fresh slice on each call, and the npm implementation similarly constructs a new slice. This is a divergence from the other builders' pattern.

If `correctBinaryNames` ever sorts or appends to the returned slice, it would corrupt PyPI's cached data. Since the existing callers don't mutate, this is advisory, but returning a copy would align with the other builders and prevent the trap.

### 3. `isValidExecutableName` now has three callers but still lives in `cargo.go`

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:272`

**Severity:** Advisory

With this PR, `isValidExecutableName()` is called from three builder files: `cargo.go`, `npm.go` (pre-existing from #1937), and now `pypi.go:435`. It's still defined in `cargo.go`. The next developer adding a new builder (e.g., RubyGems, CPAN) who greps for "executable name validation" will find it in a Cargo-specific file and might not realize it's the shared validator, or might create a duplicate.

This was noted as advisory in the #1937 review. Now that a third consumer exists, the case for moving it to a shared file (e.g., `binary_names.go`, which already hosts the `BinaryNameProvider` interface and `correctBinaryNames`) is stronger.

## What reads well

- `downloadArtifact` in `artifact.go` is a clean shared utility with clear option names, good error messages that include context (URL for HTTPS failure, size for limit errors, both hashes for mismatch), and a clever +1 byte trick for size detection that's well-commented.

- The `discoverExecutables` three-value return (`executables, warnings, fromWheel`) makes the caching contract explicit. The caller knows whether the result is authoritative without inspecting the data.

- `parseConsoleScripts` is separated from ZIP handling and accepts `io.Reader`, making it independently testable. `TestParseConsoleScripts` covers the important edge cases (section transitions, comments, malformed lines, empty input).

- The `normalizePyPIName` function correctly implements PEP 503 and has a thorough test table.

- The fallback chain (wheel -> pyproject.toml -> package name) produces warnings at each step, giving the orchestrator full visibility into what happened. This is better than silent fallbacks.

- `findBestWheel` preference logic is simple and correct: iterate once, keep upgrading to platform-independent if found. The five test cases cover the important scenarios.

- The `AuthoritativeBinaryNames` doc comment at line 725-733 clearly explains why only wheel-discovered names are authoritative, matching the established pattern from Cargo and npm where the comment explains the intentional divergence between `discoverExecutables` (always returns something) and `AuthoritativeBinaryNames` (returns nil when not authoritative).
