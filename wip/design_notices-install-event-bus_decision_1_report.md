<!-- decision:start id="notices-install-event-bus-decision-1" status="assumed" -->
### Decision: Event vocabulary, payload shape, and self-update treatment

**Context**

The `internal/notices` package today drifts from `state.json` because every code path that mutates install state writes notice files directly. A render-time staleness filter was rejected (PR #2411) as patching the wrong layer. The new design introduces an in-process event bus where every install-state mutation publishes an event and `internal/notices` reconciles its files as a subscriber.

This decision answers: what are the discrete event types, what do their payloads contain, and how does tsuku self-update fit into the vocabulary? The choice constrains every subsequent publisher and subscriber in the design.

The codebase has nine notice-mutating sites today: `internal/updates/apply.go` (auto-apply success and failure with rollback), `cmd/tsuku/update.go` (single-tool and batch manual update), `internal/updates/self.go` (self-update success only), `cmd/tsuku/install_deps.go::clearAndRecordInstallSuccess` (post-install reconciliation), `internal/progress/inbox_reporter.go::Stop` (warning notices), `internal/updates/notify.go` (renderer cleanup), and `cmd/tsuku/cmd_notices.go` (user-invoked cleanup). The state mutations behind these sites are concentrated in `install.Manager.{Install, Activate, RemoveVersion, RemoveAllVersions}` plus `updates.ApplySelfUpdate`.

**Assumptions**

- A1: Bus delivery is synchronous-with-recover (assumed Decision 2 outcome). If async, the vocabulary still holds; only ordering guarantees weaken.
- A2: Publishers are explicit `Publish()` calls at the small set of state-mutation entry points listed above (assumed Decision 3 outcome). If a state.json shim is chosen, Source must be threaded through the shim.
- A3: Renderer (`internal/updates/notify.go`) is not redesigned; it keeps reading notice files. The `Kind` and `Messages` fields stay.
- A4: `Source` is a closed enum extended only via code change; not a free-form string.
- A5: `InboxReporter` keeps writing its warning-kind notices (`KindVersionFallback`, `KindShellInitChange`) directly. These are side-channel observations during an install run, not state-change events; they do NOT flow through the bus.

**Chosen: Three-event vocabulary with publish-on-state-change, self-update reused via `Tool: "tsuku"`**

Three event types in a new package `internal/installevents` (name to be confirmed in implementation):

```go
package installevents

type Source string

const (
    SourceInstall      Source = "install"       // tsuku install <tool>
    SourceManualUpdate Source = "manual-update" // tsuku update <tool>
    SourceAutoUpdate   Source = "auto-update"   // background auto-apply
    SourceRollback     Source = "rollback"      // explicit tsuku rollback
    SourceSelf         Source = "self"          // tsuku self-update
)

// Activated fires when state.ActiveVersion changes for a tool.
// Fresh installs (FromVersion == "") and version transitions are both Activated.
// Includes self-update with Tool == "tsuku".
type Activated struct {
    Tool        string
    FromVersion string    // active before; "" if fresh install or self-update from unknown
    ToVersion   string    // active after; always non-empty
    Source      Source
    Timestamp   time.Time
}

// InstallFailed fires when an install attempt did not change state.ActiveVersion
// but the user should be informed. Carries the post-attempt active version so
// rollback outcomes are described truthfully in a single event.
type InstallFailed struct {
    Tool             string
    AttemptedVersion string
    ActiveAfter      string // active version after attempt; "" if no prior install
    Err              error
    Source           Source
    Timestamp        time.Time
}

// Removed fires when a tool or version was removed from state.
type Removed struct {
    Tool        string
    Version     string // version removed; "" if all versions removed
    ActiveAfter string // new active version; "" if tool fully gone
    Source      Source
    Timestamp   time.Time
}
```

**Publish contract** (consumed by sibling decisions on publisher location):

- `install.Manager.Install` (and `InstallWithOptions`): on success publish `Activated{From: prior, To: new, Source: SourceInstall|SourceManualUpdate|SourceAutoUpdate}`. On failure publish `InstallFailed{AttemptedVersion: target, ActiveAfter: prior_or_empty, Err}`. Caller passes the Source.
- `install.Manager.Activate`: publish-on-state-change predicate. If `oldActiveVersion != newActiveVersion`, publish `Activated{From: oldActiveVersion, To: newActiveVersion, Source}`. If they're equal (rollback after failed Install whose failure path never wrote state), publish nothing.
- `install.Manager.RemoveVersion` / `RemoveAllVersions`: publish `Removed{Version, ActiveAfter, Source}` after state mutation.
- `updates.CheckAndApplySelf`: on success publish `Activated{Tool: "tsuku", From: normalizedCurrent, To: normalizedLatest, Source: SourceSelf}`. On failure publish `InstallFailed{Tool: "tsuku", AttemptedVersion: normalizedLatest, ActiveAfter: normalizedCurrent, Err, Source: SourceSelf}`. (Failure events are new behavior — today self-update failures are silent.)

**Notices subscriber reaction table** (consumed by Decision 5):

| Event | File mutation |
|---|---|
| `Activated` | `RemoveNotice(Tool)`, then `WriteNotice{Tool, AttemptedVersion: ToVersion, Error: "", Kind: kindFor(Source), Timestamp, Shown: false}` |
| `InstallFailed` | `WriteNotice{Tool, AttemptedVersion, Error: Err.Error(), Kind: kindFor(Source), ConsecutiveFailures: priorFailureCount + 1, Timestamp, Shown: false}` |
| `Removed` | `RemoveNotice(Tool)` |

`kindFor(Source)`: `SourceAutoUpdate` -> `KindAutoApplyResult`; all others -> `KindUpdateResult` (the empty-string legacy value). This matches the current renderer's distinction.

**Out-of-bus signals**: `InboxReporter.Stop()` continues to write `KindVersionFallback` and `KindShellInitChange` notices directly. These are not state events.

**Rationale**

1. **Eliminates the bug class that motivated the design.** The drift today is uncoordinated direct writes leaving the store inconsistent with state.json. With this vocabulary plus the publish-on-state-change predicate on `Activate`, the auto-apply rollback flow (the case that produced the user-visible "niwa has been updated to 0.10.4" lie) is correct without coordination: `Install`'s failure path never writes state (`manager.go:168` only runs on success), so the subsequent `Activate(previousVersion)` finds `ActiveVersion == previousVersion` already, short-circuits, and emits nothing. The `InstallFailed` event is the sole notice-write for that flow — exactly what the user should see.

2. **Self-update integrates symmetrically.** Reusing `Activated` with `Tool: "tsuku"` makes self-update structurally indistinguishable from a tool update at the subscriber. The motivating bug ("tsuku has been updated to 0.10.4" persisting after 0.11.0) is fixed by the same `RemoveNotice + WriteNotice` flow as for any tool. No separate handler. The asymmetry where self-update doesn't emit on failure today gets fixed for free: the `InstallFailed{Tool: "tsuku"}` event makes self-update failures visible.

3. **Cardinality matches subscriber needs.** Three events map to the three distinct file mutations the notices subscriber performs. No dead handlers; no field-pattern soup.

4. **Auditable wiring.** `grep -r 'Publish(installevents\.' .` lists every publish site. Each publisher is a single Manager method or `ApplySelfUpdate`. The total publisher count is small (Install, Activate, RemoveVersion, RemoveAllVersions, CheckAndApplySelf — five sites) and stable.

5. **Source enum carries trigger intent without inflating types.** Mapping to `Kind` for the on-disk notice is a one-line helper. New triggers (e.g., `tsuku run` autoinstall) add a Source constant, not an event type.

6. **Robust to sibling decisions.** Vocabulary works under sync or async delivery (D2), explicit Publish or state.json shim (D3), single-Register or per-package init (D4). The only sensitive piece is Activate's publish-on-state-change predicate, which can be implemented at any publisher location.

**Alternatives Considered**

- **Alternative A — Single `InstallStateChanged{Tool, From, To, Err, Source}`**: a catch-all event with subscribers branching on field combinations. Rejected because (a) subscriber branching expressed as field patterns is the same logic-complexity as type discrimination but loses compile-time clarity; (b) the Remove case requires the `ToVersion: ""` convention, which is a reader puzzle; (c) the rollback-correctness requires the same publish-on-state-change predicate D uses, so the only remaining differentiator is naming and cardinality, where D wins. After Phase 4 revision, A's validator conceded structurally.

- **Alternative B — Outcome-typed (`InstallSucceeded`, `InstallFailed`, `Removed`, `RolledBack`)**: rejected because `RolledBack` doesn't earn its keep. With the publish-on-state-change predicate, the rollback Activate emits nothing and the failure event carries the rollback outcome via `ActiveAfter`. Dropping `RolledBack` collapses B into D's three-event shape, modulo naming. B's `InstallSucceeded` reads naturally for fresh installs but is wrong for `tsuku rollback` (where the active version changes without any install happening); D's `Activated` is technically truthful in both cases.

- **Alternative C — Lifecycle granularity (`InstallStarted`, `InstallSucceeded`, `InstallFailed`, `InstallCancelled`, `Removed`, `SelfUpdated`)**: rejected as premature granularity. Three of six events have no subscriber today. Publisher pairing (Started + terminal) introduces new error surface. `SelfUpdated` duplicates `InstallSucceeded` handler logic. Lifecycle events can be added later as a focused PR when a real telemetry/UI subscriber needs them; existing subscribers ignore them by not registering.

**Consequences**

What becomes easier:
- The notices store deterministically reflects the most recent state mutation. Drift bugs of the PR #2411 class cannot recur.
- Self-update notices have a well-defined lifecycle even though tsuku isn't in state.json.
- New triggers extend the Source enum, not the event types.
- Self-update failures become visible (new behavior; today they're silent).
- Removing a tool cleans up any orphaned notice (new behavior; today the file is left).

What becomes harder:
- Contributors must remember Activate's publish-on-state-change contract. Mitigation: explicit predicate in code with a comment, plus a unit test that asserts no publish for a no-op Activate.
- Source must be threaded through Manager methods. Today's Install/Activate don't take a "who's calling" argument; this design adds one (as a parameter or via an options struct). Affects ~10 call sites.
- The `Activated` name may surprise readers expecting `Installed` for fresh installs. Mitigation: godoc on the type explaining the choice. The team may rename to `VersionApplied` or similar in implementation review without structural change.

What stays the same:
- Notice file schema, renderer, `Kind`/`Messages` semantics.
- `InboxReporter` warning notices write directly, outside the bus.
- Telemetry events in `internal/telemetry/` remain independent.

**Cross-validation hooks for sibling decisions**:
- Decision 2 (sync vs async): vocabulary unaffected by choice. Recommend sync-with-recover so the subscriber's file write happens within the publisher's call frame; ordering between Activated and a subsequent Removed for the same tool is preserved.
- Decision 3 (publisher location): vocabulary works under any choice. The publish-on-state-change predicate on Activate is the load-bearing detail — wherever Publish() lives, it must inspect oldActive vs newActive.
- Decision 4 (subscriber registration): vocabulary doesn't constrain.

<!-- decision:end -->
