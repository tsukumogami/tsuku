# Architecture Review: Disambiguation Design

**Reviewer**: Architecture Review Agent
**Date**: 2026-02-11
**Design Document**: `docs/designs/DESIGN-disambiguation.md`

## Executive Summary

The proposed disambiguation architecture is **well-designed and implementable**. It integrates cleanly with the existing codebase and follows established patterns. This review identifies three refinements and one simplification opportunity, but finds no fundamental issues with the approach.

**Verdict**: Architecture is clear and correctly sequenced. Proceed with minor refinements.

---

## Question 1: Is the architecture clear enough to implement?

**Answer**: Yes, with minor clarifications needed.

### Strengths

1. **Clear data flow**: The design explicitly shows Tool Name -> Typosquat Check -> Ecosystem Probe -> Quality Filter -> Disambiguation -> Result.

2. **Well-defined interfaces**: Component 1 shows concrete method signatures (`disambiguate()`, `isClearWinner()`), Component 2 shows `CheckTyposquat()` with clear inputs/outputs.

3. **Reuses existing patterns**: The `ConfirmFunc` callback pattern from LLM discovery (`llm_discovery.go:52-53`) provides a proven model for the `ConfirmDisambiguationFunc`.

4. **Existing code alignment**: The current `EcosystemProbe.Resolve()` in `ecosystem_probe.go` already collects multiple matches and sorts by priority. The design extends this rather than replacing it.

### Clarifications Needed

1. **Return type for alternatives**: The design says "Returns selected result with alternatives stored in metadata" but `DiscoveryResult.Metadata` (`resolver.go:18-32`) currently lacks an `Alternatives` field. The implementation should either:
   - Add `Alternatives []ProbeResult` to `Metadata`
   - Return a new struct wrapping `DiscoveryResult` with alternatives
   - Pass alternatives through a separate channel (less clean)

   **Recommendation**: Add to Metadata for simplicity.

2. **`probeResult` vs `ProbeResult` confusion**: The design uses `probeResult` (unexported) but the codebase has `builders.ProbeResult` (exported). The implementation should use the existing exported type consistently.

3. **Quality filter integration point**: The design shows quality filter before disambiguation, but currently `ecosystem_probe.go:98-101` applies the filter inside the collection loop. Either approach works, but the design should be explicit that the current inline filtering remains.

---

## Question 2: Are there missing components or interfaces?

**Answer**: One missing interface, one enhancement needed.

### Missing: AmbiguousMatchError Type

The design references `AmbiguousMatchError` but doesn't define its structure. Based on the non-interactive error format, it needs:

```go
type AmbiguousMatchError struct {
    Tool        string
    Matches     []ProbeMatch  // Ranked matches with metadata
    Suggestion  string        // Generated --from suggestions
}

type ProbeMatch struct {
    Builder     string
    Source      string        // e.g., "sharkdp/bat" for crates.io
    Downloads   int
    Versions    int
    HasRepo     bool
}
```

This error type should implement `error` and provide a formatted message matching the design's output:

```
Error: Multiple sources found for "bat". Use --from to specify:
  tsuku install bat --from crates.io:sharkdp/bat
  tsuku install bat --from npm:bat-cli
```

### Enhancement: TyposquatWarning Needs Confirmation Hook

The design shows a warning with "Continue with 'rgiprep'? [y/N]" but doesn't specify how this integrates with the existing confirmation flow. Recommendation:

```go
type ConfirmTyposquatFunc func(warning *TyposquatWarning) bool
```

This parallels `ConfirmFunc` and `ConfirmDisambiguationFunc`, keeping the UI decoupled from discovery logic.

### Interface Completeness Check

| Component | Interface Defined | Implementation Clear |
|-----------|------------------|---------------------|
| Disambiguation Logic | Yes (method signatures) | Yes |
| Typosquat Detector | Yes (`CheckTyposquat()`) | Yes |
| Interactive Prompt | Partially (callback type) | Yes |
| Non-Interactive Error | No (needs `AmbiguousMatchError`) | Partially |
| Batch Tracking | Yes (`DisambiguationRecord`) | Yes |

