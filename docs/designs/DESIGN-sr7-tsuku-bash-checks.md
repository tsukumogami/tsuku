---
schema: design/v1
status: Accepted
problem: |
  Two bash checks run on every tsuku pull request beside shirabe's engine:
  strikethrough-all-docs.sh (every DESIGN issues table must strike a closing
  issue) and check-issue-status.sh (every DESIGN diagram must mark a closing
  issue's node done). The engine models issue tables only on PLAN and ROADMAP,
  MM15 in the kept mermaid check already covers the diagram rule on changed
  docs, and neither check has failed since 2026-04-28, so each rule has two
  homes and one can fail a pull request over an untouched 2025 design doc.
decision: |
  Retire both checks in one tsuku pull request: delete their two workflows and
  two scripts, plus the unrun issue-status.sh, its helper get-github-status.sh
  and the IS01/IS02 reference pages, and fix the one pointer line in
  .claude/shirabe-extensions/work-on.md. The mermaid check and its helpers stay
  untouched. A disposition table here and the pull request body record what
  each check caught and why nothing replaces it. No shirabe change.
rationale: |
  Absorbing either check with parity would give the engine a DESIGN
  issues-table model and a whole-tree, PR-context traversal, which is the
  cross-document rule shirabe deliberately deferred, to guard a shape new
  planning no longer uses. The coverage lost is a nudge for four open 2025
  issues in one doc; PLAN documents keep the rule through FC08 and FC09, and
  changed design docs keep MM15.
upstream: docs/prds/PRD-sr7-tsuku-bash-checks.md
user_visible_surface: false
decision_provenance: inline-resolved
---

# DESIGN: Retire the duplicate bash document checks in tsuku CI

## Status

Accepted

## Context and Problem Statement

tsuku validates its documents with shirabe's engine. `.github/workflows/validate-docs.yml`
calls shirabe's reusable `validate-docs.yml` at `@main`, which builds the
`shirabe` binary and runs `shirabe validate` over the docs a pull request
changes. Beside it, two bash checks still run on every pull request, and both
read the pull request's closing issues through
`.github/scripts/extract-closing-issues.sh`:

- `.github/workflows/validate-closing-issues.yml` runs
  `.github/scripts/checks/strikethrough-all-docs.sh --pr <N>`. For each closing
  issue it walks every `docs/designs/**/DESIGN-*.md` outside `archive/`, parses
  the Implementation Issues table with `.github/scripts/extract-design-issues.sh`,
  and fails if any row for that issue isn't struck through. It reruns when the
  pull request body is edited.
- `.github/workflows/validate-diagram-status.yml` greps every DESIGN doc for
  `[#N]`, and for each one with a mermaid block runs
  `.github/scripts/check-issue-status.sh <doc> <N> done`, which fails when node
  `I<N>` lacks the `done` class and exits 2 when the node isn't found (also a
  failure).

The technical problem is that these rules sit outside the engine and model a
document shape the engine doesn't. In shirabe's format model
(`crates/shirabe-validate/src/formats.rs`) only PLAN (`Issue | Dependencies |
Complexity`) and ROADMAP carry an issues table; a DESIGN has none. Issue state
for PLAN and ROADMAP documents is reconciled by FC07 (table against diagram),
FC08 (strikethrough against diagram classes) and FC09, whose Sub-check C
already reports a pull request that says `Closes #N` while the document still
shows N as ready or blocked. shirabe's own check-absorption design records the
per-document strikethrough rule as absorbed by FC08 and a corpus-wide
strikethrough rule as deferred, under-specified. Separately, the mermaid check
tsuku keeps (`.github/scripts/checks/mermaid.sh`, run by
`validate-diagram-classes.yml` on changed design docs with `--pr`) enforces
MM15: a closing issue's node must be `done`. That is the diagram-status rule,
on changed docs.

