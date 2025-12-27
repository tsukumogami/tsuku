# Architecture Review: Platform Tuple Support in When Clauses

## Review Date
2025-12-27

## Document Under Review
`/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/DESIGN-when-clause-platform-tuples.md`

## Executive Summary

The architecture is **implementable with minor clarifications needed**. The decision to use a structured `WhenClause` type (Option 2) is sound given the low migration cost (2 recipes). The implementation phases are correctly sequenced with appropriate dependencies. However, several areas need clarification to ensure smooth implementation.

**Key Recommendation:** Proceed with implementation with the clarifications and considerations outlined below.

---

## 1. Architectural Clarity Assessment

### 1.1 Overall Architecture: CLEAR

**Strengths:**
- Three-layer separation (data, unmarshaling, execution) is well-defined
- Component ownership is clear (types.go, plan_generator.go, platform.go)
- Data flow diagrams show complete lifecycle from TOML → validation → execution

**Minor Clarifications Needed:**

#### Clarification 1: TOML Array Unmarshaling Details
**Issue:** The design shows `Platform []string` in the `WhenClause` struct but doesn't specify how BurntSushi/toml will handle the TOML array → Go slice conversion.

**Current TOML unmarshaling pattern:**
```go
// From internal/recipe/types.go:199-206
if when, ok := stepMap["when"].(map[string]interface{}); ok {
    s.When = make(map[string]string)
    for k, v := range when {
        if strVal, ok := v.(string); ok {
            s.When[k] = strVal
        }
    }
}
```

**New pattern needed:**
```go
// Proposed: Handle both string and []interface{} values
if when, ok := stepMap["when"].(map[string]interface{}); ok {
    whenClause := &WhenClause{}

    // Handle platform array
    if platform, ok := when["platform"].([]interface{}); ok {
        for _, p := range platform {
            if strVal, ok := p.(string); ok {
                whenClause.Platform = append(whenClause.Platform, strVal)
            } else {
                return fmt.Errorf("when.platform must be array of strings")
            }
        }
    }

    // Handle legacy OS/Arch strings
    if os, ok := when["os"].(string); ok {
        whenClause.OS = os
    }
    // ... similar for arch, package_manager

    s.When = whenClause
}
```

**Recommendation:** Add this unmarshaling logic detail to Phase 2 implementation notes.

---

#### Clarification 2: Validation Error Handling Strategy
**Issue:** The design mentions "validation error" for mutually exclusive fields but doesn't specify where/when this occurs.

**Two validation points:**
1. **Unmarshal time** (in `Step.UnmarshalTOML()`) - syntax errors, type mismatches
2. **Load time** (in `ValidateStepsAgainstPlatforms()`) - semantic errors, platform constraints

**Recommendation:**
- Mutually exclusive field check → Unmarshal time (fail fast)
- Platform tuple validation against supported platforms → Load time (existing pattern)
- Invalid tuple format (no `/`) → Load time (consistent with install_guide validation)

Add this clarification to Phase 2 and Phase 4 deliverables.

---

#### Clarification 3: Empty Platform Array Semantics
**Design states:** `platform = []` means "match no platforms"

**Question:** Is there a valid use case for this? If not, should it be a validation error instead?

**Comparison with other fields:**
- Empty `Steps = []` is valid (no-op recipe)
- Empty `supported_os = []` means "no platforms supported"
- `when = {}` means "match all platforms"

**Consistency check:**
```toml
# Case 1: Omit when clause entirely
[[steps]]
action = "download"
# Executes on: all platforms ✓

# Case 2: Empty when map
[[steps]]
action = "download"
when = {}
# Executes on: all platforms ✓

# Case 3: Empty platform array
[[steps]]
action = "download"
when = { platform = [] }
# Executes on: no platforms ✓ (matches design)
```

**Recommendation:** Current semantics are consistent. Empty array = "never execute" is valid for conditional compilation scenarios.

---

### 1.2 Component Interfaces: CLEAR

**WhenClause API is well-defined:**
```go
type WhenClause struct {
    Platform       []string
    OS             string   // Deprecated
    Arch           string   // Deprecated
    PackageManager string
}

func (w *WhenClause) IsEmpty() bool
func (w *WhenClause) Matches(os, arch string) bool
```

**Strengths:**
- Clear separation of concerns (Matches() encapsulates all logic)
- Deprecation path is explicit (OS/Arch fields marked but supported)
- Runtime vs. plan-time distinction (package_manager ignored in Matches)

