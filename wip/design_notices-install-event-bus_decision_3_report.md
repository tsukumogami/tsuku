# Decision 3: Emitter Location for the Install Event Bus

**Status:** decided
**Confidence:** high
**Mode:** --auto (Tier 3 fast path; phases 0, 1, 2, 6)
**Date:** 2026-05-16

## Question

Where does `bus.Publish(event)` get called?

1. **State.json shim** — wrap `StateManager.UpdateTool` / `RemoveTool` / `UpdateToolWithoutLock` so every state mutation auto-publishes.
2. **Explicit Publish at each site** — every operation calls `bus.Publish(...)` after its own state mutation.
3. **Defer-based instrumentation** — `defer bus.Publish(buildEvent(err))` at function entry.

## Chosen

**Hybrid: Explicit Publish at lifecycle boundaries (option 2), backed by an audit-time linter rule.**

Use option 2 as the default mechanism — every install/update/remove/rollback/activate site explicitly calls `bus.Publish(...)` after the state mutation it is reporting. Do **not** wrap the state manager (option 1), and do **not** rely on `defer` (option 3) for top-level lifecycle events.

The "easy to forget" risk of option 2 is mitigated outside the bus itself: introduce a small list of "blessed publishers" (`Manager.InstallWithOptions`, `Manager.Activate`, `Manager.RemoveVersion`, `Manager.RemoveAllVersions`, `updates.applyUpdate`'s caller, `Manager.Remove`) and a `go vet`-style check or a code-review checklist anchored to those entry points. The set is small (~6 functions) and the design doc will enumerate it as part of the subscriber contract.

## Why (against the stated constraints)

### Audit-friendly: option 2 wins decisively

The constraint says "a reader should be able to find every emission site quickly." With option 2, `grep -rn 'bus.Publish' internal/ cmd/` returns the full set. With option 1, the emission is hidden inside a method whose name (`UpdateTool`) gives no hint of pub/sub semantics — a reader looking at `Manager.InstallWithOptions` has to follow into `state.UpdateTool` to discover that publishing happens. With option 3, the emission is at function entry but fires on every code path including early-return errors — readers have to mentally simulate defer semantics to understand what event was actually published.

### No false positives: option 1 fails outright

Look at `internal/install/manager.go:168` — `state.UpdateTool` is called only on the happy path. But look at `internal/updates/apply.go:164-182`: when `applyUpdate` fails, the install path may have already partially mutated state before erroring out. The rollback is `mgr.Activate(entry.Tool, previousVersion)`, which itself calls `state.UpdateTool` at `manager.go:279`. A naive option-1 shim would publish:

- `Installed{new}` (from the partial install mutation if any)
- `Activated{previousVersion}` (from rollback)

…with no semantic distinction between "this is a real activation" and "this is a rollback after a failed install." Option 1 emits state-shaped events, not lifecycle-shaped events — Decision 1 already settled on lifecycle vocabulary, so option 1 forces the subscriber (`notices`) to do post-hoc inference from event ordering, which is exactly the drift problem this bus is meant to solve.

### No false negatives: option 2 + enumerated publishers handles this

There are ~15 state-mutating call sites across the codebase (mapped in the appendix). Only a small subset (~6) are lifecycle-meaningful events. The rest (`AddRequiredBy`, `RemoveRequiredBy`, dependency back-references in `install_deps.go`, hidden flag flips in `hidden.go`) are bookkeeping mutations the subscriber doesn't care about. Option 1 would generate noise events on every bookkeeping mutation; the subscriber would have to filter them out. Option 2 publishes exactly the events the subscriber wants, by construction.

The "forgetting to publish" risk is real but bounded: the lifecycle entry points are stable and few. Adding a new install path is a design-significant change that warrants review anyway.

### Bounded import graph: all three options are viable, but option 1 is risky

The bus should live in a new low-level package, e.g. `internal/installbus/`, with no dependencies on `install` itself. Any of the three options can satisfy this if the bus package is properly leaf-positioned. However, option 1 (shim inside `internal/install`) creates pressure to put bus types inside `install` for convenience, which would force any package that imports `install` to also depend transitively on the bus interface. Option 2 keeps the `bus` import explicit and per-caller — same fanout, no surprise dependencies.

### Test-friendly: option 2 is the simplest

Tests for `notices` already assert on file-system effects today. With option 2, tests can either (a) install a fake bus and assert on emissions directly, or (b) keep asserting on notice files end-to-end. With option 1, tests can't easily distinguish "the state was mutated for bookkeeping" from "a lifecycle event happened" without separate fixtures. With option 3, defer-based emission interacts badly with Go test patterns that use `t.Run` and goroutines.

## Why not option 3 (defer)

The `defer bus.Publish(buildEvent(err))` pattern has three concrete problems for this codebase:

1. **Rollback happens outside the function.** In `internal/updates/apply.go:164-182`, the rollback `mgr.Activate(prev)` is called by the caller of `applyUpdate`, not by `applyUpdate` itself. A defer inside `applyUpdate` cannot see the rollback. A defer at the caller (`ApplyPendingUpdates`) can see it but is then 80+ lines away from the actual install attempt, which destroys the locality argument for defer in the first place.
2. **Named-return subtleties.** Go's `defer` interacts with named return values in non-obvious ways. The team has not standardized named returns. Mixing instrumentation defers with non-named-return functions silently captures stale error values.
3. **No richer payload than `err`.** Lifecycle events carry tool name, old/new version, rollback target, source (`auto` vs. `manual`). Constructing the payload at function entry forces you to either pre-compute fields that don't exist yet (new version) or close over mutable locals (race-prone). Constructing it explicitly after the work is done (option 2) is clearer.

## Why not option 1 (shim)

Beyond the false-positive argument above:

- `state.UpdateTool` is called for at least four semantically distinct reasons (install, activate, version-remove, dependency-bookkeeping). The shim can't tell them apart from the method signature alone — it would need callers to pass a hint enum, at which point you've reinvented option 2 with extra indirection.
- Decision 2 (delivery semantics, decided separately) constrains what the bus does on `Publish`. Putting the bus inside the lowest-level state manager bakes that contract into a place where it is hard to evolve.
- Future events Decision 1 may want (`Cancelled`, `Skipped`, `DryRun`) don't correspond to state.json mutations at all. Option 1 cannot emit them.

## Implementation shape

Add a new package: `internal/installbus/`.

- `Bus` interface with `Publish(Event)` and `Subscribe(Handler)`.
- `Event` types per Decision 1.
- A package-level `Default()` accessor or an explicit instance plumbed through `install.Manager`.

Modify these call sites to call `bus.Publish` after the relevant state mutation:

| File | Function | Event |
|------|----------|-------|
| `internal/install/manager.go:168-203` | `InstallWithOptions` | `Installed{tool, version, previousVersion}` |
| `internal/install/manager.go:235-290` | `Activate` | `Activated{tool, version, previousVersion}` |
| `internal/install/remove.go:55-142` | `RemoveVersion` | `VersionRemoved{tool, version, newActive}` |
| `internal/install/remove.go:145-180` | `RemoveAllVersions` | `Removed{tool}` |
| `internal/install/remove.go:183-220` | `removeToolEntirely` (last-version path) | covered by `Removed` from caller |
| `internal/updates/apply.go:120-200` | the auto-apply loop | `ApplyFailed{tool, version, err}` + rollback events emitted via `Activate` |

For the `cmd/tsuku/install_deps.go` and `plan_install.go` direct `UpdateTool` calls: route these through the `Manager` (do not let CLI code mutate state directly). This is a refactor the bus design forces but it is a pre-existing layering smell and worth fixing as part of this work.

## Assumptions

1. **Decision 1 settled on lifecycle vocabulary** (`Installed`, `Activated`, `Removed`, `ApplyFailed`, etc.), not state-mutation vocabulary (`StateUpdated`). If Decision 1 chose state-mutation events, option 1 becomes viable — re-evaluate this decision if so.
2. **Decision 2 settled on synchronous, in-process delivery**, so `Publish` is cheap and the call-site cost is negligible. If async, the considerations don't change but the test harness becomes slightly more complex regardless of option.
3. **Decision 4 will permit explicit subscriber wiring in `main.go`**, so packages emitting events do not need init-time registration. This is independent of emitter location.
4. **CLI direct state writes are a refactor target.** Centralizing the publishers requires moving the `cmd/tsuku/install_deps.go` and `cmd/tsuku/plan_install.go` `UpdateTool` calls into `install.Manager` methods, or adding a `Manager`-mediated alternative. Treat this as part of the bus implementation plan.
5. **The `~6 publisher` count is stable for the next 2-3 releases.** If new install entry points multiply, revisit option 1 with a lifecycle-hint parameter.

## Rejected options

- **Option 1 (state.json shim).** Cannot distinguish lifecycle events from bookkeeping mutations from the call signature alone; emits false-positive `Succeeded`-shaped events on rollback paths; cannot emit non-state events (`Cancelled`, `DryRun`). Coupling the bus to the lowest-level state manager constrains future evolution.
- **Option 3 (defer instrumentation).** Rollback happens outside the function's scope in `updates/apply.go`, defeating the locality benefit. Named-return subtleties and partial-state payload construction at function entry make this pattern fragile. No clean way to carry rich payloads.

## Appendix: State mutation call sites surveyed

Non-test callers of `UpdateTool`/`RemoveTool`/`AddRequiredBy`/`RemoveRequiredBy`/`UpdateToolWithoutLock`:

- `internal/install/hidden.go:42` — bookkeeping (no event)
- `internal/install/manager.go:168` — `InstallWithOptions` (lifecycle: Installed)
- `internal/install/manager.go:279` — `Activate` (lifecycle: Activated)
- `internal/install/remove.go:108` — `RemoveVersion` (lifecycle: VersionRemoved)
- `internal/install/remove.go:219` — `removeToolEntirely` (lifecycle: Removed)
- `internal/install/state_tool.go:62,74` — `AddRequiredBy`/`RemoveRequiredBy` (bookkeeping)
- `cmd/tsuku/remove.go:116,162,169` — bookkeeping + final removal (route through Manager)
- `cmd/tsuku/plan_install.go:137` — direct state write (route through Manager)
- `cmd/tsuku/install_distributed.go:219` — direct state write (route through Manager)
- `cmd/tsuku/install_deps.go:220,472,580` — direct state writes (route through Manager)

Six of these are lifecycle-meaningful; the rest are bookkeeping. This 6:9 ratio is what makes option 2 cleaner than option 1.
