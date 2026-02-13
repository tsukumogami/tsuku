---
summary:
  constraints:
    - Build llama.cpp via cc crate as part of cargo build (not separate build step)
    - Safe Rust wrappers must enforce proper memory lifecycle (Drop for model/context)
    - Context is not thread-safe; serialize requests or use context pooling
    - Only load manifest-listed models (SHA256 verified)
    - File path must be validated (exists, readable, within $TSUKU_HOME/models/)
    - Pin llama.cpp to specific commit for reproducible builds
  integration_points:
    - tsuku-llm/build.rs - Add cc crate configuration for llama.cpp compilation
    - tsuku-llm/src/llama/ - New module for safe Rust bindings
    - src/models.rs - ModelManager provides model paths for loading
    - src/model.rs - ModelSpec provides model metadata
    - src/hardware.rs - HardwareProfile informs backend selection
    - proto/llm.proto - Complete RPC will use inference (future #1640)
  risks:
    - Memory safety with C FFI - must wrap all raw pointers in safe types with Drop
    - GPU memory errors (CUDA OOM, Metal allocation failure) need graceful handling
    - llama.cpp API stability - pin to specific commit, document breaking changes
    - Build complexity across platforms (Metal/CUDA/Vulkan/CPU feature flags)
    - UTF-8 encoding errors when converting between C strings and Rust strings
  approach_notes: |
    Create a new llama/ module with:
    - bindings.rs: Raw unsafe extern "C" declarations for llama.cpp API
    - model.rs: Safe LlamaModel wrapper with Drop for llama_free_model
    - context.rs: Safe LlamaContext wrapper holding Arc<LlamaModel> for lifetime
    - error.rs: LlamaError enum with clear error variants
    - params.rs: ModelParams and ContextParams structs

    Build system:
    - Add cc build-dependency to Cargo.toml
    - Update build.rs to compile llama.cpp sources with appropriate flags
    - Feature flags control backend compilation (metal, cuda, vulkan)

    Integration with existing code:
    - ModelManager.model_path() returns the GGUF file location
    - HardwareProfile.gpu_backend determines which llama.cpp backend to use
    - Main server will load model on startup and reuse for inference
---

# Implementation Context: Issue #1638

**Source**: docs/designs/DESIGN-local-llm-runtime.md (Phase 4: Model Manager and Inference)

## Key Design Points for #1638

### What This Issue Delivers
- Build llama.cpp as part of `cargo build` using the `cc` crate
- Safe Rust bindings for llama.cpp's C API (model loading, context creation, inference)
- Proper memory lifecycle management (Drop implementations, no leaks)
- Error handling that surfaces llama.cpp failures clearly

### What This Issue Does NOT Deliver
- GBNF grammar constraints (#1639)
- gRPC endpoint wiring (#1640)
- Streaming token generation (future work)
- Hardware-specific optimizations beyond what llama.cpp provides

### Dependencies Already Implemented
- Hardware detection (HardwareProfile) - #1635
- Model selection (ModelSelector, ModelSpec) - #1636
- Model download (ModelManager, SHA256 verification) - #1637

### Downstream Dependents
- #1639 (GBNF grammar) - needs inference runtime
- #1640 (Complete RPC) - wires gRPC to inference

### Security Considerations
- All raw pointers wrapped in safe Rust types with Drop
- Lifetimes enforced via type system (Context holds Arc<Model>)
- File paths validated before loading
- Token buffer sizes validated before allocation
- String conversions handle UTF-8 errors
- llama.cpp version pinned in build.rs

### File Structure to Create
```
tsuku-llm/
  build.rs              # Add cc crate for llama.cpp
  llama.cpp/            # Git submodule or vendored
  src/
    llama/
      mod.rs            # Module exports
      bindings.rs       # Raw C bindings (unsafe)
      model.rs          # Safe LlamaModel wrapper
      context.rs        # Safe LlamaContext wrapper
      error.rs          # Error types
      params.rs         # Parameter structs
```