**Missing Interface Detail:**

#### Issue: Package Manager Handling in Matches()
**Design shows:** `Matches(os, arch string)` signature but `PackageManager` field exists.

**Current behavior (from plan_generator.go:230):**
```go
// Check package_manager condition (always true for plan generation)
// Package manager conditions are runtime checks, not plan-time checks
```

**Question:** Should `Matches()` method:
1. Ignore `PackageManager` field entirely (plan-time only)?
2. Accept optional `packageManager string` parameter for runtime checks?
3. Have separate `MatchesRuntime(os, arch, pkgMgr string)` method?

**Recommendation:** Keep current design (option 1). Package manager checks remain runtime-only. The `Matches()` method only handles platform tuples, OS, and Arch. Document this explicitly:

```go
// Matches returns true if the clause matches the given platform.
// Note: PackageManager is a runtime check and is not evaluated here.
// This method is used for plan generation (compile-time filtering).
func (w *WhenClause) Matches(os, arch string) bool
```

---

## 2. Missing Components Assessment

### 2.1 Identified Gaps

#### Gap 1: Deprecation Warning System (Minor)
**Missing:** Implementation details for emitting deprecation warnings for legacy OS/Arch fields.

**Current logging system:** Executor has `logger *log.Logger` field, but recipe loading doesn't have logging context.

**Options:**
1. Add warnings during validation (`ValidateStepsAgainstPlatforms()`)
2. Add warnings during unmarshaling (harder - no logger context)
3. Silent deprecation (rely on documentation only)

**Recommendation:** Add deprecation warnings in `ValidateStepsAgainstPlatforms()`. This function already returns `[]error`, extend it to return warnings separately or prefix warnings in error messages.

**Suggested API:**
```go
// ValidateStepsAgainstPlatforms returns (errors, warnings)
func (r *Recipe) ValidateStepsAgainstPlatforms() (errors []error, warnings []string) {
    // ...
    if step.When != nil && (step.When.OS != "" || step.When.Arch != "") {
        warnings = append(warnings, fmt.Sprintf(
            "step %d uses deprecated when.os/when.arch; use when.platform instead",
            i,
        ))
    }
}
```

**Impact:** Low. This is a nice-to-have, not a blocker. Can be added in Phase 4 or deferred to Phase 6 (documentation).

---

#### Gap 2: TOML Serialization (Minor)
**Missing:** Inverse operation - how to serialize `WhenClause` back to TOML for recipe hashing/exports.

**Current pattern (from types.go:92):**
```go
func (r *Recipe) ToTOML() ([]byte, error) {
    var buf bytes.Buffer
    encoder := toml.NewEncoder(&buf)
    if err := encoder.Encode(r); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}
```

**Question:** Will `toml.Encode(WhenClause)` produce correct TOML?

**Likely output:**
```toml
[when]
platform = ["darwin/arm64", "linux/amd64"]
os = ""
arch = ""
package_manager = ""
```

**Problem:** Empty fields will be serialized. Need custom `MarshalTOML()` to omit empty fields.

**Recommendation:** Add custom marshaling in Phase 2:
```go
func (w *WhenClause) MarshalTOML() (interface{}, error) {
    m := make(map[string]interface{})
    if len(w.Platform) > 0 {
        m["platform"] = w.Platform
    }
    if w.OS != "" {
        m["os"] = w.OS
    }
    if w.Arch != "" {
        m["arch"] = w.Arch
    }
    if w.PackageManager != "" {
        m["package_manager"] = w.PackageManager
    }
    return m, nil
}
```

**Impact:** Medium. Recipe hashing (used in `computeRecipeHash()`) relies on TOML serialization. Without this, hash values will change even for semantically identical recipes.

---

#### Gap 3: Migration Tooling (Optional)
**Missing:** The design mentions "migration script (optional, only 2 recipes)" but doesn't specify what this entails.

**Affected recipes (from design):** gcc-libs.toml, nodejs.toml

**Migration pattern:**
```toml
# Before
[[steps]]
when = { os = "linux" }

# After (if all linux architectures)
[[steps]]
when = { platform = ["linux/amd64", "linux/arm64"] }

# Or (if still using legacy, with warning)
[[steps]]
when = { os = "linux" }  # Deprecated: use platform instead
```

**Question:** Are these recipes still in the repository?

**Check performed:**
```bash
$ find . -name "gcc-libs.toml" -o -name "nodejs.toml"
(no results in /home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/recipes/)
```

