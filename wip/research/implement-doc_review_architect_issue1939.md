# Architect Review: #1939 PyPI wheel-based executable discovery

**Reviewer**: architect-reviewer
**Commit**: 120fdbceb5438da25b8d84813f584816e045bfad
**Files reviewed**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/artifact.go` (new)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/artifact_test.go` (new)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/pypi.go` (modified)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/pypi_wheel_test.go` (new)

## Summary

The change fits the existing architecture well. It follows the established `BinaryNameProvider` pattern from Cargo and npm builders, uses the correct caching strategy, and extracts a reusable artifact download helper that will serve #1940 (RubyGems). No blocking findings. One advisory note.

## Findings

### 1. Advisory: `fetchPyprojectExecutables` does not use `downloadArtifact`

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/pypi.go:549`

The new `downloadArtifact()` helper in `artifact.go` consolidates HTTPS enforcement, size limiting, Content-Type verification, SHA256 checking, and User-Agent headers into a single callsite. However, the existing `fetchPyprojectExecutables()` method at line 549 still performs its own inline HTTP download with `b.httpClient.Do()`, `io.LimitReader`, and manual User-Agent setting. The same applies to `fetchGemspecExecutables()` in `gem.go:275`.

This is not blocking because:
- `fetchPyprojectExecutables` predates this PR and wasn't part of the change scope.
- It has different requirements: it fetches a text file (not a binary artifact), doesn't need SHA256 verification, and uses a dedicated sub-timeout (`pyprojectFetchTimeout`).
- The two callers are contained -- `downloadArtifact` is for binary artifact downloads, `fetchPyprojectExecutables` is for source-tree text fetches. They won't diverge further because the use cases are genuinely different.

If a future change needs to download more artifact types (e.g., for #1940 where `.gem` files need to be downloaded and unpacked), `downloadArtifact` is the correct function to use. The distinction between "artifact download" (binary blob with hash verification) and "source file fetch" (text from a repo URL) is a reasonable boundary.

**Severity**: Advisory. The two HTTP patterns serve different purposes. No structural divergence risk.

### 2. Structural alignment with BinaryNameProvider pattern: PASS

The implementation follows the same caching strategy as CargoBuilder and NpmBuilder:

| Aspect | CargoBuilder | NpmBuilder | PyPIBuilder (this PR) |
|--------|-------------|-----------|----------------------|
| Cache field | `cachedCrateInfo *cratesIOCrateResponse` | `cachedPackageInfo *npmPackageResponse` | `cachedWheelExecutables []string` |
| Cache population | `Build()` line 124 | `Build()` line 135 | `Build()` line 171-173 |
| `AuthoritativeBinaryNames()` | Returns filtered `bin_names` from cached response | Returns filtered `bin` from cached response | Returns cached executables directly |
| Nil return meaning | No cached data or no usable bin_names | No cached data or no bin field | No cached data or wheel fallback used |
| Interface compliance test | `binary_names_test.go:271` | `binary_names_test.go:275` | `pypi_wheel_test.go:889` |

The PyPI variant caches the processed `[]string` rather than the raw API response. This is appropriate because the executables require downloading and parsing a wheel ZIP -- caching the extracted result avoids re-downloading. For Cargo and npm, caching the API response is cheap (already in memory from `Build()`). The difference in cache granularity is driven by the underlying data access pattern, not an inconsistency.

The key invariant is preserved: `AuthoritativeBinaryNames()` returns `nil` when the data isn't authoritative (wheel fallback to pyproject.toml), matching the contract where `nil` means "skip correction."

### 3. `artifact.go` reusability for #1940 (RubyGems): PASS

The `downloadArtifact()` function signature is:
```go
func downloadArtifact(ctx context.Context, client *http.Client, url string, opts downloadArtifactOptions) ([]byte, error)
```

For RubyGems (#1940), the GemBuilder would need to:
1. Download a `.gem` file (tar archive) from `https://rubygems.org/gems/{name}-{version}.gem`
2. Extract `metadata.gz` from the tar
3. Parse the gemspec from the gzipped metadata

`downloadArtifact` handles step 1 directly -- it returns `[]byte` with HTTPS enforcement, size limits, and optional SHA256 verification. The GemBuilder would call it the same way PyPIBuilder does for wheels, then apply gem-specific unpacking. No changes to the helper would be needed.

The function is package-private (lowercase `d`), which is correct -- it's shared across builders within the same package, not exported as a public API.

### 4. Dependency direction: PASS

`artifact.go` imports only stdlib packages (`context`, `crypto/sha256`, `encoding/hex`, `fmt`, `io`, `net/http`, `strings`). No upward dependencies. The function takes an `*http.Client` as a parameter rather than reaching into the builder struct, which keeps it decoupled from any specific builder.

### 5. No action dispatch bypass: PASS

The wheel download goes through the builder's `httpClient` (same as all other builder HTTP operations), not through a separate client or direct `http.Get`. The `downloadArtifact` helper enforces the same User-Agent convention used by `fetchPackageInfo`, `fetchPyprojectExecutables`, etc.

### 6. No state contract changes: PASS

No changes to state files, CLI surface, or template interpolation. The change is entirely within `internal/builders/`.

## Conclusion

The change respects the existing architecture. The `BinaryNameProvider` implementation follows the established pattern. The `downloadArtifact` helper is well-scoped for reuse by #1940 without modification. The only note is that the older `fetchPyprojectExecutables` uses a different HTTP pattern, but this is contained and doesn't create divergence risk.

**Blocking**: 0
**Advisory**: 1
