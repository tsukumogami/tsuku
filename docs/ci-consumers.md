# Who consumes each scheduled workflow's output

Every workflow with an `on.schedule` trigger, and what receives its result. A workflow
whose output reaches no person, no issue, no committed artifact and no notification is
not providing coverage, whatever colour it reports.

**A status check is not a consumer.** A check on a page someone has to visit is exactly
what this record exists to distinguish from a signal that arrives.

**A green run is not a consumer either.** Four workflows here run green and deliver
nothing, and the first draft of this record miscategorised one of them for exactly that
reason. Every row is placed by what arrived, checked against run history and against the
commit log, not by what the workflow file declares.

**Observed 2026-09-20, 21:50 UTC.** Label existence is repository state and changes
without a commit, so the observation time matters as much as the count. The dated events
at the end of this document record the changes made to that state so far.

Counted at that time: **21** scheduled workflows, across both `.yml` and `.yaml`.

## Summary

| Category | Workflows |
|---|---|
| Consumer works today | 4 |
| Consumer declared but does not arrive | 9 |
| Scheduled run reaches nobody; the same check does reach a PR author | 6 |
| Scheduled run reaches nobody, and nothing else runs it | 2 |

4 + 9 + 6 + 2 = 21. Every workflow is counted once; none appears in two rows.

## One symptom, four causes

The nine workflows in the second row all present identically: a workflow that says it will
tell somebody, and nobody hears anything. That single symptom has four separate causes,
and they are not variations on a theme — each needs a different fix, and two of them are
untouched by the repair that fixes the other two.

| Cause | Workflows | What would fix it |
|---|---|---|
| The label it names does not exist | `r2-cleanup` (still); seven others repaired 2026-09-20 | create the label |
| No `permissions:` block, so the token cannot write issues | `discovery-freshness`, `nightly-registry-validation`, `r2-cleanup`, `r2-cost-monitoring`, `r2-health-monitor` | declare `issues: write` |
| The job dies before the reporting step runs | `seed-queue`, `weekly-coverage-report` | fix the failure upstream of the step |
| The reporting step has never been triggered | `checksum-drift`, `r2-cost-monitoring` | nothing yet — the defect is latent |

Two things follow that a cause-blind reading misses.

**Nothing about labels or permissions fixes `seed-queue` or `weekly-coverage-report.`**
Both already declare `issues: write`, and both die before reaching the step that would use
it. A pass that fixed every label and every permission in this repository would leave both
exactly as broken as they are now, while reporting that the escalation problem was solved.

**Some workflows carry two causes at once, and the first hides the second.** `gh issue
create --label X` resolves the label before it calls the API, so a missing label fails
first and a missing permission is never reached. `r2-cleanup` carries both. Repairing one
and observing continued failure is the only way to see the other, which is why the repair
is staged rather than done in one sweep.

## Consumer works today

| Workflow | Consumer | Kind | Evidence |
|---|---|---|---|
| `r2-credential-rotation-reminder` | An issue labelled `maintenance` | issue | 2 scheduled runs, both green, quarterly |
| `batch-operations` | Commits it pushes | committed artifact | commit step green daily; commits when state changes |
| `batch-generate` | Commits and pull requests it opens | committed artifact | 1421 state commits; pull requests up to #2534 |
| `deploy-recipes` | The deployed registry itself | deployment | 98 green scheduled deploys |

`r2-credential-rotation-reminder` is the only workflow in this repository that has been
observed filing an issue. It declares `issues: write` and uses `maintenance`, which
already existed.

Three of these four deliver less than the table suggests, and the qualification belongs
beside the row rather than in a footnote:

**`batch-generate` has produced nothing since 2026-08-17.** Its first step checks for open
pull requests from `batch/` branches and skips the whole run if any exist. Two are open,
#2426 since 2026-06-26 and #2534 since 2026-08-17, so every scheduled run since has
reported success with every subsequent step skipped. The guard is deliberate and the
consumer is not broken; the workflow is idling behind pull requests it opened itself and
cannot merge. Its output resumes when those merge or close.

