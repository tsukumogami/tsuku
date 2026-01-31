# Architecture Review: Structured JSON Output for CLI Commands

## Executive Summary

The design proposes two independent changes to improve error handling in `tsuku install`:
1. Fix exit code classification to use correct codes (3, 5, 8) instead of always using 6
2. Add `--json` flag for structured error output

The architecture is **implementable** and addresses the stated problem. However, there are opportunities to simplify the implementation and reduce fragility by leveraging existing typed error infrastructure.

## Question 1: Is the Architecture Clear Enough to Implement?

**Answer: Yes, with minor clarifications needed.**

### What's Clear

The design provides concrete implementation guidance:
- Exit code mapping is well-defined (code 3 = recipe not found, code 5 = network, code 8 = dependency failed)
- JSON response structure is specified with 5 fields
- Integration points with batch orchestrator are documented
- File locations and function names are provided

### What Needs Clarification

1. **Error Classification Logic Location**: The design shows `classifyInstallError()` in `install.go`, but doesn't specify where in the call chain it's invoked. The install command has multiple exit points:
   - Line 73: sandbox install failure
   - Line 100: recipe-based install failure
   - Line 127: plan-based install failure
   - Line 161: normal install failure

   The design should clarify whether classification applies to all paths or only the normal install path.

2. **Network Error Detection**: The design references `isNetworkError(err)` (line 116 of the design) but this function doesn't exist in the codebase. The implementation needs to specify:
   - Should it use `errors.As()` to check for `*registry.RegistryError` with type `ErrTypeNetwork`?
   - Or does it need string matching like the other cases?

3. **JSON Output Timing**: The design states "emit `InstallError` struct via `printJSON()` to stdout" but doesn't specify when this happens relative to the exit. Should it:
   - Print JSON, then exit immediately?
   - Print JSON, then call `printError()` for stderr, then exit?
   - Only print JSON (no stderr output)?

4. **Dependency Name Extraction**: The design shows extracting missing recipe names from nested error messages using regex (lines 182-192). This regex lives in CLI layer but duplicates logic already in the orchestrator. The relationship between `extractMissingRecipes()` in install.go and the existing `reNotFoundInRegistry` in orchestrator.go should be clarified.

## Question 2: Are There Missing Components or Interfaces?

**Answer: No critical gaps, but existing infrastructure is underutilized.**

### Existing Infrastructure Not Leveraged

The codebase already has typed error handling that the design doesn't use:

1. **`registry.RegistryError` Struct** (internal/registry/errors.go)
   - Already distinguishes error types: `ErrTypeNotFound`, `ErrTypeNetwork`, `ErrTypeDNS`, `ErrTypeTimeout`, etc.
   - Provides `Suggestion()` method for user guidance
   - Used by registry operations but errors are unwrapped by the time they reach install command

2. **Typed Error Chain**: Registry operations return `*RegistryError`, but the install command receives generic `error` because the chain wraps it with `fmt.Errorf()`:
   ```go
   // install_deps.go:309
   return fmt.Errorf("failed to install dependency '%s': %w", dep, err)
   ```
   This wrapping preserves the error chain but loses type information at the CLI boundary.

### What's Actually Missing

1. **Network Error Detection Helper**: The design references `isNetworkError(err)` which doesn't exist. Need to either:
   - Implement it using `errors.As()` to unwrap and check `*registry.RegistryError`
   - Fall back to string matching like other classification cases

2. **JSON Output Helper for Errors**: The design shows calling `printJSON(InstallError{...})` but doesn't specify how this integrates with the existing error printing flow. Need a clear pattern for:
   - When to print JSON vs stderr
   - How to suppress normal error output when `--json` is set
   - Whether to include human-readable suggestions in JSON

3. **Exit Code Constants Mapping**: The orchestrator has `categoryFromExitCode()` which maps codes to categories. The design introduces `classifyInstallError()` which does the inverse (error to exit code). These should reference each other or share constants to prevent drift.

### Suggested Additional Component

**Error Classification Layer**: Instead of string matching in the CLI, introduce an intermediate function that uses typed error unwrapping:

