# Issue 331 Implementation Plan

## Summary

Add telemetry events for LLM operations to enable success rate measurement, debugging, and observability. This implements 6 new event types that track the full lifecycle of LLM-based recipe generation.

## Approach

### Event Type Design

The existing telemetry `Event` struct is specific to tool installation operations. Rather than overloading it with LLM-specific fields, we'll create a separate `LLMEvent` struct that shares the common base fields (OS, Arch, TsukuVersion, SchemaVersion) but has its own action-specific fields.

The telemetry client will need a new `SendLLM(event LLMEvent)` method to handle the new event type.

### Event Placement

| Event | Location | Trigger |
|-------|----------|---------|
| `llm_generation_started` | `GitHubReleaseBuilder.Build()` | Before LLM call |
| `llm_generation_completed` | `GitHubReleaseBuilder.Build()` | After generation (success or failure) |
| `llm_repair_attempt` | `GitHubReleaseBuilder.generateWithRepair()` | At start of each retry |
| `llm_validation_result` | `GitHubReleaseBuilder.generateWithRepair()` | After each validation |
| `llm_provider_failover` | `Factory.GetProvider()` | When falling back to secondary |
| `llm_circuit_breaker_trip` | `CircuitBreaker.RecordFailure()` | When breaker opens |

### Telemetry Client Injection

The `GitHubReleaseBuilder` already has infrastructure for dependencies. We'll add a telemetry client field with an option function, following the existing pattern.

The `Factory` and `CircuitBreaker` need callback mechanisms to emit events since they don't have direct access to the telemetry client.

## Alternatives Considered

1. **Extend existing Event struct** - Rejected because it would add many nullable fields that are only relevant to LLM operations, making the struct confusing.

2. **Callback-based events** - For factory/breaker, callbacks are necessary since they're lower-level components that shouldn't have telemetry dependencies.

3. **Log-based approach** - Rejected because structured telemetry events are more suitable for aggregation and analysis.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/telemetry/event.go` | Add `LLMEvent` struct with action-specific fields |
| `internal/telemetry/client.go` | Add `SendLLM(event LLMEvent)` method |
| `internal/builders/github_release.go` | Emit generation_started, generation_completed, repair_attempt, validation_result |
| `internal/llm/factory.go` | Add callback for failover events, emit llm_provider_failover |
| `internal/llm/breaker.go` | Add callback for trip events, emit llm_circuit_breaker_trip |

## Files to Create

| File | Purpose |
|------|---------|
| `internal/telemetry/llm_event_test.go` | Unit tests for LLMEvent and SendLLM |

## Implementation Steps

### Step 1: Define LLMEvent struct

Add to `internal/telemetry/event.go`:

```go
type LLMEvent struct {
    Action        string  `json:"action"`
    Provider      string  `json:"provider,omitempty"`
    ToolName      string  `json:"tool_name,omitempty"`
    Repo          string  `json:"repo,omitempty"`
    Success       bool    `json:"success,omitempty"`
    Cost          float64 `json:"cost,omitempty"`
    DurationMs    int64   `json:"duration_ms,omitempty"`
    Attempts      int     `json:"attempts,omitempty"`
    AttemptNumber int     `json:"attempt_number,omitempty"`
    ErrorCategory string  `json:"error_category,omitempty"`
    Passed        bool    `json:"passed,omitempty"`
    FromProvider  string  `json:"from_provider,omitempty"`
    ToProvider    string  `json:"to_provider,omitempty"`
    Reason        string  `json:"reason,omitempty"`
    Failures      int     `json:"failures,omitempty"`
    OS            string  `json:"os"`
    Arch          string  `json:"arch"`
    TsukuVersion  string  `json:"tsuku_version"`
    SchemaVersion string  `json:"schema_version"`
}
```

Add factory functions for each event type.

### Step 2: Add SendLLM to Client

Add to `internal/telemetry/client.go`:

```go
func (c *Client) SendLLM(event LLMEvent) {
    if c.disabled {
        return
    }
    if c.debug {
        fmt.Fprintf(os.Stderr, "[telemetry debug] LLM event: %+v\n", event)
        return
    }
    go c.sendLLM(event)
}
```

### Step 3: Add callbacks to CircuitBreaker

Add to `internal/llm/breaker.go`:

```go
type BreakerCallback func(provider string, failures int)

func (cb *CircuitBreaker) SetOnTrip(callback BreakerCallback) {
    cb.onTrip = callback
}
```

Call the callback in `RecordFailure()` when the breaker transitions to open.

### Step 4: Add callbacks to Factory

Add to `internal/llm/factory.go`:

```go
type FailoverCallback func(from, to, reason string)

func (f *Factory) SetOnFailover(callback FailoverCallback) {
    f.onFailover = callback
}
```

Call in `GetProvider()` when falling back.

### Step 5: Update GitHubReleaseBuilder

Add telemetry client field and option:

```go
type GitHubReleaseBuilder struct {
    // ... existing fields
    telemetryClient *telemetry.Client
}

func WithTelemetryClient(client *telemetry.Client) Option {
    return func(b *GitHubReleaseBuilder) {
        b.telemetryClient = client
    }
}
```

Emit events in Build() and generateWithRepair().

### Step 6: Wire up callbacks

In `GitHubReleaseBuilder` initialization, set up factory and breaker callbacks to emit telemetry events.

### Step 7: Add tests

Create `internal/telemetry/llm_event_test.go` with tests for:
- LLMEvent factory functions
- SendLLM with disabled client
- SendLLM with debug client

Update `internal/builders/github_release_test.go` with tests for:
- Telemetry events emitted during generation
- Telemetry events emitted during validation failures

## Testing Strategy

1. **Unit tests** for new telemetry event types
2. **Unit tests** for callback mechanisms in factory/breaker
3. **Integration tests** verifying events are emitted correctly during LLM operations
4. **Manual testing** with debug mode to verify event payloads

## Risks

1. **Breaking change in telemetry endpoint** - The telemetry worker may need updates to handle the new event type. Mitigation: Schema version field allows the worker to distinguish event types.

2. **Performance overhead** - Events are sent asynchronously so should not impact generation time.

3. **Cost tracking accuracy** - Cost data depends on LLM provider response metadata which may not always be available.

## Success Criteria

1. All 6 event types are emitted at the correct points
2. Events contain accurate data for their fields
3. All existing tests pass
4. No regression in generation functionality
5. Debug mode outputs events for verification
