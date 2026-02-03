---
status: Current
problem: |
  The discovery resolver runs through up to three stages (registry lookup, ecosystem probe, LLM discovery) but produces no telemetry. There's no visibility into how often each stage fires, what tools users search for, or where resolution fails. Without this data, we can't prioritize registry curation, measure ecosystem probe reliability, or evaluate LLM discovery effectiveness once it ships.
decision: |
  Add a DiscoveryEvent struct following the existing LLM event pattern: separate struct, SendDiscovery() method on the telemetry client, discovery_* action prefix for backend dispatch, and a dedicated blob layout in Analytics Engine. Instrument the chain resolver to emit events at each stage completion. Add a /stats/discovery endpoint and a dashboard section showing resolver stage distribution.
rationale: |
  The separate-struct approach matches the established pattern (Event for installs, LLMEvent for LLM ops) and keeps each category's fields clean without overloading unrelated structs. The action prefix dispatch pattern already works in the backend and scales naturally to a third event category. Emitting events at stage boundaries rather than only on final resolution captures the full resolver pipeline behavior, which is what we need to tune registry curation and probe reliability.
---

# DESIGN: Discovery Telemetry Events

## Status

Current

## Implementation Issues

| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#1319: add discovery telemetry events](https://github.com/tsukumogami/tsuku/issues/1319)~~ | ~~None~~ | ~~testable~~ |
| ~~[#1423: add backend support for discovery events](https://github.com/tsukumogami/tsuku/issues/1423)~~ | ~~None~~ | ~~testable~~ |
| ~~[#1424: add /stats/discovery endpoint](https://github.com/tsukumogami/tsuku/issues/1424)~~ | ~~[#1423](https://github.com/tsukumogami/tsuku/issues/1423)~~ | ~~testable~~ |
| ~~[#1425: add discovery stats section to dashboard](https://github.com/tsukumogami/tsuku/issues/1425)~~ | ~~[#1424](https://github.com/tsukumogami/tsuku/issues/1424)~~ | ~~simple~~ |

## Context and Problem Statement

The discovery resolver (`internal/discover/`) resolves tool names to installation recipes through a three-stage chain: registry lookup, ecosystem probe, and LLM discovery (currently a stub). Each stage has different confidence levels, latency characteristics, and failure modes.

Today, none of these stages emit telemetry. When a user runs `tsuku install ripgrep` and the resolver finds it in the registry, or falls through to ecosystem probing, or fails entirely, we have no record of what happened. The existing telemetry system tracks installs, updates, removes, and LLM generation events, but the discovery path between "user types a tool name" and "recipe gets installed" is invisible.

This gap matters now because ecosystem probing (#1316) shipped recently and LLM discovery is on the roadmap. Without baseline data on resolver behavior, we can't measure whether new stages are working, which tools are missing from the registry, or how often the quality filter rejects ecosystem matches.

### Scope

**In scope:**
- `DiscoveryEvent` struct and factory functions in `internal/telemetry/`
- `SendDiscovery()` method on the telemetry client
- Backend dispatch and blob layout for `discovery_*` events
- Backend validation for discovery event actions
- `/stats/discovery` API endpoint
- Dashboard section on `website/stats/`
- Instrumentation points in the chain resolver

**Out of scope:**
- Changing the resolver's behavior based on telemetry (feedback loops)
- Real-time alerting on resolver failures
- Per-user tracking or session correlation

## Decision Drivers

- Follow existing telemetry patterns: separate event struct per category, dedicated `Send*` method, action prefix dispatch
- Don't transmit sensitive data: no API responses, no source URLs, no user-identifiable information
- Fire-and-forget: telemetry must never block or slow down resolution
- Analytics Engine blob limit: 20 blobs maximum per data point
- Keep the dashboard simple: show what matters for registry curation decisions

## Considered Options

### Decision 1: Event Structure

The telemetry system already has two event categories with distinct structs (`Event` and `LLMEvent`). Discovery events have their own set of fields (tool name, stage, confidence, match count, latency) that don't overlap with either existing category. The question is how to represent them.

#### Chosen: Separate DiscoveryEvent struct

Add a `DiscoveryEvent` struct with discovery-specific fields, a `SendDiscovery()` method on the client, and a `discovery_*` action prefix. The backend dispatches to a third blob layout based on the prefix.

This follows the pattern established by LLM events: each event category gets its own struct, send method, and blob layout. The structs stay focused on their domain without nullable fields for other categories.

#### Alternatives Considered

**Reuse existing Event struct with discovery_* actions**: Add discovery-specific fields (stage, confidence, match_count) as optional fields on the existing `Event` struct. Rejected because it would add 6+ nullable fields to a struct used for install/update/remove events, muddying both the Go type and the blob layout. The standard event blob layout is already at 13 elements with no room for discovery fields without reshuffling.

**Generic event map**: Use `map[string]interface{}` for all event types, with the backend parsing by action. Rejected because it eliminates compile-time type safety, makes factory functions meaningless, and complicates the backend validation that currently uses typed interfaces.

### Decision 2: Instrumentation Granularity

The chain resolver can be instrumented at different levels of detail. More events give richer data but add noise and bandwidth.

#### Chosen: One event per resolution attempt

Emit a single `DiscoveryEvent` per `tsuku install <tool>` invocation that reaches the resolver. The event captures the final outcome (which stage succeeded, or that no stage found a match) along with per-stage timing. This means one event per tool lookup, not one per stage.

The action field distinguishes outcomes: `discovery_registry_hit`, `discovery_ecosystem_hit`, `discovery_llm_hit`, `discovery_not_found`, `discovery_error`. Disambiguation events use `discovery_disambiguation` when the resolver found multiple matches.

#### Alternatives Considered

**One event per stage**: Emit separate events for each stage the resolver attempts (e.g., registry miss, then ecosystem hit). Rejected because it generates 2-3x the event volume for minimal extra insight. The single event already captures which stage succeeded and total latency. If we need per-stage failure analysis later, we can add it without breaking the schema.

### Decision 3: Dashboard Presentation

#### Chosen: Add a discovery section to the existing stats page

Add a "Discovery" section below the current install stats on `website/stats/`. Show three things: resolver stage distribution (pie/bar chart of registry vs ecosystem vs LLM hits), top searched tools not in registry, and discovery error rate. The data comes from a new `/stats/discovery` endpoint.

#### Alternatives Considered

**Separate discovery dashboard page**: Create `website/stats/discovery/index.html`. Rejected because the current stats page is lightweight and discovery stats are closely related to install stats. A separate page fragments the view unnecessarily.

## Decision Outcome

### Summary

Each discovery resolution attempt emits one `DiscoveryEvent` with the outcome (which stage found a match, or failure). The event carries the tool name, confidence level (which doubles as the winning stage), builder name, match count (for disambiguation), and total resolution latency. Events use a `discovery_*` action prefix so the backend can dispatch them to a dedicated blob layout in Analytics Engine.

On the CLI side, the chain resolver in `chain.go` instruments its main loop. After the chain completes (success or failure), it calls `telemetry.SendDiscovery()` with the result. The `DiscoveryEvent` struct follows the same pattern as `LLMEvent`: separate type, factory functions per action, `newBaseDiscoveryEvent()` for common fields.

The backend adds a `DiscoveryActionType` union, a `validateDiscoveryEvent()` function, and a blob layout mapping. Discovery events write to the same `tsuku_telemetry` Analytics Engine dataset but with their own blob positions. A `/stats/discovery` endpoint queries these blobs and returns stage distribution, top undiscovered tools, and error rates. The stats dashboard adds a section to display this data.

### Rationale

This combination works because each piece follows an established pattern. The separate struct keeps type safety. The action prefix dispatch requires minimal backend changes (one more `if` branch in the event handler). The single-event-per-resolution approach keeps volume manageable while still capturing the key question: which stage is doing the work?

### Trade-offs Accepted

- One event per resolution loses per-stage failure detail (e.g., "registry missed, then ecosystem timed out, then ecosystem retried and hit"). We accept this because the winning stage and total latency answer the primary questions. Per-stage detail can be added later as additional optional fields if needed.
- The blob layout consumes another set of blob positions in Analytics Engine. With three categories (standard: 13, LLM: 16, discovery: ~14), we're using the dataset efficiently but it does mean queries must know which blob positions belong to which event type.

## Solution Architecture

### DiscoveryEvent Struct (Go)

```go
type DiscoveryEvent struct {
    Action        string `json:"action"`                    // discovery_* action type
    ToolName      string `json:"tool_name"`                 // normalized tool name (max 128 chars, [a-z0-9-])
    Confidence    string `json:"confidence,omitempty"`      // winning stage: "registry", "ecosystem", "llm"
    Builder       string `json:"builder,omitempty"`         // builder name (github, cargo, pip, etc.)
    Source        string `json:"source,omitempty"`          // source identifier (max 256 chars, e.g., "sharkdp/bat")
    MatchCount    int    `json:"match_count,omitempty"`     // number of matches (disambiguation)
    ErrorCategory string `json:"error_category,omitempty"`  // error type on failure (see table below)
    DurationMs    int64  `json:"duration_ms"`               // total resolution time in milliseconds
    OS            string `json:"os"`
    Arch          string `json:"arch"`
    TsukuVersion  string `json:"tsuku_version"`
    SchemaVersion string `json:"schema_version"`
}
```

### Error Categories

| Category | When |
|----------|------|
| `timeout` | Stage exceeded its deadline |
| `api_error` | Upstream API returned an error |
| `quality_rejected` | Ecosystem match filtered by quality thresholds |
| `parse_error` | Response couldn't be parsed |
| `internal` | Unexpected error in resolver logic |

### Action Types

| Action | When |
|--------|------|
| `discovery_registry_hit` | Registry lookup found a match |
| `discovery_ecosystem_hit` | Ecosystem probe found a match |
| `discovery_llm_hit` | LLM discovery found a match |
| `discovery_not_found` | No stage found a match |
| `discovery_disambiguation` | Multiple matches, resolver chose one |
| `discovery_error` | A stage failed with an error |

### Backend Validation

The backend validates discovery events before writing to Analytics Engine:

- `tool_name`: required, max 128 characters, must match `^[a-z0-9][a-z0-9-]*$`
- `source`: optional, max 256 characters
- `os`, `arch`, `tsuku_version`: required (same as other event types)
- Action-specific: `discovery_error` requires `error_category`; hit actions require `confidence` and `builder`

This input validation prevents oversized or malformed tool names from being stored. The charset restriction limits tool names to the same format used by recipe names.

### Backend Blob Layout

```
blob0:  action            (discovery_*)
blob1:  tool_name
blob2:  confidence
blob3:  builder
blob4:  source
blob5:  match_count       (as string)
blob6:  error_category
blob7:  duration_ms       (as string)
blob8:  os
blob9:  arch
blob10: tsuku_version
blob11: schema_version
```

Index: `tool_name` (enables per-tool queries).

### Data Flow

```
User: tsuku install bat
  → install.go:tryDiscoveryFallback()
    → chain.go:Resolve()
      → registry_lookup.go → hit
      → [ecosystem_probe.go skipped]
    → telemetry.SendDiscovery(NewDiscoveryRegistryHitEvent("bat", "github", "sharkdp/bat", 45))
      → goroutine → POST /event → Analytics Engine
```

### Stats Endpoint

`GET /stats/discovery` returns:

```json
{
  "generated_at": "2026-02-02T...",
  "period": "all_time",
  "total_lookups": 1234,
  "by_stage": {
    "registry": 800,
    "ecosystem": 350,
    "llm": 10,
    "not_found": 74
  },
  "top_not_found": [
    {"name": "sometool", "count": 42},
    {"name": "othertool", "count": 15}
  ],
  "error_rate": 0.02
}
```

## Implementation Approach

### Phase 1: CLI telemetry (Go)

Add `DiscoveryEvent` struct and factory functions to `internal/telemetry/event.go`. Add `SendDiscovery()` to `client.go`. Add unit tests for event construction and send behavior.

Instrument `chain.go` to record start time and emit an event after resolution completes. The chain resolver already has the `DiscoveryResult` with all needed fields (Builder, Source, Confidence, Reason).

### Phase 2: Backend support (TypeScript)

Add `DiscoveryActionType` union, `DiscoveryTelemetryEvent` interface, and `validateDiscoveryEvent()` to the worker. Add the `discovery_*` dispatch branch in the `/event` handler. Define the blob layout mapping.

Add the `/stats/discovery` endpoint with queries against the discovery blob positions.

### Phase 3: Dashboard (HTML/JS)

Add a "Discovery" section to `website/stats/index.html`. Fetch from `/stats/discovery` and display stage distribution, top not-found tools, and error rate. Follow the existing dashboard's styling and layout patterns.

### Phase 4: Integration testing

Verify end-to-end: CLI emits event, backend accepts and stores, stats endpoint returns data, dashboard displays it. This can be done manually with `npx wrangler dev` for the backend and a local tsuku build with `TSUKU_TELEMETRY_DEBUG=1`.

## Security Considerations

### Download Verification

Not applicable. This feature adds telemetry emission, not artifact downloads. No binaries or external files are fetched by the telemetry system itself.

### Execution Isolation

Discovery telemetry runs within the existing fire-and-forget goroutine pattern. It doesn't require any additional permissions, file system access, or network access beyond the single POST to the telemetry endpoint (already established by `Send()` and `SendLLM()`).

### Supply Chain Risks

Not applicable. No new external dependencies are introduced. The telemetry worker is a Cloudflare Worker already deployed, and the CLI uses the standard `net/http` client.

### User Data Exposure

Discovery events transmit the tool name the user searched for, plus OS/arch/tsuku version (same as existing events). The tool name is the key new data point.

**Mitigations:**
- Tool names are validated server-side: max 128 characters, `[a-z0-9-]` charset only. This prevents using tool names as a data exfiltration channel (e.g., encoding arbitrary data in the name field). The CLI also normalizes tool names before sending.
- Source field validated: max 256 characters, preventing oversized payloads
- No API keys, tokens, URLs, or response bodies are included
- No user identifiers or session correlation
- Telemetry remains opt-out via `TSUKU_NO_TELEMETRY` or `TSUKU_TELEMETRY=0`
- Dashboard escapes all values before rendering (standard XSS prevention)

**Residual risk:** Tool name patterns could reveal what a user is interested in installing. This is the same risk as existing install telemetry (which already sends recipe names) and is mitigated by the opt-out mechanism.

## Consequences

### Positive

- Visibility into which resolver stages handle traffic, enabling data-driven registry curation
- "Top not found" list identifies tools that should be added to the registry
- Error rate tracking catches ecosystem probe reliability regressions
- Baseline data for evaluating LLM discovery effectiveness when it ships

### Negative

- Third event category adds complexity to the backend dispatch logic and blob layout management
- Dashboard grows larger, potentially slower to load with additional API calls

### Mitigations

- Backend complexity is bounded: the dispatch pattern is mechanical (check prefix, call validator, write blobs)
- Dashboard can lazy-load the discovery section or collapse it by default
