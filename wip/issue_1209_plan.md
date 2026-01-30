# Issue 1209 Implementation Plan

## Summary

Add a scheduled GitHub Actions workflow that monitors checksum drift for recipes with `checksum_url` fields. The workflow downloads current checksum files from upstream, compares them against previously-recorded values, and creates security issues when drift is detected.

## Approach

The workflow uses a shell script to:
1. Find all recipe TOML files containing `checksum_url`
2. For each recipe, resolve the checksum URL template using the version from the recipe's version provider
3. Download the current checksum file from upstream
4. Compare against the checksum file stored in `data/checksums/` (a new directory for baseline snapshots)
5. If drift detected, create a GitHub issue with security + needs-triage labels

The approach is deliberately simple: store known-good checksum files as baseline, compare against upstream on each run. This avoids needing to build tsuku or parse complex TOML structures -- the script extracts `checksum_url` with grep/sed and resolves templates with the GitHub API for version resolution.

### Alternatives Considered

- **Using tsuku binary to verify**: Would require building the Go binary in CI. Adds complexity and build time. The workflow only needs to compare checksum files, not perform full installations.
- **Storing checksums in a database**: Over-engineered for 2-8 recipes with checksums. Flat files in the repo are simpler and auditable.

## Files to Create

- `.github/workflows/checksum-drift.yaml` - Scheduled workflow
- `scripts/check-checksum-drift.sh` - Main drift detection script

## Files to Modify

None.

## Implementation Steps

- [ ] Create `scripts/check-checksum-drift.sh` that:
  - Finds recipes with `checksum_url` fields
  - Extracts the checksum URL template and version info
  - Downloads the current checksum file from upstream
  - Compares against previously-stored checksums (if available)
  - Outputs drift results as structured data
- [ ] Create `.github/workflows/checksum-drift.yaml` with:
  - Schedule trigger (daily at 5 AM UTC)
  - Manual trigger via workflow_dispatch
  - Read-only default permissions, issues:write for creating alerts
  - Rate limiting between upstream requests (sleep between fetches)
  - Issue creation with `security` and `needs-triage` labels on drift
  - Graceful handling of fetch failures (warning, not error)

## Testing Strategy

- Validation script from the issue to verify workflow structure
- Manual trigger via workflow_dispatch to test end-to-end
- The script itself is testable locally by running against the recipe directory

## Risks and Mitigations

- **Template resolution complexity**: `{version}`, `{os}`, `{arch}` templates need resolution. Mitigation: use a hardcoded platform (linux/amd64) for monitoring since drift would affect all platforms equally.
- **Version resolution**: Need to know which version to check. Mitigation: use the GitHub API to get the latest release tag, which matches how most version providers work.
- **Rate limiting**: Only 2 recipes currently have checksum_url, so rate limiting is minimal concern now. Add 2-second sleep between fetches as a safeguard.

## Success Criteria

- [ ] Workflow file passes validation script from issue
- [ ] Script correctly identifies recipes with checksum_url
- [ ] Script downloads and compares checksum files
- [ ] Drift creates a GitHub issue with proper labels
- [ ] Fetch failures produce warnings, not false alerts
- [ ] Workflow uses read-only permissions except issues:write
