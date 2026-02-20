# Pragmatic Review: Issue #1779 (structured error for backend init failure)

## Summary

3 new functions in `hardware.rs` (`compiled_backend`, `describe_detected_hardware`, `format_backend_init_error`), 2 call sites in `main.rs` (model load failure, context creation failure), 5 new tests. Scope is tight and matches the issue.

## Findings

### 1. `describe_detected_hardware` is a single-caller wrapper over Display -- Blocking

`tsuku-llm/src/hardware.rs:91-98` -- `describe_detected_hardware` is called only from `format_backend_init_error`. It's a match on `GpuBackend` that returns title-cased strings. `GpuBackend` already implements `Display` (lowercase). If the title-case formatting matters for the error message, inline it into `format_backend_init_error`. If it doesn't, just use the existing `Display` impl.

This function also has its own dedicated test (`test_describe_detected_hardware`) covering all 4 variants, which adds 20 lines of test code for a trivial match expression.

**Fix:** Inline into `format_backend_init_error` or use `Display`. Remove `test_describe_detected_hardware`.

### 2. `test_compiled_backend_returns_valid_variant` tests an exhaustive match -- Advisory

`tsuku-llm/src/hardware.rs:491-498` -- The test asserts that `compiled_backend()` returns one of the enum variants. The Rust compiler already enforces this via the return type `GpuBackend`. The match statement in the test is exhaustive by construction. The only value of this test is "doesn't panic," which the `#[cfg]` blocks guarantee.

**Fix:** Could remove, but harmless. Low priority.

### 3. Duplicate error-handling block in main.rs -- Advisory

`tsuku-llm/src/main.rs:764-775` and `tsuku-llm/src/main.rs:800-809` -- The model-load failure and context-creation failure blocks are near-identical (get `compiled_backend`, format error, eprintln, cleanup, exit). This is fine for 2 call sites and the blocks are adjacent enough to find. Not worth extracting given the `process::exit` and varying log messages.

No action needed; noting for completeness.

## No Findings

- `compiled_backend()` is justified: it queries compile-time features (`cfg`), and both call sites need it. Not over-engineered.
- `format_backend_init_error()` is justified: formats a multi-line structured message used in 2 places. Testable.
- No scope creep: changes are limited to the error path, no unrelated refactors.
- Test count (5 new) is reasonable for a simple-tier issue minus the `describe_detected_hardware` test which tests dead-weight.

## Verdict

1 blocking, 2 advisory.
