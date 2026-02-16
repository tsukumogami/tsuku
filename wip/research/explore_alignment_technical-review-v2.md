# Technical Consistency Review: DESIGN-automated-seeding.md (v2)

**Reviewer**: Technical Consistency Agent
**Date**: 2026-02-16
**Review Round**: 2
**Documents reviewed**:
- `docs/designs/DESIGN-automated-seeding.md` (tactical design under review)
- `docs/designs/DESIGN-registry-scale-strategy.md` (strategic design)
- `docs/designs/DESIGN-pipeline-dashboard.md` (strategic design)

**Codebase files verified**:
- `internal/builders/probe.go` -- EcosystemProber interface
- `internal/builders/cargo.go` -- CargoBuilder
- `internal/builders/npm.go` -- NpmBuilder
- `internal/builders/pypi.go` -- PyPIBuilder
- `internal/builders/gem.go` -- GemBuilder
- `internal/seed/source.go` -- Source interface
- `internal/seed/queue.go` -- PriorityQueue, Package, Merge
- `internal/seed/homebrew.go` -- HomebrewSource
- `internal/seed/filter.go` -- FilterExistingRecipes
- `internal/batch/queue_entry.go` -- QueueEntry, UnifiedQueue
- `internal/batch/results.go` -- DisambiguationRecord, FailureRecord
- `internal/discover/ecosystem_probe.go` -- EcosystemProbe, probeOutcome
- `internal/discover/disambiguate.go` -- disambiguate(), isClearWinner()
- `internal/discover/resolver.go` -- DiscoveryResult, Metadata, DiscoveryMatch

---

## Focus Area 1: Builder Extension Feasibility

### Finding 1.1: Builder Structure Supports Discover() Addition

**Severity**: N/A (Positive confirmation)
**Status**: No action needed

All four target builders (CargoBuilder, NpmBuilder, PyPIBuilder, GemBuilder) share the same structural pattern that makes adding `Discover()` straightforward:

- Each has a private `httpClient *http.Client` field for making HTTP requests
- Each has a configurable base URL (e.g., `cratesIOBaseURL`, `npmRegistryURL`, `pypiBaseURL`, `rubyGemsBaseURL`)
- Each has `WithBaseURL` constructors for test injection
- Each already implements rate limit handling (429 detection)
- Each has response size limiting (`io.LimitReader`) and content-type validation

The `Discover()` method would use the same HTTP client and base URL infrastructure, adding new endpoint paths. This is a natural extension of the existing builder pattern.

### Finding 1.2: CargoBuilder Has Direct Category API Support

**Severity**: N/A (Positive confirmation)
**Status**: No action needed

The design proposes querying `https://crates.io/api/v1/crates?category=command-line-utilities&sort=downloads`. The existing `CargoBuilder.fetchCrateInfo()` already constructs URLs via `url.Parse(b.cratesIOBaseURL)` and `baseURL.JoinPath(...)`, and sends requests with the required `User-Agent` header (crates.io policy). The `Discover()` method would follow the same pattern.

### Finding 1.3: NpmBuilder Has Separate Downloads API Already

**Severity**: N/A (Positive confirmation)
**Status**: No action needed

`NpmBuilder` already has `npmDownloadsURL` as a separate field and `fetchWeeklyDownloads()` as a method. The design's proposal to fetch downloads from `https://api.npmjs.org/downloads/point/last-week/{name}` during discovery is already implemented as a pattern in `Probe()`. The search endpoint (`/-/v1/search?text=keywords:cli`) uses the same base registry URL.

### Finding 1.4: PyPIBuilder Uses External Data Source for Discovery

**Severity**: Minor
**Status**: Acknowledged in design

The design proposes using `hugovk.github.io/top-pypi-packages/` as the static dump for PyPI discovery. The existing `PyPIBuilder` only talks to `pypi.org`. The `Discover()` method would need to introduce a second URL (the static dump) alongside the existing `pypiBaseURL`. This is a minor structural divergence from the other builders where `Discover()` uses the same API endpoints.

