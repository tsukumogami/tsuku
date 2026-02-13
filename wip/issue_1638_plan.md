# Issue 1638 Implementation Plan

## Summary

Integrate llama.cpp into tsuku-llm using the `cc` crate and cmake for build, with safe Rust wrappers that enforce memory lifecycle via Drop implementations. The approach follows patterns from llama-cpp-sys-2 while simplifying for tsuku's needs.

## Approach

Use bindgen for raw FFI declarations combined with thin safe Rust wrappers. This balances:
- Keep up with llama.cpp API changes (minimal hand-written FFI)
- Memory safety via RAII wrappers with Drop
- Feature flags for GPU backends (metal, cuda, vulkan) already defined in Cargo.toml

### Alternatives Considered

- **Use llama-cpp-2 crate directly**: Not chosen because it adds external dependency management complexity, version pinning is harder, and we need tight integration with ModelManager's verified paths.

- **Pure cc crate without cmake**: Not chosen because llama.cpp's build system is complex (conditional GPU backends, SIMD detection, platform-specific flags) and cmake handles this better. cc used only for wrapper compilation.

- **Hand-written FFI bindings**: Not chosen because llama.cpp API changes frequently; bindgen auto-generates bindings from header files, reducing maintenance burden.

## Files to Modify

- `tsuku-llm/Cargo.toml` - Add cc, cmake, bindgen build-dependencies; add libloading for runtime linking
- `tsuku-llm/build.rs` - Add llama.cpp compilation via cmake, bindgen for FFI declarations
- `tsuku-llm/src/main.rs` - Wire up model loading at startup, update Complete RPC to use inference

## Files to Create

- `tsuku-llm/src/llama/mod.rs` - Module exports and backend initialization
- `tsuku-llm/src/llama/bindings.rs` - Bindgen-generated raw FFI declarations (include! macro)
- `tsuku-llm/src/llama/model.rs` - Safe LlamaModel wrapper with Drop for llama_model_free
- `tsuku-llm/src/llama/context.rs` - Safe LlamaContext wrapper holding Arc<LlamaModel>
- `tsuku-llm/src/llama/error.rs` - LlamaError enum with variants for load, context, decode failures
- `tsuku-llm/src/llama/params.rs` - ModelParams and ContextParams structs wrapping C defaults
- `tsuku-llm/src/llama/batch.rs` - Safe LlamaBatch wrapper for token batching
- `tsuku-llm/src/llama/sampler.rs` - Token sampling utilities (greedy, temperature)

## Implementation Steps

- [ ] Add llama.cpp as git submodule at `tsuku-llm/llama.cpp/` pinned to specific commit
- [ ] Update Cargo.toml with build-dependencies: bindgen, cmake, cc
- [ ] Extend build.rs to compile llama.cpp via cmake with feature flag configuration
- [ ] Configure bindgen to generate bindings.rs from llama.h
- [ ] Create src/llama/mod.rs with backend_init/backend_free functions
- [ ] Create src/llama/error.rs with LlamaError enum and Result type alias
- [ ] Create src/llama/params.rs with ModelParams and ContextParams structs
- [ ] Create src/llama/model.rs with LlamaModel wrapper (load_from_file, Drop)
- [ ] Create src/llama/context.rs with LlamaContext wrapper (tokenize, decode, get_logits, Drop)
- [ ] Create src/llama/batch.rs with LlamaBatch wrapper (add_token, clear, Drop)
- [ ] Create src/llama/sampler.rs with greedy token selection
- [ ] Integrate LlamaModel loading into main.rs server startup
- [ ] Update Complete RPC stub to perform actual inference
- [ ] Add unit tests for safe wrapper memory lifecycle
- [ ] Add integration test loading a small GGUF model

## Testing Strategy

### Unit Tests
- `LlamaModel::Drop` properly frees model (no double-free, use-after-free)
- `LlamaContext::Drop` properly frees context
- `LlamaBatch::Drop` properly frees batch
- Parameter builders produce valid C structs
- Error conversion from non-zero return codes

