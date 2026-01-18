# Security Analysis: Embedded Recipe List Design

**Document reviewed**: `/docs/designs/DESIGN-embedded-recipe-list.md`
**Related document**: `/docs/designs/DESIGN-recipe-registry-separation.md`
**Date**: 2026-01-18
**Reviewer role**: Security Engineer

## Executive Summary

The Embedded Recipe List design presents **low overall security risk**. This is a build-time static analysis tool that reads Go source files and recipe TOML files, then generates markdown documentation. It does not download binaries, execute external code, or handle user data.

The security considerations section correctly identifies most risk dimensions as "Not applicable" or "Low risk." However, there are minor gaps and one residual risk worth documenting.

---

## Question 1: Unconsidered Attack Vectors

### 1.1 Supply Chain Integrity of the Analysis Tool Itself

**Status**: Partially addressed but could be clearer.

The design mentions the tool uses "only Go standard library" and is "committed to the tsuku repository." However, it doesn't explicitly address:

- **Build reproducibility**: If the analysis tool produces different output on different machines (e.g., due to map iteration order in Go), it could mask malicious changes.
- **CI runner compromise**: A compromised CI runner could modify the generated EMBEDDED_RECIPES.md before commit.

**Assessment**: Low risk. Go 1.12+ guarantees deterministic map iteration when using `range` with sorted keys, and the tool already sorts output. CI runner compromise is a general risk not specific to this design.

**Recommendation**: Add a note that the tool's output must be deterministic (sorted recipe lists, no timestamps in non-comment sections) to enable reliable diff-based validation.

### 1.2 Recipe Parsing Vulnerabilities

**Status**: Not addressed.

The tool loads and parses recipe TOML files. Malformed TOML could potentially cause:
- Panic/crash during parsing
- Excessive memory allocation (billion laughs style attacks)
- Path traversal if recipe names are used to construct file paths

**Assessment**: Very low risk. The recipes are already parsed by the main tsuku codebase with the same TOML library. Any parsing vulnerability would affect the main CLI, not just this tool. Recipe content is already trusted (goes through PR review).

**Recommendation**: No action needed. This is not a new attack surface - just reusing existing parsers.

### 1.3 Stale TODO Comments Referencing Resolved Issue

**Status**: Documentation gap.

The design correctly notes that "TODO comments in `homebrew.go` and `homebrew_relocate.go` are stale and can be removed." However, these TODOs reference issue #644 which is described as "fully resolved and deployed."

**Verified in code**: The TODO comments still exist in the codebase (confirmed via grep). If someone sees these TODOs and tries to "fix" them without understanding the context, they could introduce bugs.

**Recommendation**: Remove the stale TODO comments as part of the implementation work, or file a cleanup issue.

---

## Question 2: Mitigation Sufficiency

### 2.1 Execution Isolation Mitigations

**Claimed**: "Low risk - runs at build/CI time, not runtime"

**Analysis**: The mitigations are **sufficient**. The tool:
- Only reads source files (no network, no execution)
- Writes only to stdout (redirected to file)
- Runs with standard developer/CI permissions

The validation script (`verify-embedded-recipes.sh`) does write to `/tmp` but this is standard practice and doesn't create new attack surface.

### 2.2 Supply Chain Mitigations

**Claimed**: "Low - improves security by ensuring complete bootstrap"

**Analysis**: The mitigations are **sufficient with one enhancement needed**. The design correctly identifies that:
- The tool uses only Go standard library
- Code is committed and reviewed
- CI validation ensures changes are reviewed

**Gap identified**: The design doesn't specify that regenerating EMBEDDED_RECIPES.md should trigger a diff review in PRs. Someone could merge a PR that adds a new action dependency without reviewers noticing the embedded list changed.

**Recommendation**: Add to Stage 3 (CI Workflow): "PRs that modify action code should have the updated EMBEDDED_RECIPES.md included and visible in the diff. Consider adding a CI check that fails if the embedded list changes unexpectedly."

### 2.3 Markdown Parsing Fragility

**Claimed**: "Acceptable because format is stable and well-defined"

**Analysis**: This is a **reasonable trade-off** given the alternatives. The design correctly identifies:
- JSON + Markdown (Option 2B) adds complexity
- Regex parsing (Option 2C) is more fragile

The chosen approach (regenerate-and-compare) sidesteps parsing fragility entirely - if the format changes, the regeneration reflects it.

---

## Question 3: Residual Risk to Escalate

### 3.1 Incomplete Embedded List Leading to Bootstrap Failure (Medium)