The design correctly identifies this in the Security Considerations section ("PyPI data source") and proposes validation against the official PyPI API. The uncertainty about classifier coverage is also documented. No code change is needed to the existing builder to accommodate this -- `Discover()` would add the new URL as part of its implementation.

### Finding 1.5: GemBuilder Downloads Field Already Available

**Severity**: N/A (Positive confirmation)
**Status**: No action needed

The `rubyGemsGemResponse` struct already has a `Downloads int` field. The design proposes using `/api/v1/downloads/top.json` and checking `executables` -- both use the existing `rubyGemsBaseURL` and follow the same request pattern as `fetchGemInfo()`.

---

## Focus Area 2: Interface Design

### Finding 2.1: EcosystemDiscoverer Interface Composes Cleanly

**Severity**: N/A (Positive confirmation)
**Status**: No action needed

The proposed interface:
```go
type EcosystemDiscoverer interface {
    EcosystemProber
    Discover(ctx context.Context, limit int) ([]DiscoveryCandidate, error)
}
```

This is a standard Go interface embedding pattern. Since `EcosystemProber` already embeds `SessionBuilder`, the full interface chain is `SessionBuilder -> EcosystemProber -> EcosystemDiscoverer`. Builders that implement `EcosystemDiscoverer` automatically satisfy `EcosystemProber`, allowing the same builder instance to be used for both discovery and disambiguation probing. The design explicitly describes this reuse: "The seed command constructs all builders once, passes the discoverer-capable ones to the discovery loop, and passes the full set to NewDisambiguator."

### Finding 2.2: Type Assertion Pattern Required for Discovery

**Severity**: Minor
**Status**: Implicit in design, could be more explicit

The seed command iterates over builders and calls `Discover()` on those that support it. This requires a runtime type assertion:

```go
if discoverer, ok := builder.(builders.EcosystemDiscoverer); ok {
    candidates, err := discoverer.Discover(ctx, limit)
}
```

The design describes this behavior ("iterates over discoverer-capable builders") but does not show the type assertion code. This is a standard Go pattern and not a technical risk, but implementers should note that the builder list includes non-discoverer probers (HomebrewBuilder, CaskBuilder, GoBuilder, CpanBuilder) that must be skipped during the discovery loop.

### Finding 2.3: ResolveWithDetails() API Gap Addressed

**Severity**: N/A (Previously Critical, now resolved)
**Status**: Resolved

The prior review identified that `EcosystemProbe.Resolve()` returns only `*DiscoveryResult`, but audit logs need all probe outcomes. The design now specifies `ResolveWithDetails()`:

```go
type ResolveResult struct {
    Selected  *DiscoveryResult
    AllProbes []ProbeOutcome
}
func (p *EcosystemProbe) ResolveWithDetails(ctx context.Context, toolName string) (*ResolveResult, error)
```

This requires exposing the internal `probeOutcome` type (currently unexported in `ecosystem_probe.go`). The design addresses this with a `ProbeOutcome` exported type. The implementation path is clear: `ResolveWithDetails()` follows the same logic as `Resolve()` but captures and returns all `probeOutcome` values before filtering and disambiguation.

**One observation**: The `probeOutcome` struct currently includes `builderName`, `result *builders.ProbeResult`, and `err error`. The design's `ProbeOutcome` type (exported) would need to map these fields. The audit log `probe_results` field uses `source`, `downloads`, `version_count`, `has_repository` -- which maps cleanly to `builders.ProbeResult` fields. This mapping is straightforward.

---

## Focus Area 3: Previously Identified Issues -- Resolution Status

### Finding 3.1: Audit Log Field Names Aligned

**Severity**: Previously Critical, now resolved
**Status**: Resolved

The prior review found field name mismatches between the audit log JSON and `DisambiguationRecord` struct fields. The current design shows:

