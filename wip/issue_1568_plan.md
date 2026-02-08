# Issue 1568 Implementation Plan

## Summary

Add `blocked_by` field extraction to CI validation workflow by using `--json` output and parsing `missing_recipes` from tsuku install failures with exit code 8.

## Approach

The CI workflow's per-recipe failure format lacks the `blocked_by` field that the Go orchestrator includes. The fix requires:
1. Adding `--json` flag to validation install commands
2. Capturing and parsing JSON stdout for `missing_recipes`
3. Including `blocked_by` in per-recipe failure records
4. Updating dashboard to read `blocked_by` from per-recipe format

### Alternatives Considered

- **Alternative 1: Use Go orchestrator for validation too**: Would require significant refactoring of the validation jobs which run in Docker containers. The orchestrator approach is designed for the generation phase, not cross-platform validation. Rejected due to complexity.

- **Alternative 2: Post-process failures to add blocked_by**: Could add a separate step after validation that re-parses error messages for missing recipes. Rejected because it duplicates the extraction logic and is less reliable than capturing structured output.

## Files to Modify

- `.github/workflows/batch-generate.yml` - Add `--json` flag to validation commands, capture stdout, extract `missing_recipes`, write `blocked_by` in failure records
- `internal/dashboard/dashboard.go` - Add parsing of `blocked_by` from per-recipe format records
- `internal/dashboard/dashboard_test.go` - Add test coverage for per-recipe format with `blocked_by`
- `data/schemas/failure-record.schema.json` - Add schema for per-recipe format (currently only defines batch format)

## Files to Create

None

## Implementation Steps

- [ ] Step 1: Update validation commands in batch-generate.yml
  - Change `tsuku install --force --recipe` to `tsuku install --json --force --recipe`
  - Capture stdout to a temp file for JSON parsing
  - Keep stderr for logging

- [ ] Step 2: Add blocked_by extraction in batch-generate.yml
  - After failed validation (exit code 8), parse JSON stdout for `missing_recipes`
  - Store in a shell variable for use in failure record writing

- [ ] Step 3: Update per-recipe failure format in batch-generate.yml
  - Modify jq command at lines 741-750 to include `blocked_by` when category is `missing_dep`
  - The jq filter should add the array from the parsed JSON output

- [ ] Step 4: Update dashboard.go to read blocked_by from per-recipe format
  - In `loadFailures()`, handle per-recipe format records (line 312-316) to extract `blocked_by`
  - Add field to the per-recipe format struct
  - Populate `details` map and `blockers` aggregation

- [ ] Step 5: Add tests for per-recipe blocked_by parsing
  - Add testdata file with per-recipe format including `blocked_by`
  - Add test case in `dashboard_test.go` verifying blockers are extracted

- [ ] Step 6: Update failure-record schema
  - Add `oneOf` or conditional schema for per-recipe format
  - Document both batch and per-recipe formats in same schema

- [ ] Step 7: Manual verification
  - Run batch generation workflow manually
  - Verify failures include `blocked_by` when appropriate
  - Verify dashboard shows updated blocker data

## Testing Strategy

- **Unit tests**: Add test cases in `internal/dashboard/dashboard_test.go` for:
  - Per-recipe format with `blocked_by` field
  - Mixed file with both formats
  - Verify blockers map is correctly populated

- **Integration tests**: Not required - covered by existing CI workflow

- **Manual verification**:
  - Trigger batch-generate workflow with `workflow_dispatch`
  - Check generated failures in `data/failures/` for `blocked_by` field
  - Regenerate dashboard and verify blockers section shows current data

## Risks and Mitigations

- **Risk 1: Docker JSON capture complexity**: Capturing JSON stdout while still logging to GitHub is non-trivial
  - Mitigation: Use tee to write stdout to temp file while also outputting to console

- **Risk 2: Performance impact of `--json` parsing**: Adds jq processing per recipe
  - Mitigation: Only parse JSON when exit_code is 8 (missing_dep), skip for other failures

- **Risk 3: Schema validation failures for existing files**: Adding new schema may break validation
  - Mitigation: Use `oneOf` to allow both formats, or add separate per-recipe schema

## Success Criteria

- [ ] Per-recipe failure records include `blocked_by` field when `category: "missing_dep"`
- [ ] Dashboard blockers section shows data from recent batch runs (not stale January data)
- [ ] Existing tests pass (no regressions)
- [ ] New tests verify blocked_by extraction from per-recipe format

## Open Questions

None - the approach is clear and aligns with existing patterns in both the Go code and CI workflow.
