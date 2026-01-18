# Exploration Summary: Recipe Registry Separation

## Problem (Phase 1)

As tsuku matures, embedding all 171 recipes in the CLI binary creates unnecessary binary bloat, slower build times, and couples recipe updates to CLI releases. However, the CLI depends on certain "critical" recipes that enable core actions (language toolchains, build tools). These must remain embedded to ensure tsuku can bootstrap itself.

## Decision Drivers (Phase 1)

- Binary size reduction: 171 embedded recipes increases binary size unnecessarily
- Recipe agility: Non-critical recipes should be updateable without CLI releases
- Bootstrap reliability: CLI must always be able to install its own dependencies
- CI efficiency: Different testing rigor for critical vs non-critical recipes
- Backwards compatibility: Existing installations and workflows must continue working
- Supply chain security: Critical recipes need higher verification standards

## Research Findings (Phase 2)

- **Current state:** All 171 recipes embedded via `//go:embed` from `internal/recipe/recipes/`
- **Critical recipes:** ~15-20 recipes needed for action dependencies (go, rust, nodejs, python, ruby, perl, make, zig, cmake, ninja, meson, pkg-config, patchelf, zlib, openssl, libyaml)
- **Registry fallback exists:** Loader already supports fetching from GitHub raw URLs
- **Testing architecture:** Three-layer golden file validation with exclusion mechanisms
- **Directory decision:** Non-embedded recipes move to `recipes/` at repo root

## Options (Phase 3)

- **Categorization**: Location-based (directory determines criticality) vs computed vs explicit metadata
- **Directory**: Split directories (internal/ vs recipes/) vs single directory with build filter
- **Testing**: Plan-only for community vs hash-only vs full testing for all

## Decision (Phase 5)

**Problem:** All 171 recipes are embedded in the CLI binary, causing unnecessary bloat and coupling recipe updates to CLI releases.

**Decision:** Separate recipes into critical (embedded in `internal/recipe/recipes/`) and community (fetched from `recipes/` at repo root) based on directory location, with plan-only PR testing for community recipes and nightly full execution runs.

**Rationale:** Location-based categorization provides maximum simplicity with no metadata to maintain and no computed analysis to debug. Plan-only + nightly testing balances CI speed with regression detection within 24 hours.

## Current Status

**Phase:** 8 - Final Review (completed)
**Last Updated:** 2026-01-18
