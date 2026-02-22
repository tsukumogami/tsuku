# Pragmatic Review: Issue #1881 (Round 2)

**Issue**: feat(dashboard): add library_only failure subcategory
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Round**: 2 (re-review after round 1 fix in commit 6300f70)
**Files changed**: internal/builders/homebrew.go, internal/builders/homebrew_test.go, internal/dashboard/failures.go, internal/dashboard/failures_test.go

---

## Summary

Round 1 found a blocking issue (dead code in classifier due to missing error wrapping in `generateDeterministicRecipe`). That's been fixed -- line 2184 now wraps with `fmt.Errorf("library recipe generation failed: %w", err)`, which the classifier at line 503 matches via `strings.Contains(msg, "library recipe generation failed")`.

The change remains minimal and well-scoped:

1. `internal/dashboard/failures.go:46` -- one map entry: `"library_only": true`
2. `internal/builders/homebrew.go:503-505` -- one new switch case, correctly ordered before the broader `no binaries` cases
3. `internal/builders/homebrew_test.go:2542-2545` -- table test case for classifier
4. `internal/builders/homebrew_test.go:2587-2607` -- dedicated test verifying the `[library_only]` tag and formula name in the message
5. `internal/dashboard/failures_test.go:56-60` -- table test case for bracket-tag extraction

No new abstractions, types, or speculative generality. Each change is one line or one small block. The error wrapping chain (`generateDeterministicRecipe` -> `classifyDeterministicFailure` -> `extractSubcategory`) is now fully connected and tested at each hop.

---

## Findings

### Blocking Issues

None.

### Advisory Observations

None. This is the simplest correct approach for the stated requirement.
