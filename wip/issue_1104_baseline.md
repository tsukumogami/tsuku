# Issue 1104 Baseline

## Issue Summary
**Title**: ci: add R2 cost monitoring alerts
**Milestone**: M46 - R2 Golden Operations

## Pre-Implementation State

### Test Results
All tests passing (cached).

### Existing Integration Points
- `scripts/r2-health-check.sh` - R2 credential pattern reference
- `.github/workflows/r2-cleanup.yml` - R2 workflow patterns
- `.github/workflows/r2-health-monitor.yml` - Issue management patterns

### Files to Create
- `.github/workflows/r2-cost-monitoring.yml` - Weekly cost monitoring workflow

### Design Requirements
1. **Schedule**: Weekly (sufficient for cost monitoring)
2. **Metrics to track**:
   - Storage size (vs 10 GB free tier limit)
   - Object count (indicative of operations)
3. **Alert threshold**: 80% of free tier limits
4. **Issue management**:
   - Create issue when threshold exceeded
   - Include usage breakdown and recommendations

### R2 Free Tier Limits
- Storage: 10 GB/month
- Class A operations (PUT, LIST): 1 million/month
- Class B operations (GET, HEAD): 10 million/month

## Branch
`ci/1104-r2-cost-monitoring`

## Baseline Commit
Starting from main branch HEAD (includes merged #1103).
