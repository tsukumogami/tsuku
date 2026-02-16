# Technical Consistency Review: DESIGN-automated-seeding.md

**Reviewer scope:** Cross-referencing DESIGN-automated-seeding.md (tactical) against DESIGN-pipeline-dashboard.md (strategic) and DESIGN-registry-scale-strategy.md (strategic), plus actual codebase in `internal/seed/`, `internal/batch/`, and `internal/discover/`.

**Date:** 2026-02-16

---

## 1. Schema Consistency

### Consistent

- **QueueEntry struct matches the pipeline dashboard spec.** The code in `internal/batch/queue_entry.go` defines `QueueEntry` with fields `Name`, `Source`, `Priority`, `Status`, `Confidence`, `DisambiguatedAt`, `FailureCount`, and `NextRetryAt`. The seeding design correctly references all of these fields and uses them according to their documented semantics (e.g., `Confidence: "auto"`, `FailureCount: 0`, `DisambiguatedAt` set to current timestamp for new entries).

- **Status and Confidence constants match.** The code defines `StatusPending`, `StatusSuccess`, `StatusFailed`, `StatusBlocked`, `StatusRequiresManual`, `StatusExcluded` and `ConfidenceAuto`, `ConfidenceCurated`. The seeding design uses `"pending"`, `"success"`, `"auto"`, `"curated"` consistently with these constants.

- **Priority values are consistent.** All three documents and the code use 1-3 priority (1=critical, 2=popular, 3=standard). The seeding design's tier assignment maps directly to these values.

- **UnifiedQueue wrapper structure matches.** The code defines `UnifiedQueue{SchemaVersion, UpdatedAt, Entries}`. The seeding design's data flow targets this structure through `LoadUnifiedQueue`/`SaveUnifiedQueue`.

- **DisambiguationRecord struct is accurately quoted.** The seeding design's Implementation Context section (lines 117-124) shows the `DisambiguationRecord` struct from `internal/batch/results.go`. Cross-checking the actual code confirms the fields are identical: `Tool`, `Selected`, `Alternatives`, `SelectionReason`, `DownloadsRatio`, `HighRisk`.

### Inconsistencies

- **I1: `seed.Package` uses `Tier` but `batch.QueueEntry` uses `Priority`.** The seeding design correctly identifies this mapping (line 133: "`seed.Package.Tier` maps to `batch.QueueEntry.Priority`"), but this is a field name mismatch between two actively-used types. The design proposes a `convert.go` file but does not show the conversion function signature. The risk is subtle bugs where `Tier` and `Priority` have different semantics (e.g., if `Tier` ever gains a 0 value, the QueueEntry validator rejects `Priority < 1`). **Severity: Low.** The design acknowledges the mapping; implementation just needs to validate the conversion.

- **I2: The seeding design's audit log format extends `DisambiguationRecord` with fields not in the struct.** The audit JSON example (lines 226-235) includes `probe_results`, `previous_source`, `disambiguated_at`, and `seeding_run` -- none of which exist in the current `DisambiguationRecord` struct. The design says it "reuses" the existing structure, but actually defines a superset. This needs a new struct or an embedded composition. **Severity: Medium.** The design text is misleading about reuse. A new `AuditRecord` type that embeds `DisambiguationRecord` would be clearer.

- **I3: The pipeline dashboard's audit log format differs from the seeding design's.** The dashboard design (lines 1686-1711) uses fields `name`, `selected_source`, `candidates[].source`, and `decision_reason`. The seeding design (lines 226-235) uses `tool`, `selected`, `probe_results[].source`, `selection_reason`. These are the same data with different field names:

  | Pipeline Dashboard | Seeding Design | Semantic |
  |---|---|---|
  | `name` | `tool` | Tool name |
  | `selected_source` | `selected` | Chosen source |
  | `candidates` | `probe_results` | Ecosystem probe data |
  | `decision_reason` | `selection_reason` | Why this source was picked |

  **Severity: High.** These are both specifying the same file format (`data/disambiguations/audit/<name>.json`). If both designs are implemented as written, the formats won't match. The seeding design's format is closer to the existing `DisambiguationRecord` field names, so it should take precedence. The dashboard design's format should be updated.

