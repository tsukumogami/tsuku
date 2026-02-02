---
summary:
  constraints:
    - Probe() must return *discover.RegistryEntry (not ProbeResult) to unify data shape across all discovery paths
    - QualityFilter uses OR logic for thresholds (pass any one signal = accepted)
    - Unknown builders must fail-open (accept by default)
    - All 7 builders must compile after interface change (stubs for non-Cargo)
  integration_points:
    - internal/discover/registry.go (extend RegistryEntry struct)
    - internal/builders/probe.go (change EcosystemProber interface, remove ProbeResult)
    - internal/builders/cargo/probe.go (full metadata implementation)
    - internal/builders/*/probe.go (6 stub updates)
    - internal/discover/ecosystem.go (wire QualityFilter into resolver)
  risks:
    - Interface change touches all 7 builders - compilation breakage if any missed
    - Cargo API response field names may differ from docs (recent_downloads vs downloads)
    - Filter thresholds may be too aggressive or too lenient for edge cases
  approach_notes: |
    Walking skeleton: extend schema, change interface, implement Cargo fully,
    stub remaining 6 builders, create QualityFilter, wire into resolver.
    Test with prettier (should be filtered) and ripgrep (should pass).
---
