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
  old checks ends with a written disposition: absorbed into the engine with
  evidence that it reaches the same verdict, or retired with what it caught and
  why nothing replaces it.
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
  `.github/workflows/validate-closing-issues.yml`. For each issue a pull
  request closes, every non-archive `DESIGN-*.md` that lists the issue in its
  Implementation Issues table must show it struck through.
- `.github/scripts/check-issue-status.sh`, run by
  `.github/workflows/validate-diagram-status.yml`. For each issue a pull
  request closes, every DESIGN doc with a mermaid block that contains `[#N]`
  must give node `I<N>` the `done` class.

Both guard a DESIGN-doc shape the project has moved past. The engine's model
gives a DESIGN no issues table; issue tracking lives in PLAN and ROADMAP
documents, where the engine's FC07, FC08 and FC09 reconcile the table, the
diagram and issue state. New planning in tsuku lands in `docs/plans/`, which
neither script reads. The diagram-status rule also repeats `mermaid.sh`'s MM15,
which already fails a changed design doc whose diagram doesn't mark a closing
issue `done`.

The checks did work when they fired. In the most recent 1000 runs of each
(closing-issues since 2026-04-13, diagram-status since 2026-03-23) there were 18
and 21 failed runs, every one a real bookkeeping slip fixed by a follow-up doc
edit, spread over 12 and 5 pull requests. Retained history reaches back to
January 2026 with about 322 and 122 failed runs, clustered on a few docs
branches with many pushes. Neither has failed since 2026-04-28. Today 145
non-archive DESIGN docs exist; one, `docs/designs/DESIGN-curated-recipes.md`,
has `class I<N>` diagram assignments, and the only open issues still listed
unstruck in a DESIGN issues table the strikethrough script can parse are #204,
#205, #207 and #208, in `DESIGN-version-verification.md`.

So each document rule has two possible homes, and a contributor whose pull
request closes one of those four issues would get a red job asking them to
edit a 2025 design doc they never touched, with nothing saying whether the
engine knows the rule.

## Goals

A contributor's pull request meets document checks from the engine, plus the
mermaid diagram check that's kept for now, and nothing else asks them to edit a
design doc their change didn't touch.

A maintainer can answer "is this rule still enforced, and where?" in one
lookup. For each of the two old checks the answer is written down where the
change happened: the engine carries the rule, or the check was retired on
purpose with its reason.

## User Stories

- As a tsuku contributor whose pull request says `Fixes #N`, I want the
  document checks I see to be the engine's checks on files I changed plus the
  mermaid check, so that closing an issue never fails on an unrelated old
  design doc.
- As a tsuku maintainer changing how closed issues are marked in planning
  documents, I want to change the rule in one place, the shirabe engine, so
  that no tsuku script keeps enforcing the old form beside it.
- As a reviewer who notices "Validate Closing Issues in Design Docs" and
  "Validate Diagram Status for Closing Issues" no longer report on pull
  requests, I want to find what each check caught and what replaced it (or why
  nothing did), so that I can tell a deliberate retirement from lost coverage.

## Requirements

### Functional

- **R1.** No workflow under `.github/workflows/` runs
  `.github/scripts/checks/strikethrough-all-docs.sh` or
  `.github/scripts/check-issue-status.sh`, directly or through another script.
- **R2.** Each of the two checks gets exactly one disposition: absorbed into
  the shirabe engine, or retired.
- **R3.** An absorbed check comes with a parity capture in shirabe, in the
  style of the real-bash captures under
  `crates/shirabe/tests/fixtures/absorption-golden/`: the bash script's
  verdict and the engine's verdict on the same inputs.
- **R4.** A retired check is explained in the body of the pull request that
  removes it: what it checked, what it caught (with the counts above stated as
  windowed counts, not lifetime totals), and why nothing replaces it.
- **R5.** Files that exist only to serve a removed check are removed with it.
  The candidates are `.github/scripts/checks/issue-status.sh` (run by no
  workflow), `.github/scripts/get-github-status.sh` (called only by
  `issue-status.sh`), and `.github/scripts/docs/IS01.md` and `IS02.md`
  (referenced only by `issue-status.sh`). A file is removed only after a
  repository-wide search confirms nothing else references it.
