# Friction Log: Multi-PR implement-doc for CI Job Consolidation

This log captures friction points, workarounds, and observations while implementing
the CI Job Consolidation design doc using a multi-PR approach (one PR per issue)
instead of the standard single-PR `/implement-doc` workflow.

## Context

- Design doc: `docs/designs/DESIGN-ci-job-consolidation.md`
- Tracking branch: `docs/ci-job-consolidation` (PR #1887)
- 7 issues (#1891-#1897) across 3 phases
- Each issue gets its own branch off `main`, its own PR, merged independently
- Tracking branch holds state file and design doc status updates

## Log

### Phase 0: Setup

**Observation: workflow-tool init works fine for multi-PR**
The `--branch` and `--pr` flags let us point state at the existing tracking branch/PR.
The state file doesn't encode any assumption about single vs multi-PR -- it tracks
issue status, not PR topology. This is a good separation.

**Observation: Standard workflow assumes single PR**
The Phase 0 instructions say to create a draft PR and update its body with issue
checklists and test plan. For multi-PR, the tracking PR (#1887) is a design doc PR,
not an implementation PR. We'll skip the PR body template and instead update the
design doc's Mermaid diagram directly as the source of truth.

**Observation: QA/techwriter agents may not apply well**
CI workflow changes are hard to test locally -- they're validated by the CI system
itself. The QA test plan is likely to be "run CI and check results" for every issue.
The doc plan may have nothing to do since there's no user-facing documentation change.
Spawning these agents anyway to see what they produce.

**Result: Agents produced reasonable output**
- Techwriter correctly returned 0 doc entries ("all CI/build infrastructure changes")
- Tester produced 14 scenarios. Split between structural grep checks (infrastructure)
  and "push PR and observe CI" scenarios (use-case). The use-case scenarios are all
  marked `Environment: manual (CI)` which is accurate -- you can't validate workflow
  changes without actually running them in GHA.
- The structural checks (scenarios 1,2,4,6,8,10,13) can be run locally after implementation.
  The CI observation scenarios (3,5,7,9,11,12,14) are validated by watching CI on each PR.

**Friction: testable_scenarios population feels like busywork**
The workflow wants me to parse the test plan, extract which scenarios apply to each
issue, and update the state file per-issue. For CI workflow changes, this mapping is
obvious (scenarios reference their issue numbers directly). Doing this via jq commands
feels ceremonial. Writing it anyway for compliance.
