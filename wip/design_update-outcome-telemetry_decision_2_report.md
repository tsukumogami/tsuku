# Decision 2: Where and when should outcome events fire?

## Option A: Emit from MaybeAutoApply directly

Pass `*telemetry.Client` as a fourth parameter to `MaybeAutoApply`.

**Simplicity**: High. Three `client.Send()` calls at lines 84 (success), 96 (failure), and 89 (rollback) in `apply.go`. For manual update, add failure/rollback events in `update.go` alongside the existing success event on line 110.

**Testability**: Moderate. Tests for `MaybeAutoApply` now need a telemetry client (or nil). But `Client.Send` is already fire-and-forget with a disabled mode, so passing `NewClientWithOptions("", 0, true, false)` works fine for tests. No interface extraction needed.

**Latency impact**: Zero. `Client.Send` spawns a goroutine and returns immediately. The HTTP POST happens in the background. Auto-apply loop is not blocked.

**Coupling**: `internal/updates` gains an import on `internal/telemetry`. This is a new dependency, but both packages are internal and telemetry is already a leaf package with no transitive dependencies beyond `buildinfo` and `userconfig`.

**Coverage**: All three outcome types (success, failure, rollback) in both auto and manual flows. Complete.

## Option B: Callback-based emission

Add an `OnOutcome func(OutcomeEvent)` callback to `MaybeAutoApply` or an `AutoApplyOptions` struct. The caller in `main.go` provides a closure that captures the telemetry client.

**Simplicity**: Medium. Requires defining an `OutcomeEvent` struct and an observer interface or callback type. More indirection for what amounts to three fire-and-forget calls.

**Testability**: Slightly better than A -- tests can capture outcomes via a recording callback without importing telemetry at all. But the marginal gain is small since telemetry's `Client` already supports disabled mode.

**Latency impact**: Same as A (fire-and-forget in the callback body).

**Coupling**: `internal/updates` stays free of `internal/telemetry` imports. The coupling moves to `cmd/tsuku/main.go` which already imports both. However, this requires a new intermediate type (`OutcomeEvent`) that duplicates information already in `telemetry.Event`.

**Coverage**: Same as A -- complete.

## Option C: Emit via notices system

Extend `notices.Notice` to cover success outcomes. Write a notice for every outcome. Emit telemetry in `DisplayUnshownNotices`.

**Simplicity**: Low. Requires changing the notices schema, writing success notices that users should never see, and adding telemetry emission logic to a display function. The notices system exists for user-facing messages, not machine-to-machine signaling.

**Testability**: Worse. Tests must set up a notices directory, write files, then verify telemetry emission happens when notices are displayed. More moving parts.

**Latency impact**: Delayed. Success events only fire when `DisplayUnshownNotices` runs, which is at the end of PersistentPreRun. Not a functional problem, but success telemetry for tool N fires after tools N+1..M are processed, making timestamps less accurate.

**Coupling**: Notices and telemetry become entangled. The notices package (currently pure filesystem I/O) would need telemetry awareness or its own callback mechanism.

**Coverage**: Complete but awkward. Success notices are written solely for telemetry, then suppressed from display -- a clear misuse of the abstraction.

## Recommendation: Option A

**Rationale**: Option A is the simplest approach that covers all requirements. The coupling concern (updates importing telemetry) is real but minor -- telemetry is a stable leaf package, and `MaybeAutoApply` already depends on `config`, `install`, `notices`, and `userconfig`. Adding one more internal import doesn't meaningfully change the dependency graph. Option B's decoupling benefit doesn't justify the extra abstraction for three call sites. Option C misuses an existing system.

## Instrumentation points

### Auto-apply flow (`internal/updates/apply.go`)

**Signature change** (line 28):
```go
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config,
    installFn InstallFunc, tc *telemetry.Client) {
```

**Success event** -- after line 84, when `result.err == nil`:
```go
if tc != nil {
    tc.Send(telemetry.NewUpdateEvent(entry.Tool, previousVersion, entry.LatestWithinPin).
        WithTrigger("auto"))
}
```

**Failure event** -- inside the `result.err != nil` block, around line 96:
```go
if tc != nil {
    tc.Send(telemetry.NewUpdateFailureEvent(entry.Tool, entry.LatestWithinPin,
        classifyError(result.err), "auto"))
}
```

**Rollback event** -- after successful `mgr.Activate` call, around line 89:
```go
if tc != nil {
    tc.Send(telemetry.NewUpdateRollbackEvent(entry.Tool,
        entry.LatestWithinPin, previousVersion, "auto"))
}
```

### Caller update (`cmd/tsuku/main.go`, line 76)

Pass the telemetry client created in command setup:
```go
updates.MaybeAutoApply(cfg, userCfg, installFn, telemetryClient)
```

Note: `main.go` currently passes `nil` as the telemetry client to the install callback (line 76). A telemetry client needs to be created in PersistentPreRun scope for this to work. Since `NewClient()` is cheap (no network calls), create it early.

### Manual update flow (`cmd/tsuku/update.go`)

**Failure event** -- wrap the existing error exit at line 95:
```go
if err := runInstallWithTelemetry(...); err != nil {
    if telemetryClient != nil {
        telemetryClient.Send(telemetry.NewUpdateFailureEvent(
            toolName, reqVersion, classifyError(err), "manual"))
    }
    exitWithCode(ExitInstallFailed)
}
```

**Success event** -- modify existing event at line 110 to include trigger:
```go
event := telemetry.NewUpdateEvent(toolName, previousVersion, newVersion).
    WithTrigger("manual")
```

### Manual rollback flow (`cmd/tsuku/cmd_rollback.go`)

**Rollback event** -- after successful `mgr.Activate` at line 61:
```go
if tc != nil {
    tc.Send(telemetry.NewUpdateRollbackEvent(
        toolName, currentVersion, ts.PreviousVersion, "manual"))
}
```

This requires creating a telemetry client in the rollback command, consistent with how `update.go` does it.

## Distinguishing auto vs manual

Add a `Trigger` field to the `Event` struct (`"auto"` or `"manual"`). A `WithTrigger` method on `Event` keeps the existing constructor API stable:

```go
func (e Event) WithTrigger(trigger string) Event {
    e.Trigger = trigger
    return e
}
```

New event constructors (`NewUpdateFailureEvent`, `NewUpdateRollbackEvent`) follow the existing pattern in `event.go` and include a trigger parameter directly.

## Confidence

High. The code paths are well-defined, `Client.Send` is already fire-and-forget with goroutine dispatch, and the existing telemetry patterns in `update.go` and `event.go` provide a clear template. The only design question was coupling, and the import graph makes Option A's trade-off acceptable.
