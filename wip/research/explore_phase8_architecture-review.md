# Architecture Review: Verification Self-Repair

## Executive Summary

The proposed verification self-repair architecture is **well-designed and implementable**. It integrates cleanly with the existing orchestrator validation loop, leverages established patterns, and correctly identifies the insertion point between sandbox failure detection and LLM repair. The component boundaries are clear, interfaces are minimal, and the implementation phases are correctly sequenced.

**Recommendation: Approve with minor refinements**

---

## Question 1: Is the Architecture Clear Enough to Implement?

**Assessment: Yes, with minor clarifications needed**

### Strengths

1. **Clear component boundaries**: The design correctly separates concerns:
   - `VerifyFailureAnalyzer` (pure analysis, no side effects) in `internal/validate/`
   - `VerifyRepairer` (repair logic) in `internal/builders/orchestrator.go`

2. **Well-defined insertion point**: Lines 186-209 of `orchestrator.go` show the exact location where self-repair should intercept:
   ```go
   // Validation failed - attempt repair if we have attempts left
   if attempt >= o.config.MaxRepairs {
       return nil, &ValidationFailedError{...}
   }

   // <<< INSERT SELF-REPAIR HERE >>>

   // Repair the recipe
   repairAttempts++
   result, err = session.Repair(ctx, sandboxResult)
   ```

3. **Matches existing patterns**: The design follows established conventions:
   - Uses `sandbox.SandboxResult` (already has Stdout, Stderr, ExitCode)
   - Returns modified `*BuildResult` with metadata (consistent with existing result structures)
   - Error categorization aligns with `internal/validate/errors.go` patterns

### Clarifications Needed

1. **Error category detection**: The design says "If not `ErrorVerifyFailed`, skip self-repair" but the current `ParseValidationError()` function matches verification failures based on text patterns like "verification failed", "checksum mismatch", etc. These patterns don't match the help-text scenario where the tool prints usage information.

   **Issue**: A verification failure where the tool prints help text won't match `ErrorVerifyFailed` - it would be `ErrorUnknown` or potentially `ErrorBinaryNotFound` if the output contains "not found".

   **Recommendation**: The analyzer should be invoked for all non-zero exit codes where the sandbox passed (installation succeeded but verify command failed), not just `ErrorVerifyFailed`. The entry condition should be:
   ```go
   if sandboxResult.ExitCode != 0 && !sandboxResult.Skipped {
       // Check if this is a verification failure that can be self-repaired
   }
   ```

2. **Recipe cloning**: The design mentions "Clone recipe with modified verify section" but doesn't specify the cloning mechanism. The `Recipe` struct has nested slices and pointers (`Steps`, `When` clauses) that require deep copy.

   **Recommendation**: Add a `Clone()` method to `recipe.Recipe` or document that only `Verify` section needs modification (which is a value type, not pointer).

3. **Sandbox executor interface**: The current orchestrator calls `o.sandbox.Sandbox()` which takes `*executor.InstallationPlan`. For fallback commands, we need to run just the verify command with different arguments.

   **Question**: Does the sandbox support running arbitrary commands, or does it require a full installation plan?

   **Finding**: Looking at `sandbox.Executor.Sandbox()`, it runs `tsuku install --plan`. For fallback verification, we need to either:
   - Generate a new plan with modified verify command
   - Add a simpler `sandbox.Verify()` method that runs just the verify command

   The design's Phase 3 "Clone recipe with modified verify command" implies regenerating the full plan, which is correct but may be slow.

---

## Question 2: Are There Missing Components or Interfaces?

**Assessment: Mostly complete, one gap identified**

### Present and Well-Defined

1. **`VerifyFailureAnalysis` struct**: Complete with all necessary fields:
   - `Repairable`, `ToolName`, `ExitCode`
   - `HasUsageText`, `HasToolName`, `OutputLength`
   - `SuggestedMode`, `SuggestedPattern`

2. **`RepairMetadata` struct**: Provides adequate tracking:
   - `Type`, `Original`, `Repaired`, `Method`, `ExitCode`

3. **Telemetry**: The design references adding a `verify_self_repair` event. The existing `LLMEvent` structure in `internal/telemetry/event.go` provides a good template, though a new dedicated event type would be cleaner.

