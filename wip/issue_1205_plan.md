# Issue 1205 Plan

## Approach

Create two standalone shell scripts for the circuit breaker state machine, then
integrate them into the batch-operations.yml workflow.

## Files to Create

1. `scripts/check_breaker.sh` - Reads circuit breaker state, outputs skip/proceed
2. `scripts/update_breaker.sh` - Updates state after success/failure

## Files to Modify

1. `.github/workflows/batch-operations.yml` - Add circuit breaker steps to process job

## Implementation Steps

1. Create `scripts/check_breaker.sh`:
   - Read ecosystem state from batch-control.json
   - If closed: proceed
   - If open: check if recovery timeout elapsed; if not, skip; if yes, set half-open
   - If half-open: proceed (probe request)
   - Output: skip=true/false, state for downstream

2. Create `scripts/update_breaker.sh`:
   - Takes ecosystem and outcome (success/failure) as args
   - On success: reset failures to 0, set state to closed
   - On failure in closed: increment failures, trip to open at >=5
   - On failure in half-open: reopen with fresh timeout
   - Persist state to batch-control.json

3. Integrate into batch-operations.yml:
   - Add check_breaker step before processing
   - Add update_breaker step after processing (always runs)
   - Add git commit + push with retry for state persistence
