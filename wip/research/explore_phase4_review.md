# Phase 4: Design Document Review -- Architect Reviewer

## Scope

Review of "DESIGN: Dependency Validation in Batch Pipeline" focusing on the Context and Problem Statement, Decision Drivers, and Considered Options sections.

---

## 1. Problem Statement Specificity

**Verdict: Specific enough to evaluate solutions against.**

The problem statement identifies:
- The exact gap: `create.go` never passes a `RegistryChecker` to `NewHomebrewBuilder()` (verified at line 507 of `cmd/tsuku/create.go`)
- The observable symptom: recipes with unresolvable `RuntimeDependencies` classified as successes
- Four concrete downstream effects (dashboard, requeue, user experience, reorder)

One thing worth making explicit: the problem statement says "the batch pipeline treats these as generation successes because no error is emitted during the `create` step." This is accurate -- when `create` exits 0, the orchestrator (`internal/batch/orchestrator.go:372`) treats it as success. The current `DeterministicFailedError` handler at `create.go:716` sends ALL deterministic failures to exit code 9 (`ExitDeterministicFailed`), which maps to `generation_failed` in `categoryFromExitCode()`. Even if the builder DID emit a `FailureCategoryMissingDep` error today, it would be bucketed as `generation_failed`, not `missing_dep`. The design acknowledges this in the Decision Outcome section but doesn't highlight it in the problem statement. Minor gap -- doesn't affect solution evaluation.

## 2. Missing Alternatives

**Two alternatives worth considering that aren't discussed.**

### Alternative A: Validate in the orchestrator's generate() method (post-hoc)

The orchestrator at `internal/batch/orchestrator.go:370-397` already has access to the generated recipe (or could read it from disk after a successful create). It could validate `RuntimeDependencies` there, re-classifying successes as blocked when dependencies are missing. This avoids changing the builder at all.

Why it's probably worse: it splits the validation from the data source (the builder knows the deps, the orchestrator would have to re-parse them), and it means the recipe is already written to disk before the validation catches it. But it should be mentioned and rejected, because someone will suggest it.

### Alternative B: Have the builder silently filter unresolvable dependencies instead of failing

Instead of failing with an error, the builder could drop unresolvable deps from `RuntimeDependencies` and emit a warning. The recipe would generate successfully with a partial dependency list. Users could install the tool even if some deps are missing.

This is a genuinely different trade-off: it prioritizes generating something installable over correctness. The design implicitly rejects it by framing the problem as "recipes that look valid but fail at install time," but doesn't discuss it. Worth mentioning and rejecting -- silent data loss in dependency lists would create a harder-to-debug failure mode at install time, and the partial recipe gives a false sense of completeness.

Neither of these is a serious contender, but acknowledging and rejecting them strengthens the design by showing the decision space was fully explored.

## 3. Rejection Rationale Fairness

### Decision 1: Where to validate

**"Validate in the CLI layer after recipe generation" -- rejection is fair and specific.**

The rationale ("it weakens the builder's contract -- callers would need to remember to validate after every call") is accurate. The builder already has the `registry` field and `WithRegistryChecker` option. The interface exists specifically for this purpose. Putting validation in create.go would mean every caller of `NewHomebrewBuilder` needs to independently validate, which is the classic "defensive programming at the wrong layer" problem. This isn't a strawman -- it's a real option that's correctly rejected.

### Decision 2: Fail-fast vs collect-all

**"Fail on first missing dependency" -- rejection is fair but slightly overstates the cost.**

The rationale says "N runs to discover N blockers." This is accurate in the worst case, but in practice the batch pipeline would discover at least one blocker per run, and the entry would be marked as `blocked` after the first failure. Subsequent blockers would only be discovered when the first blocker is resolved and the entry retries. The practical cost isn't N full batch runs -- it's that `BlockedBy` would only ever have one entry per failure record, making the dashboard's "top blockers" list undercount.

That said, collect-all is clearly the right choice for the stated decision driver ("complete information per pass"). The rejection just slightly overstates the waste.

## 4. Unstated Assumptions

### Assumption 1: `recipe.Loader` is available at both call sites in create.go

The design says to wrap `recipe.Loader` as the `RegistryChecker`. At the builder registry call site (line ~507), the loader isn't created yet at that point in the function. Looking at `create.go`, the `recipe.Loader` is created somewhere in the function flow. The design should verify that both call sites have access to a loader instance, or specify where it needs to be initialized.

Checking the discovery path (line ~1044): this is inside `runDiscovery()`, a separate function. It doesn't currently have a `recipe.Loader`. The design would need to either pass a loader into `runDiscovery()` or create one there. This is an implementation detail, but it affects the "minimal blast radius" claim -- `runDiscovery()` would gain a new parameter.

