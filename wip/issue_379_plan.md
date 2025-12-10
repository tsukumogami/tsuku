# Issue 379 Implementation Plan

## Summary

Wire up daily budget enforcement in the `create` command using the existing infrastructure from #372 (StateManager.CanGenerate, RecordGeneration).

## Approach

The infrastructure from #372 already provides:
- `StateManager.CanGenerate(hourlyLimit, dailyBudget)` - checks if generation is allowed
- `StateManager.RecordGeneration(cost)` - records a generation with its cost
- `StateManager.DailySpent()` - returns total spent today

Integration points:
1. Check budget before calling `builder.Build()` for GitHub sources
2. Record cost after successful generation (estimated cost per generation)
3. Show clear error message when budget exhausted

### Alternatives Considered
- **Pass budget info to builder**: Rejected - keeps budget logic centralized in create command
- **Check budget inside builder**: Rejected - builder shouldn't know about global state

## Files to Modify
- `cmd/tsuku/create.go` - Add budget check before LLM generation, record cost after

## Files to Create
- None (infrastructure already exists in `internal/install/state.go`)

## Implementation Steps
- [x] Add budget check before GitHub builder creation/usage in runCreate
- [x] Add cost recording after successful LLM generation
- [x] Add clear error message when budget exhausted (matching issue format)
- [x] Manual verification of all cases (budget, rate limit, LLM disabled)

## Design Decisions

### Cost Estimation
Per the design doc, use ~$0.10 default cost per generation. This is an estimate that can be refined later. The cost constant should be defined in the create command or config.

### Error Message Format
From issue #379:
```
Error: daily LLM budget exhausted ($5.00 spent today)
Budget resets at midnight. To adjust: tsuku config set llm.daily_budget 10.0
```

### Timing
- Check budget BEFORE making LLM calls (fail fast)
- Record cost AFTER successful generation (only count successful attempts)

## Testing Strategy
- Unit test: Mock StateManager to verify budget check is called
- Integration test: Not practical (requires real LLM calls)
- Manual test: Run `tsuku create --from github:owner/repo` with budget=0

## Success Criteria
- [ ] Budget checked before LLM generation starts
- [ ] Clear error message when budget exhausted
- [ ] Cost recorded after successful generation
- [ ] Respects `llm.daily_budget` config setting

## Open Questions
None - design doc and existing infrastructure provide clear guidance.
