# Pragmatic Review: DESIGN-gpu-backend-selection.md

## Summary

The design is well-structured and mostly follows existing patterns. Phase 1 (GPU detection + WhenClause) is clean and justified. The main concerns are scope coupling, a speculative CUDA path, and the driver library recipes introducing a new trust boundary (system package managers) under the same umbrella as a platform detection change.

---

## Findings

### 1. Scope coupling: GPU detection + addon-to-recipe migration should be separate designs

**Severity: Blocking (scope creep)**

The design bundles two independent changes:
- (A) GPU detection in `platform` + `gpu` field in `WhenClause`
- (B) Converting tsuku-llm from addon-with-manifest to standard recipe

(A) is a clean platform extension with clear value for any future GPU-aware recipe. (B) is a significant refactor of the LLM addon lifecycle (`EnsureAddon`, `ServerLifecycle`, manifest removal). They have different risk profiles, different reviewers, and different rollback stories.

The design even acknowledges this: "The addon lifecycle code stays separate from the recipe." But then it couples them into a single four-phase implementation.

**Fix:** Split into two designs. Design 1: GPU platform detection + WhenClause (Phase 1 only). Design 2: tsuku-llm recipe migration (depends on Design 1). This lets you ship GPU detection and validate sysfs accuracy before committing to the addon refactor.

### 2. Speculative CUDA step and `llm.backend` override mechanism

**Severity: Blocking (speculative generality)**

Lines 455-465: The CUDA recipe step is described as "only included if CUDA override support is added" with a `note` field. Lines 411-418: The `llm.backend` override modifies the target's GPU value before plan generation, requiring special-case code in `installViaRecipe()`.

The design defaults all Linux GPU users to Vulkan. The CUDA path exists only as a hypothetical override. No benchmarks have been run yet (the design itself has a benchmark gate). If Vulkan performs within 25%, CUDA support is never needed. If it doesn't, the design needs rethinking anyway.

**Fix:** Remove all CUDA-specific content (the CUDA step sketch, the `cuda-runtime` recipe, the `llm.backend=cuda` row in the override table). Ship Vulkan-only + CPU. Add CUDA only if the benchmark gate fails. Keep `llm.backend=cpu` as the only override (simple: override GPU to `"none"`).

### 3. GPU driver library recipes invoke system package managers

**Severity: Blocking (new trust/complexity boundary)**

Lines 424-453: `vulkan-loader.toml` uses `apt_install`, `dnf_install`, `pacman_install` to install system packages. This means `tsuku install tsuku-llm` on a fresh system triggers `sudo apt install libvulkan1`. This contradicts tsuku's "no system dependencies / no sudo" philosophy (CLAUDE.md: "installs tools to `~/.tsuku/` without requiring sudo").

The design doesn't discuss what happens when: (a) the user doesn't have sudo, (b) the Vulkan loader is already installed system-wide, (c) tsuku removes tsuku-llm -- does it also `apt remove libvulkan1`?

**Fix:** First check if `libvulkan.so.1` already exists on the system (most GPU-enabled Linux installs have it). Only prompt for system package install as a fallback. Document the sudo requirement. Or: bundle a Vulkan loader binary as a tsuku-managed library (consistent with self-contained philosophy).

### 4. `llm.backend` override is underspecified for the `vulkan` value

**Severity: Advisory**

Line 417: `llm.backend=vulkan` maps to "(any non-`none` value, e.g., detected value)". If `GPU()` returns `"none"` (no GPU hardware), setting `llm.backend=vulkan` does... what? The detected value is `"none"`, which would match the CPU step, not Vulkan. The user explicitly asked for Vulkan but gets CPU.

**Fix:** When `llm.backend=vulkan`, override GPU to a synthetic value like `"vulkan-override"` that matches the Vulkan step's `gpu = ["nvidia", "amd", "intel"]`... except it won't match that list either. This needs explicit design. Simplest: `llm.backend` overrides the *step selection*, not the *target GPU value*. But that's a different mechanism than what's described.

### 5. `macOS returns "apple" unconditionally` -- dead dimension

**Severity: Advisory**

Lines 317-318: On macOS, `DetectGPU()` returns `"apple"` always. The tsuku-llm recipe's macOS steps don't use `gpu` in their `when` clause (lines 187-195: `when = { os = ["darwin"], arch = "arm64" }`). So the `"apple"` value is never matched against. It's a dimension that exists in the detection code but serves no filtering purpose.

This is harmless today but adds noise. Every `MatchTarget` construction in tests will need a GPU parameter, even for macOS tests where it's meaningless.

**Fix:** Return `""` (empty) on macOS instead of `"apple"`. The WhenClause GPU check already skips when the target GPU is empty (same pattern as libc on non-Linux). Add `"apple"` later if a recipe actually needs it.

### 6. `NewTarget` signature change ripple

**Severity: Advisory**

`platform.NewTarget(platform, family, libc)` gains a `gpu` parameter. This function is called in `plan_generator.go:668`, `family.go:135`, and likely in tests. The design correctly notes this is a breaking (internal) change. But the `Matchable` interface change is more impactful: `recipe.MatchTarget` (used throughout executor tests) also gains a constructor parameter.

Not a design flaw, just a heads-up: this will touch many test files. Consider making GPU an option or a setter to minimize test churn for the 99% of recipes that don't use GPU filtering.

### 7. Pre-execution verification drops from compile-time to recipe-time

**Severity: Advisory**

Lines 536-542 acknowledge the security trade-off. The design mentions retaining `VerifyBeforeExecution` by "reading the plan's stored checksum." But the plan is cached in `$TSUKU_HOME`, which is user-writable. An attacker who can modify the binary can also modify the cached plan's checksum. The pre-execution check becomes a self-referential integrity test.

This is the same model every other recipe uses, so it's consistent. But the design should be explicit: pre-execution verification no longer catches local tampering. It only catches corruption (bit rot, incomplete writes).

### 8. Phase 4 "Benchmarking" is gating but has no implementation details

**Severity: Advisory**

Lines 528-530: "This benchmark can be informal (run both variants on the same machine, compare throughput) but must happen before this design is marked Current." There's no issue tracking it, no acceptance criteria beyond "25% gap," and no plan for which machine to benchmark on.

**Fix:** Create a concrete benchmark issue before starting Phase 1. Define the machine, the models, the metric (tokens/sec for generation, not prompt processing).

---

## What's good

- GPU detection via sysfs is the right call. No subprocess, no library probing, no driver dependency. Clean.
- Adding `gpu` to `WhenClause` following the libc pattern is the simplest correct approach for the platform extension.
- Defaulting to Vulkan over CUDA for all GPU vendors is pragmatic. Avoids CUDA driver version hell.
- Deferring automatic fallback is correct. Ship detection + manual override, measure, then automate.
- Removing the embedded manifest and parallel download path is the right long-term direction.

## Recommendation

Split the design. Ship Phase 1 (GPU detection + WhenClause) as its own design. Strip CUDA speculation. Address the driver library sudo question before Phase 3.
