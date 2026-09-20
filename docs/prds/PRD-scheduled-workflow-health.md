---
schema: prd/v1
status: Draft
problem: |
  Ten of tsuku's twenty-one scheduled workflows are failing, five of them since
  February, and no failure reaches a person. Five fail inside their own escalation
  step; nine referenced labels do not exist; five issue-filing workflows declare no
  token permission; and no workflow anywhere assigns the issue it files, so even a
  repaired escalation path would file into a list nobody is subscribed to. Several
  of the ten also report success without having done their work.
goals: |
  Every scheduled workflow ends in a state a maintainer can defend: passing because
  what it checks works, or carrying a decidable proposal for its future with the
  evidence attached. A failure afterwards reaches a named person who does not have to
  be watching the repository. A workflow
  that references a label nobody created fails at review time rather than on the day
  its first real failure needs reporting, and no workflow reports success without
  having done its work.
upstream: docs/briefs/BRIEF-scheduled-workflow-health.md
motivating_context: |
  Discovery Registry Freshness has failed every weekly run from 2026-06-29 to
  2026-09-14 on a missing label. It detected real registry drift on every one of those
  runs, and the count it found grew from six entries to nine while it failed to report
  any of them. That is the whole problem in one workflow: the check works, and its
  finding reaches nobody.
---

# PRD: Honest states and a working failure path for scheduled workflows

## Status

Draft

## Problem Statement

tsuku runs twenty-one scheduled workflows. Ten are substantially failing, six of them
having not passed once in their last twelve scheduled runs, with last successes reaching
back to January and February 2026.

The failures reach nobody, and there are three independent reasons for that, stacked one
behind the other. Each is invisible until the one in front of it is fixed.

**First, the escalation step itself fails.** Five of the ten die inside the step meant
to report the failure. Nine label names, referenced across fifteen call sites, do not
exist in the repository, so `gh issue create --label <name>` fails before it files
anything. Separately, five workflows that file issues declare no `permissions:` block at
any level, so the default token carries no `issues: write`. Because `gh` resolves a
label before it calls `createIssue`, a workflow missing both shows only the label error
and the permission error stays hidden behind it. Four workflows carry both faults; three
of those four are red today and the fourth has simply never had its escalation condition
met.

**Second, a filed issue is not a read issue.** No workflow in the repository sets
`--assignee` or `assignees:` anywhere. Repo watching does not cover workflow runs at any
level, which is exactly why the Actions tab goes unread — but an unassigned,
unsubscribed issue lands in a list read by the same people who are not reading the
Actions tab. Fixing labels and permissions makes issues get filed. It does not make them
get read.

R2 Cleanup shows a sharper version of the same problem: it attempts to file an issue
only when it has found cleanup candidates, so it is green exactly when it has nothing to
say and red exactly when it has something to report. Build Essentials fails only on its
Intel macOS leg, on a platform tag its runner outgrew. The arm64 bottle failures are a
separate matter observed elsewhere — in Recipe Validation and Curated Recipe Nightly,
not in Build Essentials — which is part of why they are diagnosed here rather than
repaired.

**Third, some of these workflows report success without having done their work.**
Nightly Registry Validation downloads every object, matches none of them against the
layout it expects, and concludes green; issue #2448 records this. Recipe Validation's
macOS job terminates on `No space left on device` partway through the corpus while its
sibling is cancelled at the job cap. Curated Recipe Nightly has never completed a run in
156 attempts, and validates the full registry rather than the curated subset its own
discovery step identifies. For these, repairing the escalation path would make them
reliably announce a verdict that means nothing.

There is a fourth problem that only appears once the first is fixed. R2 Health Monitor's
`degraded` verdict comes from a 2000 ms threshold measured around an entire `aws s3api`
process invocation, interpreter startup included. Its recorded latencies are strictly bimodal.
Across the last thirty scheduled runs, twelve samples fall between 1097 ms and 1306 ms
and eighteen between 3054 ms and 5692 ms, with nothing in between; the 2000 ms threshold
sits in that gap. No run has ever recorded an actual failure to reach R2. Granting it the permission it lacks, and nothing
else, would have it raising and closing issues against a verdict that does not track the
service.

