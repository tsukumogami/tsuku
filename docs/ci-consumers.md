# Who consumes each scheduled workflow's output

Every workflow with an `on.schedule` trigger, and what receives its result. A workflow
whose output reaches no person, no issue, no committed artifact and no notification is
not providing coverage, whatever colour it reports.

**A status check is not a consumer.** A check on a page someone has to visit is exactly
what this record exists to distinguish from a signal that arrives.

Counted 2026-09-20: **21** scheduled workflows, across both `.yml` and `.yaml`.

## Summary

| Category | Workflows |
|---|---|
| Consumer works today | 5 rows, 4 distinct workflows |
| Consumer declared but currently broken | 9 |
| Scheduled run reaches nobody; the same check does reach a PR author | 6 |
| Scheduled run reaches nobody, and nothing else runs it | 2 |

The rows sum to 22 against 21 workflows because `seed-queue` appears twice: it pushes
commits to the queue data, which works, and separately tries to file a report issue,
which does not. Counting distinct workflows, 4 + 9 + 6 + 2 = 21.

Only **one** of the ten issue-filing workflows can currently file an issue.

## Consumer works today

| Workflow | Consumer | Kind |
|---|---|---|
| `r2-credential-rotation-reminder` | An issue labelled `maintenance` | issue |
| `batch-generate` | Commits and pull requests it opens | committed artifact |
| `batch-operations` | Commits it pushes | committed artifact |
| `deploy-recipes` | The deployed registry itself | deployment |
| `seed-queue` (queue half) | Commits it pushes to the queue data | committed artifact |

`r2-credential-rotation-reminder` is the only workflow in this repository whose
issue-filing path works: it declares `issues: write` and uses `maintenance`, which
exists. The others in the next table differ from it in one or both of those respects.

## Consumer declared but currently broken

Each of these names a consumer it cannot reach. The cause is a missing label, a missing
token permission, or both.

| Workflow | Intended consumer | Why it does not arrive |
|---|---|---|
| `curated-nightly` | issue | label `curated-recipe-failure` does not exist |
| `checksum-drift` | issue | label `security` does not exist |
| `weekly-coverage-report` | issue | label `coverage-regression` does not exist |
| `seed-queue` (report half) | issue | label `seeding:review` does not exist |
| `discovery-freshness` | issue | label `discovery-registry` missing **and** no `permissions:` block |
| `r2-cleanup` | issue | label `automation` missing **and** no `permissions:` block |
| `nightly-registry-validation` | issue | labels `r2-unavailable`, `nightly-failure` missing **and** no `permissions:` block |
| `r2-cost-monitoring` | issue | label `r2-cost-alert` missing **and** no `permissions:` block |
| `r2-health-monitor` | issue | label `r2-degradation` exists; no `permissions:` block |

`r2-health-monitor` is the control case for the permission defect: its label exists, so
nothing about the label can explain the failure.

`r2-cost-monitoring`, `checksum-drift`, `weekly-coverage-report` and `seed-queue`'s
report half have never fired their escalation, so their breakage is latent — armed for
the day the condition they watch for first occurs.

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

The plan orders this ahead of the escalation work deliberately. Turning on escalation for
a workflow whose output nobody consumes converts a silent problem into a noisy one, so
which workflows declare `escalation-policy: none` is decided from this table rather than
from what is easiest to wire up.

A workflow in the last two sections either gains a consumer or is proposed for
retirement. That decision is not made here.
