---
schema: plan/v2
status: Active
execution_mode: single-pr
split_mode_source: none
tracking_level: none
upstream: docs/designs/DESIGN-sr7-tsuku-bash-checks.md
milestone: "Retire duplicate bash document checks"
step_count: 2
---

# PLAN: Retire the duplicate bash document checks in tsuku CI

## Status

Active

## Scope Summary

Retires the two pull-request-context bash document checks that run beside the
shirabe engine, the corpus-wide strikethrough check and the closing-issue
diagram-status check, with their workflows and the unrun strikethrough checker
they leave behind, in one tsuku pull request. The mermaid diagram check and its
helpers stay untouched, and nothing in shirabe changes.

## Decomposition Strategy

Walking skeleton. Step 1 removes the two running checks end to end, from each
workflow file to the script it runs, on the repository tree at the branch head,
so a pull request stops reporting either job while CI's own linters keep
passing. Step 2 builds on that: it removes the dead companions so no copy of the
retired rule remains, fixes the one pointer that still names the checks,
confirms no merge gate names either job, and records both retirements in the
pull request body.

## Steps

### Step 1: a pull request no longer runs the two duplicate document checks

**Goal**: A pull request stops running the two bash document checks that repeat rules the engine and the mermaid check already hold. This crosses from the workflow files to the scripts they run, and the input is the repository tree at the branch head: with both workflows and both scripts gone, nothing under `.github/` names either script and neither job reports.

**Acceptance Criteria**:

- [ ] `git grep -n -e strikethrough-all-docs -e check-issue-status -- .github/` prints nothing and exits 1.
- [ ] `.github/scripts/checks/workflow-policy_test.sh` exits 0 on the branch head.
- [ ] `.github/scripts/checks/ci-patterns-lint.sh` exits 0 on the branch head.
- [ ] The diff against the merge base for `.github/scripts/checks/mermaid.sh`, `.github/workflows/validate-diagram-classes.yml`, `.github/scripts/extract-design-issues.sh`, `.github/scripts/extract-closing-issues.sh` and `.github/scripts/checks/common.sh` is empty.
- [ ] On the pushed pull request, the checks list shows no "Validate Closing Issues in Design Docs" job and no "Validate Diagram Status for Closing Issues" job.
- [ ] On the pushed pull request, the "Validate doc formats" job still runs.

**Check**: From the repository root on the branch, run `git grep -n -e strikethrough-all-docs -e check-issue-status -- .github/` and see no output. Run `.github/scripts/checks/workflow-policy_test.sh` and `.github/scripts/checks/ci-patterns-lint.sh` and see exit code 0 from each. Run `git diff $(git merge-base origin/main HEAD) -- .github/scripts/checks/mermaid.sh .github/workflows/validate-diagram-classes.yml .github/scripts/extract-design-issues.sh .github/scripts/extract-closing-issues.sh .github/scripts/checks/common.sh` and see no output. Then push the branch, open the pull request, and read its checks list (`gh pr checks <pr>`): neither "Validate Closing Issues in Design Docs" nor "Validate Diagram Status for Closing Issues" appears, and "Validate doc formats" is listed and runs.

**Dependencies**: None

**Files**: `.github/workflows/validate-closing-issues.yml`, `.github/workflows/validate-diagram-status.yml`, `.github/scripts/checks/strikethrough-all-docs.sh`, `.github/scripts/check-issue-status.sh`

### Step 2: no copy of the retired rule remains and each retirement is on record

**Goal**: Remove the unrun `.github/scripts/checks/issue-status.sh`, its only helper `.github/scripts/get-github-status.sh`, and `.github/scripts/docs/IS01.md` and `IS02.md`; change the `.claude/shirabe-extensions/work-on.md` bullet "The PR-context checks (PR body, closing issues, diagram status)" to "The PR-context checks (PR body, and the closing-issue rule of the diagram-class check)"; confirm no merge gate names either removed job; and open the pull request whose body records both retirements.

**Acceptance Criteria**:

- [ ] A case-insensitive search of the whole tree for `strikethrough-all-docs`, `check-issue-status`, `checks/issue-status.sh`, `get-github-status`, `docs/IS01.md`, `docs/IS02.md`, `validate-closing-issues`, `validate-diagram-status`, "Validate Closing Issues" and "Validate Diagram Status", excluding `docs/designs/archive`, `docs/prds/PRD-sr7-tsuku-bash-checks.md`, `docs/designs/DESIGN-sr7-tsuku-bash-checks.md` and `docs/plans/PLAN-sr7-tsuku-bash-checks.md`, prints nothing.
- [ ] `extract-design-issues.sh`, `extract-closing-issues.sh` and `checks/common.sh` each still appear in `mermaid.sh` or `validate-diagram-classes.yml`, so the kept diagram check still has every helper it calls.
- [ ] `gh api repos/tsukumogami/tsuku/rules/branches/main` lists no `required_status_checks` rule naming either removed job, and `gh api repos/tsukumogami/tsuku/branches/main/protection` returns 404 or lists no such check.
- [ ] The pull request body names, for each of the two checks, what it enforced, its windowed failure figures labelled as windowed counts, and why nothing replaces it. Strikethrough: 18 failed runs across 12 PRs in the most recent 1000 runs, window from 2026-04-13. Diagram status: 21 failed runs across 5 PRs, window from 2026-03-23. It also gives about 322 and 122 failed runs in retained history since late January 2026, and 2026-04-28 as the last failure for both.
- [ ] The pull request body names the follow-up for the `validate-diagram-classes.yml` interpolation finding (changed file names placed in a `run:` script, and the fixed `EOF` delimiter in `$GITHUB_OUTPUT`).
- [ ] The pull request's diff, commit messages and body contain no `wip/` path, no Co-Authored-By or "Generated with" line, no emoji, and no tsukumogami repository other than tsuku, shirabe, koto, niwa, dot-niwa and .github.

**Check**: Run the whole-tree search from the first criterion and see no output. Run `grep -l -E 'extract-design-issues|extract-closing-issues|common\.sh' .github/scripts/checks/mermaid.sh .github/workflows/validate-diagram-classes.yml` and see each helper referenced in at least one of the two files. Run both `gh api` queries and see no required check for either removed job (a 404 from the protection query is acceptable). Read the body with `gh pr view --json body` and find both retirement records, the windowed figures and the follow-up; then search the body, `git log` messages and `git diff` for `wip/`, `Co-Authored-By`, `Generated with` and emoji, and see no match.

**Dependencies**: Step 1

**Files**: `.github/scripts/checks/issue-status.sh`, `.github/scripts/get-github-status.sh`, `.github/scripts/docs/IS01.md`, `.github/scripts/docs/IS02.md`, `.claude/shirabe-extensions/work-on.md`

## Implementation Sequence

Step 1, then step 2, both in the same pull request. Step 2 depends on step 1
because its whole-tree search expects the two run scripts and their workflows
to be gone already.
