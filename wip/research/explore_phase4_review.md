# Design Review: Structured JSON Output for CLI Commands

## Executive Summary

The design document is generally well-structured with a clear problem statement and reasonable options analysis. However, there are several areas requiring attention:

**Critical Issues:**
- The solution introduces a new regex layer that contradicts the stated goal of eliminating regex-based parsing
- Missing consideration of typed error handling as a proper alternative
- Scope creep concerns with the "out of scope" items

**Strengths:**
- Clear problem statement grounded in specific pain points
- Good use of concrete code examples
- Appropriate scope limitation for initial implementation

## Detailed Analysis

### 1. Problem Statement Specificity

**Assessment: ADEQUATE with minor gaps**

The problem statement is concrete and specific:
- References exact exit codes (6, 8) and their current misuse
- Cites specific code locations (`orchestrator.go:260`, `classifyValidationFailure`)
- Links to issue #1273 for traceability
- Provides concrete examples (batch orchestrator, CI workflows)

**Gaps:**
1. No quantification of impact: How often do these failures occur? How much time is spent debugging misclassified errors?
2. Missing user stories: "As a CI workflow author, I need to..." would clarify priorities
3. No mention of backward compatibility constraints with existing error parsers (if any external consumers exist)

**Recommendation:** Add a brief "Impact" subsection quantifying the problem's frequency and developer time cost.

### 2. Missing Alternatives

**Assessment: INCOMPLETE - one critical option missing**

The three presented options cover a spectrum from minimal (exit codes only) to maximal (everything gets JSON), but there's a glaring omission:

**Missing Option D: Typed Error Handling**

The document mentions this approach dismissively in line 124: "A typed error approach would require refactoring the install pipeline, which is a separate effort."

However, this deserves proper consideration as an alternative:

```go
// Example of what's missing
type InstallError struct {
    Code           ExitCode
    Category       ErrorCategory
    MissingRecipes []string
    Cause          error
}

func (e *InstallError) Error() string { ... }
func (e *InstallError) Unwrap() error { return e.Cause }
```

**Why this matters:**
- Eliminates all regex-based parsing (both in orchestrator AND in the CLI)
- Provides compile-time safety for error handling
- Aligns with Go 1.13+ error handling best practices
- Could be implemented incrementally (start with install command, expand later)

**Why it was likely excluded:**
- Requires refactoring install pipeline error handling
- More upfront work than Option B
- May have been deemed "too much" for the immediate need

**Recommendation:** Add Option D and explicitly evaluate it. If rejected, provide a clear rationale beyond "it's more work." Compare effort estimates and long-term maintenance costs.

### 3. Pros/Cons Fairness and Completeness

**Assessment: MOSTLY FAIR with one strawman concern**

**Option A (Exit Codes Only):**
- Pros are accurate and honest
- Cons correctly identify the `blockedBy` extraction problem
- Fair assessment overall

**Option B (Exit Codes + JSON):**
- Pros are well-articulated
- Cons downplay the regex issue (see Section 4 below)
- Missing con: Creates two parallel error classification systems (exit codes + JSON struct)
- Missing con: CLI becomes responsible for parsing its own error strings

**Option C (Full JSON Everywhere):**
- This is a strawman option
- The cons are designed to make it look unreasonable ("large scope," "over-engineering")
- The problem statement doesn't require `create` to support JSON, so bundling it here makes C look artificially bloated
- A fair version of Option C would focus only on install command but with schema versioning and external consumer support

**Evidence of strawman:**
- Line 74: "`create` already communicates fine via exit codes" - if true, why include it in Option C at all?
- Line 75: "CI workflows don't need JSON" - then why propose updating them as part of C?
- The option conflates "add JSON to install" with "add JSON everywhere + schema + all consumers" to make it look excessive

**Recommendation:**
1. Revise Option C to be "Option B + schema versioning and external consumer support" (focused on install only)
2. Add missing cons to Option B (dual classification systems, CLI self-parsing)
3. Add Option D (typed errors) as discussed above

### 4. Unstated Assumptions

**Assessment: SEVERAL CRITICAL ASSUMPTIONS ARE IMPLICIT**

**Assumption 1: Regex-based parsing is inherently fragile**
- Stated explicitly in line 48 ("regex fragility")
- However, the solution (lines 182-192) introduces a NEW regex in `extractMissingRecipes()`
- This assumption is undermined by the proposed implementation

