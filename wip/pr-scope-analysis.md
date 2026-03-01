# PR Scope Analysis: System-Lib Backfill

Current PR #1863 has 1,310 new + 21 modified recipes. Most were never
validated against darwin/arm targets via the test-recipe workflow. The
PR should be split into a focused PR (blocker relationships + validation
fixes) and a separate bulk queue consumption PR.

## Focused PR: Blocker Relationships (~507 recipes)

Recipes that participate in blocker->blocked dependency chains or
required fix-script intervention to pass local validation.

| Category | Count | Description |
|----------|-------|-------------|
| Library blockers (new) | 10 | libgit2, bdw-gc, ada-url, dav1d, oniguruma, glib, tree-sitter, libevent, libidn2, notmuch |
| Satisfies backfill (modified) | 21 | Existing recipes that got `[metadata.satisfies]` entries |
| Homebrew with valid deps | 403 | Tools whose `runtime_dependencies` reference library recipes we provide |
| Homebrew with stripped deps | 73 | Recipes where fix-script removed all deps (originally had invalid/non-existent refs) |

### Notes on "stripped deps" category

These 73 recipes originally had `runtime_dependencies` populated by the
generator, but every dependency was either non-existent in the registry
or had an invalid name (`gtk+3`, `sdl2_image`, etc.). The fix script
set them to `runtime_dependencies = []`. They're effectively standalone
now, but the fact they needed manual intervention makes them relevant
to the design doc scope.

## Bulk PR: Queue Consumption (~824 recipes)

Independent recipes with no dependency relationships. Generated cleanly
with no fixes needed.

| Category | Count | Description |
|----------|-------|-------------|
| Homebrew standalone | 639 | No `runtime_dependencies` field -- independent tools |
| crates.io | 102 | Rust CLI tools via `cargo_install`, all succeeded |
| RubyGems | 83 | Ruby CLI tools via `gem_install`, all succeeded |

## Open Issue

Neither set has been validated through the `test-recipe.yml` workflow
against darwin/arm64/linux targets. The design doc requires this:
platform failures should result in `when` filters in the recipe, not
blocked merges. This validation needs to happen before merge.

## Ecosystem Totals (both PRs combined)

| Ecosystem | Focused PR | Bulk PR | Total |
|-----------|-----------|---------|-------|
| Homebrew (new) | 486 | 639 | 1,125 |
| Homebrew (modified) | 21 | 0 | 21 |
| crates.io | 0 | 102 | 102 |
| RubyGems | 0 | 83 | 83 |
| **Total** | **507** | **824** | **1,331** |
