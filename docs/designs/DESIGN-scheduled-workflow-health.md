---
schema: design/v1
status: Planned
upstream: docs/prds/PRD-scheduled-workflow-health.md
problem: |
  Ten of tsuku's twenty-one scheduled workflows fail and none of the failures reach a
  person. Five fail inside their own escalation step; nine referenced labels do not
  exist; five issue-filing workflows declare no token permission; no workflow anywhere
  assigns the issue it files; and several conclude success having done no work. Each
  defect is invisible until the one in front of it is fixed.
decision: |
  Escalation moves out of the monitored workflows into a `workflow_run` listener paired
  with an hourly API sweeper, because the two fail in different places and only the
  sweeper can see a run rejected before job creation. Each scheduled workflow declares
  its escalation policy and its coverage contract in a comment block that a CI check
  reads. Labels become a committed manifest checked by a script that resolves shell
  variables and fails on anything it cannot reduce to a literal. Workflows emit a run
  receipt as they work, so a run that processed nothing cannot conclude success.
rationale: |
  Every choice here is driven by one property: a check must fail when its subject is
  removed, not merely when it changes. That rules out the arrangements that look
  simplest — a single escalation trigger, runtime label creation, end-of-run summary
  counts — because each has a silent-success mode. The evidence for pairing the two
  escalation triggers is a real run of this repository that a name-filtered listener
  could not have matched.
---

# DESIGN: Honest states and a working failure path for scheduled workflows

## Status

Planned

## Context and Problem Statement

The requirements are settled in `docs/prds/PRD-scheduled-workflow-health.md`. This
document settles how they are built.

The technical problem is not that ten workflows are broken. It is that this repository
has no path from a scheduled failure to a person, and that every component which could
have been that path has a mode in which it fails silently. The escalation steps fail on
missing labels; the label references fail against a set nothing validates; the
permission is absent from five workflows and nothing reports its absence; issues that
would be filed carry no assignee, so they would land in a list nobody is subscribed to;
and several workflows conclude `success` having validated nothing, which no amount of
escalation would surface because there is no failure to escalate.

Four independent technical questions follow, and they are settled separately below: how
escalation is triggered and delivered, how label references stop being latent, how a
workflow proves it did its work, and how Homebrew platform tags are resolved.

## Decision Drivers

- **A check must fail when its subject is removed, not merely when it changes.** This
  is the property the whole repair exists to establish, and it disqualifies several
  otherwise-simpler designs.
- **The escalation path must survive the thing it monitors being broken.** Five of the
  ten current failures are escalation steps inside workflows that could not run them.
- **Nothing may suppress its own error.** Two existing workflows already demonstrate
  the failure mode with `2>/dev/null || true`.
- **A declared capability must be verifiably connected.** A workflow declaring that it
  escalates, but not wired to the escalator, is the same defect one layer up.
- **The repository's existing conventions win where they are sound.** There is a
  five-script house pattern for CI checks; a new mechanism should look like it.
- **Repairs must not wait on decisions the repository's owners have not made.**
  Retirement and reshaping are proposals, so the design must let repairs land without
  them.

## Considered Options

### Decision 1 — how escalation is triggered

- **A step inside each workflow.** This is the status quo and is what fails today: a
  workflow that dies early never reaches it, and five of the ten do exactly that.
  Rejected.
- **A reusable workflow or composite action called by each workflow.** Better factored,
  but still invoked from inside the failing run, so it inherits the same blind spot.
  Rejected on the same grounds.
- **A `workflow_run` listener alone.** Fires on another workflow's completion, outside
  the failing run. Two things count against it as a *sole* mechanism, and they are not
  equally strong.

  The weaker one is name filtering. The run behind issue #667 is recorded with
  `name: ".github/workflows/build-essentials.yml"` — the file path, because a workflow
  rejected before job creation has no readable `name:` key — so a listener filtered on
  "Build Essentials" would not match it. But whether `workflows:` must be supplied at
  all, and whether it accepts globs, is **not established**: GitHub's reference
  documentation does not specify either, and the secondary sources disagree. If the
  filter can be omitted or globbed, this objection dissolves, and an in-job check on the
  payload's `path` covers the rest.

  The stronger one is that a listener cannot fire for an event that never occurs, and
  **whether `workflow_run` fires at all for a run that produced no jobs is unknown**. The
  requirements recorded that as an open question; replacing it with the name-filter
  argument would be trading an honest unknown for a tidier one. It stays open, and step 1
  of the implementation settles it.
