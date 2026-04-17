# Phase 3 User Assessment: Failing Test Badge

Role: User of the tsuku CLI tool

---

## 1. Does the research change your understanding?

**Yes, substantially.**

Phase 1 identified the problem as "the badge is failing — investigate why." Phase 2 research
pinpoints two concrete root causes, one already resolved and one still live:

1. **Validate Recipes failures (April 10-16)**: Batch-added recipes had invalid
   `unsupported_platforms` entries. This was fixed by PRs merged before today. Not an issue
   anymore.

2. **Functional Tests failure (still present)**: The Gherkin scenario
   `Install with invalid version shows clear error` in
   `test/functional/features/install.feature` (line 51-54) expects:
   - Exit code: `6` (ExitInstallFailed)
   - Error output containing: `"version 99.99.99 not found"`

   The actual error produced by `internal/version/resolve.go` line 48 is:
   ```
   no version matching "99.99.99" found
   ```

   This is a clear string mismatch. The test will fail every time this code path is hit.

The badge reads green right now because recent push-triggered runs had path filters that
skipped Go test jobs (only recipe TOML files changed). The nightly run will exercise the
Functional Tests job and the badge will go red again.

This is no longer just "investigate the badge." There is a specific, verifiable bug: the
error message format in `internal/version/resolve.go` does not match what the functional
test expects.

---

## 2. Is this a duplicate?

**No.** No existing issue tracks this specific mismatch.

---

## 3. Is the scope still appropriate?

**Yes, and it is now more tightly bounded.**

The original scope was "investigate why the badge is failing." With the root cause identified,
the scope narrows to a single fix: align the error message in `resolve.go` (or the test
expectation) so the scenario passes. A single issue handles this cleanly.

The fix has two valid approaches:
- Change `resolve.go` line 48 to produce `"version 99.99.99 not found"` (matches the
  test's expectation and is more user-friendly)
- Change the feature file to expect the current message

The first option is preferable from a user-facing quality standpoint: "version X not found"
reads more naturally than "no version matching X found." It also aligns with the error
message format used by every other provider in the codebase (e.g., `provider_github.go`,
`provider_npm.go`, `resolver.go`).

---

## 4. Are we ready to draft?

**Yes**, with one concern.

The fix itself is trivial (one-line string format change), but the issue should be worded so
the implementer knows which of the two approaches is recommended, and why. Without that
guidance, someone might patch the feature file to match the current (inconsistent) message,
which fixes the test but leaves a poor user-facing error string.

Additionally, I'd flag that the exit code expected by the test (6, `ExitInstallFailed`) should
be verified against what the code actually returns when version resolution fails. If that
path returns a different exit code, there may be a second mismatch. This is worth calling
out in the issue as a verification step.

---

## 5. What context should be in the issue?

- **Symptom**: Tests badge on main goes red after each nightly run
- **Affected job**: Functional Tests (`test.yml`, job `functional-tests`)
- **Affected scenario**: `Install with invalid version shows clear error`
  (`test/functional/features/install.feature`, line 51)
- **Root cause**: Error message mismatch
  - Test expects: `"version 99.99.99 not found"`
  - Code produces: `"no version matching \"99.99.99\" found"`
  - Source: `internal/version/resolve.go`, line 48
- **Why the badge looks green right now**: Recent pushes only touched recipe TOML files;
  path filters skip Go jobs. The nightly run does not have path filters and will re-expose
  the failure.
- **Recommended fix**: Update `resolve.go` line 48 to use the format
  `"version %s not found"` (consistent with every other provider's error format)
- **Verification step**: Confirm the exit code returned when version resolution fails via
  `ResolveWithinBoundary` matches `ExitInstallFailed` (6), as expected by the feature test
- **Acceptance criteria**: Functional Tests job passes on the next nightly run; badge stays
  green after a nightly run without code changes

---

## Summary

The research substantially sharpens the issue. What started as "badge is failing" is now a
specific, reproducible bug with a one-line fix. The issue is not a duplicate, scope is
appropriate for a single issue, and we are ready to draft. The draft should recommend fixing
the message in `resolve.go` rather than weakening the test, and should flag the exit code
verification step.
