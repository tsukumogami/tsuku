---
summary:
  constraints:
    - Health check contract: HEAD request to health/ping.json with 5s timeout
    - Success threshold: HTTP 200 with latency < 2000ms
    - Degraded threshold: HTTP 200 with latency >= 2000ms
    - Failure: Any other response or timeout
    - R2 99.99% uptime SLA (~52 min/year downtime expected)
  integration_points:
    - scripts/r2-health-check.sh - existing health check script
    - .github/workflows/ - new r2-health-monitor.yml workflow
    - GitHub Issues API - for creating/updating r2-degradation issues
  risks:
    - Issue spam if health checks flap (mitigated by update-existing logic)
    - False positives from transient network issues
    - Rate limiting on GitHub API for issue operations
  approach_notes: |
    Create a scheduled workflow that runs every 6 hours (0 */6 * * *).
    Uses existing r2-health-check.sh script for the actual health check.
    On failure/degradation: create or update a GitHub issue with r2-degradation label.
    On recovery: add a comment noting service restored.
    Exit cleanly on success (no noise).
---

# Implementation Context: Issue #1103

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 6: Monitoring)

## Key Design Requirements

1. **Schedule**: Run every 6 hours (4 times daily)
2. **Health check**: Use existing r2-health-check.sh script
3. **Issue management**:
   - Create new issue on first failure with `r2-degradation` label
   - Update existing open issue with comment on subsequent failures
   - Add recovery comment when service is restored
4. **Exit behavior**: No issue noise when healthy

## Existing Integration Points

- `scripts/r2-health-check.sh` - Returns exit codes:
  - 0: Success (HTTP 200, latency < 2000ms)
  - 1: Failure (timeout, error, or non-200)
  - 2: Degraded (HTTP 200, latency >= 2000ms)

## Files to Create

1. `.github/workflows/r2-health-monitor.yml` - Scheduled health monitoring workflow