Five of the nine missing labels have never fired at all, because the workflow dies
earlier or the escalation condition has not yet been met. Checksum Drift is the clearest
case: it runs daily, passes every run, correctly declares `issues: write`, and files its
issue with a `security` label that does not exist. A daily check for tampering with
recipe downloads is green every day and will fail to report the first drift it ever
finds.

Nothing in the repository connects a label argument to the set of labels that exist, and
nothing checks before a workflow merges. Two workflows attempt to self-heal with `gh
label create ... 2>/dev/null || true`. Both discard the result, so neither has ever
shown whether it worked. One of the two additionally lacks the permission that creating
a label requires; the other declares it, but the label it creates already exists, so its
attempt has never had to do anything.

## Goals

1. Every scheduled workflow ends in a state a maintainer can defend — passing honestly,
   or carrying a proposal for its future, with the evidence attached, that the
   repository's owners can decide on its own merits. Acting on such a proposal is not
   part of this work.
2. A scheduled failure reaches a named person who is not required to be watching the
   repository, through a mechanism chosen deliberately and stated per workflow.
3. A reference to a label that does not exist fails before it merges.
4. No scheduled workflow reports success without having done the work it claims.
5. The repair distinguishes the stacked defects, so that a reader can tell which change
   addressed which fault.

## User Stories

- **As a tsuku maintainer**, I want to be told when a scheduled check finds a real
  problem, so that I act on the problem rather than discovering months later that the
  check had been reporting it to nobody.
- **As a maintainer auditing CI**, I want each scheduled workflow to be either green,
  or red for a reason I can read, or gone with the reasoning recorded, so that I do not
  have to re-triage the same wall of red every time I look.
- **As a contributor adding a scheduled workflow**, I want to be told at review time
  that the label I referenced does not exist, so that I do not merge an escalation path
  that is armed to fail.
- **As a maintainer reviewing this repair**, I want to see which change fixed which
  fault, so that I can tell that both were real rather than reading one combined patch
  that proves neither.

## Requirements

Terms used below, defined once so they are not read loosely:

- **Escalation policy** — one of `issue` or `none`, declared per scheduled workflow.
  `issue` means a failing run causes an issue to be filed and assigned. `none` means
  no escalation, and carries a one-line reason.
- **Named consumer** — a person, an issue, a committed file in the repository, or a
  notification that a person receives. A status check on a page is not a consumer.
- **Declared label set** — a file committed to the repository listing every label the
  repository is meant to have. It is the label check's only truth source.
- **Attempted-item count** — the number of items a run actually processed, compared
  against the number its preparation step declared it would process.
- **Blocking** — configured as a required status check on pull requests.
- **Escalation issue** — an issue reporting that a scheduled workflow *run failed*.
  These are what R4 moves outside the monitored workflow, because a workflow that
  cannot run cannot report on itself.
- **Finding issue** — an issue reporting *what a workflow discovered* while running
  successfully: stale registry entries, cleanup candidates, checksum drift. These stay
  inside the workflow that made the finding, since only it has the finding. They are
  still subject to R2, R3 and R6 — assigned, unsuppressed, and explicitly permissioned —
  and the label check covers both kinds. The distinction matters because R4 would
  otherwise read as forbidding a workflow from filing its own findings, which is not
  intended and would remove the reports this work exists to restore.

### Functional — the escalation path

**R1.** Every workflow with an `on.schedule` trigger declares exactly one escalation
policy. A `none` declaration carries a one-line reason. The enumeration that verifies
this covers `*.yml` and `*.yaml`, and reports how many workflow files it scanned.

**R2.** Every issue filed by an escalation path is assigned to at least one repository
collaborator, and the assignment is present on the issue after creation. Sending an
assignee is not sufficient evidence: the REST API accepts and silently discards an
assignee who lacks repository access, so the assignment must be read back.

**R3.** No escalation step suppresses its own failure. No `2>/dev/null || true`, no bare
`continue-on-error: true`, and no construct that forces success on an escalation step.
An escalation that cannot complete fails its run visibly.

