# Pipeline Dashboard Architecture Review

**Date**: 2026-02-03
**Reviewer**: Architecture Review Agent
**Design Document**: docs/designs/DESIGN-pipeline-dashboard.md

## Executive Summary

The proposed architecture is sound and well-aligned with existing codebase patterns. The three-component design (shell script, JSON data file, HTML dashboard) is appropriate for the stated requirements. Key strengths include zero infrastructure dependencies, consistency with established patterns, and clear data flow. A few interface clarifications and implementation details warrant attention before development begins.

## Architecture Clarity Assessment

### Strengths

1. **Clear Component Boundaries**: The design cleanly separates data processing (shell script), data storage (JSON file), and presentation (HTML). Each component has a single responsibility.

2. **Pattern Consistency**: The approach mirrors existing patterns:
   - `scripts/batch-metrics.sh` and `scripts/gap-analysis.sh` demonstrate the shell+jq pattern works for similar data processing
   - `website/stats/index.html` proves the vanilla JS + fetch pattern works for dashboards
   - Both patterns are maintainable and familiar to contributors

3. **Data Flow Clarity**: The pipeline is linear and traceable:
   ```
   batch-generate.yml -> data files -> generate-dashboard.sh -> dashboard.json -> index.html
   ```

4. **Graceful Degradation**: The design handles missing data (e.g., `batch-runs.jsonl` not existing) explicitly.

### Areas Needing Clarification

1. **JSONL Parsing for Failures**: The `data/failures/homebrew.jsonl` file contains two record types:
   - Legacy batch failure records (with `ecosystem`, `environment`, `failures[]` array)
   - Per-recipe failure records (with `recipe`, `platform`, `exit_code`)

   The dashboard JSON schema shows `blockers[]` derived from `blocked_by` in failures, but this field only exists in the legacy batch records. The script needs to handle both record formats.

2. **Blocker Aggregation Logic**: The design shows `blockers[].packages` containing "First 5 blocked package names" but doesn't specify how to aggregate across multiple JSONL records with the same `blocked_by` dependency.

3. **Queue Tier Structure**: The `priority-queue.json` structure shows `tier` as an integer (1 or 2), but the dashboard JSON schema implies string keys for `by_tier`. This should be consistent.

## Interface Completeness

### Dashboard JSON Schema

The schema is mostly complete. Suggested additions:

| Field | Suggestion |
|-------|------------|
| `queue.by_tier[N].failed` | Currently missing - add for consistency with other statuses |
| `failures.other` | Consider a catch-all for uncategorized failures |
| `runs[].platforms` | Include per-platform breakdown from batch-runs.jsonl |

### Data Source Compatibility

| Source File | Schema Verified | Notes |
|------------|-----------------|-------|
| `data/priority-queue.json` | Yes | Contains `packages[]` with `id`, `tier`, `status` |
| `data/failures/homebrew.jsonl` | Partial | Mixed record formats - see clarification above |
| `data/metrics/batch-runs.jsonl` | Yes | Contains `batch_id`, `total`, `merged`, `platforms{}` |

## Implementation Phase Sequencing

The proposed phases are correctly sequenced:

1. **Phase 1 (Data Generation Script)** must come first since Phase 2 depends on the JSON output format.

2. **Phase 2 (HTML Dashboard)** can be developed against sample JSON, but benefits from having real data from Phase 1.

3. **Phase 3 (CI Integration)** is correctly placed last for new functionality. However, consider:
   - The commit step should be added to the `merge` job (after "Collect SLI metrics") rather than as a new step after "Persist circuit breaker state" as stated in the design. The design text says "after 'Persist circuit breaker state'" but that step only runs when `check.outputs.changes == 'false'`. The dashboard should be generated regardless.

4. **Phase 4 (GitHub Actions Summary)** is optional enhancement and correctly sequenced last.

### Suggested Phase Refinement

The current Phase 3 description is ambiguous about where the dashboard generation step goes. Recommend:

```yaml
# In merge job, after "Collect SLI metrics" and before "Check for recipes to merge":
- name: Generate dashboard
  run: ./scripts/generate-dashboard.sh
```

This ensures the dashboard is generated for every batch run, not just when there are no recipe changes.

## Simpler Alternatives Considered

### Alternative 1: GitHub Actions Artifacts Only

Instead of committing `dashboard.json` to the repo, publish it as a workflow artifact and fetch from the GitHub API.

**Pros**: No commits for data updates, cleaner git history
**Cons**: Requires API token for public access, artifacts expire, more complex deployment

**Assessment**: Rejected - current approach is simpler and integrates naturally with Cloudflare Pages deployment.

### Alternative 2: Single-File HTML with Embedded Data

Generate a complete HTML file with data embedded as a `<script>` tag JSON blob.

**Pros**: Single file to deploy, no CORS/fetch concerns
**Cons**: Larger HTML diffs, harder to test data generation separately from rendering

**Assessment**: Rejected - separation of data and presentation is cleaner for testing and caching.

### Alternative 3: Use Existing CLI Scripts

Extend `batch-metrics.sh` and `gap-analysis.sh` to output JSON and compose them.

**Pros**: Reuses existing code
**Cons**: Scripts have different input assumptions, would require refactoring

**Assessment**: Partially adopt - the new script can call existing scripts or share patterns, but a unified script for dashboard output is cleaner.

## Risk Assessment

### Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| jq processing slow on large files | Low | Low | priority-queue.json is ~8K lines; jq handles this in <1s |
| JSONL grows unbounded | Medium | Low | Monitor file size; add rotation when >1MB (per design) |
| Stale dashboard | Low | Low | Batch runs every 3 hours; max staleness is acceptable |

### Implementation Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| JSONL format ambiguity | High | Medium | Document expected record formats; add schema validation |
| CI step placement error | Medium | Medium | Test with workflow_dispatch before merging |

## Recommendations

1. **Clarify JSONL Processing**: Add a section to the design specifying which JSONL record types the script processes and how it handles the two different schemas in `homebrew.jsonl`.

2. **Fix Phase 3 Description**: Update the CI integration description to specify the dashboard step goes in the `merge` job before the "Check for recipes to merge" step.

3. **Add Validation**: Consider adding a simple JSON Schema file for `dashboard.json` to catch format drift during development.

4. **Document Failure Categories**: The `failures` object in the dashboard JSON lists specific categories. Map these explicitly to the source data:
   - `missing_dep` - from `category: "missing_dep"` in failure records
   - `validation_failed` - from `category: "validation_failed"`
   - `deterministic_insufficient` - from `category: "deterministic_insufficient"`
   - etc.

5. **Consider Idempotency**: The script should be safe to run multiple times. If `dashboard.json` already exists, regenerating it should produce the same output for the same input.

## Conclusion

The architecture is ready for implementation with minor clarifications. The design makes appropriate trade-offs for an intermediate solution: no infrastructure, quick delivery, consistent patterns. The four-panel layout directly addresses operator questions without over-engineering.

Proceed to implementation after:
- Clarifying JSONL record format handling
- Correcting the Phase 3 CI step placement description
- Adding explicit failure category mappings
