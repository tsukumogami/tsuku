---
schema: brief/v1
status: Accepted
problem: |
  Ten of tsuku's twenty-one scheduled workflows are failing, five of them since
  February, and nothing tells anyone. Five fail inside the step that exists to
  report the failure, so the escalation path is itself a source of red. A check
  whose failure reaches nobody cannot be told apart from a check that passes.
outcome: |
  Every scheduled workflow sits in an honest state — passing because what it
  checks works, or retired on the record — and a failure afterwards reaches a
  maintainer. A workflow that references a label nobody created is caught before
  it merges rather than on the day its first real failure needs reporting.
motivating_context: |
  A survey of all twenty-one scheduled workflows on 2026-09-20 found ten
  substantially red. Three have open write-ups (#2593, #2448, and #667 against a
  failure mode Build Essentials no longer exhibits); the other seven have nothing.
  Nine label names referenced across fifteen call sites do not exist, and five of those
  have never fired, so they are armed rather than fixed. The defect
  class is already catalogued against non-scheduled checks too — #2590 and #2578
  are both cases of a check that reports success, or reports nothing, when it has
  not run.
---

# BRIEF: Honest states and a working failure path for scheduled workflows

## Status

Accepted

Framing for the scheduled-workflow failures. Three of the ten have open write-ups:
#2593 (R2 Health Monitor) and #2448 (Nightly Registry Validation), plus #667 against a
Build Essentials failure mode the workflow no longer exhibits. #2590 is the same defect
class in a workflow that is `pull_request`-only rather than scheduled, so it is a
precedent here and not an item of work.

The downstream PRD owns which workflows are repaired and which are retired, what "a
failure reaches a maintainer" has to guarantee, and which escalation shape meets that
guarantee. The DESIGN owns how the chosen shape is built, how a label reference stops
being a latent failure, and whether the macOS bottle-decomposition failures are one
defect or several. The open questions this brief carried in Draft were handed to those
two documents on acceptance; each is recorded there rather than here.

Edited after acceptance to correct the label counts to nine names across fifteen call
sites, and to correct the count of workflows carrying both faults from three to four.
The earlier figure of eight across thirteen missed the `actions/github-script` `labels:`
string form in Weekly Coverage Report and miscounted the rest.

## Problem Statement

tsuku runs twenty-one scheduled workflows. On 2026-09-20, ten of them were failing, and
the failures reached nobody.

Six have not passed once in their last twelve scheduled runs: Scheduled Tests (last
success 2026-01-16), Weekly Coverage Report (2026-02-08), Seed Queue and Recipe
Validation (both 2026-02-15), Discovery Registry Freshness (2026-02-23), and Curated
Recipe Nightly, which has no scheduled success on record at all and whose most recent
run was cancelled rather than failed. Four more fail persistently but not universally:
R2 Health Monitor (ten of its last twelve), Nightly Registry Validation and Build
Essentials (nine each), and R2 Cleanup (two, including the most recent). R2 Cleanup is
counted despite its low rate because of when it fails rather than how often: as the next
paragraphs show, it goes red exactly on the runs where it has something to report. Two
further workflows — npm Builder Tests and pypi Builder Tests — have exactly one bad run
in twelve each, but theirs are download timeouts unrelated to what they check, so they
are flakes and counting them would overstate the problem. The visibility gap on the ten
is measured in months.

The reason nobody noticed is the part that matters. Five of the ten fail *inside the
step meant to report the failure* — and these are not a separate group from the rest,
since several of the five also have real failures in the thing they check. That overlap
is the point rather than an untidiness: a workflow can be broken and unable to say so,
and fixing what it checks would leave it just as silent. R2 Health Monitor's
health-check job completes and records its verdict — `degraded`, which is exactly what
is supposed to raise the alarm — and its `Issue Management` job dies on `GraphQL:
Resource not accessible by integration (createIssue)`. R2 Cleanup, Curated Recipe
Nightly, Nightly Registry Validation and Discovery Registry Freshness each die on `could
not add label: '<name>' not found`. The monitor cannot report that the monitor is
broken, so the only surviving signal is a red mark on a page nobody scrolls.

R2 Cleanup shows the shape at its worst: it only attempts to file an issue when it has
found cleanup candidates, so it is green precisely when it has nothing to say and red
precisely when it has something to report. The signal is inverted.

