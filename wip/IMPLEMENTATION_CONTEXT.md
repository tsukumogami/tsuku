---
summary:
  constraints:
    - Read-only permissions by default, issues:write only for creating alerts
    - Schedule-only trigger (no external PR trigger)
    - Rate limiting on upstream API calls
    - Must handle fetch failures gracefully (no false alerts)
  integration_points:
    - recipes/*.toml - checksum_url and checksum fields
    - .github/workflows/checksum-drift.yaml - new workflow file
  risks:
    - Only 8 recipes currently have checksums, so the scope is limited
    - checksum_url patterns need URL templating ({version}, {os}, {arch})
    - Some checksums are dynamic ({version.checksum}) from version providers
  approach_notes: |
    Shell script workflow that iterates recipes with checksums, downloads current
    checksums from upstream, and compares against recorded values. Creates GitHub
    issues on drift detection. Two patterns: checksum_url (HashiCorp SHA256SUMS files)
    and inline checksum (static hash in TOML).
---
