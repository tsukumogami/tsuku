---
schema: brief/v1
status: Draft
problem: |
  Two bash document checks still run on every tsuku pull request beside the
  shirabe engine's validate-docs workflow. Both guard an issues-table and diagram
  shape in old DESIGN docs that new planning no longer uses, and neither has
  caught a real problem in six months, so CI's document rules have two homes.
outcome: |
  A contributor's pull request meets document rules from one place, the engine,
  plus the one diagram check that's knowingly deferred. A maintainer can say for
  each of the two old checks whether the engine now carries it or why nothing
  does, without reading bash to find out.
motivating_context: |
  tsuku moved its per-file document checks onto shirabe's reusable
  validate-docs.yml. Two PR-context bash checks were left behind: the corpus-wide
  strikethrough check and the closing-issue diagram-status check. Each has about
  1000 recent runs with its last failure on 2026-04-28, and the planning work
  that lands today goes into docs/plans/PLAN-*.md, which neither script reads.
---

# BRIEF: End the duplicate bash document checks in tsuku CI

## Status

Draft

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

The cost isn't the runtime. It's that a contributor reading CI can't tell
which document rules the engine owns and which live in scripts nobody
maintains alongside it, and a maintainer who wants to change a rule has two
places to look. A check that duplicates or outlives the engine also invites
the next one: the rule set drifts because nothing says which home is the real
one.

## User Outcome

A contributor who opens a pull request sees document checks that come from
the engine, plus the mermaid diagram check that's deliberately kept for now.
Nothing else asks them to edit a design doc their change didn't touch.

A maintainer who asks "where is this rule defined?" gets one answer. For each
of the two old checks there's a record: either the engine carries the rule and
a captured run shows it reaching the same verdict the script did, or the check
was retired and the pull request that removed it says what it used to catch
and why nothing replaces it.

## User Journeys

### A contributor closes an issue

A tsuku contributor opens a pull request whose body says `Fixes #N`. The
checks that run are the engine's validation of the docs they changed and the
mermaid diagram check on changed design docs. No job walks every design doc in
the repository looking for `#N`, and nothing fails because an unrelated old
design doc still lists the issue unstruck.

### A maintainer looks for a rule's home

A maintainer wants to know where the "closed issue must be struck through"
rule lives. They find it either in the engine, with a parity capture beside
it, or nowhere, with the retiring pull request explaining the decision. They
don't have to diff a bash script against engine checks to find out whether
the two agree.

### A future reader asks why a check vanished

Months later someone notices `validate-closing-issues.yml` is gone and wonders
if coverage was lost. The pull request that removed it states what the check
caught, how often, and why the engine's PLAN and ROADMAP checks or the kept
mermaid check cover what still matters.

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