- **An API sweeper alone.** Polls for non-success runs. It sees the #667 run perfectly,
  since the runs API records it completely, and it is indifferent to whether any event
  fired. Its costs are latency and being itself a scheduled workflow subject to every
  defect in this document.
- **Chosen: both, provisionally.** The sweeper is the load-bearing mechanism: it covers
  the case the listener may be blind to, and it does not depend on an unsettled event
  behaviour. The listener's justification is **latency and nothing else** — an earlier
  draft also credited it with watching the sweeper, which does not hold, because the
  sweeper is itself registered and its next hourly run files the gap for its own prior
  failure. If step 1 shows
  `workflow_run` does fire for job-less runs and the filter can be widened, the listener
  becomes sufficient on its own and the sweeper reduces to a watchdog — that is a
  legitimate simplification and the design should be revisited rather than defended.

### Decision 2 — where the escalation policy is declared

- **A new top-level YAML key.** Rejected on a hard constraint: GitHub Actions rejects
  unknown top-level keys, so this would make all twenty-one workflow files invalid.
- **A single manifest file holding both the per-workflow policy and the escalator's
  coverage.** Machine-readable and in one place, but it collapses the two things the
  requirements need compared: if one file is the only source for both, the two-way
  comparison compares a list against itself and can never fail.
- **Chosen: a comment block in each workflow, plus a separate registry in the
  escalator.** Two independent sources, which is what makes the comparison meaningful.
  The cost is real and worth naming: two places can drift, and the check exists
  precisely because they can. A manifest for the declarations alone — separate from the
  registry — would also satisfy this, and is a reasonable alternative; comments win only
  on keeping the declaration beside the thing it describes.

### Decision 3 — how label references are validated

- **Runtime self-healing (`gh label create` before use).** Already present in two
  workflows and demonstrably ineffective: both discard the result, one lacks the
  permission to do it, and a label created at three in the morning is one nobody
  reviewed. Rejected, and the existing instances are removed.
- **A third-party label-sync action as the check.** The candidates apply a manifest but
  none fails CI on drift, so the check half is missing.
- **A YAML-parsing linter.** Almost every label reference here lives inside a `run:` or
  `script:` block scalar, which a parser sees as an opaque string, so parsing buys very
  little. It is not quite none: at least one genuine YAML
  `labels:` key exists, as an input to a container-build action. It is a Docker image
  label rather than a GitHub issue label, so it is a **false positive the scanner must
  exclude**, not coverage a parser would gain.

  The exclusion is stated as a rule, never a path allowlist: a `labels:` value that is
  wholly a `${{ }}` expression naming a step output is not an issue-label reference. An
  allowlist keyed to a file and line would silently stop covering that file the moment it
  moved, which is the defect the dynamic-reference rule already forbids.
- **Chosen: a text-scanning script in the existing checks directory, against a committed
  manifest, reconciled by scripted `gh label` calls with no delete code path.** An
  absent delete path beats a disabled flag.

### Decision 4 — how a workflow proves it did its work

- **Job outputs carrying declared and attempted counts.** Rejected on two measured
  facts: matrix legs cannot emit per-leg outputs, and a job killed at its timeout cap
  emits nothing at all — which is precisely the case that must not be silent.
- **A final summary step.** Same defect: a run that dies never reaches its summary.
- **Chosen: a run receipt written as work proceeds** — a header line declaring the
  expected set, then one line per item — uploaded with `if: always()` and asserted by a
  separate job that also runs with `if: always()`. The receipt survives the job that
  wrote it.

### Decision 5 — Homebrew platform tag resolution

- **Adopt the existing backward fallback chain.** Rejected, and this is the one option
  that looked obviously right. The chain walks sonoma → ventura → monterey, away from
  the `arm64_sequoia` that exists. It would make the arm64 case worse while appearing
  to address the error string.
- **Construct a corrected tag from the runner's OS version.** Does not help: the
  failing formulae carry no Intel macOS bottle at any version, so no constructed tag
  resolves.
