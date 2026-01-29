---
summary:
  constraints:
    - File must be valid JSON at repository root
    - Schema must match design doc's Control File Schema section exactly
    - Initial state: enabled=true, empty disabled_ecosystems, all circuit breakers closed, budget at zero
    - Repository-primary storage: this file is the source of truth for batch operational state
  integration_points:
    - batch-control.json at repo root (new file)
    - Downstream: #1204 reads this file for pre-flight checks
    - Downstream: #1205 reads/writes circuit_breaker field
    - Downstream: #1206 rollback script uses batch_id commit format
  risks:
    - Schema mismatch with design doc if fields are missed
  approach_notes: |
    Create batch-control.json at repo root with all fields from the design doc schema.
    Initial state has everything in "safe" defaults: enabled, no disabled ecosystems,
    no circuit breaker entries (empty object), budget counters at zero.
---
