# Architecture Review: Exit Code 9 for Deterministic-Only Mode

## Executive Summary

The proposed solution is **implementable but incomplete**. The architecture correctly identifies the three touchpoints (CLI flag, exit code, orchestrator), but several critical gaps need addressing before implementation:

1. **Missing error handling path in create.go** - The design shows `DeterministicFailedError` handling but doesn't account for how this error reaches the CLI from the orchestrator
2. **Incomplete orchestrator changes** - No mechanism proposed for passing `--deterministic-only` flag through the batch orchestrator's subprocess invocation
3. **Category mapping discrepancy** - The orchestrator uses `"deterministic_insufficient"` but this category isn't defined in the failure record schema
4. **Test coverage gaps** - Missing integration test for the full CLI → orchestrator → exit code flow

The sequencing is correct, but the implementation steps need refinement to address these gaps.

## Detailed Analysis

### 1. Clarity for Implementation

**Current State:** Partially clear

The three main changes are well-defined at a surface level:
- CLI flag `--deterministic-only` ✓
- New exit code `ExitDeterministicFailed = 9` ✓
- Orchestrator updates to pass flag and map exit code ✓

However, the **data flow** between these components has critical gaps.

#### Gap 1: Error Propagation Path

The design document shows this error handling in `create.go`:

```go
var detErr *builders.DeterministicFailedError
if errors.As(err, &detErr) {
    fmt.Fprintf(os.Stderr, "deterministic generation failed: [%s] %s\n",
        detErr.Category, detErr.Message)
    exitWithCode(ExitDeterministicFailed)
}
```

But looking at the actual code flow in `create.go` (lines 347-386), errors from `orchestrator.Create()` are already wrapped in specialized error types like `ValidationFailedError` and `ConfirmableError`. The proposed code assumes `DeterministicFailedError` surfaces directly from the orchestrator, but this isn't guaranteed.

**Current code flow:**
```
orchestrator.Create()
  → session.Generate()
    → HomebrewSession.Generate()
      → returns DeterministicFailedError (if deterministicOnly=true)
  → orchestrator wraps as ValidationFailedError? (unclear)
  → CLI receives wrapped error
```

**The design needs to specify:**
- Does `orchestrator.Create()` pass through `DeterministicFailedError` unwrapped?
- Or does it need special handling to preserve this error type?
- Should orchestrator check `opts.DeterministicOnly` and handle this case differently?

Looking at `orchestrator.go` lines 130-212, the orchestrator calls `session.Generate()` and if it fails, wraps it with `fmt.Errorf("generation failed: %w", err)`. This means the `DeterministicFailedError` **should** be unwrappable with `errors.As()`, but this needs explicit verification.

#### Gap 2: Orchestrator Flag Passing

The design states:

> The `generate` method adds `--deterministic-only` to the command arguments.

But the actual `generate()` method in `batch/orchestrator.go` (lines 154-201) constructs `args` for the `tsuku create` subprocess. The design doesn't show **where** in this method the flag should be added. Looking at the existing args:

```go
args := []string{
    "create", pkg.Name,
    "--from", pkg.ID,
    "--output", recipePath,
    "--yes",
    "--skip-sandbox",
}
```

The flag should be added here, but the design doesn't make this explicit. More critically, the design says "pass `--deterministic-only` for Homebrew packages" but provides no logic for **conditional** flag addition. Should it be:

1. Always pass `--deterministic-only` when ecosystem is homebrew?
2. Pass it based on some package metadata?
3. Pass it for all ecosystems?

The design document mentions "Pass `--deterministic-only` for Homebrew packages" but the orchestrator currently doesn't have ecosystem-specific logic in `generate()`. It would need either:
- A new config field `Config.DeterministicOnly bool`
- Conditional logic based on `Config.Ecosystem == "homebrew"`

This ambiguity makes the implementation unclear.

### 2. Missing Components and Interfaces

**Missing Component: Category Definition**

The design proposes mapping exit code 9 to `"deterministic_insufficient"`:

```go
case 9:
    return "deterministic_insufficient"
```