**Proposed AuditEntry struct** (line 153-160):
```go
type AuditEntry struct {
    batch.DisambiguationRecord
    ProbeResults    []ProbeResult `json:"probe_results"`
    PreviousSource  *string       `json:"previous_source"`
    DisambiguatedAt time.Time     `json:"disambiguated_at"`
    SeedingRun      time.Time     `json:"seeding_run"`
}
```

**Actual DisambiguationRecord** (from `internal/batch/results.go`):
```go
type DisambiguationRecord struct {
    Tool            string   `json:"tool"`
    Selected        string   `json:"selected"`
    Alternatives    []string `json:"alternatives"`
    SelectionReason string   `json:"selection_reason"`
    DownloadsRatio  float64  `json:"downloads_ratio,omitempty"`
    HighRisk        bool     `json:"high_risk"`
}
```

**Audit log JSON example** (line 286-300):
```json
{
  "tool": "ripgrep",
  "selected": "cargo:ripgrep",
  "alternatives": ["homebrew:ripgrep", "github:BurntSushi/ripgrep"],
  "selection_reason": "10x_popularity_gap",
  "downloads_ratio": 14.0,
  "high_risk": false,
  "probe_results": [...],
  "previous_source": null,
  "disambiguated_at": "2026-02-16T06:00:00Z",
  "seeding_run": "2026-02-16T06:00:00Z"
}
```

The JSON field names match the Go struct tags. The embedded `DisambiguationRecord` produces `tool`, `selected`, `alternatives`, `selection_reason`, `downloads_ratio`, `high_risk` -- all matching the JSON example. The seeding-specific fields (`probe_results`, `previous_source`, `disambiguated_at`, `seeding_run`) are added by `AuditEntry`. This is consistent.

### Finding 3.2: priority_fallback Contradiction Resolved

**Severity**: Previously Important, now resolved
**Status**: Resolved

The prior review found a contradiction: the pipeline dashboard design stated that `priority_fallback` should not be auto-selected, but the seeding design proposed auto-queuing these as `pending`.

The current design resolves this in Assumption 3 (line 343): "When the result is a `priority_fallback` (no clear winner), the entry gets `status: "requires_manual"` instead -- consistent with the pipeline dashboard's constraint that ambiguous matches need human review. These entries are marked `high_risk` in the audit log."

This is consistent with `DESIGN-pipeline-dashboard.md` line 1682: "`confidence: 'priority'` (ecosystem priority fallback) is NOT valid for auto-selection per DESIGN-disambiguation.md."

### Finding 3.3: New-Source Trigger Now Specified

**Severity**: Previously Important, now resolved
**Status**: Resolved

The prior review found that the "new source discovery" re-disambiguation trigger was mentioned but not specified. The current design provides a clear definition (line 259): "A source discovers a package already in the queue, and the discovering ecosystem is not present in the package's audit candidates (`data/disambiguations/audit/<name>.json`)."

This maps to the pipeline dashboard's specification (line 371-376):
```
discovered_source IN audit[tool].candidates -> already considered, skip
discovered_source NOT IN audit[tool].candidates -> new source, re-disambiguate
```

The trigger is now consistently defined across both designs.

---

## Focus Area 4: New Technical Issues

### Finding 4.1: Audit Log probe_results Type vs. ProbeResult

**Severity**: Minor
**Status**: Needs clarification

The `AuditEntry` struct declares:
```go
ProbeResults []ProbeResult `json:"probe_results"`
```

But which `ProbeResult` is this? The codebase has `builders.ProbeResult`:
```go
type ProbeResult struct {
    Source        string
    Downloads     int
    VersionCount  int
    HasRepository bool
}
```

The audit log JSON example uses `"source"` as the field name, which matches `builders.ProbeResult.Source`. However, in the audit JSON example (line 517), the source is written as `"cargo:ripgrep"` (with ecosystem prefix), while in the actual `builders.ProbeResult`, the `Source` field is the package name without prefix (e.g., `"ripgrep"` -- see CargoBuilder.Probe() line 363: `Source: name`).

