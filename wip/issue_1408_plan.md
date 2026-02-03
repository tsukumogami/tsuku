# Issue 1408 Implementation Plan

## Overview

Add quality metadata to CPAN and Cask builders for the Probe Quality Filtering feature.

## Changes Summary

### 1. CPAN Builder (`internal/builders/cpan.go`)

**Goal**: Fetch river metrics (downstream dependency count) from MetaCPAN to populate quality metadata.

**API Endpoint**: `GET /v1/distribution/{name}`
- Response includes `river.total` (downstream dependency count)
- Response includes `resources.repository.url` (repository URL)

**Changes**:
1. Add `metacpanDistribution` struct for the distribution endpoint response
2. Add `fetchDistributionMetrics(ctx, name) (riverTotal int, repoURL string)` method
   - Fetches `/v1/distribution/{name}`
   - Returns river.total and repository URL
   - Returns (0, "") on any error (graceful degradation)
3. Update `Probe()` to:
   - Call `fetchDistributionMetrics()` after successful distribution lookup
   - Set `result.Downloads` to river.total value
   - Set `result.HasRepository` based on repository URL presence

### 2. Cask Builder (`internal/builders/cask.go`)

**Goal**: Handle deprecated/disabled flags and reject disabled casks.

**Changes**:
1. Add `Deprecated bool` and `Disabled bool` fields to `caskAPIResponse` struct
2. Update `Probe()` to:
   - Check `info.Disabled` after successful fetch
   - Return `nil, nil` if disabled (cask is rejected)
   - Continue with existing logic if not disabled

**Note**: Cask remains exempt from quality thresholds (no entry in QualityFilter). This is implicit exemption - builders without configured thresholds pass through.

### 3. Quality Filter (`internal/discover/quality_filter.go`)

**Goal**: Add CPAN threshold configuration.

**Changes**:
1. Add CPAN threshold to `NewQualityFilter()`:
   ```go
   "cpan": {MinDownloads: 1, MinVersionCount: 3}
   ```
   - MinDownloads represents river_total (downstream dependencies)
   - MinVersionCount is 3 per design doc specification

### 4. Tests

**CPAN Tests** (`internal/builders/cpan_test.go`):
1. `TestCPANBuilder_Probe_ReturnsQualityMetadata` - successful probe with river metrics
2. `TestCPANBuilder_Probe_GracefulDegradation` - distribution endpoint fails, returns 0
3. `TestCPANBuilder_Probe_NotFound` - distribution not found, returns nil

**Cask Tests** (`internal/builders/cask_test.go`):
1. `TestCaskBuilder_Probe_ReturnsQualityMetadata` - successful probe returns metadata
2. `TestCaskBuilder_Probe_DisabledCask` - disabled cask returns nil
3. `TestCaskBuilder_Probe_NotFound` - cask not found, returns nil

**Quality Filter Tests** (`internal/discover/quality_filter_test.go`):
1. Add test case for CPAN threshold verification

## Implementation Order

1. Add CPAN distribution struct and `fetchDistributionMetrics()` method
2. Update CPAN `Probe()` to use new method
3. Add CPAN tests
4. Add Deprecated/Disabled fields to caskAPIResponse
5. Update Cask `Probe()` to check disabled flag
6. Add Cask tests
7. Add CPAN threshold to QualityFilter
8. Add QualityFilter test for CPAN
9. Run full test suite

## MetaCPAN API Response Structure

Distribution endpoint (`/v1/distribution/{name}`):
```json
{
  "name": "App-Ack",
  "river": {
    "total": 42,
    "bucket": "5",
    "immediate": 10
  },
  "resources": {
    "repository": {
      "url": "https://github.com/beyondgrep/ack3",
      "type": "git"
    }
  }
}
```

## Homebrew Cask API Response Structure

Cask endpoint (`/api/cask/{name}.json`):
```json
{
  "token": "visual-studio-code",
  "version": "1.96.4",
  "deprecated": false,
  "disabled": false,
  ...
}
```

## Risk Assessment

- **Low risk**: Changes are additive and follow established patterns from sibling issues
- **Graceful degradation**: All secondary fetches return zero/false on failure
- **No breaking changes**: Existing functionality preserved
