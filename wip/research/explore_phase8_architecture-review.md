# Architecture Review: Probe Quality Filtering Design

## Executive Summary

The design is **implementable and well-structured**, with clear component separation and solid rationale. However, there are several areas where the architecture could be simplified or clarified, and a few potential issues around API data availability and filter logic.

**Overall Assessment: 7.5/10** - Good foundation, but benefits from refinement before implementation.

---

## 1. Clarity and Implementability

### Strong Points

**Component Separation**: The three-layer architecture (data collection → policy → enforcement) is clean:
- Builders: `Probe()` methods populate quality metadata
- `QualityFilter`: Centralized policy logic
- `EcosystemProbe.Resolve()`: Enforcement point

**Extension Strategy**: Adding two new fields to `ProbeResult` is straightforward and backward-compatible. The existing `Downloads` field is already present but unused, which is good.

**Per-Registry Thresholds**: The map-based threshold configuration correctly acknowledges that registries have different scales and signals. This is a key insight.

### Clarity Issues

**1. API Response Structures Missing**

The design shows what fields to add to `ProbeResult`, but doesn't document the actual API response structures per registry. For example:
- crates.io: Returns `crate.recent_downloads` (last 90 days), `crate.downloads` (all-time), and `versions` array
- npm: Registry endpoint doesn't have downloads; requires separate downloads API call
- RubyGems: Returns `downloads` (all-time) and `version_downloads` (unclear timeframe)

The design mentions these in passing but doesn't specify which exact JSON paths to parse. This will lead to implementation questions.

**Recommendation**: Add a table showing the exact JSON field paths per registry:

| Registry | Downloads Field | Version Count Field | Repository Field |
|----------|----------------|---------------------|------------------|
| crates.io | `crate.recent_downloads` | `versions.length` | `crate.repository` |
| npm | Downloads API `/downloads/point/last-week/{name}` | `versions` object keys count | `repository.url` or `repository` (string) |
| RubyGems | `downloads` (all-time) | Requires separate `/api/v1/versions/{gem}.json` call | `source_code_uri` |
| PyPI | N/A | `releases` object keys count | `info.project_urls["Repository"]` or `["Source"]` |
| MetaCPAN | N/A | Need to verify API | `metadata.resources.repository.url` |

**2. ProbeResult Field Semantics Unclear**

The design adds `VersionCount` and `HasRepository` but doesn't specify:
- What counts as a "version"? Pre-releases? Yanked versions? All published versions?
- What if a registry returns `null` vs empty string for repository? Both map to `false`?
- Should `Downloads` be normalized to a timeframe (e.g., always weekly)? Or registry-specific?

The comment says `Downloads` is "Monthly/recent downloads (0 if unavailable)" but crates.io returns 90-day downloads, npm would return weekly, and RubyGems returns all-time. This inconsistency could confuse threshold setting.

**Recommendation**: Either:
- Document that `Downloads` is registry-specific and thresholds must account for this, OR
- Add a `DownloadsTimeframe` field (enum: Weekly, Monthly, AllTime, Unknown) so filters can normalize

**3. RubyGems Requires Extra API Call**

The design claims RubyGems returns version count in the standard endpoint, but the code shows `rubyGemsGemResponse` only has `Name`, `Info`, `HomepageURI`, and `SourceCodeURI`. Version count requires a separate call to `/api/v1/versions/{gem}.json`.

This contradicts the design's claim that "downloads + version count + repository" come from the standard endpoint for most registries. RubyGems needs two calls (like npm), not one.

**Recommendation**: Update the design to explicitly list RubyGems as requiring an extra API call for version count, and document the latency trade-off.

---

## 2. Missing Components and Interfaces

### QualityFilter Constructor Missing

The design shows the `QualityFilter` struct and its `Accept()` method, but doesn't show how thresholds are initialized. Is there a `NewQualityFilter()` constructor? Are thresholds hardcoded or configurable?

**Recommendation**: Add constructor signature:

```go
func NewQualityFilter() *QualityFilter {
    return &QualityFilter{
        thresholds: map[string]QualityThreshold{
            "crates.io": {MinDownloads: 100, MinVersionCount: 5},
            "npm":       {MinDownloads: 100, MinVersionCount: 5},
            // ... etc
        },
    }
}
```

