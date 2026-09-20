# Who consumes each scheduled workflow's output

Every workflow with an `on.schedule` trigger, and what receives its result. A workflow
whose output reaches no person, no issue, no committed artifact and no notification is
not providing coverage, whatever colour it reports.

**A status check is not a consumer.** A check on a page someone has to visit is exactly
what this record exists to distinguish from a signal that arrives.

**A green run is not a consumer either.** Several workflows below run green and deliver
nothing, and one of them was miscategorised in the first draft of this record for exactly
that reason. Each row is placed by what arrived, checked against run history and against
the commit log, not by what the workflow file declares.

Counted 2026-09-20: **21** scheduled workflows, across both `.yml` and `.yaml`.

## Summary

| Category | Workflows |
|---|---|
| Consumer works today | 4 |
| Consumer declared but does not arrive | 9 |
| Scheduled run reaches nobody; the same check does reach a PR author | 6 |
| Scheduled run reaches nobody, and nothing else runs it | 2 |

4 + 9 + 6 + 2 = 21. Every workflow is counted once; none appears in two rows.

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

**`batch-generate` has produced nothing since 2026-08-17.** Its first step checks for
open pull requests from `batch/` branches and skips the whole run if any exist. Two are
open, #2426 since 2026-06-26 and #2534 since 2026-08-17, so every scheduled run since has
reported success with every subsequent step skipped. The guard is deliberate and the
consumer is not broken; the workflow is idling behind pull requests it opened itself and
cannot merge. Its output resumes when those merge or close.

**`deploy-recipes` is deprecated in its own header.** It deploys `registry.tsuku.dev`,
which `deploy-website.yml` supersedes by serving `recipes.json` from the same origin, and
the file says it is kept for backwards compatibility during the transition (#353). The
deployment succeeds and the consumer is real. Whether that consumer should still exist is
the retirement question this record feeds, not one it answers.

**`batch-operations` commits rarely by design.** Its `Persist control file` step runs
green on every scheduled run and exits early when `batch-control.json` is unchanged, so
one commit reaching main (2026-03-03) reflects a circuit breaker that rarely trips rather
than a path that does not work.

## Consumer declared but does not arrive

Nine workflows name a consumer that does not receive their output. They do not share a
cause, and the difference decides what would fix them, so they are grouped by where
delivery stops.

### The escalation step runs and fails

| Workflow | Intended consumer | Why it does not arrive |
|---|---|---|
| `curated-nightly` | issue | label `curated-recipe-failure` was missing until 2026-09-20; `issues: write` is declared |
| `discovery-freshness` | issue | no `permissions:` block anywhere in the file |
| `nightly-registry-validation` | issue | no `permissions:` block anywhere in the file |
| `r2-cleanup` | issue | no `permissions:` block, **and** label `automation` still does not exist |
| `r2-health-monitor` | issue | no `permissions:` block; its label `r2-degradation` has always existed |

Each of these has a failing step whose name is the escalation itself: `Create curated
recipe failure issue`, `Create failure issue`, `Create R2 unavailable issue`, `Create
summary`, `Handle health status`. A workflow that cannot report its own failure is a
defect separate from whatever it was watching.

`r2-health-monitor` is the control case for the permission defect. Its label exists and
always has, so nothing about a label can explain why it fails.

### The job dies before the escalation step runs

| Workflow | Intended consumer | Where it stops |
|---|---|---|
| `seed-queue` | queue commits **and** review issues | fails at `Run seeding` |
| `weekly-coverage-report` | issue | fails at `Build tsuku` |

Neither reaches its consumer step at all, so no label and no permission would fix either.
Both declare `issues: write`, and `seed-queue` additionally declares `contents: write`.

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

## What the label bootstrap changed, and what it did not

Eight label names were created on 2026-09-20. Before that, nine names referenced across
these workflows did not exist; afterwards only `automation` does not, and `r2-cleanup` is
the single workflow still referencing it.

That removes the first of two stacked defects. It does not by itself make any of these
workflows file an issue, and this record does not claim it does:

- Five workflows declare no `permissions:` block. For them the label was never the binding
  constraint, and nothing has changed.
- Three — `curated-nightly`, `checksum-drift`, `weekly-coverage-report` — declare
  `issues: write` and were blocked only by the missing label. Whether they now file is
  **unproven**. `checksum-drift` will not say until drift occurs,
  `weekly-coverage-report` dies before reaching the step, and `curated-nightly` fires
  nightly, so its next run is the first real test.
- `seed-queue` has both the label and the permission and still delivers nothing, because
  it never gets that far.

Of the ten workflows that try to file an issue, exactly one, `r2-credential-rotation-reminder`,
has been observed doing it. How many *can* is not knowable from green runs, and one pass
would not establish it either.

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