What the two checks caught is real but small. In the most recent 1000 runs of
each workflow, measured 2026-10-30, the strikethrough check failed 18 times
across 12 pull requests (window from 2026-04-13) and the diagram check 21 times
across 5 pull requests (window from 2026-03-23); retained history goes back to
late January 2026 with about 322 and 122 failed runs. Every windowed failure
was a real slip fixed by a follow-up doc edit, mostly a curated-recipes batch
that forgot to strike its row in `docs/designs/DESIGN-curated-recipes.md`. The
last failure of either was 2026-04-28. Today the strikethrough check can fire
on only four open issues, #204, #205, #207 and #208, all unstruck in
`docs/designs/current/DESIGN-version-verification.md`; the diagram check has
one document with `class I<N>` nodes, `DESIGN-curated-recipes.md`, whose one
open item (#2349) neither script can see, because its row reads
`[#2349: title](url)` rather than `[#2349]`.

### Requirements this design answers

The requirements come from `docs/prds/PRD-sr7-tsuku-bash-checks.md`:

- No workflow runs `strikethrough-all-docs.sh` or `check-issue-status.sh`,
  directly or through another script (R1).
- Each check gets exactly one disposition, absorbed or retired (R2), recorded
  in a disposition table in this document with the columns Check,
  Disposition, Reason and Where (R3).
- An absorbed check needs a shirabe parity capture in the style of
  `crates/shirabe/tests/fixtures/absorption-golden/`, agreeing with the bash
  script on pass or fail and on the flagged (issue, document) pairs over a set
  of struck, unstruck, archive and multi-document inputs (R4), and it keeps the
  bash check's whole-tree reach, with this design saying how the engine
  receives closing issues (R9).
- A retired check is explained in the removing pull request's body: what it
  enforced, its failure figures labelled as windowed counts, and why nothing
  replaces it (R5).
- Files that exist only for a removed check go with it, after a whole-tree
  search confirms no remaining caller (R6). The candidates are
  `.github/scripts/checks/issue-status.sh`, `.github/scripts/get-github-status.sh`,
  and `.github/scripts/docs/IS01.md` and `IS02.md`.
- No tsuku file outside this topic's own documents describes a removed check
  as running; today that is one line in `.claude/shirabe-extensions/work-on.md`
  (R7).
- `mermaid.sh`, `validate-diagram-classes.yml`, `extract-design-issues.sh`,
  `extract-closing-issues.sh` and `checks/common.sh` stay byte-for-byte
  unchanged (R8).
- No engine regression for FC05, FC09, FC15 or L07 (R10); the CI linters
  `workflow-policy_test.sh` and `ci-patterns-lint.sh` pass (R11); neither
  workflow is a required status check, re-checked before merge (R12); public
  content rules hold (R13); and if both checks retire, the work is one tsuku
  pull request with no shirabe change (R14).

## Decision Drivers

- **One home per rule.** A document rule should live in the engine or not at
  all; the kept mermaid check is the one stated exception.
- **Don't invent engine behaviour.** shirabe's absorption design deferred a
  corpus-wide strikethrough rule until its semantics are stated. Absorbing
  here would mean writing that rule, plus a DESIGN issues-table model, to
  guard a shape new planning doesn't use.
- **Coverage actually lost.** What a removal gives up should be measured, not
  assumed: four open issues in one 2025 doc, one diagram doc whose open item
  neither script parses, and changed design docs that MM15 still covers.
- **Contributor friction.** A whole-tree check can fail a pull request over a
  document the change never touched.
- **Smallest change that meets the PRD.** tsuku-only if possible, one pull
  request, no edits to the kept mermaid check or its helpers.
- **CI hygiene.** The removal must leave `workflow-policy_test.sh` and
  `ci-patterns-lint.sh` passing and break no required status check.

## Considered Options

### Decision 1: Absorb or retire each check

Resolved inline.

Both checks enforce one idea: when a pull request closes an issue, every
design doc that tracks the issue must show it finished, as a struck row or a
`done` node. The question is whether that idea belongs in the engine or is
retired, and whether the two checks deserve different answers. The engine
receives a pull request's closing issues today only for FC09 Sub-check C, and
only for the document being validated; it validates changed files, one at a
time. Absorbing either check with parity means giving the engine an issues
table on DESIGN docs and a traversal over unchanged documents with the pull
request's closing issues as input.

Key assumptions:

- New planning work in tsuku lands in `docs/plans/PLAN-*.md`, where FC07, FC08
  and FC09 already reconcile issue state.
- MM15 stays in `mermaid.sh` (deferred, unchanged) and keeps reading closing
  issues with `--pr` on changed design docs.
- Neither workflow is a required status check on `main`.

#### Chosen: Retire both checks

Delete `validate-closing-issues.yml` and `validate-diagram-status.yml` and the
two scripts they run. Nothing in the engine replaces them, and the reason is
stated per check in the disposition table below and in the removing pull
request. For the strikethrough check, the rule it enforced on DESIGN docs
belongs to a format the engine deliberately moved into PLAN and ROADMAP, where
FC08 and FC09 hold it per document; its live reach is four open issues in one
2025 doc. For the diagram check, MM15 already enforces the same rule on any
changed design doc with a diagram, and the only doc with `I<N>` classes has
no open item the script can see.

The trade accepted: a pull request that closes #204, #205, #207 or #208
without touching `DESIGN-version-verification.md` will no longer be told to
strike the row. That's a bookkeeping nudge on a 2025 doc, which the
contributor-friction driver argues against anyway.

#### Alternatives Considered

**Absorb both into the engine.** Add an Implementation Issues table to the
Design format, a whole-tree traversal (like `--lifecycle`) that takes the pull
request's closing issues, and a parity capture for each script. Rejected
because it writes exactly the cross-document rule shirabe's absorption design
deferred as under-specified, adds a DESIGN issues-table model that new work
doesn't use, and keeps the whole-tree failure on untouched documents. It would
also add a shirabe pull request and a merge-ordering dependency for coverage
of four open issues.

**Absorb the strikethrough check, retire the diagram check.** Retire the
diagram check because MM15 covers it, and absorb the strikethrough rule alone.
Rejected because the strikethrough half carries all of the cost of the first
alternative (the DESIGN table model, the traversal, PR-context input) and the
smaller half of the value; it is the rule shirabe deferred.

**Keep both checks as they are.** Leave the workflows running. Rejected
because it leaves one rule with two homes, contradicts the one-home driver,
and keeps a check that can fail a pull request over a document it didn't
touch. The checks have not failed in six months, so keeping them buys little.

### Decision 2: What goes with the retired checks

Resolved inline.

Retiring the two run scripts leaves related files behind. A whole-tree search
on 2026-10-30 found that `.github/scripts/checks/issue-status.sh` (a
per-document strikethrough-versus-GitHub check with codes IS01 and IS02) is run
by no workflow, `.github/scripts/get-github-status.sh` is called only by it,
and `.github/scripts/docs/IS01.md` and `IS02.md` are referenced only by it. No
test, testdata, Makefile or lint references any of them.

Key assumptions:

- `mermaid.sh`, `extract-design-issues.sh`, `extract-closing-issues.sh` and
  `checks/common.sh` keep live callers in `validate-diagram-classes.yml` and
  stay.

#### Chosen: Remove the unrun strikethrough checker and its helpers too

Delete `issue-status.sh`, `get-github-status.sh`, `IS01.md` and `IS02.md` in
the same pull request, after re-running the search on the branch. Update the
one pointer: `.claude/shirabe-extensions/work-on.md` lists "closing issues,
diagram status" among the pull-request-context checks its verification map
leaves out.

#### Alternatives Considered

**Remove only the two run scripts and their workflows.** Smallest diff.
Rejected because it leaves an unrun strikethrough checker beside the retired
one, which is the same two-homes problem with no caller to justify it.

**Keep `issue-status.sh` as a manual tool.** Rejected because nothing and
nobody runs it, and for PLAN documents FC08 and FC09 already do its job.

## Decision Outcome

**Chosen: retire both checks (1) and remove their dead companions (2).**

### Summary

One tsuku pull request deletes eight files: the workflows
`validate-closing-issues.yml` and `validate-diagram-status.yml`, the scripts
`scripts/checks/strikethrough-all-docs.sh` and `scripts/check-issue-status.sh`,
the unrun `scripts/checks/issue-status.sh`, its helper
`scripts/get-github-status.sh`, and the reference pages
`scripts/docs/IS01.md` and `scripts/docs/IS02.md`, all under `.github/`. It edits one
line in `.claude/shirabe-extensions/work-on.md`. Nothing in shirabe changes.
The kept mermaid check, its workflow and its helpers are not touched, so
MM15's closing-issue rule keeps running on changed design docs.

After the change, the document checks a pull request meets are the engine's
(through `validate-docs.yml`) and `validate-diagram-classes.yml`. The
disposition table below is the record a maintainer reads; the pull request
body repeats what each check caught and why nothing replaces it, so a reviewer
who notices the two jobs gone can follow the removal back.

### Disposition table

| Check | Disposition | Reason | Where |
|---|---|---|---|
| `strikethrough-all-docs.sh` (`validate-closing-issues.yml`) | retired | Guards an Implementation Issues table on DESIGN docs, a shape the engine models only on PLAN and ROADMAP, where FC08 and FC09 hold the rule per document. A corpus-wide version is deferred in shirabe as under-specified. Live reach today is four open issues in one 2025 doc. | removing pull request |
| `check-issue-status.sh` (`validate-diagram-status.yml`) | retired | Repeats MM15, which the kept mermaid check enforces on changed design docs. The one doc with `I<N>` classes has no open item the script can parse. | removing pull request |

### Rationale

The two decisions reinforce each other. Retiring both means the engine
doesn't grow a model for a deprecated shape, and removing the unrun checker
too means no copy of the rule survives anywhere outside the engine and MM15.
The trade accepted is a lost nudge for four open issues in one old doc, in
exchange for one home per rule and no whole-tree failures on untouched files.

## Solution Architecture

### Overview

A pure removal in tsuku. No new component, no engine change. The remaining
document checks on a pull request are shirabe's reusable `validate-docs.yml`
(called from `.github/workflows/validate-docs.yml`) and
`.github/workflows/validate-diagram-classes.yml` running `mermaid.sh -q --pr`.

### Components

Removed, all under `.github/`:

| File | Why it exists today | Remaining callers after removal |
|---|---|---|
| `workflows/validate-closing-issues.yml` | runs the strikethrough check | none |
| `workflows/validate-diagram-status.yml` | runs the diagram-status check | none |
| `scripts/checks/strikethrough-all-docs.sh` | the strikethrough check | none |
| `scripts/check-issue-status.sh` | the diagram-status check | none |
| `scripts/checks/issue-status.sh` | unrun per-doc strikethrough check | none |
| `scripts/get-github-status.sh` | helper for `issue-status.sh` | none |
| `scripts/docs/IS01.md`, `scripts/docs/IS02.md` | reference pages for `issue-status.sh` | none |

Edited: `.claude/shirabe-extensions/work-on.md`, the bullet "The PR-context
checks (PR body, closing issues, diagram status)" becomes "The PR-context
checks (PR body, and the closing-issue rule of the diagram-class check)",
keeping the reason that follows.

Unchanged, verified by an empty diff against the merge base:
`scripts/checks/mermaid.sh`, `workflows/validate-diagram-classes.yml`,
`scripts/extract-design-issues.sh`, `scripts/extract-closing-issues.sh`,
`scripts/checks/common.sh`.

### Key Interfaces

The only interface touched is GitHub's check list on a pull request: the jobs
"Validate Closing Issues in Design Docs" and "Validate Diagram Status for
Closing Issues" stop reporting. Neither is a required status check (the only
ruleset on `main` carries deletion, non-fast-forward and Copilot-review rules,
and branch protection is unset), so no merge gate changes. This is re-checked
with `gh api repos/tsukumogami/tsuku/rules/branches/main` before merge.

