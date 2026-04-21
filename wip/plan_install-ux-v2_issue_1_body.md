---
complexity: testable
complexity_rationale: New struct field + method on Manager, plus signature change to installWithDependencies — behavior change to how the shared reporter flows through recursive calls requires test coverage
---

## Goal

Add `reporter progress.Reporter` field to `internal/install/Manager` with `SetReporter()` and `getReporter()` methods. Replace all `fmt.Printf` calls in `manager.go`, `library.go`, and `bootstrap.go` with `m.getReporter().Status(...)` or remove them. Move the reporter creation from inside `installWithDependencies` to `runInstallWithTelemetry`, add reporter as a new parameter to `installWithDependencies`, and call both `mgr.SetReporter(reporter)` and `exec.SetReporter(reporter)` in each invocation. Move `defer reporter.Stop()` to `runInstallWithTelemetry`.

## Acceptance Criteria

- `Manager` has a `reporter progress.Reporter` field
- `SetReporter(r progress.Reporter)` stores the reporter on the struct
- `getReporter()` returns `progress.NoopReporter{}` when the field is nil, stored reporter otherwise
- All `fmt.Printf` calls in `manager.go`, `library.go`, `bootstrap.go` are removed or replaced with `m.getReporter().Status(...)` — none remain writing to stdout
- `installWithDependencies` accepts a `reporter progress.Reporter` parameter as its last argument
- `runInstallWithTelemetry` creates the reporter, passes it to `installWithDependencies`, and defers `reporter.Stop()`
- Each invocation of `installWithDependencies` calls `mgr.SetReporter(reporter)` and `exec.SetReporter(reporter)` with the same shared instance
- `go test ./...` passes with no new failures

## Dependencies

None
