# DESIGN: Consistent Version Sorting

**Status:** Proposed

## Context and Problem Statement

tsuku's version providers return versions in inconsistent order. There are two distinct issues:

**Issue A: Missing sorting in some providers**

GitHub releases and Go proxy providers return versions in API response order without any sorting. This means the "first" version is arbitrary.

**Issue B: No unified sorting algorithm**

Even providers that do sort (npm, PyPI, crates.io, RubyGems) may use different algorithms, potentially leading to inconsistent ordering for edge cases like prereleases or calver formats.

These issues cause several problems:

1. **Unreliable "latest" detection**: Scripts using `tsuku versions <tool> | head -1` get arbitrary versions instead of the actual latest. This caused issue #889 where golden files were regenerated with dlv v1.9.0 instead of the latest v1.26.0.

2. **Poor user experience**: Users running `tsuku versions dlv` see versions in seemingly random order, making it difficult to identify the latest or find a specific version.

3. **Non-reproducibility**: The "first" version returned may vary between environments or cache states, leading to inconsistent behavior.

**Example of current behavior:**
```bash
$ tsuku versions dlv
Available versions (50 total):

  v1.9.0      # <- First result, but NOT the latest
  v1.3.0
  v1.7.0
  v0.7.0-alpha
  ...
  v1.26.0     # <- Actual latest, buried in the list
```

### Scope

**In scope:**
- Unified version sorting algorithm for all version providers
- Handling of semver, calver, and custom version formats
- Prerelease version handling policy
- Consistent descending order (latest first) across all providers

**Out of scope:**
- Changes to version resolution logic (ResolveLatest, ResolveVersion)
- Version filtering or selection UI
- Version constraint expressions (e.g., "^1.2.0")

## Decision Drivers

- **Consistency**: All providers should return versions in the same order
- **Correctness**: Latest version should always be first
- **Performance**: Sorting should not significantly impact response times
- **Maintainability**: Single sorting algorithm, not per-provider implementations
- **Compatibility**: Handle diverse version formats without breaking existing functionality

## Considered Options

### Option A: Centralized Sort Function

Add a single `SortVersions(versions []string) []string` function that all providers call before returning results.

**Approach:**
- Create `version_sort.go` with sorting logic
- Each provider calls `SortVersions()` on its result before returning
- Handles semver detection and comparison

**Pros:**
- Simple to implement
- Easy to test in isolation
- Providers remain independent

**Cons:**
- Each provider must remember to call the sort function
- Easy to forget when adding new providers
- Duplicated sort calls in some code paths

### Option B: Wrapper Interface with Automatic Sorting

Create a wrapper that intercepts `ListVersions()` calls and automatically sorts results.

**Approach:**
- Create `SortedVersionLister` wrapper type
- Factory functions return wrapped providers
- Sorting happens transparently

**Pros:**
- Impossible to forget sorting (automatic)
- Single point of enforcement
- Existing provider code unchanged

**Cons:**
- Adds wrapper layer complexity
- May conflict with existing caching wrapper
- Harder to debug (hidden behavior)

### Option C: Sort at Display/Cache Layer

Sort versions only at the point where they're displayed or cached, not in providers.

**Approach:**
- Add sorting in `cmd/tsuku/versions.go` before display
- Add sorting in `cache.go` before storing

**Pros:**
- Minimal changes to provider code
- Sorting happens once per consumer, not per-provider
- Quick fix for the immediate UX problem

**Cons:**
- Only a partial solution: fixes CLI output but not programmatic access
- Two places to maintain (display and cache)
- Doesn't address Issue B (inconsistent algorithms across providers)
- Scripts calling providers directly still get unsorted output

### Option D: Sort in Provider Base Implementation

Create a base implementation that providers embed, which handles sorting automatically.

**Approach:**
- Create `BaseVersionLister` with `ListVersions()` that calls abstract `fetchVersions()` and sorts
- Providers embed base and implement `fetchVersions()`

**Pros:**
- Guaranteed sorting for all providers using base
- Single implementation

**Cons:**
- Major refactor of existing providers
- Go embedding patterns can be confusing
- Some providers may not fit the pattern

## Decision Outcome

**Chosen: Option A (Centralized Sort Function) with validation**

### Summary

Implement a centralized `SortVersionsDescending()` function that providers call before returning results, combined with test-time validation to catch providers that forget to sort.

