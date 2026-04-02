---
status: Current
problem: |
  Auto-updates produce no outcome telemetry. Successful auto-applies reuse the
  existing install flow which fires a generic update event with no outcome field.
  Failures write local notices only. Rollbacks are invisible. There is no way to
  measure update reliability, failure rates, or rollback frequency at scale.
decision: |
  Add a separate UpdateOutcomeEvent struct with dedicated blob layout and
  update_outcome_* action prefix. Emit events directly from MaybeAutoApply at
  success/failure/rollback branch points, and from manual update/rollback commands.
  Add a /stats/updates endpoint and dashboard section for update reliability metrics.
rationale: |
  The separate-struct approach matches every existing telemetry pattern in the
  codebase (five precedents). Direct emission from auto-apply is simplest with zero
  latency impact since Client.Send is already fire-and-forget. A dedicated stats
  endpoint follows the /stats/discovery precedent and keeps concerns separated.
upstream: docs/prds/PRD-auto-update.md
---

# DESIGN: Update Outcome Telemetry

## Status

Accepted

## Context and Problem Statement

The auto-update system (Features 1-4 of the auto-update roadmap) can now check for
updates, apply them within pin boundaries, roll back on failure, and self-update the
tsuku binary. But the telemetry system hasn't kept pace. Today's `NewUpdateEvent`
fires only on successful updates and carries no outcome field -- it's structurally
identical whether the update was manual or automatic.

Failures and rollbacks are tracked locally via the notices system
(`$TSUKU_HOME/notices/`) but never leave the machine. This means:

- No visibility into what percentage of auto-updates succeed vs fail
- No data on which tools or versions cause failures
- No way to detect if a specific upstream release is breaking auto-updates across users
- No understanding of how often rollback is triggered

PRD requirement R22 calls for extending the existing telemetry system with
success/failure/rollback outcomes. The telemetry worker, stats API, and dashboard
all need updates to receive, store, and surface this data.

### Scope

**In scope:**
- CLI event struct and emission points for update outcomes
- Telemetry worker validation, dispatch, and blob layout
- Stats API endpoint for update outcome data
- Dashboard section for update reliability metrics
- Respect for existing opt-out mechanisms

**Out of scope:**
- Changing auto-apply behavior based on telemetry (feedback loops)
- Per-user or per-machine tracking
- Alerting on failure spikes
- Modifying the existing successful update event path

## Decision Drivers

- Follow established telemetry patterns (separate struct per event category, dedicated send method, action prefix dispatch)
- Analytics Engine constraint: 20 blobs max per data point
- Fire-and-forget: telemetry must never block or slow down updates, rollbacks, or any user-facing operation
- Schema version coordination between CLI and worker
- Full pipeline: CLI emission, backend processing/storage, dashboard consumption
- Privacy: no PII, no tool paths, no error messages that could contain filesystem details

## Considered Options

### Decision 1: Event Data Model

The telemetry system needs a way to represent update outcomes. The existing `Event` struct
has no outcome field and its blob layout (13 of 20 blobs used) was designed for
install/update/remove counting, not reliability tracking. The key question is whether to
extend what exists or follow the pattern every other event category uses: a dedicated struct.

Key assumptions:
- Error messages from Go contain filesystem paths and can't be transmitted as-is
- The error taxonomy must be a fixed set classified at emission time in Go
- Success events coexist with the existing `Event{Action: "update"}` (different data streams for different questions)

#### Chosen: Separate UpdateOutcomeEvent struct

A new `UpdateOutcomeEvent` struct with its own `SendUpdateOutcome()` method,
`update_outcome_*` action prefix, and dedicated blob layout. This follows the pattern
used by every other event category in the codebase: `LLMEvent`, `DiscoveryEvent`,
`VerifySelfRepairEvent`, and `BinaryNameRepairEvent` all use separate structs with
dedicated send methods and action prefixes. Five for five.

The struct uses 10 of 20 available blobs, leaving room for future fields (duration,
channel, retry count). Error classification uses a fixed taxonomy of 9 values, mapped
from Go errors at emission time -- raw error strings never cross the wire.

