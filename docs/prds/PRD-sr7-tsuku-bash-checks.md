---
schema: prd/v1
status: Draft
problem: |
  Two bash PR-context document checks, the corpus-wide strikethrough check and
  the closing-issue diagram-status check, still run on every tsuku pull request
  beside the shirabe engine's validate-docs workflow. They guard an old
  DESIGN-doc issues-table and diagram shape that new planning no longer uses,
  one repeats a rule the kept mermaid check enforces, and neither has failed
  since 2026-04-28, so contributors and maintainers can't tell which home a
  document rule lives in.
goals: |
  Every document rule a tsuku pull request meets comes from the shirabe engine,
  except the mermaid diagram check that's deliberately deferred. Each of the two
  old checks ends with a written disposition in this topic's design: absorbed
  into the engine with a capture showing the same verdicts, or retired with
  what it caught and why nothing replaces it.
upstream: docs/briefs/BRIEF-sr7-tsuku-bash-checks.md
---

# PRD: End the duplicate bash document checks in tsuku CI

## Status

Draft

## Problem Statement

tsuku runs shirabe's document engine through
`.github/workflows/validate-docs.yml`, and that's meant to be where document
rules live. Two older bash checks still run beside it on every pull request:

- `.github/scripts/checks/strikethrough-all-docs.sh`, run by
  `.github/workflows/validate-closing-issues.yml` (job display name "Validate
  Closing Issues in Design Docs"). For each issue a pull request closes, every
  non-archive `DESIGN-*.md` that lists the issue in its Implementation Issues
  table must show it struck through.
- `.github/scripts/check-issue-status.sh`, run by
  `.github/workflows/validate-diagram-status.yml` ("Validate Diagram Status for
  Closing Issues"). For each issue a pull request closes, every DESIGN doc with
  a mermaid block that contains `[#N]` must give node `I<N>` the `done` class.

Both guard a DESIGN-doc shape the project has moved past. The engine's model
gives a DESIGN no issues table; issue tracking lives in PLAN and ROADMAP
documents, where the engine's FC07, FC08 and FC09 reconcile the table, the
diagram and issue state. New planning in tsuku lands in `docs/plans/`, which
neither script reads. The diagram-status rule also repeats `mermaid.sh`'s MM15,
which already fails a changed design doc whose diagram doesn't mark a closing
issue `done`.

The checks did work when they fired. Failure figures, measured on 2026-10-30
with `gh run list`:

| Check | Most recent 1000 runs | Failed runs in that window | PRs those failures span | Failed runs in retained history | Last failure |
|---|---|---|---|---|---|
| Closing issues (strikethrough) | 2026-04-13 to 2026-10-30 | 18 | 12 | about 322, from 2026-01-27 | 2026-04-28 |
| Diagram status | 2026-03-23 to 2026-10-30 | 21 | 5 | about 122, from 2026-01-28 | 2026-04-28 |

Every failure in the two windows was a real bookkeeping slip (a closing issue
left unstruck, or its node not `done`) fixed by a follow-up doc edit. Eight of
the twelve strikethrough PRs were curated-recipe batches that forgot to strike
a row in `docs/designs/DESIGN-curated-recipes.md`.

Today 145 non-archive DESIGN docs exist; one, `DESIGN-curated-recipes.md`, has
`class I<N>` diagram assignments. The only open issues still listed unstruck
in a DESIGN issues table the strikethrough script can parse are #204, #205,
#207 and #208, in `docs/designs/current/DESIGN-version-verification.md`. A
contributor whose pull request closes one of them would get a red job asking
them to edit a 2025 design doc they never touched, with nothing saying whether
the engine knows the rule. Each document rule has two possible homes, and
nothing records which is real.

## Goals

A contributor's pull request meets document checks from the engine, plus the
mermaid diagram check that's kept for now.

A maintainer can answer "is this rule still enforced, and where?" by reading
one place: the disposition table in this topic's design,
`docs/designs/DESIGN-sr7-tsuku-bash-checks.md` (planned). Each row names the
check, its disposition and the reason, and points at the engine rule or the
removing pull request.

## User Stories

- As a tsuku contributor whose pull request says `Fixes #N`, I want the
  document checks I see to be the engine's checks plus the mermaid check, so
  that a bash script outside the engine never fails my pull request over a
  design doc. (Verified by the R1 and R9 criteria.)
- As a tsuku maintainer changing how closed issues are marked in planning
  documents, I want the rule to have one home, so that no tsuku script keeps
  enforcing an old form beside the engine. (Verified by the R1, R6 and R7
  criteria.)
- As a reviewer who notices "Validate Closing Issues in Design Docs" and
  "Validate Diagram Status for Closing Issues" no longer report on pull
  requests, I want to find what each check caught and what replaced it (or why
  nothing did) by reading the design's disposition table or the removing pull
  request, so that I can tell a deliberate retirement from lost coverage.
  (Verified by the R3 and R4 criteria.)

## Requirements

### Functional

- **R1.** No workflow under `.github/workflows/` runs
  `.github/scripts/checks/strikethrough-all-docs.sh` or
  `.github/scripts/check-issue-status.sh`, and no script that a workflow runs
  sources or executes either one.
- **R2.** Each of the two checks gets exactly one disposition, absorbed or
  retired.
- **R3.** The dispositions are recorded in a table in
  `docs/designs/DESIGN-sr7-tsuku-bash-checks.md` with one row per check and the
  columns Check, Disposition (`absorbed` or `retired`), Reason, and Where (the
  engine check code for an absorbed check, or "removing pull request" for a
  retired one).
- **R4.** An absorbed check comes with a parity capture in shirabe, in the
  style of the real-bash captures under
  `crates/shirabe/tests/fixtures/absorption-golden/`. Parity means: on every
  input in the capture, the bash script and the engine agree on pass or fail,
  and on the set of (issue, document) pairs they flag. The capture's inputs
  include at least: a closing issue struck through (pass), the same issue
  unstruck (fail), a document that doesn't list the issue (no finding), an
  archive document listing it unstruck (ignored), an issue listed unstruck
  in two documents (both flagged), and, for the diagram check, a node marked `done` (pass) and one
  marked `ready` (fail).
- **R5.** A retired check is explained in the body of the pull request that
  removes it: what it enforced, its row from the failure table above (window,
  failed runs, PRs spanned, retained-history count, last failure date, labelled
  as windowed counts), and why nothing replaces it.
- **R6.** Files that exist only to serve a removed check are removed with it.
  The candidates are `.github/scripts/checks/issue-status.sh` (run by no
  workflow), `.github/scripts/get-github-status.sh` (called only by
  `issue-status.sh`), and `.github/scripts/docs/IS01.md` and `IS02.md`
  (referenced only by `issue-status.sh`). Each is removed when a search for
  its repository path finds no reference from a file that stays, other than
  this topic's own BRIEF, PRD, DESIGN and PLAN; otherwise it is kept and the
  pull request body names the file that still uses it.
- **R7.** No tsuku file outside this topic's own BRIEF, PRD, DESIGN and PLAN, and
  outside `docs/designs/archive/`, describes a removed check as running. Today
  that means updating `.claude/shirabe-extensions/work-on.md`, which lists
  "closing issues, diagram status" among the pull-request-context checks.
- **R8.** `.github/scripts/checks/mermaid.sh`,
  `.github/workflows/validate-diagram-classes.yml`,
  `.github/scripts/extract-design-issues.sh`,
  `.github/scripts/extract-closing-issues.sh` and
  `.github/scripts/checks/common.sh` are left byte-for-byte unchanged.
- **R9.** An absorbed rule keeps the bash check's reach (every non-archive
  document, not only changed ones), since that's what parity means; the design
  must say how the engine receives the pull request's closing issues. A
  retired check needs no engine change.

### Non-functional

- **R10.** No engine regression: `shirabe validate` still reports FC15, and
  FC05, FC09, FC15 and L07 behave as before. On a path with no shirabe change
  this holds by construction; on a path with one, shirabe's existing tests for
  those checks pass unchanged.
- **R11.** Repository CI linters that enumerate workflows or scripts
  (`.github/scripts/checks/workflow-policy_test.sh`,
  `.github/scripts/checks/ci-patterns-lint.sh`) pass after the change.
- **R12.** No pull request is blocked by the change: neither removed workflow
  is a required status check on `main`. On 2026-10-30 the only ruleset on
  `main` carried deletion, non-fast-forward and Copilot-review rules, and
  branch protection was not set; this is re-checked before merge.
- **R13.** Public-repository content rules hold for every commit message, pull
  request body and doc the work produces: no reference to a private
  repository or private document, no `wip/` paths, no AI attribution, no
  emojis, and tsuku's writing-style word list respected.
- **R14.** Pull request structure follows the dispositions. Both retired: one
  tsuku pull request and no shirabe change. Any check absorbed: a shirabe pull
  request adds the rule and its capture and merges first; then the tsuku pull
  request removes the script. tsuku's `validate-docs.yml` calls shirabe's
  workflow at `@main`, so no pin bump sits between them.

## Acceptance Criteria

- [ ] `git grep -n -e strikethrough-all-docs -e check-issue-status -- .github/`
      returns no match on the branch after the change.
- [ ] `git grep -l -e 'scripts/checks/' -e 'scripts/check-' --
      .github/workflows/` lists exactly the files it lists on `main` minus
      `validate-closing-issues.yml` and `validate-diagram-status.yml`, and of
      the files it lists, `validate-diagram-classes.yml` is the only one whose
      `pull_request` paths filter matches `docs/designs/`.
- [ ] `docs/designs/DESIGN-sr7-tsuku-bash-checks.md` has a disposition table
      with exactly two rows, one per check, each with Disposition set to
      `absorbed` or `retired`, a non-empty Reason, and a Where cell naming an
      engine check code (absorbed) or "removing pull request" (retired).
- [ ] For each check marked `absorbed`, the design has a section naming how
      the engine receives the pull request's closing issues, and the capture
      includes a case where the flagged document is not changed by the pull
      request and both bash and engine flag it.
- [ ] When any check is absorbed, the shirabe pull request merged before the
      tsuku pull request that removes the script.
- [ ] For each check marked `absorbed`, a merged shirabe pull request adds a
      capture under `crates/shirabe/tests/fixtures/absorption-golden/` covering
      every input listed in R4, and the capture's test passes with bash and
      engine verdicts in agreement.
- [ ] For each check marked `retired`, the removing pull request's body names
      the check, what it enforced, the check's row of the failure table
      labelled as windowed counts, and the reason nothing replaces it.
- [ ] For each file in R6, either the file is gone and `git grep -n -F
      -e checks/issue-status.sh -e get-github-status.sh -e docs/IS01.md
      -e docs/IS02.md -- ':!docs/briefs/BRIEF-sr7-tsuku-bash-checks.md'
      ':!docs/prds/PRD-sr7-tsuku-bash-checks.md'
      ':!docs/designs/DESIGN-sr7-tsuku-bash-checks.md'
      ':!docs/plans/PLAN-sr7-tsuku-bash-checks.md'` returns no line naming
      it, or the file remains and the pull request body names the remaining
      file that uses it.
- [ ] `git grep -n -i -e validate-closing-issues -e validate-diagram-status
      -e "Validate Closing Issues" -e "Validate Diagram Status" --
      ':!docs/designs/archive' ':!docs/briefs/BRIEF-sr7-tsuku-bash-checks.md'
      ':!docs/prds/PRD-sr7-tsuku-bash-checks.md'
      ':!docs/designs/DESIGN-sr7-tsuku-bash-checks.md'
      ':!docs/plans/PLAN-sr7-tsuku-bash-checks.md'` returns no match, and
      `.claude/shirabe-extensions/work-on.md` no longer lists closing issues or
      diagram status among checks.
- [ ] `git diff $(git merge-base origin/main HEAD) --
      .github/scripts/checks/mermaid.sh
      .github/workflows/validate-diagram-classes.yml
      .github/scripts/extract-design-issues.sh
      .github/scripts/extract-closing-issues.sh
      .github/scripts/checks/common.sh` is empty.
- [ ] `.github/scripts/checks/workflow-policy_test.sh` and
      `.github/scripts/checks/ci-patterns-lint.sh` exit 0 on the branch.
- [ ] `shirabe validate --check FC15` reports FC15 on a PRD whose required
      sections are out of order, and reports nothing on this topic's PRD.
- [ ] If a shirabe pull request exists, shirabe's test suite passes on it and
      no existing fixture or expected output for FC05, FC09, FC15 or L07
      changes in its diff.
- [ ] Before merge, `gh api repos/tsukumogami/tsuku/rules/branches/main`
      shows no `required_status_checks` rule naming either removed workflow's
      job.
- [ ] When both checks are retired, the work lands as one tsuku pull request
      and no shirabe pull request.
- [ ] Every check that runs on the pull request's head commit passes,
      including `validate-docs.yml` on the documents the pull request adds.
- [ ] Across the files the pull request adds or changes (read with `git
      diff`), its commit messages (`git log --format=%B`) and its body (`gh pr
      view --json body`): no `wip/` path appears; no
      `Co-Authored-By` or "Generated with" line appears; no emoji appears; no
      `tsukumogami/` repository is named other than tsuku, shirabe, koto,
      niwa, dot-niwa and .github; and `shirabe validate` reports no banned-word
      finding.

## Out of Scope

- The mermaid diagram check (`mermaid.sh`, `validate-diagram-classes.yml`)
  and the helpers it uses. Its absorption is deferred because it would pull a
  diagram grammar into the engine; it stays as is.
- Rewriting old DESIGN docs: striking stale issue rows, moving issue tables
  into PLANs, or fixing `DESIGN-curated-recipes.md`'s diagram. The corpus keeps
  its historical shape.
- A new cross-document rule in the engine beyond what parity with an absorbed
  script needs. The engine's own design defers a corpus-wide strikethrough rule
  until its semantics are stated.
- The pull-request body check (`validate-pr-body.yml`), a different concern.

## Known Limitations

- If a check is retired, a pull request that closes an issue still listed
  unstruck in an unchanged DESIGN doc passes silently. Today that's #204,
  #205, #207 and #208 in `DESIGN-version-verification.md`. A changed design
  doc with a diagram stays covered by MM15.
- `DESIGN-curated-recipes.md` lists open issue #2349 unstruck with
  `class I2349 blocked`, in a row format (`[#2349: title](url)`) that
  `extract-design-issues.sh` doesn't parse and a label the `[#N]` grep doesn't
  match. Neither bash check would catch a pull request closing #2349 today, so
  retiring them loses nothing there; MM15 covers it when that doc changes.
- Run logs older than GitHub's retention are gone, so failures before the
  1000-run windows can't be classified, and the retained-history counts may
  themselves be truncated at the early end.

## Decisions and Trade-offs

- **Absorb or retire is left to the design.** The requirements hold either
  way, and the choice turns on how the engine models DESIGN docs, which is a
  design question. Alternative: decide here; rejected because it would settle
  architecture in a requirements document. The design records its choice in
  the disposition table R3 names.
- **Parity keeps the bash check's reach.** An absorbed rule would still fire
  on unchanged documents, which works against the contributor goal; R9 makes
  that explicit so the design weighs it instead of narrowing the rule quietly.
  Alternative: allow an absorbed rule to cover changed files only; rejected
  because the capture could then never show parity.
- **Failure counts are stated as windowed counts.** The 18 and 21 figures are
  failures in the most recent 1000 runs of each workflow, the API cap;
  retained history shows about 322 and 122. Alternative: quote 18 and 21 as
  lifetime totals; rejected as inaccurate.
- **Helpers go only after a search.** R6 removes `issue-status.sh` and its
  helpers rather than leaving dead code, but only once a whole-tree search
  shows no caller. Alternative: leave them; rejected because an unrun check
  beside a retired one is the same two-homes problem.
