---
schema: brief/v1
status: Accepted
problem: |
  A contributor or maintainer can't tell from tsuku CI which document rules the
  shirabe engine owns. Two bash checks still run beside it, guarding an old
  DESIGN-doc shape new planning no longer uses; one repeats a rule the mermaid
  check already enforces, and neither has caught a real problem in six months.
outcome: |
  A contributor's pull request meets document rules from one place, the engine,
  plus the one diagram check that's knowingly deferred. A maintainer can answer
  "is this rule still enforced, and where?" for each of the two old checks in
  one lookup, without reading bash.
motivating_context: |
  tsuku moved its per-file document checks onto shirabe's reusable
  validate-docs.yml. Two PR-context bash checks were left behind: the corpus-wide
  strikethrough check and the closing-issue diagram-status check. Each has about
  1000 recent runs with its last failure on 2026-04-28, and the planning work
  that lands today goes into docs/plans/PLAN-*.md, which neither script reads.
---

# BRIEF: End the duplicate bash document checks in tsuku CI

## Status

Accepted

## Problem Statement

tsuku validates its documents with shirabe's engine through
`.github/workflows/validate-docs.yml`. That's meant to be the single place a
document rule lives. Two older bash checks still run beside it on every pull
request:

- `.github/scripts/checks/strikethrough-all-docs.sh`, run by
  `validate-closing-issues.yml`. For each issue a pull request closes, every
  non-archive `DESIGN-*.md` that lists the issue in its Implementation Issues
  table must show it struck through.
- `.github/scripts/check-issue-status.sh`, run by
  `validate-diagram-status.yml`. For each issue a pull request closes, every
  DESIGN doc with a mermaid diagram that mentions `[#N]` must give node `I<N>`
  the `done` class.

Both police a DESIGN-doc shape the project has moved away from. The engine's
model gives a DESIGN no issues table; issue tracking lives in PLAN and ROADMAP
documents, where the engine already reconciles the table, the diagram and
issue state. New planning work in tsuku lands in `docs/plans/`, which neither
script reads. Each workflow has run roughly a thousand times with no failure
since April.

The cost isn't the runtime. Two examples show it. First, the diagram-status
job repeats a rule the kept mermaid check already enforces: `mermaid.sh`'s
MM15 fails a changed design doc whose diagram doesn't mark a closing issue
`done`, so that one rule now has two definitions in two workflows. Second,
`docs/designs/current/DESIGN-version-verification.md` still lists #204, #205,
#207 and #208 unstruck in its issues table. A contributor whose pull request
finally closes one of them gets a red strikethrough job that asks them to edit
a 2025 design doc their change never touched, and nothing in the failure says
whether the engine knows about the rule. A maintainer who wants to change a
document rule has two places to look and no record of which one is real.

## User Outcome

A contributor who opens a pull request sees document checks that come from
the engine, plus the mermaid diagram check that's deliberately kept for now.
Nothing else asks them to edit a design doc their change didn't touch.

A maintainer who asks "is this rule still enforced, and where?" answers it in
one lookup instead of by reading bash. For each of the two old checks the
answer is already written down: the engine holds the rule, or the check was
retired on purpose and the reason sits where the removal happened.

## User Journeys

### A contributor closes an issue

A tsuku contributor opens a pull request whose body says `Fixes #N`. The
checks that run are the engine's validation of the docs they changed and the
mermaid diagram check on changed design docs. No job walks every design doc in
the repository looking for `#N`, and nothing fails because an unrelated old
design doc still lists the issue unstruck.

### A maintainer changes a document rule

A maintainer wants to tighten how closed issues are marked in planning
documents. They change it in one place, the shirabe engine, and know that no
tsuku script will keep enforcing the old form beside it. They don't have to
diff a bash script against engine checks to find out whether the two agree.

### A reviewer audits coverage after the removal

A reviewer looking at the checks list on a tsuku pull request sees that "Validate
Closing Issues in Design Docs" and "Validate Diagram Status for Closing
Issues" no longer report and asks whether a gap opened. They follow the
removal back and find what each check caught in its lifetime, and either the
engine rule that replaced it or the reason nothing needs to.

## Scope Boundary

**In scope:**

- The two scripts above and the two workflows that run them.
- A decision for each: absorb into the shirabe engine with a parity capture,
  or retire with the reason stated.
- Helper scripts and reference pages used only by the checks being removed,
  including the unrun `.github/scripts/checks/issue-status.sh` if it turns out
  to be unused.
- Pointers in tsuku's own docs or skill extensions that name the removed
  checks.

**Out of scope:**

- The mermaid diagram check (`.github/scripts/checks/mermaid.sh`, run by
  `validate-diagram-classes.yml`) and the helpers it uses
  (`extract-design-issues.sh`, `extract-closing-issues.sh`, `common.sh`). Its
  absorption is deferred for its own reasons and it stays as is.
- Rewriting old DESIGN docs to strike through stale issue rows or move their
  issue tables into PLAN documents. The corpus keeps its historical shape.
- Any new cross-document rule in the engine beyond what parity with an
  absorbed script would need.
- The PR-body check (`validate-pr-body.yml`), which is a different concern.
