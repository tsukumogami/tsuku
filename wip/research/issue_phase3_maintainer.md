# Phase 3 Maintainer Analysis: Failing Tests Badge

## 1. Does the Research Change My Understanding?

**Yes, significantly.**

Phase 1 identified the most likely causes as a nightly scheduled failure or stale badge state,
with "real test failure" as the second-ranked candidate. Phase 2 research has now confirmed the
specific root causes and removed the ambiguity:

**Root cause 1 (resolved):** The `validate-recipes` job in recent nightly runs failed because
batch-added recipes contained invalid `unsupported_platforms` entries. This has already been
fixed by merged PRs, so this failure will not recur.

**Root cause 2 (active, will recur tonight):** The `Functional Tests` job is failing with a
concrete assertion mismatch:

- Test at `test/functional/features/install.feature:51-54` expects:
  ```
  the error output contains "version 99.99.99 not found"
  ```
- `internal/version/resolve.go` line 48 actually produces:
  ```
  no version matching "99.99.99" found
  ```

These strings do not match. I verified both directly:
- `install.feature` line 54: `And the error output contains "version 99.99.99 not found"`
- `resolve.go` line 48: `return nil, fmt.Errorf("no version matching %q found", requested)`

The badge currently shows green because recent push-triggered runs skipped Go jobs due to
path filters (only recipe files changed). The nightly run tonight will fail again unless this
is fixed. The mismatch is either:
- An error message that changed in `resolve.go` without updating the test, or
- A test written against an intended (not yet implemented) error message format.

Either way, one side needs to change.

**Push-triggered vs. nightly discrepancy:** This confirms Phase 1's hypothesis about path
filters. Go jobs don't run on documentation-only pushes, so the badge shows green from the
last skipped (= success) run, not from a passing test suite.

## 2. Is This a Duplicate?

**No.** Phase 2 research confirmed no duplicate issues exist.

## 3. Is the Scope Still Appropriate?

**Yes.** There are exactly two root causes:

1. `unsupported_platforms` recipe failures -- already fixed, no action needed in this issue.
2. Error message mismatch in `install.feature` -- one targeted change, either to `resolve.go`
   or to the feature file.

The scope is tighter than Phase 1 suggested. This is no longer an "investigate and find"
issue -- the root cause is known. The fix is a single file change.

**Recommendation:** Narrow the title to reflect the specific bug rather than "investigate and
resolve."

Proposed title:
```
fix(functional-test): align error message with version resolver output
```

Or, if the intent is to fix the error message format in production code:
```
fix(version): use user-friendly error message when version is not found
```

The second is preferable if the `resolve.go` message is considered poor UX, since "version
99.99.99 not found" reads more naturally to users than "no version matching '99.99.99' found."

## 4. Are We Ready to Draft?

**Yes.** The root cause is confirmed and the fix is well-scoped.

One concern to flag before drafting: we need to decide which side of the mismatch to fix. Two
valid options:

**Option A: Fix `resolve.go` to match the test**

Change line 48 of `internal/version/resolve.go` from:
```go
return nil, fmt.Errorf("no version matching %q found", requested)
```
to:
```go
return nil, fmt.Errorf("version %s not found", requested)
```

This makes the user-facing error more readable and makes the test pass. The test was likely
written to describe the desired behavior; the implementation drifted.

**Option B: Fix `install.feature` to match `resolve.go`**

Update the scenario step to match the actual output. This is valid if the current error
message format is intentional.

Option A is preferable: "version 99.99.99 not found" is clearer to users than the current
phrasing. The test arguably documents the intended UX, not a bug.

## 5. What Context Should Be in the Issue?

The issue should include:

1. **Observable symptom**: Tests badge showing as failing on main.

2. **Why it looked green recently**: Path filters cause Go test jobs to be skipped when only
   recipe files change. Skipped jobs count as success, so the badge shows green between nightly
   runs. The badge will go red again on the next nightly run if not fixed.

3. **Root cause (already known)**: Test `Install with invalid version shows clear error`
   (`test/functional/features/install.feature:51`) expects the error output to contain
   `"version 99.99.99 not found"`, but `internal/version/resolve.go` line 48 returns
   `"no version matching \"99.99.99\" found"`. The strings don't match, so the assertion fails.

4. **Prior resolved cause**: A second failure (`validate-recipes`) from invalid
   `unsupported_platforms` entries in batch-added recipes has already been fixed. No action
   needed there.

5. **Fix direction**: Update `resolve.go` to emit `"version %s not found"` (matching the
   test's expectation, which describes the better user experience), or update the test to
   match the current message. Include the tradeoff so the implementer can decide.

6. **Verification**: After the fix, trigger a manual run of the `Tests` workflow on main (or
   wait for the next nightly) and confirm the `Functional Tests` job passes.

## Recommended Final Title

```
fix(version): use clear error message when requested version is not found
```

This title:
- Describes the production-code fix (Option A, preferred)
- Is specific enough to find later in the git log
- Follows the repo's conventional commits style
- Doesn't mention the symptom (badge) in the title, since that's the consequence, not the cause
