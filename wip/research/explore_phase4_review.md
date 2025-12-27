# Design Document Review: Hardcoded Version Detection

## Review Summary

This review evaluates the problem statement and options analysis for the Hardcoded Version Detection design document (`docs/DESIGN-hardcoded-version-detection.md`).

---

## 1. Problem Statement Specificity

**Assessment: Mostly specific, but has gaps**

### Strengths
- Concrete example (curl recipe with `8.11.1` in multiple places)
- Clear scope boundaries (in-scope vs out-of-scope)
- Well-defined decision drivers

### Gaps and Recommendations

1. **Missing success criteria**: The problem statement lacks quantifiable success metrics.
   - **Add**: "Detection should catch 90%+ of hardcoded versions with <5% false positive rate"
   - **Add**: "Validation runtime overhead should be <100ms per recipe"

2. **No severity distinction**: Not all hardcoded versions have equal impact.
   - A version in `url` blocks all version updates
   - A version in `source_dir` affects extraction path only
   - **Recommendation**: Define severity levels (error vs warning)

3. **Unclear on edge case volume**: How many existing recipes have this issue?
   - **Add**: Baseline data from scanning existing registry (e.g., "5 of 50 recipes have hardcoded versions")

4. **Missing user journey context**: Who creates hardcoded versions and why?
   - External contributors may not understand templating
   - Copy-paste from working examples without understanding
   - **Recommendation**: Add this context to explain why prevention > correction

---

## 2. Missing Alternatives

### Alternative A: AST-Based Detection (Not Considered)

Parse TOML into an AST and analyze structure rather than raw string patterns.

**Pros:**
- Natural boundary awareness (knows field from value)
- Can detect patterns like "URL ends with version.tar.gz"
- No regex maintenance

**Cons:**
- TOML parsing already exists; adds complexity
- May be overkill for the problem scope

**Verdict:** Worth mentioning as rejected alternative. The document's Option 2 implicitly uses structure via field rules, but could be more explicit.

### Alternative B: Machine Learning/Heuristic Learning (Not Considered)

Train a simple model on known-good recipes to detect anomalies.

**Verdict:** Over-engineered for the problem. Correct to omit, but worth a one-line mention in "Considered but rejected" section.

### Alternative C: Pre-commit Hook with Interactive Fix

Run detection at commit time and offer to replace hardcoded versions interactively.

**Pros:**
- Catches issues before PR creation
- Better contributor experience

**Cons:**
- Requires contributor tooling setup
- Deferred, not in scope

**Verdict:** Should be mentioned as future enhancement in "Out of scope" section.

### Alternative D: Template Linting (Partial Coverage)

Check that any field supporting `{version}` actually uses it when the recipe has a `[version]` section.

**Pros:**
- Simpler than pattern matching
- Zero false positives

**Cons:**
- Only works when version section exists
- Doesn't catch the curl case where action was wrong

**Verdict:** This is essentially Option 3. The document should note the relationship more clearly.

---

## 3. Pros/Cons Fairness Assessment

### Option 1: Pattern-Based Detection

| Claim | Fair? | Notes |
|-------|-------|-------|
| "Simple implementation" | Yes | Regex-based is straightforward |
| "Fast execution" | Yes | Single-pass scan |
| "False positives on tool names" | Yes | `python3`, `ncursesw6-config` are real risks |
| "Cannot distinguish context" | Yes | This is the core limitation |

**Missing Con:** Regex patterns vary by version format (semver, calver, date). Maintenance burden is understated.

**Missing Pro:** Can run without loading action registry (standalone).

### Option 2: Context-Aware Detection

| Claim | Fair? | Notes |
|-------|-------|-------|
| "Reduces false positives" | Yes | Field semantics help significantly |
| "Different rules for actions" | Yes | Core design insight |
| "More complex" | Yes | But complexity is bounded |
| "Rules must be maintained" | Partially | Could be auto-derived from action metadata |

**Missing Pro:** Aligns with existing validation architecture (per-action Preflight pattern).

**Missing Con:** Requires knowledge of all action types. New actions need rule updates.

### Option 3: Comparative Analysis

| Claim | Fair? | Notes |
|-------|-------|-------|
| "Semantic comparison" | Yes | Strongest detection when applicable |
| "Can suggest fix" | Yes | "Replace 8.11.1 with {version}" |
| "Only works with [version]" | Yes | Major limitation |

**Missing Pro:** No regex needed when version is known.

**Missing Con:** Version provider may have different format than hardcoded (e.g., `v8.11.1` vs `8.11.1`).

### Option 4: Heuristic Scoring

| Claim | Fair? | Notes |
|-------|-------|-------|
| "Nuanced detection" | Yes | But nuance may not be needed |
| "Tunable thresholds" | Yes | Adds configuration surface |
| "Over-engineered" | **Strawman language** | See below |

