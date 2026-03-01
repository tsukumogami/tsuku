# Architecture Review: DESIGN-dependency-validation-in-batch-pipeline.md

Reviewer: architect-reviewer
Scope: Solution Architecture, Implementation Approach, Security Considerations, Consequences

---

## 1. Architecture Clarity and Implementability

### Verdict: Implementable with corrections

The design correctly identifies a real wiring gap: the `RegistryChecker` interface exists at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:41-44`, the `FailureCategoryMissingDep` constant exists at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/errors.go:140`, exit code 8 is defined at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/cmd/tsuku/exitcodes.go:32-33`, and the orchestrator's `categoryFromExitCode()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/batch/orchestrator.go:523-540` already maps exit code 8 to `"missing_dep"`. These are all verified against the current codebase.

The overall data flow is accurate. No new packages or interfaces need to be invented.

---

## 2. Missing Components and Interface Gaps

### FINDING 1 (Blocking): Wrong function names in implementation spec

The design says validation should be added to "the three code paths that write `RuntimeDependencies` (`generateBottleRecipe`, `generateDeterministicRecipe`, `generateRecipe`)."

There is no `generateBottleRecipe` function. The actual functions that write `RuntimeDependencies` are:

- `generateRecipe` (line 1954) -- LLM-assisted path
- `generateLibraryRecipe` (line 2046) -- deterministic library path
- `generateToolRecipe` (line 2135) -- deterministic tool path

`generateDeterministicRecipe` (line 2146) is a dispatcher that calls `generateToolRecipe` or `generateLibraryRecipe`; it doesn't write `RuntimeDependencies` itself. The implementer needs to add validation in the three leaf functions, not in the dispatcher.

Getting this wrong would leave one code path unvalidated (the library path), which is the path that generates the most dependency-heavy recipes. This needs correction before implementation.

### FINDING 2 (Blocking): Error routing logic will produce exit code 9, not 8

The design says: "For `FailureCategoryMissingDep`, it should exit with code 8 (`ExitDependencyFailed`) instead of code 9 (`ExitDeterministicFailed`)."

But the current error handler at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/cmd/tsuku/create.go:711-716` is:

```go
var detErr *builders.DeterministicFailedError
if errors.As(err, &detErr) {
    fmt.Fprintf(os.Stderr, "deterministic generation failed: [%s] %s\n",
        detErr.Category, detErr.Message)
    exitWithCode(ExitDeterministicFailed)
}
```

This catches *all* `DeterministicFailedError` instances and always exits with code 9, regardless of the `Category` field. The design doesn't specify where the category-based branching should go or what the code should look like. An implementer reading the design literally would add a `DeterministicFailedError` with `FailureCategoryMissingDep` from the builder, but it would still be caught by the existing handler and exit with code 9.

The fix needs to be explicit: either (a) the handler must check `detErr.Category == FailureCategoryMissingDep` before choosing the exit code, or (b) a separate error type should be used. Option (a) fits the existing pattern. The design should specify this rather than leaving it implicit.

### FINDING 3 (Advisory): Alias-to-canonical name rewriting is underspecified

The design mentions: "when all dependencies resolve via satisfies (the builder should use the canonical recipe name in `RuntimeDependencies`, not the Homebrew alias)."

This is important but underspecified. If the builder writes the canonical name (e.g., `openssl`) instead of the Homebrew name (e.g., `openssl@3`) into `RuntimeDependencies`, this changes the recipe's dependency resolution semantics. Does the install path resolve dependencies by exact recipe name or by satisfies lookup? If only by exact name, the canonical rewrite is correct. If install also does satisfies lookup, it's unnecessary but harmless. The design should clarify which is the case, or point to the install dependency resolution code.

---

## 3. Implementation Phase Sequencing

### Verdict: Correctly sequenced with one gap

Phase 1 (wire up) -> Phase 2 (tests) -> Phase 3 (verify integration) is the right order. The dependency validation is added before tests because tests need something to exercise.

**Gap**: Phase 1 combines four logically separable changes:
1. The `loaderRegistryChecker` adapter
2. Passing it at both call sites
3. Validation logic in the builder
4. Exit code routing fix

Changes 1-3 can be tested in isolation with unit tests. Change 4 requires integration testing against the orchestrator. If the exit code routing is wrong (per Finding 2), unit tests for the builder validation will pass but the pipeline integration will fail silently. The implementation should wire and test the exit code routing as a discrete step with a test that verifies the specific exit code, not just that an error was returned.

---

## 4. Simpler Alternatives

### No simpler approach exists