But this category doesn't exist in the current codebase. Looking at `errors.go` lines 134-145, the defined categories are:

```go
const (
    FailureCategoryNoBottles      = "no_bottles"
    FailureCategoryMissingDep     = "missing_dep"
    FailureCategoryBuildFromSrc   = "build_from_source"
    FailureCategoryComplexArchive = "complex_archive"
    FailureCategoryAPIError       = "api_error"
    FailureCategoryValidation     = "validation_failed"
)
```

The design acknowledges this:

> The builder's categories (`no_bottles`, `complex_archive`, etc.) describe *why* deterministic failed. The orchestrator's category describes *what happened* from the pipeline's perspective.

But introduces a **new category** (`"deterministic_insufficient"`) without:
1. Defining it in the appropriate schema/constants
2. Explaining how it relates to the batch orchestrator's failure record schema
3. Clarifying whether this is a batch-only category or should be added to `DeterministicFailureCategory`

**Missing Interface: Test Helpers**

The design proposes tests but doesn't account for the complexity of testing the full flow. Current tests in `orchestrator_test.go` use fake shell scripts that simulate exit codes. To test exit code 9:

1. The fake `tsuku` binary needs to recognize `--deterministic-only` flag
2. It needs to simulate deterministic failure with exit code 9
3. The test needs to verify the category mapping

Example missing test helper structure:
```go
// TestCategoryFromExitCode needs this case added:
{9, "deterministic_insufficient"},

// TestRun_deterministicFailure needs to be added (similar to TestRun_withFakeBinary)
```

### 3. Implementation Phase Sequencing

**Current sequencing is logical but could be optimized:**

Proposed:
1. Exit code constant
2. CLI flag and error handling
3. Orchestrator update
4. Tests

**Issue:** Step 2 (CLI) and Step 3 (orchestrator) are tightly coupled. The CLI flag handler depends on `DeterministicFailedError` being properly unwrapped from orchestrator errors. Without understanding the orchestrator's error wrapping behavior, implementing Step 2 could lead to bugs.

**Better sequencing:**

1. **Exit code constant** - No dependencies, safe first step ✓
2. **Error unwrapping verification** - Add test to confirm `orchestrator.Create()` preserves `DeterministicFailedError` through error wrapping
3. **Orchestrator update** - Add category constant, update `categoryFromExitCode()`, and add batch subprocess flag passing
4. **CLI flag and handler** - Now we know errors propagate correctly
5. **Integration tests** - Full flow verification

This ensures each step builds on verified behavior from the previous step.

### 4. Simpler Alternatives Analysis

**Alternative 1: Reuse existing exit code 6 (ExitInstallFailed)**

The design proposes a new exit code 9, but exit code 6 already exists for `ExitInstallFailed`. We could potentially:

- Use exit code 6 with stderr parsing to distinguish deterministic failures
- Pros: No new exit code needed, simpler for scripts that already handle code 6
- Cons: Less explicit, requires stderr regex parsing (brittle), loses structured signaling

**Verdict:** New exit code is justified. The orchestrator needs to distinguish deterministic failures from generic install failures without parsing stderr (which is brittle and ties the subprocess protocol to human-readable text).

**Alternative 2: JSON output instead of exit codes**

Instead of exit code 9, `tsuku create --deterministic-only` could output JSON on failure:

```json
{"status": "deterministic_failed", "category": "no_bottles", "message": "..."}
```

The orchestrator would parse JSON instead of exit codes.

- Pros: More structured, can pass detailed category info, no exit code proliferation
- Cons: Requires issue #1273 (JSON output) to be implemented first, more complex parsing, stdout/stderr handling complications

**Verdict:** Exit code 9 is simpler for the current architecture. JSON output is a better long-term solution but requires coordinating with #1273. The design correctly identifies this as a future enhancement: "If finer-grained categorization is needed later... the stderr format supports it."

**Alternative 3: No CLI flag, orchestrator always passes it**

Instead of `--deterministic-only` flag, the orchestrator could always pass it (since it never wants LLM fallback in batch mode), and the CLI could detect "running under orchestrator" context.