- **Chosen: select from the manifest rather than construct.** Filter the GHCR manifest
  entries tsuku already fetches by OS and architecture, and take the newest that is not
  newer than the target. This removes the hardcoded codename table entirely and cannot
  go stale as new macOS releases ship.

  Two honest qualifications. Selection still needs to know the target's macOS major
  version, which is the same input the rejected alternative needed — what changes is that
  the version is compared against values the manifest supplies rather than against a
  table this repository maintains, so the failure mode is "no entry matches" rather than
  "the table is out of date". And selecting by the resolving machine's version makes a
  resolved digest depend on where resolution happened, which would break golden-file
  reproducibility across runners; the selected tag is therefore pinned into the generated
  plan rather than re-resolved by each consumer.

  It does not make either macOS leg green — nothing in this design does — but it makes
  the failure report what is actually wrong.

Separately, and independent of tag selection, the decomposition path ignores Homebrew's
`rebuild` counter, so a formula published with `rebuild >= 1` is looked up at a manifest
reference that does not exist. This mis-resolves on Linux as well as macOS, it is the
one measured code defect in this family, and it is the only part of it this work
repairs.

## Decision Outcome

Escalation becomes two workflows and one shared script. A `workflow_run` listener reacts
to a registered workflow completing; an hourly sweeper asserts that every non-success
run of a registered workflow has an open tracked item and files the gaps itself. The
sweeper is also what notices the listener has gone quiet.

Each scheduled workflow carries a comment block declaring its escalation policy, its
assignee, and its coverage contract. A CI check parses those blocks, compares the set of
workflows declaring `issue` against the escalator's registry in both directions, and
fails on any difference.

Labels move to a committed manifest. A script in the existing checks directory resolves
every reference — including shell variables with a single static assignment — and fails
on anything it cannot reduce to a literal. The check becomes required only after the
path filter that would make it unsatisfiable is removed.

Workflows emit a run receipt as they process items. A coverage job asserts the receipt
against the declaration. A run that processed nothing fails; a run that processed some
fails naming both numbers.

The macOS bottle failures are diagnosed and not repaired. The tag resolver is improved
so the failure is reported accurately, and the two platforms become separate proposals.

## Solution Architecture

### Components

| Component | Location | Responsibility |
|---|---|---|
| Escalation listener | `.github/workflows/escalate.yml` | Reacts to `workflow_run` completion for registered workflows; opens, comments on, or closes a tracked item |
| Escalation sweeper | `.github/workflows/escalate-sweep.yml` | Hourly; asserts every non-success run of a registered workflow has an open item; files gaps; notices listener silence |
| Shared escalation logic | `.github/scripts/escalate.sh` | Title-keyed dedup, assignee pre-flight and read-back, open/comment/close |
| Policy parser | `.github/scripts/checks/workflow-policy.sh` | Reads the comment blocks, including `escalation-only-on`; enforces declaration presence, registry agreement, and that `escalation-only-on` is present on every workflow declaring both `schedule` and `pull_request` |
| Label check | `.github/scripts/checks/workflow-labels.sh` | Resolves every label reference against the manifest |
| Label manifest | `.github/labels.yml` | The declared label set |
| Coverage assertion | `.github/scripts/checks/assert-coverage.sh` | Compares a run receipt against its declaration |
| Suppression check | `.github/scripts/checks/escalation-hygiene.sh` | Enumerates every step that files or edits an issue and fails on suppressed errors or an undeclared token permission |
| Label drift check | `.github/scripts/checks/label-drift.sh` | Fails when the manifest and the repository's labels disagree |
| Consumer record | `docs/ci-consumers.md` | Names, per scheduled workflow, who or what consumes its output |
| Escalation backstop | `.github/scripts/checks/escalation-backstop.sh` | In the pull-request lint job; queries the Actions API for unescalated non-success runs, independently of the escalator's script and token |

### The declaration block

The policy parser reads declaration blocks from the default branch, not from a pull
request's head, so a pull request cannot register itself with the escalator or change
another workflow's assignee by editing a comment. Each scheduled workflow carries, in
comments near the top:

```
# escalation-policy: issue
# escalation-assignee: <login>
# escalation-only-on: schedule     # optional; present on the six dual-trigger workflows
# coverage: items
```

