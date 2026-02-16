# Strategic Alignment Review: DESIGN-automated-seeding.md

**Reviewer role**: Strategic alignment reviewer
**Documents reviewed**:
- Tactical: `DESIGN-automated-seeding.md` (the design under review)
- Strategic: `DESIGN-registry-scale-strategy.md` (upstream)
- Strategic: `DESIGN-pipeline-dashboard.md` (upstream, direct parent)

---

## 1. Vision Delivery

### What the strategic designs expect from seeding

**DESIGN-pipeline-dashboard.md** defines seeding as Phase 3, explicitly labeled "Needs Design." It specifies:

1. **`cmd/seed-queue/` command** with PackageDiscovery, FreshnessChecker, DisambiguationRunner, QueueMerger components (line 1879)
2. **`seed-queue.yml` weekly workflow** (line 1880)
3. **Bootstrap Phase B**: full multi-ecosystem disambiguation run locally (line 1881)
4. **New source detection logic**: check audit candidates to determine if re-disambiguation is needed (line 1882)
5. **Ecosystem APIs**: crates.io, npm registry, PyPI, RubyGems (line 1885)
6. **Rate limiting strategy**: per-ecosystem limits, backoff (line 1888)
7. **API call estimation**: ~5K initial, ~260/week ongoing (lines 1899-1901)
8. **Seeding stats dashboard page** (`seeding.html`) with ecosystem coverage, disambiguation breakdown, source changes, seeding run history (lines 1354-1423)
9. **Source stability alerts**: GitHub issues for high-priority source changes (lines 446-449)
10. **Curated override validation**: check curated sources exist and alert on failures (lines 388-390)
11. **New source discovery check**: re-disambiguate when discovered source not in audit candidates (lines 370-380)

**DESIGN-registry-scale-strategy.md** references seeding more indirectly:

12. **Phase 3: Multi-Ecosystem** scope includes "cross-ecosystem scoring" and "ecosystem rate limiting" (lines 908-912)
13. **Milestone: Multi-Ecosystem** lists issue #1191 (system library backfill) as the tracked deliverable (lines 69-73)
14. **Priority queue** with popularity-based ordering from ecosystem APIs (lines 593-604)
15. **User request mechanism** feeding into queue prioritization (lines 599-604)

### How the tactical design addresses each

| Strategic expectation | Addressed? | Notes |
|---|---|---|
| cmd/seed-queue command | Yes | Full command spec with flags, exit codes, stdout JSON |
| Weekly workflow | Yes | Complete seed-queue.yml with source change issue creation |
| Bootstrap Phase B | Yes | Detailed local execution procedure with expected results |
| New source detection | Partial | Not directly implemented; relies on freshness-based re-disambiguation instead of audit candidate checking |
| Ecosystem APIs (4 ecosystems) | Yes | CratesIOSource, NpmSource, PyPISource, RubyGemsSource with specific endpoints |
| Rate limiting | Yes | Per-ecosystem limits documented per source |
| API call estimation | Yes | Consistent with parent (~260/week) |
| Seeding stats dashboard | No | Explicitly deferred: "No dashboard changes" (line 758) |
| Source stability alerts | Yes | GitHub issues for priority 1-2 source changes |
| Curated override validation | No | Not mentioned in the seeding design |
| New source discovery check (audit candidates) | No | The design uses staleness-based re-disambiguation, not audit candidate comparison |
| Cross-ecosystem scoring | Partial | Tier assignment uses per-ecosystem thresholds but no unified popularity normalization |
| User request mechanism | No | Not addressed |

---

## 2. Goal Alignment

### Strategic goals served

The tactical design serves the correct problem. Both strategic designs identify the root cause as "wrong ecosystem routing" -- packages like bat/fd/rg going through homebrew when they should use github/cargo sources. The seeding design directly addresses this by:

1. Running disambiguation at seeding time (not at generation time)
2. Expanding ecosystem coverage beyond homebrew
3. Producing a unified queue with pre-resolved sources

These goals are consistent with:
- DESIGN-pipeline-dashboard Decision 3 (unified disambiguated queue)
- DESIGN-pipeline-dashboard Decision 4 (multi-source seeding)
- DESIGN-registry-scale-strategy Phase 3 (multi-ecosystem deterministic)

