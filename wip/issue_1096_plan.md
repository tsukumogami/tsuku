# Issue 1096 Implementation Plan

## Overview

Integrate R2 golden file storage into nightly validation workflow with health check and graceful degradation.

## Approach

The workflow will be restructured to:
1. Run health check first to determine R2 availability
2. If R2 available, download golden files and run validation
3. If R2 unavailable, skip validation and create tracking issue

## Workflow Structure

```yaml
jobs:
  health-check:
    # Run r2-health-check.sh
    # Output: r2_available (true/false)

  download-golden-files:
    needs: health-check
    if: needs.health-check.outputs.r2_available == 'true'
    # Download all golden files from R2 to artifact

  validate-plans-linux:
    needs: [health-check, download-golden-files]
    if: needs.health-check.outputs.r2_available == 'true'
    # Download artifact, validate against R2 golden files

  validate-plans-macos:
    needs: [health-check, download-golden-files]
    if: needs.health-check.outputs.r2_available == 'true'
    # Download artifact, validate against R2 golden files

  execute-sample-linux:
    needs: [health-check, download-golden-files]
    if: needs.health-check.outputs.r2_available == 'true'
    # Download artifact, execute sample recipes

  create-skip-issue:
    needs: health-check
    if: needs.health-check.outputs.r2_available == 'false'
    # Create issue: "Nightly validation skipped - R2 unavailable"

  create-failure-issue:
    needs: [validate-plans-linux, validate-plans-macos, execute-sample-linux]
    if: failure()
    # Existing failure issue creation
```

## Implementation Steps

### Step 1: Add health-check job

- Use r2-health-check.sh script
- Set R2 credentials from secrets
- Output `r2_available` based on exit code (0=true, 1/2=false)

### Step 2: Add download-golden-files job

- Download all golden files from R2 to temp directory
- Use r2-download.sh for each file or AWS CLI for bulk download
- Upload as artifact for downstream jobs

### Step 3: Modify validation jobs

- Add `if: needs.health-check.outputs.r2_available == 'true'` condition
- Download golden files artifact
- Point validation to downloaded files instead of git

### Step 4: Add create-skip-issue job

- Triggered when R2 unavailable
- Create issue with label `r2-unavailable`
- Include run link and health check output

### Step 5: Pin all actions to SHA

- Update existing actions/checkout, actions/setup-go, etc.

## Key Decisions

1. **Bulk download vs per-file**: Use AWS CLI `s3 sync` for efficiency rather than per-file r2-download.sh
2. **Golden files location**: Download to temp directory, pass via artifact
3. **Skip issue label**: Use `r2-unavailable` label distinct from `nightly-failure`
4. **Degraded handling**: Treat degraded (exit 2) same as failure (exit 1) - skip validation

## Files to Modify

1. `.github/workflows/nightly-registry-validation.yml` - Main workflow changes