**Finding:** The design document states "2 recipes total: gcc-libs.toml and nodejs.toml" but these files don't exist in the current repository. This may be outdated information or these recipes haven't been committed yet.

**Recommendation:**
1. Verify actual current `when` clause usage: `grep -r "when =" recipes/`
2. If no recipes use `when` currently, migration is not needed
3. Update Phase 5 based on actual usage

**Impact:** Low if recipes don't exist. Zero migration cost makes breaking change even more attractive.

---

### 2.2 Interface Completeness

**All required interfaces are present:**
- ✅ WhenClause struct definition
- ✅ IsEmpty() method
- ✅ Matches() method
- ✅ Step.When type change
- ✅ shouldExecuteForPlatform() signature update
- ✅ ValidateStepsAgainstPlatforms() extension

**Optional additions identified:**
- ⚠️ MarshalTOML() for correct serialization (recommended)
- ⚠️ Deprecation warning system (nice-to-have)

---

## 3. Implementation Phase Sequencing

### 3.1 Dependency Analysis

**Current phase order:**
1. Core Type Definition (no deps)
2. TOML Unmarshaling (depends on Phase 1)
3. Execution Layer Updates (depends on Phase 2)
4. Validation Layer (depends on Phase 3)
5. Recipe Migration (depends on Phase 4)
6. Documentation (depends on Phase 5)

**Dependency validation:**
✅ Phase 1 → Phase 2: Correct (need WhenClause before unmarshaling)
✅ Phase 2 → Phase 3: Correct (need unmarshaling before execution can consume)
✅ Phase 3 → Phase 4: **Question mark** - see issue below
✅ Phase 4 → Phase 5: Correct (validation must pass before migration complete)
✅ Phase 5 → Phase 6: Correct (document final state)

---

### 3.2 Sequencing Issues

#### Issue: Phase 3-4 Order
**Current design:** Phase 3 (Execution) → Phase 4 (Validation)

**Analysis:**
- Validation layer (`ValidateStepsAgainstPlatforms()`) doesn't depend on execution logic
- Validation can be implemented immediately after unmarshaling (Phase 2)
- Tests for validation don't require working execution layer

**Recommendation:** Consider reordering:

**Option A (current):**
1. Core Type
2. Unmarshaling
3. Execution ← may fail validation tests
4. Validation

**Option B (proposed):**
1. Core Type
2. Unmarshaling
3. Validation ← catches bad recipes early
4. Execution ← already validated

**Rationale:** Validation should come before execution to catch errors earlier in development. This follows the data flow: TOML → Parse → Validate → Execute.

**Impact:** Low. Both orders work, but Option B is slightly more intuitive.

---

### 3.3 Phase Granularity

**Assessment:** Phase granularity is appropriate.

**Rationale:**
- Each phase produces testable deliverables
- Phases are small enough to review independently
- Natural rollback points if issues arise

**No changes recommended.**

---

### 3.4 Missing Phases?

**Identified gap:** No phase for integration testing across the full pipeline.

**Recommendation:** Add Phase 5.5 (between Migration and Documentation):

**Phase 5.5: End-to-End Integration Testing**
- Load recipe with platform tuples
- Generate plan for each supported platform
- Verify correct step filtering
- Test backwards compatibility with legacy OS/Arch
- Test error cases (invalid tuples, mutually exclusive fields)

**Deliverables:**
- Integration test in `internal/executor/plan_generator_test.go`
- Test recipe fixtures in `testdata/recipes/` with platform tuple examples

**Dependencies:** Phase 4 (validation must work), Phase 5 (if migration needed)

**Impact:** Medium. Integration tests are critical for catching cross-layer issues.

---

## 4. Simpler Alternatives Analysis

### 4.1 Design Decision Review

**Chosen option:** Option 2 (Structured WhenClause Type)
**Rejected options:**
- Option 1 (CSV string hack in map[string]string)
- Option 3 (Separate when_platform field)

**Was the right option chosen?** **YES**, given the constraints.

---

### 4.2 Overlooked Alternatives

#### Alternative 4: Extend Existing Map with Type Assertions

**Concept:** Keep `Step.When` as `map[string]string` but allow comma-separated platform tuples in a new `"platforms"` key (note plural).

```toml
[[steps]]
action = "download"
when = { platforms = "darwin/arm64,linux/amd64" }
```