### Goal consistency check

The tactical design's stated goals match the strategic direction well. The design correctly focuses on:
- Extending the existing `Source` interface (not inventing new patterns)
- Reusing `internal/discover` for disambiguation (not building new logic)
- Writing to the unified queue format defined in Phase 1 of the pipeline dashboard design
- Maintaining incremental seeding costs after bootstrap

---

## 3. Missing Commitments

### Gap 1: New Source Discovery via Audit Candidates (SIGNIFICANT)

The pipeline dashboard design specifies a specific mechanism for triggering re-disambiguation (lines 370-380):

```
discovered_source IN audit[tool].candidates -> already considered, skip
discovered_source NOT IN audit[tool].candidates -> new source, re-disambiguate
```

This is a distinct trigger from staleness. A package disambiguated 5 days ago (fresh by the 30-day threshold) should still be re-disambiguated if a newly added ecosystem discovers it as a candidate. The seeding design does not implement this mechanism. It relies solely on `disambiguated_at` freshness and `failure_count` thresholds. A fresh entry discovered from a new ecosystem source would be skipped.

This matters because the pipeline dashboard design explicitly describes this as the mechanism for handling:
- Bootstrap: Phase A entries with no audit log getting triggered by Phase B discoveries
- New ecosystems: existing tools lacking the new ecosystem in their candidates
- The design notes these as separate cases from the "normal weekly run"

**Impact**: Medium-high. This could cause packages to retain suboptimal sources for up to 30 days after a better source is discovered from a new ecosystem.

### Gap 2: Curated Override Validation (MODERATE)

The pipeline dashboard design specifies (line 389-390):

> Curated sources must be validated during seeding: if a curated source returns 404 or fails deterministic generation in a test run, alert operators rather than silently using a broken override.

The seeding design states curated entries are "never re-disambiguated" (line 196) but does not mention validation of curated sources. The pipeline dashboard's curated.html wireframe (lines 1425-1465) shows validation status for each override. Without validation during seeding, this dashboard page would have no data source.

The source stability alerts section of the pipeline dashboard (lines 447-448) also specifies: "Curated override validation fails (source returns 404)" as a required alert.

**Impact**: Medium. Broken curated overrides would silently persist, and the dashboard's curated validation feature would lack data.

### Gap 3: Seeding Stats Dashboard Data (MODERATE)

The pipeline dashboard design specifies a `seeding.html` page (lines 1354-1423) and a `seeding` section in `dashboard.json` (lines 573-574):

```json
"seeding": { "last_run", "packages_discovered", "stale_refreshed",
             "source_changes", "curated_invalid", "by_ecosystem" }
```

