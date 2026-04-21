---
design_doc: docs/designs/DESIGN-install-ux-v2.md
input_type: design
decomposition_strategy: horizontal
strategy_rationale: "Refactoring existing code across independent layers with clear prerequisites — each layer can only be completed after its foundation is in place"
confirmed_by_user: false
issue_count: 5
execution_mode: single-pr
---

# Plan Decomposition: DESIGN-install-ux-v2

## Strategy: Horizontal

Each implementation phase from the design maps to one issue. The layers have clear prerequisite ordering: Manager wiring first (issues 1), command layer next (issues 2–3), action files independently (issue 4), tests last (issue 5). Issues 2–4 can be worked in parallel after issue 1 lands.

## Issue Outlines

### Issue 1: refactor(install): add Reporter to Manager and thread through install call chain
- **Type**: standard
- **Complexity**: testable
- **Goal**: Add `SetReporter()`/`getReporter()` to `internal/install/Manager`; replace fmt.Printf calls in manager.go/library.go/bootstrap.go with Status/silence; move reporter creation to `runInstallWithTelemetry`; add reporter as parameter to `installWithDependencies`; call `mgr.SetReporter(reporter)` and `exec.SetReporter(reporter)` in each invocation
- **Section**: Solution Architecture / Implementation Approach Phase 1
- **Milestone**: Install UX v2
- **Dependencies**: None

### Issue 2: refactor(install): reclassify orchestration output to Reporter channels
- **Type**: standard
- **Complexity**: testable
- **Goal**: Replace all printInfof/fmt.Printf calls in `install_deps.go` and `install_lib.go` with `reporter.Log()` for start/done, `reporter.Status()` for intermediate activity, and `reporter.DeferWarn()` for PATH guidance
- **Section**: Implementation Approach Phase 2
- **Milestone**: Install UX v2
- **Dependencies**: Issue 1

### Issue 3: refactor(install): suppress verify sub-steps during post-install check
- **Type**: standard
- **Complexity**: simple
- **Goal**: Change the `RunToolVerification` call in `install_deps.go` from `Verbose: true` to `Verbose: false`
- **Section**: Implementation Approach Phase 3
- **Milestone**: Install UX v2
- **Dependencies**: Issue 1

### Issue 4: refactor(actions): reclassify sub-step Log calls to Status or silence
- **Type**: standard
- **Complexity**: testable
- **Goal**: Convert ~20 `reporter.Log()` calls across extract.go, run_command.go, install_binaries.go, install_libraries.go, link_dependencies.go to `reporter.Status()` or remove them per the Decision 4 classification table
- **Section**: Implementation Approach Phase 4
- **Milestone**: Install UX v2
- **Dependencies**: None

### Issue 5: test(install): verify Reporter output classification and single-spinner invariant
- **Type**: standard
- **Complexity**: testable
- **Goal**: Add property tests covering Manager stdout escape, action sub-step Log/Status/silence classification, and the single-Stop invariant across recursive install calls
- **Section**: Implementation Approach Phase 5
- **Milestone**: Install UX v2
- **Dependencies**: Issues 1, 2, 3, 4