`escalation-only-on` is the key the three enforcement points in Security Considerations
read. It is absent from most workflows, and present only where a workflow declares
`pull_request` alongside `schedule`; when absent, every conclusion is eligible for
escalation.

or, where escalation is deliberately not wanted:

```
# escalation-policy: none
# escalation-reason: <one line>
# coverage: none
# coverage-reason: <one line>
```

### Every check this design adds carries a zero-floor

Each of the checks named above — the label check, the policy parser, the suppression
check, the label drift check, the coverage assertion and the backstop — reports what it
scanned and what it found, and fails when it scanned nothing or found no subjects to
examine. The rule binds all of them without exception. A check that reports success
having examined nothing is the defect this work exists to remove, and an exemption is
where it comes back.

### The escalator is subject to its own contracts

The mechanism that enforces these rules is the one place where exempting it would be
most tempting and most damaging, so it is stated explicitly.

**Both escalation workflows carry declaration blocks like any other.** The sweeper is a
scheduled workflow, so it declares `escalation-policy: issue` and is registered like the
rest. The listener is not scheduled, but it declares a policy anyway and the sweeper
covers it.

**Both emit receipts, and neither may declare `coverage: none`.** The naive reading is
that a healthy sweeper run has nothing to report and therefore processes zero items,
which would force `coverage: none` and reintroduce silent success in the component
everything else depends on. That reading picks the wrong item. The sweeper's items are
**the registered workflows it checked**, not the problems it found. A healthy run
declares N registered workflows and attempts N; finding zero problems is an ordinary
green with a non-zero attempted count. A sweeper that checked nothing — because its
registry failed to load, or the API call returned empty — attempts zero and fails, which
is exactly the behaviour wanted.

The listener's items are the runs it was notified about, which is one per invocation.

### What watches the watchers

The design claims the sweeper watches the escalator. The reciprocal question is not
rhetorical, and the native scheduled-failure email is not an acceptable answer here —
the requirements rejected it as a sole path because its delivery depends on a
per-account setting that defaults to off.

Two distinct failures, two distinct answers:

- **Either escalation workflow runs and fails.** Both are registered, so a failure of one
  is escalated on the next run of the other — the sweeper covers the listener on its next
  hourly pass, and the sweeper's own failures are escalated by its following pass, since
  a failed run is a non-success run like any other. This covers failure, not silence.
- **The sweeper stops running at all** — disabled, unscheduled, or silently dropped.
  Nothing detects absence by waiting for a failure, because there is no failure. An
  earlier draft put this freshness assertion in the listener, which does not work: the
  listener's only trigger is a registered workflow completing, so GitHub's repository-wide
  inactivity disablement — the exact behaviour the assertion exists to catch — stops the
  sweeper and the listener's trigger in the same instant, and the check never runs.

  **The backstop therefore lives in the pull-request lint job**, alongside the label
  check, where its trigger is someone opening a pull request. That trigger shares no
  failure mode with cron scheduling, is not subject to inactivity disablement, and fires
  when a human is already present to read the result.

  It does **not** ask the sweeper how it is doing. It queries the Actions API itself for
  non-success runs of registered workflows within a stated window and asserts that each
  was tracked — read-only, and implemented independently of `escalate.sh` and of the
  escalator's token. That independence is what lets it cover the common-mode delivery
  failure rather than only the trigger failure.

  Two qualifications keep the assertion true in the healthy state, and both were wrong in
  a previous draft. It applies the same `escalation-only-on` scoping as the other two
  enforcement points, because the six dual-trigger workflows have ordinary failed
  pull-request runs that are not meant to be tracked at all. And it accepts a **closed**
  tracked item as satisfying the assertion, because a tracked item closes when its
  condition clears — so demanding an open one would fail for every run that has since
  been fixed, which is the outcome the repair is aiming at.

  Two earlier drafts of this check were wrong in instructive ways. Asserting that the
  escalator *delivered* something within a window has no ground truth: a healthy
  repository has nothing to file, so the check would hold only while the system was
  broken and go red on every pull request the moment the repair succeeded — under
  merge-blocking pressure, which is how a `continue-on-error` gets added. Asserting
  against the sweeper's own receipt is better, but still trusts the component being
  checked. Querying the API has ground truth in both directions, and it reports how many
  runs it examined so that "nothing to check" is distinguishable from "did not look".