```go
type UpdateOutcomeEvent struct {
    Action          string `json:"action"`           // "update_outcome_success", "update_outcome_failure", "update_outcome_rollback"
    Recipe          string `json:"recipe"`           // Tool name
    VersionPrevious string `json:"version_previous"` // Version before update attempt
    VersionTarget   string `json:"version_target"`   // Version the update tried to reach
    Trigger         string `json:"trigger"`          // "auto" or "manual"
    ErrorType       string `json:"error_type"`       // Taxonomy value; empty on success
    OS              string `json:"os"`
    Arch            string `json:"arch"`
    TsukuVersion    string `json:"tsuku_version"`
    SchemaVersion   string `json:"schema_version"`   // "1"
}
```

Blob layout:

| Blob | Field | Notes |
|------|-------|-------|
| blob0 | action | `update_outcome_success`, `update_outcome_failure`, `update_outcome_rollback` |
| blob1 | recipe | Tool name |
| blob2 | version_previous | Version before attempt |
| blob3 | version_target | Target version |
| blob4 | trigger | `"auto"` or `"manual"` |
| blob5 | error_type | From taxonomy; empty on success |
| blob6 | os | Operating system |
| blob7 | arch | CPU architecture |
| blob8 | tsuku_version | CLI version |
| blob9 | schema_version | `"1"` |

Error type taxonomy:

| Value | When used |
|-------|-----------|
| `download_failed` | HTTP error or timeout fetching the artifact |
| `checksum_failed` | SHA256/signature verification mismatch |
| `extraction_failed` | Archive extraction error (tar, zip, gzip) |
| `permission_failed` | chmod or filesystem permission error |
| `symlink_failed` | Failed to create/update symlinks in `$TSUKU_HOME/bin` |
| `verification_failed` | Post-install verification command returned non-zero |
| `state_failed` | Could not read or write `state.json` |
| `version_resolve_failed` | Version provider couldn't resolve target version |
| `unknown` | Catch-all for unrecognized errors |

The `classifyError()` function prefers `errors.Is`/`errors.As` for typed errors from
tsuku's install packages (download, extraction, verification, symlink). For loosely
wrapped errors from third-party libraries where typed matching isn't possible,
`strings.Contains` serves as a fallback. `unknown` is the final catch-all. Typed
matching is preferred over string matching to avoid fragile coupling to error message
text and to reduce the risk of accidentally leaking raw error content.

#### Alternatives Considered

**Extend existing Event struct**: Add optional `outcome`, `error_type`, and `trigger`
fields to the existing `Event` struct and bump schema version to "2". Rejected because
no existing event category shares a struct across conceptually different operations.
Schema version "2" would require the worker to handle dual layouts simultaneously during
rollout, and the existing blob layout has only 7 slots remaining -- constraining future
growth for all event types sharing the struct.

**Repurpose empty blobs on existing struct**: Use new action values
(`auto_update_success`, etc.) on the existing `Event` struct, putting outcome data in
blob9-11 (currently unused for updates). Rejected because it gives blob positions dual
semantics depending on action type, breaking the assumption that blob positions have
fixed meaning within a layout. Query logic becomes action-dependent, and anyone writing
SQL must know that `blob9` means "command" for some events and "error_type" for others.

### Decision 2: Emission Architecture

The auto-apply flow in `MaybeAutoApply()` has clear success/failure/rollback branch
points. The manual `tsuku update` and `tsuku rollback` commands have their own flows.
The question is how to get telemetry events emitted at these points without adding
latency or unnecessary abstractions.

Key assumptions:
- `telemetry.Client.Send` is fire-and-forget (goroutine, 2s timeout, returns immediately)
- The `telemetry` package is a stable leaf with no transitive dependencies beyond `buildinfo` and `userconfig`
- `MaybeAutoApply` already imports 5 internal packages; one more doesn't meaningfully change the graph

#### Chosen: Emit from MaybeAutoApply directly

