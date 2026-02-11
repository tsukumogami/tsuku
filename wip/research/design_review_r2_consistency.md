# Design Review Round 2: Consistency and Proofreading

**Document:** DESIGN-llm-discovery-implementation.md
**Reviewer Role:** Consistency and Proofreading
**Date:** 2026-02-11

## 1. Verification of Previous Issue Fixes

### Issue A: "Claude native search" terminology (Line 394)

**Previous Problem:** Line 394 mentioned "Claude native search" which was inconsistent with the provider-transparent tool architecture described elsewhere.

**Status: FIXED**

The document now consistently uses provider-transparent language:
- Line 129: "Simplicity: Minimal new dependencies; Claude native search over third-party APIs" - **Note: This line still references "Claude native search"**
- Line 139: "Chosen: Search as a Tool (Provider-Transparent)"
- Line 389: "Uses Claude's native web search tool" - This is acceptable since it describes what happens specifically for Claude
- Line 519-521: Correctly distinguishes "For Claude/Gemini: This maps to their native search capability. For local LLMs: tsuku handles the tool call via DDG/Tavily/Brave."

**New Finding:** Line 129 still says "Claude native search over third-party APIs" which is inconsistent with the provider-transparent messaging. This should say something like "native provider search over third-party APIs" for consistency.

### Issue B: Non-deterministic handling scope

**Previous Problem:** Non-deterministic handling appeared both in-scope and out-of-scope.

**Status: FIXED**

- Lines 75-76: In-scope includes "Structured output schema supporting deterministic and non-deterministic results"
- Lines 83-84: Out-of-scope lists "Non-deterministic result handling (see Required Subsystem Designs below)"
- Lines 89-109: Required Subsystem Designs section properly explains the Non-Deterministic Builder as a deferred dependency
- Lines 811-823: Phase 8 clearly marks this as blocked on a separate design

The document now clearly distinguishes:
- **In scope:** Defining the schema to CAPTURE non-deterministic results
- **Out of scope:** Actually HANDLING/EXECUTING those results (deferred to subsystem design)

This is a logical separation and no longer contradictory.

### Issue C: Redundant Stars >= 500 override

**Previous Problem:** Document had multiple threshold values that were inconsistent.

**Status: VERIFIED**

- Line 391: "Applies deterministic thresholds (confidence >= 70 AND stars >= 50)"
- Line 413: "Stars >= 50 may exclude legitimate but obscure tools"

Threshold values are now consistent. The document uses 50 stars as the example threshold throughout.

## 2. New Issues Introduced by Changes

### Issue 2.1: Line 129 Terminology Inconsistency

**Location:** Line 129
**Problem:** "Claude native search over third-party APIs" still uses Claude-specific language in a section (Decision Drivers) that should be provider-agnostic.
**Severity:** Minor
**Recommendation:** Change to "provider native search over third-party APIs" or "native LLM search over third-party APIs"

### Issue 2.2: Threshold Values in Summary vs. Body

**Location:** Line 391 vs. Lines 330-336
**Observation:** Line 391 states specific values "confidence >= 70 AND stars >= 50" but Lines 330-336 say "Specific threshold values will be determined during implementation."
**Assessment:** This is not quite a contradiction. Line 391 appears in the Decision Outcome summary, suggesting these are proposed/initial values, while the body text correctly notes they'll be tuned. However, the disconnect could confuse readers.
**Severity:** Very Minor
**Recommendation:** Either remove specific numbers from line 391 (use "confidence and quality thresholds") or add "initial/proposed" qualifier.

## 3. Document Flow Analysis

The document flows logically:

1. **Frontmatter** (1-23): Clean decision record format
2. **Context** (43-108): Problem statement, key insight, scope, subsystem dependencies
3. **Success Criteria** (110-122): Clear metrics table
4. **Decisions** (124-378): Five numbered decisions with alternatives
5. **Outcome** (381-415): Summary and rationale
6. **Architecture** (417-703): Technical details with component tables and diagrams
7. **Implementation** (705-823): Phased roadmap
8. **Security** (825-898): Thorough threat analysis
9. **Consequences** (899-925): Balanced positive/negative assessment

The structure is well-organized. The Required Subsystem Designs table (lines 93-96) properly signals dependencies that are addressed later in Phase 8.

## 4. Remaining Terminology Issues

### Issue 4.1: Mixed "builder" terminology

The document uses "builder" in two contexts:
1. As a result type (lines 258-259): `result_type: "builder"` meaning "mappable to existing builder"
2. As a subsystem (line 95): "Non-Deterministic Builder" as a component name

This dual usage is acceptable since context makes the meaning clear, but readers unfamiliar with the codebase might be briefly confused.

### Issue 4.2: SearchProvider capitalization

- Line 164: `SearchProvider` (PascalCase, as Go type)
- Line 195: `SearchProvider` (consistent)
- Lines 452-458: `SearchProvider` (consistent)

Capitalization is consistent throughout.

### Issue 4.3: DDG abbreviation

The document uses "DDG" consistently for DuckDuckGo after first establishing the abbreviation at line 164. This is fine.

## 5. Cross-Reference Verification

### Internal References
- Line 33: References `DESIGN-discovery-resolver.md` - should exist (upstream design)
- Line 33: References `issue #1318` - external GitHub reference
- Line 722: References `#1421` (Embedded LLM Runtime) - consistent with line 729

### Code Sample Consistency
- Lines 143-155: `WebSearchTool` struct is referenced conceptually in lines 504-516 with matching schema
- Lines 177-189: `WebSearchHandler` pattern is described in lines 445-451 - consistent
- Lines 195-219: `SearchProvider` interface aligns with later usage in lines 452-458

## 6. Readiness Verdict

**Ready with Minor Fixes**

The document is well-structured and the major contradictions from Round 1 have been successfully resolved. There are two minor issues remaining:

### Required Fixes (Minor)

1. **Line 129:** Change "Claude native search over third-party APIs" to "provider native search over third-party APIs" for consistency with the provider-transparent architecture.

2. **Line 391 (Optional):** Consider adding "initial" qualifier to threshold values: "Applies initial deterministic thresholds (confidence >= 70 AND stars >= 50)" to align with the "determined during implementation" language in lines 330-336.

### Summary

| Check | Status |
|-------|--------|
| Previous contradictions fixed | Yes |
| New contradictions introduced | No (1 minor terminology issue) |
| Document flow logical | Yes |
| Terminology consistent | Mostly (1 minor issue at line 129) |
| Cross-references valid | Yes |
| Code samples consistent | Yes |

The document is ready for approval after addressing the line 129 terminology fix.
