# Exploration Summary: GPU Backend Selection

## Problem (Phase 1)

The tsuku-llm addon builds 10 platform variants (CUDA, Vulkan, Metal, CPU across architectures), but the download system has a 1:1 mapping from OS-architecture to a single binary. There's no way to select which GPU backend to download, no hardware probing before download, and no fallback when a GPU backend fails at runtime.

More broadly, tsuku has no awareness of GPU hardware at the platform level. The platform detection system knows OS, architecture, Linux family, and libc, but not GPU vendor. This prevents any recipe from filtering on GPU capabilities.

## Decision Drivers (Phase 1)

- tsuku already has a pattern for platform-specific filtering (when clauses with libc, linux_family, etc.)
- Hardware detection currently lives in Rust, runs after download
- CUDA binaries are coupled to specific toolkit/driver versions
- macOS always uses Metal (no variant selection needed)
- tsuku's self-contained philosophy: users shouldn't need to install GPU SDKs manually
- Ecosystem value: other tools may need GPU filtering in the future

## Research Findings (Phase 2)

**Upstream context:** DESIGN-local-llm-runtime.md acknowledged GPU variant distribution as an open question.

**Industry patterns:**
- **Ollama**: Bundles all backends, runtime probing. Trade-off: large download.
- **llama.cpp**: Separate binaries per variant. Trade-off: user picks manually.
- **PyTorch**: Separate wheel indexes per CUDA version. Trade-off: explicit user action.

**Key codebase patterns:**
- `platform.Target` already detects OS, arch, linuxFamily, libc via `DetectTarget()`
- `WhenClause` matches on these dimensions for recipe step filtering
- Step-level dependencies are filtered by when clause (no phantom deps)
- Library recipes exist (`type = "library"`, installed to `$TSUKU_HOME/libs/`)
- GPU vendor detectable via PCI sysfs (stdlib only, no drivers needed)

## Options (Phase 3) - REVISED

4 decision questions, revised after user feedback to align with existing platform/recipe infrastructure:

1. **Where GPU awareness lives**: Platform package extending Matchable (chosen) vs addon-specific detection vs manual config only
2. **How to detect GPU hardware**: PCI sysfs vendor scanning (chosen) vs library file probing vs shell out to nvidia-smi
3. **How tools use GPU info**: Extend WhenClause + tsuku-llm as recipe (chosen) vs custom addon manifest schema vs addon-specific filtering
4. **Runtime failure handling**: Informative error + manual override (chosen, auto-fallback deferred)

Key design choices: GPU as first-class platform dimension. tsuku-llm as standard recipe. GPU drivers as library recipe dependencies. Vulkan as default for all GPU vendors on Linux.

## Decision (Phase 5) - REVISED

**Problem:**
tsuku has no GPU awareness at the platform level, preventing both the tsuku-llm addon and any future recipes from filtering on GPU capabilities. The addon works around this with its own embedded manifest, but that's a one-off mechanism.

**Decision:**
Extend the platform detection system with GPU vendor identification (PCI sysfs on Linux) and add a `gpu` field to WhenClause. Convert tsuku-llm from addon-with-embedded-manifest into a standard recipe with when clauses for variant selection. GPU driver packages become library recipe dependencies.

**Rationale:**
GPU detection follows the same pattern as libc detection (read system file, return string, match in when clause). Reusing existing infrastructure means no new schema formats and any future tool gets GPU filtering for free. Converting tsuku-llm to a recipe eliminates the only non-recipe distribution path in tsuku.

## Current Status
**Phase:** 8 - Final Review (Design revised based on user feedback)
**Last Updated:** 2026-02-19