---

## Question 3: Are the implementation phases correctly sequenced?

**Answer**: Yes, with one reordering suggestion.

### Current Phase Sequence

| Phase | Component | Dependencies |
|-------|-----------|-------------|
| 1 | Core Disambiguation Logic | None (foundational) |
| 2 | Typosquatting Detection | None |
| 3 | Interactive Prompt | Phase 1 (disambiguation returns matches) |
| 4 | Non-Interactive Error | Phase 1, 3 (error formatting) |
| 5 | Batch Integration | Phase 1, 4 (tracking, deterministic selection) |

### Analysis

**Phase 1 -> 2 independence**: Both can proceed in parallel since they're independent concerns. However, starting with Phase 1 is correct because:
- Disambiguation is the core value proposition
- Typosquatting is a defense layer that can be added later

**Phase 3 -> 4 dependency**: Phase 4 depends on Phase 3 because the non-interactive error format mirrors the interactive prompt structure. This sequencing is correct.

**Phase 5 correctness**: Batch integration depends on Phases 1 and 4 being complete. The batch pipeline needs:
- Deterministic selection algorithm (Phase 1)
- Structured error data for tracking (Phase 4)

### Suggested Optimization

Consider combining Phases 3 and 4 into a single "User Feedback" phase since:
- They both touch the same files (`create.go`, `install.go`)
- The `ConfirmDisambiguationFunc` and `AmbiguousMatchError` formatting share similar data structures
- Testing can cover both interactive and non-interactive paths together

**Revised phases**:
1. Core Disambiguation Logic
2. Typosquatting Detection
3. User Feedback (Interactive + Non-Interactive)
4. Batch Integration

---

## Question 4: Are there simpler alternatives we overlooked?

**Answer**: Yes, one simplification opportunity.

### Alternative: Inline Disambiguation Without Separate File

The design proposes `disambiguate.go` as a separate file. Given the current size of `ecosystem_probe.go` (137 lines) and the relatively small disambiguation logic, an alternative is:

**Inline in ecosystem_probe.go**:
- Add `disambiguate()` and `isClearWinner()` as private methods
- Keeps related logic together
- Reduces file navigation during debugging

**Counter-argument**: The design's separate file approach is better because:
- Disambiguation has distinct test cases (ratio calculations, ranking edge cases)
- Future extensions (e.g., learning from user choices) would bloat ecosystem_probe.go
- Clear separation of concerns

**Recommendation**: Keep the separate file approach.

### Alternative: Reuse LLM Discovery Ranking

The design notes "shared package with LLM discovery ranking" as deferred. Review of `llm_discovery.go:254-295` shows:
- `rankCandidates()` sorts by confidence score then stars
- `shouldSwap()` compares `ConfidenceScore` and `Stars`

Ecosystem probe would rank by downloads and version count, which is fundamentally different. **The design's decision to defer abstraction is correct.**

### Alternative: Simpler 10x Threshold Check

The current design computes download ratio. A simpler approach would use logarithmic comparison:

```go
// Simple: 10x means order of magnitude difference
func isClearWinner(first, second ProbeResult) bool {
    return first.Downloads > 0 && second.Downloads > 0 &&
           first.Downloads / second.Downloads >= 10
}
```

This is what the design already proposes. No simplification needed.

---

## Architectural Risks

### Risk 1: Download Count Incomparability (Medium)

The design acknowledges this in "Uncertainties": npm weekly downloads vs crates.io recent downloads may not be comparable.

**Mitigation already in design**: Fall back to ecosystem priority when download data is incomplete or when comparing across ecosystems.

**Additional mitigation**: Add a "same-ecosystem comparison only" mode for the 10x threshold. Cross-ecosystem comparisons should use priority ranking exclusively.

### Risk 2: Interactive Detection Reliability (Low)

The design uses `isInteractive()` but doesn't specify the implementation. The codebase has this in `create.go:119-120`:

```go
if !isInteractive() {
    fmt.Fprintln(os.Stderr, "Error: --skip-sandbox requires interactive mode")
```

This likely checks `os.Stdin.Fd()` or similar. The pattern exists and can be reused.

