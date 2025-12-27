# Architecture Review: Hardcoded Version Detection

**Reviewer**: Architecture Review Agent
**Date**: 2025-12-27
**Design Document**: docs/DESIGN-hardcoded-version-detection.md

## Executive Summary

The proposed architecture is **sound and implementable**, with the context-aware detection approach (Option 2 + Option 1 elements) being the correct choice. However, this review identifies several gaps, potential simplifications, and sequencing adjustments that would strengthen the implementation.

**Overall Assessment**: Ready for implementation with minor refinements.

---

## Question 1: Is the architecture clear enough to implement?

### Verdict: Yes, with minor clarifications needed

**Strengths:**
1. Clear component placement (`internal/recipe/hardcoded.go`)
2. Well-defined data structures (`VersionFieldRule`, `ActionVersionRules`, `HardcodedVersionWarning`)
3. Explicit detection flow diagram
4. Concrete examples of version patterns to detect

**Clarifications Needed:**

1. **Step Indexing in Warnings**: The design shows 1-based step indexing for user display, but the detection flow iterates "For each Step" without specifying how to track step index. Add explicit `stepIndex` tracking to the flow.

2. **Params Access Pattern**: The design assumes direct field access via `step.Params["url"]`, but the existing codebase uses helper functions like `GetString()`. Recommend using existing `actions.GetString()` pattern for consistency:
   ```go
   // Preferred (matches existing code)
   url, hasURL := actions.GetString(step.Params, "url")

   // Design shows direct access (works but inconsistent)
   url := step.Params["url"].(string)
   ```

3. **Placeholder Detection Logic**: The design mentions `hasPlaceholder(value)` but doesn't define it. Should specify:
   ```go
   func hasVersionPlaceholder(s string) bool {
       return strings.Contains(s, "{version}") ||
              strings.Contains(s, "{version_tag}")
   }
   ```

4. **Exclusion Pattern Integration**: The `excludePatterns` are defined but the flow diagram doesn't show where they're applied. Add a step: "Filter out excluded patterns before reporting warning."

---

## Question 2: Are there missing components or interfaces?

### Missing Components Identified

1. **Suppression Mechanism**: The design mentions "recipe-level annotation for suppression" as a future enhancement, but provides no placeholder interface. Recommend defining:
   ```go
   // In types.go - future extension point
   type RecipeLintConfig struct {
       Suppress []string `toml:"suppress,omitempty"` // e.g., ["hardcoded-version"]
   }
   ```
   Even if not implemented now, this documents the extension point.

2. **Warning Categorization**: The current `ValidationWarning` struct lacks categorization. For grouping in CLI output (Phase 2), consider:
   ```go
   type ValidationWarning struct {
       Category string `json:"category"` // e.g., "hardcoded-version", "redundancy"
       Field    string `json:"field"`
       Message  string `json:"message"`
   }
   ```

3. **Missing Action Rules**: The `ActionVersionRules` map is incomplete. Based on codebase review:
   ```go
   // Missing from design - should be added:
   "download_archive": {
       {Field: "url", ExpectPlaceholder: true},
   },
   "cargo_build": {
       {Field: "source_dir", ExpectPlaceholder: true},
   },
   "go_build": {
       {Field: "source_dir", ExpectPlaceholder: true},
   },
   ```

4. **Composite Action Handling**: The design doesn't address whether detection runs on composite actions before or after decomposition. Since validation runs on recipe TOML (before decomposition), composite actions need rules too.

### Interface Alignment

The existing validation interfaces are well-suited for integration:

| Existing Interface | Proposed Usage | Alignment |
|-------------------|----------------|-----------|
| `ValidationWarning` | Direct reuse | Perfect fit |
| `ValidateRecipe()` | Call `DetectHardcodedVersions` | Need minor extension |
| `runRecipeValidations()` | Add hardcoded detection call | Single line change |

---

## Question 3: Are the implementation phases correctly sequenced?

### Current Sequence Assessment

| Phase | Description | Dependencies | Issues |
|-------|-------------|--------------|--------|
| 1 | Core Detection Logic | None | Correct |
| 2 | Integration with Validate | Phase 1 | Correct |
| 3 | CI Integration | Phase 2 | **Premature** |
| 4 | Recipe Remediation | Phase 3 | **Reorder** |

### Recommended Sequence Adjustment

**Original Phase 3 should come after Phase 4:**

1. **Phase 3 depends on clean recipes**: If CI integration runs before remediation, existing recipes will fail the new strict validation, blocking all PRs until remediation is complete.

2. **Better sequence**:
   - Phase 1: Core Detection Logic
   - Phase 2: Integration with Validate (warnings only, not strict mode)
   - Phase 3: Recipe Remediation (fix existing issues)
   - Phase 4: CI Integration (enable strict mode)

### Additional Phase Recommendation

Insert a "Dry Run" phase between current Phase 2 and 3:

**Phase 2.5: Detection Audit**
- Run detection on all existing recipes
- Analyze false positive rate
- Tune regex patterns if needed
- Document exceptions

This prevents deploying detection that generates excessive false positives.

---

## Question 4: Are there simpler alternatives we overlooked?

### Alternative 1: Leverage Existing Preflight Validation

**Current Approach**: New `DetectHardcodedVersions()` function in `internal/recipe/hardcoded.go`

**Simpler Alternative**: Extend action Preflight methods with version detection.