### Ambiguities

- **A1: `seed.Package.Source` vs `batch.QueueEntry.Source` semantics.** In the current `HomebrewSource.Fetch()`, `Package.Source` is set to `"homebrew"` (ecosystem name only, no identifier). But `QueueEntry.Source` uses `"ecosystem:identifier"` format. The seeding design says new sources set `Source` to the ecosystem name (line 176: "Source set to the ecosystem name (e.g., `"cargo"`, `"npm"`)"), then disambiguation resolves to `"ecosystem:identifier"`. This is consistent with the intended flow, but it means `seed.Package.Source` has a different format from `batch.QueueEntry.Source`, which could confuse implementers who see both `Source` fields.

---

## 2. Interface Compatibility

### Consistent

- **Source interface is correctly described.** The seeding design quotes the `Source` interface verbatim from `internal/seed/source.go`: `Name() string` and `Fetch(limit int) ([]Package, error)`. The four new implementations follow the same pattern as `HomebrewSource`.

- **EcosystemProbe usage is correct.** The seeding design calls `discover.NewEcosystemProbe()` with `WithForceDeterministic()` for batch mode, which matches the actual code in `internal/discover/ecosystem_probe.go` (lines 40-44). The `Resolve(ctx, toolName)` method returns `*DiscoveryResult` containing the selected source and metadata -- exactly what the seeding design needs.

- **disambiguate() function signature matches code.** The design references `disambiguate(name, outcomes, priorities, nil, true)` (line 384), which matches the code signature `disambiguate(toolName string, matches []probeOutcome, priority map[string]int, confirm ConfirmDisambiguationFunc, forceDeterministic bool)`.

- **Queue Merge() method correctly identified as ID-based.** The design correctly notes that `PriorityQueue.Merge()` deduplicates by `ID` (not `Name`), and proposes a separate `FilterByName()` method for name-based deduplication (line 101). This matches the code in `internal/seed/queue.go` (lines 63-77).

### Inconsistencies

- **I4: The pipeline dashboard references `discover.Disambiguate()` which doesn't exist.** The dashboard design (line 315) shows `result, err := discover.Disambiguate(toolName, opts)`. There is no exported `Disambiguate` function in `internal/discover/`. The actual disambiguation is an unexported function `disambiguate()` in `internal/discover/disambiguate.go`, invoked internally by `EcosystemProbe.Resolve()`. The seeding design correctly calls `EcosystemProbe.Resolve()` instead (line 383), which is the right approach. **Severity: Medium.** The seeding design gets this right; the dashboard design's pseudocode is wrong. No code change needed in the seeding design, but the dashboard design should be corrected.

- **I5: The seeding design's `Disambiguator` wrapper type is not necessary.** The design proposes a new `internal/seed/disambiguator.go` with a `Disambiguator` struct wrapping `discover.EcosystemProbe` (lines 662-667). But `EcosystemProbe` already exposes `Resolve(ctx, toolName)` which does everything the wrapper does. The wrapper adds no value beyond naming. This isn't an inconsistency per se, but it introduces unnecessary indirection. **Severity: Low.** This is a design preference, not a technical mismatch.

### Ambiguities

- **A2: The seeding design says it calls `discover.NewEcosystemProbe()` directly, bypassing `ChainResolver`.** This is stated explicitly (line 198: "This avoids the registry lookup and LLM stages"). However, `NewEcosystemProbe()` requires `[]builders.EcosystemProber` as input (line 48 of ecosystem_probe.go). The design doesn't specify how the seeding command obtains the list of probers. Currently, this happens inside the chain setup. The seeding command will need to construct probers independently, which means importing `internal/builders` and setting up HTTP clients per ecosystem. This is implementable but not trivial, and the design doesn't address it.

