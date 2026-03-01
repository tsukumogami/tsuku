# Friction Log: Library Recipe Creation

This log tracks library recipes that failed deterministic generation via
`tsuku create --from homebrew --deterministic-only` and required manual
construction. Each entry records the failure, the manual fix applied, and
the pipeline enhancement that would prevent the issue in future runs.

## bdw-gc

**Command:** `tsuku create bdw-gc --from homebrew --deterministic-only --skip-sandbox --yes`

**Error:** `deterministic generation failed: [complex_archive] formula bdw-gc bottle contains no binaries in bin/`

**Root cause:** The deterministic generator requires at least one binary in `bin/` to produce a recipe. Pure library packages that only ship shared objects, static archives, and headers have no binaries and are classified as `complex_archive`.

**Manual fix:** Constructed the recipe manually following the gmp.toml library pattern with `type = "library"`, platform-specific homebrew/apk_install steps, and `install_binaries` with `install_mode = "directory"` listing library outputs.

**Pipeline enhancement:** The deterministic generator should recognize library-only bottles (no binaries, but `.so`/`.a`/`.dylib` files in `lib/`) and generate library recipes with `type = "library"` and `install_mode = "directory"` automatically.

## tree-sitter

**Command:** `tsuku create tree-sitter --from homebrew --deterministic-only --skip-sandbox --yes`

**Error:** `deterministic generation failed: [complex_archive] formula tree-sitter bottle contains no binaries in bin/`

**Root cause:** Same as bdw-gc. tree-sitter is a parser library that ships `.so`/`.dylib`/`.a` files and headers but no standalone CLI binary. (Note: the separate `tree-sitter-cli` formula provides the CLI tool and already has a recipe at `recipes/t/tree-sitter-cli.toml`.)

**Manual fix:** Constructed the recipe manually with library outputs. Added `satisfies = { homebrew = ["tree-sitter", "tree-sitter@0.25"] }` to cover the versioned Homebrew formula alias.

**Pipeline enhancement:** Same as bdw-gc -- detect library-only bottles and generate `type = "library"` recipes.