The seeding design explicitly defers this: "No dashboard changes: Seeding stats are not added to the dashboard in this design" (line 758). However, the stdout JSON output (lines 481-493) does produce most of the data needed for the dashboard. The gap is that nobody writes this data to `data/metrics/seeding-runs.jsonl` (mentioned in the pipeline dashboard's seed-queue.yml spec at line 592), and `queue-analytics` isn't updated to consume it.

**Impact**: Low-medium. The data is produced but not persisted in the format the dashboard expects. This creates integration work that falls between the seeding and observability designs with no clear owner.

### Gap 4: User Request Mechanism (LOW)

DESIGN-registry-scale-strategy mentions a user request mechanism (lines 599-604) where `tsuku install <unknown>` requests feed into queue prioritization. The seeding design doesn't address this, which is reasonable since it's a separate system. However, the registry scale strategy lists "Request-based priority: Boost packages users are requesting" as a Phase 4 deliverable (line 934), which means the seeding infrastructure should be extensible to accept user requests as an additional input. The current design doesn't mention this extensibility.

**Impact**: Low. This is a Phase 4 concern and the seeding design is Phase 3. But noting it for future-proofing.

### Gap 5: `data/metrics/seeding-runs.jsonl` Output (LOW)

The pipeline dashboard's seed-queue.yml spec (line 592) includes: "Write seeding stats to data/metrics/seeding-runs.jsonl". The tactical design's workflow writes a summary JSON to stdout and to `seeding-summary.json` (a temporary file), but does not persist seeding run history to `data/metrics/seeding-runs.jsonl`. This means there's no historical record of seeding runs for the dashboard's "Seeding History" table.

**Impact**: Low. Easy to add, but it's a specified deliverable in the parent design that's missing.

---

## 4. Scope Creep

### Assessment: Minimal scope creep

The seeding design stays within the boundaries set by the strategic designs. Every major component is either explicitly called for or a reasonable implementation detail.

One area worth noting:

**Tier assignment for non-homebrew sources** (lines 414-421): The tactical design introduces per-ecosystem download thresholds for tier assignment (crates.io > 100K, npm > 500K weekly, RubyGems > 1M total). This is a reasonable implementation detail, but the strategic designs don't specify how tiers should be assigned for non-homebrew sources. The registry scale strategy's priority queue uses "popularity score: downloads/stars normalized across ecosystems" (line 595), which suggests cross-ecosystem normalization rather than per-ecosystem thresholds. This isn't a conflict -- it's a necessary implementation decision -- but it does diverge from the "normalized across ecosystems" framing.

**PyPI disambiguation blind spot analysis** (lines 718-719): The seeding design identifies that PyPI's prober returns `Downloads: 0`, which creates a systematic bias against Python tools in disambiguation. This analysis and the proposed hint mechanism are valuable additions not called for in the strategic designs. This is useful scope expansion, not creep.

No conflicts with strategic direction were identified.

---

## 5. Timeline/Phasing Alignment

### Strategic phasing context

**DESIGN-registry-scale-strategy.md phases:**
- Phase 3: Multi-Ecosystem Deterministic (all deterministic builders, cross-ecosystem scoring, per-ecosystem rate limiting)
- Phase 4: Automation & Intelligence (auto-merge, re-queue triggers)

**DESIGN-pipeline-dashboard.md phases:**
- Phase 1: Unblock Pipeline (queue schema, bootstrap A, orchestrator) -- DONE
- Phase 2: Observability (dashboard enhancements) -- Needs design
- Phase 3: Automated Seeding -- THIS DESIGN

### Alignment assessment

The seeding design's internal phases are well-structured:
1. Phase 1: New Source Implementations
2. Phase 2: Disambiguation Integration and Queue Bridging
3. Phase 3: Command Extensions
4. Phase 4: Workflow and Bootstrap

These are implementation phases within the pipeline dashboard's Phase 3, not to be confused with the strategic phases. They sequence logically: build sources first, then wire up disambiguation, then expose through the command, then deploy.

**Dependency on Phase 1 completion**: The seeding design correctly assumes Phase 1 (unified queue schema) is done (all issues are struck through in the pipeline dashboard). It writes directly to `priority-queue.json` using `batch.QueueEntry`, which was defined in Phase 1.

**Relationship to Phase 2 (Observability)**: The seeding design explicitly defers dashboard changes, which means Phase 2 and Phase 3 can proceed in parallel. This is good. However, the pipeline dashboard design's seeding.html wireframe assumes seeding data is available, which creates a soft dependency: Phase 2's seeding stats page can't be fully implemented until Phase 3 produces the data. The seeding design should acknowledge this by ensuring its output format is compatible with what Phase 2 will need.

**Bootstrap Phase B timing**: The design correctly positions Bootstrap Phase B as a one-time local operation that runs after the seeding infrastructure is built but before weekly automation takes over. This matches the pipeline dashboard's description (lines 1890-1901).

---

## Findings Summary

### Aligned

- **Core architecture**: Extending `internal/seed.Source` interface with 4 new implementations follows the exact pattern the strategic designs call for.
- **Disambiguation integration**: Using `internal/discover.NewEcosystemProbe()` with `forceDeterministic: true` for batch seeding is the right reuse of existing infrastructure.
- **Freshness tracking**: 30-day threshold matches the pipeline dashboard specification exactly.
- **Source change alerting**: Priority 1-2 manual review via GitHub issues matches the pipeline dashboard's source stability alert requirements.
- **Curated entry protection**: Never re-disambiguating curated entries matches both strategic designs.
- **Queue type bridging**: Writing to the unified queue format (`batch.QueueEntry`) is consistent with Phase 1 deliverables.
- **Bootstrap Phase B**: The procedure matches the pipeline dashboard's description, including expected scope (5K packages) and output format (PR with queue + audit logs).
- **Audit log format**: Per-package JSON files in `data/disambiguations/audit/` match the pipeline dashboard's specification.
- **Rate limiting approach**: Per-ecosystem rate limiting in the source implementations is the approach both strategic designs recommend.
- **Security model**: Source change alerting, curated protection, and the deference to batch generation for binary verification are all aligned with strategic security requirements.

### Gaps

1. **New source discovery trigger missing**: The pipeline dashboard specifies re-disambiguation when a newly discovered source isn't in a package's audit candidates. The seeding design relies solely on time-based freshness, which could delay source improvements by up to 30 days.

2. **Curated override validation missing**: The pipeline dashboard requires validating curated sources during seeding (checking for 404s, alerting operators). The seeding design skips curated entries entirely without validation.

3. **Seeding stats persistence missing**: The pipeline dashboard expects `data/metrics/seeding-runs.jsonl` for historical seeding data. The seeding design produces equivalent data on stdout but doesn't persist it.

4. **Dashboard data contract undefined**: The seeding design defers dashboard changes but doesn't define the data contract (what fields the dashboard will need from seeding runs), creating integration risk with Phase 2.

### Conflicts

No direct conflicts were identified. The seeding design is consistent with both strategic designs in its approach, goals, and technical decisions. The per-ecosystem tier thresholds vs. cross-ecosystem normalization is a minor divergence in approach, but not a conflict since the strategic design doesn't mandate a specific normalization algorithm at this stage.

### Recommendations

1. **Add new-source discovery trigger**: Implement the audit candidate comparison mechanism from the pipeline dashboard design. When a source discovers a package already in the queue, check if that ecosystem was in the package's audit candidates. If not, trigger re-disambiguation regardless of freshness. This is a small addition to the freshness check logic and prevents stale source assignments when new ecosystems are onboarded.

2. **Add curated source validation**: During the freshness check phase, validate curated entries by checking if their source still exists (HTTP HEAD or equivalent). Don't re-disambiguate, but report broken curated sources in the stdout summary JSON under a `curated_invalid` field. This provides the data the dashboard's curated.html page needs and satisfies the pipeline dashboard's alerting requirement.

3. **Persist seeding run history**: Write a JSONL record to `data/metrics/seeding-runs.jsonl` after each seeding run. The data is already computed for stdout output -- just write it to a file too. This satisfies the pipeline dashboard's spec and gives Phase 2 the data it needs for the seeding history table.

4. **Document the dashboard data contract**: Add a section specifying what fields the seeding run produces that Phase 2 (observability) will consume. This doesn't require building the dashboard, just defining the interface so the two designs can proceed independently without integration surprises.

5. **Consider `queue-analytics` integration**: The pipeline dashboard's seed-queue.yml spec (line 593) includes "Run queue-analytics to update dashboard.json" as a step after seeding. The tactical design's workflow doesn't include this step. Adding it would keep the dashboard fresh after each seeding run.

---

## Executive Summary

The automated seeding design is well-aligned with both upstream strategic designs. It correctly identifies the extension points (`internal/seed.Source` interface, `internal/discover.EcosystemProbe`), targets the right output format (unified `batch.QueueEntry` queue), and implements the core workflow that both strategic documents call for: multi-ecosystem discovery, disambiguation at seeding time, freshness tracking, and source change alerting. The four gaps identified are all additive (missing features from the parent design's spec) rather than directional (wrong approach). The most significant gap is the absence of the new-source discovery trigger, which could cause packages to retain suboptimal sources for up to 30 days when new ecosystems are onboarded. The other gaps (curated validation, seeding stats persistence, dashboard data contract) are smaller and mostly affect the integration between seeding and the observability phase. No conflicts or scope creep were found. With the five recommended additions, the design would fully deliver on both strategic documents' vision for automated seeding.
