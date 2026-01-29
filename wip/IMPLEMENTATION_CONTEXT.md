---
summary:
  constraints:
    - State transitions: CLOSED->OPEN (>=5 failures), OPEN->HALF-OPEN (after timeout), HALF-OPEN->CLOSED (success), HALF-OPEN->OPEN (failure)
    - State persisted in batch-control.json circuit_breaker field
    - Push with retry for concurrent updates
    - Warning annotations when skipping processing
  integration_points:
    - batch-control.json circuit_breaker field (read/write)
    - .github/workflows/batch-operations.yml (add breaker check + post-batch update steps)
    - Scripts: check_breaker.sh and update_breaker.sh for testable state machine
  risks:
    - Date arithmetic in shell (GNU vs BSD date)
    - Concurrent workflow updates to batch-control.json
  approach_notes: |
    Create two shell scripts (check_breaker.sh, update_breaker.sh) that implement
    the state machine. Integrate them into batch-operations.yml as steps in the
    process job. The validation script from the issue tests the scripts directly.
---
