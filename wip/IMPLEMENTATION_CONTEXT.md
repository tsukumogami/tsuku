---
summary:
  constraints:
    - R2 Free Tier Limits:
      - Storage: 10 GB/month
      - Class A operations (PUT, LIST): 1 million/month
      - Class B operations (GET, HEAD): 10 million/month
    - Cost thresholds: $50/month acceptable, $100/month triggers reconsideration
    - Alert threshold: 80% of free tier limits
  integration_points:
    - .github/workflows/ - new r2-cost-monitoring.yml workflow
    - Cloudflare R2 API - for usage metrics
    - GitHub Issues API - for creating usage alert issues
  risks:
    - R2 API may not expose all usage metrics directly (may need Cloudflare API)
    - Historical tracking requires persistent storage mechanism
  approach_notes: |
    Create a scheduled workflow that runs weekly to check R2 usage metrics.
    Uses S3-compatible API to list objects and calculate storage size.
    Alerts when approaching 80% of free tier limits.
    Creates GitHub issue with usage breakdown and recommendations.
---

# Implementation Context: Issue #1104

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 6: Monitoring)

## Key Design Requirements

1. **Schedule**: Weekly run (sufficient for cost monitoring)
2. **Metrics to track**:
   - Storage size (vs 10 GB limit)
   - Object count (for estimating Class A/B operations)
3. **Alert threshold**: 80% of free tier limits
4. **Issue management**:
   - Create issue when threshold exceeded
   - Include usage breakdown and trend
   - Include actionable recommendations

## R2 Usage Metrics Approach

Cloudflare R2 doesn't expose billing metrics via S3 API. Options:
1. **Calculate storage from object listing**: Use `aws s3api list-objects-v2` to sum object sizes
2. **Use Cloudflare API**: Would require additional token with account analytics access

For simplicity, calculate storage from object listing which is already available with existing credentials.

## Existing Integration Points

- `scripts/r2-health-check.sh` - Uses R2 credentials pattern
- `.github/workflows/r2-cleanup.yml` - Reference for R2 workflow patterns
- `.github/workflows/r2-health-monitor.yml` - Reference for issue management

## Files to Create

1. `.github/workflows/r2-cost-monitoring.yml` - Weekly cost monitoring workflow