### Integration Tests
- Load a small test GGUF model (download via ModelManager or fixture)
- Tokenize input text and verify token count
- Run single decode step and get logits
- Full inference loop producing output tokens

### Manual Verification
- Run with RUST_LOG=debug to observe llama.cpp initialization
- Monitor memory with valgrind to confirm no leaks
- Test on each supported platform (Linux, macOS, Windows)

## Risks and Mitigations

- **llama.cpp API instability**: Mitigated by pinning to specific commit in submodule, documenting version in build.rs comments
- **Memory safety with C FFI**: Mitigated by wrapping all raw pointers in types with Drop, using Arc for shared ownership between Context and Model
- **Build complexity across platforms**: Mitigated by using cmake (handles platform detection), feature flags for optional backends
- **GPU memory errors (OOM)**: Mitigated by catching llama.cpp error returns and converting to Rust errors before panicking
- **UTF-8 encoding**: Mitigated by validating CString conversions, returning errors on invalid sequences

## Success Criteria

- [ ] `cargo build` compiles llama.cpp and links correctly on Linux
- [ ] `cargo build --features metal` compiles with Metal on macOS
- [ ] `cargo build --features cuda` compiles with CUDA on Linux (when toolkit available)
- [ ] All existing tests pass (baseline: 39 tests)
- [ ] New unit tests for safe wrappers pass
- [ ] Integration test loads model and runs inference without crash
- [ ] Valgrind shows no memory leaks in inference path
- [ ] Server returns actual inference output instead of stub response

## Open Questions

None blocking. The implementation context and llama.cpp API research provide sufficient detail to proceed.

## Technical Notes

### Build Configuration

The build.rs will follow this pattern from llama-cpp-sys-2:

```rust
// 1. Configure cmake for llama.cpp
let mut cmake_config = cmake::Config::new("llama.cpp");
cmake_config
    .define("BUILD_SHARED_LIBS", "OFF")
    .define("LLAMA_BUILD_TESTS", "OFF")
    .define("LLAMA_BUILD_EXAMPLES", "OFF");

// Feature-gated backends
#[cfg(feature = "cuda")]
cmake_config.define("GGML_CUDA", "ON");

#[cfg(feature = "metal")]
cmake_config.define("GGML_METAL", "ON");

#[cfg(feature = "vulkan")]
cmake_config.define("GGML_VULKAN", "ON");

let dst = cmake_config.build();

// 2. Link the static library
println!("cargo:rustc-link-search=native={}/lib", dst.display());
println!("cargo:rustc-link-lib=static=llama");
println!("cargo:rustc-link-lib=static=ggml");
```

### Safe Wrapper Pattern

```rust
pub struct LlamaModel {
    ptr: NonNull<llama_model>,
}

impl LlamaModel {
    pub fn load_from_file(path: &Path, params: ModelParams) -> Result<Self> {
        let c_path = CString::new(path.to_str().ok_or(LlamaError::InvalidPath)?)?;
        let ptr = unsafe { llama_model_load_from_file(c_path.as_ptr(), params.into_raw()) };
        let ptr = NonNull::new(ptr).ok_or(LlamaError::ModelLoadFailed)?;
        Ok(Self { ptr })
    }
}

impl Drop for LlamaModel {
    fn drop(&mut self) {
        unsafe { llama_model_free(self.ptr.as_ptr()) }
    }
}

// Context holds Arc<LlamaModel> to ensure model outlives context
pub struct LlamaContext {
    ptr: NonNull<llama_context>,
    _model: Arc<LlamaModel>, // prevents model from being freed while context exists
}
```

### Dependency Diagram

```
ModelManager (models.rs)
     |
     | model_path() -> PathBuf
     v
LlamaModel (llama/model.rs)
     |
     | Arc<LlamaModel>
     v
LlamaContext (llama/context.rs)
     |
     | tokenize(), decode(), get_logits()
     v
LlamaServer (main.rs Complete RPC)
```