**`deploy-recipes` is deprecated in its own header.** It deploys `registry.tsuku.dev`,
which `deploy-website.yml` supersedes by serving `recipes.json` from the same origin, and
the file says it is kept for backwards compatibility during the transition (#353). The
deployment succeeds and the consumer is real. Whether that consumer should still exist is
the retirement question this record feeds, not one it answers.

**`batch-operations` commits rarely by design.** Its `Persist control file` step runs green
on every scheduled run and exits early when `batch-control.json` is unchanged, so one
commit reaching main (2026-03-03) reflects a circuit breaker that rarely trips rather than
a path that does not work.

## Consumer declared but does not arrive

The nine, grouped by the cause table above.

### The escalation step runs and fails

| Workflow | Intended consumer | Why it does not arrive |
|---|---|---|
| `curated-nightly` | issue | label `curated-recipe-failure` was missing until 2026-09-20; `issues: write` is declared |
| `discovery-freshness` | issue | no `permissions:` block anywhere in the file |
| `nightly-registry-validation` | issue | no `permissions:` block anywhere in the file |
| `r2-cleanup` | issue | no `permissions:` block, **and** label `automation` still does not exist |
| `r2-health-monitor` | issue | no `permissions:` block; its label `r2-degradation` has always existed |

Each has a failing step whose name is the escalation itself: `Create curated recipe failure
issue`, `Create failure issue`, `Create R2 unavailable issue`, `Create summary`, `Handle
health status`. A workflow that cannot report its own failure is a defect separate from
whatever it was watching.

`r2-health-monitor` is the control case for the permission defect. Its label exists and
always has, so nothing about a label can explain why it fails.

### The job dies before the escalation step runs

| Workflow | Intended consumer | Where it stops |
|---|---|---|
| `seed-queue` | queue commits **and** review issues | fails at `Run seeding` |
| `weekly-coverage-report` | issue | fails at `Build tsuku` |

Neither reaches its consumer step at all. Both declare `issues: write`, and `seed-queue`
additionally declares `contents: write`.

`seed-queue` is the row this record got wrong the first time. It was listed as a working
consumer because its commit-and-push step exists. That step does not run. Every scheduled
run since 2026-02-15 has failed; 24 of them retain job data and all 24 fail at `Run
seeding` with `Create source change issues` and `Commit and push` skipped. Its
queue-writing half demonstrably worked once, with eight commits reaching main between
2026-01-31 and 2026-02-15, the date of its last green scheduled run, and has written
nothing since. Its current commit message has never appeared on main. The priority queue
is still maintained, by `update-queue-status.yml` and by the batch generator, which is why
the file looks alive while this workflow's contribution to it is dead.

### The escalation has never been triggered

| Workflow | Intended consumer | State |
|---|---|---|
| `checksum-drift` | issue | 60 green scheduled runs; label `security` was missing until 2026-09-20 |
| `r2-cost-monitoring` | issue | 33 green scheduled runs; no `permissions:` block |

Both are green because the condition they watch for has not occurred. Their defects are
armed rather than broken, and would first be discovered on the day the thing they exist to
catch finally happens, which is the worst available day to discover them.

`r2-cost-monitoring` also tries to create its own label at run time with `gh label create
... || true`. The suppression makes a failure there invisible, and the call needs the same
`issues: write` the workflow does not declare, so it cannot stand in for the missing
permission.

## Outstanding evidence

Three workflows declared `issues: write` on 2026-09-20 and were blocked only by a label that
now exists (`curated-nightly` has since dropped the permission along with its filing step).
Their only known blocker is gone and **none of them has been observed filing an issue.**
Each has a specific outstanding test, named here rather than left implied:

| Workflow | The test | When it can run |
|---|---|---|
| `curated-nightly` | superseded 2026-09-26: the step this test watched was removed, and failures now reach the escalator instead (see Dated events) | no longer applies |
| `checksum-drift` | its escalation fires and files | only when checksum drift actually occurs; cannot be forced without manufacturing drift |
| `weekly-coverage-report` | its escalation is reached at all | only after `Build tsuku` is repaired; the label was never its binding constraint |

Until those land, the count of workflows that *can* file an issue remains one:
`r2-credential-rotation-reminder`. Green runs do not raise that count, and a single pass
would not either.

## Dated events

Changes to repository state that this record depends on, so a reader can reconstruct the
picture at any point rather than only the latest one.

**2026-09-20 — eight labels created.** `coverage-regression`, `curated-recipe-failure`,
`discovery-registry`, `nightly-failure`, `r2-cost-alert`, `r2-unavailable`, `security` and
`seeding:review` were applied from the manifest in `.github/labels.yml` by
`.github/scripts/checks/label-manifest.sh --apply` (#2602). Before: 34 labels on the
repository, 8 of the manifest's 42 missing. After: 42, none missing, none differing. No
label was removed or modified; the script has no delete path.

Before that event, nine label names referenced across these workflows did not exist.
Afterwards only `automation` does not, and `r2-cleanup` is the single workflow still
referencing it. The event removed one of the two stacked causes. It did not make any
workflow file an issue, and the Outstanding evidence section above says what would show
that it had.

**2026-09-26 — `curated-nightly` stops filing its own issue.** Its `Create failure issue`
job opened a new dated issue on every failing night, each listing the whole curated set
rather than what failed (#2641). The shared escalator had by then filed for the same
failure (#2659, 14 seconds after the workflow's own #2658), so the job was removed along
with `issues: write`. A failed run now reaches its assignee through the escalator alone.
The `curated-recipe-failure` label stays in the manifest: issues already carry it, and
the manifest has no delete path.

**2026-09-26 — `curated-nightly` routes its failures to #2611.** The workflow now declares
`escalation-owned-by: 2611`. #2611 is the open decision on this workflow's scope, and every
failing night is evidence for that decision, not a new problem. So the escalator comments
there, and stops opening or commenting on an item of its own (#2659). The declaration expires
when #2611 closes: the policy check goes red, and the escalator falls back to filing its own
item for the workflow.

**2026-09-26 — `curated-nightly` retired (#2611).** The workflow and its registrations are
gone: the escalator registry entry, the listener's `workflow_run` entry, and the policy
check's pinned count, which drops from 22 to 21 scheduled workflows. Its owner declaration
went with the file. It never validated the curated set it discovered. It called the shared
validation core with no recipe list, so it ran the same full-registry job as Recipe
Validation, daily. In 162 runs it never completed, and the core never fails on a failing
recipe, so its verdict couldn't have tracked the recipes even if it had. The rows above
describe it as observed on 2026-09-20 and are left as history. Curated recipes are still
install-tested at pull-request time by `test-recipe.yml`. Nothing checks them for upstream
drift between changes.

## Scheduled run reaches nobody; the same check does reach a PR author

These also run on `pull_request`, where the check is consumed by whoever opened the pull
request. On the scheduled run there is no pull request and no consumer.

`build-essentials`, `cargo-builder-tests`, `gem-builder-tests`, `npm-builder-tests`,
`pypi-builder-tests`, `test`.

Their scheduled runs are the ones this record calls unconsumed. The pull-request runs are
genuinely consumed and are not in question.

## Scheduled run reaches nobody, and nothing else runs it

| Workflow | Triggers | Output |
|---|---|---|
| `recipe-validation` | `schedule`, `workflow_dispatch` | status check only |
| `scheduled-tests` | `schedule`, `workflow_dispatch` | status check only |

These two produce a status check and nothing else, and no pull request ever runs them.
Whatever they find reaches a person only if someone opens the Actions tab and looks.

Both carry open questions about their future: `recipe-validation` in #2610, and
`scheduled-tests` is the workflow whose matrix defect was #2596.

## What this record is for

The plan orders this ahead of the escalation work deliberately. Turning on escalation for a
workflow whose output nobody consumes converts a silent problem into a noisy one, so which
workflows declare `escalation-policy: none` is decided from this table rather than from
what is easiest to wire up.

A workflow in the last two sections either gains a consumer or is proposed for retirement.
`deploy-recipes` raises the same question from inside the working set. Those decisions are
not made here.
