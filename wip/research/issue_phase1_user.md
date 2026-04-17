# Issue Assessment: Failing Test Badge in README

Role: User of the tsuku CLI tool visiting the GitHub repo

---

## 1. Is the problem/request clearly defined?

**Yes**, with minor caveats.

The report states the Tests badge in the README is showing as failing. The badge links to
`https://github.com/tsukumogami/tsuku/actions/workflows/test.yml`, which is the main CI
workflow. From a user's perspective, a red badge on the project page is immediately visible
and signals that the tool may be broken or unstable. The problem is concrete: a specific
badge on a specific file is in a failing state.

The only gap is that the report does not say *which job* within `test.yml` is failing, or
whether the failure is transient (flaky test, external dependency) vs. systematic (broken
code, misconfigured workflow). That detail matters for resolution but does not block filing
the issue.

---

## 2. What type is this?

**Bug** (infrastructure/CI category).

The badge reflects CI health. A failing badge means either tests are actually broken, or a
workflow is misconfigured in a way that produces false failures. Either way it is broken
behavior that needs a fix, not a new capability.

---

## 3. Is the scope appropriate for a single issue?

**Yes.**

The scope is narrow: investigate why `test.yml` is failing and restore green CI. A single
issue is the right container for this. The fix might touch test code, a workflow step, or
an external dependency, but the goal is singular and well-bounded.

---

## 4. Gaps and ambiguities

- **Which job is failing?** `test.yml` has multiple jobs: unit-tests, lint-tests,
  functional-tests, rust-test, llm-integration, llm-quality, validate-recipes,
  integration-linux, integration-macos. The report does not say which one is red.
- **Is this a new regression or a long-standing failure?** Unknown.
- **Transient vs. systematic?** External network calls (recipe downloads, GitHub API) can
  cause flakes that look like test failures on the badge.
- **Scheduled run vs. push?** The workflow runs on push, pull_request, and a nightly
  schedule. A badge that reflects a nightly run failure may not indicate broken HEAD code.

These are investigation items, not blockers for filing. The issue should call out that
diagnosis is the first step.

---

## 5. Recommended title

```
fix(ci): restore failing Tests badge on main
```

Rationale: `fix` is appropriate because CI is broken (not a new feature). `ci` scope targets
the workflow. "restore failing Tests badge" is explicit about the symptom and the desired
outcome. Conventional commits format keeps it consistent with the existing commit style in
this repo (see recent commits: `chore(dashboard): ...`, `chore(batch): ...`).
