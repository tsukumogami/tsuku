# Summary: Adopt Shirabe Reusable Release Workflows

## Changes

### New Files
- `.github/workflows/prepare-release.yml` — workflow_dispatch caller for shirabe's release workflow
- `.release/set-version.sh` — hook script stamping Cargo.toml version fields
- `.github/workflows/finalize.yml` — bridge promoting draft release after builds complete

### Unchanged
- `.github/workflows/release.yml` — existing tag-triggered build pipeline

## Version Surface Audit

All version injection points are now managed:
- **Go binary**: goreleaser ldflags from git tag (automatic, no file stamping)
- **cmd/tsuku-dltest/Cargo.toml**: managed by `.release/set-version.sh`
- **tsuku-llm/Cargo.toml**: managed by `.release/set-version.sh`

No other version-bearing files found (no package.json, no hardcoded version strings).

## Testing
- `set-version.sh` tested with release (`0.6.2`) and dev (`0.6.3-dev`) versions
- YAML syntax validated for both new workflows
- 48/48 Go test packages pass (no regressions)
- Existing release.yml confirmed unchanged via git diff