### Missing Components

1. **Verify-only sandbox execution**: The design assumes fallback commands run in the sandbox with the same environment. But currently `sandbox.Executor.Sandbox()` runs a full installation. For efficient fallback testing, we need:

   ```go
   // Proposed addition to sandbox.Executor
   func (e *Executor) RunVerifyCommand(
       ctx context.Context,
       toolDir string,  // Path to installed tool
       command string,  // e.g., "tool --help"
       timeout time.Duration,
   ) (*SandboxResult, error)
   ```

   **Alternative**: Reuse existing infrastructure by creating a minimal plan that only runs the verify step. This is slower but requires no new code in the sandbox package.

   **Recommendation**: Start with the slower approach (regenerate plan with modified verify) in Phase 1. Optimize with dedicated verify execution in a future iteration if performance is a concern.

2. **Recipe modification helper**: The design needs a way to modify the verify section cleanly:

   ```go
   // Proposed helper
   func (r *Recipe) WithVerify(v VerifySection) *Recipe {
       copy := *r  // Shallow copy is sufficient - Steps slice is not modified
       copy.Verify = v
       return &copy
   }
   ```

   This is a minor addition but improves clarity.

---

## Question 3: Are Implementation Phases Correctly Sequenced?

**Assessment: Yes, phases are correctly ordered**

### Phase Dependency Analysis

```
Phase 1: VerifyFailureAnalyzer
    |
    v
Phase 2: Orchestrator Integration (depends on Phase 1)
    |
    v
Phase 3: Fallback Commands (depends on Phase 2)
    |
    v
Phase 4: Telemetry (can run in parallel with Phase 3)
```

### Phase Details

**Phase 1: Verify Failure Analyzer**
- Standalone, no dependencies
- Pure function, easy to test in isolation
- Correct starting point

**Phase 2: Orchestrator Integration**
- Depends on Phase 1 for analysis
- Modifies `orchestrator.go` to insert self-repair logic
- Can be tested with mocked analyzer

**Phase 3: Fallback Commands**
- Depends on Phase 2 infrastructure
- Adds complexity: recipe cloning, re-validation
- Correctly separated from basic pattern detection

**Phase 4: Telemetry**
- Could run in parallel with Phase 3
- Low risk, non-functional change
- Correct to defer to end

### Recommendation

The phases are correctly sequenced. Consider:
- Merging Phase 3 with Phase 2 if the fallback logic is simple enough
- Running Phase 4 in parallel with Phase 3 to reduce total implementation time

---

## Question 4: Are There Simpler Alternatives?

**Assessment: One simpler alternative worth considering**

### Alternative A: Pre-emptive Verify Detection (Simpler but Less General)

Instead of detecting failures after the fact, detect likely --version failures before running the sandbox:

```go
// During recipe generation, check if tool is known to not support --version
func isKnownNoVersionFlag(toolName string) bool {
    noVersionTools := map[string]bool{
        "age": true, "exa": true, "lsd": true, // etc.
    }
    return noVersionTools[toolName]
}
```

**Pros**:
- Avoids wasted sandbox execution
- No complex pattern matching needed

**Cons**:
- Requires maintaining a list of tools
- Doesn't help with unknown tools
- Not self-updating

**Verdict**: Reject. The proposed approach is more general and self-learning.

### Alternative B: Always Use --help (Simpler, Less Accurate)

Default verification to `--help` instead of `--version`:

**Pros**:
- Eliminates most verification failures
- Near-universal support

**Cons**:
- Loses version-specific verification
- May pass when wrong version is installed
- Semantic regression

**Verdict**: Reject. Version verification is valuable when available.

### Alternative C: Lazy Repair Metadata (Slightly Simpler)

Instead of adding `RepairMetadata` to `BuildResult`, log repairs to telemetry only:

```go
// Instead of:
result.RepairMetadata = &RepairMetadata{...}

// Do:
o.telemetryClient.Send(NewVerifySelfRepairEvent(...))
```

**Pros**:
- Simpler BuildResult structure
- No API changes

**Cons**:
- Loses local debugging capability
- Caller can't distinguish repaired vs. original

