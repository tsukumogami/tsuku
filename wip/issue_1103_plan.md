# Issue 1103 Implementation Plan

## Overview

Create `.github/workflows/r2-health-monitor.yml` - a scheduled workflow that monitors R2 health and manages GitHub issues on degradation/failure.

## Implementation Steps

### Step 1: Create workflow file

Create `.github/workflows/r2-health-monitor.yml` with:

1. **Triggers**:
   - Schedule: every 6 hours (`0 */6 * * *`)
   - Manual dispatch with optional force_issue input for testing

2. **Concurrency**: Prevent parallel runs with `concurrency: { group: r2-health-monitor, cancel-in-progress: false }`

3. **Jobs**:
   - `health-check`: Run health check and capture status/latency
   - `issue-management`: Create/update/resolve GitHub issues based on health status

### Step 2: Health Check Job

```yaml
health-check:
  runs-on: ubuntu-latest
  outputs:
    status: ${{ steps.check.outputs.status }}
    latency: ${{ steps.check.outputs.latency }}
  steps:
    - uses: actions/checkout@...
    - name: Run health check
      id: check
      env:
        R2_BUCKET_URL: ${{ secrets.R2_BUCKET_URL }}
        R2_ACCESS_KEY_ID: ${{ secrets.R2_ACCESS_KEY_ID_READONLY }}
        R2_SECRET_ACCESS_KEY: ${{ secrets.R2_SECRET_ACCESS_KEY_READONLY }}
      run: |
        # Capture exit code and output from r2-health-check.sh
        # Set outputs: status (healthy/degraded/failure), latency
```

### Step 3: Issue Management Job

Runs always after health-check. Logic:

1. **If failure or degraded**:
   - Search for existing open issue with `r2-degradation` label
   - If exists: add comment with new failure details
   - If not: create new issue

2. **If healthy**:
   - Search for existing open issue with `r2-degradation` label
   - If exists: add recovery comment and close issue
   - If not: exit silently (no noise)

### Step 4: Issue Format

**Issue title**: `R2 Storage Degradation Detected`

**Issue body**:
```markdown
## R2 Health Issue

**Status**: [failure|degraded]
**Detected**: [timestamp]
**Latency**: [X]ms (threshold: 2000ms)

### Details

[Failure-specific message]

### Workflow Run

[Link to workflow run]

---
*This issue was automatically created by the R2 health monitoring workflow.*
```

**Comment on subsequent failures**:
```markdown
## Health Check Update - [timestamp]

**Status**: [failure|degraded]
**Latency**: [X]ms

[Workflow link]
```

**Recovery comment**:
```markdown
## Service Recovered - [timestamp]

R2 health check passed. Closing issue.

**Latency**: [X]ms

[Workflow link]
```

## Testing Strategy

1. Workflow syntax validation via `gh workflow list`
2. Manual trigger to verify execution
3. Actual R2 health checks will run every 6 hours in production

## Files Changed

| File | Action |
|------|--------|
| `.github/workflows/r2-health-monitor.yml` | Create |
