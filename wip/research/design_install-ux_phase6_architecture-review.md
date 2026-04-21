# Architecture Review: DESIGN-install-ux.md

Scope: Solution Architecture and Implementation Approach sections only.
Grounded in: executor.go, action.go, spinner.go, progress.go, download_file.go, install_deps.go.

---

## 1. Is the architecture clear enough to implement?

Overall yes, but two missing details will cause confusion at implementation time.

**Missing: `ExecutionContext` is in `internal/actions`, not `internal/executor`**

The design places the `Reporter` field addition in `internal/executor/executor.go`, but
`ExecutionContext` is actually defined in `internal/actions/action.go`. Every action calls
`action.Execute(ctx *ExecutionContext, ...)` using the `actions.ExecutionContext` type, not
any type from the executor package. Adding `Reporter progress.Reporter` to the struct that
exists in `internal/actions/action.go` creates a cross-package import: `internal/actions`
would import `internal/progress`. That direction is clean (progress has no tsuku-domain
dependencies), but the design's stated injection point ("in `internal/executor/executor.go`")
is simply wrong and will send an implementor to the wrong file.

**Missing: goroutine lifecycle ownership on `TTYReporter`**

The security section (goroutine lifecycle) mentions that `TTYReporter` must expose a `Stop()`
method and that install orchestration must `defer reporter.Stop()`. However `Stop()` is not
listed in the `Reporter` interface, and none of the phase diagrams show where it gets called.
If `Stop()` is not on the interface then callers that hold the interface type cannot call it
without a type assertion. Two options: add `Stop()` to the interface (changing
`NoopReporter` too), or require callers to hold a concrete `*TTYReporter` for lifecycle.
The design does not commit to either and leaves the caller code unspecified. This must be
resolved before Phase 1 lands.

**Component diagram is accurate otherwise.** `internal/progress/` as the home for
`Reporter`, `TTYReporter`, `NoopReporter`, and `ProgressWriter` is the right call. The
package already owns `Spinner` and `Writer`, and the new types are in the same domain.
No new packages are required.

---

## 2. Are the implementation phases correctly sequenced?

Phases 1–4 are well sequenced. Each phase's deliverables are prerequisites for the next:
Reporter types (1) → wiring into context (2) → action descriptions (3) → executor
migration (4). This is the right order.

**Phase 5 has an implicit Phase 4 dependency that is not stated**

Phase 5 replaces `progress.NewWriter` in `download_file.go` and `download.go` with
`ProgressWriter + reporter.Status`. This requires that the action `Execute()` methods
can reach a `Reporter` via `ctx.Reporter`. That only works after Phase 2 wires
`ExecutionContext.Reporter`. The design says Phase 5 delivers `ProgressWriter` but
the actual callsite change in the download actions is blocked until the field is present.
Phase 2 is the actual blocker — this is fine since Phase 2 precedes Phase 5 — but
the dependency is not stated and a reader might attempt Phase 5 changes before Phase 2
is merged.

There is also a subtle ordering issue within Phase 5: `progress.go` (`progress.Writer`)
must not be deleted until all callers are migrated. `download_file.go` uses
`progress.NewWriter` (confirmed in source). The design says "Delete
`internal/progress/progress.go` once all callsites are migrated" but `ShouldShowProgress()`
in `progress.go` is also called by `download_file.go` directly (line 252). Any deletion
must audit all exported symbols from `progress.go`, not just `NewWriter`.

**Phase 6 migration volume is underestimated**

Phase 6 is described as "mechanical" but 384 `fmt.Printf` occurrences across 43 files is
a substantial surface. The design does not address: (a) how to handle the `fmt.Printf`
calls inside functions that do not receive `ExecutionContext` (e.g., `downloadFileHTTP` is
a package-level function with no ctx parameter), (b) whether `downloadFileHTTP` should be
refactored to accept a callback or a `Reporter`, and (c) retry messages
(`"Retry %d/%d after %v...\n"` in download_file.go line 164) that are currently printed
from context-free helper functions. These cannot be migrated by simply swapping the
`fmt.Printf` call — they need a structural refactor to receive a reporter. Phase 6 should
call this out explicitly.

---

## 3. Are there simpler alternatives we overlooked?

**The existing `Spinner` in `internal/progress/spinner.go` is 90% of what `TTYReporter` needs**

`Spinner` already has TTY detection (`ShouldShowProgress()`), a 100ms goroutine ticker,
in-place redraws with `\r`, and `StopWithMessage()`. The main gap between `Spinner` and
`TTYReporter` as defined is:

