# Issue 1672 Implementation Plan

## Summary

Update the model manifest in `tsuku-llm/src/model.rs` to use real HuggingFace Hub URLs and SHA256 checksums for the Qwen 2.5 GGUF models.

## Changes Required

### File: `tsuku-llm/src/model.rs`

Update `ModelManifest::new()` (lines 146-189) with real values:

| Model | Old URL | New URL |
|-------|---------|---------|
| 3B | `cdn.tsuku.dev/models/qwen2.5-3b-instruct-q4_k_m.gguf` | `huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf` |
| 1.5B | `cdn.tsuku.dev/models/qwen2.5-1.5b-instruct-q4_k_m.gguf` | `huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/main/qwen2.5-1.5b-instruct-q4_k_m.gguf` |
| 0.5B | `cdn.tsuku.dev/models/qwen2.5-0.5b-instruct-q4_k_m.gguf` | `huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf` |

### Model Data (from HuggingFace LFS metadata)

| Model | Size (bytes) | SHA256 |
|-------|--------------|--------|
| qwen2.5-3b-instruct-q4 | 2104932768 | 626b4a6678b86442240e33df819e00132d3ba7dddfe1cdc4fbb18e0a9615c62d |
| qwen2.5-1.5b-instruct-q4 | 1117320736 | 6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e |
| qwen2.5-0.5b-instruct-q4 | 491400032 | 74a4da8c9fdbcd15bd1f6d01d621410d31c6fc00986f5eb687824e7b93d7a9db |

## Implementation Steps

1. **Update model.rs manifest**
   - Replace placeholder URLs with HuggingFace resolve URLs
   - Replace placeholder SHA256 hashes with real checksums
   - Update size_bytes with accurate file sizes

2. **Verify changes compile**
   - Run `cargo build` in tsuku-llm
   - Run `cargo test` to ensure existing tests pass

3. **Update Go integration test skip logic**
   - Remove or modify `skipIfModelCDNUnavailable` since HuggingFace should be accessible
   - Alternatively, update the check to verify HuggingFace availability

4. **Update design doc**
   - Mark #1672 as done in the dependency graph

## Test Strategy

- Rust unit tests continue to pass (they mock the manifest)
- Integration tests that were skipping due to CDN unavailability should now be able to download from HuggingFace
- Manual verification: `cargo run -- serve` should successfully download a model

## No Alternatives Considered

This is a straightforward data update - the code is already implemented, just needs correct URLs and checksums.

## Risks

- HuggingFace rate limiting on downloads (mitigated: retry with backoff already implemented)
- URL format changes on HuggingFace (low risk: resolve/main is stable)
