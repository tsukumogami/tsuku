<!-- decision:start id="bus-delivery-semantics" status="assumed" -->
### Decision: Bus delivery semantics

**Context**

tsuku is introducing an in-process event bus. Every install-state mutation publishes an event, and `internal/notices` subscribes to reconcile notice files on disk. v1 has exactly one required subscriber (notices), but the bus is being designed to accept more later.

Each `tsuku` invocation is a fresh, short-lived process: foreground commands (`tsuku install`, `tsuku update`) and the auto-apply background subprocess each have their own subscriber set within their own process. There is no long-running daemon, no general-purpose runtime loop, and no shared in-memory queue between processes.

The relevant constraints pull in three directions:
- **Best-effort.** A failing subscriber (returned error or panic) must not break the install path or crash the host process.
- **Synchronous-feeling exit.** By the time the foreground command returns to the shell, the notices directory must reflect what happened. Users (and tests) treat the CLI as if it ran to completion.
- **Test determinism.** Subscriber side effects must be observable synchronously in unit tests without sleeps, channels, or polling.

These constraints jointly rule out true fire-and-forget delivery and rule out delivery models that let subscriber failures escape.

**Assumptions**

- Subscriber work is bounded and short: writing a small JSON notice file, occasional logging. No subscriber will block on the network or on user input.
- The bus is not used for hot-loop events. Publication happens at install-state transition boundaries, on the order of single-digit events per CLI invocation.
- Go's `recover()` inside a per-subscriber wrapper is sufficient containment; we are not trying to survive memory corruption or runtime-level faults.
- Subscriber registration is static within a process (wired up at startup). Dynamic add/remove during event dispatch is not a v1 requirement.
- A `log/slog` logger (or equivalent) is available to the bus for recording subscriber errors and recovered panics.

**Chosen: Synchronous-with-recover, deterministic ordering, re-entrancy queued-and-flushed, errors swallowed-and-logged, named subscribers**

The bus delivers events synchronously: `Publish(evt)` returns only after every subscriber has been invoked. Each subscriber call is wrapped so that:

- A returned `error` from the subscriber is logged (with subscriber name, event type, and error) but not propagated to the publisher. `Publish` itself returns no error.
- A panic inside a subscriber is caught with `recover()`, logged (with subscriber name, event type, recovered value, and a stack trace), and treated like a returned error. The next subscriber still runs. The publisher is unaffected.

Subscribers are invoked in **registration order**. There is no priority field; ordering is whatever order the wiring code calls `bus.Subscribe(...)`. This is deterministic, easy to reason about, and matches how Go slices work.

Each subscriber has a stable string **name** supplied at registration (e.g. `"notices"`). The name appears in every error and panic log line so failures can be attributed without reading stack traces.

**Re-entrancy** (a subscriber calling `Publish` from inside its handler) is handled by **queue-and-flush**: the bus detects an in-progress dispatch on the current goroutine, appends the new event to a per-dispatch queue, and drains the queue after the current event's subscribers all return. This keeps the call graph flat (no unbounded stack growth), preserves the invariant that each event is fully delivered before the next, and avoids the "lost event" trap of disallowing re-entrancy outright.

There is **no goroutine, no channel, no flush step, and no shutdown handshake** in the bus itself. Publish blocks until done; when the process exits, everything the bus needed to do has already happened.

**Rationale**

The constraints are nearly self-deciding once you write them down:

1. *Synchronous-feeling exit* + *no runtime loop* eliminates async-with-flush. Adding a goroutine plus a `Flush()` call at every CLI exit point (including panic paths, `os.Exit`, signal handlers, and the auto-apply subprocess) is real surface area for marginal benefit. Subscriber work is cheap; we don't need parallelism.
2. *Test determinism* eliminates pure asynchronous delivery. A test that does `bus.Publish(evt); assertNoticeExists(t, ...)` must pass without timing tricks. Synchronous delivery makes this free.
3. *Best-effort* + *must not crash the host* eliminates pure synchronous delivery (no recover). One panic in any subscriber would tank the install. `defer recover()` per subscriber is idiomatic Go and the lowest-cost mitigation.

Within the synchronous-with-recover family, the sub-choices fall out the same way:

- **Registration order, not priority.** v1 has one subscriber. Adding a priority field now is YAGNI and invites accidental coupling between subscribers. Registration order is deterministic and visible at the call site.
- **Queue-and-flush re-entrancy.** Allowing direct re-entrant calls risks stack growth and surprising interleavings (subscriber B sees event 2 before event 1 finishes). Disallowing re-entrancy (panic on nested publish) is harsh and forces awkward workarounds in callers. Queue-and-flush gives the natural semantic: events are delivered in publish order, fully, one at a time.
- **Swallow-and-log errors.** The bus is best-effort by constraint. Surfacing errors to the publisher would force every `Publish` call site to handle an error it can do nothing useful about. Silent swallow would make failures invisible. Log-and-continue is the standard Go pattern for fire-and-forget side effects (e.g., metrics, audit logs).
- **Named subscribers.** Without names, an error log says "subscriber #2 failed" and the on-call has to grep wiring code. A name is one extra string per subscriber and pays for itself the first time something goes wrong in production.

This design is also boring in the good sense: it is roughly forty lines of Go (a slice of subscribers, a `for` loop, a `defer/recover`, a re-entrancy queue keyed by goroutine state). There is no concurrency to reason about, nothing to mock in tests, and no lifecycle for the caller to manage.

**Alternatives Considered**

- **Pure synchronous (no recover).** Simplest implementation: just call each subscriber in a loop and propagate errors. Rejected because a single panicking subscriber crashes the install. The best-effort constraint is non-negotiable, and the cost of `defer recover()` per call is negligible.

- **Asynchronous fire-and-forget on a goroutine.** Each `Publish` spawns a goroutine (or sends to a buffered channel drained by a worker). Rejected because:
  - The "synchronous-feeling exit" constraint forces a `Flush()`/`Close()` step at every exit path. CLIs exit in many ways (normal return, `os.Exit`, signal, panic), and getting flush right at all of them is error-prone for no real win.
  - Tests would need to wait for delivery, which means either exposing flush in tests or using polling. Both are friction.
  - Subscribers writing to the same notice file would race unless we add ordering guarantees back in.
  - The performance gain is imaginary: subscriber work is a small JSON write at install-state transitions.

- **Async-with-flush (background worker plus explicit `Flush()` before exit).** Rejected for the same flush-surface-area reason, plus it adds a goroutine and a channel to a codebase that otherwise has no runtime loop. The complexity is not justified by any observed bottleneck.

- **Synchronous with errors propagated to publisher.** Rejected because it inverts the constraint: a notice-write failure should never fail an install. Every `Publish` call site would need to either ignore the error (defeating the design) or handle it (with no useful action available). Logging at the bus is the right layer.

- **Priority-ordered subscribers.** Rejected as premature. With one subscriber, priority is meaningless. With two or three future subscribers, registration order is clear enough and the wiring code is the single place to look. Adding priority later, if genuinely needed, is a non-breaking change to the registration API.

- **Disallow re-entrancy (panic on nested `Publish`).** Rejected because re-entrancy will happen accidentally (e.g., a subscriber updates state that triggers another event), and the failure mode (panic in best-effort code path) is worse than the alternative (queue-and-flush, which Just Works).

**Consequences**

What becomes easier:
- Callers of `bus.Publish(evt)` write one line and move on. No error checking, no flush, no shutdown.
- Tests do `bus.Publish(evt)` and immediately assert on the file system or subscriber state. No timing, no sync primitives.
- Adding a subscriber later is one line in the wiring code. Failure mode is contained to that subscriber.
- Reasoning about ordering is trivial: events are processed in publish order, subscribers in registration order.

What becomes harder:
- A genuinely slow subscriber will block the install path. We are explicitly betting that no subscriber needs to be slow. If that bet breaks (e.g., someone adds a network-calling subscriber), the bus design needs to change, not just the subscriber.
- Subscribers cannot meaningfully reject events. If notices fails to write, the install still succeeds and the user sees no error from the publisher. The error is in the log; that is the right trade-off but it means the log matters.
- Re-entrant publication is supported but not free-form. A subscriber that publishes events synchronously expecting to see other subscribers' side effects immediately will be surprised; nested events are delivered after the current event finishes.

What stays the same:
- No new dependencies, no new goroutines, no new lifecycle. The bus is a value held by `cmd/tsuku` wiring and passed where needed.
<!-- decision:end -->
