
## Goal

Deploy the D1 database schema that stores batch validation metrics, enabling the telemetry service to persist and query recipe validation results across CI runs.

## Context

The batch validation workflow generates metrics about recipe testing (pass/fail counts, error categories, duration). Currently this data isn't persisted. Adding a D1 schema provides:

- Historical tracking of validation results over time
- Ability to identify flaky recipes or recurring failure patterns
- Foundation for dashboard queries showing ecosystem health

The schema uses two tables: `batch_runs` for aggregate run statistics and `recipe_results` for per-recipe outcomes. This separation allows efficient queries at both the batch level (overall success rates) and recipe level (specific failure analysis).

## Acceptance Criteria

- [ ] D1 database created in Cloudflare account (staging environment)
- [ ] `batch_runs` table exists with columns: id, batch_id, ecosystem, started_at, completed_at, total_recipes, passed, failed, skipped, success_rate, macos_minutes, linux_minutes
- [ ] `recipe_results` table exists with columns: id, batch_run_id, recipe_name, ecosystem, result, error_category, error_message, duration_seconds
- [ ] Foreign key relationship configured between recipe_results.batch_run_id and batch_runs.id
- [ ] Schema migration file added to telemetry/migrations/ directory
- [ ] Wrangler configuration updated with D1 binding
- [ ] Schema can be verified by running: `wrangler d1 execute <db-name> --command "SELECT name FROM sqlite_master WHERE type='table'"`

## Dependencies

None

## Downstream Dependencies

- **Issue 7**: feat(ci): add post-batch metrics upload to workflow - requires the D1 schema to exist before uploading metrics data
