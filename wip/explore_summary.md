# Exploration Summary: Optional Executables in Compile Actions

## Problem (Phase 1)
The `configure_make`, `cmake_build`, and `meson_build` actions require at least one executable name. This blocks their use for library-only packages (gmp, abseil, cairo) that produce `.so`/`.a` files but no binaries.

## Decision Drivers (Phase 1)
- Backward compatibility: existing recipes must continue working
- Library recipes already have `type = "library"` and optional verify sections
- `ExtractBinaries()` already handles missing executables gracefully
- `install_binaries` with `install_mode = "directory"` handles library outputs
- meson_build uses executables for RPATH fixup (needs special handling)

## Research Findings (Phase 2)
- 3 library recipes use `apk_install` on musl as a workaround (gmp, abseil, cairo)
- `apk_install` loses version pinning and self-containment guarantees
- Only `configure_make` has Preflight validation for executables; cmake/meson skip static validation
- The post-build verify step (checking bin/<exe> exists) is the only thing executables drives
- For meson_build, executables also drives RPATH fixup on binaries in bin/
- Library RPATH is handled by the build system itself, not the post-build fixup

## Options (Phase 3)
- Skip post-build verification (chosen): empty executables list means loops are no-ops
- Add outputs parameter to compile actions: rejected, duplicates install_binaries
- Require explicit empty list: rejected, awkward TOML, contradicts omission pattern

## Decision (Phase 5)

**Problem:**
The configure_make, cmake_build, and meson_build actions require an executables parameter with at least one binary name. This prevents using them for library-only packages that produce .so/.a files and headers but no executables. Three library recipes (gmp, abseil, cairo) work around this by using apk_install on musl Linux, which loses version pinning and self-containment.

**Decision:**
Make executables optional in all three compile actions. When omitted, the actions skip post-build binary verification and RPATH fixup on executables. The subsequent install_binaries step with its outputs parameter still validates that the build produced expected files. configure_make's Preflight validation downgrades executables from required to optional.

**Rationale:**
This is the minimum change that unblocks library compilation. The post-build executable check exists as a convenience verification, not a security gate -- the install_binaries step already validates outputs independently. Making executables optional rather than introducing new parameters or action variants keeps the surface area small and avoids recipe-level churn for existing packages.

## Current Status
**Phase:** 8 - Review Final (complete)
**Last Updated:** 2026-02-23
