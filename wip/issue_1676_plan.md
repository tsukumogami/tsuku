# Issue 1676 Implementation Plan

## Summary

Fix tokenization by correctly interpreting `llama_tokenize` return values. When passed a null buffer or insufficient buffer size, the function returns the negative of the required token count, not an error.

## Root Cause

In `tsuku-llm/src/llama/context.rs`, the `tokenize` method calls `llama_tokenize` with `null` and `0` to query the required token count. The code incorrectly treats a negative return value as an error:

```rust
if n_tokens < 0 {
    return Err(LlamaError::Tokenization(
        "llama_tokenize returned negative count".to_string(),
    ));
}
```

However, according to the llama.cpp API (see `llama-vocab.cpp:3640-3642`):
- When `n_tokens_max < required_tokens`, the function returns `-required_tokens`
- A negative return is NOT an error; it's the negated required buffer size

The correct pattern (from `simple.cpp:97`) is:
```cpp
const int n_prompt = -llama_tokenize(vocab, prompt.c_str(), prompt.size(), NULL, 0, true, true);
```

## Approach

The fix involves:
1. Negate the return value to get the actual required token count
2. Only treat `INT32_MIN` as an error (indicates tokenization overflow)
3. Allocate the buffer with the correct size
4. Call `llama_tokenize` again with the properly sized buffer

## Files to Modify

- `tsuku-llm/src/llama/context.rs` - Fix the tokenization logic
- `internal/llm/lifecycle_integration_test.go` - Remove skip directive from `TestIntegration_gRPCComplete`

## Files to Create

None.

## Implementation Steps

- [ ] **Step 1: Fix tokenize method in context.rs**
  - Change the first call to negate the return value
  - Only treat `i32::MIN` as an error (overflow)
  - The negated value is the required buffer size

- [ ] **Step 2: Remove skip directive from integration test**
  - Remove the skip from `TestIntegration_gRPCComplete`

- [ ] **Step 3: Run tests to verify fix**
  - Build tsuku-llm: `cd tsuku-llm && cargo build --release`
  - Run Rust unit tests: `cargo test`
  - Run Go integration tests: `go test -tags=integration -v ./internal/llm/... -run gRPCComplete`

## Testing Strategy

**Unit tests:**
- Existing Rust tests should continue to pass

**Integration tests:**
- `TestIntegration_gRPCComplete`: Should now pass - sends completion request and receives response

**Manual verification:**
1. Build and start daemon: `./tsuku-llm serve`
2. Send a gRPC Complete request
3. Verify tokenization succeeds and response is returned

## Risks and Mitigations

- **Risk: Edge case where result is exactly INT32_MIN**
  - This indicates tokenization result exceeds int32 limit (very rare)
  - Return error in this case per llama.cpp semantics

## Success Criteria

- [ ] `TestIntegration_gRPCComplete` passes
- [ ] All 48 Rust tests pass
- [ ] All Go non-integration tests pass
- [ ] No regressions in other functionality

## Open Questions

None. The fix is straightforward based on the llama.cpp API documentation and examples.
