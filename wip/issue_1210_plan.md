# Issue 1210 Implementation Plan

## Summary

Add a `POST /batch-metrics` endpoint to the telemetry worker that writes batch validation results to D1, then add a workflow step to `batch-operations.yml` that collects and uploads results after validation completes.

## Approach

The telemetry worker already has the D1 binding (`BATCH_METRICS`) configured in `wrangler.toml` and the schema migration exists. The implementation adds a bearer-token-authenticated endpoint that inserts into `batch_runs` and `recipe_results` tables in a single transaction, plus a workflow step that assembles a JSON payload from validation output and POSTs it.

### Alternatives Considered

- **Direct D1 access from GitHub Actions**: Rejected because the issue explicitly requires using the telemetry worker API endpoint, and direct D1 access would need Cloudflare credentials in CI rather than a simple bearer token.
- **Separate worker for batch metrics**: Rejected because the telemetry worker already has the D1 binding and serves as the single observability entry point. Adding a route is simpler than deploying and maintaining a second worker.

## Files to Modify

- `telemetry/src/index.ts` - Add `BATCH_METRICS` D1 binding to `Env` interface, add `BATCH_METRICS_TOKEN` secret, add `POST /batch-metrics` route handler with validation, D1 insert logic
- `telemetry/src/index.test.ts` - Add test suite for `POST /batch-metrics` endpoint (auth, validation, success, error handling)
- `telemetry/vitest.config.ts` - Add `BATCH_METRICS_TOKEN` test binding to miniflare config
- `.github/workflows/batch-operations.yml` - Add post-processing step that collects results and uploads to telemetry endpoint

## Files to Create

None.

## Implementation Steps

- [ ] Extend `Env` interface in `index.ts` with `BATCH_METRICS: D1Database` and `BATCH_METRICS_TOKEN: string`
- [ ] Define TypeScript interfaces for the batch metrics request payload (`BatchMetricsPayload` with batch run fields and `RecipeResult[]` array)
- [ ] Add `POST /batch-metrics` route in the fetch handler, gated by bearer token auth (`Authorization: Bearer <BATCH_METRICS_TOKEN>`)
- [ ] Implement request validation (required fields: batch_id, ecosystem, started_at, total_recipes, results array)
- [ ] Implement D1 insert logic: insert batch_runs row first, get the returned id, then insert recipe_results rows referencing that id
- [ ] Return 201 on success with the batch_run_id, 400 for validation errors, 401 for auth failures, 500 for D1 errors
- [ ] Add `BATCH_METRICS_TOKEN` to vitest miniflare bindings in `vitest.config.ts`
- [ ] Write tests: auth rejection (missing/invalid token), validation rejection (missing required fields), successful upload with mocked D1, failed D1 insert handling
- [ ] Add workflow step in `batch-operations.yml` after batch processing with `if: always()` that assembles JSON from validation output and POSTs to the telemetry endpoint using `BATCH_METRICS_TOKEN` secret
- [ ] Ensure the upload step uses `continue-on-error: true` so failures don't block the workflow

## Testing Strategy

- Unit tests: Test the `/batch-metrics` endpoint via vitest with cloudflare:test SELF bindings. Cover auth (401 without token, 401 with wrong token), validation (400 for missing batch_id, missing ecosystem, empty results), success path (201 with valid payload), and error handling (500 when D1 fails). D1 is available in the vitest-pool-workers test environment via the wrangler.toml binding.
- Manual verification: After deployment, confirm the endpoint rejects unauthenticated requests and accepts valid payloads by curling the worker URL with test data.

## Risks and Mitigations

- **D1 not available in test environment**: The vitest-pool-workers config reads wrangler.toml which declares the D1 binding, so miniflare should provide an in-memory D1 instance. If not, tests can mock the binding.
- **Workflow step depends on structured validation output**: The batch processing step currently outputs placeholder text. The upload step should handle missing/malformed output gracefully and skip upload with a warning rather than failing.
- **Token secret not yet configured**: The `BATCH_METRICS_TOKEN` secret must be set via `wrangler secret put` and added as a GitHub Actions secret. Document this in a code comment.

## Success Criteria

- [ ] `POST /batch-metrics` returns 401 without valid bearer token
- [ ] `POST /batch-metrics` returns 400 for invalid payloads (missing required fields)
- [ ] `POST /batch-metrics` returns 201 and inserts batch_runs + recipe_results rows for valid payloads
- [ ] Workflow step runs even when validation fails (`if: always()`)
- [ ] Failed upload does not block workflow completion (`continue-on-error: true`)
- [ ] All existing telemetry tests continue to pass
- [ ] New tests cover auth, validation, success, and error paths

## Open Questions

None blocking. The batch processing step currently outputs placeholder text; the upload step will need to be updated when real validation output is available, but it can be wired with the expected JSON structure now.
