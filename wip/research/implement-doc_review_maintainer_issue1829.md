# Maintainer Review: Issue #1829

**Issue**: feat(registry): include satisfies data in registry manifest
**Focus**: maintainability (clarity, readability, duplication)
**Date**: 2026-02-21

---

## Files Reviewed

- `internal/registry/manifest.go` (new)
- `internal/registry/manifest_test.go` (new)
- `internal/recipe/loader.go` (modified -- `buildSatisfiesIndex`)
- `internal/recipe/satisfies_test.go` (modified -- new manifest integration tests)
- `scripts/generate-registry.py` (modified -- satisfies generation + validation)
- `scripts/test_generate_registry.py` (new -- Python tests for cross-recipe validation)
- `recipes/l/libcurl.toml` (modified -- satisfies removal + explanatory comment)

---

## Finding 1: Duplicated cross-recipe validation logic in test (Advisory)

**File**: `scripts/test_generate_registry.py:38-71`

The `_run_cross_recipe_validation` method replicates the validation logic from `generate-registry.py` `main()` (lines 321-345) verbatim. The docstring says as much: "This replicates the validation logic from generate-registry.py main() (lines 322-345)."

The next developer who changes the validation in `main()` -- say, adding a new collision rule -- will need to find and update this copy too. The line-number reference in the docstring will go stale quickly, making the connection even harder to follow.

**What the next person will think**: "I updated the validation in `generate-registry.py` and all the existing tests still pass" -- because the tests exercise a copied version of the old logic, not the actual function.

**Suggestion**: Extract the cross-recipe validation from `main()` into a standalone function (e.g., `validate_cross_recipe_satisfies(recipes, recipe_names) -> list[ValidationError]`). Call it from both `main()` and the test. The test then exercises production code.

**Severity**: Advisory. The duplication is documented and both copies are identical today. The risk materializes only when the validation changes, and the proximity of the files (both in `scripts/`) provides some protection.

---

## Finding 2: `_build_recipes` helper is dead code (Advisory)

**File**: `scripts/test_generate_registry.py:34-36`

```python
def _build_recipes(self, recipe_list):
    """Build a list of parsed recipe dicts for cross-recipe validation."""
    return recipe_list
```

This method does nothing (returns its argument unchanged) and is never called. All three tests construct recipe lists inline. The next developer will wonder whether tests should use it or whether it's a leftover from an earlier draft.

**Severity**: Advisory. Small dead code, low misread risk, but it should be removed.

---

## Finding 3: Divergent duplicate-detection behavior is well-commented (No issue)

**File**: `internal/recipe/loader.go:370-417`

`buildSatisfiesIndex()` handles duplicates differently for embedded recipes (warns at runtime, line 389-392) versus manifest entries (silently skips, line 411). The inline comment at line 411 explains the reasoning: "No warning for manifest duplicates: the generate script already validates cross-recipe duplicates at CI time."

The next developer reading this will understand the divergence. The comment is specific about *why* the behavior differs and *where* the validation happens instead. This is the right approach.

**Severity**: No issue.

---

## Finding 4: `FetchManifest` side effect is documented in docstring (No issue)

**File**: `internal/registry/manifest.go:52-95`

`FetchManifest` fetches from the network and writes to the cache directory (lines 87-91). The name alone suggests a read-only operation, but the docstring on line 51-52 says "fetches the registry manifest from the network and caches it locally." This matches the existing `fetchFromRegistry` pattern in `loader.go:282-305`, which also caches as a side effect.

The next developer will find the caching behavior from the docstring. Consistent with codebase patterns.

**Severity**: No issue.

---

## Finding 5: `libcurl.toml` explanatory comment prevents a common mistake (No issue)

**File**: `recipes/l/libcurl.toml:7-9`

```toml
# No [metadata.satisfies] for homebrew "curl" -- that name is the canonical recipe
# recipes/c/curl.toml (the CLI tool). This recipe installs the library via
# Homebrew's curl formula but is a distinct package from the curl CLI.
```

This comment prevents a future developer from "helpfully" adding `satisfies.homebrew = ["curl"]` to `libcurl.toml`. The comment names the specific canonical recipe that would conflict and explains the relationship between the two packages. The Python test `test_libcurl_claiming_curl_rejected` reinforces the same constraint from the CI side.

**Severity**: No issue. Good proactive documentation.

---

## Finding 6: Test names in satisfies_test.go and manifest_test.go are accurate (No issue)

New tests added by #1829:
- `TestSatisfies_BuildIndex_IncludesManifestData` -- verifies manifest data enters the index
- `TestSatisfies_BuildIndex_EmbeddedOverManifest` -- verifies priority ordering
- `TestSatisfies_BuildIndex_NoManifest` -- verifies graceful degradation when no manifest cached
- `TestSatisfies_ManifestRecipeResolvable` -- end-to-end: alias lookup resolves through manifest satisfies
- `TestSatisfies_LoadDirect_SkipsSatisfiesFallback` -- documents cycle-safety contract
- `TestParseManifest_WithSatisfies` -- verifies JSON parsing with and without satisfies
- `TestFetchManifest_CachesLocally` -- verifies network fetch + cache write
- `TestManifestRecipe_SatisfiesOmittedWhenEmpty` -- verifies omitempty behavior

Each test name matches what the test asserts. No test name lies.

**Severity**: No issue.

---

## Finding 7: Conditional satisfies inclusion is consistent across Python and Go (No issue)

**Python** (`scripts/generate-registry.py:271-275`): `if satisfies:` guards inclusion, so falsy values (None, empty dict) are omitted from JSON output.

**Go** (`internal/registry/manifest.go:40`): `json:"satisfies,omitempty"` on `ManifestRecipe.Satisfies` ensures the same omission semantics on the consumer side.

Both sides agree: absent satisfies means the field doesn't appear in JSON. The test `TestManifestRecipe_SatisfiesOmittedWhenEmpty` verifies the Go side. The next developer working on either side will find consistent behavior.

**Severity**: No issue.

---

## Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | Duplicated cross-recipe validation in Python test | Advisory |
| 2 | `_build_recipes` is dead code | Advisory |
| 3 | Divergent duplicate-detection well-commented | No issue |
| 4 | `FetchManifest` side effect documented | No issue |
| 5 | `libcurl.toml` comment prevents wrong mapping | No issue |
| 6 | Test names accurate | No issue |
| 7 | Python/Go satisfies omission consistent | No issue |

**Blocking: 0**
**Advisory: 2**

The code is clear and well-organized. The new `internal/registry/manifest.go` is a focused addition following existing package patterns. Test coverage is thorough across both Go and Python, and test names match behavior. The `libcurl.toml` comment is a good example of documenting an intentional absence. The only maintainability concern is the duplicated validation logic in the Python test, which could diverge from production code on future changes -- but both copies are identical today and live in adjacent files.