**Risk**: If the analysis tool misses a transitive dependency, the embedded recipe list will be incomplete. Users who install a tool offline would fail when an action tries to use a missing embedded recipe.

**Current mitigation**: Recipe count sanity check (10-30 range).

**Residual risk**: The sanity check catches major failures but not off-by-one errors. A single missing recipe could cause silent failures in offline installs.

**Recommendation**: Consider adding an integration test that:
1. Builds tsuku with only embedded recipes
2. Runs in network-isolated mode
3. Attempts to install a tool requiring each action type
4. Verifies all action dependencies resolve from embedded recipes

This is not blocking for the current design but should be tracked for Stage 3 (CI and Testing Adaptation).

### 3.2 EvalTime Dependency Handling Uncertainty (Low)

**Risk**: The design notes uncertainty about "whether to include EvalTime deps (needed for decomposition) or only InstallTime deps."

**Analysis**: After reviewing the codebase, EvalTime dependencies ARE needed for actions like `cargo_install`, `npm_install`, and `go_install` because these actions run `Decompose()` which requires the respective toolchain (rust, nodejs, go) to generate lock files.

If EvalTime deps are excluded, `tsuku eval` would fail offline for these actions.

**Recommendation**: Explicitly decide to include EvalTime dependencies. The current code in resolver.go already handles EvalTime deps, so the analysis tool should capture them.

---

## Question 4: "Not Applicable" Justification Review

### 4.1 Download Verification - "Not applicable"

**Justification**: "This feature does not download external artifacts."

**Assessment**: **Correct**. The tool reads local files only. Verified by reviewing the proposed implementation - no `http.Get`, no network calls.

### 4.2 User Data Exposure - "Not applicable"

**Justification**: "Does not access user data, does not transmit any data externally."

**Assessment**: **Correct**. The tool processes only:
- Action source code (internal/actions/*.go)
- Recipe definitions (internal/recipe/recipes/*.toml)
- No user-specific data, no PII, no telemetry.

### 4.3 Execution Isolation - Marked as "Low risk" not "N/A"

**Justification**: "Runs at build/CI time, not runtime; read-only access; no special permissions."

**Assessment**: **Correct characterization**. This is appropriately marked as "Low risk" rather than "N/A" because the tool does execute code, even if minimal. The risk is properly characterized.

---

## Additional Observations

### Positive Security Properties

1. **Defense in depth**: The CI validation (regenerate-and-compare) catches drift without complex parsing logic.

2. **Minimal attack surface**: Using only Go stdlib means no transitive dependency vulnerabilities to monitor.

3. **Transparency**: EMBEDDED_RECIPES.md is human-readable, making it easy for reviewers to spot unexpected changes.

4. **Existing infrastructure reuse**: Leveraging the proven resolver.go code rather than building parallel extraction logic reduces the risk of implementation bugs.

### Implementation Notes for Security

1. **Deterministic output**: Ensure recipe lists are sorted alphabetically, action lists are sorted, and no timestamps appear in the body of the markdown (only comments if at all).

2. **Error handling**: If the tool fails to load a recipe or action, it should fail loudly rather than silently omit dependencies.

3. **Idempotency**: Running the tool twice with the same input should produce byte-identical output.

---

## Summary Table

| Dimension | Claimed Risk | Actual Risk | Notes |
|-----------|--------------|-------------|-------|
| Download Verification | N/A | N/A | Correct - no downloads |
| Execution Isolation | Low | Low | Correct - build-time only |
| Supply Chain | Low | Low | Correct with minor gap (diff visibility) |
| User Data | N/A | N/A | Correct - no user data |

## Recommendations Summary

| Priority | Recommendation | Rationale |
|----------|----------------|-----------|
| Medium | Add integration test for offline action dependency resolution | Catches incomplete embedded list before users hit it |
| Low | Remove stale TODO comments for #644 | Prevents confusion during future maintenance |
| Low | Ensure EMBEDDED_RECIPES.md changes visible in PR diffs | Improves review visibility of embedded list changes |
| Info | Explicitly include EvalTime dependencies | Clarifies design decision, ensures offline eval works |

---

## Conclusion

The security analysis in DESIGN-embedded-recipe-list.md is **appropriate for the risk level**. The design correctly identifies this as a low-risk build-time tool with minimal attack surface. No blocking security issues were found.

The main residual risk (incomplete embedded list causing offline bootstrap failure) is a **correctness issue** rather than a security vulnerability. The recommended integration test would address this comprehensively.

No escalation required. Approve the design from a security perspective.
