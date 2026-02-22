# Maintainer Review (Round 2): Issue #1881

**Issue**: feat(dashboard): add library_only failure subcategory
**Focus**: maintainability (clarity, readability, duplication)
**Date**: 2026-02-22
**Round**: 2 (re-review after commit 6300f70 fixed the dead-code blocker from round 1)

---

## Round 1 Blocker Status: RESOLVED

The previous round found that `classifyDeterministicFailure` matched on `"library recipe generation failed"` but no application code produced that string. Commit 6300f70 added the wrapper at `internal/builders/homebrew.go:2184`:

```go
return nil, fmt.Errorf("library recipe generation failed: %w", err)
```

This connects the classifier to the actual error path. The `[library_only]` tag is now reachable when `generateLibraryRecipe` returns an error (e.g., if `scanMultiplePlatforms` returns an empty slice because the runner's platform doesn't match either target and both bottle downloads fail).

---

## Finding 1: Case ordering comment still missing (Advisory)

**File**: `internal/builders/homebrew.go:503-510`
**Severity**: Advisory

Round 1 noted that the `library recipe generation failed` case at line 503 is placed before the `no binaries` cases at line 507-508 for specificity. This ordering is intentional (documented in the coder's summary) but has no code comment. The strings don't currently overlap, so there's no bug risk today. A one-line comment would prevent the next developer from reordering these cases when adding a new classification branch.

---

## Finding 2: Test at line 2593 uses a synthetic but realistic error (Advisory)

**File**: `internal/builders/homebrew_test.go:2587-2607`
**Severity**: Advisory

`TestHomebrewSession_classifyDeterministicFailure_libraryOnlyTag` constructs:

```go
fmt.Errorf("library recipe generation failed: %w", fmt.Errorf("no platform contents provided"))
```

With the wrapper now in place at line 2184, this matches the actual error format that production code would produce. The inner message `"no platform contents provided"` is the real error from `generateLibraryRecipe:2028`. The test is valid and correctly exercises the classifier + bracket-tag emission path end-to-end for the message format (though it doesn't go through the full call chain -- it tests the classifier in isolation, which is appropriate for a unit test).

No action needed. Noting this for completeness since round 1 flagged it.

---

## Finding 3: Dashboard map entry and test follow conventions (Positive)

- `internal/dashboard/failures.go:46`: `"library_only": true` -- single line, consistent with surrounding entries.
- `internal/dashboard/failures_test.go:56-60`: Table-driven test case matches the established pattern exactly. The test message matches the format produced by `classifyDeterministicFailure` at line 505.

---

## Summary

The round 1 blocking finding is resolved. The error wrapper at `homebrew.go:2184` connects the classifier's string match to the actual error path. The `[library_only]` bracket tag flows correctly through `classifyDeterministicFailure` into messages that `extractSubcategory` can parse via the Level 1 bracket-tag extraction.

One advisory remains: the case ordering in the classifier switch has no comment explaining why `library recipe generation failed` precedes `no binaries`. This is minor -- the strings don't overlap today, and the ordering only matters as a future-proofing measure.

The code is clear and the changes are minimal. No blocking findings.
