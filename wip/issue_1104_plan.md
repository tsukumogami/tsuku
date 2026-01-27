# Issue 1104 Implementation Plan

## Overview

Create `.github/workflows/r2-cost-monitoring.yml` - a scheduled workflow that monitors R2 storage usage and creates GitHub issues when approaching free tier limits.

## Implementation Steps

### Step 1: Create workflow file

Create `.github/workflows/r2-cost-monitoring.yml` with:

1. **Triggers**:
   - Schedule: weekly on Monday at 6 AM UTC (`0 6 * * 1`)
   - Manual dispatch for immediate check

2. **Concurrency**: Prevent parallel runs with `concurrency: { group: r2-cost-monitoring, cancel-in-progress: false }`

3. **Jobs**:
   - `usage-check`: Calculate storage metrics from R2
   - `alert`: Create/update GitHub issue if threshold exceeded

### Step 2: Usage Check Job

Calculate storage metrics by listing all objects:

```yaml
usage-check:
  runs-on: ubuntu-latest
  outputs:
    storage_bytes: ${{ steps.metrics.outputs.storage_bytes }}
    storage_gb: ${{ steps.metrics.outputs.storage_gb }}
    object_count: ${{ steps.metrics.outputs.object_count }}
    storage_percent: ${{ steps.metrics.outputs.storage_percent }}
    threshold_exceeded: ${{ steps.metrics.outputs.threshold_exceeded }}
  steps:
    - uses: actions/checkout@...
    - name: Calculate R2 usage
      id: metrics
      env:
        R2_BUCKET_URL: ${{ secrets.R2_BUCKET_URL }}
        R2_ACCESS_KEY_ID: ${{ secrets.R2_ACCESS_KEY_ID_READONLY }}
        R2_SECRET_ACCESS_KEY: ${{ secrets.R2_SECRET_ACCESS_KEY_READONLY }}
      run: |
        # List all objects and sum sizes
        # Calculate percentage of 10 GB limit
        # Set threshold_exceeded if >= 80%
```

### Step 3: Alert Job

Runs if threshold is exceeded (80% of 10 GB = 8 GB):

1. **Create or update issue**:
   - Search for existing open issue with `r2-cost-alert` label
   - If exists: add comment with updated metrics
   - If not: create new issue

2. **Issue content**:
   - Current storage usage (GB and %)
   - Object count
   - Trend (compared to previous if available)
   - Recommendations (cleanup, retention changes)

### Step 4: Issue Format

**Issue title**: `R2 Storage Usage Alert`

**Issue body**:
```markdown
## R2 Storage Usage Alert

**Current Usage**: X.XX GB / 10 GB (XX%)
**Object Count**: N files
**Checked**: [timestamp]

### Thresholds

| Metric | Current | Limit | Status |
|--------|---------|-------|--------|
| Storage | X.XX GB | 10 GB | ⚠️ XX% |

### Recommendations

1. Run cleanup workflow to remove orphaned/excess files
2. Review version retention policy (currently 2 versions/recipe)
3. Consider archiving or removing unused recipes

### Workflow Run

[View workflow run]([link])

---
*This issue was automatically created by the R2 cost monitoring workflow.*
```

### Step 5: Logging for Historical Tracking

Add GitHub Actions job summary with metrics:

```markdown
## R2 Usage Report - [date]

| Metric | Value |
|--------|-------|
| Storage | X.XX GB (XX%) |
| Objects | N files |
| Threshold | 80% (8 GB) |

Status: ✅ Within limits / ⚠️ Threshold exceeded
```

## Testing Strategy

1. Workflow syntax validation via YAML parse
2. Manual trigger to verify execution
3. Validation script from issue acceptance criteria

## Files Changed

| File | Action |
|------|--------|
| `.github/workflows/r2-cost-monitoring.yml` | Create |
