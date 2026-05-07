<!-- decision:start id="version-fallback-signal-routing" status="assumed" -->
### Decision: Version Fallback Signal Routing

**Context**

`GitHubArchiveAction.Decompose` returns `([]Step, error)` and cannot add a return value. When it
falls back from version X to X-1 because `FetchReleaseAssets` finds no matching asset, the
install succeeds — so returning a non-nil error is wrong. Yet the caller needs to know about
the fallback to write a `KindVersionFallback` notice to `$TSUKU_HOME/notices/`.

Three mechanisms were considered: threading `PlanConfig.OnWarning` into `EvalContext` as a
narrow string callback, adding a mutable output field to `EvalContext`, or adding a `Reporter`
field to `EvalContext` (mirroring `ExecutionContext.Reporter`).

The exploration phase already committed to `InboxReporter` as the routing abstraction: a
`progress.Reporter` implementation whose `Warn()` method calls `notices.WriteNotice()`. The
routing decision lives at construction time — callers that want notice routing supply an
`InboxReporter`; others supply the zero-value `NoopReporter`.

**Assumptions**

- The InboxReporter design (routing Warn/DeferWarn to notices.WriteNotice) is the chosen
  implementation strategy, as established in the exploration phase.
- EvalContext is not concurrently accessed during a single GeneratePlan call (steps are resolved
  sequentially in the current code). If this changes, Reporter implementations already use a
  mutex.

**Chosen: Reporter.Warn in Decompose (Reporter field on EvalContext)**

Add a `Reporter progress.Reporter` field to `EvalContext` and a `GetReporter()` helper (returning
`NoopReporter{}` when nil). When `GitHubArchiveAction.Decompose` detects a version fallback, it
calls `ctx.GetReporter().Warn("installed %s instead of %s: ...", fallback, requested)`.

Callers that want notice routing construct `GeneratePlan` with an `InboxReporter` wired to the
tool's notice directory. Callers that don't (eval.go, validate pipelines) supply nothing;
`NoopReporter{}` discards the call silently. No polling after `Decompose`; the notice is written
at the moment the fallback is detected. Multiple fallbacks within a single plan each fire
`Warn()` and are individually captured.

**Rationale**

1. **Mirrors ExecutionContext.Reporter.** `ExecutionContext` already has a `Reporter` field and a
   `GetReporter()` helper. Applying the same pattern to `EvalContext` is idiomatic and consistent;
   contributors familiar with `Execute()` methods immediately understand how `Decompose()` methods
   surface warnings.

2. **Fulfills the InboxReporter commitment directly.** The exploration concluded that InboxReporter
   is the right abstraction and that routing should be decided at construction time. Adding Reporter
   to EvalContext is the minimal structural change that honors that conclusion. The alternative
   options (OnWarning callback, output field) require additional wiring that partially duplicates
   Reporter's job.

3. **No call-site changes in the install engine.** `actions/`, `executor/`, and `install/` remain
   unchanged except for the single new field on `EvalContext`. Callers that generate plans
   (`helpers.go`, `install_lib.go`) pass in whatever reporter they already have; they don't need
   to add closures or poll fields.

4. **Handles multiple fallbacks.** If a plan has multiple `github_archive` steps that each fall
   back, every fallback fires `Warn()` independently. An output field would overwrite on each call;
   a single callback would need to accumulate externally.

5. **Concurrent-safe at no extra cost.** `ttyReporter` and `InboxReporter` will use a mutex
   internally (following the established pattern). An EvalContext output field would require
   explicit locking at the callsite.

**Alternatives Considered**

- **PlanConfig.OnWarning callback**: The callback is `func(action, message string)` and lives in
  `PlanConfig`, not `EvalContext`. To reach `Decompose`, it must be threaded into `EvalContext`
  anyway — so the structural change is equivalent to adding a Reporter field, but the result is a
  weaker, string-only API. All install-path callers that currently omit `OnWarning` would need to
  add a closure for notice routing. Rejected: equivalent structural cost, weaker semantics, misaligns
  with the InboxReporter decision.

- **EvalContext output field (FallbackInfo struct)**: Mutating a context object as an output
  channel is unusual in Go — context objects are conventionally input-only. The field would be
  overwritten if multiple steps fall back (last-write wins). `resolveStep` would need to check the
  field after every `Decompose` call. Rejected: unconventional, fragile for multiple fallbacks, and
  requires per-call polling that is easy to miss when adding new callers.

**Consequences**

- `EvalContext` gains one field: `Reporter progress.Reporter`. The zero value (`nil`) is handled by
  `GetReporter()` returning `NoopReporter{}`, so all existing callsites continue to compile and
  behave identically without modification.
- `GitHubArchiveAction.Decompose` (and `GitHubFileAction.Decompose` if it also supports fallback)
  call `ctx.GetReporter().Warn(...)` on fallback — a one-line addition at the fallback site.
- `GeneratePlan` in plan_generator.go constructs `EvalContext` with a Reporter when one is
  available. The reporter comes from the same source as the reporter already passed to the executor
  for `Execute()` calls.
- `InboxReporter` implements `progress.Reporter`. Its `Warn()` method writes a `KindVersionFallback`
  notice. It is constructed in `helpers.go` (the install path that calls `GeneratePlan`) and passed
  through. Callers that do not care about routing (eval.go, validate pipelines) are unaffected.
- The `EvalContext` and `ExecutionContext` Reporter fields become symmetric. Future Decompose methods
  that need to surface non-fatal information have a clear, established path.
<!-- decision:end -->
