# Implementation Context: Issue #928

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Information

- **Title**: ci(golden): re-enable validate-golden-code.yml workflow
- **Tier**: critical
- **Dependencies**: #927 (closed - updated validate-golden.sh)

## Key Implementation Requirements

From the design document, Phase 7 (Re-enable CI Workflow):

1. Remove `if: false` from `validate-golden-code.yml`
2. Update workflow to use `--pin-from` flag (already done in #927)
3. Monitor for a few PRs to confirm stability

## Current Workflow State

The workflow at `.github/workflows/validate-golden-code.yml` has:
- Line 67: `if: false` - disabling the `validate-all` job

## Implementation

Simply remove the `if: false` line to re-enable the workflow.

## Exit Criteria

- `validate-golden-code.yml` workflow is enabled
- CI job runs on relevant file changes
- No immediate failures on clean codebase
