# Issue 1406 Summary

## Changes

Populated quality metadata (Downloads, VersionCount, HasRepository) in npm, PyPI, and Cask `Probe()` methods. Added per-registry thresholds to `QualityFilter`.

### npm (`internal/builders/npm.go`)
- Added `npmDownloadsURL` field and `npmDownloadsResponse` struct
- `Probe()` extracts VersionCount from `len(Versions)`, HasRepository from Repository field
- Weekly downloads fetched from separate npm downloads API (non-fatal on failure)

### PyPI (`internal/builders/pypi.go`)
- Extended `pypiPackageResponse` with `Releases` field
- `Probe()` extracts VersionCount from `len(Releases)`, HasRepository from ProjectURLs

### Cask (`internal/builders/cask.go`)
- `Probe()` extracts HasRepository from Homepage field
- No thresholds (curated registry, fail-open)

### QualityFilter (`internal/discover/quality_filter.go`)
- Added thresholds: npm (100 downloads OR 5 versions), pypi (3 versions)
- Fixed zero-value guard: thresholds with value 0 are skipped

## Scope Note

Cask metadata was originally scoped for #1408 but included here per user request.