The `AuditEntry.ProbeResults` would need to use a different type that includes the ecosystem prefix, or the mapping code needs to prepend the builder name to the source. This is a minor implementation detail, but the design's JSON example is slightly misleading about what `builders.ProbeResult.Source` actually contains.

**Recommendation**: The `AuditEntry.ProbeResults` should use the full `ecosystem:name` format as shown in the JSON examples. The conversion from `builders.ProbeResult.Source` (which is just the package name) to the audit format (which includes the ecosystem prefix) needs to happen during audit file generation, combining `probeOutcome.builderName` with `probeOutcome.result.Source`.

### Finding 4.2: DiscoveryCandidate Download Semantics Differ Across Ecosystems

**Severity**: Minor
**Status**: Acknowledged in design but worth highlighting

The `DiscoveryCandidate.Downloads` field has different semantics across ecosystems:
- **Cargo**: `recent_downloads` (recent period, not lifetime)
- **npm**: Weekly downloads (from separate API)
- **PyPI**: 0 (not available at discovery time from the static dump; the static dump has "download_count" but the design says downloads are 0)
- **RubyGems**: Lifetime downloads (`downloads` field in gem response)

The tier assignment thresholds (line 499-502) partially account for this: "crates.io > 100K recent downloads, npm > 500K weekly downloads, RubyGems > 1M total downloads." But the PyPI case is not covered at all for tier-2 assignment. The design says "PyPI candidates don't have download counts at discovery time, so they start at tier 3."

This is internally consistent but worth noting: the `hugovk.github.io/top-pypi-packages/` static dump does contain download counts. If those were plumbed into `DiscoveryCandidate.Downloads`, PyPI candidates could also receive tier-2 assignments. This is an optimization, not a bug.

### Finding 4.3: Homebrew tier1Formulas Map vs. "Shared" Claim

**Severity**: Minor
**Status**: Slight ambiguity

The design says (line 499): "Tier 1: Only assigned via the shared `tier1Formulas` map (same 27 package names across all sources)." The actual `tier1Formulas` map in `internal/seed/homebrew.go` contains ~33 entries (not 27). The count discrepancy is minor, but the "shared" claim needs implementation: currently `tier1Formulas` is a package-level variable in the `seed` package, not a shared constant accessible to builders. The `Discover()` methods in `internal/builders/` would need access to this map for tier assignment.

**Resolution path**: Either move `tier1Formulas` to a shared location (e.g., `internal/seed/tiers.go` exported as `Tier1Packages`), or have the conversion happen in `cmd/seed-queue` where both packages are importable.

The design's component diagram (line 429) shows `convert.go` in `internal/seed/` handling the conversion from `DiscoveryCandidate` to `seed.Package`, which would have access to `tier1Formulas`. This works without moving the map.

### Finding 4.4: Bootstrap Phase B with -limit 0 Semantics

**Severity**: Minor
**Status**: Needs clarification

The Bootstrap Phase B procedure (line 699) uses `-limit 0`:
```bash
./seed-queue -source homebrew -limit 0 ...
```

The flag description (line 546) says "-limit int: Max packages per source (default 500)". The Bootstrap section explains "set freshness to 0 to force all" but doesn't clarify what `-limit 0` means. In the context of the `HomebrewSource.Fetch()` implementation (line 69):
```go
if limit > 0 && limit < len(items) {
    items = items[:limit]
}
```

When limit is 0, the condition `limit > 0` is false, so no truncation occurs -- all items pass through. This is correct for bootstrap (process all entries), but the command's flag documentation should explicitly state that `-limit 0` means "no limit." The current flag description says "Max packages per source" which could imply 0 means "none."

### Finding 4.5: Queue Name Deduplication vs. Merge ID Deduplication

**Severity**: Important
**Status**: Correctly identified in design, but implementation requires attention