---

## 3. Data Flow Consistency

### Consistent

- **Discovery-to-queue flow matches across documents.** All three designs agree: ecosystem APIs provide candidates, disambiguation selects the best source, the queue stores pre-resolved `ecosystem:identifier` sources, and batch generation uses the source directly. The seeding design's data flow diagram (lines 370-402) is consistent with the pipeline dashboard's architecture (lines 580-608).

- **Freshness threshold is consistent.** Both the seeding design and the pipeline dashboard specify 30 days as the freshness threshold for `disambiguated_at`. The seeding design adds a configurable `-freshness` flag with a default of 30, which is a compatible extension.

- **Bootstrap phasing is consistent.** The pipeline dashboard defines Bootstrap Phase A (queue migration from homebrew format to unified format, lines 1793-1814) and defers Phase B (full multi-ecosystem disambiguation, lines 1890-1897) to Phase 3. The seeding design covers Phase B in detail (lines 603-642), consistent with this sequencing.

- **Curated entries are protected consistently.** All documents agree: `confidence: "curated"` entries are never re-disambiguated by the seeding workflow.

- **Source change alerting is consistent.** The pipeline dashboard's "Source stability alerts" (lines 446-449) specify manual review for high-priority source changes. The seeding design refines this to priority 1-2 (lines 254-262), which is a valid refinement of the strategic guidance.

### Inconsistencies