### Data Flow

Before: pull request opened or synchronized, `extract-closing-issues.sh`
reads its body and commits, each workflow walks `docs/designs/` and fails on a
stale row or node. After: the engine validates the changed docs, and the
mermaid workflow (triggered only when a design doc changes) reads closing
issues for MM15. No workflow walks unchanged design docs for closing issues.

## Implementation Approach

### Phase 1: Remove and record (one pull request)

1. Re-run the whole-tree search, excluding this topic's own PRD, DESIGN and
   PLAN and `docs/designs/archive/`, for the patterns
   `strikethrough-all-docs`, `check-issue-status`, `checks/issue-status.sh`,
   `get-github-status`, `docs/IS01.md`, `docs/IS02.md`,
   `validate-closing-issues`, `validate-diagram-status`, "Validate Closing
   Issues" and "Validate Diagram Status", and confirm the only hits are among
   the files being removed and the `work-on.md` bullet.
   `.github/escalation-registry.yml` has no entry for either workflow today.
2. Delete the eight files with `git rm`.
3. Edit the `work-on.md` bullet.
4. Run `.github/scripts/checks/workflow-policy_test.sh` and
   `.github/scripts/checks/ci-patterns-lint.sh`; confirm the empty diff on the
   kept files against the merge base, and that `extract-design-issues.sh`,
   `extract-closing-issues.sh` and `checks/common.sh` still have callers in
   `mermaid.sh` or `validate-diagram-classes.yml`.
