# Exploration Summary: GPU Backend Selection

## Problem (Phase 1)

The tsuku-llm addon builds 10 platform variants (CUDA, Vulkan, Metal, CPU across architectures), but the download system has a 1:1 mapping from OS-architecture to a single binary. There's no way to select which GPU backend to download, no hardware probing before download, and no fallback when a GPU backend fails at runtime.

## Decision Drivers (Phase 1)

- Manifest is embedded in the Go binary at compile time (supply chain security)
- Hardware detection currently lives in Rust, runs after download
- CUDA binaries are coupled to specific toolkit/driver versions
- Vulkan VRAM detection isn't implemented yet (returns 0)
- macOS always uses Metal (no variant selection needed)
- Must preserve atomic download + SHA256 verification workflow
- tsuku's self-contained philosophy: users shouldn't need to install GPU SDKs manually

## Research Findings (Phase 2)

**Upstream context:** DESIGN-local-llm-runtime.md acknowledged GPU variant distribution as an open question. The design deferred it, noting "Haven't fully validated the CI infrastructure for this."

**Industry patterns for GPU variant distribution:**

- **Ollama** (most relevant): Ships a single installer that bundles all backend libraries (cuda_v11, cuda_v12, vulkan, cpu). At runtime, spawns subprocess per backend to probe initialization. If a backend crashes during probing, it's filtered out. Fallback to CPU is transparent. Trade-off: large download (all backends bundled), slow first-run probing (30-90s).

- **llama.cpp**: Ships separate pre-built binaries per backend variant. Users pick the right one manually. The `GGML_BACKEND_DL` mode compiles backends as dynamic libraries for runtime loading, but this isn't the default for releases. Trade-off: small downloads but high user friction.

- **PyTorch**: Separate wheel indexes per CUDA version (`+cu121`, `+cu124`, `+cpu`). PEP 817 (Wheel Variants) is in draft to auto-detect hardware at install time. Trade-off: works well at scale but requires explicit user action today.

**Key codebase patterns:**
- Manifest is `//go:embed` in the Go binary (no external manifest fetches)
- `PlatformKey()` returns `GOOS-GOARCH` -- the only dimension for binary selection
- Download flow: temp file → SHA256 verify → chmod → atomic rename
- Hardware detection in Rust runs at server startup, not at download time
- File-based locking prevents concurrent downloads

**Implications for tsuku:**
- Can't bundle all variants (10 binaries, massive download)
- Can't ask users to pick manually (violates self-contained philosophy)
- Need Go-side hardware probing before download to pick the right variant
- Must add a backend dimension to the manifest alongside the platform dimension

## Options (Phase 3)

4 decision questions, each with chosen approach and alternatives:

1. **Distribution model**: Keep separate per-backend binaries (chosen) vs bundled single binary vs dynamic backend loading
2. **Manifest schema**: Nested variant map with default (chosen) vs flat composite keys vs priority-ordered list
3. **Pre-download backend selection**: Go-side library file probing + config override (chosen) vs shell out to nvidia-smi vs probe binary vs user-config-only
4. **Runtime failure handling**: Informative error + manual override (chosen, auto-fallback deferred) vs pre-download CPU alongside GPU

Key design choice: Vulkan as preferred Linux GPU backend (over CUDA) to avoid driver version coupling. CUDA available as user override via `llm.backend` config.

Review feedback incorporated: moved Vulkan VRAM fix and automatic runtime fallback out of scope; added distro-agnostic probe paths; added variant tracking for checksum verification; added legacy path cleanup.

## Decision (Phase 5)

**Problem:**
The tsuku-llm addon builds 10 platform variants across GPU backends (CUDA, Vulkan, Metal, CPU), but the addon manifest and download system map each OS-architecture pair to a single binary. There's no mechanism to select which GPU variant to download, no hardware probing before download, and no fallback when a GPU backend fails at runtime. Users with NVIDIA GPUs get the wrong binary, and CUDA driver mismatches fail silently.

**Decision:**
Expand the manifest schema to a two-level lookup (platform then backend variant) and add Go-side GPU library probing before download to select the right variant. Vulkan is the preferred GPU backend on Linux to avoid CUDA driver version coupling, with CUDA available as a user override. Automatic runtime fallback is deferred to a follow-up design after measuring detection accuracy in practice.

**Rationale:**
Go-side library probing is fast, requires no extra downloads, and catches the common case (GPU libraries present on disk). Vulkan as default avoids the CUDA toolkit/driver version matrix that caused silent failures on development machines. Separate per-backend binaries (already built by CI) keep downloads small and supply chain verification simple. Deferring automatic runtime fallback keeps the initial scope tight while the config override provides a manual escape hatch for detection errors.

## Current Status
**Phase:** 6 - Architecture (Phases 5+6 complete)
**Last Updated:** 2026-02-19