The `download` action's Preflight already checks for missing variables:
```go
// Existing in download.go:54
if hasURL && url != "" && !strings.Contains(url, "{") {
    result.AddError("download URL contains no variables; use 'download_file' action for static URLs")
}
```

**Pros of extending Preflight**:
- Detection co-located with action definition
- Automatic coverage as new actions are added
- Reuses existing infrastructure

**Cons**:
- Preflight runs during semantic validation, not structural
- Would need to distinguish "missing any variable" from "missing {version} specifically"
- Hardcoded version detection is a cross-cutting concern, not action-specific

**Recommendation**: Keep the separate `hardcoded.go` approach. The cross-cutting nature of version detection justifies a dedicated module. However, consider migrating the existing `download` Preflight check into the unified detection.

### Alternative 2: Simple String Matching Before Regex

**Current Approach**: Apply regex patterns to fields, then filter with exclusions.

**Simpler Alternative**: Check for `{version}` presence first, only run regex if placeholder is missing.

```go
func DetectHardcodedVersions(r *Recipe) []ValidationWarning {
    var warnings []ValidationWarning

    for i, step := range r.Steps {
        rules, ok := ActionVersionRules[step.Action]
        if !ok {
            continue
        }

        for _, rule := range rules {
            if !rule.ExpectPlaceholder {
                continue
            }

            value, ok := step.Params[rule.Field].(string)
            if !ok {
                continue
            }

            // Fast path: if placeholder present, skip regex entirely
            if hasVersionPlaceholder(value) {
                continue
            }

            // Slow path: only run regex when placeholder is missing
            if version := findVersionPattern(value); version != "" {
                warnings = append(warnings, HardcodedVersionWarning{...})
            }
        }
    }

    return warnings
}
```

This optimization avoids regex evaluation in the common case (valid recipes with placeholders).

### Alternative 3: Schema-Based Validation

**Concept**: Define expected field patterns in a declarative schema rather than code.

```toml
# validation-schema.toml
[action.download]
url = { expect_placeholder = true, version_fields = ["version", "version_tag"] }
checksum_url = { expect_placeholder = true }

[action.extract]
archive = { expect_placeholder = true }
```

**Pros**: Easier to maintain, self-documenting
**Cons**: Requires schema loading infrastructure, more complex than Go maps

**Recommendation**: Not worth the complexity for this scope. The `ActionVersionRules` map is sufficient and more performant.

---

## Additional Observations

### 1. Interaction with `download` vs `download_file` Semantics

The codebase has a clear distinction:
- `download`: Template URLs with `{version}` placeholders, checksums computed at plan time
- `download_file`: Static URLs with inline checksums, no placeholders expected

The design correctly identifies that `download_file` should be skipped in version detection. However, there's an edge case:

**Problem**: What if someone uses `download` (which requires placeholders) but provides a URL with a hardcoded version?

**Current Handling**: The `download` action's Preflight already catches this:
```go
if hasURL && url != "" && !strings.Contains(url, "{") {
    result.AddError("download URL contains no variables; use 'download_file' action for static URLs")
}
```

**Recommendation**: Document that the new hardcoded detection complements (not replaces) existing Preflight checks. Both should run.

### 2. False Positive Risk Assessment

The design's edge case handling is reasonable but should be tested against real recipes. Specific risks:

| Pattern | Risk | Mitigation |
|---------|------|------------|
| `python3` in URL | Low | Single digit, not multi-part version |
| `api/v2` | Medium | Exclusion pattern handles this |
| `openssl-3.0` | Medium | Part of tool name, needs exclusion |
| `go1.21.5` | High | Legitimate version format in Go toolchain URLs |

**Recommendation**: Before Phase 3 (CI Integration), run detection on the recipe corpus and create an exclusion list for legitimate patterns.

### 3. Performance Considerations

The design notes "minor performance cost (regex scanning per field)" but doesn't quantify. Analysis:

- Recipe validation runs frequently (every `tsuku validate` call)
- Number of fields to scan: ~2-3 per step, ~5-10 steps per recipe = ~30 regex evaluations
- Regex compilation: Should be done once (using `sync.Once` or package-level `var`)

**Recommendation**: Ensure regex patterns are compiled at package initialization, not per-call:
```go
var versionPatterns = []*regexp.Regexp{
    regexp.MustCompile(`...`), // Compiled once at startup
}
```

The design already shows this pattern, which is correct.

---

## Summary of Recommendations

### Critical (Must Address Before Implementation)

1. **Resequence phases**: Move CI integration after recipe remediation
2. **Add missing action rules**: `download_archive`, `cargo_build`, `go_build`
3. **Specify placeholder detection function**: Define `hasVersionPlaceholder()`

### Important (Should Address)

4. Add "Detection Audit" phase before CI integration
5. Consider warning categorization for grouped CLI output
6. Document interaction with existing Preflight checks

### Nice to Have

7. Define suppression mechanism placeholder for future extension
8. Add fast-path optimization (skip regex when placeholder present)
9. Create exclusion list based on recipe corpus analysis

---

## Conclusion

The Hardcoded Version Detection design is well-thought-out and ready for implementation. The context-aware approach (Option 2) is the right choice, providing precision without over-engineering. The main adjustments needed are:

1. Reordering implementation phases to avoid breaking CI during the transition
2. Adding missing action rules for complete coverage
3. Minor clarifications to the detection flow specification

With these refinements, the design should proceed to implementation with high confidence of success.
