# Decision 3: Stats API and Dashboard for Update Reliability

## Options Analysis

### Option A: New `/stats/updates` endpoint

A dedicated endpoint mirrors the pattern set by `/stats/discovery`. The discovery endpoint has its own response type (`DiscoveryStatsResponse`), its own query function (`getDiscoveryStats`), and its own route handler. This separation keeps queries focused and avoids bloating the general `/stats` response.

- **API clarity**: High. Consumers know exactly where to find update reliability data.
- **Query efficiency**: Good. Queries filter on update-outcome action types only, avoiding scans across unrelated events.
- **Dashboard UX**: Clean. A dedicated "Update Reliability" section sits alongside the existing "Discovery Resolver" section, both fetched independently.
- **Backwards compatibility**: Full. The existing `/stats` response is untouched.
- **Pattern alignment**: Direct match with `/stats/discovery`.

### Option B: Extend existing `/stats` endpoint

The current `/stats` response already includes per-recipe update counts. Adding outcome breakdowns here would mean the `recipes` array gains nested fields like `update_success`, `update_failure`, `update_rollback`.

- **API clarity**: Lower. Mixes installation popularity data with reliability data, two different questions.
- **Query efficiency**: Worse. The existing `getStats` function runs three queries in parallel; adding outcome queries inflates response time for every `/stats` call, even when callers only want install counts.
- **Dashboard UX**: Cluttered. Update reliability metrics crammed into the recipe table dilute the current clean layout.
- **Backwards compatibility**: Breaking. Existing consumers parsing the `/stats` response may not expect new fields or the increased response size.
- **Pattern alignment**: Breaks the established pattern where each concern gets its own endpoint.

### Option C: Dual endpoints

`/stats/updates` handles aggregate reliability. `/stats` gets per-recipe outcome counts. This splits the same data across two endpoints with unclear ownership.

- **API clarity**: Confusing. Callers must decide which endpoint to use for what slice of update data.
- **Query efficiency**: Worst case. Both endpoints run update-related queries, potentially duplicating work.
- **Dashboard UX**: Adds complexity without clear benefit over Option A.
- **Backwards compatibility**: Still modifies `/stats`.
- **Pattern alignment**: Partial. The dedicated endpoint fits, but extending `/stats` does not.

## Recommendation: Option A

Create a dedicated `GET /stats/updates` endpoint. This follows the exact pattern established by `/stats/discovery`: separate response type, separate query function, separate route, fetched independently by the dashboard. The dashboard already demonstrates how to optionally fetch a second stats endpoint in parallel (see the `loadStats` function fetching both `/stats` and `/stats/discovery` with `Promise.all`).

If per-recipe outcome data is needed later, it belongs in `/stats/updates` as a `top_failing` list, not grafted onto the general `/stats` response.

## Proposed Endpoint Response Schema

```json
{
  "generated_at": "2026-04-01T12:00:00Z",
  "period": "all_time",
  "total_updates": 1482,
  "by_outcome": {
    "success": 1350,
    "failure": 87,
    "rollback": 45
  },
  "success_rate": 0.911,
  "by_trigger": {
    "auto": 980,
    "manual": 502
  },
  "by_error_type": [
    { "type": "download_failed", "count": 42 },
    { "type": "checksum_mismatch", "count": 23 },
    { "type": "version_resolve_failed", "count": 15 },
    { "type": "extract_failed", "count": 7 }
  ],
  "top_failing": [
    { "name": "terraform", "failures": 12, "total": 89 },
    { "name": "node", "failures": 8, "total": 145 }
  ]
}
```

Field rationale:
- `by_outcome` and `success_rate` answer "are updates working?" at a glance.
- `by_trigger` separates auto from manual, directly answering whether auto-updates are reliable.
- `by_error_type` (top 10, ordered by count) pinpoints what's breaking.
- `top_failing` (top 10 recipes by failure count) identifies which tools need attention.

## Proposed Dashboard Section

Add an "Update Reliability" section after the existing "Discovery Resolver" section. Layout:

**Overview cards** (same grid as existing overview):
- Total Updates (count)
- Success Rate (percentage, large font)
- Auto-Update Share (percentage of updates triggered automatically)

**Distribution cards** (reusing `distribution-grid` pattern):
- **Outcome Distribution**: horizontal bars for success/failure/rollback percentages
- **Trigger Breakdown**: horizontal bars for auto vs manual
- **Top Errors**: horizontal bars showing error type counts (like the "Top Missing Tools" card in discovery)
- **Top Failing Recipes**: horizontal bars showing failure counts per recipe

The dashboard fetches `/stats/updates` in the same `Promise.all` that already fetches `/stats` and `/stats/discovery`, with the same graceful fallback pattern (`.catch(() => null)`) so the section simply does not render if the endpoint is unavailable.

## Confidence

High. Option A is the clear winner. It matches the established pattern exactly, keeps concerns separated, maintains backwards compatibility, and the dashboard already has the infrastructure for adding optional parallel-fetched sections.