**Implementation:**
```go
// Step.When remains map[string]string
func shouldExecuteForPlatform(when map[string]string, targetOS, targetArch string) bool {
    // Check new platforms key (comma-separated)
    if platforms, ok := when["platforms"]; ok {
        tuple := fmt.Sprintf("%s/%s", targetOS, targetArch)
        for _, p := range strings.Split(platforms, ",") {
            if strings.TrimSpace(p) == tuple {
                return true
            }
        }
        return false
    }

    // Legacy OS/Arch logic...
}
```

**Pros:**
- No type signature change (Step.When stays map[string]string)
- Minimal code changes
- TOML syntax works with current unmarshaling

**Cons:**
- CSV parsing is fragile (what if platform contains comma?)
- Not idiomatic Go (arrays are better than delimited strings)
- Harder to validate (need to split then check each)
- Doesn't solve the general problem (still a map of strings)

**Verdict:** **Inferior to Option 2**. This is essentially Option 1 with a different key name. The design document already correctly rejected this approach.

---

#### Alternative 5: Union Type with Discriminator

**Concept:** Make `when` accept either a map (legacy) or an array (new).

```toml
# Legacy style
[[steps]]
when = { os = "linux" }

# New style (totally different structure)
[[steps]]
when = ["darwin/arm64", "linux/amd64"]
```

**Implementation:**
```go
type Step struct {
    Action string
    When   WhenCondition  // Interface or sum type
    // ...
}

type WhenCondition interface {
    Matches(os, arch string) bool
}

type LegacyWhen struct { OS, Arch, PackageManager string }
type PlatformWhen struct { Platforms []string }
```

**Pros:**
- Clean separation of old and new
- TOML syntax is very concise for new style

**Cons:**
- **Major breaking change:** TOML syntax changes completely (not backwards compatible)
- Type assertions everywhere: `when.(LegacyWhen)` vs `when.(PlatformWhen)`
- Harder to handle package_manager in new style
- Validation becomes complex (which type am I?)

**Verdict:** **Worse than Option 2**. Doesn't provide backwards compatibility and increases complexity without clear benefits.

---

#### Alternative 6: No Breaking Change - Additive Only

**Concept:** Keep `Step.When` as `map[string]string`, add `Step.WhenPlatforms []string` (essentially Option 3 from the design).

```toml
[[steps]]
action = "download"
when = { os = "linux" }           # Legacy, still works
when_platforms = ["darwin/arm64"]  # New field
```

**Why was this rejected in the design?**
> "Inconsistent with existing pattern: Other when conditions use the when map"

**Counterargument:** Is consistency worth the breaking change?

