# Issue 381 Summary

## What Was Implemented

Added cost display after LLM-generated recipe creation to provide transparency into LLM usage costs.

## Changes Made

- `cmd/tsuku/create.go`:
  - Moved `userCfg` variable to outer scope for access in cost display
  - Added cost display line after "Source:" output for GitHub builds
  - Two formats depending on budget setting:
    - With budget: `Cost: ~$0.10 (today: $0.20 of $5.00 budget)`
    - Unlimited (budget=0): `Cost: ~$0.10 (today: $0.20)`

## Key Decisions

- **Display after RecordGeneration**: DailySpent() includes the current generation's cost
- **Conditional on budget > 0**: Show budget context only when a limit is configured
- **Use ~ prefix**: Indicates cost is an estimate, not exact (per existing constant naming)

## Trade-offs Accepted

- **Fixed cost estimate**: Uses `defaultLLMCostEstimate` constant ($0.10) rather than actual token-based cost
- This is acceptable because accurate token-based costing would require API changes

## Test Coverage

- Existing tests pass
- Manual verification: Format matches issue specification

## Future Improvements

- Calculate actual cost from LLM provider token usage response (requires API integration)
- Add `tsuku llm status` command for detailed usage report