```go
// classifyError uses typed error checking before falling back to string matching
func classifyError(err error) int {
    // Check typed errors first
    var regErr *registry.RegistryError
    if errors.As(err, &regErr) {
        switch regErr.Type {
        case registry.ErrTypeNotFound:
            return ExitRecipeNotFound
        case registry.ErrTypeNetwork, registry.ErrTypeDNS,
             registry.ErrTypeTimeout, registry.ErrTypeConnection:
            return ExitNetwork
        }
    }

    // Fall back to string matching for non-registry errors
    msg := err.Error()
    if strings.Contains(msg, "failed to install dependency") {
        return ExitDependencyFailed
    }

    return ExitInstallFailed
}
```

This reduces fragility because it uses compile-time type checking for registry errors and only falls back to string matching for dependency installation errors.

## Question 3: Are Implementation Phases Correctly Sequenced?

**Answer: Mostly yes, but Step 1 and 2 have a hidden dependency.**

### Current Sequence (from design)

1. Exit code classification in install.go
2. Add `--json` flag and error response
3. Update batch orchestrator
4. Tests

### Issues with Current Sequence

**Step 1 and 2 are not fully independent**: The design claims exit code fixes work standalone, but the `classifyInstallError()` function (Step 1) includes a call to `extractMissingRecipes()` which is only introduced in Step 2:

```go
// Design line 182-192 (Step 2)
func extractMissingRecipes(err error) []string {
    // regex extraction
}
```

But this is needed by `InstallError` struct in Step 2, not by exit code classification in Step 1.

**Actual dependency**: Exit code classification needs to identify "missing dependency" errors but doesn't need to extract the specific recipe names. Recipe name extraction is only needed for JSON output.

### Improved Sequence

**Phase 1: Exit Code Classification** (truly standalone)
- Add `classifyInstallError()` that maps errors to exit codes using typed errors + string matching
- Replace all `exitWithCode(ExitInstallFailed)` with `exitWithCode(classifyInstallError(err))`
- Verify orchestrator's `categoryFromExitCode()` produces correct categories
- Tests: exit code correctness for different error types

**Phase 2: JSON Error Output** (builds on Phase 1)
- Add `--json` flag to install command
- Add `InstallError` struct and `extractMissingRecipes()` helper
- Modify error handling to emit JSON when flag is set
- Tests: JSON structure, missing recipe extraction

**Phase 3: Orchestrator Integration** (builds on Phase 2)
- Add `--json` to orchestrator's validate() args
- Parse JSON response for `missing_recipes` field
- Remove `classifyValidationFailure()` and `reNotFoundInRegistry`
- Tests: orchestrator uses exit codes + JSON correctly

**Phase 4: Cleanup** (optional)
- Consider refactoring error chain to preserve typed errors through install flow
- Add integration tests for full orchestrator workflow

This sequence ensures each phase delivers incremental value and doesn't break existing behavior.

## Question 4: Are There Simpler Alternatives Overlooked?

**Answer: Yes, there's a simpler approach using typed errors.**

### Alternative 1: Typed Error Propagation (Simpler, More Robust)

**Key Insight**: The registry already uses typed errors (`*registry.RegistryError`). The fragility comes from converting these to strings too early and then parsing them back.

**Proposed approach**:
1. Preserve `*registry.RegistryError` through the install chain by using `%w` wrapping (already done)
2. At CLI boundary, use `errors.As()` to unwrap and check type
3. For dependency errors, create a new typed error in install_deps.go:

```go
// internal/install/errors.go (new file)
type DependencyError struct {
    DependencyName  string
    MissingRecipes  []string
    Err             error
}

func (e *DependencyError) Error() string {
    return fmt.Sprintf("failed to install dependency '%s': %v", e.DependencyName, e.Err)
}

func (e *DependencyError) Unwrap() error {
    return e.Err
}
```

4. In install_deps.go:309, return typed error instead of `fmt.Errorf`:
```go
return &install.DependencyError{
    DependencyName: dep,
    MissingRecipes: extractMissingRecipes(err), // extract once, here
    Err:            err,
}
```