**Analysis:**
- Current usage: 0 recipes (gcc-libs.toml and nodejs.toml don't exist)
- Future recipes: Would use new field exclusively
- Migration cost: Zero
- Maintenance burden: Two code paths forever

**Recommendation:** **Consider re-evaluating Option 3** given zero current usage.

If no recipes currently use `when` clauses:
- No migration needed regardless of approach
- Breaking change affects zero recipes
- Consistency argument is weaker (no existing pattern to break)

**Updated verdict:** Option 2 is still better for long-term maintainability. The structured type approach provides better foundation even if current usage is zero.

---

### 4.3 Radical Alternative: Remove When Clauses Entirely?

**Concept:** Instead of making `when` more powerful, remove it and use separate recipes per platform.

```toml
# Instead of:
name = "python"
[[steps]]
when = { platform = ["darwin/arm64"] }

# Use:
name = "python-darwin-arm64"  # Separate recipe
```

**Pros:**
- Simpler: No conditional logic in executor
- Explicit: Recipe filename shows platform
- Precedent: Some package managers do this (e.g., platform-specific wheels)

**Cons:**
- **Massive duplication:** Most steps are platform-agnostic
- **Maintenance nightmare:** Change one step, update N recipes
- **Breaks tsuku UX:** Users expect `tsuku install python`, not `tsuku install python-darwin-arm64`

**Verdict:** **Non-starter**. When clauses solve a real problem. This alternative doesn't scale.

---

### 4.4 Alternative Summary

| Alternative | Pros | Cons | Verdict |
|------------|------|------|---------|
| Alt 4: CSV in map | No type change | Fragile parsing, not idiomatic | Inferior to Option 2 |
| Alt 5: Union type | Clean separation | Not backwards compatible, complex | Worse than Option 2 |
| Alt 6: Additive field | No breaking change | Two code paths forever | Worth reconsidering, but Option 2 still better |
| Alt 7: Remove when | No conditionals | Massive duplication | Non-starter |

**Final recommendation:** Stick with Option 2 (Structured WhenClause Type) as designed.

---

## 5. Additional Considerations

### 5.1 Backwards Compatibility Strategy

**Design states:** "Support both formats during one release cycle with deprecation warnings"

**Questions:**

#### 5.1.1 What constitutes "one release cycle"?
- One minor version (e.g., v0.10.0 with warnings, v0.11.0 removes support)?
- One major version (e.g., v1.0 with warnings, v2.0 removes)?
- Time-based (e.g., 6 months)?

**Recommendation:** Define this in Phase 4 or 6. Suggest: Two minor versions (v0.10.0 warns, v0.11.0 still warns, v0.12.0 removes).

#### 5.1.2 What happens if user combines old and new?
```toml
# User mistake: both formats
[[steps]]
when = { os = "linux", platform = ["darwin/arm64"] }
```

**Design states:** "Validation fails if both platform and os/arch are specified (mutually exclusive)"

**Recommendation:** Ensure error message is helpful:
```
Error: step 3 has both legacy (os/arch) and new (platform) when fields.
Please use only one format. Migrate to platform arrays:
  when = { platform = ["linux/amd64", "linux/arm64"] }
```

---

### 5.2 Testing Strategy

**Design mentions test coverage in each phase but doesn't specify test types.**

**Recommended test pyramid:**

**Unit tests (Phases 1-4):**
- `WhenClause.Matches()` - 15-20 test cases
- `shouldExecuteForPlatform()` - update existing tests
- `ValidateStepsAgainstPlatforms()` - add platform tuple cases
- Unmarshaling edge cases (malformed TOML, type mismatches)

**Integration tests (Phase 5.5, recommended addition):**
- Full recipe load → validate → generate plan → execute cycle
- Test fixtures in `testdata/recipes/` with platform tuples
- Backwards compatibility: recipes with legacy OS/Arch still work

**Regression tests:**
- Ensure no existing tests break (update them as needed)
- Verify hash stability (if `MarshalTOML()` is implemented)

**Current test locations:**
- `internal/recipe/types_test.go` - add WhenClause tests
- `internal/executor/plan_generator_test.go` - update shouldExecuteForPlatform tests
- `internal/recipe/platform_test.go` - add validation tests

---

### 5.3 Error Message Quality

**Design doesn't specify error messages for common failures.**

**Recommended error messages:**

```go
// Invalid platform tuple format
"when.platform[%d] = '%s' is not a valid platform tuple (expected 'os/arch' format)"

// Platform not supported by recipe
"when.platform[%d] = '%s' is not in the recipe's supported platforms: %v"

// Mutually exclusive fields
"when clause cannot specify both 'platform' and 'os'/'arch' fields (choose one format)"

// Empty platform array
// (No error - this is valid per design, means "never execute")

// TOML unmarshaling failure
"step %d: when.platform must be an array of strings, got %T"
```

**Recommendation:** Add these to Phase 2 (unmarshaling) and Phase 4 (validation) implementation notes.

---

### 5.4 Documentation Scope

**Phase 6 deliverables:**
- Update `docs/platform-tuple-support.md` with when clause examples
- Add migration guide for recipe authors
- Update GUIDE-actions-and-primitives.md with when clause usage

**Missing documentation:**
- **API reference:** Document WhenClause struct in godoc
- **Error message reference:** List all validation errors and how to fix them
- **Examples in recipes:** Add commented examples to existing recipes or testdata

**Recommendation:** Expand Phase 6 to include:
- Godoc comments on WhenClause, IsEmpty(), Matches()
- Inline examples in package documentation
- Update CLI help text if `when` clauses are user-facing (are they?)

---

### 5.5 Performance Implications

**Design doesn't mention performance impact.**

**Analysis:**

**Before (map[string]string):**
```go
func shouldExecuteForPlatform(when map[string]string, targetOS, targetArch string) bool {
    if len(when) == 0 { return true }
    if when["os"] != targetOS { return false }  // O(1) map lookup
    if when["arch"] != targetArch { return false }
    return true
}
```

**After (WhenClause with Platform array):**
```go
func (w *WhenClause) Matches(os, arch string) bool {
    if len(w.Platform) > 0 {
        tuple := fmt.Sprintf("%s/%s", os, arch)  // String allocation
        for _, p := range w.Platform {           // O(n) iteration
            if p == tuple { return true }
        }
        return false
    }
    // Legacy path...
}
```

**Impact:**
- **Plan generation:** Called once per step per recipe load (not hot path)
- **Typical array size:** 1-4 elements (design examples show max 4 platforms)
- **Allocation overhead:** One `fmt.Sprintf()` per step (negligible)

**Verdict:** Performance impact is **negligible**. This is not a hot path and array sizes are tiny.

**No optimization needed.**

---

### 5.6 Security Implications Review

**Design includes comprehensive security analysis:**
- ✅ Download verification: Not applicable
- ✅ Execution isolation: No new risks
- ✅ Supply chain: No new vectors
- ✅ User data: No exposure

**Additional security consideration:**

#### Denial of Service via Large Platform Arrays

**Threat:** Malicious recipe with `platform = ["a/b", "c/d", ..., "y/z"]` (1000s of entries)

**Impact:** Validation would iterate over large array, but:
- Validation happens at load time (once per recipe, not per install)
- Iteration is O(n) over supported platforms × platform array size
- Supported platforms is bounded (current recipes: 4 max)
- Platform array size has no upper bound in design

**Mitigation options:**
1. Do nothing (not a realistic attack vector)
2. Add validation limit: `if len(when.Platform) > 100 { error }`
3. Add validation limit based on supported platforms: `if len(when.Platform) > len(supportedPlatforms) { warn }`

**Recommendation:** Add sanity check in validation (option 3):
```go
if len(step.When.Platform) > len(platforms) {
    warnings = append(warnings, fmt.Sprintf(
        "step %d has more platform entries (%d) than supported platforms (%d); some are redundant",
        i, len(step.When.Platform), len(platforms),
    ))
}
```

**Impact:** Very low priority. This is a recipe quality issue, not a security issue.

---

## 6. Summary of Findings

### 6.1 Architecture Clarity: 8/10
- Core design is clear and implementable
- Minor clarifications needed for unmarshaling and validation sequence
- Data flow and component boundaries are well-defined

### 6.2 Completeness: 7/10
- All essential interfaces are present
- Missing: TOML serialization (MarshalTOML)
- Missing: Integration testing phase
- Missing: Deprecation warning system details

### 6.3 Phase Sequencing: 8/10
- Dependencies are mostly correct
- Minor improvement: Consider swapping Phase 3 and 4 (validation before execution)
- Add Phase 5.5 for integration testing

### 6.4 Alternative Analysis: 9/10
- Design correctly evaluated main alternatives
- No simpler viable alternatives identified
- Option 2 remains the best choice

### 6.5 Overall Assessment: 8/10

**Strengths:**
- Structured type approach is sound
- Backwards compatibility path is clear
- Security analysis is thorough
- Examples are comprehensive

**Weaknesses:**
- Some implementation details are underspecified (unmarshaling, serialization)
- Testing strategy could be more explicit
- Migration cost may be zero (recipes don't exist?)

---

## 7. Recommendations

### Critical (Must Address Before Implementation)
1. **Add MarshalTOML() method** to WhenClause for correct serialization (affects recipe hashing)
2. **Verify actual recipe usage** - gcc-libs.toml and nodejs.toml may not exist
3. **Clarify TOML array unmarshaling** - add code snippet to Phase 2
4. **Define deprecation timeline** - how long to support legacy OS/Arch?

### Important (Should Address During Implementation)
5. **Add Phase 5.5** for integration testing
6. **Consider reordering** Phase 3 (execution) and Phase 4 (validation)
7. **Implement deprecation warnings** in ValidateStepsAgainstPlatforms()
8. **Define error messages** for all validation failures

### Nice-to-Have (Can Defer)
9. Add sanity check for oversized platform arrays
10. Expand documentation scope to include godoc and examples
11. Consider adding recipe quality linter for redundant platform entries

---

## 8. Final Verdict

**Proceed with implementation** with the following adjustments:

**Before Phase 1:**
- Verify actual `when` clause usage in current recipes
- Update Phase 5 migration plan based on findings

**During Phase 2:**
- Add `MarshalTOML()` method to WhenClause
- Implement detailed unmarshaling logic for arrays
- Add mutually exclusive field validation

**During Phase 4:**
- Implement deprecation warnings
- Add comprehensive error messages

**After Phase 5:**
- Add Phase 5.5 for integration testing
- Test full pipeline with real recipe examples

**The architecture is sound. Implementation can proceed with confidence.**
