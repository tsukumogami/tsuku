# Design Summary: install-ux-v2

## Input Context (Phase 0)
**Source:** Freeform topic + live failure evidence
**Problem:** `tsuku install` produces 40+ sequential lines instead of a single updating status line. PR #2280 wired `Reporter` only through the executor/action layer (`internal/executor/`, `internal/actions/`). The command-entry layer (`cmd/tsuku/install_deps.go`), install orchestration (`internal/install/manager.go`, `library.go`), and verify output (`cmd/tsuku/verify.go`) still use raw `fmt.Printf`, so the spinner has nowhere to live for ~95% of execution time.

**Goal:** One status line updating in place throughout the entire `tsuku install`/`tsuku update` execution. Permanent lines only for final notices (completion, PATH guidance) and errors.

**Constraints:**
- Must build on existing `progress.Reporter` interface (TTYReporter, NoopReporter) from PR #2280
- Must preserve clean sequential plain-text output in non-TTY/CI mode
- Recursive dependency installs must share the same reporter instance
- 53 fmt.Printf calls remain across: cmd/tsuku/install_deps.go (10), internal/install/manager.go (9), internal/install/library.go (2), cmd/tsuku/verify.go (15), cmd/tsuku/verify_deps.go (11), cmd/tsuku/install_lib.go (2), internal/install/bootstrap.go (1)
- Action-level verbosity: current reporter.Log() calls in actions are too many — most should become Status()

## Security Review (Phase 5)
**Outcome:** Option 3 — N/A with justification
**Summary:** Pure output routing refactoring; no new artifact handling, permissions, dependencies, or data exposure. All existing security mechanisms unchanged.

## Current Status
**Phase:** 6 - Final Review
**Last Updated:** 2026-04-21
