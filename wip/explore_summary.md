# Exploration Summary: Library Recipe Generation

## Problem (Phase 1)
The deterministic recipe generator fails on library-only Homebrew bottles because it only inspects bin/ for executables. Packages like bdw-gc and tree-sitter that ship .so/.a/.dylib files in lib/ but no binaries get classified as complex_archive and require manual recipe authoring.

## Decision Drivers (Phase 1)
- Minimize manual recipe authoring for library packages
- Reuse existing library recipe pattern (type = "library", install_mode = "directory")
- Keep deterministic generation self-contained (no LLM fallback needed)
- Handle platform-specific library extensions (.so vs .dylib)
- 22 library recipes already exist as reference patterns

## Research Findings (Phase 2)
- extractBottleBinaries() only scans parts[2] == "bin" in the tar; lib/ and include/ are ignored
- Bottle is already downloaded to a temp file -- scanning lib/ and include/ adds no network cost
- 22 library recipes exist as patterns; all use type = "library" + install_mode = "directory"
- Library bottles have structure: formula/version/lib/*.so, formula/version/include/*.h
- Libraries are exempt from [verify] section requirement (can't execute .so to verify)
- The generate function builds a recipe.Recipe struct directly -- needs a library branch
- 70 unique packages have hit complex_archive in failure data; estimated 20-40 are true libraries
- Platform-specific extensions: .so (Linux), .dylib (macOS), .a (both)
- ToTOML() does not emit metadata.type -- must be fixed for library recipes
- Current code uses "binaries" key; library recipes should use "outputs" (binaries is deprecated)

## Options (Phase 3)
- Decision 1 (Detection): Scan lib/ in bottle tarball when bin/ is empty -> chosen
- Decision 2 (Outputs): Enumerate all lib/ and include/ files -> chosen (fragility documented)
- Decision 3 (Platforms): Multi-platform with when clauses, download multiple bottles -> chosen
- Decision 4 (Categories): Add library_only as subcategory of complex_archive -> chosen

## Phase 4 Review Feedback (incorporated)
- Quantified library fraction estimate (20-40 of 70)
- Changed library_only from top-level category to subcategory (no schema migration)
- Acknowledged ToTOML gap (must emit metadata.type)
- Committed to "outputs" key over deprecated "binaries"
- Explicit verify section handling (omit for libraries)
- Added versioned symlink filtering as rejected alternative under Decision 2

## Current Status
**Phase:** 5 - Decision (complete), proceeding to Phase 8 - Final Review
**Last Updated:** 2026-02-22