**The residual, stated precisely.** Both escalation workflows share one script, one
permission grant and one assignee pre-flight. Mutual coverage is therefore genuine for
*trigger* failure — one path firing when the other does not — and not for *delivery*
failure, where a defect in the shared script or a revoked permission silences both at
once. The pull-request backstop is the answer to that case, and it is the only control
here independent of both escalation workflows: it shares neither their trigger, their
script, nor their token. If the repository has neither scheduled runs nor pull requests
nothing fires — but a repository with neither is dormant, which is the condition under
which the disablement happens in the first place.

### Data flow

A scheduled workflow runs and writes a receipt artifact as it processes items. Its
coverage job reads the receipt and fails if the attempted set does not match the
declared set. The run concludes. The listener fires on that conclusion, reads the
declaration and the last receipt header, and opens or updates a tracked item assigned to
a named collaborator. If the listener does not act, the sweeper notices within the hour
and files the gap itself.

### Interfaces

The receipt is newline-delimited JSON: a header object carrying the declared count and
the identity digest of the declared set, then one object per processed item. The
assertion script compares counts and identities; identity comparison is what catches a
run that processed the right *number* of the wrong things.

Identity checks belong where identity is chosen, not where it is used. For the matrix
defect in issue #2596 that is the matrix-generation step, where a one-line comparison
between the declared recipe set and the projected set would have failed the day the
field was dropped.

## Implementation Approach

The order matters and is not arbitrary; several steps are unsafe in the wrong sequence.

1. **Settle the two cheap unknowns first.** Verify on a real run that `workflow_run`
   fires for `schedule`-triggered runs, and that an assignment made by the Actions bot
   notifies. Both are load-bearing and both are answerable in under an hour. If the
   second fails, the delivery mechanism changes and the rest of the escalation work is
   rewritten.
2. **Run the two probe experiments** that establish the permission defect and the label
   defect independently, and record the evidence.
3. **Create the label manifest and reconcile it**, then repair the nine references.
4. **Remove the path filter from the linting workflow**, then add the label check, then
   make it required. In that order.
5. **Add the declaration blocks and the policy check.**
6. **Build the escalator and the sweeper**, register workflows incrementally, and add
   the escalation backstop to the pull-request lint job. The backstop is not optional
   follow-up: it is the only control independent of both escalation workflows, so
   shipping the escalator without it leaves the independent-path requirement unmet.
7. **Add the coverage contract**, starting with the workflows whose shortfall is already
   understood.
8. **Land the individual repairs** — they are independent of everything above and can
   proceed in parallel.
9. **File the retirement and reshaping proposals** with their evidence.

Two requirements are deliberately sequenced against the rest, and one is deferred:

- **The consumer record comes first, not last.** The requirements order it ahead of
  escalation, and that ordering is load-bearing: turning on escalation for a workflow
  whose output nobody consumes converts a silent problem into a noisy one. Writing the
  record is cheap and it determines which workflows get `escalation-policy: none`.
- **The R2 Health Monitor threshold is addressed before that workflow escalates**, or
  its policy is `none` with the unreliable verdict as its recorded reason. It is not
  left declaring `issue` against a verdict known not to track the service.
- **The macOS bottle repair is deferred entirely**, per D5, and becomes two separate
  proposals.

## Security Considerations

### Every CODEOWNERS rule in this repository is currently inert

GitHub's own validation endpoint reports `Unknown owner` for both
`@tsukumogami/core-team` and `@tsukumogami/security-team` on every rule in
`.github/CODEOWNERS`, with the suggestion that the team "exists, is publicly visible,
and has write access to the repository". The only active ruleset on the default branch
carries no review requirement.

So `/.github/workflows/**` is **not** review-protected, and an earlier draft of this
section was wrong to say the check scripts were unprotected "while the workflow that
calls them cannot" be modified. Neither is protected. The file looks like a control,
reads like a control, and does nothing — which is precisely the defect class this design
exists to remove, sitting in the repository's own access configuration.

This also invalidates the fix that draft proposed. Adding a `/.github/scripts/**` rule
naming the same two teams would have produced a fourth inert rule and the appearance of
having addressed it. The actual fix is to make the owners resolve — create the teams,
make them visible, and grant write access, or name individual accounts — and to add a
review requirement to the ruleset. That is repository administration rather than a code
change, it predates this work, and it is proposed separately rather than folded in here.