**R4.** Escalation fires from outside the workflow it monitors, on that workflow's run
conclusion, and covers the `failure`, `cancelled`, `timed_out` and `startup_failure`
conclusions. A workflow that fails in its first step, is cancelled at its job cap, or is
rejected before any job is created must still escalate.

**R5.** Repeated failures of the same condition produce one tracked item rather than one
per run, and that item closes when the condition clears.

**R6.** Every job that files or edits an issue declares the token permission it needs
explicitly at the workflow or job level.

**R6b.** Every workflow declaring the `issue` policy is registered with the escalation
mechanism, and the set of declaring workflows equals the set the mechanism covers. A
declared policy that is not wired up escalates nothing.

**R6c.** A failure of the escalation mechanism itself reaches a person by a path that
does not depend on the escalation mechanism working.

### Functional — labels stop being latent

**R7.** A CI check resolves every label referenced anywhere in `.github/workflows/`
against the declared label set, and fails naming file and line for any that does not
resolve. It scans `*.yml` and `*.yaml`, and extracts references from `--label`,
`--add-label`, and `actions/github-script` `labels:` in both string and array form,
splitting comma-joined values.

**R8.** A label referenced through a shell variable is resolved against a single static
assignment in the same file. A reference that cannot be reduced to a literal fails the
check. Skipping an unresolvable reference is a defect, not a fallback.

**R9.** The declared label set is a file in the repository. A job reconciles it with the
labels the repository actually has, deletion of labels absent from the file is disabled,
and a check fails when the file and the repository have drifted apart.

**R10.** All nine currently-missing label references are resolved — each either created
as a declared label or repointed at an existing one — before the check in R7 is made
blocking.

### Functional — honest verdicts

**R11.** No scheduled workflow concludes `success` on a run whose attempted-item count is
zero. A run that processed its full declared input set and found no problems concludes
`success` normally; a run that processed nothing concludes as a failure, or as a skip
that is itself visible as a non-success conclusion.

**R12.** A run that does not cover its full declared input set fails, naming the count it
reached and the count it declared. This applies whether the shortfall comes from disk
exhaustion, a `timeout-minutes` cap, or an early exit. For the workflows that cannot
currently cover their input set, this requirement is satisfied by the shortfall becoming
visible and reported — not by the shortfall being eliminated, which R17 places outside
this work.

**R13.** A workflow's declared scope matches the set its own discovery step produces.

**R14.** A health verdict reflects the service being measured rather than the measurement
apparatus. Widening a threshold so that existing measurements fall under it does not
satisfy this; measuring the operation rather than the surrounding process does. A
workflow whose measurement is not corrected within this work does not escalate from the
uncorrected verdict: its escalation policy becomes `none` with that as its reason.

**R15.** Every scheduled workflow has a named consumer, or is proposed for retirement
under R17.

### Functional — the specific repairs

**R16.** These individually-diagnosed defects are repaired without weakening what the
workflow checks:

- Weekly Coverage Report's hardcoded `go-version: '1.22'`, against a `go.mod` requiring
  1.25.8, in the only workflow not using `go-version-file`.
- Scheduled Tests dropping the `recipe` field its own matrix declares, so all seven tests
  that declare one resolve from the registry instead. Six have a same-named registry
  recipe and therefore pass while exercising a different code path; only the seventh,
  whose name is absent from the registry, fails visibly. Tracked as #2596.
- Scheduled Tests' cask app-bundle leg, whose verification environment does not carry the
  tool-home variable the check reads.
- Seed Queue's audit writer creating its root directory but not the directory implied by
  a scoped package name, so every scoped package fails.
- Homebrew bottle resolution for the Intel macOS runner, which asks for a Sonoma tag
  against a Sequoia runner.

The arm64 bottle failures, the manifest 404s, and the uncharacterised decomposition
failures are **diagnosed but not repaired here**. This is a deliberate narrowing of the
upstream BRIEF, which placed the macOS bottle repair in scope as repair rather than
diagnosis. The narrowing is recorded in D5: those failures do not share a cause with the
Intel one, their cause is not yet established, and committing to repair an undiagnosed
defect would be committing to unknown work.

### Governance

