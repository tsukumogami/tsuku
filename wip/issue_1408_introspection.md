# Issue 1408 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-probe-quality-filtering.md`
- Sibling issues reviewed:
  - #1405: Quality filter skeleton with Cargo builder (closed)
  - #1406: npm and PyPI builders (closed)
  - #1407: Gem and Go builders (closed)
  - #1410: Seed-discovery wiring (closed)
- Prior patterns identified:
  - `Probe()` returns `*builders.ProbeResult` (not `*discover.RegistryEntry` as originally planned)
  - `QualityFilter` accepts builder name string and `*builders.ProbeResult`
  - Secondary API fetches are sequential (not parallel goroutines) in npm and gem implementations
  - Graceful degradation: if secondary fetch fails, return 0 for that field
  - Cask already has a stub `Probe()` that populates `Source` and `HasRepository`

## Gap Analysis

### Minor Gaps

1. **Cask already partially implemented**: The current `cask.go` (lines 429-439) already has a `Probe()` implementation that returns `ProbeResult` with `Source` and `HasRepository` populated from the `homepage` field. Issue #1408 doesn't need to change the signature - it needs to add `deprecated`/`disabled` flag handling and set `QualityFilter.Exempt = true`.

2. **Design doc mentions Cask/Homebrew were done in #1406**: The design doc's Implementation Issues table (line 30) says: "Cask and Homebrew formula metadata were completed in #1406." This contradicts the issue body which lists Cask work under this issue. Looking at the actual code, `cask.go` has a basic `Probe()` stub but doesn't handle disabled/deprecated flags.

3. **No threshold configured for CPAN in QualityFilter**: Looking at `quality_filter.go`, only `crates.io`, `npm`, and `pypi` have thresholds configured. The design doc specifies CPAN should use `river_total >= 1` OR `version_count >= 3`. This needs to be added to the filter.

4. **CPAN Probe() is minimal stub**: The current `cpan.go` (lines 249-257) returns only `Source` - no quality metadata. It needs the parallel `/v1/distribution/{name}` fetch for river metrics.

5. **Homebrew formulae builder mentioned in issue but not in title**: The issue title says "CPAN and Cask" but the design doc row says "Cask and Homebrew formula metadata were completed in #1406." Checking issue #1406 body confirms it covers "npm, PyPI, Cask, and Homebrew formula builders" - so Homebrew is done and not in scope for #1408.

### Moderate Gaps

1. **Cask exemption mechanism not specified**: The issue says "Set `QualityFilter.Exempt = true` for cask" but `QualityFilter` has no `Exempt` field. Looking at the actual implementation, builders without configured thresholds pass through (fail-open). The cask builder can simply not have a threshold entry, achieving exemption implicitly. However, the issue's acceptance criterion explicitly mentions this field which doesn't exist.

2. **Disabled cask rejection location unclear**: The issue says "Return nil if `disabled` is true (reject disabled casks immediately in Probe)" but currently Probe() returns nil only when fetchCaskInfo() errors. Adding disabled check means:
   - Fetch succeeds
   - Check disabled flag
   - Return nil if disabled

   This is consistent with sibling patterns but the AC should clarify this returns nil (not found) rather than an error.

### Major Gaps

None identified. The issue is implementable with minor clarifications.

## Recommendation

**Proceed** - The issue is implementable. The minor gaps are clarifications that can be incorporated into the implementation plan without user input.

## Amendments to Incorporate

When implementing, apply these patterns from sibling issues:

1. **CPAN builder pattern** (from npm.go and gem.go):
   - Add a `fetchRiverMetrics(ctx, name) int` method that fetches `/v1/distribution/{name}`
   - Parse `river.total` from the response
   - Call it from `Probe()` and set `result.Downloads` to the river value
   - Parse `repository` from the release endpoint for `HasRepository`
   - Return 0 on any fetch error (graceful degradation)

2. **Cask builder pattern** (extend existing Probe):
   - Fetch already happens via `fetchCaskInfo()`
   - Add `Deprecated bool` and `Disabled bool` to `caskAPIResponse` struct
   - In `Probe()`, check `info.Disabled` and return nil if true
   - No changes needed for quality filter - cask has no threshold entry (implicit exemption)

3. **Add CPAN threshold to QualityFilter**:
   - Add entry: `"cpan": {MinDownloads: 1, MinVersionCount: 3}` where MinDownloads represents river_total

4. **Test patterns** (from sibling test files):
   - Mock server returning both endpoints
   - Test graceful degradation when secondary endpoint fails
   - Test disabled cask returns nil