Until it resolves, every check this design adds can be modified by anyone who can push,
with no review. The design proceeds anyway, because a check nobody is required to review
still beats no check, but the limitation is recorded rather than assumed away.

### Fork-controlled content reaches the escalator

Six scheduled workflows also declare `pull_request`: `build-essentials.yml`,
`cargo-builder-tests.yml`, `gem-builder-tests.yml`, `npm-builder-tests.yml`,
`pypi-builder-tests.yml` and `test.yml`. On a pull request from a fork, the workflow
file and everything it produces come from the fork. So the run's `name`, `head_branch`
and `display_title`, and any receipt artifact it uploads, are attacker-authored.

An earlier draft asserted that receipt content originates in this repository's own
workflows and not from forks. That is false, and the consequences follow directly:

- **The event filter is a per-workflow property, enforced at three points.** Six
  registered workflows also declare `pull_request`. For those six — and only those six —
  a run is escalated only when its triggering event is `schedule`. The declaration block
  carries it as `escalation-only-on: schedule`, and three components enforce it: the
  listener via `github.event.workflow_run.event`, the sweeper via the Actions API's
  `event` field (no `workflow_run` context exists there), and the pull-request backstop,
  which would otherwise demand a tracked item for every ordinary failed pull-request run
  of those six.

  Scoping it per workflow rather than applying it globally is load-bearing. A blanket
  `event == 'schedule'` filter would discard the listener's own runs, which carry
  `event: workflow_run`, so the sweeper would stop covering the listener and one of the
  two escalation workflows would be watched by nothing. Enforcing it at only one or two
  of the three points leaves the others filing or demanding issues for fork
  pull-request runs — the issue-spam primitive this section exists to prevent.
- **Receipt content is parsed defensively** — size-capped, schema-validated, and a
  parse failure is a loud failure rather than a skipped assertion.
- **A fork pull request must not be able to open an assigned issue.** Without the event
  filter, any pull-request author would have an issue-spam primitive aimed at a named
  assignee, with no injection required.

### `${{ }}` interpolation cannot be made safe by quoting

An earlier draft said quoting is required at every interpolation. That is wrong. `${{
}}` is substituted textually into the script *before* a shell parses it, so a quote
character in the substituted value terminates the quoting the author wrote. The only
mitigation is to pass the value through the `env:` block and reference it as a shell
variable.

This is not hypothetical here. Two existing workflows interpolate `${{ }}` directly into
an issue-writing step — one into a `run:` block, one inside a JavaScript template
literal — while `r2-health-monitor.yml` shows the correct pattern, passing values
through `env:`. The escalator follows the latter. One of the two is a hardening fix
tracked separately; the other is under assessment, as below.

Both existing sites were checked for reachability, and they did not come out the same
way.

`weekly-coverage-report.yml:109` runs only on `schedule` and `workflow_dispatch`, with
the interpolated value produced by tooling over the default-branch checkout. Reaching it
requires write access. That is hardening.

A second pre-existing site is **under assessment and is not characterised here**. An
earlier draft asserted it was unreachable by outside contributors; that assertion was
withdrawn because it reasoned from how a workflow is intended to be triggered rather
than from the rule by which the trigger actually matches. The re-assessment is happening
outside this document. The general lesson it produced is recorded above and is why this
design applies an event filter to its own escalator: `workflow_run` matches a recorded
workflow name, not the manner in which the run was triggered. That site is treated as
unresolved rather than as either safe or exploitable, and is not characterised further
here.

### Token scope

The escalator needs `issues: write`, `actions: read` **and `contents: read`**. The
second is easy to miss and fatal: declaring a `permissions:` block drops every unlisted
scope to none, and downloading another run's artifact requires `actions: read`. Without
it the escalator cannot read the receipt it exists to report. `contents: read` is listed
for the same reason and is the same mistake one step further on: declaring the block
drops it too, and the escalation script has to be checked out before it can run.

Concentrating `issues: write` in one component means a defect there silences reporting
repository-wide. That is accepted knowingly — the alternative is the permission spread
across every workflow that might need it, which is the arrangement that produced five
broken escalation paths and five latent ones — and the sweeper plus the backstop
assertion are the compensating controls.