Nightly Registry Validation carries the mirror image of that problem, recorded in
#2448: when it does run, it downloads every object, matches none of them against the
layout it expects, and concludes green having validated nothing. So one of the ten is
simultaneously unable to report a failure and able to report a success it did not earn.
Repairing its escalation path would make it reliably announce a result that means
nothing, which is a reminder that a working failure path is necessary here and not
sufficient.

The clearest cost sits with Discovery Registry Freshness. Across its last twelve weekly
runs — 2026-06-29 through 2026-09-14 — every one died on `could not add label:
'discovery-registry' not found`, and every one had something to say first. The count of
invalid registry entries it found rose steadily across those weeks: six, then seven,
then eight, then nine. The check worked perfectly. It detected real drift every week,
watched that drift grow by half, and failed to tell anyone twelve times in a row.
Everywhere else the cost is risk rather than damage anyone can point to: a regression
could have slipped through unseen for seven months and nobody can now demonstrate that
none did. That is the honest shape of the harm, and it bears on how much mechanism the
repair deserves.

Underneath the escalation failures sit two independent defects that stack. Nine label
names, referenced across fifteen call sites in the workflow corpus, do not exist in the
repository, and five workflows that file issues — R2 Health Monitor, R2 Cleanup, Nightly
Registry Validation, Discovery Registry Freshness and R2 Cost Monitoring — declare no
`permissions:` block at any level, so the default token carries no `issues: write`.
Because `gh issue create --label X` resolves the label before it calls `createIssue`, a
workflow missing both shows only the label error; the permission error is hidden behind
it. Four workflows carry both faults at once, and three of those four are red today.
Repairing the labels alone would therefore look like a complete fix and leave those
three exactly as red, which is why the two defects have to be established as independent
rather than patched together.

Five of the nine have never fired, because the workflow dies earlier or the escalation
condition has not yet been met. Those are not working code. They are failures armed for
the day they first matter, which is the day someone needs the report. The clearest case
is Checksum Drift: it runs daily, has passed every recent run, correctly declares
`issues: write`, and files its issue with a `security` label that does not exist. A
daily check for tampering with recipe downloads is green every day and will fail to
report the first drift it ever finds. One of the five, used by R2 Cost Monitoring, sits
in the one place someone saw the problem coming: that workflow runs `gh label create`
before using the label. The attempt is defeated twice over — creating a label needs the
same `issues: write` the file never declares, and the call ends in `2>/dev/null ||
true`, so its failure is discarded rather than reported. Nothing else in the repository
connects a `--label` argument to the set of labels that exist, and nothing checks before
a workflow merges, so the gap stays invisible until a real failure walks into it.

The failures in the monitored thing — some of them sitting underneath the reporting
failures above rather than alongside them — are a mixed set: a Go toolchain pinned to
1.22 in the one workflow that never moved to `go-version-file` while `go.mod` requires
1.25.8; a seeding step that writes an audit entry per package and cannot create the
directory for a scoped npm name; a test matrix leg whose declared recipe the workflow
never reads; a second leg of that same workflow whose cask app-bundle verification fails
for a reason the log does not give, so that repairing the first leg alone would leave
Scheduled Tests red; and a family of macOS Homebrew bottle-decomposition failures.

This is one problem rather than ten. It is not that tsuku has ten broken workflows; it
is that tsuku has no working path from a scheduled failure to a person, and ten
workflows are the evidence.

## User Outcome

A maintainer can trust the Actions tab again.

When a scheduled workflow fails, they find out — through a path that has been chosen
deliberately and is known to work, rather than one inherited from whichever workflow was
copied last. A failure that needs a human reaches a human. A workflow that is not worth
that attention has been retired, and the reasoning is written where the next person
looking for it will find it.

Every scheduled workflow is in a state a maintainer can defend. Green means the thing it
checks works, and also that it could report if it stopped working — a workflow that has
never yet had to escalate is known to be able to, rather than merely untested. Red means
something is actually wrong and somebody has been told. There is no third category of
workflow that is red because its own plumbing is broken, and none that is green because
it never got far enough to report what it found.

A contributor adding a scheduled workflow that files an issue finds out that the label
they referenced does not exist before it can cost anyone anything, rather than months
later when the first real failure fails to escalate.

## User Journeys

### Pushed a signal: a maintainer who was not looking finds out anyway