The design correctly identifies (line 115): "Merge() deduplicates by ID (e.g., 'homebrew:ripgrep'), which includes the ecosystem prefix. Since a package can be discovered from multiple ecosystems with different IDs, the seed command must deduplicate by name (not by ID) before merging."

The solution architecture (line 429) adds `FilterByName` to `internal/seed/queue.go`. But the unified queue (`batch.QueueEntry`) uses `Name` not `ID` for its entries, and there is no `Merge()` method on `UnifiedQueue`. The design says the command "converts seed.Package -> batch.QueueEntry" and uses "queue.Merge()".

Looking at the actual code, `seed.PriorityQueue.Merge()` deduplicates by `ID` on `seed.Package`, but the unified queue (`batch.UnifiedQueue`) has no `Merge()` method at all. The design targets the unified queue directly, so a new merge method on `UnifiedQueue` (or equivalent logic in the command) is needed. The design's component diagram lists this under `queue.go (existing, add FilterByName method)`, but the merge logic for `batch.QueueEntry` is a new capability.

This is correctly scoped by the design but should be highlighted: the implementation needs a merge function for `batch.QueueEntry` that deduplicates by `Name`, not by any ecosystem-prefixed ID.

### Finding 4.6: Concurrency Group Shared with Batch Generation

**Severity**: Important
**Status**: Correctly designed but worth verifying

The workflow (line 604) uses:
```yaml
concurrency:
  group: queue-operations
  cancel-in-progress: false
```

The existing `batch-generate.yml` workflow also modifies `priority-queue.json`. If both workflows use different concurrency groups, they could race on queue writes. The `cancel-in-progress: false` ensures seeding runs complete, but if batch generation runs hourly and seeding runs weekly, they need the same concurrency group to prevent concurrent queue modifications.

**Verification needed**: Check whether `batch-generate.yml` also uses `concurrency: group: queue-operations` or a compatible locking mechanism. The push-with-rebase retry loop (line 679) provides eventual consistency but could lose seeding changes if batch generation pushes between the seed command finishing and the git push.

### Finding 4.7: AuditEntry Embeds DisambiguationRecord but Alternatives Format Differs

**Severity**: Important
**Status**: Inconsistency in design

The `batch.DisambiguationRecord.Alternatives` field is typed as `[]string`:
```go
Alternatives []string `json:"alternatives"`
```

The audit JSON example shows alternatives as strings: `["homebrew:ripgrep", "github:BurntSushi/ripgrep"]`.

However, the pipeline dashboard's audit log format (line 1694-1707) shows alternatives as objects with full probe metadata:
```json
"candidates": [
    {"source": "cargo:ripgrep", "downloads": 1250000, ...},
    {"source": "homebrew:ripgrep", "downloads": 89000, ...}
]
```

The seeding design's audit log uses `probe_results` (separate from `alternatives`) to hold the full probe data, and `alternatives` is just `[]string` of source names. The pipeline dashboard uses `candidates` (a different field name) with full probe objects.

This means the pipeline dashboard's audit log format and the seeding design's audit log format use different field names for the same concept:
- Pipeline dashboard: `candidates` (array of objects with downloads, version_count, etc.)
- Seeding design: `probe_results` (array of `ProbeResult` objects) + `alternatives` (array of source name strings)

The seeding design's format is actually richer (it has both the full probe data AND the alternatives list from `DisambiguationRecord`), but the field naming is inconsistent. The "new source discovery" trigger (checking if a discovered source is "in the audit candidates") would need to read `probe_results[].source`, not a `candidates` field.

**Recommendation**: This is an implementation detail that doesn't block the design, but the implementation should clarify that the "audit candidates" referenced in the re-disambiguation trigger maps to `probe_results[].source` in the audit file, not a `candidates` field.

---

## Focus Area 5: Cross-Document Consistency

### Finding 5.1: Phase Numbering Alignment

