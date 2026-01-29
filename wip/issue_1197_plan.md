# Issue 1197 Implementation Plan

## Summary

Create `batch-control.json` at the repository root with the control file schema defined in DESIGN-batch-operations.md. The file will contain safe default values: batch processing enabled, no disabled ecosystems, circuit breaker entries empty, and budget counters initialized to zero.

## Approach

This is a straightforward configuration file creation task. The schema is fully specified in the design doc's "Control File Schema" section. We'll create a single JSON file with all required and optional fields initialized to their safe defaults, following the JSON schema exactly.

The file serves as the source of truth for batch operational state and acts as a pre-flight check mechanism for downstream workflows (#1204 pre-flight checks, #1205 circuit breaker updates).

## Files to Create

- `batch-control.json` (repository root) - Batch operations control plane configuration with safe defaults

## Implementation Steps

- [ ] Create `batch-control.json` at repository root with all schema fields
- [ ] Initialize required fields to safe defaults:
  - `enabled`: `true` (batch processing active)
  - `disabled_ecosystems`: `[]` (no disabled ecosystems)
  - `circuit_breaker`: `{}` (empty object, entries added per ecosystem during runtime)
  - `budget`: `{ "macos_minutes_used": 0, "linux_minutes_used": 0, "week_start": <ISO timestamp>, "sampling_active": false }`
- [ ] Include optional incident tracking fields initialized to null:
  - `reason`: `null`
  - `incident_url`: `null`
  - `disabled_by`: `null`
  - `disabled_at`: `null`
  - `expected_resume`: `null`
- [ ] Validate the file is well-formed JSON (parseable without errors)
- [ ] Verify all fields from schema section match exactly

## Success Criteria

- [ ] File exists at `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/batch-control.json`
- [ ] File is valid JSON (no syntax errors)
- [ ] All required fields present: `enabled`, `disabled_ecosystems`, `circuit_breaker`, `budget`
- [ ] All optional incident tracking fields present: `reason`, `incident_url`, `disabled_by`, `disabled_at`, `expected_resume`
- [ ] Initial state matches safe defaults: enabled=true, empty disabled_ecosystems, empty circuit_breaker object, budget counters at zero
- [ ] File can be successfully parsed by `jq` (used by downstream workflows)

## Open Questions

None. The schema is fully specified in the design doc, and the initial state is documented in IMPLEMENTATION_CONTEXT.md.
