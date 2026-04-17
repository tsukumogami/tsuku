# Ops/CI Review: Test Badge Failing in README

## Issue Description

The test badge in the tsukumogami/tsuku GitHub repo README is showing as failing.

---

## 1. Is the Problem/Request Clearly Defined?

**Partially** -- the symptom is clear (badge shows failing), but the root cause is not stated. From a CI engineer's perspective, a failing badge has several distinct causes that require different fixes. The report is sufficient to open an investigation issue, but whoever files it should specify which job is failing (if known) or note that the cause is unknown.

---

## 2. Type

**Bug** -- the badge should reflect the current test status of the main branch, and displaying "failing" when tests are (presumably) expected to pass is a regression in CI visibility.

---

## 3. Is the Scope Appropriate for a Single Issue?

**Yes** -- fixing a badge URL or CI configuration is a narrow, self-contained change. It touches at most one or two files (README.md, possibly the workflow YAML), and can be verified by a single passing run.

---

## 4. Gaps and Ambiguities

### Badge URL

README.md line 3:

```
[![Tests](https://github.com/tsukumogami/tsuku/actions/workflows/test.yml/badge.svg)](https://github.com/tsukumogami/tsuku/actions/workflows/test.yml)
```

The badge URL uses the default branch filter (no `?branch=` param), which defaults to the repository's default branch. This is correct as long as `main` is the default branch, but it means the badge reflects the last run targeting that branch -- not per-PR runs.

### Workflow Path Filter (Critical Finding)

The `test.yml` workflow has path filters on both `push` and `pull_request` triggers:

```yaml
on:
  push:
    branches: [main]
    paths:
      - '**/*.go'
      - 'go.mod'
      - 'go.sum'
      - 'test/scripts/**'
      - 'test/functional/**'
      - 'test-matrix.json'
      - 'testdata/**'
      - '.github/workflows/test.yml'
      - 'recipes/**/*.toml'
      - 'internal/recipe/recipes/**/*.toml'
```

**If the most recent push to `main` only touched files outside these paths** (e.g., documentation, website/, telemetry/, scripts, README.md), the workflow would not run at all. When a workflow has never run -- or has not run since a branch was renamed -- GitHub displays the badge as "no status," which many clients render as failing or as a grey/unknown state.

More critically: if the most recent run that *did* trigger actually failed (any job in the workflow), the badge stays red until a new successful run occurs on `main`.

### The `matrix` + `if` Condition Chain

Jobs in `test.yml` use a two-level gate:

1. The `matrix` job runs `dorny/paths-filter` to set `code`, `rust`, `llm`, etc.
2. Downstream jobs have `if: ${{ needs.matrix.outputs.code == 'true' }}` (or similar).

If no code files changed on the last `main` push that triggered the workflow, `code` outputs `false` and all gated jobs are **skipped**. GitHub Actions counts a workflow run where all jobs are skipped as "success," so this should not cause a red badge -- but it's worth confirming the last run status directly.

### Workflow Name Mismatch

The workflow file is named `test.yml` and `name: Tests` (line 1). The badge URL references `test.yml`, which matches. No mismatch here.

### Branch Targeting

The badge URL has no explicit `?branch=main` query parameter. Adding it is a minor hardening improvement but is not the cause of a failure state.

### Schedule Trigger

The workflow runs nightly (`0 0 * * *`) regardless of path filters. If nightly runs are failing (e.g., flaky integration tests, expired tokens, rate limits), the badge will show failing even when the code itself is fine.

---

## 5. Recommended Issue Title

```
fix(ci): investigate and resolve failing Tests badge on main
```

---

## Diagnosis Checklist for the Filer

The issue should include a check of these items (or note which one was confirmed as the root cause):

1. **Last run status** -- go to Actions > Tests and check the most recent run on `main`. Is it actually failing, or "no status"?
2. **Failing job** -- if failing, which job? (`unit-tests`, `lint-tests`, `functional-tests`, `integration-linux`, `integration-macos`, `validate-recipes`, or one of the scheduled/nightly jobs?)
3. **Nightly schedule** -- are nightly runs failing? Likely causes: expired secrets (`CODECOV_TOKEN`, `GITHUB_TOKEN` rate limits), flaky integration tests, or broken external deps.
4. **Path filter gap** -- was the last `main` push docs-only? If so, the badge may show stale state from a prior failing run.
5. **Branch name** -- is `main` still the default branch? (Unlikely to have changed, but worth confirming.)

---

## Summary for Issue Filing

- **Type**: bug
- **Scope**: single issue, narrow fix
- **Primary suspects**: (a) a failing nightly run due to expired/rate-limited secrets or flaky integration tests, (b) stale badge state from a prior failing run that was never cleared by a successful push-triggered run
- **Not a badge URL problem**: the URL format in README.md is correct
- **Recommended title**: `fix(ci): investigate and resolve failing Tests badge on main`
