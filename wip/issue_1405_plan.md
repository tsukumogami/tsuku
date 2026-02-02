# Issue 1405 Implementation Plan

## Summary

This walking skeleton extends RegistryEntry with quality metadata fields (Downloads, VersionCount, HasRepository), changes the Probe() interface to return *discover.RegistryEntry instead of *builders.ProbeResult, implements full quality metadata extraction for the Cargo builder, creates a QualityFilter with per-registry thresholds, and wires it into the ecosystem probe resolver between match collection and priority sorting.

## Approach

The approach follows the interface unification pattern: move from builder-specific ProbeResult to the shared RegistryEntry format so all discovery paths (registry lookup, ecosystem probe, batch seeding, LLM discovery) use the same data structure. This enables consistent quality filtering without parallel structs.

The walking skeleton implements the full path with Cargo (crates.io has the richest quality signals available without extra API calls), then stubs the remaining 6 builders to keep the codebase compiling. Quality filtering uses OR logic (pass any threshold = accept) to balance false positives vs false negatives. Unknown builders fail-open (accept by default) to avoid breaking future ecosystem additions.

### Alternatives Considered

- **Alternative 1: Keep ProbeResult and convert to RegistryEntry later** - This would require conversion logic at the ecosystem probe boundary and duplicate quality filtering for different data shapes. Rejected because it creates parallel code paths and makes later issues (#1406-1408) harder to implement.

- **Alternative 2: Add quality metadata to ProbeResult instead of switching to RegistryEntry** - This keeps two separate structs with overlapping fields (Source, Builder, quality metadata). Rejected because the design explicitly calls for unifying on RegistryEntry as the single data format across all discovery stages.

- **Alternative 3: Implement full metadata for all 7 builders in this issue** - This would make the walking skeleton too large and delay validation of the architecture. Rejected because the issue description explicitly calls for Cargo as the proof-of-concept with stubs for the rest.

## Files to Modify

- `internal/discover/registry.go` - Add quality metadata fields to RegistryEntry struct (Downloads, VersionCount, HasRepository)
- `internal/builders/probe.go` - Change EcosystemProber.Probe() signature to return (*discover.RegistryEntry, error), remove ProbeResult struct
- `internal/builders/cargo.go` - Update cratesIOCrateResponse struct to include recent_downloads, num_versions, repository; update Probe() to return *discover.RegistryEntry with quality metadata populated
- `internal/builders/npm.go` - Update Probe() signature to return (*discover.RegistryEntry, error), populate Builder/Source only
- `internal/builders/pypi.go` - Update Probe() signature to return (*discover.RegistryEntry, error), populate Builder/Source only
- `internal/builders/gem.go` - Update Probe() signature to return (*discover.RegistryEntry, error), populate Builder/Source only
- `internal/builders/go.go` - Update Probe() signature to return (*discover.RegistryEntry, error), populate Builder/Source only
- `internal/builders/cpan.go` - Update Probe() signature to return (*discover.RegistryEntry, error), populate Builder/Source only
- `internal/builders/cask.go` - Update Probe() signature to return (*discover.RegistryEntry, error), populate Builder/Source only
- `internal/builders/probe_test.go` - Update all Probe() tests to expect *discover.RegistryEntry instead of *builders.ProbeResult
- `internal/discover/ecosystem_probe.go` - Update probeOutcome to use *discover.RegistryEntry, wire QualityFilter into Resolve() between match collection and priority sorting
- `internal/discover/ecosystem_probe_test.go` - Update mockProber to return *discover.RegistryEntry, update all test assertions

## Files to Create

- `internal/discover/quality_filter.go` - QualityFilter type with Accept(builderName string, entry *RegistryEntry) (ok bool, reason string) method and per-registry threshold configuration
- `internal/discover/quality_filter_test.go` - Unit tests for QualityFilter.Accept() covering threshold logic, fail-open behavior, OR logic

## Implementation Steps

- [ ] Extend RegistryEntry struct with quality metadata fields: Downloads int, VersionCount int, HasRepository bool (all with json:",omitempty" tags)
- [ ] Update cratesIOCrateResponse in cargo.go to include RecentDownloads, NumVersions, Repository fields from crates.io API
- [ ] Change EcosystemProber interface in probe.go: Probe(ctx context.Context, name string) returns (*discover.RegistryEntry, error) instead of (*ProbeResult, error)
- [ ] Remove ProbeResult struct definition from probe.go (no longer needed)
- [ ] Update Cargo builder Probe() to return *discover.RegistryEntry with quality fields populated from fetchCrateInfo response (Downloads from recent_downloads, VersionCount from num_versions, HasRepository from repository non-empty check)
- [ ] Update npm builder Probe() signature to match new interface, populate Builder="npm" and Source=name only (quality fields zero-valued)
- [ ] Update PyPI builder Probe() signature to match new interface, populate Builder="pypi" and Source=name only (quality fields zero-valued)
- [ ] Update Gem builder Probe() signature to match new interface, populate Builder="rubygems" and Source=name only (quality fields zero-valued)
- [ ] Update Go builder Probe() signature to match new interface, populate Builder="go" and Source=name only (quality fields zero-valued)
- [ ] Update CPAN builder Probe() signature to match new interface, populate Builder="cpan" and Source=name only (quality fields zero-valued)
- [ ] Update Cask builder Probe() signature to match new interface, populate Builder="cask" and Source=name only (quality fields zero-valued)
- [ ] Create QualityFilter type in internal/discover/quality_filter.go with Accept() method implementing OR logic for per-registry thresholds (crates.io: Downloads >= 100 OR VersionCount >= 5)
- [ ] Wire QualityFilter into ecosystem_probe.go Resolve() method: instantiate filter, call filter.Accept() for each match before adding to candidate list, log rejection reasons
- [ ] Update ecosystem_probe.go probeOutcome struct to hold *discover.RegistryEntry instead of *builders.ProbeResult
- [ ] Update ecosystem_probe.go match collection logic to use RegistryEntry.Source for name matching (replace result.Source with outcome.result.Source)
- [ ] Update ecosystem_probe.go result construction to use RegistryEntry fields (replace best.result.Downloads/Age with best.result.Downloads/VersionCount)
- [ ] Update mockProber in ecosystem_probe_test.go to return *discover.RegistryEntry instead of *builders.ProbeResult
- [ ] Update all probe_test.go test cases to expect *discover.RegistryEntry instead of *builders.ProbeResult (check result.Builder, result.Source fields)
- [ ] Update ecosystem_probe_test.go assertions to check RegistryEntry fields instead of ProbeResult fields
- [ ] Add unit tests for Cargo builder metadata extraction (test that recent_downloads → Downloads, num_versions → VersionCount, repository → HasRepository)
- [ ] Add unit tests for QualityFilter.Accept() covering: Cargo passing download threshold, Cargo passing version threshold, Cargo failing both (rejected), unknown builder (fail-open)
- [ ] Run validation script: verify prettier no longer resolves to crates.io (filtered), ripgrep still resolves to crates.io (passes)

## Testing Strategy

- **Unit tests for QualityFilter**: Test threshold logic with synthetic RegistryEntry data covering all combinations (pass downloads, pass versions, fail both, unknown builder). Verify Accept() returns correct ok/reason for each case.
- **Unit tests for Cargo Probe()**: Mock crates.io HTTP response with quality fields (recent_downloads, num_versions, repository). Verify Probe() populates Downloads, VersionCount, HasRepository correctly. Test with missing fields (zero-values).
- **Unit tests for ecosystem probe integration**: Update existing ecosystem_probe_test.go to use RegistryEntry instead of ProbeResult. Verify filter integration (matches rejected when quality fails).
- **Unit tests for stub builders**: Update probe_test.go to verify all 7 builders return valid RegistryEntry with Builder and Source populated (no compilation errors).
- **Manual verification**: Run validation script with real prettier/ripgrep queries to confirm filter behavior end-to-end.

## Risks and Mitigations

- **Risk: crates.io API field names may differ from documentation** (recent_downloads vs downloads, num_versions vs versions_count)
  - **Mitigation**: Check actual crates.io API response format before implementation. The API docs at https://crates.io/data-access show the exact field names. Fallback: if field names are uncertain, use manual curl test to verify.

- **Risk: Interface change touches all 7 builders - compilation breakage if any Probe() implementation is missed**
  - **Mitigation**: Use compile-time interface assertion check (var _ EcosystemProber = (*BuilderType)(nil)) in probe_test.go. Run `go build ./...` after interface change to catch missing implementations immediately.

- **Risk: Filter thresholds may be too aggressive (filter legitimate low-download tools) or too lenient (accept squatters)**
  - **Mitigation**: Use OR logic (pass any one signal = accepted) to reduce false negatives. Start with conservative thresholds (Downloads >= 100 OR VersionCount >= 5) based on prettier squatter data (87 downloads, 3 versions). Monitor validation results and adjust if needed.

- **Risk: Tests break due to mock data format change (ProbeResult → RegistryEntry)**
  - **Mitigation**: Update mockProber and all test assertions in a single commit after interface change. Run `go test ./internal/discover/... ./internal/builders/...` to verify all tests pass.

## Success Criteria

- [ ] All 7 builder Probe() methods compile with new signature (return *discover.RegistryEntry, error)
- [ ] Cargo builder populates Downloads, VersionCount, HasRepository from crates.io API
- [ ] QualityFilter.Accept() correctly implements OR threshold logic for crates.io (Downloads >= 100 OR VersionCount >= 5)
- [ ] QualityFilter.Accept() fails-open for unknown builders (returns true with "unknown builder" reason)
- [ ] prettier query does not resolve to crates.io (filtered due to low downloads and version count)
- [ ] ripgrep query still resolves to crates.io (passes quality filter)
- [ ] All unit tests pass: `go test ./internal/discover/... ./internal/builders/...`
- [ ] Full build succeeds: `go build ./...`

## Open Questions

None (all blocking questions resolved during research phase)
