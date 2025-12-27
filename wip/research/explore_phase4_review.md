# Design Review: Platform Tuple Support in `when` Clauses

## Executive Summary

The problem statement is well-defined and the options are viable, but the analysis contains a critical error in understanding current behavior. The design document mischaracterizes how `when.os` and `when.arch` work today, which undermines the rationale for change. Additionally, Option 2 is not actually a strawman despite appearing to be one.

## Problem Statement Analysis

### Accuracy Assessment

**Critical Error in Problem Statement:**
The design document states:
> "The existing `when` clause uses independent boolean logic... this syntax is confusing - it reads like 'darwin AND arm64' when in reality separate os/arch keys would match ANY darwin with ANY arm64"

This is **incorrect**. The actual implementation in `shouldExecuteForPlatform()` (plan_generator.go:211-234) uses AND logic:

```go
func shouldExecuteForPlatform(when map[string]string, targetOS, targetArch string) bool {
    // Check OS condition
    if osCondition, ok := when["os"]; ok {
        if osCondition != targetOS {
            return false  // OS must match
        }
    }

    // Check arch condition
    if archCondition, ok := when["arch"]; ok {
        if archCondition != targetArch {
            return false  // Arch must match
        }
    }

    return true  // Both conditions passed (AND logic)
}
```

**Current behavior:** `when = { os = "darwin", arch = "arm64" }` executes **only** on darwin/arm64 (correct AND logic).

**Corrected problem:** The real limitation is that you cannot express OR conditions like "darwin/arm64 OR linux/amd64" without duplicating the entire step.

### Real-World Use Case Validation

**Validated use case:** The example recipes (gcc-libs.toml, nodejs.toml) demonstrate the actual need:

```toml
# gcc-libs.toml - Linux-only steps
[[steps]]
action = "homebrew"
[steps.when]
os = "linux"
```

These recipes show:
1. Current when clauses work correctly for single OS/arch filtering
2. The real gap is expressing multi-tuple conditions without step duplication
3. Examples like "apply patch only on darwin/arm64 and linux/amd64" are plausible

### Consistency Claim Verification

**PR #689 context:** The design references install_guide's platform tuple support as precedent. Examining require_system.go confirms:

```go
// getPlatformGuide implements hierarchical fallback
func getPlatformGuide(installGuide map[string]string, os, arch string) string {
    // Try exact platform tuple first (e.g., "darwin/arm64")
    tuple := fmt.Sprintf("%s/%s", os, arch)
    if guide, ok := installGuide[tuple]; ok {
        return guide
    }

    // Fall back to OS-only key (e.g., "darwin")
    if guide, ok := installGuide[os]; ok {
        return guide
    }

    // Fall back to generic fallback key
    if guide, ok := installGuide["fallback"]; ok {
        return guide
    }

    return ""
}
```

**Consistency argument is valid:** install_guide already uses platform tuples with hierarchical fallback. Parallel support in when clauses would be consistent.

### Scope Assessment

**In-scope items are appropriate:**
- Platform tuple format in `when.platform` field - necessary
- Update `shouldExecuteForPlatform()` - required for implementation
- Validation against recipe's supported platforms - follows existing pattern in ValidateStepsAgainstPlatforms()
- Documentation and test coverage - standard requirements

**Out-of-scope items are correct:**
- install_guide changes already done (verified in PR #689)
- Recipe-level constraints are separate concern
- Hierarchical fallback doesn't make sense for when clauses (they're boolean filters, not lookup tables)

**Missing from scope:** Migration strategy for existing recipes. The design should clarify whether this is purely additive or if there's value in migrating existing `when.os` to `when.platform`.

## Options Analysis

### Option 1: Add `when.platform` as Array Field

**Implementation constraints:**

The design correctly identifies TOML unmarshaling complexity. Current Step.UnmarshalTOML() implementation (types.go:186-225):

```go
if when, ok := stepMap["when"].(map[string]interface{}); ok {
    s.When = make(map[string]string)
    for k, v := range when {
        if strVal, ok := v.(string); ok {
            s.When[k] = strVal  // Only handles string values
        }
    }
}
```

**Critical constraint:** `when` is stored as `map[string]string`, which cannot natively hold arrays.

**CSV storage concern:** The design mentions storing as CSV ("darwin/arm64,linux/amd64") to preserve the map[string]string type. This is inelegant but workable.

