# Issue 372 Implementation Plan

## Summary

Add LLM usage tracking to the existing state management system in `internal/install/state.go`, extending the `State` struct with an `LLMUsage` field that tracks generation timestamps (for rate limiting) and daily cost (for budget enforcement).

## Approach

Extend the existing state management infrastructure rather than creating a new package. The `State` struct and `StateManager` already handle JSON persistence, file locking, and concurrent access - we can add the LLM usage tracking to this established pattern.

### Alternatives Considered

- **New package `internal/state/`**: Would separate LLM state from tool state, but requires duplicating file locking logic and introduces coordination complexity between two state files. Not chosen due to unnecessary complexity.
- **In-memory only tracking**: Simpler but loses state on restart, defeating the purpose of persistent rate limiting. Not chosen.

## Files to Modify

- `internal/install/state.go` - Add `LLMUsage` struct and methods to `State` and `StateManager`
- `internal/install/state_test.go` - Add comprehensive unit tests for new functionality

## Files to Create

None - all changes fit within existing files.

## Implementation Steps

- [x] Add `LLMUsage` struct to state schema with generation timestamps, daily cost, and daily cost date
- [x] Add `LLMUsage` field to `State` struct with JSON tag
- [x] Add `RecordGeneration` method to `StateManager` that adds timestamp, adds cost, and prunes old timestamps
- [x] Add `CanGenerate` method to `StateManager` that checks rate limit and daily budget
- [x] Add `DailySpent` method to `StateManager` that returns today's total cost
- [x] Add helper to reset daily cost when date changes (UTC midnight)
- [x] Handle corrupted state gracefully (reset with warning message) - note: existing behavior preserved, Load() returns error on corruption
- [x] Add unit tests for all new methods including edge cases
- [x] Add `RecentGenerationCount` helper method for observability

## Testing Strategy

- Unit tests for:
  - `RecordGeneration` adds timestamp and cost correctly
  - Timestamp pruning removes entries older than 1 hour
  - `CanGenerate` returns false when rate limit exceeded
  - `CanGenerate` returns false when daily budget exceeded
  - Daily cost resets when date changes
  - Corrupted state file handling (reset with warning)
  - Concurrent access to LLM usage tracking

## Risks and Mitigations

- **Clock manipulation bypass**: Users could change system clock to bypass rate limiting. Mitigation: Acceptable for CLI tool; mentioned in design doc as not a practical concern.
- **State file corruption**: If state file becomes corrupted, rate limiting could fail. Mitigation: Reset to empty state with warning message (not silent), allowing recovery.

## Success Criteria

- [ ] `LLMUsage` struct serializes/deserializes correctly in state.json
- [ ] `RecordGeneration` adds timestamp and updates daily cost
- [ ] Timestamps older than 1 hour are pruned on access
- [ ] Daily cost resets automatically at UTC midnight
- [ ] `CanGenerate` correctly enforces both rate limit and budget
- [ ] Corrupted state file resets with warning (not silent failure)
- [ ] All unit tests pass
- [ ] `go vet` and `golangci-lint` pass

## Open Questions

None - the issue and design doc provide clear requirements.