A tsuku maintainer is not watching the Actions tab and has no reason to. Overnight, the
nightly recipe validation finds a curated recipe whose download URL has broken. The
maintainer learns of it without going looking — the recipe is named, the run is linked,
and what failed is legible without reconstructing it from a log. They act on the recipe.
They never have to discover that the notification path was itself broken.

### Pulling a status read: a returning maintainer asks which checks actually matter

A maintainer comes back from a month away, or is new to the repository and doing a first
pass over CI, and needs to know what the scheduled workflows are telling them. Each one
is either green, or red for a stated reason they can read without opening the run log,
or absent because it was retired deliberately. Where a workflow was retired, they can
find the reasoning without asking anyone — it was argued for explicitly rather than
disappearing inside a commit called "fix CI". Nothing is red for a reason that turns out
to be its own plumbing, so nothing has to be triaged twice.

### Caught at authoring time: a contributor's label gap surfaces before merge

A contributor writes a new scheduled workflow that files an issue on failure, labelling
it with a category name that reads naturally but does not exist in the repository.
Before the change merges, they are told the label does not exist and either create it or
use one that does. They do not merge a workflow whose escalation path is armed to fail,
and no future maintainer inherits a silent gap that only surfaces on the day it matters
most.

## Scope Boundary

### In

- The health state of every scheduled workflow in `.github/workflows/` — all
  twenty-one, not only the ten currently failing, since the latent label references
  sit in workflows that are green today.
- The mechanism by which a scheduled failure reaches a person, chosen on purpose and
  stated, including the deliberate choice that a given workflow does not warrant one.
- The repository labels the workflows reference, and the token permissions the
  escalating jobs run under.
- **Both stacked defects.** The missing labels and the missing token permissions are
  separate faults that happen to hide behind one error message. Both are in scope.
- Whatever keeps a label reference from becoming a latent failure again.
- The supporting scripts and matrix data behind the failing workflows, where the
  failure is in them: the test matrix, the seeding audit path, the Go toolchain pin.
- The macOS Homebrew bottle-decomposition failures affecting Build Essentials, Recipe
  Validation and Curated Recipe Nightly. Their repair is in scope, not merely their
  diagnosis: those three workflows cannot reach an honest state while it stands.
- Retiring a workflow, or removing its schedule, where that is the honest answer —
  argued explicitly, with the reasoning recorded.

### Out

- **Weakening a check to make it pass.** Changing what a workflow checks is in scope
  when the change is argued on the record and the argument survives review. It is out
  of scope when it makes a red go away without one. Deleting a failing matrix leg,
  dropping a `--label`, swallowing an exit code, adding `continue-on-error: true`, or
  widening a latency threshold until the measurement fits underneath it are the moves
  this rules out. The test matrix leg that fails because the workflow never reads its
  declared recipe is the case that tests the rule: deleting the leg would remove the
  only coverage of that code path, so the honest repair preserves the assertion rather
  than removing it.
- **Any release.** Repairing these workflows may make a release look due; cutting one
  is separate work and not part of this.
- **Other repositories.** A reader might expect the repair pattern to be rolled out
  across the organisation at the same time. No other repository here files issues from
  a workflow at all, so there is nothing yet to roll it out to; if that changes, the
  pattern is worth replicating only once it has been proven in one repository, and
  proving it is what this work is for.
- **Non-scheduled workflows**, except where a scheduled workflow shares a file or a
  referenced label with one. #2590's golden check is the nearest example: same defect
  class, `pull_request`-only, and not this work's to close.
- **The R2 latency threshold's own validity.** R2 Health Monitor's `degraded` verdict
  comes from a 2000 ms threshold measured around an entire `aws s3api` process
  invocation, interpreter startup included, which is roughly the measurement floor.
  Whether that threshold can distinguish signal from its own overhead is a real
  question, but it is a question about what the workflow checks, and it belongs to a
  separate conversation unless the answer is to retire the workflow.
- **A general-purpose alerting or on-call system.** The outcome needs failures to
  reach a maintainer, not a notification platform.


## References

- `docs/ci-patterns.md` — the runner types and matrix conventions this repository
  already uses. The repair stays inside them rather than introducing a parallel set.
- `docs/workflow-validation-guide.md` — the existing validation guidance for recipe
  workflows, and the natural home for whatever check keeps a label reference from
  going stale.
