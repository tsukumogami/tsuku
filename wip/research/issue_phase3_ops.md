# Ops/CI Phase 3 Review: Test Badge Failing (Post-Research Evaluation)

## 1. Does the Research Change My Understanding?

**Yes, significantly.** Phase 1 identified the nightly schedule as the most likely suspect but could not confirm root cause without CI access. The phase 2 research has now confirmed two concrete failures:

### Confirmed Failure 1: Validate Recipes (already fixed)

Batch-added recipes had invalid `unsupported_platforms` entries. The `validate-recipes` job caught this. This is already resolved — no action needed.

### Confirmed Failure 2: Functional Tests (still present)

Scenario "Install with invalid version shows clear error" (`features/install.feature:51`) expects:

```
version 99.99.99 not found
```

The actual error message returned by `internal/version/resolve.go:48` is:

```
no version matching "99.99.99" found
```

This is a **test expectation mismatch** — the Go code's error format changed at some point and the Gherkin expectation was not updated to match. The fix is a one-line change to `test/functional/features/install.feature:54`.

This changes my analysis from "investigate and find the root cause" to "we know the root cause, fix it." The issue can now be filed with full specificity rather than as an investigation task.

---

## 2. Is This a Duplicate?

**No.** No existing issue covers this specific mismatch between the functional test expectation and the version resolution error message. The research confirmed no duplicates were found.

---

## 3. Is the Scope Still Appropriate?

**Yes, and it can be narrowed.** The original title was appropriate for an investigation issue. Now that root cause is confirmed, the scope is even more clearly a single-file, one-line fix. The two failures (recipe validation + functional test mismatch) are causally independent. Failure 1 is already fixed. Failure 2 is the remaining actionable item.

The scope is a single issue. No splitting needed.

---

## 4. Are We Ready to Draft?

**Yes.** All the information needed for a precise, actionable issue is in hand:

- Which workflow: `test.yml`, `Functional Tests` job
- Which scenario: `Install with invalid version shows clear error` (`install.feature:51`)
- Which file to fix: `test/functional/features/install.feature`, line 54
- What to change: `"version 99.99.99 not found"` → `"no version matching \"99.99.99\" found"` (or, better, just `"no version matching"` as a substring match to avoid brittle quoting)
- Why it broke: the error format in `internal/version/resolve.go:48` changed and the test was not updated

One concern: the issue should verify whether `install.feature:54` uses a substring match or exact match in the step definition. If it uses `contains`, then only the substring needs to match, which means we could pick a more stable fragment. Either way the fix is trivial.

---

## 5. What Context Should Be in the Issue?

### Title

```
fix(ci): functional test expects stale error message for invalid version install
```

This is now specific enough to name the actual problem, not just the symptom.

### Body should include

**Symptom**
- The `Tests` badge on main is red.
- The `Functional Tests` job in `test.yml` is failing on nightly runs.

**Root Cause**
- Scenario "Install with invalid version shows clear error" (`test/functional/features/install.feature:51`) asserts the error output contains `"version 99.99.99 not found"`.
- `internal/version/resolve.go:48` returns `fmt.Errorf("no version matching %q found", requested)`, producing `no version matching "99.99.99" found`.
- The two strings do not match, so the assertion fails and the job exits non-zero.

**Fix**
- Update `test/functional/features/install.feature:54` to match the actual error message format.
- Preferred: use a stable substring like `"no version matching"` rather than the full quoted form, to avoid future brittleness if quoting style changes again.

**Verification**
- Run the functional test suite locally or trigger `test.yml` manually on main after the fix.
- Confirm the nightly run passes.

**Not in scope**
- The recipe validation failure (already fixed by the batch recipe commits).
- Badge URL changes (URL is correct, not the cause).

### Labels / Metadata

- Type: `bug`
- Component: `ci` (or `testing`)
- Priority: high (badge is public-facing signal of health)

---

## Ops/CI Perspective Notes

From a CI operations standpoint, this failure pattern -- test expectation not updated when error message format changes -- is a maintenance gap, not a flaky test. It will fail every nightly run until fixed and will not self-heal. The badge will remain red.

The fact that push-triggered runs appear green is a path-filter artifact: Go code changes trigger the test jobs, but the most recent pushes (recipe batch commits) only touched TOML files. The functional test job ran last on a nightly, not on push, so the push-triggered badge state is misleading. This is worth noting in the issue so the team understands why they might think it's passing when it isn't.

One hardening recommendation to include as a follow-up (not in scope for this fix): consider adding a step to the workflow that explicitly fails if no test jobs ran, to prevent path-filter blind spots from masking nightly failures in the badge.
