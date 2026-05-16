---
status: Proposed
problem: |
  The install pipeline accumulates cross-cutting concerns at a cadence of roughly one new
  orthogonal parameter every 4-6 weeks, and three in-tree workarounds (nine package-level
  CLI flag globals in cmd/tsuku/install.go, a parallel runDryRun function that bypasses
  the install pipeline, and a stranded legacy telemetryClient parameter that PR #2412 could
  not remove) show authors routing around the threading cost rather than paying it.
  installWithDependencies has grown from 7 to 10 positional parameters in three months.
  This design must decide between (a) status quo, (b) Candidate C — a context.Context-
  based attribution refactor with a recursion-collapse cleanup of installWithDependencies,
  or (c) Candidate B — a new installops Service layer above Manager that hides state.json
  semantics behind named operations. The decision turns on the 12-month forecast for new
  cross-cutting concerns, on whether the mgr.GetState().UpdateTool(...) pattern leaking
  state semantics into the CLI layer counts as a smell to heal or a convenience to preserve,
  and on how any restructure composes with the lifecycle event bus shipped in PR #2412.
upstream: docs/designs/current/DESIGN-notices-install-event-bus.md
---

# DESIGN: Install State Abstraction

## Status

Proposed

## Upstream Design Reference

`docs/designs/current/DESIGN-notices-install-event-bus.md` (status: Current).
That design introduced the lifecycle event bus, the `Source` attribution
parameter, and the verb-per-event vocabulary. The implementation experience
of that design is what motivates this one — specifically, Decision 3
(publisher placement chose method-param threading) and Decision 4 (subscriber
wiring in a helper) are now empirical data points to revisit.

## Context and Problem Statement

The motivating issue (#2413) was filed after PR #2412 landed the install
lifecycle event bus. Threading the new `Source` attribution parameter
through the install pipeline drove the PR's file count higher than the
upstream design predicted ("~10 call sites; manageable; one-time"). The
hypothesis in #2413: state-mutating operations on installed tools live as
a loose collection of methods on `install.Manager`, called from a variety
of entry points (CLI commands, the auto-apply subprocess, the autoinstall
path, the self-update path). Each cross-cutting concern that needs to
thread through every operation — attribution tags like the new `Source`
enum, lifecycle event emission, telemetry hooks, future per-operation
policy like dry-run or rate-limiting — ends up touching every entry point
and every wrapper between the CLI and the Manager.

The exploration (`/explore` Round 1, six leads) produced these refinements:

**The diagnosis was partially right but the proposed shape is wrong.**

- PR #2412's 36 files decompose into 16 one-time-cost files (bus,
  subscribers, schema, renderer, docs), 5 net-negative direct-write-removal
  files, and only **11 files / ~120 LOC** representing the actual
  cross-cutting `Source`-threading cost. The headline "file count was too
  high" framing understates how much of PR #2412 was one-time abstraction
  work that will not recur.