**R17.** Retiring a scheduled workflow, or removing its schedule, is proposed and never
executed by this work. Each proposal is a separate, separable item carrying its own
evidence, so it can be accepted or rejected on its own without blocking any repair. No
pull request from this work deletes a scheduled workflow, removes an `on.schedule`
trigger, or disables a scheduled run. Eliminating the input-set shortfalls behind R12,
and reshaping a workflow's scope under R13, are likewise proposed rather than executed
where doing so would change what a workflow checks.

### Non-functional

**R18.** Every check introduced or relied on by this work is proved by removing its
subject, not merely by changing it. A check that cannot fail is not evidence.

**R19.** Every check reports what it scanned and what it found, and fails when it scanned
zero files or found zero references to check.

**R20.** The two stacked escalation defects are established independently, each by a
single-variable experiment with a control, and the method admits a disconfirming
outcome. The experiments do not stage changes through the production workflows: those
gate their escalation on conditions that cannot be reproduced on demand, so a dispatched
run can conclude green having attempted nothing. If the experiment shows the default
token does carry `issues: write`, the permission defect does not exist and R6 is
withdrawn rather than satisfied.

## Acceptance Criteria

Each criterion names what it inspects and what makes it fail. Where a criterion could be
satisfied by a run that did nothing, it states the floor that prevents it.

### The escalation probe — proving the two defects independently

The two stacked defects are proved with a temporary probe workflow rather than by
staging changes through the production workflows. Most production workflows gate their
escalation on conditions that are not reproducible on demand — a verdict, a validation
outcome, the presence of cleanup candidates — so a dispatched run can conclude green
having attempted nothing. R2 Health Monitor is the exception: it carries a `force_issue`
dispatch input that drives its escalation path directly, and that input is what AC6 and
AC11 use where a real production escalation is wanted. It is not used for the two
experiments below, because a probe isolates one variable and a production workflow
carries everything else with it. Labels are also repository-global and R9 disables their
deletion, so a staged intermediate state could not be recreated if a run were
inconclusive. The probe is unconditional, repeatable, and touches no production
workflow.

- [ ] **AC1.** A temporary `workflow_dispatch`-only probe workflow, declaring **no**
      `permissions:` block, whose single step unconditionally runs `gh issue create`
      using `r2-degradation` — a label that already exists — is dispatched. Its run
      fails carrying the literal text `Resource not accessible by integration`. The run
      URL and quoted log line are recorded in the pull request.
- [ ] **AC2.** The same probe, with `permissions: issues: write` added and nothing else
      changed, is dispatched again and creates an issue successfully.
- [ ] **AC3.** A second probe, declaring `permissions: issues: write` and naming a label
      that does not exist, fails carrying the literal text `could not add label`. This
      establishes the label defect independently of the permission defect.
- [ ] **AC4.** If AC1 does not hold — if the probe files successfully with no
      `permissions:` block — the default token does carry `issues: write`, the permission
      defect does not exist, R6 is withdrawn, and AC2 is struck rather than satisfied.
      This outcome is recorded either way, because the repository's default
      workflow-permission setting could not be read directly.
- [ ] **AC5.** Both probe workflows are removed from the repository before the work is
      complete, and no probe remains on the default branch.

### Escalation reaches a person

- [ ] **AC6.** An escalation issue filed on a real run is read back from the API and has
      a non-empty `assignees` array containing the intended collaborator.
- [ ] **AC7.** An assignment attempt naming a non-collaborator fails the run, rather than
      producing an issue with an empty `assignees` array. The REST API accepts and
      silently discards such an assignee, so the read-back is the only evidence.
- [ ] **AC8.** It is demonstrated on a real run that the assignee receives a notification
      while their repository watch level is not "watch all activity". If this cannot be
      demonstrated, D1 is reopened and the mechanism reconsidered before the work
      proceeds.
- [ ] **AC9.** Escalation is demonstrated for a `failure` conclusion, for a `cancelled`
      conclusion, and for a `timed_out` conclusion — the last matters because R12 names a
      `timeout-minutes` cap as a real source of shortfall in this repository. For `startup_failure`, either escalation is demonstrated,
      or it is established that the mechanism cannot observe that conclusion and the
      blind spot is recorded with a stated fallback. Assuming coverage is not acceptable:
      issue #667 records a real past run of this repository rejected before any job was
      created, which is precisely the case a `workflow_run`-based escalator may not see.
