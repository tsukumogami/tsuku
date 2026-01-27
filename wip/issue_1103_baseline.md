# Issue 1103 Baseline

## Issue Summary
**Title**: ci: add R2 health monitoring workflow
**Milestone**: M31 - R2 Golden Storage

## Pre-Implementation State

### Test Results
All tests passing (cached from previous run).

### Existing Integration Points
- `scripts/r2-health-check.sh` - Health check script (exists, returns exit codes 0/1/2)
- `.github/workflows/r2-cleanup.yml` - Reference for R2 workflow patterns

### Files to Create
- `.github/workflows/r2-health-monitor.yml` - Scheduled health monitoring workflow

### Design Requirements
1. **Schedule**: Run every 6 hours (`0 */6 * * *`)
2. **Health check**: Use existing `scripts/r2-health-check.sh`
3. **Exit codes from health check**:
   - 0: Success (HTTP 200, latency < 2000ms)
   - 1: Failure (timeout, error, or non-200)
   - 2: Degraded (HTTP 200, latency >= 2000ms)
4. **Issue management**:
   - Create new issue on first failure with `r2-degradation` label
   - Update existing open issue with comment on subsequent failures
   - Add recovery comment when service is restored
5. **Exit behavior**: No issue noise when healthy

## Branch
`ci/1103-r2-health-monitoring`

## Baseline Commit
Starting from main branch HEAD.
