# Review: #1645 docs(llm): update documentation for local inference

**Focus**: maintainer (clarity, readability, accuracy)

**Files reviewed**:
- `docs/GUIDE-local-llm.md` (new)
- `docs/ENVIRONMENT.md` (modified)
- `README.md` (modified)

---

## Finding 1: Design doc model table contradicts actual code -- documentation copied the stale version

**File**: `docs/GUIDE-local-llm.md`, lines 24-28 and `docs/designs/DESIGN-local-llm-runtime.md`, lines 291-297
**Severity**: Blocking

The design doc's model selection table (DESIGN-local-llm-runtime.md lines 291-297) lists four models: 3B, 1.5B, 0.5B with GPU, and CPU-only options. The actual code (`tsuku-llm/src/model.rs`) only has two models in the manifest: `qwen2.5-14b-instruct-q4` and `qwen2.5-7b-instruct-q4`. The test at line 541-542 explicitly confirms the smaller models were removed:

```rust
assert!(manifest.get("qwen2.5-3b-instruct-q4").is_none()); // removed: below quality floor
assert!(manifest.get("qwen2.5-0.5b-instruct-q4").is_none()); // removed: below quality floor
```

The GUIDE-local-llm.md correctly reflects the actual implementation (14B and 7B, GPU-only), so the guide itself is accurate. But the design doc's model selection table is now stale. A contributor reading the design doc will form a wrong mental model about CPU support and smaller model options.

**Suggestion**: Update the design doc's Decision 2 table and "Decision Outcome > Summary" paragraph to match reality (14B/7B, GPU-required, no CPU fallback).

---

## Finding 2: ENVIRONMENT.md missing TSUKU_LLM_MODEL and TSUKU_LLM_BACKEND

**File**: `docs/ENVIRONMENT.md`
**Severity**: Blocking

The GUIDE-local-llm.md (lines 81-87) documents two environment variables:
- `TSUKU_LLM_MODEL` -- override model selection
- `TSUKU_LLM_BACKEND` -- override inference backend

The Rust code (`tsuku-llm/src/main.rs` lines 703-705) confirms these are real:
```rust
local_model: std::env::var("TSUKU_LLM_MODEL").ok().filter(|s| !s.is_empty()),
local_backend: std::env::var("TSUKU_LLM_BACKEND").ok().filter(|s| !s.is_empty()),
```

But ENVIRONMENT.md only documents `TSUKU_LLM_IDLE_TIMEOUT`. The summary table at the bottom also omits these two variables. A developer looking at ENVIRONMENT.md for the full list of environment variables won't find them.

**Suggestion**: Add `TSUKU_LLM_MODEL` and `TSUKU_LLM_BACKEND` entries to the "Local LLM Runtime" section in ENVIRONMENT.md, and add them to the summary table.

---

## Finding 3: Config reference shows `local_model` and `local_backend` but code only supports `backend`

**File**: `docs/designs/DESIGN-local-llm-runtime.md` lines 211-215, `docs/GUIDE-local-llm.md` lines 56-57
**Severity**: Advisory

The design doc's config example shows:
```toml
local_model = "qwen2.5-1.5b-instruct-q4"
local_backend = "cuda"
```

The actual `userconfig.go` has no `local_model` config key. The `Set` method's switch statement does not include `llm.local_model`. There's only `llm.backend` (not `llm.local_backend`).

The GUIDE-local-llm.md handles this correctly -- it does NOT show a `local_model` config key and documents `backend` (not `local_backend`). It says "There's no config key for overriding model selection" (line 77). It correctly shows `TSUKU_LLM_MODEL` as an env-var-only override.

The design doc is stale here but since the guide is correct, this is advisory. The next developer reading only the guide will have the right understanding.

---

## Finding 4: Guide's backend config documents values inconsistent with code

**File**: `docs/GUIDE-local-llm.md` lines 55-57
**Severity**: Blocking

The guide says:
```toml
# Valid values: "cpu" (force CPU variant). Leave unset for auto-detection.
# backend = "cpu"
```

