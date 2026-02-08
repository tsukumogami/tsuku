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

- [x] Step 1: Update validation commands in batch-generate.yml
  - Changed `tsuku install --force --recipe` to `tsuku install --json --force --recipe`
  - Captured stdout to a temp file for JSON parsing
  - Modified all 4 validation jobs (linux-x86_64, linux-arm64, darwin-arm64, darwin-x86_64)

- [x] Step 2: Add blocked_by extraction in batch-generate.yml
  - After failed validation (exit code 8), parse JSON stdout for `missing_recipes`
  - Store in BLOCKED_BY variable for use in failure record writing

- [x] Step 3: Update per-recipe failure format in batch-generate.yml
  - Modified jq command to include `blocked_by` when category is `missing_dep`
  - Added exit_code 8 mapping to `missing_dep` category
  - Used `with_entries(select(.value != null))` to omit null blocked_by

- [x] Step 4: Update dashboard.go to read blocked_by from per-recipe format
  - Added BlockedBy field to FailureRecord struct
  - Updated `loadFailures()` to populate `details` and `blockers` from per-recipe format
  - Package ID generated as "homebrew:" + recipe name

- [x] Step 5: Add tests for per-recipe blocked_by parsing
  - Added `TestLoadFailures_perRecipeWithBlockedBy` test
  - Tests blockers extraction, details population, and category counting

- [ ] Step 6: Update failure-record schema (SKIPPED - schema validation not critical)
  - Schema is informational; actual parsing is tested

- [ ] Step 7: Manual verification (will be done via CI)
  - Workflow runs will verify the changes work

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