- [ ] **AC10.** A failure of the escalation mechanism itself is surfaced to a person, by
      a path that does not depend on the escalator working. A red run is not sufficient
      evidence, because the whole premise of this PRD is that a red run reaches nobody.
- [ ] **AC11.** Running the escalation path three times against the same failure leaves
      exactly one open issue with an increased comment count; running it once against a
      healthy state closes that issue.
- [ ] **AC12.** Every workflow declaring the `issue` escalation policy is registered with
      the escalation mechanism, verified by a check that compares the set of declaring
      workflows against the set the escalator actually covers and fails on any difference
      in either direction. A declared policy that is not wired up is the defect this PRD
      exists to remove, one layer up.
- [ ] **AC13.** No escalation step suppresses its own failure. A check enumerates every
      step that files or edits an issue — wherever it lives, including inside the
      escalation mechanism itself — reports how many it found, fails if that number is
      zero, and fails on any `2>/dev/null`, `|| true`, or `continue-on-error: true`
      applied to one. The enumeration is not scoped to the scheduled workflows, because
      R4 moves escalation out of them; a check scoped that way would go green by finding
      nothing precisely when R4 succeeds.
- [ ] **AC14.** Every job that files or edits an issue declares its token permission
      explicitly. A check reports the count of such jobs, fails if the count is zero, and
      fails on any that does not declare.

### The escalation policy declaration

- [ ] **AC15.** A check enumerates every workflow with an `on.schedule` trigger and fails
      if any lacks an escalation policy, or declares `none` without a reason. It reports
      the count of scheduled workflows it found — not the count of workflow files — and
      fails if that count is zero or does not equal the number independently known to
      carry a `schedule:` trigger.
- [ ] **AC16.** Adding a scheduled workflow in a file named `*.yaml` with no escalation
      policy makes AC15's check fail. Renaming the repository's existing `.yaml`
      scheduled workflow to `.yml` and back does not change any check's result. The
      first clause proves extension coverage; the second proves it is not accidental.

### Labels stop being latent

- [ ] **AC17.** The label check, run against a committed fixture of the repository as it
      stood at 2026-09-20 so the criterion stays re-runnable after the repair, reports
      exactly nine missing names across fifteen call sites and exits non-zero.
- [ ] **AC18.** The label check passes the workflow whose shell variable resolves to an
      existing label and fails the one whose variable resolves to a missing label, in the
      same run, with no per-site configuration.
- [ ] **AC18b.** Introducing a label reference the resolver cannot reduce to a literal —
      a variable assigned conditionally, or built by string concatenation — makes the
      check fail naming that file and line. The repository contains no such reference
      today, so this criterion is tested by adding one deliberately; without it, an
      implementation that silently skips whatever it cannot parse passes every other
      criterion here.
- [ ] **AC19.** Deleting an in-use label from the declared set turns the check red and
      names the referencing file and lines. The label chosen is one whose name does not
      appear literally at those lines, so the test also proves the variable resolver ran.
- [ ] **AC20.** Stripping all label arguments from the workflows makes the check fail on
      a zero-reference floor rather than report success.
- [ ] **AC21.** Adding a label to the declared set results in it existing on the
      repository after the reconciliation job runs; deleting a label from the repository
      while leaving it in the file makes the drift check fail; and a label present on the
      repository but absent from the file is not deleted.
- [ ] **AC22.** The label check is configured as a required status check on pull
      requests, and a pull request introducing a reference to a nonexistent label is
      blocked from merging by it.
- [ ] **AC22b.** The label check, run against the live repository rather than the frozen
      fixture, reports zero unresolved references before it is made blocking. The fixture
      in AC17 pins the extractor's coverage; this pins the actual repair.
- [ ] **AC23.** Discovery Registry Freshness completes a run that files and assigns its
      issue successfully, ending a failure streak running back to at least 2026-06-29.
      The issue names the invalid entries it found, and the criterion is not satisfied by
      a run that found none.

### Honest verdicts