5. Before merge, query both `gh api repos/tsukumogami/tsuku/rules/branches/main`
   and `gh api repos/tsukumogami/tsuku/branches/main/protection` (a 404 means
   unprotected; it returned 404 on 2026-10-30) and confirm no required status
   check names either removed job.
6. Open the pull request. Its body states, for each check: what it enforced,
   its windowed failure figures (18 failed runs across 12 pull requests since
   2026-04-13, and 21 across 5 since 2026-03-23), about 322 and 122 failed
   runs in retained history since late January 2026, the last failure on
   2026-04-28, and the reason nothing replaces it, from the disposition table.

Deliverables:

- the eight deletions and the `work-on.md` edit
- a pull request body with both retirement records

## Security Considerations

This change only removes code. It adds no workflow, script, dependency or
permission, so it introduces no new attack surface and no new data flow.

The two removed workflows ran with a read-only token (`contents`, `issues`
and `pull-requests` read) and processed pull request text, which its author
controls. `extract-closing-issues.sh` reduces that text to a list of integers
before any later step uses it, so the removed workflows weren't exploitable
through the pull request body. `validate-diagram-status.yml` did place a step
output inside a single-quoted `run:` string, a pattern to avoid even when the
value is numeric; deleting the workflow removes it.

Removing the checks removes two status checks from each pull request. Before
merge, the branch rules for `main` are queried to confirm neither is a
required status check, so no merge gate changes silently. What's given up is
a bookkeeping nudge on a few legacy design docs, not a security control.

