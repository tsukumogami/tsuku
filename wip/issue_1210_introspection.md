# Issue 1210 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-batch-operations.md`
- Sibling issues reviewed: #1208 (closed, PR #1220 merged)
- Prior patterns identified: D1 binding added to `wrangler.toml` as `BATCH_METRICS`, migration file at `telemetry/migrations/0001_batch_metrics_schema.sql`

## Gap Analysis

### Minor Gaps

- The D1 binding name is `BATCH_METRICS` (per `wrangler.toml`). The workflow upload step should use this binding name when referencing the telemetry worker's D1 database. The issue doesn't mention the binding name but it's discoverable from `wrangler.toml`.
- The `batch_runs.id` column uses `INTEGER PRIMARY KEY AUTOINCREMENT` rather than `TEXT PRIMARY KEY` as the design doc suggested. The `recipe_results.batch_run_id` is `INTEGER` with a foreign key. The upload logic must use the autoincrement pattern (insert batch_run, get last_insert_rowid, use that for recipe_results).
- Column types for `macos_minutes`, `linux_minutes`, and `duration_seconds` are `REAL` in the actual schema (design doc showed `INTEGER`). Upload payload should use float values.

### Moderate Gaps

- **Telemetry worker has no batch metrics API endpoint.** The `Env` interface in `telemetry/src/index.ts` does not include the `BATCH_METRICS` D1 binding, and no route handles batch metrics uploads. The issue says "Upload uses telemetry worker API endpoint (not direct D1 access)" but this endpoint doesn't exist. This issue must either (a) add the endpoint to the telemetry worker as part of implementation, or (b) depend on a separate issue that adds it. Since no such issue exists in the milestone, option (a) is the implicit requirement.
- **No authentication specified for the batch metrics endpoint.** The existing `/event` endpoint is unauthenticated (public telemetry). A batch metrics upload endpoint accepting CI data should have some form of authentication (e.g., a shared secret in GitHub Actions secrets) to prevent unauthorized data injection. The issue and design doc don't specify this.

### Major Gaps

None.

## Recommendation

Amend

## Proposed Amendments

1. **Scope clarification**: This issue requires adding a new API endpoint (e.g., `POST /batch-metrics`) to the telemetry worker (`telemetry/src/index.ts`) that accepts batch run data and per-recipe results, then writes them to the D1 `BATCH_METRICS` database. The `Env` interface must be extended to include the D1 binding.
2. **Authentication**: The batch metrics endpoint should require a bearer token (e.g., `BATCH_METRICS_TOKEN` secret) to prevent unauthorized writes. The workflow step should pass this token from GitHub Actions secrets.