1. `Spinner.Start()` takes the initial message; `TTYReporter.Status()` updates the message at any time.
2. `Spinner` has no `Log()`, `Warn()`, `DeferWarn()`, `FlushDeferred()`.
3. `Spinner` uses `|/-\` frames; niwa uses braille frames.

A lighter implementation path: extend `Spinner` to support `SetMessage()` (already present)
and add a thin `TTYReporter` wrapper that composes a `Spinner` plus a deferred-warning
queue. This avoids rewriting the goroutine lifecycle that `Spinner` already handles
correctly. The design rejects "concrete struct" but that was about avoiding coupling in
action files; composing on top of `Spinner` internally is fine since `TTYReporter` still
exposes the `Reporter` interface.

This is not a blocking concern — writing `TTYReporter` from scratch is also workable and
might be cleaner — but the existing `Spinner` should be evaluated rather than ignored.

**`ProgressWriter` as a separate type vs inlining into the download actions**

The `ProgressWriter` struct proposed is a generic io.Writer with a callback. This is clean
and testable. No simpler alternative is obviously better here.

---

## 4. Are there hidden coupling issues?

**Cross-package import direction is safe but unverified**

The proposed `internal/actions` → `internal/progress` import is new. `internal/progress`
currently imports only `golang.org/x/term` (in progress.go) and stdlib. Adding `Reporter`
to `internal/progress` keeps this clean. However, `internal/actions/action.go` adding
`Reporter progress.Reporter` to `ExecutionContext` creates a new mandatory import from
`actions` to `progress`. This is benign in the dependency graph, but all 43 action files
that currently only import `actions` will now transitively pull in `progress`. This is
fine — confirm no circular path via `go mod graph` once Phase 2 lands.

**`ActionDescriber` interface location is ambiguous**

The design says "Add the `ActionDescriber` optional interface in `internal/actions/`" but
doesn't specify which file. `Preflight`, `Decomposable`, and `NetworkValidator` are all
defined in `internal/actions/` (preflight.go, decomposable.go, action.go respectively).
`ActionDescriber` should follow the same pattern — a dedicated file
(`internal/actions/describer.go`) matches how `Preflight` and `Decomposable` are organized.
Without this guidance, implementors may scatter the definition.

**`download_file.go` has a context-free download helper — two reporters needed**

`doDownloadFileHTTP` → `io.Copy` path is where `progress.NewWriter` currently lives.
That helper is package-level with no `ExecutionContext` parameter. The `ProgressWriter`
callback approach requires the callback to close over a `Reporter`. This works if
`DownloadFileAction.Execute()` creates the `ProgressWriter` with a closure over
`ctx.Reporter`. But the retry loop in `downloadFileHTTP` calls `doDownloadFileHTTP`
multiple times, and the progress callback must reset `written` state between retries
otherwise percentage calculations will be wrong on retry. The design does not address
`ProgressWriter` reset semantics for retries. If `ProgressWriter.written` is not reset
between retries, the displayed percentage will be inflated on the second attempt.

**`installSingleDependency` constructs its own `ExecutionContext` without a `Reporter` field**

`executor.go` line 736 constructs a second `ExecutionContext` for dependency installation
with all fields wired manually. Phase 2 adds `Reporter` to `ExecutionContext`, but the
design only mentions wiring the reporter in the primary `execCtx` (line 411 block). The
dependency `execCtx` (line 736 block) must also receive the reporter — otherwise dependency
installation steps get no output, which contradicts the goal. This second construction site
is not mentioned in the design and will be missed if the implementor follows the
described injection point only.

---

## 5. Is the 6-phase approach realistic?

**Phases 1–3 are realistic and independently mergeable.** No behavioral change in 1–2;
Phase 3 adds StatusMessage to 10 actions and the executor type-assertion — straightforward.

**Phase 4 is the first phase with visible behavior change.** Swapping `fmt.Printf` in
`executor.go` and `install_deps.go` for reporter calls is well-scoped. The phase correctly
defers all action-file migration to Phase 6. No blocking issues.

**Phase 5 is realistic but the `ProgressWriter` reset issue (retry semantics) must be
resolved before merging.** The current `download_file.go` simply skips progress on retries;
the new implementation must preserve this or explicitly handle it.

**Phase 6 is the phase most likely to be blocked.**

Three categories of call sites require structural changes, not just substitution:

1. `downloadFileHTTP` / `doDownloadFileHTTP` — context-free functions that emit retry
   messages. These need a `Reporter` or callback parameter threaded through.
2. Actions that call sub-actions directly (composites) — the sub-action receives the
   context, but parent print statements in the composite's own body need ctx.Reporter.
3. Functions called from `installSingleDependency` in executor — they log to fmt.Printf
   and have their own ctx variables; the secondary ctx must carry the reporter.

Phase 6 is not blocked at the phase level, but the "mechanical substitution" framing
understates the effort. A realistic estimate is that 20–30% of occurrences require a
structural change rather than a simple `ctx.Reporter.Log(...)` substitution.

---

## Summary of Findings

1. **Wrong file named for `ExecutionContext`**: the design says to add `Reporter` to
   `internal/executor/executor.go`, but `ExecutionContext` lives in
   `internal/actions/action.go`. Implementors will look in the wrong place.

2. **`Stop()` lifecycle gap**: `Stop()` is needed to prevent goroutine leaks but is not
   included in the `Reporter` interface definition. The design must commit to either adding
   it to the interface or documenting that callers must hold a `*TTYReporter` for cleanup.

3. **Second `ExecutionContext` construction in `installSingleDependency` not addressed**:
   dependency installs construct their own context at executor.go ~line 736; this site must
   also receive the reporter, but it's not mentioned. Silent dependency installation would
   be a regression.

4. **`ProgressWriter` state reset on retries**: retry logic in `downloadFileHTTP` calls
   `doDownloadFileHTTP` multiple times; if `ProgressWriter.written` is not reset between
   retries, the displayed percentage will overcorrect on subsequent attempts.

5. **Phase 6 "mechanical" framing understates scope**: ~20–30% of the 384 fmt.Printf
   calls are in context-free helper functions that need parameter or closure changes, not
   just identifier substitution. Phase 6 should be treated as a migration sprint, not
   a find-and-replace pass.

---

## Verdict

**Conditional pass.** The core architecture is sound — Reporter interface, optional
ActionDescriber, ProgressWriter, and NoopReporter default are all correct design choices
with good precedent in the codebase. The phased approach is well ordered. However, three
issues (#1, #2, #3) must be resolved before implementation begins to avoid silent regressions
and implementation confusion. Issues #4 and #5 should be addressed in the design before
Phase 5/6 work starts, not discovered during implementation.
