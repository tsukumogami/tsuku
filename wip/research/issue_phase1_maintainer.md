# Maintainer Analysis: Test Badge Showing as Failing

## Issue Evaluation

### 1. Is the problem clearly defined?

Partially. The report states the test badge in the README is showing as failing, which is observable and specific enough to act on. However, it doesn't say:
- What the badge actually shows (failing vs. no status vs. unknown)
- Whether the underlying workflow actually failed, or just the badge is stale/misconfigured
- When it started showing as failing
- Whether anyone verified this is not a real test failure

Clear enough to investigate, but not enough to classify as a config bug vs. a real test failure without checking CI.

### 2. Type

Chore (most likely), or bug — depending on root cause:
- If the badge URL is misconfigured: chore (fix the README)
- If the workflow name changed and the URL is stale: chore
- If the workflow is genuinely failing: bug (in Go code, a recipe, or the workflow itself)

The badge itself is cosmetic, but a failing test workflow is always a bug.

### 3. Scope

Yes. Fixing a badge or investigating a CI failure is a single, bounded task. Even if the root cause turns out to be a real test failure, diagnosing and fixing it is still one issue.

### 4. Gaps and ambiguities

Key gaps:
- **Is the workflow actually failing?** The badge reflects the last run. If the workflow hasn't run recently (e.g., no Go files changed since the last merge), GitHub shows the last run status, which could be a stale failure.
- **Which jobs failed?** `test.yml` has multiple jobs: `unit-tests`, `lint-tests`, `functional-tests`, `rust-test`, `llm-integration`, `llm-quality`, `validate-recipes`, `integration-linux`, `integration-macos`. A failure in any of these makes the badge go red.
- **Did the workflow name change?** The badge URL in README.md uses `test.yml` and the workflow `name: Tests`. Both match the current file, so this is not the cause.
- **Path filters:** `test.yml` only triggers on `push` to `main` when specific paths change. If recent commits to main only touched non-Go files, the workflow may not have run at all — and GitHub shows the last run result, which could be old.
- **Schedule:** The workflow runs nightly at 00:00 UTC. A nightly failure would make the badge go red without any code change.

### 5. Likely causes (ranked by probability)

1. **Nightly scheduled run failed** -- The workflow runs on a cron schedule. External factors (GitHub API rate limits hitting the `GITHUB_TOKEN`, a tool download URL returning 404, a recipe validation failure) can cause a red badge without any code change. This is the most common cause for mature repos.
2. **Real test failure on a recent push** -- A code change broke unit tests, lint, or functional tests on the last push to main.
3. **Path filter caused the workflow to be skipped, showing stale failure** -- If the last workflow run failed and subsequent pushes only modified docs or website files, the badge stays red because the workflow didn't re-run to clear it.
4. **Wrong badge branch** -- The badge URL in README.md doesn't specify a branch, so it defaults to the default branch (main). This is correct and not the issue.
5. **Workflow file renamed** -- `test.yml` still exists and the badge URL matches. Not the cause.

### 6. Fix complexity

- If it's a badge URL misconfiguration: 5 minutes.
- If it's a stale failure from a flaky nightly (e.g., rate limits): re-running the workflow clears it; root cause fix depends on what failed.
- If it's a real test failure: depends entirely on what broke. Could be minutes or hours.

The issue reporter should check the GitHub Actions tab for `test.yml` before filing, and include the specific job and error output.

## Recommended Title

```
fix(ci): investigate and resolve failing Tests badge on main
```

If the root cause turns out to be a specific thing:
- Badge URL wrong: `fix(readme): update Tests badge URL`
- Nightly flake: `fix(ci): handle rate-limit failures in nightly test run`
- Real failure: `fix(<package>): <specific failing test>`

## Summary for Filing

The issue is clear enough to investigate but should include a link to the failing workflow run. The most likely causes are a failed nightly scheduled run or a stale failure from a skipped push run. The badge URL and workflow name are correctly matched in the current codebase, so that's not the culprit. The reporter should check the GitHub Actions tab and link the specific failed run before filing.
