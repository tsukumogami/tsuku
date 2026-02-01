# Issue 1349 Summary

## Changes Made

### .github/workflows/batch-generate.yml
- Added "Generate batch ID" step after aggregation that produces `BATCH_ID` in `{date}-{ecosystem}` format using UTC dates
- Sequence number (`-002`, `-003`) appended when same-day batches exist for the same ecosystem
- `BATCH_ID` exported to `$GITHUB_ENV` for downstream steps (#1351)
- Replaced generic commit message with structured message including git trailers: `batch_id`, `ecosystem`, `batch_size`, `success_rate`
- Success rate calculated as `INCLUDED_COUNT / TOTAL` with two decimal places via `awk`

## Acceptance Criteria Status
- [x] Batch ID generation with UTC dates
- [x] Sequence number for same-day batches
- [x] BATCH_ID exported to GITHUB_ENV
- [x] Structured commit message with all four trailers
- [x] Compatible with scripts/rollback-batch.sh (git grep for `batch_id:` trailer)

## Validation
All 7 checks from the issue validation script pass.