5. In install.go, classify using typed errors:
```go
func classifyInstallError(err error) int {
    var regErr *registry.RegistryError
    if errors.As(err, &regErr) {
        switch regErr.Type {
        case registry.ErrTypeNotFound:
            return ExitRecipeNotFound
        case registry.ErrTypeNetwork, registry.ErrTypeDNS, registry.ErrTypeTimeout:
            return ExitNetwork
        }
    }

    var depErr *install.DependencyError
    if errors.As(err, &depErr) {
        return ExitDependencyFailed
    }

    return ExitInstallFailed
}

func buildInstallError(err error) InstallError {
    result := InstallError{
        Status:   "error",
        Message:  err.Error(),
        ExitCode: classifyInstallError(err),
    }

    // Map exit code to category
    switch result.ExitCode {
    case ExitRecipeNotFound:
        result.Category = "recipe_not_found"
    case ExitNetwork:
        result.Category = "network"
    case ExitDependencyFailed:
        result.Category = "missing_dep"
        // Extract missing recipes from typed error
        var depErr *install.DependencyError
        if errors.As(err, &depErr) {
            result.MissingRecipes = depErr.MissingRecipes
        }
    default:
        result.Category = "install_failed"
    }

    return result
}
```

**Benefits**:
- No regex parsing at CLI layer (only in install_deps.go where error originates)
- Compile-time type safety
- Errors carry structured data through the call chain
- Simpler to test (check error type instead of string matching)
- Future-proof: adding new error types doesn't require updating string patterns

**Drawbacks**:
- More files/types to maintain
- Requires refactoring install_deps.go error handling
- Slightly larger scope than design proposes

### Alternative 2: Exit Codes Only (Simplest, Least Feature-Complete)

This is Option A from the design document. Worth reconsidering because:

**What the orchestrator actually needs**:
- Category classification (already available from exit codes via `categoryFromExitCode`)
- Missing recipe names for `blockedBy` field

**Simple approach**:
1. Fix exit codes as designed (no JSON)
2. In orchestrator, keep regex parsing for missing recipe names
3. Use exit codes for category classification

**Benefits**:
- Smallest possible change
- Exit codes alone solve 80% of the problem
- Regex stays in orchestrator where it's already tested

**Drawbacks**:
- Doesn't eliminate regex fragility (original motivation)
- Can't provide detailed error info to other consumers
- Less extensible for future needs

**Verdict**: This is simpler but doesn't achieve the design's stated goal of eliminating regex parsing.

### Alternative 3: Hybrid Approach (Recommended)

Combine the best of typed errors and pragmatic scope:

**Phase 1**: Fix exit codes using typed error unwrapping (not string matching)
- Use `errors.As()` to check `*registry.RegistryError`
- Use string matching only for dependency errors (unavoidable without refactoring install_deps.go)
- Delivers value immediately

**Phase 2**: Add `--json` with the simple `InstallError` struct from the design
- Orchestrator uses exit codes + JSON
- Removes `classifyValidationFailure` as planned

**Phase 3** (future milestone): Refactor install_deps.go to use typed errors
- Eliminates remaining string matching
- Makes JSON output more robust

This approach:
- Keeps scope manageable (same as design)
- Reduces fragility in Phase 1 by using typed errors where available
- Creates clear path for future improvement
- Doesn't require new packages/types in Phase 1-2

## Detailed Findings

### Finding 1: String-Based Classification Is Partially Avoidable

**Current Design** (line 108-122):
```go
func classifyInstallError(err error) int {
    msg := err.Error()
    switch {
    case strings.Contains(msg, "not found in registry"):
        return ExitRecipeNotFound
    case strings.Contains(msg, "failed to install dependency"):
        return ExitDependencyFailed
    case isNetworkError(err):  // <-- this doesn't exist
        return ExitNetwork
    default:
        return ExitInstallFailed
    }
}
```

**Issue**: The design uses string matching for "not found in registry" even though `registry.RegistryError` has a typed `ErrTypeNotFound`. This is fragile because:
- The error message format is defined in internal/registry/registry.go:117
- If that message changes, classification breaks
- Tests won't catch this unless they verify exact strings

**Better approach using existing types**:
```go
func classifyInstallError(err error) int {
    // Check for typed registry errors first
    var regErr *registry.RegistryError
    if errors.As(err, &regErr) {
        switch regErr.Type {
        case registry.ErrTypeNotFound:
            return ExitRecipeNotFound
        case registry.ErrTypeNetwork, registry.ErrTypeDNS,
             registry.ErrTypeTimeout, registry.ErrTypeConnection,
             registry.ErrTypeTLS:
            return ExitNetwork
        }
    }

    // Fall back to string matching for dependency errors
    // (these come from install_deps.go which uses fmt.Errorf)
    msg := err.Error()
    if strings.Contains(msg, "failed to install dependency") {
        return ExitDependencyFailed
    }

    return ExitInstallFailed
}
```