**Verdict**: Consider. Metadata is useful for debugging, but if telemetry covers the use case, this simplifies the implementation.

---

## Detailed Code Integration Analysis

### Entry Point: orchestrator.go

Current validation loop (lines 158-209):
```go
for attempt := 0; attempt <= o.config.MaxRepairs; attempt++ {
    sandboxResult, err := o.validate(ctx, result.Recipe)
    // ... error handling ...

    if sandboxResult.Passed {
        return &OrchestratorResult{...}, nil
    }

    // Validation failed - attempt repair if we have attempts left
    if attempt >= o.config.MaxRepairs {
        return nil, &ValidationFailedError{...}
    }

    // <<< INSERT SELF-REPAIR LOGIC HERE >>>

    repairAttempts++
    result, err = session.Repair(ctx, sandboxResult)
}
```

Proposed modification:
```go
// After "if sandboxResult.Passed {...}" block
if !sandboxResult.Passed {
    // Attempt deterministic self-repair before LLM repair
    repairedRecipe, repairMeta, selfRepairErr := o.attemptVerifySelfRepair(ctx, result.Recipe, sandboxResult)
    if selfRepairErr == nil && repairedRecipe != nil {
        // Re-validate with repaired recipe
        repairedResult, err := o.validate(ctx, repairedRecipe)
        if err == nil && repairedResult.Passed {
            result.Recipe = repairedRecipe
            result.RepairMetadata = repairMeta
            return &OrchestratorResult{
                Recipe:         repairedRecipe,
                BuildResult:    result,
                RepairAttempts: repairAttempts,
            }, nil
        }
    }
    // Self-repair failed or not applicable, fall through to LLM repair
}
```

### Integration with BuildResult

The `BuildResult` struct in `builder.go` already has several optional fields. Adding `RepairMetadata` is consistent:

```go
type BuildResult struct {
    Recipe         *recipe.Recipe
    Warnings       []string
    Source         string
    RepairAttempts int
    Provider       string
    SandboxSkipped bool
    Cost           float64
    RepairMetadata *RepairMetadata  // NEW
}
```

### VerifySection Modification

The `VerifySection` in `recipe/types.go` supports all needed fields:
```go
type VerifySection struct {
    Command       string
    Pattern       string
    Mode          string   // "version" or "output"
    ExitCode      *int     // Expected exit code
    Reason        string   // For documentation
}
```

Creating a modified verify section is straightforward:
```go
repairedVerify := recipe.VerifySection{
    Command:  r.Verify.Command,  // Keep original or use fallback
    Pattern:  analysis.SuggestedPattern,
    Mode:     analysis.SuggestedMode,
    ExitCode: &analysis.ExitCode,
    Reason:   "verification repaired: tool does not support --version",
}
```

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| False positive detection (repair when not needed) | Low | Low | Re-validation catches incorrect repairs |
| False negative (miss repairable case) | Medium | Low | Falls through to LLM repair |
| Pattern matching edge cases | Medium | Low | Conservative patterns, extensive test cases |
| Performance regression (extra sandbox runs) | Low | Medium | Hybrid detection minimizes fallback usage |
| Recipe mutation bugs | Low | Medium | Shallow copy is sufficient; verify section is value type |

---

## Recommendations Summary

1. **Approve the architecture** with the following adjustments:

2. **Clarify entry condition**: Use exit code check rather than `ErrorVerifyFailed` category.

3. **Add recipe helper**: Simple `WithVerify()` method for clean modification.

4. **Defer sandbox optimization**: Start with plan regeneration; optimize later if needed.

5. **Consider simplified metadata**: Telemetry-only tracking may suffice if local debugging isn't critical.

6. **Test coverage priorities**:
   - Unit tests for `VerifyFailureAnalyzer` with diverse tool outputs
   - Integration tests for orchestrator self-repair path
   - Edge cases: empty output, binary output, very long output

---

## Appendix: Reviewed Files

- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/builders/orchestrator.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/builders/builder.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/validate/errors.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/validate/executor.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/sandbox/executor.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/types.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/telemetry/event.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/docs/designs/DESIGN-verification-self-repair.md`