### Rationale

Option A provides the right balance of simplicity and correctness:

1. **Low implementation risk**: Adding a sort call to each provider is straightforward and doesn't require architectural changes.

2. **Test-time enforcement**: A test helper that validates sorted output catches regressions without runtime overhead.

3. **Transparent behavior**: Unlike wrappers (Option B), the sorting is explicit in provider code, making debugging easier.

4. **Complete coverage**: Unlike display-only sorting (Option C), this ensures sorted results for all consumers including programmatic access.

5. **Addresses both issues**: Solves Issue A (missing sorting) by adding sort calls, and Issue B (inconsistent algorithms) by using a single `CompareVersions()` function.

Option B was rejected because the wrapper adds complexity and may conflict with existing caching infrastructure.

Option C was rejected because it only fixes CLI output, not programmatic access. Scripts that call provider functions directly would still receive unsorted versions.

Option D was rejected because the refactoring effort doesn't justify the benefit over Option A with tests.

## Solution Architecture

### Components

```
internal/version/
├── version_sort.go       # New: SortVersionsDescending() function
├── version_sort_test.go  # New: Sort algorithm tests
├── version_utils.go      # Existing: CompareVersions() enhanced
├── resolver.go           # Modified: Add sort calls to List* functions
├── provider_*.go         # Modified: Add sort calls where missing
└── *_test.go             # Modified: Add sorted output assertions
```

### Version Sort Algorithm

```go
// SortVersionsDescending sorts versions in descending order (latest first).
// Handles semver, calver, and custom formats with graceful fallback.
func SortVersionsDescending(versions []string) []string {
    result := make([]string, len(versions))
    copy(result, versions)

    sort.Slice(result, func(i, j int) bool {
        return CompareVersions(result[i], result[j]) > 0
    })

    return result
}
```

### Enhanced CompareVersions

The existing `CompareVersions()` function needs enhancement to handle:

1. **Version prefix stripping**: `v1.2.3` and `1.2.3` should compare equal
2. **Prerelease ordering**: `1.0.0-alpha` < `1.0.0-beta` < `1.0.0-rc.1` < `1.0.0`
3. **Calver support**: `2024.01.15` should sort correctly
4. **Custom formats**: `go1.21.5` should extract numeric components

```go
// CompareVersions compares two version strings.
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
//
// Supported formats:
//   - Semver: "1.2.3", "v1.2.3", "1.2.3-rc.1"
//   - Calver: "2024.01.15", "24.05"
//   - Go toolchain: "go1.21.5"
//   - Fallback: lexicographic comparison
func CompareVersions(v1, v2 string) int
```

### Version Format Handling

**Key insight**: Versions are only compared within the same tool. A tool using semver (like `dlv`) will never have its versions compared against a tool using calver (like `python`). The algorithm only needs to sort correctly *within* each tool's version list.

**Approach: Numeric extraction with fallback**

Most versioning schemes share a common property: they're sequences of numbers separated by delimiters. The algorithm:

1. Normalizes the version string (strips prefixes like `v`, `go`, `Release_`)
2. Extracts numeric components by splitting on non-numeric characters
3. Compares numeric sequences element-by-element
4. Falls back to lexicographic comparison when extraction fails

**Format examples:**

| Format | Input | After Normalization | Numeric Sequence |
|--------|-------|---------------------|------------------|
| Semver | `v1.2.3` | `1.2.3` | `[1, 2, 3]` |
| Semver+pre | `1.0.0-rc.2` | `1.0.0-rc.2` | `[1, 0, 0]` + prerelease |
| Calver | `2024.01.15` | `2024.01.15` | `[2024, 1, 15]` |
| Go | `go1.21.5` | `1.21.5` | `[1, 21, 5]` |
| Custom | `Release_1_15_0` | `1.15.0` | `[1, 15, 0]` |

**Prerelease handling:**

Prereleases are detected by the presence of `-` after the version core:
- Stable versions sort higher than prereleases: `1.0.0 > 1.0.0-rc.1`
- Prereleases of the same version sort lexicographically: `1.0.0-rc.2 > 1.0.0-rc.1 > 1.0.0-beta > 1.0.0-alpha`
- Common prerelease tags are recognized: `alpha < beta < rc`

**Fallback behavior:**

