---
summary:
  constraints:
    - ProbeResult is in builders package (not RegistryEntry due to import cycle from #1405)
    - npm downloads require separate API call to api.npmjs.org
    - PyPI standard API returns -1 for downloads; rely on version count
    - User asked to include Homebrew/Cask builder metadata in this PR too
  integration_points:
    - internal/builders/npm.go - NpmBuilder.Probe()
    - internal/builders/pypi.go - PyPIBuilder.Probe()
    - internal/builders/cask.go - CaskBuilder.Probe() (user request)
    - internal/discover/quality_filter.go - add npm/PyPI/cask thresholds
    - internal/builders/probe_test.go - update tests
  risks:
    - npm downloads API may have different base URL than registry API
    - PyPI releases dict parsing needs response struct extension
  approach_notes: |
    Extend npm, PyPI, and Cask Probe() methods to populate quality metadata
    in ProbeResult. Add per-registry thresholds to QualityFilter. Follow
    the pattern established by Cargo in #1405.
---