### Risk 3: Batch Pipeline Silent Selection (Low)

In batch mode, deterministic selection happens silently. If the algorithm consistently picks the wrong ecosystem for a tool, it could generate many incorrect recipes.

**Mitigation in design**: `DisambiguationRecord` tracks selections for human review. The `disambiguations.json` seed file can be updated based on these reports.

**Enhancement**: Add a "high_risk" flag to `DisambiguationRecord` when selection_reason is "priority_fallback" (no download data available). This surfaces the riskiest selections for priority review.

---

## Integration Points Verification

### Files Modified by Design

| File | Modification | Verified Exists |
|------|-------------|-----------------|
| `internal/discover/ecosystem_probe.go` | Integrate disambiguation | Yes |
| `internal/discover/resolver.go` | Add AmbiguousMatchError | Yes |
| `internal/discover/chain.go` | Add typosquat check | Yes |
| `cmd/tsuku/create.go` | Add callbacks | Yes |
| `cmd/tsuku/install.go` | Add callbacks | Yes (implied) |
| `internal/batch/orchestrator.go` | Track disambiguation | Yes |
| `internal/dashboard/dashboard.go` | Display metrics | Yes |

### New Files

| File | Purpose | Location Correct |
|------|---------|------------------|
| `internal/discover/disambiguate.go` | Ranking and selection | Yes |
| `internal/discover/disambiguate_test.go` | Unit tests | Yes |
| `internal/discover/typosquat.go` | Edit distance | Yes |
| `internal/discover/typosquat_test.go` | Unit tests | Yes |

All locations follow existing package structure conventions.

---

## Testability Assessment

### Unit Test Coverage

| Component | Testable Units | Test Strategy |
|-----------|---------------|---------------|
| `isClearWinner()` | Ratio calculation, edge cases (0 downloads, equal downloads) | Table-driven tests |
| `rankProbeResults()` | Sorting by downloads, version count, priority | Table-driven tests |
| `disambiguate()` | Single match, clear winner, close matches, no download data | Mock probeOutcome inputs |
| `CheckTyposquat()` | Distance calculation, threshold boundary | Known typosquat examples |
| `AmbiguousMatchError.Error()` | Format output | Golden file or string comparison |

### Integration Test Coverage

| Scenario | Test Approach |
|----------|--------------|
| Full disambiguation flow | Mock ecosystem probers returning multiple matches |
| Interactive prompt | Inject `ConfirmDisambiguationFunc` that returns fixed selection |
| Non-interactive error | Run with piped stdin, verify error message |
| Batch tracking | Verify `DisambiguationRecord` in batch results |

### Existing Test Patterns to Follow

- `ecosystem_probe_test.go`: Mock `EcosystemProber` implementations
- `llm_discovery_test.go`: Mock HTTP and confirmation callbacks
- `batch/orchestrator_test.go`: End-to-end batch flow tests

---

## Summary of Recommendations

### Required Before Implementation

1. **Define `AmbiguousMatchError` struct** with full method signatures
2. **Decide on alternatives storage**: Add `Alternatives` field to `Metadata` or use wrapper struct
3. **Define `ConfirmTyposquatFunc`** callback signature

### Recommended Refinements

1. **Combine Phases 3 and 4** into single "User Feedback" phase
2. **Add `HighRisk` flag** to `DisambiguationRecord` for priority_fallback selections
3. **Cross-ecosystem threshold**: Use priority ranking (not 10x) when comparing different ecosystems

### No Changes Needed

- File locations and package structure are correct
- Phase sequencing is correct (with optional combination)
- The 10x threshold approach is appropriate
- Separate `disambiguate.go` file is the right design
- Deferring shared ranking abstraction is correct

---

## Conclusion

The disambiguation architecture is ready for implementation. The design demonstrates strong codebase awareness, reuses existing patterns effectively, and has appropriate escape hatches for edge cases. The three refinements above are minor and can be addressed during Phase 1 implementation.

**Implementation risk**: Low
**Confidence in design**: High
**Recommended action**: Proceed to implementation with refinements incorporated