The design already identifies this as a wiring change, not new architecture. The alternatives considered (validate in CLI layer, fail-fast) are reasonably evaluated. The chosen approach of validating inside the builder is architecturally correct -- it keeps the builder's output contract clean.

One alternative not discussed: **validating in `generateDeterministicRecipe` once, before dispatching to `generateToolRecipe`/`generateLibraryRecipe`**. Since all three leaf functions read from `info.Dependencies`, the validation could happen once in the dispatcher. This would be three lines of code instead of adding validation to three separate functions. The design's claim that `generateDeterministicRecipe` dispatches to both tool and library paths means it's the natural chokepoint. `generateRecipe` (the LLM path) would still need separate validation since it gets dependencies from `data.Dependencies` rather than `info.Dependencies`.

This is a minor simplification, not a different architecture.

---

## 5. Security Analysis

### 5.1 "Not Applicable" Justifications

**Download Verification -- Not Applicable**: Correct. No downloads are introduced.

**Execution Isolation -- Not Applicable**: Correct. Validation runs before installation.

**User Data Exposure -- Not Applicable**: Correct. Only local registry data is read.

All three "not applicable" justifications hold.

### 5.2 Supply Chain Assessment

The design's supply chain analysis is accurate. Dependency names originate from the Homebrew JSON API, which is already the trusted source. The validation adds a check, not a new trust boundary.

The false-negative failure mode (rejecting a valid dependency due to stale registry) is correctly identified as safe -- conservative failure that self-resolves on the next batch run.

### 5.3 Unaddressed Attack Vectors

**No new vectors introduced.** The change is strictly additive (adding a check) and doesn't expand the trust boundary. The `RegistryChecker.HasRecipe()` call reads local files that are already part of the trust model.

**One pre-existing vector worth noting**: The error message includes dependency names sourced from the Homebrew API (e.g., `"recipe openssl@3 not found in registry"`). The orchestrator's `extractBlockedByFromOutput()` regex (`\S+`) would capture any non-whitespace sequence. If a malicious Homebrew formula injected a dependency name like `"foo\nnot found in registry\nrecipe ../../etc/passwd"`, the regex could extract a path-like string. However, `extractBlockedByFromOutput()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/batch/orchestrator.go:559` already validates extracted names and rejects those containing `/`, `\`, `..`, `<`, or `>` (per the pipeline blocker tracking design). This is a pre-existing mitigation, not something this design introduces or needs to address. No residual risk to escalate.

### 5.4 Residual Risk

None that warrants escalation. The failure modes are conservative (false negatives, not false positives), and the attack surface is unchanged.

---

## 6. Consequences Assessment

The identified consequences are accurate:

- The success-to-blocked count shift is real and correctly described. No mitigation needed beyond what the design proposes.
- Registry staleness is correctly identified as self-mitigating through the existing `update-registry` step.

**One unmentioned consequence**: The `batch-generate.yml` workflow (the CI validation pipeline) runs `tsuku install --json`, not `tsuku create`. That pipeline reads `missing_recipes` from JSON output, not from stderr regex extraction. This design only addresses the `tsuku create` path used by the Go orchestrator in `internal/batch/orchestrator.go`. The CI validation workflow is a separate pipeline that validates already-generated recipes; it won't be affected by this change because it doesn't invoke `create`. This isn't a missing consequence, but the design should note this distinction to prevent confusion during implementation.

---

## Summary

| # | Finding | Severity | Action |
|---|---------|----------|--------|
| 1 | Wrong function names: `generateBottleRecipe` doesn't exist; actual targets are `generateRecipe`, `generateLibraryRecipe`, `generateToolRecipe` | Blocking | Correct the function names in the design before implementation |
| 2 | Exit code routing: existing handler catches all `DeterministicFailedError` and always exits 9; design doesn't specify the category-based branch needed to emit exit code 8 | Blocking | Add explicit specification for category check in the error handler |
| 3 | Alias-to-canonical rewriting underspecified | Advisory | Clarify whether install resolves by satisfies or exact name |
| 4 | Possible simplification: validate once in `generateDeterministicRecipe` dispatcher instead of three leaf functions | Advisory | Consider, but not required |
| 5 | Batch CI workflow uses `tsuku install --json`, not `tsuku create` -- unrelated but worth noting to prevent confusion | Advisory | Add a note distinguishing the two pipeline paths |

The design respects the existing architecture. It doesn't introduce parallel patterns, bypasses, or dependency inversions. The `RegistryChecker` interface was built for this exact use, and routing through the existing error/exit-code/category chain is the correct approach. The two blocking findings are about implementation accuracy, not architectural direction.