- However: `installWithDependencies` grew from **7 to 10 positional
  parameters in three months**. Five install-touching PRs (#2198, #2201,
  #2213, #2280, #2412) each added a new cross-cutting concern at a
  cadence of one every 4-6 weeks. Three in-tree workarounds — package-
  level CLI flag globals, a parallel `runDryRun` implementation that
  bypasses the install pipeline, and a stranded legacy `telemetryClient`
  parameter PR #2412 could not remove — prove the cumulative cost is
  real and producing maintenance debt.
- The literal Java-style repository pattern as proposed in the issue is
  over-engineered for tsuku's actual shape. The Manager surface for
  state-mutating ops is 5 methods. There is no second storage backend to
  abstract over. Go's struct-with-methods on `*Manager` already IS the
  repository pattern in many readings.

**Two viable architectural shapes emerged:**

- **Candidate C — context.Context-based attribution + standalone recursion-collapse refactor.**
  Thread `ctx` through Manager methods (~15 files; same shape as
  Source-as-param), store `Source` in ctx via a typed key, extract it at
  publish callsites. Separately collapse `installWithDependencies`'s
  trailing-arg recursion into a request struct. Picks up SIGINT-aware
  cancellation (which `Manager` does not support today — a Ctrl-C during
  the final `os.Rename` window is currently a silent hazard) as a free
  bonus. Does not address `state.json` exposure via `mgr.GetState()`.
  Aligns with mainstream Go practice for request-scoped metadata. The
  Go-community guidance against `ctx.WithValue` for non-cancellation
  data is well-known but `Source` qualifies as request-scoped (one value
  per CLI invocation, never changes).

- **Candidate B — `internal/install/installops` Service layer above Manager.**
  A new Service owns the lifecycle verbs (Install, Rollback, Remove*)
  and publishes events. Manager keeps low-level state and symlink
  primitives. Request-struct shape (`InstallRequest`, `RollbackRequest`)
  absorbs future cross-cutting concerns as fields. Critically, hides
  `state.json` semantics behind named methods (`MarkExplicit`,
  `RecordDependency`) — addressing the actual structural smell the
  exploration surfaced: the `mgr.GetState().UpdateTool(name, func(ts){...})`
  pattern leaks state semantics into the CLI layer at three sites in
  `cmd/tsuku/install_deps.go`. Incremental migration possible across
  ~5 PRs.

**The status-quo position (Candidate A) remains defensible** if the
design judges that (a) the cumulative cross-cutting cost is overstated
by the workaround evidence, (b) the `mgr.GetState().UpdateTool` pattern
is fine as a CLI-layer convenience rather than a smell, and (c) the
12-month forecast is closer to 1 new concern than 3+.

## Decision Drivers

In rough priority order:

1. **Cross-cutting recurrence is empirically a recurring class of problem in this codebase.**
   One new orthogonal install-pipeline concern every 4-6 weeks across the
   last five PRs. Whatever shape the design chooses must reduce per-concern
   threading cost at least somewhat, or it does nothing useful.

2. **Cancellation is a real missing capability.** Manager doesn't take
   `ctx` anywhere. SIGINT during the final atomic-rename window cannot
   interrupt cleanly. This is independently worth fixing — and once ctx
   is threaded, `Source`-via-ctx becomes essentially free.

3. **The lifecycle event bus (DESIGN-notices-install-event-bus, Current)
   must compose unchanged.** Any restructure must preserve the
   verb-per-event vocabulary, the synchronous-with-recover delivery, the
   publish-after-state invariant, and the subscriber-locality contract.
   The bus is shipped and load-bearing; the design does not reopen it.

4. **Bounded blast radius.** A migration approaching the size of PR #2412
   itself is acceptable only if it pays for itself across multiple future
   concerns. Incremental migration paths are strongly preferred.

5. **Address the actual smells, not the proposed shape.** The exploration
   surfaced two underlying problems: the `installWithDependencies`
   recursive-trailing-arg pattern, and `state.json` exposure via
   `mgr.GetState()`. A restructure that doesn't touch these leaves the
   threading problem half-solved.

6. **No external API contract.** `install.Manager` is in `internal/`.
   The cost of getting the shape wrong is bounded — there is no
   backward-compatibility obligation. This argues for choosing the
   genuinely better shape rather than the minimum-viable one.

7. **Library install (`InstallLibrary`) scope.** The exploration
   confirmed libraries are second-class to the lifecycle events (no bus
   integration). The design must decide whether libraries join any
   restructure (clean, more work) or stay parallel for now (pragmatic,
   leaves a known gap).

## Decisions Already Made

Captured in `wip/explore_install-state-abstraction_decisions.md` during
the exploration:

- **Java-style literal repository pattern eliminated.** Over-engineered
  for ~5 lifecycle operations with no second storage backend. Go's
  struct-with-methods on `*Manager` is already the repository pattern.
- **Command/middleware/decorator-chain patterns eliminated.** Pays off
  with N independent concerns × M operations where the cross-product
  would otherwise be hand-written. Here N=2-3, M=5; indirection cost
  dominates benefit.
- **Aggressive `OperationOptions` struct (Candidate D from exploration)
  deprioritized.** Essentially "Candidate A plus shared struct"; does
  not address the structural `state.json` exposure smell. May appear
  in the final design as a tactical sub-element of B or C but not as
  the top-level shape.

These are constraints the design treats as settled. Reopening them
requires concrete evidence not surfaced during the exploration.

## Considered Options

(To be populated by the /design workflow's decision decomposition. The
exploration already identified the candidate shapes — Candidate A
status-quo, Candidate B installops layer, Candidate C ctx-attribution +
recursion-collapse — plus sub-questions on state.json exposure, library
scope, and migration sequencing.)

## Decision Outcome

(To be populated.)

## Solution Architecture

(To be populated.)

## Implementation Approach

(To be populated.)

## Security Considerations

(To be populated.)

## Consequences

(To be populated.)