- **R6.** `.github/scripts/checks/mermaid.sh`,
  `.github/workflows/validate-diagram-classes.yml`,
  `.github/scripts/extract-design-issues.sh`,
  `.github/scripts/extract-closing-issues.sh` and
  `.github/scripts/checks/common.sh` are left byte-for-byte unchanged.
- **R7.** No tsuku doc or skill extension describes a removed check as
  running. `.claude/shirabe-extensions/work-on.md`, which lists "closing
  issues, diagram status" among the pull-request-context checks, is updated to
  match.

### Non-functional

- **R8.** No engine regression: `shirabe validate` still reports FC15, and
  the behaviour of FC05, FC09, FC15 and L07 is unchanged by this work.
- **R9.** Repository CI linters that enumerate workflows or scripts
  (`.github/scripts/checks/workflow-policy_test.sh`,
  `.github/scripts/checks/ci-patterns-lint.sh`) pass after the change.
- **R10.** No pull request is blocked by the change: neither removed workflow
  is a required status check on `main` (confirmed: the only ruleset on `main`
  carries deletion, non-fast-forward and Copilot-review rules, and branch
  protection is not set).
- **R11.** Public-repository content rules hold for every commit, pull request
  body and doc the work produces: no reference to a private repository or
  private document, no `wip/` paths, no AI attribution, no emojis, and tsuku's
  writing-style word list respected.
- **R12.** If both checks are retired, the change is a single tsuku pull
  request with no shirabe change.

## Acceptance Criteria

- [ ] `git grep -n -e strikethrough-all-docs -e check-issue-status -- .github/`
      returns no match on the branch after the change.
- [ ] `.github/workflows/validate-closing-issues.yml` and
      `.github/workflows/validate-diagram-status.yml` either no longer exist or
      no longer invoke either script.
- [ ] The only workflows under `.github/workflows/` that run a bash script
      reading `docs/` content beside `validate-docs.yml` are those running
      `.github/scripts/checks/mermaid.sh`.
- [ ] For each of the two checks, the design records exactly one disposition,
      absorbed or retired, with its reason.
- [ ] For each absorbed check, a shirabe pull request adds a capture under
      `crates/shirabe/tests/fixtures/absorption-golden/` that records the bash
      script's verdict and the engine's verdict on the same input documents.
- [ ] For each retired check, the removing pull request's body names the
      check, what it enforced, the windowed failure counts and the date of its
      last failure, and the reason nothing replaces it.
- [ ] Each file in R5 is either removed, or kept with a stated live caller;
      `git grep` for its basename returns no reference from a remaining file.
- [ ] `git diff origin/main -- .github/scripts/checks/mermaid.sh
      .github/workflows/validate-diagram-classes.yml
      .github/scripts/extract-design-issues.sh
      .github/scripts/extract-closing-issues.sh
      .github/scripts/checks/common.sh` is empty.
- [ ] `.claude/shirabe-extensions/work-on.md` no longer lists closing issues
      or diagram status among checks that run.
- [ ] `.github/scripts/checks/workflow-policy_test.sh` and
      `.github/scripts/checks/ci-patterns-lint.sh` pass on the branch.
- [ ] `shirabe validate --check FC15` on a PRD with sections out of order
      still reports FC15.
- [ ] The pull request's CI is green, including `validate-docs.yml` on the
      new documents.
- [ ] `git grep -n 'wip/'` finds no reference in a document or file the
      work added or changed.

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
  1000-run windows can't be classified.

## Decisions and Trade-offs

- **Absorb or retire is left to the design.** The requirements hold either
  way, and the choice turns on how the engine models DESIGN docs, which is a
  design question. Alternative: decide here; rejected because it would settle
  architecture in a requirements document.
- **Failure counts are stated as windowed counts.** The 18 and 21 figures are
  the failures in the most recent 1000 runs of each workflow, the API cap;
  retained history shows about 322 and 122. Alternative: quote 18 and 21 as
  lifetime totals; rejected as inaccurate.
- **Helpers go only after a search.** R5 removes `issue-status.sh` and its
  helpers rather than leaving dead code, but only once a repository-wide search
  shows no caller. Alternative: leave them; rejected because an unrun check
  beside a retired one is the same two-homes problem.