### Assumption 2: The Homebrew API dependency names match what tsuku recipes and satisfies entries use

The design assumes `info.Dependencies` from the Homebrew API uses names that can be looked up via `recipe.Loader.Get()` or `LookupSatisfies()`. In practice, Homebrew uses formula names like `openssl@3`, `python@3.12`, `berkeley-db@5`. The satisfies system only covers a few recipes (openssl, gcc-libs, python-standalone per memory). For most Homebrew dependency names, neither `Get()` nor `LookupSatisfies()` will match, because the tsuku recipe names differ from Homebrew formula names.

This means the validation will classify most dependencies as "missing" -- which is probably correct (those recipes genuinely don't exist in the registry yet), but the design should explicitly acknowledge that the satisfies fallback only covers a handful of cases today. It's not a bug in the design, but it affects expectations about how many entries will shift from "success" to "blocked."

### Assumption 3: The error message format is stable

The design relies on the error message matching `recipe (\S+) not found in registry`. This regex is defined in three places: `cmd/tsuku/install.go:389`, `internal/batch/orchestrator.go:553`, and `cmd/remediate-blockers/main.go:32`. The design adds a fourth producer of messages in this format. There's no shared constant for the pattern string. This isn't a new problem (the duplication already exists), but the design should note that adding another producer makes the implicit format contract more load-bearing.

### Assumption 4: The `generateBottle` path also needs validation

The design mentions "three code paths that write RuntimeDependencies" as `generateBottleRecipe`, `generateDeterministicRecipe`, and `generateRecipe`. The actual function names are `generateRecipe` (line 1915), `generateLibraryRecipe` (line 2018), and `generateToolRecipe` (line 2093). The `generateBottle` function (line 568) doesn't directly write `RuntimeDependencies` -- it calls these lower-level functions. The design should use the correct function names to avoid confusion during implementation. `generateDeterministicRecipe` (line 2146) is a dispatcher that calls either `generateToolRecipe` or `generateLibraryRecipe`, so validating in the three leaf functions is correct.

## 5. Strawman Assessment

**No options are strawmen.** Both rejected alternatives are plausible approaches that a reasonable developer might suggest:

- CLI-layer validation is the natural first instinct ("validate before writing the file")
- Fail-fast is the Go convention for error handling and would be the default without specific batch pipeline requirements

Both are rejected on specific, verifiable grounds tied to the codebase's existing patterns, not on vague principles.

## 6. Structural Fit Assessment (Architect Perspective)

The design fits the existing architecture well:

- **No action dispatch bypass**: Validation happens inside the builder, which is the correct layer. The builder's `RegistryChecker` interface was designed for this.
- **No dependency direction violation**: `create.go` (CLI layer) creates the adapter and passes it down to the builder (lower layer). Dependencies flow downward.
- **No parallel pattern introduction**: Uses the existing `DeterministicFailedError` type, the existing `FailureCategoryMissingDep` constant, and the existing exit code mapping. No new error types or categories.
- **Preserves the batch pipeline contract**: Exit code 8 already maps to `missing_dep` in the orchestrator. The error message format matches the existing regex. No orchestrator changes needed.

One minor structural concern: the `loaderRegistryChecker` adapter is placed in `create.go` (the CLI layer). This means only the CLI can create a registry-aware builder. If another entry point (like a future `check-deps` command) needs the same validation, it would duplicate the adapter. A cleaner placement would be in `internal/recipe/` or `internal/builders/` where it's importable. But the design correctly notes that the `RegistryChecker` interface already lives in `internal/builders/`, so the adapter could live there too. This is advisory, not blocking -- the adapter is trivial and has no callers besides create.go today.

## Summary

| Question | Assessment |
|----------|-----------|
| Problem specific enough? | Yes. Identifies exact gap, observable symptoms, and downstream effects. |
| Missing alternatives? | Two minor ones: orchestrator-side validation and silent filtering. Neither is competitive but should be mentioned for completeness. |
| Rejection rationale fair? | Yes. CLI-layer rejection is well-grounded. Fail-fast rejection slightly overstates batch cost but reaches the right conclusion. |
| Unstated assumptions? | Four: loader availability at both call sites, Homebrew-to-tsuku name mapping coverage, error message format stability, and correct function names for the three write points. |
| Any strawmen? | No. Both rejected alternatives are reasonable approaches with specific rejection criteria. |

The design is solid. The unstated assumptions are implementation details that won't change the architectural decision, but should be documented to prevent surprises during implementation.
