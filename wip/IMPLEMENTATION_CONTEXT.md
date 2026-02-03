---
summary:
  constraints:
    - MetaCPAN uses river metrics (downstream dependency counts) instead of downloads
    - Cask is exempt from quality thresholds (curated by Homebrew maintainers)
    - Disabled casks should be rejected immediately in Probe()
    - Secondary API calls must run in parallel with shared context timeout
    - If secondary fetch fails, field stays at zero (graceful degradation)
  integration_points:
    - internal/builders/cpan.go - MetaCPAN Probe() needs parallel distribution fetch
    - internal/builders/cask.go - Cask Probe() needs deprecated/disabled flag handling
    - internal/discover/quality_filter.go - CPAN thresholds (river_total >= 1 OR version_count >= 3)
  risks:
    - MetaCPAN /v1/distribution/{name} endpoint may have different response format than expected
    - Cask deprecated vs disabled semantics need to be understood correctly
    - Parallel fetch timing might affect test reliability
  approach_notes: |
    1. MetaCPAN: Add parallel fetch to /v1/distribution/{name} for river.total, parse
       repository from existing release endpoint. Threshold: river_total >= 1 OR version_count >= 3
    2. Cask: Parse deprecated/disabled flags, return nil for disabled casks,
       set HasRepository from homepage. Cask is exempt from quality thresholds.
    3. Both builders follow the parallel fetch pattern established in npm, Gem, Go builders.
---

# Implementation Context: Issue #1408

**Source**: docs/designs/DESIGN-probe-quality-filtering.md

## Key Design Points for #1408

### MetaCPAN Builder
- `/v1/distribution/{name}` endpoint provides `river.total` (downstream dependency count)
- River metrics serve as quality signal since MetaCPAN has no download counts
- Parse `repository` object from existing release endpoint for HasRepository
- If distribution fetch fails, set Downloads to 0 (filter falls back to version count)
- Threshold: `river_total >= 1` OR `version_count >= 3`

### Cask Builder
- Exempt from quality thresholds (curated by Homebrew maintainers)
- Must check `deprecated` and `disabled` boolean flags
- Return nil if `disabled` is true (reject immediately in Probe)
- Populate HasRepository from `homepage` field
- Already done in #1406: Cask and Homebrew formula metadata

### Parallel Fetch Pattern
- Secondary call runs in goroutine with shared context timeout
- If secondary call fails, missing field stays at zero value
- Filter falls back to available signals