One related finding sits outside this change. The kept
`validate-diagram-classes.yml` interpolates the list of changed file names
(`${{ steps.changed.outputs.docs }}`) into a `run:` script, so a branch author
with write access could in principle craft a file name that runs commands in
that job. The job uses a read-only token under the `pull_request` trigger, so
fork pull requests can't exploit it and the risk is low. The same step writes
that list to `$GITHUB_OUTPUT` with a fixed `EOF` delimiter, so a file name
containing a newline or the delimiter could add extra step outputs. This
design keeps that file byte-for-byte unchanged, so the fix (passing the value
through `env:` and using a generated delimiter) belongs to separate follow-up
work, not this change; the removing pull request names it as a follow-up.

## Consequences

### Positive

- Each document rule a pull request meets has one home: the engine, or the
  deliberately kept mermaid check.
- No job fails a pull request over a design doc the change didn't touch.
- Two workflow runs per pull request go away, along with the `issues: read`
  and `pull-requests: read` token use they carried.
- The engine stays free of a DESIGN issues-table model for a deprecated shape.

### Negative

- A pull request that closes #204, #205, #207 or #208 without editing
  `DESIGN-version-verification.md` passes silently, leaving the row unstruck.
- A pull request that closes an issue tracked in an unchanged design doc's
  diagram no longer gets a nudge; MM15 runs only when the doc changes.
- The record of what the checks caught lives in this design and one pull
  request body rather than in a running check.

### Mitigations

- New planning lands in PLAN documents, where FC08 and FC09 keep the
  per-document rule, so the gap covers only legacy DESIGN tables.
- The four affected issues are needs-design items from 2025; whoever closes
  one will likely be editing its design doc, which MM15 then covers if the doc
  has a diagram.
- The disposition table gives a maintainer a single place to read the
  decision, and to reverse it by restoring the workflows if the gap matters.
