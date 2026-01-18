# Exploration Summary: library-verify-dlopen

## Problem (Phase 1)

Levels 1-2 of library verification can't definitively confirm a library will load. The only way to know is to call `dlopen()`, which requires a separate helper binary since tsuku is built with CGO_ENABLED=0. The helper executes library initialization code, creating security considerations that must be addressed.

## Decision Drivers (Phase 1)

- Security: Code execution must be isolated and opt-out-able
- Trust chain: Helper binary verifiable without network requests
- Performance: Batching for libraries with many files
- Graceful degradation: Missing helper shouldn't block verification
- Cross-platform: Linux (glibc/musl) and macOS (arm64/x86_64)
- Debuggability: dlopen errors must surface clearly

## Research Findings (Phase 2)

**CGO_ENABLED=1 alternative rejected:** Would require complex build infrastructure (musl static linking or zig cross-compiler) to maintain portability. Helper binary approach keeps main tsuku distribution simple.

**Existing pattern:** nix-portable provides a template—embedded checksums, version tracking, file locking, atomic installs.

**dlopen semantics:** Use RTLD_NOW for verification to catch symbol resolution failures immediately.

## Options (Phase 3)

**Decision 1 - dlopen invocation:**
- 1A: Dedicated helper binary (follows nix-portable pattern)
- 1B: Python ctypes fallback (common but slower)
- 1C: CGO in main tsuku (simplest but portability issues)

**Decision 2 - Communication protocol:**
- 2A: JSON stdout (extensible, debuggable)
- 2B: Exit codes only (simple but no detail)
- 2C: Line-based text (streamable but fragile)

**Decision 3 - Batching:**
- 3A: Multiple paths per invocation (fast, some crash risk)
- 3B: One path per invocation (safe but slow)

## Decision (Phase 5)

**Problem:** Levels 1-2 of library verification can't confirm a library will actually load; only dlopen() can test this, but it requires cgo which conflicts with tsuku's static build.
**Decision:** Use a dedicated helper binary (tsuku-dltest) with JSON protocol and batched invocation, following the existing nix-portable pattern.
**Rationale:** This preserves tsuku's simple distribution model while providing isolated code execution, embedded checksum verification, and good performance through batching.

## Current Status

**Phase:** 5 - Decision (complete)
**Last Updated:** 2026-01-18