**Pros assessment:**
- "Clean TOML syntax" - TRUE, from user perspective
- "Backwards compatible" - TRUE, existing when.os/when.arch work unchanged
- "Consistent with install_guide" - TRUE, same tuple format
- "No type signature change" - MISLEADING, the type doesn't change but implementation complexity increases significantly

**Cons assessment:**
- "CSV storage is inelegant" - TRUE, increases parsing complexity
- "TOML parsing complexity" - TRUE, requires custom handling in UnmarshalTOML()

**Missing consideration:** What happens if user specifies both `when.platform` and `when.os`? Design should clarify precedence or make them mutually exclusive.

### Option 2: Replace when with Structured Type

**Strawman assessment:** The design presents this as having "Breaking change" as a major con, suggesting it's designed to fail. However, this deserves deeper analysis.

**Breaking change severity:**

Examining actual usage in recipes:
- gcc-libs.toml uses `[steps.when]` section syntax
- nodejs.toml uses `[steps.when]` section syntax
- All when clauses use TOML table syntax, not inline syntax

**Migration path exists:** A phased approach could:
1. Support both old and new formats during transition
2. Add deprecation warnings for old format
3. Provide migration script to convert recipes

**Pros assessment:**
- "Type safety" - TRUE, compile-time guarantees
- "Clean implementation" - TRUE, eliminates CSV hack
- "Future extensibility" - TRUE, adding new fields is straightforward

**Cons assessment:**
- "Breaking change" - TRUE but potentially manageable
- "Broader scope" - TRUE, touches more code
- "Migration risk" - TRUE but can be mitigated

**Missing analysis:** The design doesn't quantify the breaking change impact. With only 3-5 recipes using when clauses currently, migration cost may be low.

**Verdict:** This is NOT a strawman. If the recipe count is small, this might be the cleaner long-term solution.

### Option 3: Separate `when_platform` Field

**Assessment:** This feels like a genuine strawman.

**Pros assessment:**
- "No CSV hack" - TRUE
- "Minimal changes" - QUESTIONABLE, still requires validation logic

**Cons assessment:**
- "Inconsistent pattern" - TRUE, creates two parallel filtering mechanisms
- "Two filter mechanisms" - TRUE, confusing for users
- "Potential conflicts" - TRUE, what if both when.os="linux" and when_platform=["darwin/arm64"]?

**Missing analysis:** How would these two mechanisms interact? If they're AND'd together, it creates bizarre edge cases. If they're OR'd, it's even more confusing.

**Verdict:** This is likely a strawman, though the design doesn't explicitly acknowledge it.

## Unstated Assumptions

### Critical Assumptions Not Made Explicit

1. **Platform tuple validation timing:** When does validation happen? Load time (good) or runtime (bad)? Design mentions "validate at load time" in conventions but doesn't make this a requirement.

2. **Error handling for invalid tuples:** What happens if a recipe specifies `when.platform = ["darwin/mips64"]` but recipe doesn't support that platform? Silent ignore or load error?

3. **Interaction with existing when.os/when.arch:** Can they coexist? What's the precedence? This is mentioned as missing in Option 1 analysis.

4. **CSV delimiter choice:** Why comma? What if platform tuple format changes to use comma in future? Pipe (|) might be safer.

5. **Empty array behavior:** What does `when.platform = []` mean? Match nothing? Match everything? This should be explicit.

6. **Single-element optimization:** Should `when.platform = ["darwin/arm64"]` be optimized to `when.os="darwin", when.arch="arm64"` internally?

### Implementation Assumptions

1. **Backward compatibility guarantee:** Design assumes existing when clauses must work unchanged, but doesn't state this as a hard requirement.

2. **Validation coverage:** Assumes ValidateStepsAgainstPlatforms() will catch invalid tuples, but doesn't specify error message format.

3. **TOML encoder handling:** ToMap() method must also handle the new format for round-trip correctness, but design doesn't mention this.

## Missing Alternatives

### Alternative 4: Multi-Step Deduplication

**Description:** Instead of adding platform tuple syntax, introduce a step-level deduplication mechanism:

```toml
[[steps]]
id = "linux-specific-patch"
action = "apply_patch"
when = { os = "linux", arch = "amd64" }

[[steps]]
action = "apply_patch"
same_as = "linux-specific-patch"
when = { os = "linux", arch = "arm64" }
```