The `userconfig.go` (line 88) confirms:
```go
var validLLMBackends = []string{"cpu"}
```

But the env var table at lines 85-86 says:
```
| `TSUKU_LLM_BACKEND` | Override inference backend | `cuda`, `metal`, `vulkan` |
```

And the Rust code's `Backend::from_str` (model.rs lines 38-46) accepts `cuda`, `metal`, `vulkan` but NOT `cpu`. The `Backend` enum has no CPU variant.

So there are two contradictions the next developer hits:
1. The config key `llm.backend` only accepts `"cpu"` (Go side), but the env var `TSUKU_LLM_BACKEND` accepts `cuda`/`metal`/`vulkan` (Rust side). These are different override mechanisms with different valid values, but the guide doesn't make this distinction clear.
2. The guide's env var table shows `cuda`, `metal`, `vulkan` as examples for `TSUKU_LLM_BACKEND`, which is correct for the Rust addon. But a reader might try `tsuku config set llm.backend cuda` which would fail because Go only accepts `"cpu"`.

**Suggestion**: The guide should explicitly note that `llm.backend` in config.toml controls which addon BINARY VARIANT to download (only `cpu` to force CPU build), while `TSUKU_LLM_BACKEND` overrides which INFERENCE BACKEND the running addon uses. These are different layers. Or consolidate the valid values.

---

## Finding 5: Design doc says GBNF grammar constraints force JSON; code shows they're disabled

**File**: `docs/designs/DESIGN-local-llm-runtime.md` line 544, `docs/GUIDE-local-llm.md` line 13
**Severity**: Advisory

The design doc says "Tool calling is implemented through GBNF grammar constraints that force valid JSON". The guide says "The addon constrains output to valid JSON using grammar rules, same as cloud providers."

The actual code (`tsuku-llm/src/main.rs` lines 394-399) has a comment:
```rust
// NOTE: Grammar-constrained generation is disabled due to llama.cpp compatibility
// issues with Qwen models (crashes with "Unexpected empty grammar stack").
```

The addon uses prompt engineering + JSON extraction instead. The guide's claim that grammar rules constrain output is misleading -- it uses JSON extraction from free-form output. This affects quality expectations but won't cause a developer to introduce a bug, so advisory.

---

## Finding 6: Guide says "~50 MB" for addon; README says "~50 MB" -- both correct but unverifiable

**File**: `docs/GUIDE-local-llm.md` line 9, `README.md` line 119
**Severity**: Advisory (observation only)

Both documents state the addon binary is ~50 MB. This is a claim about the Rust binary size after compilation. No code enforces this -- it's just an estimate. If the binary grows, both places need updating. Consider adding a comment near one of them noting the source of this estimate.

---

## Finding 7: README LLM section is clear and well-structured

The README's "LLM-Powered Recipe Generation" section (lines 104-149) cleanly separates which builders need LLM from those that don't, frames local inference as the default, and provides a direct path to cloud fallback. The "Local LLM Management" section (lines 216-228) is concise. No issues here.

---

## Summary

| Severity | Count |
|----------|-------|
| Blocking | 3 |
| Advisory | 3 |

**Blocking issues**:
1. Design doc model table is stale (lists 3B/1.5B/0.5B models and CPU support; code only has 14B/7B with GPU-required).
2. ENVIRONMENT.md missing `TSUKU_LLM_MODEL` and `TSUKU_LLM_BACKEND` -- the guide documents them but the canonical env var reference doesn't.
3. Config `llm.backend` accepts only `"cpu"`, but env var `TSUKU_LLM_BACKEND` accepts `cuda`/`metal`/`vulkan` -- the guide's config and env var sections give conflicting mental models about what "backend override" means.

The GUIDE-local-llm.md is well-written and mostly accurate. The hardware requirements table matches the actual model selection code. The troubleshooting section covers the right failure modes. The main issues are consistency gaps between the guide, ENVIRONMENT.md, and the design doc.
