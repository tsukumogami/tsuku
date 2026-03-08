# Design Summary: unified-release-versioning

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** Tsuku's three binaries (CLI, dltest, llm) lack unified versioning. They release independently with inconsistent naming, different version resolution, and incomplete version enforcement. Only dltest has compile-time pinning; llm accepts any version.
**Constraints:**
- GPU builds (CUDA/Vulkan) add ~10 min but parallelize fully
- No consumers of separate llm release tags exist (zero migration risk)
- Backend suffix asymmetry (macOS implicit Metal, Linux explicit cuda/vulkan) is intentional
- Proto evolution for gRPC handshake is safe in proto3 but requires coordinated release

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-03-08