When numeric extraction produces identical sequences (or fails entirely), the algorithm falls back to lexicographic comparison. This handles edge cases like:
- Build metadata: `1.0.0+build.123` vs `1.0.0+build.456`
- Non-standard formats: `nightly-2024-01-15`

**Cross-format comparison:**

If two versions from the same tool use different formats (unlikely but possible), numeric extraction still produces a reasonable ordering. For example, if a tool switched from calver to semver:
- `2024.01.15` → `[2024, 1, 15]`
- `1.0.0` → `[1, 0, 0]`
- Result: `2024.01.15 > 1.0.0` (calver sorts higher due to larger first component)

This may not reflect the tool's actual release order, but such format changes are rare and would require manual intervention regardless of the sorting algorithm.

### Provider Modifications

Sorting is added at the resolver's internal list methods (e.g., `ListGoProxyVersions`, `ListGitHubVersions`). These are the lowest-level functions that return version lists, ensuring all consumers receive sorted results.

Provider wrappers (e.g., `GitHubProvider.ListVersions()`) call these resolver methods and pass through the already-sorted results. No double-sorting occurs because sorting is idempotent.

```go
// Before
func (r *Resolver) ListGoProxyVersions(ctx context.Context, modulePath string) ([]string, error) {
    // ... fetch versions ...
    return versions, nil
}

// After
func (r *Resolver) ListGoProxyVersions(ctx context.Context, modulePath string) ([]string, error) {
    // ... fetch versions ...
    return SortVersionsDescending(versions), nil
}
```

### Prerelease Handling Policy

Prereleases are included but sorted correctly:
- `1.0.0` > `1.0.0-rc.2` > `1.0.0-rc.1` > `1.0.0-beta` > `1.0.0-alpha`
- No filtering of prereleases (user can filter if desired)

### Test Validation

Add a test helper to verify sorted output:

```go
// AssertVersionsSorted fails the test if versions are not in descending order
func AssertVersionsSorted(t *testing.T, versions []string) {
    for i := 1; i < len(versions); i++ {
        if CompareVersions(versions[i-1], versions[i]) < 0 {
            t.Errorf("versions not sorted: %s came before %s",
                versions[i-1], versions[i])
        }
    }
}
```

## Implementation Approach

### Phase 1: Core Sorting (2 issues)

1. **Enhance CompareVersions()**: Add prerelease handling, prefix stripping, calver support
2. **Add SortVersionsDescending()**: Create the centralized sort function with tests

### Phase 2: Provider Updates (3 issues)

3. **Fix GoProxy provider**: Add sorting to `ListGoProxyVersions()`
4. **Fix GitHub provider**: Add sorting to `ListGitHubVersions()`
5. **Audit remaining providers**: Verify npm, PyPI, crates.io, RubyGems, MetaCPAN providers use the unified `CompareVersions()` function. Replace any custom sorting logic.

### Phase 3: Validation (1 issue)

6. **Add sorted output tests**: Test helper and assertions for all providers

## Security Considerations

### Download verification
**Not applicable**: This design only affects version listing order, not download or verification logic.

### Execution isolation
**Not applicable**: No code execution is involved in version sorting.

### Supply chain risks
**Not applicable**: Version sorting doesn't change what versions are available or where they come from. The same versions are returned, just in a different order.

### User data exposure
**Not applicable**: Version sorting is a pure transformation of data already fetched from public APIs. No additional data is accessed or transmitted.

## Consequences

### Positive

- **Predictable behavior**: `tsuku versions <tool> | head -1` reliably returns the latest version
- **Better UX**: Users can easily identify latest and find versions
- **Script compatibility**: Tools like `regenerate-golden.sh` work correctly
- **Single source of truth**: One sorting algorithm, consistent behavior

### Negative

- **Breaking change for scripts**: Any scripts relying on the current (undefined) order may break. However, this is unlikely since the current order is not documented or guaranteed.
- **Minor performance overhead**: Sorting adds O(n log n) overhead to version listing. For typical version counts (<100), this is negligible.

### Neutral

- **Cache invalidation**: Existing cached version lists will have old ordering until they expire. This is acceptable since caches expire within hours.
- **Single algorithm for all formats**: The enhanced `CompareVersions()` becomes the single source of truth for version comparison. A regression in this function would affect all providers simultaneously. This is offset by comprehensive test coverage and the benefit of consistent behavior.