**Why this works**: The error chain is already preserved via `%w` wrapping. At install_deps.go:309:
```go
return fmt.Errorf("failed to install dependency '%s': %w", dep, err)
```

When the underlying `err` is a `*registry.RegistryError`, `errors.As()` can still unwrap it.

### Finding 2: Missing Recipe Extraction Is Duplicated

**Current Design**:
- orchestrator.go:18 has `reNotFoundInRegistry` regex
- Design adds same regex to install.go as `extractMissingRecipes()`

**Issue**: Two places maintaining identical regex patterns. If error message format changes, both need updates.

**Root cause**: The error information is converted to string too early (at install_deps.go:309) and then needs to be parsed back out at CLI layer.

**Better approach**: Extract once at the source (install_deps.go) and carry in typed error.

### Finding 3: JSON Schema Could Be More Informative

**Current design**:
```go
type InstallError struct {
    Status         string   `json:"status"`           // always "error"
    Category       string   `json:"category"`         // matches exit code meaning
    Message        string   `json:"message"`          // human-readable error
    MissingRecipes []string `json:"missing_recipes"`  // for category "missing_dep"
    ExitCode       int      `json:"exit_code"`        // the exit code being used
}
```

**Observations**:
- `Status` is always "error", doesn't provide information (could remove or use for success cases later)
- `Category` is string but has fixed values - consider enum or document valid values
- `MissingRecipes` is only populated for one category - could be confusing when empty vs null
- No `suggestion` field even though `registry.RegistryError` provides suggestions

**Potential improvements**:
```go
type InstallError struct {
    Status         string   `json:"status"`                    // "error"
    Category       string   `json:"category"`                  // recipe_not_found, network, missing_dep, install_failed
    Message        string   `json:"message"`                   // human-readable error
    Suggestion     string   `json:"suggestion,omitempty"`      // actionable suggestion
    MissingRecipes []string `json:"missing_recipes,omitempty"` // populated only for missing_dep
    ExitCode       int      `json:"exit_code"`                 // numeric exit code
}
```

**Trade-off**: Design explicitly avoids schema versioning. Adding fields is backward-compatible but removing/renaming isn't. The suggestion field is genuinely useful (orchestrator could include in failure messages) but adds surface area.

### Finding 4: Exit Code to Category Mapping Is Inconsistent

**orchestrator.go:315-330** (`categoryFromExitCode`):
```go
switch code {
case 5: return "api_error"           // ExitNetwork
case 6: return "validation_failed"   // ExitInstallFailed
case 7: return "validation_failed"   // ExitVerifyFailed
case 8: return "missing_dep"         // ExitDependencyFailed
case 9: return "deterministic_insufficient"
}
```

**Design's proposed categories** (in JSON output):
- "recipe_not_found" (exit code 3)
- "network" (exit code 5)
- "missing_dep" (exit code 8)
- "install_failed" (exit code 6)

**Issue**:
- Orchestrator maps code 5 to "api_error" but design uses "network"
- Orchestrator doesn't handle code 3 (falls through to "validation_failed")
- Terminology inconsistency: "api_error" vs "network", both mean the same thing

**Impact**: If orchestrator uses exit codes + JSON, the category names should match. Otherwise:
- Exit code 5 produces category "api_error" (from `categoryFromExitCode`)
- JSON output has category "network"
- Confusion when debugging

**Fix**: Update `categoryFromExitCode` to match JSON category names:
```go
switch code {
case 3: return "recipe_not_found"
case 5: return "network"
case 6: return "install_failed"
case 7: return "verification_failed"
case 8: return "missing_dep"
case 9: return "deterministic_insufficient"
}
```

Or define category constants shared between CLI and orchestrator:
```go
// exitcodes.go
const (
    CategoryRecipeNotFound = "recipe_not_found"
    CategoryNetwork        = "network"
    CategoryMissingDep     = "missing_dep"
    CategoryInstallFailed  = "install_failed"
)
```

### Finding 5: Test Coverage Gaps Not Addressed

**Design mentions tests** (Step 4):
- Unit test: `classifyInstallError` returns correct exit codes
- Unit test: JSON output includes `missing_recipes`
- Unit test: orchestrator parses JSON correctly

