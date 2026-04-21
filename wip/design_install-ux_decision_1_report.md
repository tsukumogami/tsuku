<!-- decision:start id="reporter-interface-contract" status="assumed" -->
### Decision: Reporter Interface Contract

**Context**

tsuku's executor and 43 action implementations currently write progress output via
raw `fmt.Printf` / `fmt.Println` calls — 396 occurrences across the actions package
alone. Replacing this with a structured progress/UX layer requires choosing how to
define and inject the replacement abstraction into `ExecutionContext`.

The prior decision established that the Reporter will be a field on
`actions.ExecutionContext`. The remaining question is whether callers depend on a Go
interface (any implementation) or a concrete struct (one canonical implementation).

The codebase already has two relevant reference points: `internal/progress/spinner.go`
(a TTY-aware goroutine spinner with Start/Stop API), and niwa's `Reporter` struct
(a goroutine-based in-place redraw with Status/Log/Warn/Defer/FlushDeferred API).
Neither is a direct fit — niwa's struct is the right behavioral model, but tsuku
needs its own definition.

**Assumptions**

- Assumed: the Reporter interface will be defined in `internal/progress` or a new
  `internal/reporter` package (not in `internal/actions` to avoid circular imports).
  If wrong: the interface can live in `internal/actions` directly with the concrete
  implementation in a sub-package; the decision remains valid.
- Assumed: the 43 action files will migrate from `fmt.Printf` to `ctx.Reporter.Log`
  in a single pass. If migration is incremental, a no-op default on `ExecutionContext`
  ensures backward compatibility during the transition.
- Running in --auto mode without user confirmation.

**Chosen: Minimal Go Interface**

Define a `Reporter` interface in tsuku's codebase:

```go
// Reporter handles progress display and log output during tool installation.
// The concrete implementation is TTY-aware; Status is a no-op on non-TTY output.
type Reporter interface {
    // Status sets a transient status message. On TTY output a background goroutine
    // redraws the line with an advancing spinner. On non-TTY output this is a no-op.
    Status(msg string)

    // Log writes a permanent log line. On TTY output the spinner is stopped and
    // its line cleared first. Format follows fmt.Sprintf conventions.
    Log(format string, args ...any)

    // Warn writes a permanent warning line (prepends "warning: ").
    Warn(format string, args ...any)

    // DeferWarn queues a warning for FlushDeferred.
    DeferWarn(format string, args ...any)

    // FlushDeferred prints all deferred messages in order and clears the queue.
    FlushDeferred()
}
```

A concrete `TTYReporter` struct implements this interface with:
- TTY detection at construction via `term.IsTerminal` (or `progress.ShouldShowProgress`)
- Background goroutine started by `Status`, advancing spinner at 100 ms ticks using
  `\r\033[K` in-place redraws (matching niwa's pattern)
- `Log`/`Warn` stop the goroutine and clear the line before printing
- `Status` is structurally a no-op when `isTTY == false`

A `NoopReporter` (or `DiscardReporter`) implements the interface with all methods as
no-ops, used as the zero-value default on `ExecutionContext` to avoid nil panics during
the migration from `fmt.Printf`.

Injection point: `ExecutionContext.Reporter Reporter` (interface type). The executor
constructs `NewTTYReporter(os.Stderr)` and assigns it before calling `ExecutePlan`.

**Rationale**

With 43 action files being migrated, testability is the decisive factor. An interface
lets each action test inject a capturing test double (`&captureReporter{}`) without
any TTY setup or real goroutine management. The concrete struct approach can be tested
via a non-TTY writer, but it can't distinguish which method was called (Status vs Log)
without adding a wrapper — which is effectively what a test double is. Making the
interface explicit is idiomatic Go and costs nothing: callers always see the interface,
and there is one production implementation. External contributors encounter the interface
shape first, which is cleaner than exposing goroutine lifecycle internals.

The existing `progress.Spinner` implements the goroutine lifecycle but uses a
Start/Stop API rather than the Status/Log shape required here. The concrete Reporter
implementation can delegate to Spinner internally or replicate the pattern; the
interface is unaffected either way.

**Alternatives Considered**

- **Concrete struct (embed niwa's pattern)**: Define `type Reporter struct` directly
  in tsuku with no interface. Rejected because it couples all 43 action files to the
  concrete type; tests must use a non-TTY writer and can't assert which method was
  called without additional wrapping. Adds no benefit over the interface approach
  since there's only one production implementation.

- **io.Writer adapter**: Thin wrapper around `io.Writer` where Status writes `\r+msg`
  to the writer. Rejected because it cannot satisfy the goroutine spinner lifecycle
  constraint — a plain `io.Writer` has no concept of a background ticker, and
  in-place redraws during a long `git clone` (where no new Status calls arrive for
  seconds) require the goroutine to advance the spinner frame independently of caller
  activity. The constraint "must support goroutine spinner lifecycle" is structural,
  not optional.

**Consequences**

- `ExecutionContext` gains a `Reporter Reporter` field (interface type).
- A new `TTYReporter` concrete type is added (in `internal/progress` or `internal/reporter`).
- A `NoopReporter` is provided as a safe default for tests and migration.
- 43 action files migrate `fmt.Printf` → `ctx.Reporter.Log`; `fmt.Println` step
  headers in the executor → `ctx.Reporter.Status` / `ctx.Reporter.Log`.
- Test doubles become a one-file addition; no real TTY or goroutine management in tests.
- The existing `progress.Spinner` is not removed — it may be used internally by
  `TTYReporter` or kept for other callsites.
<!-- decision:end -->
