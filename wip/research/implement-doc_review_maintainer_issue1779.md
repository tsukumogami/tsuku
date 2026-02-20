# Maintainer Review: Issue #1779 -- Structured Error for Backend Init Failure

## Summary

The commit adds three public functions to `hardware.rs` (`compiled_backend()`, `describe_detected_hardware()`, `format_backend_init_error()`) and uses them in two error-handling blocks in `main.rs` (model loading failure at line 764 and context creation failure at line 798). Five new tests cover these functions.

The code is clear and well-scoped. A next developer can understand the flow: binary compiled with one GPU feature, hardware detection discovers what's actually available, mismatch produces a structured error on stderr.

## Findings

### 1. `describe_detected_hardware` is a redundant layer over `Display` -- Advisory

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/tsuku-llm/src/hardware.rs:91-98`

```rust
pub fn describe_detected_hardware(profile: &HardwareProfile) -> String {
    match profile.gpu_backend {
        GpuBackend::Cuda => "CUDA".to_string(),
        GpuBackend::Metal => "Metal".to_string(),
        GpuBackend::Vulkan => "Vulkan".to_string(),
        GpuBackend::None => "None".to_string(),
    }
}
```

This function duplicates the concept already expressed by `GpuBackend::Display` (lines 22-31), but with different casing ("CUDA" vs "cuda", "None" vs "cpu"). The next developer will see `GpuBackend` has a `Display` impl and wonder why it isn't used in `format_backend_init_error`. The Display impl maps `None` to `"cpu"` while this function maps it to `"None"` -- the divergence is intentional (Display is for config values, this is for human-readable diagnostic), but it's invisible without reading both functions carefully.

Not blocking because the function is only called from one place (`format_backend_init_error`) and the tests document the expected output. But adding a one-line doc comment like "Uses human-readable names (CUDA, None) rather than config-style names (cuda, cpu)" would prevent the next person from "fixing" this to use `Display`.

**Advisory.**

### 2. Suggestion in error message hardcodes a command that may not exist yet -- Advisory

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/tsuku-llm/src/hardware.rs:109`

```rust
"Suggestion: tsuku config set llm.backend cpu",
```

If `tsuku config set` isn't implemented yet, this sends the user on a wrong path. However, this is a forward reference to planned CLI functionality, so it's a known gap rather than a misread risk.

The bigger question: when the detected hardware actually supports a different GPU backend (e.g., compiled=cuda, detected=vulkan), the suggestion always says "switch to CPU." A next developer might expect the suggestion to say "switch to vulkan" when Vulkan is detected. The current behavior is defensible (CPU always works), but the error message doesn't explain why it doesn't suggest the detected backend.

**Advisory.**

### 3. Duplicate error handling blocks in main.rs -- Advisory

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/tsuku-llm/src/main.rs:764-776` and `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/tsuku-llm/src/main.rs:798-810`

```rust
// Block 1 (model loading, line 764):
let compiled = hardware::compiled_backend();
let error_msg = hardware::format_backend_init_error(
    compiled, &hardware_profile,
);
eprintln!("{}", error_msg);
error!("Model loading failed: {}", e);
cleanup_files(&socket, &lock);
std::process::exit(1);

// Block 2 (context creation, line 798):
let compiled = hardware::compiled_backend();
let error_msg = hardware::format_backend_init_error(compiled, &hardware_profile);
eprintln!("{}", error_msg);
error!("Context creation failed: {}", e);
cleanup_files(&socket, &lock);
std::process::exit(1);
```

These two blocks are nearly identical -- the only difference is the `error!()` log message. A next developer modifying one (say, adding a return code or telemetry) might miss the other. At two occurrences this is tolerable, but worth noting as a consolidation candidate if a third call site appears.

**Advisory.**

### 4. `format_backend_init_error` assumes all model load failures are backend init failures -- Blocking

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/tsuku-llm/src/main.rs:764-776`

```rust
Err(e) => {
    // Model loading failed -- this is where a compiled-in GPU backend
    // fails to initialize
    let compiled = hardware::compiled_backend();
    let error_msg = hardware::format_backend_init_error(
        compiled, &hardware_profile,
    );
    eprintln!("{}", error_msg);
```

The comment says "this is where a compiled-in GPU backend fails to initialize," but `LlamaModel::load_from_file` can fail for many other reasons: corrupt file, wrong GGUF format version, insufficient memory, filesystem permission error. In all of these cases, the user sees `ERROR: Backend "vulkan" failed to initialize` with a suggestion to switch backends, which is a misleading error message that sends debugging in the wrong direction.

The next developer who sees a user report about a corrupt model file will look at this error path and either (a) not realize the error message is misleading, or (b) need to add error-type discrimination that should already be here.

A guard that checks whether the error is actually backend-related (e.g., pattern matching on the error message from llama.cpp, or at minimum logging the actual error `e` to stderr alongside the structured message) would prevent wrong debugging paths. The `error!()` call does log `e` via tracing, but the user-facing stderr message (`eprintln!`) says nothing about the actual error, and most users won't have RUST_LOG enabled.

The same issue applies to the context creation block at line 798.

**Blocking.**

### 5. Test names and coverage are solid -- No finding

The five new tests (`test_compiled_backend_returns_valid_variant`, `test_describe_detected_hardware`, and the three `test_format_backend_init_error_*` variants) match their names and cover the key scenarios. The test for `compiled_backend` correctly avoids asserting a specific variant since it depends on cargo features.

## Severity Summary

| # | Finding | Severity | File:Line |
|---|---------|----------|-----------|
| 1 | `describe_detected_hardware` duplicates Display with different casing, undocumented divergence | Advisory | `hardware.rs:91-98` |
| 2 | Suggestion hardcodes `tsuku config set` and always suggests CPU even when another GPU backend is available | Advisory | `hardware.rs:109` |
| 3 | Duplicate error-handling blocks in main.rs | Advisory | `main.rs:764-776, 798-810` |
| 4 | All model load / context creation failures emit backend-init error, misleading for non-backend failures | Blocking | `main.rs:764-776, 798-810` |

## Recommended Fix for #4

Include the underlying error in the user-facing message so the user and supporting developer can distinguish backend failures from other causes:

```rust
Err(e) => {
    let compiled = hardware::compiled_backend();
    let error_msg = hardware::format_backend_init_error(
        compiled, &hardware_profile,
    );
    eprintln!("{}", error_msg);
    eprintln!("  Underlying error: {}", e);  // <-- add this
    error!("Model loading failed: {}", e);
    cleanup_files(&socket, &lock);
    std::process::exit(1);
}
```

This way even if the structured message is wrong about the cause, the actual error is visible on stderr without requiring tracing configuration.
