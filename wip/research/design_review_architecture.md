# Architecture Review: DESIGN-llm-discovery-implementation.md

**Reviewer Role**: Architecture Reviewer
**Review Date**: 2026-02-10
**Design Document**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-llm-discovery-implementation.md`

## Summary Ratings

| Criterion | Rating (1-10) | Notes |
|-----------|---------------|-------|
| Pattern Consistency | 7 | Follows most existing patterns; some divergence in session design |
| Component Boundaries | 8 | Clear separation of concerns; minor coupling issues |
| Data Flow | 7 | Logical flow but some unclear paths for non-deterministic results |
| Extensibility | 6 | Search providers extensible; result handling less so |
| Interface Design | 7 | SearchProvider is clean; extract_source schema deferred |

**Overall Rating: 7/10**

---

## Strengths Identified

### 1. Tool-Based Search Architecture is Sound (Lines 139-228)

The decision to expose `web_search` as a tool that Cloud LLMs handle natively while local LLMs use a tsuku-provided handler is elegant. This provides:

- **Unified abstraction**: The LLM doesn't know or care where results come from
- **Provider flexibility**: Cloud providers get native quality; local LLMs get capability
- **LLM-controlled strategy**: The model can refine searches or do multiple queries

This matches how the existing HomebrewBuilder exposes tools like `fetch_formula_json` and `inspect_bottle` (lines 1711-1768 of homebrew.go).

### 2. Separation of LLM Extraction from Algorithmic Decision (Lines 60-68)

The key architectural insight is well-articulated:
> "LLMs excel at understanding web content; algorithms excel at consistent decisions. The LLM doesn't 'decide' which source to use - it extracts data, and the algorithm decides."

This principle correctly mirrors the existing QualityFilter pattern in `internal/discover/quality_filter.go`, which applies per-registry thresholds using OR logic.

### 3. Defense-in-Depth Layers (Lines 735-775)

The six defense layers form a coherent security model:
1. Input Normalization (existing)
2. HTML Stripping (new)
3. URL Validation (new)
4. GitHub API Verification (new)
5. User Confirmation (new)
6. Sandbox Validation (existing)

Each layer addresses distinct threat vectors without redundancy.

### 4. Error Type Hierarchy (Lines 776-807)

The error types are well-designed:
- `TimeoutError`, `BudgetError`, `VerificationError`, `ConfirmationDeniedError`
- Clear `IsRetryable()` method for error classification
- Consistent with existing patterns in `internal/builders/errors.go`

### 5. Phased Implementation Plan (Lines 874-999)

The 8-phase rollout is realistic:
- Phase 1-5 deliver core functionality
- Phase 6 (alternative search providers) is correctly marked optional
- Phase 8 (non-deterministic builder) is properly deferred with explicit dependency

---

## Concerns and Issues

### 1. Session Architecture Mismatch (Lines 359-377) - **Medium Severity**

**Issue**: The design proposes `LLMDiscoverySession` following the "HomebrewSession pattern," but there's a conceptual mismatch.

**Current Codebase**:
- `BuildSession` interface (builder.go:178-192) has `Generate()` and `Repair()` methods
- `HomebrewSession` implements `BuildSession` and produces recipes
- Discovery produces `DiscoveryResult`, not recipes

**Design Proposal**:
```go
type LLMDiscoverySession struct {
    provider     llm.Provider
    messages     []llm.Message
    // ...
}
```

The session "follows the HomebrewSession pattern" but doesn't implement `BuildSession`. This is acknowledged (line 380: "discovery doesn't generate recipes"), but it creates inconsistency.

**Recommendation**: Either:
1. Define a new `DiscoverySession` interface in `internal/discover/` that parallels `BuildSession` semantics
2. Or clarify that this is an internal implementation detail, not a reusable pattern

**Line Reference**: 359-380

---

### 2. Deferred extract_source Schema (Lines 613-621) - **High Severity**

**Issue**: The most critical interface - the `extract_source` tool schema - is punted to implementation:
> "Exact schema to be designed during Phase 2 implementation."

This is problematic because:
- The schema defines the contract between LLM and verification/decision layers
- It determines what quality signals are available for threshold filtering
- It affects how deterministic vs. non-deterministic results are routed

The design mentions two result types (`builder` and `instructions`) but doesn't specify:
- What fields are required vs. optional
- How `confidence` is represented (0-100? normalized?)
- What `evidence` looks like (array of strings? structured objects?)

**Recommendation**: Define the schema now. Even a draft that may change is better than deferring entirely. Example:

```json
{
  "result_type": "builder",
  "builder": "github",
  "source": "stripe/stripe-cli",
  "confidence": 95,
  "evidence": ["Official Stripe docs link here", "8k+ stars"],
  "warnings": []
}
```

**Line Reference**: 613-621, 277

---

### 3. Non-Deterministic Result Handling is Underspecified (Lines 93-109) - **Medium Severity**

**Issue**: The design acknowledges `instructions` result types but routes them to a "future subsystem" without specifying the interim behavior.

What happens when LLM discovery returns an `instructions` result today?
- Does it display the instructions to the user?
- Does it fail with an error?
- Does it log and continue to "not found"?

**Current design text** (line 275):
> "Capturing these findings enables: 1) Displaying helpful instructions to the user (immediate value)"

But the implementation phases don't include this display logic until Phase 8.

**Recommendation**: Add explicit handling in Phase 2 or Phase 5 for `instructions` results. Minimum viable: display the URL and instruction text, then exit with a message like "Manual installation required. See: <url>"

**Line Reference**: 93-109, 269-277

---

### 4. Quality Threshold Logic Inconsistency (Lines 683-708) - **Low Severity**

**Issue**: The `passesThresholds()` function mixes two different threshold patterns:

```go
// Confidence is a gate (AND logic)
if result.Confidence < 70 {
    return false
}

