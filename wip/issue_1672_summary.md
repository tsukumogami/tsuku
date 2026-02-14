# Issue 1672 Summary

## What Was Implemented

Updated the model manifest in tsuku-llm to use real HuggingFace Hub download URLs and SHA256 checksums for the Qwen 2.5 GGUF models. This enables end-to-end model download and inference without requiring a custom CDN.

## Changes Made

- `tsuku-llm/src/model.rs`: Updated `ModelManifest::new()` with real HuggingFace URLs, accurate file sizes, and SHA256 checksums for all three model variants (0.5B, 1.5B, 3B)
- `internal/llm/lifecycle_integration_test.go`: Updated skip logic to check HuggingFace availability instead of non-existent cdn.tsuku.dev
- `docs/designs/DESIGN-local-llm-runtime.md`: Marked #1672 as done in the issues table and dependency graph

## Key Decisions

- **Use HuggingFace Hub directly**: No need for a custom CDN - HuggingFace already hosts official GGUF releases from Qwen. This eliminates infrastructure costs and maintenance burden.
- **SHA256 from LFS metadata**: Obtained checksums from HuggingFace's Git LFS pointer files, which are authoritative and match what users will download.

## Trade-offs Accepted

- **Dependency on HuggingFace availability**: Model downloads depend on HuggingFace Hub being accessible. This is acceptable because HuggingFace is highly available and widely used infrastructure. The existing retry logic handles transient failures.

## Test Coverage

- Rust unit tests: 48/48 passing (no changes to test code)
- Go integration tests: Updated skip logic; tests will now run when HuggingFace is reachable

## Known Limitations

- Large model downloads (500MB-2GB) may be slow on limited bandwidth connections
- HuggingFace may implement rate limiting on high-volume downloads (mitigated by existing retry with backoff)

## Future Improvements

- Consider caching model files in CI to speed up integration tests
- Could add alternative mirrors if HuggingFace becomes unavailable in certain regions
