# Phase 8 Architecture Review: Error UX and Verbose Mode

## Summary

The design is solid and implementable. Key actions:

1. **Clarify Suggester interface**: The `errmsg.Suggester` interface already exists at `internal/errmsg/errmsg.go`. Decide whether to reuse it or create a new interface in discover.

2. **Specify LLMAvailability propagation**: How does the CLI pass LLM availability state to the chain resolver? Currently determined in `create.go` when constructing the chain.

3. **Swap Phases 3 and 4**: CLI formatting (Phase 4) can be tested with basic error types. Configuration-aware errors (Phase 3) need CLI formatting first.

4. **Add ecosystem probe logger**: Phase 2 should explicitly add logger injection to `EcosystemProbe`.

## Implementation Phases Assessment

| Phase | Correct? | Notes |
|-------|----------|-------|
| 1. Error Types | Yes | Foundation, no dependencies |
| 2. Chain Resolver Logging | Yes | Add EcosystemProbe logger |
| 3. Configuration-Aware Errors | Move to 4 | Needs CLI formatting first |
| 4. CLI Formatting | Move to 3 | Can test with basic types |

## Simpler Alternatives Considered

- Single `DiscoveryError` with Kind field: Rejected - less idiomatic Go
- Using `log.Default()` instead of injection: Rejected - less testable
- Per-stage logging instead of centralized: Rejected - more distributed code