- Pros: Simpler CLI API, no flag to document
- Cons: Loses CLI composability (users can't manually test deterministic mode), orchestrator becomes magic caller, harder to debug

**Verdict:** Explicit flag is better. It makes the behavior testable by humans and debuggable with standard `tsuku create` commands.

**Alternative 4: Builder-level config instead of session option**

Instead of `SessionOptions.DeterministicOnly`, add it to `BuildRequest`:

```go
type BuildRequest struct {
    Package           string
    Version           string
    SourceArg         string
    DeterministicOnly bool  // NEW
}
```

- Pros: More explicit part of the request contract, doesn't pollute session options
- Cons: Couples deterministic mode to the request instead of runtime session behavior, less flexible if we want to change mid-session

**Verdict:** SessionOptions is correct. Deterministic-only is a runtime policy (like rate limits), not a request parameter (like package name).

## Missing Architectural Considerations

### 1. Backward Compatibility

The design doesn't address:
- What happens if old batch orchestrator code calls new CLI with exit code 9?
- Current `categoryFromExitCode()` has a `default: return "validation_failed"` case, so unknown codes already degrade gracefully
- But should we add a comment warning about backwards compat?

### 2. Error Message Quality

The proposed CLI error output:

```go
fmt.Fprintf(os.Stderr, "deterministic generation failed: [%s] %s\n",
    detErr.Category, detErr.Message)
```

This outputs: `deterministic generation failed: [no_bottles] formula jq has no bottles available`

**Issues:**
- The `[category]` prefix is unusual for tsuku's error style (other errors don't use this format)
- Looking at other error messages in `create.go`, they use simple descriptive text without category tags
- The design justifies this with "If finer-grained categorization is needed later... the stderr format supports it" but this assumes the orchestrator will parse stderr in the future, which contradicts the design's own reasoning that exit codes are sufficient

**Recommendation:** Drop the `[category]` prefix for consistency:
```go
fmt.Fprintf(os.Stderr, "Error: %s\n", detErr.Message)
```

If categorization is needed, issue #1273's JSON output is the right place for it.

### 3. Homebrew-Specific Coupling

The design says "pass `--deterministic-only` for Homebrew packages" but the Homebrew builder's `RequiresLLM()` currently returns `false` (line 214 of homebrew.go). This creates an inconsistency:

- The builder says "I don't require LLM" (can work deterministically)
- But the orchestrator needs to explicitly enable deterministic-only mode
- Other builders like Cargo also don't require LLM but use `DeterministicSession` which has no repair capability

**Architectural question:** Should deterministic-only mode apply to ALL builders, or just Homebrew?

Looking at `create.go` lines 318-319:
```go
effectiveSkipSandbox := skipSandbox || !builder.RequiresLLM()
```

The code already treats non-LLM builders (Cargo, PyPI, etc.) differently - they skip sandbox by default. So the batch orchestrator's deterministic-only flag should probably apply to ALL ecosystems, not just Homebrew. The design's "for Homebrew packages" restriction seems arbitrary.

**Recommendation:** Make `--deterministic-only` ecosystem-agnostic. Any builder can handle it:
- Homebrew: tries bottle inspection, fails with categorized error
- Cargo/PyPI/etc: already deterministic, flag is effectively a no-op but doesn't hurt

### 4. State Tracking

The `HomebrewSession` tracks `usedDeterministic bool` (line 122 of homebrew.go). This state is used to decide repair behavior:

```go
// If the failed recipe was generated deterministically, use LLM to generate a new one
```

But with `deterministicOnly = true`, the `Repair()` method returns early:

```go
if s.deterministicOnly {
    return nil, &RepairNotSupportedError{BuilderType: "homebrew-deterministic"}
}
```

The orchestrator then catches `RepairNotSupportedError` and returns `ValidationFailedError`. But the original **deterministic failure error** is lost at this point. The CLI would receive a generic `ValidationFailedError` instead of `DeterministicFailedError`.

**This is the critical gap identified earlier.** The orchestrator needs special handling:

