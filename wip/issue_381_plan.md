# Issue 381 Implementation Plan

## Summary

Display the estimated cost of each LLM generation and cumulative daily spend after recipe creation completes.

## Approach

Add a cost display line after successful GitHub recipe creation, before the "To install" message. The display format:
```
Cost: ~$0.10 (today: $0.20 of $5.00 budget)
```

All required infrastructure already exists:
- `defaultLLMCostEstimate` (0.10) - single generation cost
- `stateManager.DailySpent()` - cumulative today
- `userCfg.LLMDailyBudget()` - configured budget

## Files to Modify
- `cmd/tsuku/create.go` - Add cost display after recipe creation

## Files to Create
- None

## Implementation Steps
- [ ] Add cost display line after "Source:" line for GitHub builds
- [ ] Use stateManager.DailySpent() for cumulative (already updated after RecordGeneration)
- [ ] Use userCfg.LLMDailyBudget() for budget display
- [ ] Handle unlimited budget case (budget = 0) gracefully

## Design Decisions

### Display Format
Per issue specification:
```
Cost: ~$0.10 (today: $0.20 of $5.00 budget)
```

### Edge Cases
- Budget = 0 (unlimited): Display without budget portion: `Cost: ~$0.10 (today: $0.20)`
- First generation of day: Display normally (daily spent = cost of this generation)

### Timing
Display AFTER RecordGeneration() so DailySpent() includes current generation.

## Testing Strategy
- Manual test: Run `tsuku create test --from github:test/test` (will fail but show output format)
- Unit tests: Not practical for CLI output

## Success Criteria
- [ ] Cost displayed after successful GitHub recipe creation
- [ ] Shows both single-generation cost and daily cumulative
- [ ] Budget display respects unlimited (0) setting
- [ ] Clean formatting matches issue specification
