# Issue 331 Summary

## Overview

Implemented telemetry events for LLM operations to enable success rate measurement, debugging, and observability.

## Changes Made

### Files Modified

| File | Changes |
|------|---------|
| `internal/telemetry/event.go` | Added `LLMEvent` struct and 6 factory functions for each event type |
| `internal/telemetry/client.go` | Added `SendLLM()` method and refactored to use shared `sendJSON()` |
| `internal/llm/breaker.go` | Added `BreakerTripCallback` type, `onTrip` field, and `SetOnTrip()` method |
| `internal/llm/factory.go` | Added `FailoverCallback` type, `onFailover` field, `SetOnFailover()` and `SetOnBreakerTrip()` methods |
| `internal/builders/github_release.go` | Added telemetry client support, emit all 6 event types at appropriate points |

### Events Implemented

1. **`llm_generation_started`** - Emitted when LLM recipe generation begins
   - Fields: provider, tool_name, repo, os, arch, tsuku_version, schema_version

2. **`llm_generation_completed`** - Emitted when LLM generation ends (success or failure)
   - Fields: provider, tool_name, success, duration_ms, attempts

3. **`llm_repair_attempt`** - Emitted when a repair retry starts (not first attempt)
   - Fields: provider, attempt_number, error_category

4. **`llm_validation_result`** - Emitted after each validation completes
   - Fields: passed, error_category, attempt_number

5. **`llm_provider_failover`** - Emitted when factory falls back to secondary provider
   - Fields: from_provider, to_provider, reason

6. **`llm_circuit_breaker_trip`** - Emitted when a circuit breaker trips open
   - Fields: provider, failures

## Test Results

All 19 packages pass:

```
ok  github.com/tsukumogami/tsuku
ok  github.com/tsukumogami/tsuku/cmd/tsuku
ok  github.com/tsukumogami/tsuku/internal/actions
ok  github.com/tsukumogami/tsuku/internal/builders
ok  github.com/tsukumogami/tsuku/internal/buildinfo
ok  github.com/tsukumogami/tsuku/internal/config
ok  github.com/tsukumogami/tsuku/internal/errmsg
ok  github.com/tsukumogami/tsuku/internal/executor
ok  github.com/tsukumogami/tsuku/internal/install
ok  github.com/tsukumogami/tsuku/internal/llm
ok  github.com/tsukumogami/tsuku/internal/progress
ok  github.com/tsukumogami/tsuku/internal/recipe
ok  github.com/tsukumogami/tsuku/internal/registry
ok  github.com/tsukumogami/tsuku/internal/telemetry
ok  github.com/tsukumogami/tsuku/internal/testutil
ok  github.com/tsukumogami/tsuku/internal/toolchain
ok  github.com/tsukumogami/tsuku/internal/userconfig
ok  github.com/tsukumogami/tsuku/internal/validate
ok  github.com/tsukumogami/tsuku/internal/version
```

## Technical Notes

- Uses callback pattern for factory/breaker events to avoid tight coupling
- Telemetry events are fire-and-forget (async, non-blocking)
- Respects `TSUKU_NO_TELEMETRY` and `TSUKU_TELEMETRY_DEBUG` environment variables
- Added `WithTelemetryClient()` option for testing with custom telemetry client