- [ ] **AC24.** A workflow run whose attempted-item count is zero concludes as something
      other than `success`, demonstrated by pointing a run at an empty input set.
- [ ] **AC25.** For every scheduled workflow this work repairs, a run reports both its
      attempted-item count and its declared-item count, the two are equal, and both are
      greater than zero. For every workflow whose shortfall is not repaired here, a run
      reports both numbers and concludes as a failure.
- [ ] **AC26.** For each workflow whose declared scope is meant to match its own
      discovery step, a run's processed set is compared against that step's output and
      the two are shown to be equal, with the size of both reported and non-zero.
- [ ] **AC27.** Each repair in R16 is demonstrated against what the defect actually
      broke, not against the run's colour. Where the symptom is a failing job, the job
      completes its declared input set with both counts reported and non-zero. Where the
      symptom is a passing job doing the wrong thing — the matrix `recipe` field being
      dropped, which leaves six of seven tests green while installing a registry recipe
      instead of the fixture they declare — the criterion asserts the **identity** of what
      was resolved: each of the seven tests is shown to install from its declared path,
      and the check is shown to fail on all seven before the fix rather than on the one
      that was red. Counts cannot satisfy this, because attempted and declared counts
      already match while the identities differ. In no case is a matrix leg removed, a
      `--label` dropped, an exit code swallowed, a `continue-on-error` added, or a
      threshold widened.
- [ ] **AC28.** For the R2 Health Monitor threshold, one of two outcomes is recorded, and
      which one is stated explicitly. Either the measurement is shown to cover the storage
      operation rather than the surrounding process invocation, with a recorded window
      showing a healthy service yielding a healthy verdict on the large majority of runs;
      or the measurement is not changed, and the workflow's escalation policy is set to
      `none` with the unreliable verdict as its recorded reason. Leaving the threshold
      unexamined while the policy remains `issue` does not satisfy this criterion.

### Governance and proof discipline

- [ ] **AC29.** Every workflow proposed for retirement or reshaping has its reasoning and
      evidence recorded in an issue that can be decided on its own merits. The count of
      such proposals is stated, so that "none were proposed" is distinguishable from
      "none were needed".
- [ ] **AC30.** No pull request produced by this work deletes a scheduled workflow file,
      removes an `on.schedule` trigger, or disables a scheduled run. Verified by
      inspecting the diff of every pull request in this work.
- [ ] **AC31.** Every scheduled workflow has a named consumer recorded, and the record
      says what kind of consumer it is. A workflow with no consumer appears in AC29's
      proposal count instead.
- [ ] **AC32.** Each check introduced by this work is demonstrated failing when its
      subject is removed, with the mutation applied and the resulting exit status
      recorded. A check that only fails when its subject is altered does not satisfy this.

## Out of Scope

- **Executing any retirement or reshaping.** The upstream BRIEF placed retiring a
  workflow in scope. This PRD narrows that deliberately: retirement and reshaping are
  proposed here with their evidence, and the decision belongs to the repository's
  owners. R17 states the rule; this entry records that it is a narrowing of what the
  BRIEF scoped, so the change is visible rather than silent.
- **Repairing the arm64 macOS bottle failures, the manifest 404s, and the
  uncharacterised decomposition failures.** Diagnosed here, repaired separately. Also a
  narrowing of the BRIEF, recorded in R16 and argued in D5.
- **Cutting a release.** Repairing these workflows may make a release look due; it is
  separate work.
- **Other repositories.** No other repository in the organisation files issues from a
  workflow, so there is nothing to roll this pattern out to yet.
- **A general-purpose alerting or on-call platform.** The goal is that failures reach a
  maintainer, not that the project acquires a notification product.
- **Making a scheduled run gate a pull request.** GitHub evaluates pull-request checks
  only for runs triggered by `push`, `pull_request`, `pull_request_review`,
  `pull_request_target`, `deployment` or `deployment_status`. A scheduled run can never
  be a required check, so no requirement here depends on it.
- **Non-scheduled workflows**, except where they share a file or a referenced label with
  a scheduled one. Issue #2590 covers the same defect class in a `pull_request`-only
  workflow and is not closed by this work.
