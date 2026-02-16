# Strategic Alignment Review v2: DESIGN-automated-seeding.md

**Reviewer role**: Strategic alignment reviewer (second round)
**Documents reviewed**:
- Tactical: `DESIGN-automated-seeding.md` (updated design under review)
- Strategic: `DESIGN-registry-scale-strategy.md` (upstream)
- Strategic: `DESIGN-pipeline-dashboard.md` (upstream, direct parent)
- Prior reviews: `explore_alignment_strategic-review.md`, `explore_alignment_technical-review.md`

**Date**: 2026-02-16

---

## 1. Previously Identified Gaps -- Resolution Status

### Gap 1: New Source Discovery Trigger (was SIGNIFICANT)

**Previous finding**: The pipeline dashboard specifies re-disambiguation when a newly discovered source is not in a package's audit candidates. The seeding design relied solely on time-based freshness, which could delay source improvements by up to 30 days.

**Resolution: RESOLVED.**

The updated design adds a third re-disambiguation trigger (Decision 2, trigger #3, line 259):

> "A source discovers a package already in the queue, and the discovering ecosystem is not present in the package's audit candidates (`data/disambiguations/audit/<name>.json`)."

This directly matches the pipeline dashboard's specified mechanism (lines 370-380):

```
discovered_source IN audit[tool].candidates -> already considered, skip
discovered_source NOT IN audit[tool].candidates -> new source, re-disambiguate
```

The design also correctly explains the three use cases this handles: Bootstrap Phase A entries with no audit log, new ecosystems added later, and normal weekly runs where discoveries match existing candidates. The data flow diagram (line 473) and freshness check logic (lines 403-409) both include this trigger. No issues remaining.

### Gap 2: Curated Override Validation (was MODERATE)

**Previous finding**: The pipeline dashboard requires validating curated sources during seeding (HTTP HEAD to check for 404s, alerting operators). The seeding design skipped curated entries without validation.

**Resolution: RESOLVED.**

The updated design adds curated validation to the freshness check phase (lines 260-261):

> "Entries with `confidence: "curated"` are never re-disambiguated but are validated: the seeding command checks whether the curated source still exists (HTTP HEAD against the ecosystem API). Broken curated sources (404s, timeouts) are reported in the stdout summary under a `curated_invalid` field and create GitHub issues, but the queue entry is left unchanged."

The stdout JSON output (line 572-574) includes the `curated_invalid` field with package name, source, and error details. The design also notes this "provides the data the pipeline dashboard's curated validation page needs." The freshness check logic (lines 406-407) lists this as step 6. This satisfies both the pipeline dashboard's curated.html wireframe data requirements and its source stability alerting requirement.

### Gap 3: Seeding Stats Dashboard Data Persistence (was MODERATE)

**Previous finding**: The pipeline dashboard expects `data/metrics/seeding-runs.jsonl` for historical seeding data. The seeding design produced equivalent data on stdout but did not persist it.

**Resolution: RESOLVED.**

The updated design includes `data/metrics/seeding-runs.jsonl` in the output section of the component diagram (line 415):

> "Append to data/metrics/seeding-runs.jsonl (run history)"

The workflow's git commit step (line 670) explicitly adds this file: `git add data/queues/priority-queue.json data/disambiguations/audit/ data/metrics/seeding-runs.jsonl`. The consequences section (line 847) acknowledges this: "Seeding run history is persisted to `data/metrics/seeding-runs.jsonl` for future dashboard consumption."

### Gap 4: Dashboard Data Contract (was LOW)

**Previous finding**: The seeding design deferred dashboard changes but did not define the data contract for Phase 2 (observability) consumption.

**Resolution: PARTIALLY RESOLVED.**

The design now produces well-defined output: stdout JSON (lines 563-576), per-package audit files (lines 507-533), and seeding-runs.jsonl (line 415). The stdout JSON schema serves as an implicit contract -- its fields (`sources_processed`, `new_packages`, `stale_refreshed`, `source_changes`, `curated_invalid`) map directly to what the pipeline dashboard's `seeding.html` wireframe displays.

However, the design still does not explicitly name or document this as a contract that Phase 2 depends on. The consequences section (line 847) acknowledges the data is "for future dashboard consumption" but doesn't say "Phase 2 observability will consume these fields." This is a minor gap -- the data is there and the mapping is obvious, but a formal contract statement would reduce integration risk.

**Severity: Minor.** The data exists in the right shape; just lacks an explicit "this is the contract" declaration.

### Gap 5: `data/metrics/seeding-runs.jsonl` Output (was LOW)

**Previous finding**: The pipeline dashboard's seed-queue.yml spec includes writing seeding stats to `data/metrics/seeding-runs.jsonl`, which was missing from the tactical design.

**Resolution: RESOLVED.** See Gap 3 above. The file is now in the output list and the workflow's git add command.

### Technical Review Gap -- ResolveWithDetails() API (was HIGH from technical review)

**Previous finding (from technical review I11)**: The audit log format requires probe results from all ecosystems, but `EcosystemProbe.Resolve()` only returns the winning `*DiscoveryResult`. Raw `[]probeOutcome` data needed for audit logs was not exposed.

**Resolution: RESOLVED.**

The updated design adds `ResolveWithDetails()` to the Implementation Context (lines 127-136):

```go
type ResolveResult struct {
    Selected  *DiscoveryResult
    AllProbes []ProbeOutcome // expose the internal probeOutcome type
}

func (p *EcosystemProbe) ResolveWithDetails(ctx context.Context, toolName string) (*ResolveResult, error)
```

The data flow diagram (line 460) references this method directly: `EcosystemProbe.ResolveWithDetails(name) -> ResolveResult`. Phase 2 deliverables (line 761) include this method. The `Disambiguator.Resolve()` wrapper (line 753) returns `*AuditEntry` directly, showing the conversion path from `ResolveResult` to audit output. This is clean and consistent.

### Technical Review Gap -- AuditEntry Type (was MEDIUM from technical review I2)

**Previous finding**: The design claimed to "reuse" `DisambiguationRecord` but actually defined a superset with additional fields, which was misleading.

**Resolution: RESOLVED.**

The updated design defines a dedicated `AuditEntry` type in `internal/seed/audit.go` (lines 153-160) that explicitly embeds `DisambiguationRecord`:

```go
type AuditEntry struct {
    batch.DisambiguationRecord
    ProbeResults    []ProbeResult `json:"probe_results"`
    PreviousSource  *string       `json:"previous_source"`
    DisambiguatedAt time.Time     `json:"disambiguated_at"`
    SeedingRun      time.Time     `json:"seeding_run"`
}
```

The design text now accurately says "the implementation defines a new `AuditEntry` type that embeds it" (line 150). This is honest about the relationship and aligns with Go composition patterns.

### Technical Review Gap -- priority_fallback Handling (was HIGH from technical review I12)

**Previous finding**: The seeding design used `forceDeterministic: true` which auto-selects via priority_fallback, contradicting the pipeline dashboard's prohibition that priority_fallback results need manual review.

**Resolution: RESOLVED.**

The updated design addresses this directly in Assumption 3 (line 343):

> "When the result is a clear `10x_popularity_gap` or `single_match`, the entry gets `status: "pending"` with `confidence: "auto"`. When the result is a `priority_fallback` (no clear winner), the entry gets `status: "requires_manual"` instead -- consistent with the pipeline dashboard's constraint that ambiguous matches need human review."

The Decision Outcome section (lines 361-362) reinforces this:

> "When disambiguation produces a clear winner (`10x_popularity_gap` or `single_match`), the entry is auto-queued as `pending`. When it falls back to priority ranking (`priority_fallback`), the entry gets `status: "requires_manual"` for human review."

This satisfies the pipeline dashboard's constraint (line 1682) that `confidence: "priority"` is not valid for auto-selection. Entries with ambiguous results are properly routed to manual review rather than auto-accepted.

---

## 2. New Issues from Redesign

### Issue N1: EcosystemDiscoverer Interface Placement (Minor)

**Severity: Minor**

The design places the `EcosystemDiscoverer` interface in `internal/builders/probe.go` (line 419), alongside the existing `EcosystemProber` interface. The four builders that implement it (CargoBuilder, NpmBuilder, PyPIBuilder, GemBuilder) gain `Discover()` methods.

The pipeline dashboard design's "Incremental Seeding Pipeline" component diagram (lines 596-600) describes the seeding command's components as:

```
cmd/seed-queue/main.go (NEW)
  PackageDiscovery (fetch popular packages from each ecosystem)
  FreshnessChecker (stale/failing/new-source detection)
  DisambiguationRunner (imports internal/discover directly)
  QueueMerger (update entries, preserve freshness metadata)
```

The `PackageDiscovery` component in the dashboard design is a command-level abstraction. The tactical design pushes this down into the builders themselves. This is architecturally fine -- the builders already have the HTTP clients and API knowledge -- but it changes where ecosystem API knowledge for batch listing lives compared to what the dashboard design implied.

**Impact**: No functional gap. The dashboard design used `internal/discover/ (EXISTING - reused)` as the discovery mechanism, while the tactical design correctly puts discovery in `internal/builders/` where the ecosystem API knowledge already lives. The dashboard design's description was higher-level pseudocode, not a prescriptive architecture.

**Status**: Not a problem. The design documents different (but compatible) levels of detail.

### Issue N2: HomebrewBuilder Excluded from EcosystemDiscoverer (Minor)

**Severity: Minor**

The design explicitly excludes HomebrewBuilder from the `EcosystemDiscoverer` interface (line 228):

> "HomebrewBuilder does not implement `EcosystemDiscoverer`. Homebrew discovery uses the analytics endpoint which has a fundamentally different shape (popularity ranking of all formulae). The existing `HomebrewSource` in `internal/seed/` stays as-is."

This creates two discovery patterns in the same seeding command: `EcosystemDiscoverer.Discover()` for four builders, and `seed.Source.Fetch()` for Homebrew. The pipeline dashboard design's seeding workflow (lines 582-594) lists all ecosystems uniformly. The tactical design's component diagram (lines 383-436) handles this by having the seed command iterate both discoverer-capable builders AND the existing HomebrewSource.

**Impact**: Minor complexity in the seed command's main loop, which needs to handle two different discovery interfaces. The design acknowledges this (line 232) and the rationale is sound -- Homebrew's analytics endpoint truly is a different shape. But it means the "iterate over all discoverer-capable builders" loop in the data flow (line 444) must be supplemented by a separate Homebrew step. The design handles this correctly; this is just a note for implementers.

### Issue N3: Prober Construction for Seeding Command Not Fully Specified (Minor)

**Severity: Minor**

The technical review (A2) noted that the design did not specify how `cmd/seed-queue` obtains `[]builders.EcosystemProber` for constructing the `EcosystemProbe`. The updated design partially addresses this in Phase 2 (lines 745-756):

> "The disambiguator takes the same builder instances used for discovery (they implement both `EcosystemDiscoverer` and `EcosystemProber`), plus any non-discoverer probers (HomebrewBuilder, CaskBuilder, GoBuilder, CpanBuilder). It initializes `EcosystemProbe` with all 8 probers."

This explains the design intent: construct all builders once, use some for discovery and all for disambiguation. The `NewDisambiguator` function takes `[]builders.EcosystemProber` (line 752). However, the design does not specify how builders are constructed (what configuration, HTTP client setup, etc.). Currently, builder construction happens inside the chain resolver.

**Impact**: This is an implementation detail that doesn't affect strategic alignment. The design's architecture is clear; the builder construction code will need to be factored out of the chain resolver, which is a straightforward refactoring task.

### Issue N4: No queue-analytics Integration in Workflow (Minor)

**Severity: Minor**

The pipeline dashboard's seed-queue.yml description (line 593) includes: "Run queue-analytics to update dashboard.json." The tactical design's workflow (lines 581-685) does not include a step to run `queue-analytics` after seeding.

The first review mentioned this in recommendation #5 but the design was not updated to include it. This means the dashboard will not reflect seeding changes until the next independent trigger (e.g., a batch generation run or a manual data push).

**Impact**: Low. The dashboard will update on the next `update-dashboard.yml` trigger. The seeding workflow commits changes to `data/`, which likely triggers the dashboard update workflow. But the seeding design should confirm this trigger chain rather than relying on it implicitly.

---

## 3. Remaining Gaps

### RG1: User Request Mechanism Extensibility (Low)

**Severity: Minor**

The registry scale strategy mentions user requests feeding into queue prioritization (Phase 4, line 934: "Request-based priority: Boost packages users are requesting"). The seeding design still does not mention this. This was identified in the first review (Gap 4, rated LOW) and remains unaddressed.

**Impact**: Negligible for Phase 3. The seeding infrastructure is extensible enough -- adding a new discovery source for user requests would follow the same pattern as adding a new ecosystem. But the design does not acknowledge this extensibility path.

**Status**: Still open, still low severity. Phase 4 concern.

### RG2: Cross-Ecosystem Popularity Normalization (Low)

**Severity: Minor**

The registry scale strategy's priority queue uses "popularity score: downloads/stars normalized across ecosystems" (line 595). The seeding design uses per-ecosystem thresholds for tier-2 assignment (cargo > 100K, npm > 500K, rubygems > 1M). This was noted in the first review as "not a conflict" since the strategic design doesn't mandate a specific normalization algorithm.

The updated design hasn't changed this approach. The per-ecosystem thresholds are a practical implementation choice, but they don't produce a normalized popularity score across ecosystems. A cargo crate with 101K downloads gets tier-2 while a rubygems gem with 999K downloads gets tier-3. Whether this asymmetry is acceptable depends on the actual download count distributions, which the design acknowledges as an uncertainty.

**Impact**: Low. The tier assignment affects queue ordering but not correctness. The most popular tools in each ecosystem will get appropriate tiers via the shared tier-1 list. Tier-2/3 boundary differences across ecosystems are unlikely to affect user-visible outcomes.

**Status**: Still open, still low severity. May need revisiting after bootstrap data is analyzed.

### RG3: Audit Log Format Alignment with Pipeline Dashboard (Low)

**Severity: Minor**

The technical review (I3) found that the pipeline dashboard's audit log format uses different field names (`name`, `selected_source`, `candidates`, `decision_reason`) than the seeding design (`tool`, `selected`, `probe_results`, `selection_reason`). The technical review recommended the seeding design's names should take precedence since they match the existing `DisambiguationRecord` struct.

The updated seeding design keeps its field names (they match the code), but the pipeline dashboard design has not been updated. This means the two designs still describe different JSON schemas for the same file path (`data/disambiguations/audit/<name>.json`).

**Impact**: Low for implementation (the seeding design's format aligns with code and will be implemented first). But the pipeline dashboard design should be updated to match, or developers implementing the Phase 2 dashboard will reference stale field names.

**Status**: Open, but this is a pipeline dashboard design correction, not a seeding design issue. The seeding design is correct.

---

## 4. Overall Assessment

### Scorecard

| Category | v1 Rating | v2 Rating | Notes |
|----------|-----------|-----------|-------|
| Core architecture alignment | Strong | Strong | No change; extending builders is correct |
| Disambiguation integration | Strong | Strong | No change; EcosystemProbe reuse is right |
| Re-disambiguation triggers | Partial | Strong | All three triggers now present |
| Curated entry handling | Partial | Strong | Validation added |
| Seeding stats persistence | Missing | Strong | seeding-runs.jsonl added |
| Audit log API support | Missing | Strong | ResolveWithDetails() specified |
| AuditEntry type definition | Weak | Strong | Embedded composition defined |
| priority_fallback routing | Conflicting | Strong | Routes to requires_manual |
| Dashboard data contract | Missing | Adequate | Implicit via stdout schema |
| Workflow completeness | Good | Good | queue-analytics step still missing |
| New-source discovery trigger | Missing | Strong | Audit candidate check added |
| Security model | Strong | Strong | No change; aligned with both designs |

### Readiness for Implementation

**Yes, the design is ready for implementation from a strategic alignment perspective.** All five previously identified gaps from the first strategic review are resolved. All three high-severity findings from the technical review (ResolveWithDetails API, AuditEntry type, priority_fallback handling) are resolved. The four new issues identified in this second review are all Minor severity and concern implementation details rather than strategic direction.

The remaining low-severity items (user request extensibility, cross-ecosystem normalization, audit log field name alignment in the dashboard design, queue-analytics workflow step) are either deferred to later phases, implementation details that won't cause conflicts, or corrections needed in the upstream design rather than the tactical design.

---

## Findings Summary

### Resolved (from v1 review)

| # | Finding | Severity (v1) | Resolution |
|---|---------|--------------|------------|
| Gap 1 | New source discovery trigger | Significant | Added as re-disambiguation trigger #3 |
| Gap 2 | Curated override validation | Moderate | HTTP HEAD validation with curated_invalid reporting |
| Gap 3 | Seeding stats persistence | Moderate | seeding-runs.jsonl in output and workflow |
| Gap 5 | seeding-runs.jsonl output | Low | Same as Gap 3 |
| I11 | ResolveWithDetails() API | High | New method specified with ResolveResult type |
| I2 | AuditEntry type honesty | Medium | Embedded composition with batch.DisambiguationRecord |
| I12 | priority_fallback handling | High | Routes to requires_manual status |

### Partially Resolved

| # | Finding | Severity (v1) | Severity (v2) | Notes |
|---|---------|--------------|---------------|-------|
| Gap 4 | Dashboard data contract | Low | Minor | Data exists but no explicit contract statement |

### New Issues (v2)

| # | Finding | Severity | Impact |
|---|---------|----------|--------|
| N1 | EcosystemDiscoverer interface placement | Minor | Compatible with dashboard design's higher-level description |
| N2 | HomebrewBuilder excluded from EcosystemDiscoverer | Minor | Two discovery patterns in seed command; design handles correctly |
| N3 | Prober construction not fully specified | Minor | Implementation detail; architecture is clear |
| N4 | No queue-analytics step in workflow | Minor | Dashboard may not refresh immediately after seeding |

### Still Open (from v1)

| # | Finding | Severity | Notes |
|---|---------|----------|-------|
| RG1 | User request mechanism | Minor | Phase 4 concern; seeding infra is extensible |
| RG2 | Cross-ecosystem normalization | Minor | Practical choice; may revisit after bootstrap data |
| RG3 | Audit log field names in dashboard design | Minor | Dashboard design needs correction, not seeding design |

---

## Executive Summary

The updated DESIGN-automated-seeding.md resolves all significant gaps identified in the first-round strategic and technical reviews. The five changes made -- adding the audit-candidate-based new-source discovery trigger, curated source validation with reporting, seeding-runs.jsonl persistence, ResolveWithDetails() API for audit log population, AuditEntry type with proper embedding, and the priority_fallback-to-requires_manual routing -- directly address every gap rated Moderate or higher. The redesign choice to extend existing EcosystemProber builders with `Discover()` rather than creating separate Source implementations introduces no new strategic alignment problems; it keeps ecosystem API knowledge in one place and is compatible with the dashboard design's higher-level architecture. Four minor issues were found in this round: the dual discovery pattern for Homebrew vs. other ecosystems, incomplete prober construction details, a missing queue-analytics workflow step, and an implicit (rather than explicit) dashboard data contract. None of these affect strategic direction or would block implementation. The design is ready to proceed.