### Artifact semantics are load-bearing and default to silence

- `actions/upload-artifact` defaults to `if-no-files-found: warn`, so a missing receipt
  uploads nothing and the step still passes. The design sets `error`.
- Matrix legs writing to one artifact name collide. Under the current artifact action
  that collision fails loudly rather than silently, because uploaded artifacts are
  immutable — but receipts are still named per leg, so the loud failure never arises.
- Artifacts expire. The assertion runs in the same workflow run as the upload, so
  retention does not affect it; the escalator reading a receipt from an older run must
  treat absence as unknown rather than as zero.

### Label reconciliation

Reconciliation runs from the pull-request lint workflow on changes to the manifest, and
on manual dispatch. It needs `issues: write`, which is the same permission the escalator
holds and is granted to that job alone rather than workflow-wide. It applies the
manifest and never deletes. A label renamed in the UI therefore appears as a new
undeclared label rather than a rename, and the drift check reports it; the operator
decides. A label removed from the manifest stays on the repository and is reported.
Colour and description changes in the manifest are applied, so the manifest is
authoritative for presentation but never for existence.

### Dedup key integrity

Tracked items are keyed by exact title. Because a title composed from run metadata could
be influenced by a fork-triggered run, the escalator composes titles only from the
workflow's own declared name and a fixed prefix, never from run-supplied text.

### Action pinning

Every action the new workflows use is pinned to a full commit SHA. This is a rule for
what this design adds, not a description of existing practice: a substantial minority of
`uses:` references in this repository carry mutable refs, including reusable workflows
at `@main` and toolchain actions at `@master`. Bringing those into line is worth doing
and is not part of this work.

## Consequences

### Positive

- A failing scheduled workflow reaches a named person by a path that does not depend on
  the failing workflow working.
- A label reference that does not resolve fails before it merges, and the check cannot
  pass by scanning nothing.
- A run that validated nothing can no longer conclude `success`.
- The two escalation defects are established independently, so the next reader can tell
  which change addressed which.

### Requirement coverage

Every requirement in the PRD maps to a component above or to an explicit deferral. The
ones that are easy to lose, named here so a reader can check rather than assume:

| Requirement | Where it is built |
|---|---|
| No escalation step suppresses its own failure | `escalation-hygiene.sh` |
| Every issue-filing job declares its token permission | `escalation-hygiene.sh` |
| Manifest and repository labels do not drift | `label-drift.sh` |
| A health verdict reflects the service | R2 measurement change, or policy `none` with reason |
| Every workflow has a named consumer | `docs/ci-consumers.md`, written before escalation |
| Both escalation defects established independently | Two probe experiments, step 2 |
| MacOS bottle failures | Deferred; tag resolver improved, repair proposed separately |

### Negative

- **Roughly ten workflows become loudly and permanently red without being able to pass.**
  Recipe Validation reporting `declared=1256 attempted=1193` is a correct check reporting
  a true shortfall, and it will be red every run until the shortfall is separately fixed.
  This is the intended end state. Saying so is not optional: if the design does not state
  it, the first week of noise buys exactly the `continue-on-error` the requirements
  forbid.
- The escalator is a new single point of failure for reporting. The backstop narrows this
  considerably — it shares neither trigger, script nor token with the escalation
  workflows — but it depends on pull requests happening. If both escalation workflows
  stop running at the
  same time, nothing inside the repository notices. That residual is real and is not
  designed away.
- The design carries two triggers where one might do. If step 1 settles the `workflow_run`
  behaviour favourably, the sweeper reduces to a watchdog and the arrangement should be
  simplified rather than kept for symmetry.
- The coverage contract touches fourteen of twenty-one workflows, which is real work
  spread over time rather than one change.

### Mitigations

- A workflow whose shortfall is known and separately proposed declares
  `escalation-policy: none` with that reason, so it stays visibly red without filing
  nightly. The red is preserved; the noise is not.
- The sweeper watches the escalator, and the native scheduled-failure email is the
  recorded third layer beneath both.
- The receipt contract is rolled out incrementally, with the declaration adopted
  everywhere at once so nothing is silently uncovered.