- **Adopting a general workflow linter.** Worth doing for the unlinted workflow files,
  but it does not solve the label problem and should be argued on its own.
- **The four labels referenced by the repository's own documentation** rather than by any
  workflow. Two are enforced by an existing CI script and two are retired upstream, so
  the right fix differs per label; adjacent work, proposed separately.

## Decisions and Trade-offs

This section closes the open questions the upstream BRIEF deferred.

**D1. What makes a signal actually reach someone: assignment, not artifact type.**
The BRIEF's gate question was whether filing an issue is any better than the Actions
tab, given that both are pages nobody reads. The answer turns on notification behaviour,
and the deciding fact is that repository watching does not cover workflow runs at any
level — GitHub's custom watch settings cover issues, pull requests, releases, security
alerts and discussions, and not Actions. So an issue genuinely is subscribable where a
workflow run is not. But subscription alone still requires someone to have opted in.
Assignment is the one mechanism documented to notify a person independently of their
watch level, and it has the property of being visible from inside the repository: an
assignee is greppable in the workflow file and reviewable under CODEOWNERS.
*Alternatives considered:* team `@`-mention, rejected because a team mention posted by
`GITHUB_TOKEN` renders as plain text and notifies nobody — a fourth instance of the same
silent no-op family as the missing labels; the native scheduled-failure email, rejected
as a sole path because the Actions notification setting defaults to not notifying; a
required status check, rejected because GitHub cannot evaluate a scheduled run as a PR
check.

**D2. Escalation lives outside the workflow it monitors.**
A workflow that fails in its first step cannot report its own failure from a later step,
and the five failures in this repository that die inside their own escalation step are
the evidence. Escalation therefore fires on a run's conclusion from outside.
*Trade-off:* one component holding the permission to file issues is a concentration of
privilege, and it becomes a single point of failure for reporting. That is accepted
because the alternative — the permission spread across every workflow that might need it
— is the arrangement that produced five broken escalation paths and five more latent
ones.

**D3. Missing labels fail before merge, not at runtime.**
Runtime self-healing is already present in this repository in two workflows and neither
instance demonstrates that it works. Both discard the result with `2>/dev/null || true`,
so neither has ever reported whether the label was created. One of the two also lacks
the permission that creating a label requires; the other declares it but creates a label
that already exists, so its attempt has never had to do anything. The pattern's defect
is therefore not only the missing permission — it is that the outcome is unobservable by
construction, and that a label created at three in the morning is one nobody reviewed.
The check therefore runs in CI against a declared label set committed to the repository.
*Trade-off:* a committed manifest can drift from the repository's real labels, so a
reconciliation job and a drift check are required, and deletion is disabled so that
reconciliation cannot strip labels off live issues.

**D4. Unresolvable label references fail the check.**
The two dynamic references in this repository point in opposite directions: one resolves
to a label that exists and one to a label that does not. A checker that skipped what it
could not parse would pass this repository while missing a real gap — reproducing the
exact defect class this work exists to remove. Simple single-assignment variables are
resolved; anything else fails loudly.

**D5. The macOS bottle failures are at least two defects, not one.**
An earlier reading treated them as a single defect because they share an error string.
They do not share a cause. The Intel leg runs on a Sequoia runner while the code
asks for the Sonoma tag. Two distinct things are wrong there, and naming only the first
would send an implementer to the wrong line. The decomposition path calls a singular
`getPlatformTag` returning one tag with no alternatives. A plural `getPlatformTags` with
an ordered fallback chain already exists elsewhere in the tree, but it is passed a macOS
version of `0` behind a TODO that was never done, so it defaults to Sonoma and walks
backwards from there. Neither path can reach a Sequoia tag. Adopting the existing chain
is necessary and not sufficient; the version has to arrive too. The arm64 leg runs on a Sonoma runner and asks for the Sonoma tag, so its
failures are not a version mismatch and a fallback chain would not address them. The
remaining arm64 failures, the manifest 404s, and a set of uncharacterised decomposition
failures need their own diagnosis, which the DESIGN owns.