**Pros:**
- No type changes
- More flexible (can dedupe any steps, not just platform-filtered ones)
- Clear semantics

**Cons:**
- More verbose
- Introduces new concept (step references)
- Harder to validate

**Why not mentioned:** Overly complex for the problem at hand.

### Alternative 5: Expression-Based When Clauses

**Description:** Allow boolean expressions in when:

```toml
[[steps]]
action = "apply_patch"
when_expr = "(os == 'darwin' && arch == 'arm64') || (os == 'linux' && arch == 'amd64')"
```

**Pros:**
- Maximum flexibility
- Handles complex boolean logic
- No CSV storage

**Cons:**
- Requires expression parser
- Security concerns (code injection)
- Over-engineering for current needs

**Why not mentioned:** Way too complex for the actual use cases.

## Fairness Assessment

### Bias Detection

**Option 1 favored:** The design seems to favor Option 1 based on:
- More detailed implementation notes
- Pros listed first
- Cons are acknowledged but downplayed

**Option 2 dismissed prematurely:** The "breaking change" con is presented as disqualifying, but actual impact isn't quantified. If only 3-5 recipes use when clauses, migration cost is negligible.

**Option 3 likely strawman:** Minimal detail, obvious cons, no serious consideration.

### Missing Counterarguments

**Against Option 1:**
- CSV parsing adds hidden complexity that will bite during debugging
- Platform tuple list could get very long for truly cross-platform steps
- No precedent for CSV-in-map storage elsewhere in codebase

**For Option 2:**
- Clean break prevents accumulating technical debt
- Type safety prevents entire class of bugs
- Better foundation for future when clause enhancements

**Against "consistency with install_guide" argument:**
- install_guide uses hierarchical fallback (lookup table semantics)
- when clauses use boolean filtering (different semantics)
- Consistency in syntax doesn't mean consistency in purpose

## Recommendations

### Immediate Actions

1. **Correct the problem statement:** Remove the incorrect claim about current AND/OR behavior. Reframe as "limitation: cannot express OR conditions across platform tuples without step duplication."

2. **Quantify Option 2 migration cost:** Count recipes using when clauses, estimate conversion effort. If small, Option 2 becomes more attractive.

3. **Specify interaction rules:** Make explicit whether when.platform and when.os can coexist, and if so, how they interact (likely: mutually exclusive, validation error if both present).

4. **Add validation requirements:** Specify that platform tuples in when.platform must exist in Recipe.GetSupportedPlatforms(), with clear error messages.

5. **Consider phased rollout:** If Option 1 is chosen, add deprecation plan for when.os/when.arch in favor of when.platform for consistency.

### Long-Term Considerations

1. **Monitor complexity growth:** If when clauses gain more features (environment variables, runtime checks, etc.), Option 2's structured type becomes increasingly valuable.

2. **Document CSV format:** If Option 1 is chosen, document the CSV storage format as internal implementation detail, not public API.

3. **Consider validation tooling:** Provide recipe authors with a validation tool that catches platform tuple errors early.

## Decision Matrix

| Criterion | Option 1 | Option 2 | Option 3 |
|-----------|----------|----------|----------|
| User experience | Excellent | Good (after migration) | Poor |
| Implementation complexity | Medium (CSV parsing) | High (type change) | Low |
| Long-term maintainability | Medium (hidden complexity) | Excellent | Poor |
| Migration cost | Zero | Low-Medium (depends on recipe count) | Zero |
| Consistency with install_guide | High | High | Low |
| Type safety | Low | High | Low |
| Extensibility | Medium | High | Low |

**Weighted recommendation:** If recipe count with when clauses < 10: **Option 2** (clean break). If recipe count > 20: **Option 1** (minimize disruption).

## Conclusion

The design document is well-structured and the problem is real, but the analysis contains a critical factual error about current behavior. Option 2 deserves more serious consideration than presented, especially if migration cost is low. The design should be revised to:

1. Correct the AND/OR behavior description
2. Quantify migration costs for Option 2
3. Add explicit interaction rules between when.platform and when.os/when.arch
4. Specify validation error behavior
5. Consider whether Option 2 might be superior if recipe usage is currently low

The core insight remains valid: platform tuple support in when clauses would improve recipe authoring and align with install_guide patterns. The implementation path requires more careful analysis of trade-offs.