**Severity**: N/A (Consistent)
**Status**: No action needed

- `DESIGN-pipeline-dashboard.md` defines Phase 3 as "Automated Seeding (Needs Design)"
- `DESIGN-automated-seeding.md` correctly references this: "This design implements Phase 3 of DESIGN-pipeline-dashboard.md"
- `DESIGN-registry-scale-strategy.md` defines Phase 3 as "Multi-Ecosystem Deterministic" at a higher level

The pipeline dashboard's Phase 3 is a subset of the registry scale strategy's broader Phase 3. The automated seeding design correctly positions itself within the pipeline dashboard's phasing, not the registry scale strategy's phasing.

### Finding 5.2: Queue Field Names Consistent

**Severity**: N/A (Consistent)
**Status**: No action needed

All three designs and the codebase use the same field names for `QueueEntry`:
- `name`, `source`, `priority`, `status`, `confidence`, `disambiguated_at`, `failure_count`, `next_retry_at`

The actual `batch.QueueEntry` struct in `queue_entry.go` matches these fields exactly, including JSON tags and Go types (`*time.Time` for nullable timestamps).

### Finding 5.3: Re-Disambiguation Trigger Alignment

**Severity**: N/A (Consistent)
**Status**: No action needed

The three re-disambiguation triggers in the seeding design (line 255-259) align with the pipeline dashboard's specification (line 365-368):

| Trigger | Pipeline Dashboard | Automated Seeding |
|---------|-------------------|-------------------|
| Staleness | `disambiguated_at >= 30 days` | `disambiguated_at > 30 days or null` |
| Failures | `next_retry_at set and past` | `failure_count >= 3 AND stale disambiguated_at` |
| New source | `discovered NOT IN audit candidates` | Same, with explicit file path |

The failure trigger has a minor refinement: the seeding design adds the conjunction with staleness (`failure_count >= 3 AND disambiguated_at is stale`) which the pipeline dashboard didn't specify. The seeding design explicitly documents this as "an intentional refinement" (line 258). This is a tighter condition that avoids re-disambiguating packages that were recently checked. It's technically an extension, not a contradiction.

### Finding 5.4: Exponential Backoff Model Difference

**Severity**: Minor
**Status**: Intentional divergence, documented

The pipeline dashboard design (line 392-398) specifies exponential backoff for failures:
```
1st failure: Retry on next batch selection (no delay)
2nd failure: Set next_retry_at to +24 hours
3rd failure: Set next_retry_at to +72 hours, trigger re-disambiguation
4th+ failure: Double the backoff (max 7 days), re-disambiguate each time
```

The seeding design (line 408-409) says: "Reset failure_count/next_retry_at on source change." This is consistent -- source changes clear the backoff.

However, the seeding design's freshness check (line 402-403) triggers re-disambiguation on `failure_count >= 3 AND stale disambiguated_at`, while the pipeline dashboard triggers re-disambiguation on the 3rd failure regardless of staleness. The seeding design's conjunction prevents re-disambiguation of recently checked packages, which is a stricter but reasonable condition.

### Finding 5.5: "requires_manual" Status Usage Consistent

**Severity**: N/A (Consistent)
**Status**: No action needed

The seeding design uses `status: "requires_manual"` for `priority_fallback` disambiguation results (line 343, 361). This maps to the `StatusRequiresManual` constant in `batch.QueueEntry` (line 52: `StatusRequiresManual = "requires_manual"`). The pipeline dashboard mentions packages that "need LLM or human intervention" get this status (line 1656-1657). All three sources agree.

### Finding 5.6: Selection Reason Constants Duplicated

**Severity**: Minor
**Status**: Pre-existing issue, not introduced by this design

The selection reason constants (`SelectionSingleMatch`, `Selection10xPopularityGap`, `SelectionPriorityFallback`) are defined in both `internal/discover/disambiguate.go` and `internal/batch/results.go` with identical values. The seeding design uses the `discover` package's constants via `disambiguate()`. This duplication pre-dates the seeding design and doesn't create a new inconsistency, but a future cleanup could consolidate them.