**D6. Three workflows cannot reach an honest green inside this work. What is repairable
in them is repaired; what is not becomes a proposal.** Recipe Validation dies on disk
exhaustion partway through its corpus with its sibling cancelled at the job cap; Curated
Recipe Nightly has never completed a run and validates a set an order of magnitude
larger than the one it advertises; Weekly Coverage Report writes its report to a runner
workspace that nothing reads. These three are not exempt from R16: Weekly Coverage
Report's Go pin is repaired here regardless, because a workflow proposed for retirement
should not also be failing for a reason nobody looked at. What is not repaired is the
part that would change what the workflow checks or how much of a corpus it can process —
the disk exhaustion, the job cap, and Curated Recipe Nightly's scope. Those become
separate proposals carrying their own evidence, per R17, AC29 and AC30, decided by the
repository's owners. The repairs do not wait on those decisions. Narrowing Curated
Recipe Nightly to the recipes its own discovery step identifies is a reduction in what
runs, and is not a weakening: the workflow is currently not running the check its name
advertises, and the reduction makes its verdict mean what it says.

**D6b. The R2 latency threshold is pulled back into scope, reversing the BRIEF.**
The BRIEF placed the threshold's validity out of scope as a question about what the
workflow checks. That was right in isolation and is wrong once escalation works. The
recorded latencies are bimodal: across the last thirty scheduled runs, twelve samples
fall between 1097 ms and 1306 ms and eighteen between 3054 ms and 5692 ms, with nothing
in between, and the 2000 ms threshold sits in that gap. No run has ever recorded an
actual failure to reach R2. The window is named because the clusters move, and a claim
about their extremes is only checkable against a stated set of runs. Granting this
workflow the permission it lacks, and nothing else, would have it raising and closing an
issue against a verdict that does not track the service. Its own dedup logic holds that
to one open issue at a time, so the cost is churn and false signal rather than volume. R14 and AC28 therefore
bring the threshold in, with AC28 offering an escape: if the measurement is not fixed
within this work, the workflow's escalation policy becomes `none` with that as its
stated reason, so it does not file from a verdict known to be unreliable.

**D7. Consumers are settled before escalation is switched on.**
R15 is ordered before the escalation work rather than after it. Filing issues that
nobody reads is not an improvement on filing none, and turning on escalation for a
workflow whose verdict is meaningless — R2 Health Monitor's threshold, Nightly Registry
Validation's empty runs — would convert a silent problem into a noisy one.

## Known Limitations

- **Whether an assignment made by the Actions bot delivers a notification is the
  load-bearing assumption in D1 and is not settled by documentation.** AC8 exists to
  settle it on a real run before the mechanism is committed to. If it does not deliver,
  D1 must be revisited; the fallback is a human-owned token or a different mechanism, and
  neither is chosen here.
- **The repository's default workflow-token permission setting could not be read
  directly**, returning 403 to the available credentials. The diagnosis that the default
  token lacks `issues: write` rests on observed behaviour instead. AC1 settles it
  directly with an unconditional probe, and AC4 states what happens if the diagnosis is
  wrong.
- **The escalation mechanism may be unable to observe a `startup_failure` conclusion.**
  A workflow rejected before any job is created produces no check runs, and it is not
  established that a `workflow_run`-based escalator sees it. The repository has no
  startup-failure run in the retained window to test against, and issue #667 records this
  exact failure mode occurring in December 2025. AC9 requires the behaviour be
  established and the blind spot recorded with a fallback rather than assumed away.
- **The residue after the macOS bottle repair is not characterised.** The arm64 failures
  do not share a cause with the Intel one, and the manifest 404s and uncharacterised
  decomposition failures are unexplained. R12 and AC25 surface them rather than hide them,
  but the size of that work is unknown at PRD time.
- **Concentrating issue-filing permission in one component** means a defect there silences
  reporting for every workflow at once. R3, R6c, AC10 and AC13 exist to make such a defect
  loud, but the concentration is real and is accepted knowingly.
- **Several of the ten workflows gate their escalation on conditions that cannot be
  reproduced on demand** — a health verdict, a validation outcome, the presence of cleanup
  candidates, a cost threshold. Their escalation paths are therefore established by the
  probe in AC1–AC3 plus file inspection, not by a live demonstration on each workflow.
