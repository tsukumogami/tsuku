# Architect Review: Issue #1779 (feat(llm): add structured error for backend init failure)

## Summary

This change adds three public functions to `hardware.rs` (`compiled_backend()`, `describe_detected_hardware()`, `format_backend_init_error()`) and uses them in `main.rs` to emit a structured error to stderr when the compiled GPU backend fails to initialize at runtime. The error message tells users to run `tsuku config set llm.backend cpu` as a workaround.

## Structural Assessment

The change fits the existing architecture cleanly.

**Layering is correct.** The new functions live in `hardware.rs`, which is the right module -- it already owns `GpuBackend`, `HardwareProfile`, and `HardwareDetector`. The `compiled_backend()` function queries cargo feature flags, which is build-time information about the hardware backend. `format_backend_init_error()` composes the compile-time and runtime hardware info into a user-facing message. Both belong in the hardware module.

**Dependency direction is correct.** `main.rs` imports from `hardware`, not the reverse. No new cross-module dependencies are introduced.

**No parallel patterns.** The error formatting doesn't duplicate any existing error mechanism. `llama/error.rs` defines `LlamaError` for operational errors during inference (model load, tokenization, decode). The new `format_backend_init_error()` produces a user-facing diagnostic message for a startup failure, not an error type for programmatic handling. These serve different purposes with different consumers.

**Consumer exists.** The Go-side lifecycle manager pipes tsuku-llm's stderr to `os.Stderr` (`internal/llm/lifecycle.go:165`), so the formatted error reaches the user. The design doc (`docs/designs/DESIGN-gpu-backend-selection.md:731`) explicitly states "This is an informational message, not a protocol change. The Go side doesn't parse it." The message format matches the design doc's specification (lines 726-729).

**Two callsites, same pattern.** The error formatting is used in both the model loading failure path (`main.rs:767-774`) and the context creation failure path (`main.rs:803-809`). Both are correct -- either path can fail when the GPU backend can't initialize. Both follow the same pattern: format error, write to stderr, log to tracing, cleanup files, exit.

## Findings

### Advisory: `describe_detected_hardware()` duplicates `GpuBackend::Display`

`describe_detected_hardware()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/tsuku-llm/src/hardware.rs:91-98` maps `GpuBackend` variants to strings ("CUDA", "Metal", "Vulkan", "None"). The `Display` impl for `GpuBackend` at lines 22-31 does the same mapping but with lowercase ("cuda", "metal", "vulkan", "cpu"). The difference is casing and "cpu" vs "None" for the no-GPU case.

This is a minor redundancy. The two representations serve different audiences (Display is for machine-readable output like the status response backend field; `describe_detected_hardware` is for human-readable error messages). But if `GpuBackend::None`'s display value changes, this function won't update in sync.

Not blocking because: the function has exactly one caller (`format_backend_init_error`), so the duplication is contained. If it grows more callers, consider unifying the display representations.

## Verdict

No blocking findings. The change implements a design-specified feature in the correct module, respects existing layering, and introduces no parallel patterns.

| Level | Count |
|-------|-------|
| Blocking | 0 |
| Advisory | 1 |