### Finding 5.7: Dashboard Seeding Stats Page Deferred Consistently

**Severity**: N/A (Consistent)
**Status**: No action needed

The seeding design (line 847): "Dashboard data available, page deferred: Seeding run history is persisted to `data/metrics/seeding-runs.jsonl` for future dashboard consumption. The seeding stats page itself (wireframed in the pipeline dashboard design) is deferred to Phase 2 (Observability)."

The pipeline dashboard design (line 551-557) wireframes `seeding.html` under Phase 2 observability. The seeding design writes the data but defers the page. This is consistent.

---

## Summary of All Findings

| # | Finding | Severity | Status |
|---|---------|----------|--------|
| 1.1 | Builder structure supports Discover() | N/A | Confirmed |
| 1.2 | CargoBuilder has direct category API support | N/A | Confirmed |
| 1.3 | NpmBuilder has separate downloads API | N/A | Confirmed |
| 1.4 | PyPIBuilder uses external data source | Minor | Acknowledged |
| 1.5 | GemBuilder downloads field available | N/A | Confirmed |
| 2.1 | EcosystemDiscoverer composes cleanly | N/A | Confirmed |
| 2.2 | Type assertion pattern needed | Minor | Implicit |
| 2.3 | ResolveWithDetails() gap addressed | N/A | Resolved |
| 3.1 | Audit log field names aligned | N/A | Resolved |
| 3.2 | priority_fallback contradiction resolved | N/A | Resolved |
| 3.3 | New-source trigger specified | N/A | Resolved |
| 4.1 | ProbeResult.Source lacks ecosystem prefix | Minor | Needs mapping |
| 4.2 | Download semantics differ across ecosystems | Minor | Acknowledged |
| 4.3 | tier1Formulas map access and count | Minor | Solvable |
| 4.4 | -limit 0 semantics undocumented | Minor | Needs docs |
| 4.5 | UnifiedQueue lacks Merge() method | Important | New capability needed |
| 4.6 | Concurrency group shared with batch | Important | Needs verification |
| 4.7 | Audit alternatives vs. candidates naming | Important | Inconsistency |
| 5.1 | Phase numbering aligned | N/A | Consistent |
| 5.2 | Queue field names consistent | N/A | Consistent |
| 5.3 | Re-disambiguation triggers aligned | N/A | Consistent |
| 5.4 | Exponential backoff model difference | Minor | Intentional |
| 5.5 | requires_manual status consistent | N/A | Consistent |
| 5.6 | Selection reason constants duplicated | Minor | Pre-existing |
| 5.7 | Dashboard seeding stats deferred | N/A | Consistent |

---

## Executive Summary

The v2 design is substantially improved. All three previously identified high-severity issues (audit log field names, ResolveWithDetails() API gap, priority_fallback contradiction, and new-source trigger) are resolved. The central decision to extend existing `EcosystemProber` builders with `Discover()` instead of creating parallel `Source` implementations in `internal/seed/` is well-supported by the codebase: all four target builders have the HTTP clients, base URLs, response parsing, and rate-limit handling needed for batch discovery. The interface composition (`EcosystemDiscoverer` embedding `EcosystemProber`) is clean and allows a single builder instance to serve both discovery and disambiguation probing. Three Important-severity findings remain: (1) the unified queue needs a new merge capability since `batch.UnifiedQueue` has no `Merge()` method, (2) the seeding workflow's concurrency group must be verified against `batch-generate.yml` to prevent queue write races, and (3) the audit log's "alternatives" field (from embedded `DisambiguationRecord`) and "probe_results" field need clear documentation so the new-source trigger implementation knows to check `probe_results[].source` rather than a `candidates` field. None of these findings block the design; they are implementation details that should be addressed during Phase 2 and Phase 3 coding.