Pass `*telemetry.Client` as a parameter to `MaybeAutoApply`. Emit events at the three
branch points in the auto-apply loop: after successful install, after failed install
(alongside notice writing), and after successful rollback activation. For manual
`tsuku update`, add a failure event before `exitWithCode` and tag the existing success
path. For `tsuku rollback`, add a rollback event after successful `mgr.Activate`.

This is three `client.SendUpdateOutcome()` calls in `apply.go`, one failure event in
`update.go`, one trigger tag on the existing success event in `update.go`, and one
rollback event in `cmd_rollback.go`. Six instrumentation points total.

The trigger field (`"auto"` or `"manual"`) distinguishes auto-apply from manual updates
in telemetry data. Auto-apply events fire from `MaybeAutoApply` in `apply.go`; manual
events fire from the command handlers.

#### Alternatives Considered

**Callback-based emission**: Add an `OnOutcome func(OutcomeEvent)` callback to
`MaybeAutoApply`. The caller provides a closure capturing the telemetry client.
Rejected because it adds an intermediate type and indirection for what amounts to three
fire-and-forget calls. The testability gain is marginal since `telemetry.Client` already
supports a disabled mode.

**Emit via notices system**: Extend notices to cover success outcomes and emit telemetry
from `DisplayUnshownNotices()`. Rejected because it misuses the notices abstraction
(writing success notices that users should never see), delays success events until
display time, and entangles two systems that currently have clean separation.

### Decision 3: Consumption Pipeline

The telemetry data needs to be queryable and visible. The worker currently has
`/stats` for install/update counts and `/stats/discovery` for resolver metrics.
Update outcome data answers a different question ("are updates reliable?") than
existing stats ("what's popular?").

Key assumptions:
- The dashboard already fetches multiple endpoints in parallel with graceful degradation (`.catch(() => null)`)
- Analytics Engine queries against a dedicated action prefix are efficient (no cross-event-type scans)

#### Chosen: New `/stats/updates` endpoint

A dedicated `GET /stats/updates` endpoint returning update outcome metrics.
This directly follows the pattern set by `/stats/discovery`: separate response type,
separate query function, separate route handler. The existing `/stats` response stays
untouched.

The endpoint returns:
- **Outcome counts** with computed success rate (the headline metric)
- **Trigger breakdown** (auto vs manual) to measure auto-update adoption
- **Error type distribution** (top 10) to pinpoint what's breaking
- **Top failing recipes** (top 10 by failure count) to identify tools needing attention

The dashboard gets a new "Update Reliability" section with overview cards (total
updates, success rate, auto-update share) and distribution bars (outcome distribution,
trigger breakdown, top errors, top failing recipes). It reuses the existing card grid
and `distribution-grid` patterns.

#### Alternatives Considered

**Extend existing `/stats` endpoint**: Add outcome breakdowns to the per-recipe data
in the existing response. Rejected because it mixes installation popularity with
reliability data (two different questions), inflates response time for every `/stats`
call, and breaks the endpoint-per-domain pattern.

**Dual endpoints**: `/stats/updates` for aggregates plus outcome fields on `/stats`.
Rejected because it splits the same data across two endpoints with unclear ownership,
still modifies `/stats`, and duplicates queries.

## Decision Outcome

**Chosen: Separate UpdateOutcomeEvent + direct emission + dedicated /stats/updates**

### Summary

The design adds a new telemetry event category for update outcomes, following the
same pattern used by every other event category in the codebase. An
`UpdateOutcomeEvent` struct captures the tool name, previous and target versions,
trigger source (auto or manual), and an error type from a fixed taxonomy. Three
action values distinguish the outcomes: `update_outcome_success`,
`update_outcome_failure`, and `update_outcome_rollback`. The struct uses 10 of 20
available Analytics Engine blobs.