```go
// In orchestrator.Create() after session.Generate() fails
if err != nil {
    // Check if this is a deterministic failure with DeterministicOnly set
    var detErr *DeterministicFailedError
    if errors.As(err, &detErr) && opts != nil && opts.DeterministicOnly {
        // Pass through without wrapping
        return nil, err
    }
    return nil, fmt.Errorf("generation failed: %w", err)
}
```

Without this, the exit code 9 path never triggers.

## Recommendations

### Critical Changes

1. **Add orchestrator error passthrough for DeterministicFailedError:**
   ```go
   // In orchestrator.Create() after session.Generate()
   result, err := session.Generate(ctx)
   if err != nil {
       var detErr *DeterministicFailedError
       if errors.As(err, &detErr) {
           return nil, err // Pass through for CLI to handle
       }
       return nil, fmt.Errorf("generation failed: %w", err)
   }
   ```

2. **Define category constant or remove `"deterministic_insufficient"`:**
   Either:
   - Add `FailureCategoryDeterministicInsufficient = "deterministic_insufficient"` to `errors.go`
   - Or map exit code 9 to an existing category like `"validation_failed"` (less specific but simpler)

3. **Clarify orchestrator flag logic:**
   ```go
   // In batch/orchestrator.go generate() method
   args := []string{
       "create", pkg.Name,
       "--from", pkg.ID,
       "--output", recipePath,
       "--yes",
       "--skip-sandbox",
       "--deterministic-only", // ADD: Always pass for batch mode
   }
   ```

   Or if ecosystem-specific:
   ```go
   if o.cfg.Ecosystem == "homebrew" {
       args = append(args, "--deterministic-only")
   }
   ```

4. **Fix error message format for consistency:**
   ```go
   // Remove [category] prefix
   fmt.Fprintf(os.Stderr, "Error: %s\n", detErr.Message)
   exitWithCode(ExitDeterministicFailed)
   ```

### Implementation Steps (Revised)

**Phase 1: Foundation**
1. Add `ExitDeterministicFailed = 9` to `exitcodes.go`
2. Add category constant (if using `"deterministic_insufficient"`) to appropriate location
3. Update `categoryFromExitCode()` in `batch/orchestrator.go` to map `9 → category`

**Phase 2: Orchestrator**
1. Add deterministic flag passing in `generate()` method
2. Add error passthrough logic in builders `orchestrator.Create()` for `DeterministicFailedError`
3. Add unit test for `categoryFromExitCode(9)`

**Phase 3: CLI**
1. Add `--deterministic-only` flag to `create` command
2. Thread `DeterministicOnly` through to `SessionOptions`
3. Add `DeterministicFailedError` handler in error path
4. Add unit test for CLI flag → exit code 9

**Phase 4: Integration**
1. Add integration test: fake binary with exit code 9 → orchestrator categorizes correctly
2. Add integration test: `create --deterministic-only` with failing homebrew formula → exit code 9
3. Verify error message quality and clarity

### Testing Gaps to Address

1. **Unit test:** `orchestrator.Create()` preserves `DeterministicFailedError` unwrapping
2. **Unit test:** `create` command with `--deterministic-only` returns exit code 9 on deterministic failure
3. **Integration test:** Batch orchestrator with fake binary returning exit code 9 maps to correct category
4. **Integration test:** Real homebrew formula with no bottles + `--deterministic-only` → structured error + exit code 9

## Conclusion

The architecture is **60% complete**. The three main touchpoints are correctly identified, but the **glue code** between them is underspecified. The critical missing piece is the orchestrator's error handling logic to preserve `DeterministicFailedError` so the CLI can detect it and exit with code 9.

**Is it implementable?** Yes, with the clarifications above.

**Are there missing components?** Yes - error passthrough logic, category constant definition, and flag passing details.

**Is the sequencing correct?** Mostly, but should verify error propagation before implementing CLI handler.

**Are there simpler alternatives?** Exit code 9 is the right choice. JSON output would be better long-term but requires #1273. Reusing exit code 6 would be too ambiguous.

**Overall assessment:** The design is on the right track but needs refinement before implementation. The gaps are fixable with targeted additions to the orchestrator's error handling and clearer specifications for flag passing logic.