**Assessment:** This option feels like a strawman. The "over-engineered" label in the cons is a conclusion, not a fair criticism. A better con: "Requires calibration data and ongoing tuning."

---

## 4. Unstated Assumptions

### Assumption 1: Version patterns are recognizable

**Implicit:** Hardcoded versions will match known patterns (semver, calver, date).

**Risk:** Custom version formats (e.g., `YYYYMMDD`, `23.1.123`) may be missed.

**Recommendation:** Acknowledge this assumption explicitly and note that the system starts with common patterns and can be extended.

### Assumption 2: Actions declare version-sensitive fields

**Implicit:** The `ActionVersionRules` map will be comprehensive and accurate.

**Risk:** New actions added without updating rules will have no detection.

**Recommendation:** Consider deriving rules from action definitions (e.g., actions that expand `{version}` in a param should flag that param).

### Assumption 3: Contributors will understand warnings

**Implicit:** Actionable warnings lead to fixes.

**Risk:** Contributors may not understand what `{version}` means or how to use it.

**Recommendation:** Link to documentation in warning messages. Consider examples in output.

### Assumption 4: download_file is always static

**Implicit:** `download_file` never uses version placeholders.

**Reality:** Looking at the codebase, `download_file` is the primitive action post-decomposition. The design correctly handles this. But the assumption should be explicit.

### Assumption 5: Pattern matching is sufficient

**Implicit:** Regex can reliably distinguish versions from non-versions.

**Counterexamples from codebase:**
- `openssl-3.6.0` in rpath (cmake.toml line 19) - this IS hardcoded but appears in `set_rpath`
- `python3`, `ncursesw6-config` - tool names with digits

**Recommendation:** The design mentions exclusion patterns but should formalize the allowlist approach.

---

## 5. Strawman Analysis

### Is Option 4 a Strawman?

**Yes, partially.** The framing suggests Option 4 is "over-engineered" without providing a fair case for when scoring would be appropriate. The design could:

1. **Remove Option 4** if it's not a serious consideration, OR
2. **Steelman it**: "Scoring is appropriate when detection volume is high enough to need triaging. For tsuku's recipe volume (~50 recipes), this is unnecessary."

### Is Option 1 a Strawman?

**No.** Option 1 is presented fairly as a baseline. The design correctly identifies its limitations and explains why Option 2 builds on it.

### Is Option 3 a Strawman?

**No.** Option 3 has a legitimate use case (when version is known) and legitimate limitation (requires version section). The design correctly notes that elements of Option 3 inform the chosen approach.

---

## 6. Additional Observations

### Existing Pattern: Preflight Validation

The download action already has this check (line 53-56 of download.go):

```go
// ERROR: URL without variables - should use download_file instead
if hasURL && url != "" && !strings.Contains(url, "{") {
    result.AddError("download URL contains no variables; use 'download_file' action for static URLs")
}
```

This is essentially hardcoded version detection scoped to the `download` action. The design should:
1. Acknowledge this existing pattern
2. Consider extending Preflight rather than adding parallel detection

### Existing Pattern: Redundancy Detection

`internal/version/redundancy.go` detects redundant `[version]` sections by comparing explicit config to inferred sources. This is a similar pattern:
- Analyze recipe structure
- Compare against expectations
- Report warnings

The design could mention this as a reference implementation.

### Field Rule Source of Truth

The design proposes `ActionVersionRules` as a map. Consider whether this should be:
1. A static map (as proposed)
2. Derived from action Preflight (actions declare their version-sensitive params)
3. Part of action metadata (actions.Action interface gains `VersionSensitiveFields()`)

Option 2 or 3 would reduce maintenance burden but add complexity.

---

## 7. Recommendations Summary

### High Priority

1. **Add success criteria** to problem statement (false positive rate, detection rate)
2. **Acknowledge existing Preflight pattern** in download action
3. **Revise Option 4 framing** to remove strawman language or drop the option
4. **Make assumptions explicit** in a dedicated section

### Medium Priority

5. **Add severity levels** (warning vs error) based on field impact
6. **Consider action-derived rules** instead of static map
7. **Link redundancy.go as reference** for implementation pattern

### Low Priority

8. **Mention pre-commit hook** as future enhancement
9. **Add baseline data** from existing recipe scan
10. **Consider AST-based approach** as rejected alternative with rationale

---

## 8. Conclusion

The design document is well-structured and the chosen approach (Option 2) is sound. The main issues are:

1. **Strawman concern**: Option 4's "over-engineered" label should be replaced with objective criteria
2. **Missing assumptions**: Several implicit assumptions should be explicit
3. **Missing success criteria**: No way to evaluate if implementation succeeds
4. **Existing pattern oversight**: The download action already does partial detection

Overall, the design is ready for implementation with minor revisions to address these concerns. The core insight (context-aware detection with field rules) correctly balances precision and maintenance burden.