// Quality signals use OR logic
if metadata.Stars >= 50 {
    return true
}

// Very strong signals can override (more OR logic)
if metadata.Stars >= 500 {
    return true
}
```

The "very strong signals" override (Stars >= 500) is redundant - if Stars >= 50 already passes, Stars >= 500 will never be reached as a separate condition.

**Recommendation**: Remove the redundant condition or clarify the intent. If the intent was "high stars can override low confidence," the logic should be:

```go
if result.Confidence >= 70 {
    // Normal threshold logic
} else if metadata.Stars >= 500 {
    // Very high stars overrides low confidence
}
```

**Line Reference**: 683-708

---

### 5. Fork Detection Logic Gap (Lines 671-679) - **Low Severity**

**Issue**: Fork detection fetches parent metadata but the logic for "suggest parent if 10x stars" (line 676) isn't shown in the verification flow code.

The `verifyGitHub()` function returns `GitHubMetadata` with `Fork`, `ParentRepo` fields, but there's no code path showing how the "suggest parent" warning integrates with the confirmation flow or threshold logic.

**Recommendation**: Add pseudocode or clarify in Phase 4 (Quality Thresholds) how fork detection affects the decision algorithm. Currently it just says "never auto-select forks" in `passesThresholds()`.

**Line Reference**: 671-679, 688-690

---

### 6. DDG Scraper Fragility (Lines 519-539, 389) - **Medium Severity**

**Issue**: The design acknowledges DDG endpoint instability as an uncertainty but the mitigation strategy is weak:
> "Mitigation: the lite POST endpoint is a fallback"

The DDG HTML scraper depends on:
1. CSS class names (`result__a`, `result__snippet`) remaining stable
2. No bot detection/CAPTCHA
3. HTML structure not changing

**Recommendation**: Add a health check or fallback chain:
1. Try DDG HTML endpoint
2. On failure, try DDG lite POST endpoint
3. On failure, emit clear error with suggestion to use `--search-provider=tavily`

This should be explicit in Phase 1, not an afterthought.

**Line Reference**: 519-539, 389, 420-424

---

### 7. Resolver Interface Alignment (Lines 556-567)

**Issue**: The `LLMDiscoverySession.Discover()` method signature:

```go
func (s *LLMDiscoverySession) Discover(ctx context.Context, toolName string) (*DiscoveryResult, error)
```

But `LLMDiscovery` (the stub in llm_discovery.go) implements the `Resolver` interface:

```go
func (d *LLMDiscovery) Resolve(_ context.Context, _ string) (*DiscoveryResult, error)
```

The design shows a session with a `Discover()` method but the existing codebase has a resolver with a `Resolve()` method. The integration point is unclear.

**Recommendation**: Show how `LLMDiscovery` (the resolver) creates and uses `LLMDiscoverySession` (the session). Suggested:

```go
func (d *LLMDiscovery) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
    session, err := d.newSession(ctx)
    if err != nil {
        return nil, nil // No LLM available, soft miss
    }
    defer session.Close()
    return session.Discover(ctx, toolName)
}
```

**Line Reference**: 556-567, current llm_discovery.go

---

### 8. Cost Tracking Integration (Lines 979-986) - **Low Severity**

**Issue**: Phase 7 mentions "Add cost tracking to LLMDiscoverySession" and "Integrate with LLMStateTracker for budget checks," but the `LLMDiscoverySession` struct already has `totalUsage llm.Usage` (line 374).

The design doesn't clarify:
- When budget is checked (before first LLM call? before each turn?)
- How discovery cost differs from generation cost in tracking
- Whether discovery hits the same daily budget as recipe generation

**Recommendation**: Clarify that discovery shares the existing budget/rate-limit infrastructure from `builders.SessionOptions` and `LLMStateTracker` interface. Add explicit check points.

**Line Reference**: 374, 979-986

---

## Architectural Red Flags

### No Major Anti-Patterns Detected

The design avoids common pitfalls:
- No God objects - responsibilities are distributed across files
- No circular dependencies - clear layer separation (discover -> llm -> http)
- No hidden state - session maintains explicit message history

### Minor Concern: Dual HTTP Client Pattern

The design shows `httpClient` in both `LLMDiscoverySession` (line 553) and passed through `genCtx` for HomebrewBuilder. This is fine but should use a shared client pattern to honor connection pooling and timeout settings.

---

## Recommendations Summary

| Priority | Recommendation |
|----------|---------------|
| High | Define `extract_source` schema now, not during implementation |
| Medium | Clarify interim handling of `instructions` result type |
| Medium | Add DDG fallback chain with health check |
| Medium | Define `DiscoverySession` interface or clarify session isn't a pattern |
| Low | Remove redundant Stars >= 500 check in threshold logic |
| Low | Show fork detection integration in confirmation flow |
| Low | Clarify budget tracking integration points |

---

## Conclusion

The design is architecturally sound. It correctly reuses existing patterns (Factory, Provider, QualityFilter) and maintains clear separation between LLM extraction and algorithmic decision-making. The tool-based search architecture is the right choice.

The main gaps are in specification completeness rather than architectural flaws. The deferred `extract_source` schema is the most significant oversight - this is a core interface that affects multiple downstream components. Defining it now would prevent integration issues during implementation.

The phased approach is practical, and the explicit acknowledgment of the "Non-Deterministic Builder" as a separate design dependency is appropriate.
