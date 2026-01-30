# Issue 1202 Implementation Plan

## Summary

Create a bash script at `scripts/seed-queue.sh` that fetches Homebrew formula metadata and analytics data, assigns tiers based on download counts and a hardcoded curation list, and outputs a conformant `data/priority-queue.json` file.

## Approach

The script follows existing bash script patterns in the codebase (validate-queue.sh, validate-failures.sh) using:
- Standard bash with `set -euo pipefail`
- `curl` for API fetching with retry/backoff for rate limits
- `jq` for JSON processing (already used in validation test)
- Progress output to stderr
- Exit codes for success/failure states

The tier assignment logic uses:
1. **Tier 1**: Hardcoded curation list of top 20-100 high-impact tools (from DESIGN-registry-scale-strategy.md: ripgrep, jq, bat, fd, terraform, kubectl, etc.)
2. **Tier 2**: Packages with >10K weekly downloads (from analytics API)
3. **Tier 3**: Everything else

This approach was chosen because it directly implements the design specification and leverages existing patterns in the codebase.

### Alternatives Considered

- **Python script with requests library**: More powerful JSON handling, but introduces Python dependency. Rejected because bash+jq is already used extensively in the codebase and CI.
- **Go tool**: Better type safety and error handling, but overkill for a simple data transformation script. Rejected to maintain consistency with existing validation scripts.
- **No retry logic**: Simpler implementation, but fragile against transient API failures. Rejected because the issue explicitly requires "handles API rate limits gracefully (retry with backoff)".

## Files to Create

- `scripts/seed-queue.sh` - Main seed script with curl+jq processing, retry logic, and tier assignment

## Files to Modify

None - this is a standalone script that outputs to `data/priority-queue.json` (which may or may not exist yet).

## Implementation Steps

- [ ] Create script skeleton with shebang, usage docs, and argument parsing (--source, --limit)
- [ ] Add validation for required arguments and dependencies (curl, jq)
- [ ] Implement Homebrew API fetch function with retry/backoff logic
- [ ] Define tier 1 curation list as hardcoded array (based on DESIGN-registry-scale-strategy.md examples)
- [ ] Fetch formula list from https://formulae.brew.sh/api/formula.json
- [ ] Fetch analytics from https://formulae.brew.sh/api/analytics/install-on-request/30d.json
- [ ] Merge formula metadata with analytics data using jq
- [ ] Assign tiers based on curation list (tier 1), >10K downloads (tier 2), or default (tier 3)
- [ ] Build conformant JSON output with schema_version, updated_at, tiers descriptions, packages array
- [ ] Write output to data/priority-queue.json with proper package structure (id, source, name, tier, status, added_at, metadata)
- [ ] Add progress output to stderr at key stages (fetching, processing, writing)
- [ ] Make script executable (chmod +x)
- [ ] Test with --limit 10 to verify output structure

## Testing Strategy

- **Unit validation**: The issue includes a comprehensive validation script that tests:
  - Script is executable
  - Output file is created
  - Valid JSON structure
  - Required fields present (name, tier, source)
  - Tier values are 1-3
  - Source is "homebrew"
  - Limit flag is respected

- **Integration tests**:
  - Run script with --limit 10 and validate against schema using scripts/validate-queue.sh
  - Test retry logic by simulating API failures (mock curl response)
  - Verify tier assignments by checking known packages (e.g., ripgrep should be tier 1)

- **Manual verification**:
  - Run without limit to see realistic queue size
  - Inspect output to confirm tier distribution is reasonable
  - Check stderr output for progress messages

## Risks and Mitigations

- **Homebrew API schema changes**: The formula.json and analytics API formats could change without notice.
  - *Mitigation*: Use defensive jq queries with `// empty` fallbacks. Document expected API structure in script comments.

- **Large API responses**: formula.json contains all 8K+ formulas; slow to fetch/process.
  - *Mitigation*: Use `curl -s` to suppress progress bar, add stderr progress messages, and implement --limit flag for testing.

- **Rate limiting**: Homebrew may throttle requests.
  - *Mitigation*: Implement exponential backoff retry logic (3 attempts with 1s, 2s, 4s delays).

- **Tier 1 curation list maintenance**: Hardcoded list will need updates over time.
  - *Mitigation*: Document list source in script comments (DESIGN-registry-scale-strategy.md), use array structure for easy updates.

- **ID pattern compliance**: The schema requires `id` to match `^[a-z0-9_-]+:[a-z0-9_@.+-]+$`.
  - *Mitigation*: Use jq to construct ID as "homebrew:" + lowercase formula name, validate format.

## Success Criteria

- [ ] Script runs successfully with --source homebrew --limit 10
- [ ] Output passes validation in issue acceptance criteria (all assertions pass)
- [ ] Output conforms to data/schemas/priority-queue.schema.json (validated by scripts/validate-queue.sh)
- [ ] Retry logic handles API failures (tested with forced failure)
- [ ] Progress output appears on stderr during execution
- [ ] Tier assignments are correct: tier 1 formulas are from curation list, tier 2 have >10K downloads
- [ ] Script is executable without elevated privileges
- [ ] No external dependencies beyond curl, jq, and standard bash tools

## Open Questions

None - all requirements are clear from the issue and design docs.