- **I6: The pipeline dashboard says re-disambiguation triggers at `failure_count >= 3` (with exponential backoff), but the seeding design says `failure_count >= 3` AND stale `disambiguated_at`.** The pipeline dashboard (lines 393-399) uses failure count alone as a re-disambiguation trigger at the 3rd failure. The seeding design (line 196) adds a staleness requirement: "Entries with `failure_count >= 3` also trigger re-disambiguation, but only if their `disambiguated_at` is stale." This is a tighter condition. The seeding design is more conservative (won't re-disambiguate a recently-disambiguated package just because it failed 3 times), which is arguably better, but it deviates from the strategic design. **Severity: Medium.** The seeding design should document this as an intentional refinement, or the dashboard design should be updated.

- **I7: The pipeline dashboard defines a "new source discovery check" using audit log candidates (lines 371-380) that the seeding design does not implement.** The dashboard design says: when seeding discovers a source for a tool already in the queue, check the audit log to see if that source was already considered. If not, re-disambiguate. The seeding design's freshness check (lines 335-342) only checks `disambiguated_at` age, `failure_count`, `curated` status, and `success` status -- it does not check audit log candidates for new sources. This means a newly added ecosystem (e.g., if a 9th ecosystem were added) wouldn't trigger re-disambiguation for existing entries until their 30-day freshness expires. **Severity: Medium.** The seeding design is missing a trigger that the dashboard design specified.

### Ambiguities

- **A3: What happens to `seed.PriorityQueue` writes?** The current `cmd/seed-queue` writes to per-ecosystem files using `seed.PriorityQueue` format. The seeding design says it targets the unified queue directly (line 131), and that per-ecosystem files "remain for backward compatibility but are no longer the primary output" (line 138). But the design doesn't specify whether the per-ecosystem files are still written or just left as stale artifacts. The pipeline dashboard doesn't mention per-ecosystem files at all (its data files section, line 654, archives `priority-queue-homebrew.json`).

---

## 4. Naming/Terminology

### Consistent

- **"Ecosystem" vs "source" used correctly.** All three documents consistently use "ecosystem" to refer to the package registry (homebrew, cargo, npm) and "source" to refer to the `ecosystem:identifier` string (e.g., `cargo:ripgrep`). The code follows this convention.

- **"Curated" and "auto" confidence values are used consistently** across all documents and match the code constants.

- **Priority levels 1/2/3 are used consistently** and the tier-1 list is shared.

### Inconsistencies

- **I8: "Tier" vs "Priority" terminology.** The `seed.Package` struct uses `Tier` (as does the seeding design's "Tier Assignment" section, lines 414-422). The `batch.QueueEntry` struct uses `Priority`. The pipeline dashboard consistently uses "priority" (e.g., "priority 1-2 packages"). The seeding design uses both terms: "Tier 1" for source assignment (line 417) and "Priority 1-2" for alerting (lines 256-258). The mapping is documented (Tier maps to Priority), but using both terms in the same document creates confusion. **Severity: Low.** The design should pick one term for external-facing documentation and note the internal mapping.

- **I9: Audit log field name `selection_reason` vs `decision_reason`.** As noted in I3 above, the pipeline dashboard uses `decision_reason` and the seeding design uses `selection_reason`. The codebase's `DisambiguationRecord` uses `SelectionReason` (json: `"selection_reason"`). The seeding design aligns with the code; the dashboard design does not. **Severity: Medium** (duplicate of I3, listed here for completeness under naming).

- **I10: Builder name format inconsistency in priority map.** The `EcosystemProbe` priority map (ecosystem_probe.go line 53) uses `"crates.io"` as the builder name. But the seeding design's source format uses `"cargo:"` as the ecosystem prefix (line 176). The mapping between `"crates.io"` (builder name) and `"cargo"` (ecosystem prefix) is implicit. Similarly, `"pypi"` is used as the ecosystem prefix but the builder name in discover is likely different. The seeding design's probe_results example (line 231) uses `"cargo:ripgrep"` but the builder that produces this result is named `"crates.io"`. This is a pre-existing disconnect, not introduced by the seeding design, but it will surface during implementation when converting probe outcomes to audit records.

---

## 5. Dependency Assumptions

### Consistent

- **The seeding design correctly assumes `forceDeterministic: true` behavior.** It references the priority_fallback selection reason and marks those as `high_risk` in audit logs (line 278). This matches the code in `disambiguate.go` (lines 120-131) which sets `SelectionPriorityFallback` and the `EcosystemProbe` option `WithForceDeterministic()`.

- **The seeding design correctly assumes null `disambiguated_at` means stale** (Assumption 4, line 281). The bootstrap.go code (lines 358-366) creates homebrew entries without setting `DisambiguatedAt`, which results in nil in the JSON output. This nil value will cause the freshness check to treat all bootstrapped entries as stale, triggering Phase B disambiguation.

- **The seeding design correctly assumes exponential backoff exists in the batch pipeline.** The `QueueEntry` struct has `FailureCount` and `NextRetryAt` fields (queue_entry.go lines 38-43). The seeding design's freshness check (lines 341-342) correctly resets these on source change.

### Inconsistencies

- **I11: The seeding design assumes `EcosystemProbe.Resolve()` returns probe results for audit logging, but it only returns `*DiscoveryResult`.** The `Resolve()` method (ecosystem_probe.go, lines 80-141) returns a single `*DiscoveryResult` with selected source and metadata. The raw `[]probeOutcome` results (all ecosystems probed) are internal to `Resolve()` and not exposed. The seeding design's audit log format (lines 228-234) requires `probe_results` for all ecosystems, not just the winner. To populate the audit log, the seeding command would need access to raw probe outcomes, which requires either:
  - Modifying `EcosystemProbe` to expose raw outcomes (breaking the clean API)
  - Duplicating the probe logic in the seed package
  - Adding a new method like `ResolveWithAudit()` that returns both result and raw probes

  **Severity: High.** This is a significant implementation gap. The seeding design's audit format is valuable but can't be populated with the current `EcosystemProbe` API. The design should specify how raw probe data is obtained.

- **I12: The pipeline dashboard says `confidence: "priority"` is NOT valid for auto-selection (line 1682).** The seeding design uses `forceDeterministic: true` which falls back to priority ranking when there's no clear 10x winner. The selection reason becomes `"priority_fallback"` and is marked `high_risk`. The dashboard design prohibits this for auto-selection and says it should be "flagged for manual review rather than auto-selected." The seeding design's behavior with `forceDeterministic` auto-selects with priority_fallback and stores `confidence: "auto"` -- directly contradicting the dashboard's prohibition. **Severity: High.** The seeding design should either:
  - Not use `forceDeterministic` and instead return an error for ambiguous matches (setting status to `requires_manual`)
  - Use `forceDeterministic` but set `confidence` to something other than `"auto"` for priority_fallback selections
  - Or the dashboard design should relax its prohibition to allow `"auto"` with `high_risk: true` for priority_fallback

---

## Recommendations

### R1: Unify audit log field names (addresses I3, I9)

The seeding design's field names (`tool`, `selected`, `selection_reason`, `probe_results`) should be the canonical format since they align with the existing `DisambiguationRecord` struct. Update the pipeline dashboard's audit log section (lines 1686-1711) to match. Alternatively, define the canonical schema in one place and reference it from both designs.

### R2: Expose raw probe outcomes from EcosystemProbe (addresses I11)

Add a `ResolveWithDetails()` method or a `ProbeAll()` method to `EcosystemProbe` that returns both the selected result and the full list of probe outcomes. This is needed to populate audit logs with all ecosystem data, which is central to the seeding design's debugging value.

```go
type ResolveResult struct {
    Selected *DiscoveryResult
    AllProbes []ProbeOutcome // expose the internal probeOutcome type
}

func (p *EcosystemProbe) ResolveWithDetails(ctx context.Context, toolName string) (*ResolveResult, error)
```

### R3: Clarify priority_fallback handling (addresses I12)

Resolve the contradiction between the dashboard's prohibition of priority_fallback auto-selection and the seeding design's use of `forceDeterministic: true`. Recommended approach: use `forceDeterministic` but set `confidence: "auto"` only for `10x_popularity_gap` and `single_match` selections. For `priority_fallback` selections, set `status: "requires_manual"` instead of `"pending"`. This satisfies both designs: the seeding design still runs the full probe, and the dashboard's constraint that priority_fallback isn't auto-accepted is honored.

### R4: Add new-source detection to freshness check (addresses I7)

The seeding design's freshness check should include the audit-log-based new-source detection specified in the pipeline dashboard (lines 371-380). When a new ecosystem source is discovered that wasn't in the audit log's candidates, trigger re-disambiguation regardless of `disambiguated_at` age. This is the mechanism that handles the "new ecosystem added" case.

### R5: Define audit log as a new type, not reuse of DisambiguationRecord (addresses I2)

Create a dedicated `AuditEntry` type that embeds `DisambiguationRecord` and adds the seeding-specific fields (`probe_results`, `previous_source`, `disambiguated_at`, `seeding_run`). This makes the design honest about extending rather than reusing the struct.

### R6: Document prober construction for seeding command (addresses A2)

The seeding design should specify how `cmd/seed-queue` obtains `[]builders.EcosystemProber` for constructing the `EcosystemProbe`. The current code constructs probers inside the chain resolver setup. The seeding command will need a standalone prober factory or a helper function that returns the prober list without the full chain setup.

---

## Executive Summary

The DESIGN-automated-seeding.md is largely consistent with both strategic designs and the current codebase. It correctly uses the `Source` interface for extension, the `QueueEntry` struct for queue management, and the `EcosystemProbe` for disambiguation. The most significant issues are: (1) the audit log field names conflict between the seeding design and pipeline dashboard for the same file format -- the seeding design's names should win since they match existing code; (2) raw probe outcome data needed for audit logs isn't exposed by the current `EcosystemProbe` API, requiring an API extension; and (3) the seeding design's use of `forceDeterministic: true` auto-selects with priority_fallback, directly contradicting the pipeline dashboard's prohibition of auto-selection without clear 10x threshold or secondary signals. These three issues must be resolved before implementation to avoid building code that conflicts with the strategic architectural contract. The remaining findings are naming inconsistencies and minor ambiguities that can be addressed during implementation.
