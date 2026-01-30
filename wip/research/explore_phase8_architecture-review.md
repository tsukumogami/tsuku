# Architecture Review: DESIGN-homebrew-deterministic-mode

## 1. Is the architecture clear enough to implement?

**Yes.** The design is well-structured and implementation-ready. Key observations:

- The solution architecture section provides concrete Go code snippets for the new error type, modified Generate/Repair flows, and session option changes. An implementer can follow these directly.
- The data flow diagram clearly shows the interaction between the batch pipeline and the builder.
- Failure classification table maps internal errors to schema categories with clear "when" conditions.

**One gap:** The design mentions fixing a nil dereference at homebrew.go:430 (line 46, scope section) but doesn't describe the fix in the solution architecture. Looking at the code, line 430 calls `s.provider.Name()` before the provider is initialized (it's nil until LLM fallback). The fix is straightforward -- the progress message at line 430 must be moved inside `generateBottle()` after `ensureLLMProvider()` succeeds, or the provider name must be replaced with a placeholder. However, this should be stated explicitly in the solution architecture since it's listed as in-scope work.

## 2. Are there missing components or interfaces?

**No missing components.** The design correctly identifies that:

- `SessionBuilder` interface is unchanged (same signature, different return value for `RequiresLLM()`)
- `BuildSession` interface is unchanged
- `SessionOptions` gets one new field
- One new error type and category enum are added
- `RepairNotSupportedError` already exists and is reused correctly

**Minor observations:**

- The `DeterministicFailedError` should implement `Unwrap() error` to support `errors.Is`/`errors.As` chains. The design shows an `Err` field but doesn't show the `Unwrap()` method. Other error types in `errors.go` (e.g., `GitHubRateLimitError`, `SandboxError`) do implement `Unwrap()`. This is a small omission.
- The `DeterministicFailedError` could benefit from a `Suggestion() string` method to match the pattern used by other error types in the codebase (`RateLimitError`, `BudgetError`, `GitHubRateLimitError`, etc.). Not strictly required since the batch pipeline won't display suggestions, but it would maintain consistency.

## 3. Are the implementation phases correctly sequenced?

**Yes, the sequencing is correct:**

- Phase 1 (error type + session option) is purely additive -- no behavioral change.
- Phase 2 (Generate/Repair guards + classification) depends on Phase 1 types existing.
- Phase 3 (RequiresLLM change) is independent of Phase 2 but logically follows it since tests in Phase 4 need both.
- Phase 4 (tests) depends on all prior phases.

**Suggestion:** Phase 3 could be merged into Phase 2 since it's a one-line change. Keeping it separate is fine for PR review clarity, but it could also be a single commit with Phase 2.

## 4. Are there simpler alternatives we overlooked?

The design already considered and rejected the simpler alternatives (Option 1B: separate builder type, Option 3C: batch pipeline handles differently). The rejections are well-reasoned.

**One alternative worth noting:** Instead of a `DeterministicOnly` session option, the builder could detect "no LLM available" by checking that `opts.LLMConfig` is nil or LLM is disabled, and treat that as implicit deterministic-only mode. This would avoid the new flag entirely. However, this would make the behavior implicit rather than explicit, and a caller could accidentally get deterministic-only behavior by misconfiguring LLM options. The design's explicit flag approach is better.

## Summary

The architecture is clear, complete, and correctly sequenced. Two small gaps to address before implementation:

1. Describe the nil dereference fix (homebrew.go:430) in the solution architecture section.
2. Add `Unwrap() error` method to `DeterministicFailedError` to match codebase conventions.

Neither gap blocks implementation.