**Assumption 2: String-based error classification is acceptable**
- Lines 123-124 justify string matching because errors are "wrapped with `fmt.Errorf`"
- This assumes it's okay to parse strings you control, but not okay to parse strings from... the same codebase
- Inconsistent with the anti-regex stance

**Assumption 3: The batch orchestrator is the only consumer**
- Line 29: "The batch orchestrator is the primary consumer"
- What about future consumers? CI workflows? User scripts?
- If there are external consumers, the JSON shape becomes a public API (versioning matters)

**Assumption 4: Success cases don't need structured output**
- Lines 24, 92: Success output doesn't need structure "yet"
- What if a consumer wants to know what version was installed, or which dependencies were pulled in?
- This assumption may be correct, but it's not validated against use cases

**Assumption 5: Exit codes 3, 5, 6, 7, 8 are sufficient**
- No discussion of whether these codes adequately distinguish all failure types
- What about permission errors? Disk full? Checksum mismatch?
- The existing exit code set is assumed to be complete

**Recommendation:**
1. Make Assumption 3 explicit: State whether external consumers are expected or not, and how that affects versioning decisions
2. Justify the regex in `extractMissingRecipes()` or acknowledge it as technical debt
3. Add a subsection listing exit codes and their mappings to validate completeness
4. Add a future work section acknowledging success case structuring may be needed later

### 5. Self-Contradiction: The Regex Problem

**Assessment: MAJOR INCONSISTENCY**

The document's central thesis is that regex-based parsing is fragile and should be eliminated:
- Line 11: "works around this by parsing stderr text with a regex"
- Line 48: "Doesn't address the regex fragility"
- Line 57: "Orchestrator can drop `classifyValidationFailure` regex entirely"

However, the proposed solution moves the regex from the orchestrator into the CLI:

**Before (orchestrator):**
```go
// Line 11 describes this
re := regexp.MustCompile(`recipe (\S+) not found in registry`)
```

**After (CLI, lines 182-192):**
```go
func extractMissingRecipes(err error) []string {
    msg := err.Error()
    var names []string
    // Match "recipe <name> not found in registry" from nested errors
    re := regexp.MustCompile(`recipe (\S+) not found in registry`)
    for _, m := range re.FindAllStringSubmatch(msg, -1) {
        names = append(names, m[1])
    }
    return names
}
```

**This is the same regex.** The problem hasn't been solved; it's been relocated.

**Why this is problematic:**
1. The CLI is now parsing its own error strings to extract structured data
2. If the error message format changes (e.g., in `install_deps.go:309`), the regex breaks
3. This is the same fragility that motivated the change in the first place

**What's actually improved:**
- The regex and the error message live in the same repo (so changes are caught by tests)
- But this improvement applies equally to the current state (orchestrator and CLI are both in tsuku repo)

**Mitigation claimed (lines 258-260):**
> `classifyInstallError` matches on strings that come from code we control... Unlike the orchestrator's regex, these strings are in the same repo

**This is misleading:** The orchestrator's regex is ALSO in the same repo as the CLI error messages. The orchestrator is in `internal/batch/`, the install command is in `cmd/tsuku/`, and they're both in the tsuku monorepo.

**Recommendation:**
1. Acknowledge that `extractMissingRecipes()` uses regex and that this is technical debt
2. Either:
   - Accept this as acceptable (CLI parsing its own strings is less fragile than cross-component parsing)
   - Or use typed errors to eliminate the regex entirely (Option D)
3. Revise the "Positive Consequences" to be honest: "Moves regex from orchestrator to CLI" not "eliminates regex"

### 6. Architecture Concerns

**Assessment: SOUND with one layering issue**

**Positive aspects:**
- Clear separation between exit codes (coarse) and JSON (fine-grained)
- Reuses existing `printJSON()` infrastructure
- Minimal API surface (non-exported struct)

**Layering issue:**

The CLI is now responsible for:
1. Generating errors (in `internal/install/`)
2. Formatting errors for humans (`printError()`)
3. Parsing those same errors to extract structure (`extractMissingRecipes()`)
4. Re-formatting as JSON (`InstallError` struct)

This is a code smell. The CLI shouldn't parse its own output to discover information it should already have.

**Root cause:** The install pipeline doesn't preserve structured error information through the call stack. By the time the error reaches `cmd/tsuku/install.go`, it's a flattened string.

