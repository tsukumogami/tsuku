---
summary:
  constraints:
    - Must use read-only R2 credentials (no environment protection needed)
    - Health check exit codes: 0=healthy, 1=failure, 2=degraded
    - Two-tier degradation: R2 available → validate, R2 unavailable → skip + issue
    - Actions should be pinned to SHA for security
  integration_points:
    - scripts/r2-health-check.sh (exit codes 0/1/2)
    - scripts/r2-download.sh (download with checksum verification)
    - GitHub Secrets: R2_BUCKET_URL, R2_ACCESS_KEY_ID_READONLY, R2_SECRET_ACCESS_KEY_READONLY
    - Existing validate-all-golden.sh script
  risks:
    - R2 unavailability should not block development
    - Need to handle both degraded (slow) and failure (unreachable) cases
    - Issue creation for skipped validation must not duplicate issues
  approach_notes: |
    Modify nightly-registry-validation.yml to:
    1. Add health-check job that runs before validation
    2. Pass R2 status to downstream jobs via outputs
    3. Skip validation jobs when R2 unavailable
    4. Create "R2 unavailable" issue when validation skipped
    5. Keep existing failure-issue creation for validation failures
---

# Implementation Context: Issue #1096

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 3: Validation Integration)

## Key Design Excerpts

### Nightly Validation Flow

```
1. Run health check (HEAD request to health/ping.json)
   |
   +-- Success (HTTP 200, latency < 2000ms)
   |   |
   |   v
   |   Download golden files from R2
   |   |
   |   v
   |   Run validation against downloaded files
   |   |
   |   v
   |   Report results, create issue on validation failure
   |
   +-- Failure (timeout, error, or latency >= 2000ms)
       |
       v
       Skip validation
       |
       v
       Create GitHub issue: "Nightly validation skipped - R2 unavailable"
```

### Health Check Contract

- Endpoint: HEAD request to `health/ping.json`
- Timeout: 5 seconds
- Success: HTTP 200 with latency < 2000ms
- Degraded: HTTP 200 with latency >= 2000ms
- Failure: Any other response or timeout

### Credentials

- Read-only tokens used for validation workflows (no environment protection needed)
- R2_BUCKET_URL, R2_ACCESS_KEY_ID_READONLY, R2_SECRET_ACCESS_KEY_READONLY
