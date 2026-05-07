# Design Summary: tsuku-homebrew-dylib-chaining

## Input Context (Phase 0)

**Source:** /explore handoff (round 1 findings)

**Problem:** tsuku's homebrew action does not chain dylibs from sibling tsuku-installed library deps into a tool recipe's RPATH. The mechanism exists for `Type == "library"` recipes (`fixLibraryDylibRpaths`) but is gated to that one type. Tool recipes whose homebrew bottles reference non-system shared libraries (libutf8proc, libevent, libidn2, gettext, etc.) cannot wire those refs to the matching tsuku-installed deps without an explicit `set_rpath` chain — and that chain has been adopted by exactly 1 recipe out of the 1168 homebrew-using recipes in the registry.

**Blast radius (from exploration):** >15 affected recipes by every accounting. Top-100 strict: 10 tools at-risk. Including current handcrafted recipes that exist only because of bespoke workarounds: 19+. Including macOS punts: 26+. Crosses the >15 stop-signal threshold, ruling tsuku-core as the right fix level.

**Constraints:**
- Must not break existing `Type == "library"` recipes that depend on current `fixLibraryDylibRpaths` behavior.
- Public repo; design must read for external contributors.
- macOS Mach-O (`@rpath` + `install_name_tool`) and Linux ELF (`patchelf` + `$ORIGIN`) need different patching primitives.
- Recipe schema change is acceptable but should be additive (no breaking changes to existing recipes).
- Must work with the existing sandbox foundation-image build flow.

## Scope

**In:**
- The dylib chaining mechanism (or lack thereof) for tool recipes that use homebrew bottles.
- How `RuntimeDependencies` and per-step `dependencies` flow from recipe schema → ExecutionContext → homebrew action's RPATH patching.
- Both Linux ELF and macOS Mach-O paths.
- Backward compatibility with existing curated tool and library recipes.

**Out:**
- Refactoring the homebrew action beyond what's needed to fix the dylib chain.
- Source-build escapes (already a documented fallback for problematic bottles like curl).
- The findMake/gmake fallback (separate concern, already in flight).
- Foundation-image-build infrastructure (cascade gaps with infra-package-vs-tsuku-managed-deps ordering — separate issue).
- Replacing homebrew bottles entirely with source builds (philosophical question outside this design's scope).

## Current Status

**Phase:** 0 — Setup (Explore Handoff)

**Last updated:** 2026-05-04

**Next:** Phase 1 (Decision Decomposition) — break the design into independent decision questions.
