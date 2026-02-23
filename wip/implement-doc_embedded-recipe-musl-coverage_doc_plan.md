# Documentation Plan: embedded-recipe-musl-coverage

Generated from: docs/designs/DESIGN-embedded-recipe-musl-coverage.md
Issues analyzed: 5
Total entries: 3

---

## doc-1: docs/GUIDE-hybrid-libc-recipes.md
**Section**: Migration Templates
**Prerequisite issues**: #1912, #1914
**Update type**: modify
**Status**: pending
**Details**: Add a "Template D: Toolchain Recipe" showing the pattern for embedded toolchain recipes that download glibc-linked binaries and fall back to apk_install on musl. The existing templates only cover library recipes and tools with library dependencies. The new template should show the download-with-libc-guard plus apk_install pattern used by rust, nodejs, and similar recipes. Also add a note that toolchain recipes (not just libraries) now follow the hybrid libc pattern.

---

## doc-2: docs/GUIDE-hybrid-libc-recipes.md
**Section**: CI Validation
**Prerequisite issues**: #1915
**Update type**: modify
**Status**: pending
**Details**: Update the CI Validation section to reflect the new structural musl coverage check in AnalyzeRecipeCoverage. Currently the section says only library recipes must have musl support. After #1915, all embedded recipes using download/homebrew actions without libc when clauses and no apk_install fallback are flagged. Also document the supported_libc metadata field for statically-linked tools (go, zig) that work on musl without apk_install.

---

## doc-3: docs/EMBEDDED_RECIPES.md
**Section**: Notes
**Prerequisite issues**: #1912, #1913, #1914
**Update type**: modify
**Status**: pending
**Details**: Add a "musl Support" subsection in Notes explaining that all embedded recipes now support musl-based systems (Alpine Linux). Toolchain and build tool recipes use apk_install with Alpine system packages as a fallback when the glibc dynamic linker isn't available. Reference the hybrid libc guide for the full pattern. This aligns with the existing library recipes section which already has musl support via the work in #1092.
