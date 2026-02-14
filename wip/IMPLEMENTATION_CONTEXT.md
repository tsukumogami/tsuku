---
summary:
  constraints:
    - Model manifest is embedded in the addon binary (not external config)
    - SHA256 verification required at download AND before each model load
    - Must use HuggingFace Hub direct download URLs (no custom CDN needed)
    - Models: Qwen 2.5 in 0.5B, 1.5B, 3B sizes with Q4_K_M quantization
  integration_points:
    - tsuku-llm/src/models.rs - ModelManager downloads from manifest URLs
    - tsuku-llm/src/model.rs - ModelSelector uses model names from manifest
    - internal/llm/lifecycle_integration_test.go - Tests skip when CDN unavailable
  risks:
    - HuggingFace URL format may differ from expected (case sensitivity, exact filenames)
    - Large file downloads (500MB-2.5GB) - need to verify checksums correctly
    - Integration tests currently skipped - need to enable once manifest is correct
  approach_notes: |
    1. Find exact GGUF filenames from HuggingFace repos for each model size
    2. Download files and compute SHA256 checksums locally
    3. Update manifest.json with real URLs, checksums, and sizes
    4. Test model download works end-to-end
    5. Re-enable integration tests that were skipped due to missing CDN
---

# Implementation Context: Issue #1672

**Source**: docs/designs/DESIGN-local-llm-runtime.md

## Key Points

- **ModelManager** (`src/models.rs`) manages model downloads in `$TSUKU_HOME/models/`
- The manifest maps model names to download URLs and SHA256 checksums
- Design expects models from "tsuku's CDN" but we're using HuggingFace Hub instead
- SHA256 verification happens at download AND before each model load (security requirement)

## Model Selection Table

| Available Resources | Model | Size (Q4) |
|-------------------|-------|-----------|
| 8GB+ VRAM (CUDA/Metal) | Qwen 2.5 3B | ~2.5GB |
| 4-8GB VRAM | Qwen 2.5 1.5B | ~1.5GB |
| CPU only, 8GB+ RAM | Qwen 2.5 1.5B | ~1.5GB |
| CPU only, <8GB RAM | Qwen 2.5 0.5B | ~500MB |

## HuggingFace Sources

- https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF
- https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF
- https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF

Direct download URL format: `https://huggingface.co/Qwen/Qwen2.5-{SIZE}-Instruct-GGUF/resolve/main/{filename}.gguf`