Events are emitted directly from the auto-apply loop in `MaybeAutoApply()` at the
three existing branch points (success, failure after rollback, rollback activation).
Manual `tsuku update` and `tsuku rollback` commands emit from their respective
handlers. The telemetry client is passed into `MaybeAutoApply` as a parameter; since
`Client.Send` spawns a goroutine and returns immediately, there's zero latency impact
on the auto-apply loop. Error classification happens in Go at emission time: a
`classifyError(err)` function maps Go errors to one of 9 fixed taxonomy values
before the event crosses the wire, so raw error strings (which may contain filesystem
paths) never leave the machine.

The existing `Event{Action: "update"}` continues to fire on successful updates as
it does today -- it feeds the `/stats` endpoint's recipe popularity counts. The new
`UpdateOutcomeEvent` is a parallel data stream for reliability tracking, not a
replacement. Successful manual updates emit both events (different datasets, different
questions).

On the worker side, a new action prefix check (`update_outcome_*`) dispatches to a
dedicated validation function and blob layout, following the `discovery_*` template.
A new `GET /stats/updates` endpoint queries the outcome data and returns aggregate
metrics: outcome counts with success rate, auto-vs-manual breakdown, error type
distribution, and top failing recipes. The dashboard adds an "Update Reliability"
section using the existing card grid and distribution bar patterns, fetched in the
same `Promise.all` that loads other stats endpoints.

### Rationale

The three decisions reinforce each other. The separate struct gives the worker a clean
dispatch target and the stats endpoint a focused query surface. Direct emission keeps
the CLI changes minimal (six instrumentation points) without adding abstraction layers.
The dedicated endpoint keeps update reliability queries isolated from install
popularity queries, matching the pattern already validated by `/stats/discovery`.

The main trade-off is that successful manual updates emit two telemetry events (the
existing update event and the new outcome event). This is acceptable because they
serve different analytical purposes and go to different Analytics Engine datasets. The
alternative -- replacing the old event -- would break continuity of existing update
counts in `/stats`.

## Solution Architecture

### Components

```
CLI (Go)                    Worker (TypeScript)           Dashboard (HTML)
+-----------------------+   +------------------------+   +------------------+
| UpdateOutcomeEvent    |   | validate update_outcome|   | /stats/updates   |
| SendUpdateOutcome()   |-->| blob layout (10 blobs) |-->| fetch + render   |
| classifyError()       |   | GET /stats/updates     |   | reliability cards|
+-----------------------+   +------------------------+   +------------------+
```

### Data Flow

1. Auto-apply loop or manual command determines outcome (success/failure/rollback)
2. `classifyError()` maps Go error to taxonomy value (failure events only)
3. `UpdateOutcomeEvent` constructed with all fields populated
4. `Client.SendUpdateOutcome()` spawns goroutine, POSTs to worker
5. Worker validates action prefix, maps to blob layout, writes to Analytics Engine
6. `/stats/updates` queries Analytics Engine, aggregates, returns JSON
7. Dashboard fetches endpoint, renders reliability section

### Files Modified

| File | Change |
|------|--------|
| `internal/telemetry/event.go` | Add `UpdateOutcomeEvent` struct, constructors, `classifyError()` |
| `internal/telemetry/client.go` | Add `SendUpdateOutcome()` method |
| `internal/updates/apply.go` | Add `*telemetry.Client` parameter, emit at 3 branch points |
| `cmd/tsuku/main.go` | Create telemetry client early, pass to `MaybeAutoApply` |
| `cmd/tsuku/update.go` | Add failure event; emit `UpdateOutcomeEvent` alongside existing success event |
| `cmd/tsuku/cmd_rollback.go` | Add `telemetry.NewClient()` and rollback event after `mgr.Activate` |
| `telemetry/src/index.ts` | Add `update_outcome_*` dispatch, validation, blob layout |
| `telemetry/src/index.ts` | Add `GET /stats/updates` route and query function |
| `website/stats/index.html` | Add "Update Reliability" section and inline JS for fetch/render |

### API Contract

**Worker endpoint: `GET /stats/updates`**

