# Plan Analysis: DESIGN-install-ux-v2

## Source Document
Path: docs/designs/DESIGN-install-ux-v2.md
Status: Accepted
Input Type: design

## Scope Summary
Wire the existing `progress.Reporter` interface through all layers of `tsuku install`/`tsuku update` so a single spinner owns the terminal for the full install, including recursive dependency installs. Non-TTY output is reduced to 4–6 permanent lines per tool.

## Components Identified
- `internal/install/Manager` — needs `reporter` field, `SetReporter()`/`getReporter()`, and replacement of fmt.Printf calls in manager.go, library.go, bootstrap.go
- `installWithDependencies` + `runInstallWithTelemetry` — reporter creation moved to top level; reporter threaded as new parameter so all recursive calls share one instance
- `cmd/tsuku/install_deps.go` + `install_lib.go` — printInfof/fmt.Printf calls reclassified to Log/Status/DeferWarn per Decision 2
- Verify sub-step suppression — one-line `Verbose: false` change at RunToolVerification call site
- Action verbosity reduction — ~20 `reporter.Log()` calls across 5 action files converted to Status() or silence per Decision 4
- Tests — property tests for Manager stdout escape, action sub-step classification, and single-Stop invariant

## Implementation Phases (from design)
1. SetReporter on Manager + reporter parameter threading
2. Install start/done Log lines (install_deps.go, install_lib.go)
3. Verify sub-step suppression (Verbose:false)
4. Action verbosity reduction (5 action files)
5. Tests and validation

## Success Metrics
- TTY: single spinner line for entire install; 2+ permanent lines only at end
- Non-TTY: 4–6 permanent lines for a binary recipe with one dep
- Zero bytes escape to os.Stdout/os.Stderr outside Reporter during install
- reporter.Stop() called exactly once per install invocation

## External Dependencies
- `progress.Reporter` interface from PR #2280 — unchanged; implementation builds on it
- Existing `testReporter` in `internal/executor/install_output_test.go` — extended with Stop tracking