**Missing test cases**:
1. **Error chain unwrapping**: Verify `errors.As()` works through multiple wrapping layers
2. **Multiple missing recipes**: Design shows extracting multiple names but doesn't test order, deduplication
3. **Network error subtypes**: DNS, timeout, connection refused should all map to code 5
4. **JSON output when `--json` not set**: Verify normal stderr output unchanged
5. **Empty vs null in JSON**: What happens when `missing_recipes` is empty list vs not present?
6. **Integration test**: Full flow from install command through orchestrator with --json

**Recommendation**: Add integration test that exercises realistic error scenarios:
```bash
# Test recipe not found
tsuku install nonexistent-tool --json
# Should exit 3, JSON category="recipe_not_found"

# Test missing dependency (requires mock/fixture)
tsuku install tool-with-missing-dep --json --recipe testdata/broken-recipe.toml
# Should exit 8, JSON category="missing_dep", missing_recipes=["dep1"]
```

### Finding 6: Backward Compatibility Not Fully Analyzed

**Orchestrator changes**:
- Adds `--json` flag to install command invocation
- Removes `classifyValidationFailure` function
- Changes how `blockedBy` is populated

**Potential issues**:
1. **Old tsuku binary + new orchestrator**: If orchestrator passes `--json` but binary doesn't support it, install will fail with "unknown flag"
2. **New binary + old orchestrator**: Will work (orchestrator will continue using exit codes + regex)

**Impact**: The orchestrator and CLI are in same repo so this is low-risk. But if the orchestrator is ever extracted or if there's a version compatibility concern, the design should specify:
- How to detect `--json` support (version check? flag probe?)
- Fallback behavior if JSON not available

**Current verdict**: Not a concern for this change, but worth documenting the assumption that orchestrator and CLI versions stay in sync.

## Recommendations

### Priority 1: Critical for Implementation

1. **Use typed error unwrapping for registry errors**: Replace string matching with `errors.As(*registry.RegistryError)` in `classifyInstallError()`. This eliminates most string fragility and uses existing infrastructure.

2. **Align category names between orchestrator and CLI**: Use consistent terminology ("network" not "api_error", add "recipe_not_found" to orchestrator mapping).

3. **Clarify network error detection**: Either implement `isNetworkError()` using typed unwrapping or remove it from the design and rely on registry error type checking.

4. **Specify JSON output behavior**: Document when JSON is printed (before exit? instead of stderr?) and whether normal error output is suppressed.

### Priority 2: Recommended for Quality

5. **Add integration tests**: Don't just unit test components, verify full flow with realistic error scenarios.

6. **Consider adding `suggestion` field to JSON**: The registry errors already provide suggestions; exposing them in JSON makes the orchestrator's failure messages more helpful.

7. **Document category values**: Either as code comments, design doc, or as constants that CLI and orchestrator share.

8. **Extract missing recipes at error source**: If refactoring install_deps.go is acceptable scope, create typed `DependencyError` that carries missing recipe names. This eliminates regex parsing entirely.

### Priority 3: Future Enhancements

9. **Schema versioning**: Even if not needed now, add a `version` field to JSON output for future compatibility.

10. **Success case JSON**: The design defers this, which is fine, but plan for consistent structure (same top-level fields with `status: "success"`).

11. **Typed error refactoring**: Create issue/milestone for refactoring install_deps.go to use typed errors throughout, eliminating the last string matching case.

## Conclusion

The architecture is **implementable and addresses the core problem**. The main weaknesses are:

1. **Underutilizes existing typed error infrastructure**: The registry already has `RegistryError` with type classification. Using it via `errors.As()` is more robust than string matching.

2. **Introduces regex duplication**: `extractMissingRecipes()` in install.go duplicates orchestrator's `reNotFoundInRegistry`. Both could be eliminated by extracting once at the error source.

3. **Category naming inconsistency**: Orchestrator and CLI use different names for the same concept ("api_error" vs "network").

**Recommended path forward**:

- **Adopt Hybrid Approach (Alternative 3)**: Fix exit codes using typed error unwrapping where possible, add JSON as designed, defer install_deps.go refactoring to future milestone.

- **Address Priority 1 issues** before implementation: Use typed errors for registry classification, align category names, clarify network error detection.

- **Consider Priority 2 recommendations** during code review: Add integration tests, document category values, possibly include suggestions in JSON.

The result will be more robust than the current design while keeping scope reasonable.