Make it explicit that thresholds are initially hardcoded but the struct is extensible for future config file support.

### Error Handling for Partial Failures

The design doesn't specify what happens if a builder's `Probe()` fetches the main API successfully but fails to fetch auxiliary data (e.g., npm downloads API times out).

Current behavior in `EcosystemProbe.Resolve()`: If `Probe()` returns an error, the outcome is discarded. But the design says "If the downloads call fails, Downloads stays at 0 and the filter falls back to version count."

This implies `Probe()` should NOT return an error for auxiliary failures—it should return a partial `ProbeResult` with some fields missing. But that's not documented.

**Recommendation**: Clarify error handling policy:
- `Probe()` returns `(*ProbeResult, error)`
- If the primary existence check fails (package doesn't exist), return `&ProbeResult{Exists: false}, nil`
- If the primary check succeeds but quality metadata fetch fails, return `&ProbeResult{Exists: true, Source: name, Downloads: 0, ...}, nil` (partial data)
- Only return an error for hard failures (network unreachable, 500s)

### No Integration Point with Discovery Registry Seeding

The design mentions the `QualityFilter` is reusable for "discovery registry seeding" but doesn't show how. Where does the seeding pipeline call `QualityFilter.Accept()`? What interface does it use?

If the seeding pipeline operates on different data structures (e.g., bulk dumps instead of individual Probe calls), the filter might need a different method signature.

**Recommendation**: Add a second use case example showing how the seeding pipeline would use `QualityFilter`. If the interface is the same, great. If not, document the adapter layer.

---

## 3. Implementation Phase Sequencing

The three-phase plan is logical but has a dependency issue:

**Phase 1**: Extend `ProbeResult` + create `QualityFilter` + wire into resolver
**Phase 2**: Update builder `Probe()` methods to populate fields
**Phase 3**: Integration testing

**Issue**: Phase 1 wires the filter into the resolver before any builders populate the new fields. This means the filter will run but always see `VersionCount: 0` and `HasRepository: false` for all results, effectively filtering everything out (unless all thresholds default to 0).

You can't validate the filter logic in Phase 1 without at least one builder populating real data.

**Recommendation**: Reorder phases:

- **Phase 1a**: Extend `ProbeResult` struct (non-breaking change, all fields default to zero values)
- **Phase 1b**: Update one builder (e.g., CargoBuilder) to populate all three fields as a proof-of-concept
- **Phase 1c**: Create `QualityFilter` with unit tests using realistic data from Phase 1b
- **Phase 1d**: Wire filter into resolver with feature flag (disabled by default)
- **Phase 2**: Update remaining builders in parallel (each is independent)
- **Phase 3**: Enable filter, run integration tests, tune thresholds

This allows incremental validation and avoids a "big bang" integration where everything is wired but untested.

---

## 4. Simpler Alternatives

### Alternative 1: Filter Inside Each Probe()

**Design rejected this**, arguing it scatters policy across 7 files. However, there's a middle ground:

```go
// In discover package
type ProbeQualityPolicy struct {
    MinDownloads    int
    MinVersionCount int
}

func (p *ProbeQualityPolicy) MeetsThreshold(result *ProbeResult) bool {
    // Centralized logic, called by each builder
}

// In each builder's Probe()
if !probePolicy.MeetsThreshold(result) {
    return &ProbeResult{Exists: false}, nil
}
```

This keeps the policy in one place but shifts the rejection decision to the builders, making the resolver simpler (no post-filter step).

**Trade-off**: Builders now have a dependency on the `discover` package, which creates a mild circular reference (discover imports builders, builders import discover's policy). The design's chosen approach avoids this.

**Verdict**: The design's choice is reasonable, but the alternative isn't significantly worse. If the circular dependency is acceptable, filtering in Probe() simplifies the resolver.

### Alternative 2: Relative Scoring Instead of Absolute Thresholds

The design rejected this as "overengineered," but consider:

A package with 99 downloads and 4 versions fails both thresholds and is rejected. But a package with 101 downloads and 0 versions passes (meets download threshold) and is accepted, even though it might be equally suspicious.

Relative scoring (e.g., `score = downloads/1000 + versions/10`, accept if `score > 1.0`) would handle this more gracefully.

**Verdict**: For the stated problem (reject obvious squatters), absolute thresholds are sufficient. Relative scoring adds complexity without clear benefit. The design is correct to defer this.

### Alternative 3: Fail-Closed for Signal-Rich Registries

The design uses "fail-open" logic: if a registry doesn't expose quality signals, don't block its results. But for registries that DO expose signals (crates.io, npm, RubyGems), should the filter fail-closed if metadata fetch fails?

Example: If the npm downloads API times out, should the filter reject the package (fail-closed) or accept it (fail-open)?

Current design: Fail-open (downloads = 0, falls back to version count).

**Trade-off**: Fail-closed is safer (avoid false positives from squatters) but worse user experience (legitimate tools randomly fail). Fail-open is better UX but weaker security.

**Verdict**: The design's fail-open approach is pragmatic for a package manager. Users expect `tsuku create prettier` to work, even if the npm downloads API is slow. A safer middle ground: **warn but don't block** when quality signals are unavailable.

---

## 5. QualityFilter API Assessment

### Signature Analysis

```go
func (f *QualityFilter) Accept(builderName string, result *builders.ProbeResult) bool
```

**Good**: Simple, pure function (no side effects). Easy to test.

**Issue 1: Tight Coupling**: The method takes `builderName` as a string, which must match keys in the `thresholds` map exactly. If a builder is renamed (e.g., "crates.io" → "cargo"), the filter silently breaks (unknown builder → default to exempt?).

**Recommendation**: Use a builder identifier type instead of raw string:

```go
type BuilderID string

const (
    BuilderCratesIO BuilderID = "crates.io"
    BuilderNpm      BuilderID = "npm"
    // ...
)

func (f *QualityFilter) Accept(builder BuilderID, result *builders.ProbeResult) bool
```

This makes typos a compile-time error.

**Issue 2: Boolean Return Hides Reason**: `Accept()` returns true/false but doesn't indicate WHY a package was rejected. This makes debugging threshold tuning difficult.

**Recommendation**: Return a struct instead:

```go
type AcceptanceResult struct {
    Accepted bool
    Reason   string // e.g., "downloads 87 < threshold 100, versions 3 < threshold 5"
}

func (f *QualityFilter) Accept(builder BuilderID, result *builders.ProbeResult) AcceptanceResult
```

The resolver can log the reason when a package is filtered, helping diagnose false negatives.

### Threshold Logic Clarity

The design says "A package must pass at least one threshold to be accepted." But the struct has three fields: `MinDownloads`, `MinVersionCount`, `Exempt`.

What about `HasRepository`? The design mentions it as a quality signal but doesn't include it in `QualityThreshold`. Should a package with `HasRepository: false` be penalized?

**Recommendation**: Either:
1. Add `RequireRepository bool` to `QualityThreshold`, OR
2. Clarify that `HasRepository` is informational only (not used in filtering)

The design doc mentions repository URL in the metadata table but doesn't use it in the threshold logic. This is confusing.

### Edge Case: Exempt Registries

The design exempts Cask and Go. But what if a malicious actor publishes a Go module `github.com/evil/prettier` that shadows the real prettier? The exemption means it passes the filter automatically.

Go's domain-based naming does deter squatting, but it doesn't eliminate it. Should exempt registries have a looser threshold instead of no threshold?

**Recommendation**: Document the trade-off explicitly. If Go is truly safe, keep the exemption. If there's any doubt, set a very low threshold (e.g., `MinVersionCount: 1`) rather than exempting entirely.

---

## 6. Data Flow Walkthrough

The design shows a clear flow:

```
Probe() → ProbeResult (with quality metadata)
    ↓
EcosystemProbe.Resolve()
    ↓
QualityFilter.Accept() → reject / accept
    ↓
Priority ranking (unchanged)
    ↓
DiscoveryResult
```

Let's trace a concrete example to validate:

### Example 1: `tsuku create prettier`

1. EcosystemProbe queries 7 builders in parallel
2. Cargo: Finds "prettier" crate (exists)
   - Fetches metadata: `{recent_downloads: 87, versions: 3, repository: null}`
   - Returns `&ProbeResult{Exists: true, Downloads: 87, VersionCount: 3, HasRepository: false, Source: "prettier"}`
3. Npm: Finds "prettier" package (exists)
   - Fetches metadata + downloads API: `{weekly_downloads: 68173154, versions: 60, repository: "https://github.com/prettier/prettier"}`
   - Returns `&ProbeResult{Exists: true, Downloads: 68173154, VersionCount: 60, HasRepository: true, Source: "prettier"}`
4. Resolver collects matches: `[cargo, npm]`
5. Filter phase:
   - Cargo: `QualityFilter.Accept("crates.io", cargo_result)`
     - Check: `87 >= 100`? No. `3 >= 5`? No.
     - Result: **Rejected**
   - Npm: `QualityFilter.Accept("npm", npm_result)`
     - Check: `68173154 >= 100`? Yes.
     - Result: **Accepted**
6. Remaining matches: `[npm]`
7. Priority ranking: npm wins (only candidate)
8. Return: `DiscoveryResult{Builder: "npm", Source: "prettier", ...}`

**Outcome**: Correct! The squatter is filtered out.

### Example 2: `tsuku create brand-new-tool`

1. Queries all builders
2. Cargo: Finds "brand-new-tool" crate
   - Metadata: `{recent_downloads: 10, versions: 2, repository: "https://github.com/newauthor/tool"}`
   - Returns `&ProbeResult{Exists: true, Downloads: 10, VersionCount: 2, HasRepository: true, Source: "brand-new-tool"}`
3. No other matches
4. Filter phase:
   - Cargo: `10 >= 100`? No. `2 >= 5`? No.
   - Result: **Rejected**
5. Remaining matches: `[]`
6. Resolver returns `(nil, nil)` (no matches)

**Outcome**: Legitimate new tool is filtered out. This is a known trade-off (documented in design), but the user gets no explanation. They see "tool not found" even though it exists on crates.io.

**Recommendation**: When all matches are filtered, log a warning: "Found brand-new-tool on crates.io but it doesn't meet quality thresholds (downloads: 10, versions: 2). If this is the tool you want, open an issue to report the false negative."

---

## 7. Potential Issues and Risks

### Issue 1: Threshold Values are Heuristic

The design sets:
- crates.io: `recent_downloads >= 100` OR `version_count >= 5`
- npm: `weekly_downloads >= 100` OR `version_count >= 5`

But "recent_downloads" on crates.io is 90 days, while npm is 7 days. So crates.io's threshold of 100 is ~1 download/day, while npm's is ~14 downloads/day. This is a 14x difference in effective strictness.

**Recommendation**: Normalize thresholds to "downloads per day" equivalent:
- crates.io: `100 / 90 ≈ 1.1 per day` → Threshold 100 (90 days)
- npm: `100 / 7 ≈ 14 per day` → Threshold 100 (7 days)

Or make npm's threshold higher: `weekly_downloads >= 1400` to match crates.io's effective rate. The current values are imbalanced.

### Issue 2: Version Count Can Be Gamed

A squatter can publish 10 empty releases (v0.0.1, v0.0.2, ..., v0.0.10) to pass the `version_count >= 5` threshold with zero meaningful code. This is cheaper than faking download counts.

**Recommendation**: Document this limitation. If it becomes a problem in practice, add a secondary signal like "versions must span at least N days" or "at least one version must have non-trivial downloads."

### Issue 3: npm Parallel Fetch Adds Latency

The design says npm's downloads API call is parallel with the registry fetch, "so it only adds latency if the downloads API is slower." But:

- Registry fetch: ~200ms (package metadata)
- Downloads API fetch: ~100-200ms (separate endpoint)

If they run in parallel, total time is `max(200ms, 150ms) = 200ms`. But if the downloads API is slow (300ms), it adds 100ms to the probe. And if it fails, the timeout is wasted time.

**Recommendation**: Set a shorter timeout for the downloads API call (e.g., 5 seconds instead of the default 60s from the HTTP client). If it times out, fall back to version count immediately.

### Issue 4: RubyGems Needs Two API Calls

If RubyGems requires `/api/v1/versions/{gem}.json` for version count, that doubles the latency and failure modes. The design doesn't account for this.

**Recommendation**: Either:
1. Skip version count for RubyGems and rely solely on downloads (simplify), OR
2. Make the versions API call parallel (like npm) and document the latency trade-off

### Issue 5: No Caching Strategy

If a user runs `tsuku create prettier` twice in a row, the probe fetches npm downloads API twice. Given that quality metadata changes slowly (downloads update daily, versions change on releases), caching could reduce latency.

**Out of scope for this design**, but worth noting for future optimization.

---

## 8. Recommended Changes

### High Priority

1. **Document exact API field paths** per registry (add table to design)
2. **Clarify RubyGems version count fetch** (requires extra API call, not free)
3. **Fix phase sequencing** (update at least one builder before wiring filter)
4. **Add rejection reason logging** to `QualityFilter.Accept()` for debugging
5. **Normalize download thresholds** to account for timeframe differences

### Medium Priority

6. **Use BuilderID type** instead of raw strings for builder names
7. **Add `RequireRepository` flag** to threshold config (or document it's unused)
8. **Log filtered results** so users know why a tool wasn't found
9. **Set shorter timeout** for npm downloads API (5s instead of 60s)

### Low Priority

10. **Consider fail-closed mode** for signal-rich registries (or document trade-off)
11. **Add integration point example** for discovery registry seeding
12. **Document version count gaming** as a known limitation

---

## 9. Alternative Architectures Overlooked

### Option: Confidence Scoring Instead of Binary Accept/Reject

Instead of filtering matches entirely, what if the filter assigned a confidence score?

```go
type ProbeMatch struct {
    Result     *ProbeResult
    Builder    string
    Confidence float64 // 0.0 = definitely squatter, 1.0 = definitely legit
}
```

The resolver could then prioritize by `(priority_rank, confidence)` instead of just priority. A high-confidence npm match could beat a low-confidence crates.io match, even though crates.io has higher priority.

**Trade-off**: More flexible, but adds complexity to the priority ranking logic. The design's binary filter is simpler and solves the stated problem (prettier/httpie resolving wrong).

**Verdict**: Binary filtering is sufficient for MVP. Confidence scoring is a good follow-up feature.

### Option: LLM-Assisted Quality Check

For ambiguous cases (e.g., a package with 90 downloads and 4 versions—just below both thresholds), the system could ask an LLM: "Is this a legitimate tool or a squatter?" and use the response to override the filter.

**Trade-off**: Adds LLM cost and latency. Only helps for edge cases. Probably overkill.

**Verdict**: Not worth it for this problem.

---

## 10. Final Verdict

### Is the architecture clear enough to implement?

**Mostly yes**, but needs clarification on:
- Exact API field paths per registry
- RubyGems extra API call requirement
- Error handling for partial failures
- Threshold normalization across timeframes

### Are there missing components or interfaces?

**Yes**:
- `QualityFilter` constructor
- Rejection reason return value
- Integration point for registry seeding (example needed)
- BuilderID type for type safety

### Are the implementation phases correctly sequenced?

**No**—Phase 1 wires the filter before any builders populate data, making validation impossible. Needs reordering (update one builder first as proof-of-concept).

### Are there simpler alternatives we overlooked?

**No major simplifications missed**. The design correctly rejects "downloads only" (excludes PyPI/Go) and "relative scoring" (overengineered). Filtering inside `Probe()` is a valid alternative but has circular dependency issues.

### Does the QualityFilter API make sense?

**Mostly**, but:
- Use `BuilderID` type instead of string for type safety
- Return a struct with rejection reason, not just bool
- Clarify `HasRepository` field usage (informational or filter criteria?)

---

## Summary

**Strengths**:
- Clean component separation (data/policy/enforcement)
- Per-registry thresholds correctly handle heterogeneous signals
- Reusable filter component for multiple contexts
- Fail-open strategy is pragmatic for a package manager

**Weaknesses**:
- Missing API implementation details (field paths, extra calls)
- Phase sequencing allows invalid intermediate state
- Threshold values are imbalanced across registries
- No rejection reason logging (hard to debug false negatives)

**Recommendation**: Implement with the clarifications and reordering suggested above. The core architecture is sound, but the details need tightening before coding begins.