```json
{
  "generated_at": "2026-04-01T12:00:00Z",
  "period": "all_time",
  "total_updates": 1482,
  "by_outcome": {
    "success": 1350,
    "failure": 87,
    "rollback": 45
  },
  "success_rate": 0.911,
  "by_trigger": {
    "auto": 980,
    "manual": 502
  },
  "by_error_type": [
    {"type": "download_failed", "count": 42},
    {"type": "checksum_failed", "count": 23},
    {"type": "version_resolve_failed", "count": 15},
    {"type": "extraction_failed", "count": 7}
  ],
  "top_failing": [
    {"name": "terraform", "failures": 12, "total": 89},
    {"name": "node", "failures": 8, "total": 145}
  ]
}
```

## Implementation Approach

### Phase 1: CLI events (Go)

Add the `UpdateOutcomeEvent` struct, constructors, `classifyError()` function, and
`SendUpdateOutcome()` method. Instrument `apply.go`, `update.go`, `cmd_rollback.go`,
and `main.go`. Unit tests for `classifyError()` and event construction.

### Phase 2: Worker backend (TypeScript)

Add `update_outcome_*` action prefix dispatch, validation function, and blob layout
to `telemetry/src/index.ts`. Add `GET /stats/updates` route with query function.
Tests for validation and stats aggregation.

### Phase 3: Dashboard (HTML/JS)

Add "Update Reliability" section to `website/stats/`. Fetch `/stats/updates` in
parallel with existing endpoints. Render overview cards and distribution bars using
existing patterns. Graceful degradation when endpoint is unavailable.

## Security Considerations

**Data privacy**: The error taxonomy is a fixed enum classified in Go. Raw error
messages (which may contain filesystem paths, usernames, or other local details) never
leave the machine. Only the taxonomy value (e.g., `download_failed`) is transmitted.
The `classifyError()` function is the critical privacy boundary -- implementation must
include unit tests verifying it always returns a taxonomy value and never leaks raw
error content. The taxonomy values should be a Go `const` block for compile-time safety.

**Opt-out**: All events flow through the existing `telemetry.Client` which checks
`TSUKU_NO_TELEMETRY`, `TSUKU_TELEMETRY`, and `config.toml` telemetry settings.
`SendUpdateOutcome()` uses the same `disabled` check as every other send method.
A unit test must confirm this, consistent with how other `Send*` methods are tested.

**No new attack surface**: The `/stats/updates` endpoint is read-only and returns
only aggregate counts. No per-user, per-machine, or per-session data is queryable.
The event ingestion path reuses the existing `POST /event` endpoint with the same
validation and rate limiting.

**Input validation**: The worker validates that all fields match expected patterns
(action must be one of three values, trigger must be `"auto"` or `"manual"`,
error_type must be from the taxonomy or empty). Invalid events are rejected with 400.
Maximum field lengths must be enforced: recipe names (128 chars), version strings
(64 chars). The dashboard must HTML-escape all values rendered from the stats API.

**Known system limitations**: The shared `POST /event` handler has no request body
size limit and no rate limiting. These are pre-existing gaps across all event types,
not introduced by this design. Metric poisoning via fabricated events is possible but
bounded by the design's explicit scoping out of telemetry-driven automation. If
telemetry ever drives automated decisions, rate limiting becomes a prerequisite.

## Consequences

### Positive

- Visibility into auto-update reliability at scale for the first time
- Ability to detect problematic upstream releases breaking auto-updates across users
- Data to prioritize recipe fixes (top failing recipes)
- Clean separation from existing telemetry streams (no migration risk)

### Negative

- Successful manual updates emit two telemetry events (existing + outcome). Marginal
  bandwidth cost, acceptable for analytical clarity.
- One more event category to maintain in the worker (validation, blob layout, stats query).
  Follows established patterns so maintenance cost is low.

### Mitigations

- The dual-event cost is bounded: only successful manual updates emit both. Auto-apply
  success, all failures, and all rollbacks emit only the outcome event.
- Worker maintenance is templated: the `update_outcome_*` dispatch block is structurally
  identical to the `discovery_*` block.
