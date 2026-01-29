# Issue 1208 Implementation Plan

## Summary
Create a D1 migration file with the batch metrics schema (two tables) and add D1 binding to wrangler.toml, enabling the telemetry worker to persist batch validation results.

## Approach
Add a single SQL migration file under `telemetry/migrations/` and update `wrangler.toml` with the D1 database binding. This is the standard Cloudflare D1 workflow -- migrations are applied via `wrangler d1 migrations apply`. The database itself is created manually via `wrangler d1 create` (a one-time operation outside of code).

### Alternatives Considered
- **Embed schema in worker code**: Would require the worker to check/create tables at runtime. Fragile, harder to version, and doesn't follow D1 conventions.
- **Use Analytics Engine instead of D1**: Already in use for CLI telemetry events, but Analytics Engine lacks relational queries and foreign keys needed for batch-to-recipe relationships.

## Files to Modify
- `telemetry/wrangler.toml` - Add D1 database binding configuration

## Files to Create
- `telemetry/migrations/0001_batch_metrics_schema.sql` - Initial schema migration with batch_runs and recipe_results tables

## Implementation Steps
- [x] Create `telemetry/migrations/` directory
- [x] Create `telemetry/migrations/0001_batch_metrics_schema.sql` with CREATE TABLE statements for `batch_runs` and `recipe_results`, including foreign key constraint and indexes
- [x] Add `[[d1_databases]]` binding to `telemetry/wrangler.toml` referencing the D1 database name and binding name
- [x] Verify SQL syntax is valid by reviewing against D1/SQLite constraints (e.g., no unsupported column types)

## Testing Strategy
- Unit tests: Not applicable (pure SQL schema, no Go/TS logic changes)
- Manual verification: Run `wrangler d1 migrations apply tsuku-batch-metrics --local` to apply migrations locally, then query `SELECT name FROM sqlite_master WHERE type='table'` to confirm both tables exist
- Verify existing telemetry tests still pass (`npm test` in telemetry/)

## Risks and Mitigations
- **D1 database not yet created in Cloudflare account**: Migration file can be committed first; actual `wrangler d1 create` is a manual step documented in acceptance criteria. The binding in wrangler.toml will use a placeholder database_id until the real one is provisioned.
- **SQLite FK enforcement**: D1 uses SQLite which doesn't enforce foreign keys by default. Add `PRAGMA foreign_keys = ON;` at the top of the migration or document that the application layer should enable it.

## Success Criteria
- [ ] `telemetry/migrations/0001_batch_metrics_schema.sql` exists with valid SQL creating both tables
- [ ] `batch_runs` table has all specified columns: id, batch_id, ecosystem, started_at, completed_at, total_recipes, passed, failed, skipped, success_rate, macos_minutes, linux_minutes
- [ ] `recipe_results` table has all specified columns: id, batch_run_id, recipe_name, ecosystem, result, error_category, error_message, duration_seconds
- [ ] Foreign key from recipe_results.batch_run_id to batch_runs.id is defined
- [ ] `telemetry/wrangler.toml` contains a `[[d1_databases]]` section with binding name
- [ ] Existing telemetry tests pass without changes

## Open Questions
None.
