# Issue 1406 Plan

## Scope

Update npm, PyPI, and Cask builders to populate quality metadata in ProbeResult.
Add per-registry thresholds to QualityFilter for npm and pypi.
Cask is included per user request (originally #1408).

## Changes

### 1. npm builder (internal/builders/npm.go)
- Update Probe() to use fetchPackageInfo result
- Extract VersionCount from len(Versions)
- Extract HasRepository from Repository field
- Add separate download count fetch from npmjs downloads API
- Need npmDownloadsBaseURL field on NpmBuilder + constructor update

### 2. PyPI builder (internal/builders/pypi.go)
- Extend pypiPackageResponse with Releases field
- Update Probe() to use fetchPackageInfo result
- Extract VersionCount from len(Releases)
- Extract HasRepository from ProjectURLs (any URL present)
- Downloads left at 0 (PyPI returns -1)

### 3. Cask builder (internal/builders/cask.go)
- Update Probe() to use fetchCaskInfo result
- HasRepository = Homepage != "" (casks link to project homepages)
- No downloads/version count (Homebrew is curated, fail-open in filter)

### 4. QualityFilter (internal/discover/quality_filter.go)
- Add npm threshold: {MinDownloads: 100, MinVersionCount: 5}
- Add pypi threshold: {MinDownloads: 0, MinVersionCount: 3}
- Cask: no threshold (fail-open, curated registry)

### 5. Tests
- Update probe_test.go for npm, PyPI, Cask with metadata assertions
- Update quality_filter_test.go with npm/pypi threshold tests
- Update ecosystem_probe_test.go if needed