**Better approach (if typed errors are used):**
```go
// In install pipeline
return &InstallError{
    Code: ExitDependencyFailed,
    MissingRecipes: []string{"missing-tool"},
    Cause: err,
}

// In CLI
if jsonFlag {
    printJSON(err) // err is already structured
} else {
    printError(err.Error())
}
```

**Recommendation:** Add a note in "Negative Consequences" about the layering issue and consider adding this to the "Future Work" section as a refactoring opportunity.

### 7. Testing Strategy

**Assessment: ADEQUATE but missing integration tests**

The testing section (lines 217-223) covers:
- Unit tests for error classification ✓
- Unit tests for JSON output ✓
- Unit tests for orchestrator parsing ✓

**Missing:**
1. Integration test: Run `tsuku install --json <missing-tool>` and verify the actual JSON output
2. End-to-end test: Run batch orchestrator with JSON-enabled install and verify `blockedBy` is populated
3. Backward compatibility test: Ensure `tsuku install` (without `--json`) still produces the same stderr output

**Recommendation:** Add integration and E2E tests to the test plan.

### 8. Scope Clarity

**Assessment: GOOD with one meta concern**

The "Scope" section clearly delineates in-scope vs. out-of-scope work. However:

**Out of scope items reveal scope creep risk:**
- "Adding `--json` to `tsuku create`" - Why was this even considered? The problem statement doesn't mention it.
- "JSON output for success cases" - This is actually a reasonable exclusion
- "Schema versioning" - Also reasonable given internal-only use

The fact that `create` JSON is explicitly called out suggests someone pushed for it during planning. This is fine, but it hints that the scope may have been challenged and needed explicit defense.

**Recommendation:** No changes needed. The scope is well-defined. Just be aware of scope creep pressure.

## Summary of Findings

### Critical Issues

1. **Regex not eliminated, just moved**: The solution introduces `extractMissingRecipes()` with regex, contradicting the goal of removing regex-based parsing
2. **Missing Option D (typed errors)**: A proper alternative that truly eliminates regex is not evaluated
3. **Option C is a strawman**: Bundled unrelated work to make it look unreasonable

### Major Issues

4. **Unstated assumptions about consumers**: External vs. internal consumers affects versioning decisions
5. **Layering problem**: CLI parsing its own error strings is a code smell
6. **Self-contradiction in consequences**: Claims to eliminate regex but actually relocates it

### Minor Issues

7. **Missing integration tests**: Unit tests alone don't validate end-to-end behavior
8. **No impact quantification**: Problem statement lacks frequency/cost data
9. **Incomplete exit code analysis**: No discussion of whether codes 3, 5, 6, 7, 8 cover all cases

## Recommendations

### High Priority

1. **Add Option D (Typed Errors)** and evaluate it fairly against Option B
2. **Revise Option C** to not be a strawman (remove `create`, CI, schema bundling)
3. **Acknowledge regex in solution**: Update consequences to reflect that regex is moved, not eliminated
4. **Add missing cons to Option B**: Dual classification systems, CLI self-parsing

### Medium Priority

5. **Clarify consumer assumptions**: Are external consumers expected? Does this affect versioning?
6. **Add integration tests**: Cover end-to-end JSON output validation
7. **Quantify impact**: How often do these errors occur? What's the debugging time cost?

### Low Priority

8. **Validate exit code completeness**: Explicitly list all codes and their error type mappings
9. **Consider success case structure**: Even if out of scope, acknowledge it may be needed later

## Overall Assessment

**Verdict: APPROVE WITH REVISIONS**

The design addresses a real problem and proposes a reasonable incremental solution. However, the self-contradiction around regex usage undermines confidence in the approach. The document should:

1. Either acknowledge the regex as acceptable technical debt (CLI parsing its own strings is less risky than cross-component parsing)
2. Or evaluate typed errors as a proper alternative that truly eliminates regex

Once those issues are addressed, this is a solid design that delivers value incrementally.

**Key strengths:**
- Clear problem statement with concrete examples
- Appropriate scope limitation
- Good use of code examples
- Realistic implementation plan

**Key weaknesses:**
- Regex not actually eliminated, just moved
- Missing evaluation of typed errors alternative
- Strawman Option C
- Layering issue (CLI parsing its own output)
