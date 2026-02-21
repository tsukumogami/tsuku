# Scrutiny Review: Intent - Issue #1645

**Issue**: #1645 - docs(llm): update documentation for local inference
**Focus**: intent (design intent alignment + cross-issue enablement)
**Files changed**: `README.md`, `docs/ENVIRONMENT.md`, `docs/GUIDE-local-llm.md`

---

## Sub-check 1: Design Intent Alignment

### Finding 1: Hardware requirements table documents models that no longer exist (BLOCKING)

**What the documentation says** (`docs/GUIDE-local-llm.md` lines 22-28):

| Available Resources | Model | Download Size |
|---------------------|-------|---------------|
| 8 GB+ VRAM (CUDA/Metal) | Qwen 2.5 3B Q4 | ~2.5 GB |
| 4-8 GB VRAM | Qwen 2.5 1.5B Q4 | ~1.5 GB |
| CPU only, 8 GB+ RAM | Qwen 2.5 1.5B Q4 | ~1.5 GB |
| CPU only, < 8 GB RAM | Qwen 2.5 0.5B Q4 | ~500 MB |
| < 4 GB RAM | Disabled | -- |

**What the code actually does** (`tsuku-llm/src/model.rs` lines 205-274):

- The manifest contains only two models: `qwen2.5-14b-instruct-q4` (9 GB) and `qwen2.5-7b-instruct-q4` (4.7 GB)
- Test at line 541 explicitly confirms: `assert!(manifest.get("qwen2.5-3b-instruct-q4").is_none()); // removed: below quality floor`
- Test at line 542 explicitly confirms: `assert!(manifest.get("qwen2.5-0.5b-instruct-q4").is_none()); // removed: below quality floor`
- Model selection requires GPU (`SelectionError::NoGpuDetected` when `gpu_backend == GpuBackend::None`)
- Minimum VRAM is 8 GB (line 212: `const MINIMUM_VRAM_GB: f64 = 8.0`)
- Selection logic: 14 GB+ VRAM -> 14B model, 8-14 GB -> 7B model
- CPU-only inference is not supported at all (line 12-14: "CPU-only inference is not supported because models below 7B... are too slow on CPU to be practical")

The documentation's hardware table is entirely wrong. Every row is inaccurate:
- The 3B, 1.5B, and 0.5B models don't exist in the manifest
- CPU-only rows describe behavior that returns an error in the actual code
- The 4-8 GB VRAM row describes a configuration that errors out (minimum is 8 GB)
- The < 4 GB RAM row says "disabled" but the actual threshold is < 8 GB VRAM with GPU required

A user with 6 GB VRAM reads this table and expects "Qwen 2.5 1.5B Q4, Good quality." They'll actually get an error: "insufficient VRAM: 6.0 GB available, 8.0 GB required for minimum model (7B)."

A user with CPU-only hardware reads this table and expects "Qwen 2.5 1.5B Q4, Good (slower)." They'll actually get: "no GPU detected: tsuku-llm requires a GPU with at least 8 GB VRAM."

**Severity**: Blocking. The documentation will send users on a wrong debugging path. They'll think their hardware is supported, try to use local inference, get a cryptic error, and not understand why the documented behavior doesn't match.

### Finding 2: Config reference documents non-existent TOML keys `local_model` and `local_backend` (BLOCKING)

**What the documentation says** (`docs/GUIDE-local-llm.md` lines 51-56):

```toml
# Override automatic model selection. Leave unset for auto-detection.
# local_model = "qwen2.5-1.5b-instruct-q4"

# Override automatic GPU backend. Leave unset for auto-detection.
# Valid values: "cpu"
# local_backend = "cpu"
```

**What the code actually does**:

The Go `LLMConfig` struct (`internal/userconfig/userconfig.go` lines 35-71) has no `local_model` field. It has a `Backend` field (TOML key `backend`), not `local_backend`. The `tsuku config set` command supports `llm.backend` (line 399), not `llm.local_backend` or `llm.local_model`.

The Rust addon reads model and backend overrides from environment variables `TSUKU_LLM_MODEL` and `TSUKU_LLM_BACKEND` (`tsuku-llm/src/main.rs` lines 704-705), not from config.toml.

Additionally, the documented valid value `"cpu"` for `local_backend` won't work in the Rust addon. The `Backend::from_str` implementation (model.rs lines 36-46) rejects "cpu" with the error "unknown backend: cpu (gpu required, cpu not supported)". The Go config validator (`validLLMBackends`, line 88) only accepts `"cpu"` for `llm.backend`, which selects the CPU variant of the addon binary -- this is different from forcing CPU inference, which isn't supported.

A user reading this guide who adds `local_model = "qwen2.5-1.5b-instruct-q4"` to their config.toml will see no effect. The key is silently ignored because the Go TOML struct doesn't have that field. The model they specified doesn't exist in the manifest anyway.

The documentation on line 71 shows `tsuku config set llm.backend cpu` which IS a valid command (it sets the Go-side backend to download the CPU addon variant), but the guide presents it in the context of `local_backend` which doesn't exist as a TOML key. The next developer maintaining this will be confused about the relationship between these three different mechanisms (Go config `backend`, Rust env `TSUKU_LLM_BACKEND`, and documented but nonexistent `local_backend`).

**Severity**: Blocking. Users will add config keys that silently do nothing. The example model name `qwen2.5-1.5b-instruct-q4` references a model that has been removed from the manifest.

### Finding 3: Troubleshooting omits GPU requirement (Advisory)

The troubleshooting section at line 163 says: "Insufficient hardware? Systems with less than 4 GB RAM can't run local inference."

The actual requirement is a GPU with 8 GB+ VRAM. A user with 16 GB RAM but no discrete GPU reads this and thinks they should be fine. The actual error they'll see is "no GPU detected: tsuku-llm requires a GPU with at least 8 GB VRAM."

The "Slow inference on CPU" section (lines 179-187) suggests CPU-only inference takes "5-30 seconds per turn" and recommends installing GPU drivers or using a smaller model. But CPU inference is not supported at all -- the addon won't start without a GPU.

**Severity**: Advisory (borderline blocking). The troubleshooting section actively gives wrong advice for the most common failure mode (no GPU), but the error message from the addon itself is clear enough that users will likely figure it out.

### Finding 4: README local_backend comment says "cpu" but design says "cuda, metal, vulkan, cpu" (Advisory)

The guide's config reference (line 55) says `# Valid values: "cpu"` for `local_backend`. The design doc (line 215) says `local_backend = "cuda"  # or "metal", "vulkan", "cpu"`. Neither matches the actual Rust backend enum which accepts cuda/metal/vulkan but not cpu.

The Go-side `llm.backend` config does accept only "cpu" (line 88: `var validLLMBackends = []string{"cpu"}`), but this controls which addon binary variant to download, not which inference backend to use. This is a different concept entirely.

**Severity**: Advisory. The mismatch across design doc, guide, and code creates confusion, but it's commented out in the guide so fewer users will hit it.

## Sub-check 2: Cross-Issue Enablement

No downstream issues depend on #1645 (terminal issue). Skipped.

## Backward Coherence

No previous_summary provided (not applicable for this review context).

---

## Summary

| Severity | Count |
|----------|-------|
| Blocking | 2 |
| Advisory | 2 |
