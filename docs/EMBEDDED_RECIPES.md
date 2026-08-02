# Embedded Recipes

This document lists recipes that must remain in `internal/recipe/recipes/` because they are dependencies of tsuku's actions. These recipes are embedded in the tsuku binary and available without network access.

## Toolchain Recipes

Language runtimes needed by package installation actions.

| Recipe | Required By | Rationale |
|--------|-------------|-----------|
| go | go_install, go_build | Go toolchain for building Go packages |
| rust | cargo_install, cargo_build | Rust toolchain for building crates |
| nodejs | npm_install, npm_exec | Node.js runtime for npm packages |
| python-standalone | pipx_install | Self-contained Python for pipx packages |
| ruby | gem_install, gem_exec | Ruby runtime for gem packages |
| perl | cpan_install | Perl runtime for CPAN modules |

Note: `pip_install` depends on `python` which is not currently embedded. This gap will be caught by `--require-embedded` validation and tracked in the exclusions file.

## Build Tool Recipes

Build systems and compilers needed by source-build actions.

| Recipe | Required By | Rationale |
|--------|-------------|-----------|
| make | configure_make, cmake_build | GNU Make for building from source |
| cmake | cmake_build | CMake build system |
| meson | meson_build | Meson build system |
| ninja | meson_build | Ninja build tool (backend for Meson) |
| zig | configure_make, cmake_build, meson_build | Zig CC for cross-compilation |
| pkg-config | configure_make, cmake_build | Package configuration tool |
| patchelf | homebrew, homebrew_relocate, meson_build (Linux) | ELF binary patching for relocation |

## Library Recipes

Transitive dependencies of embedded toolchains and build tools.

| Recipe | Required By | Rationale |
|--------|-------------|-----------|
| gcc-libs | nodejs | libstdc++ and libgcc_s runtime libraries (nodejs dependency) |
| libyaml | ruby | YAML parsing library (ruby dependency) |
| openssl | cmake | TLS/crypto library (cmake dependency) |
| zlib | openssl | Compression library (openssl dependency) |

## Notes

### Validation

This list is validated by CI using the `--require-embedded` flag:

```bash
tsuku eval <recipe> --require-embedded
```

When this flag is set, action dependencies must resolve from the embedded registry. If a dependency is missing, evaluation fails with a clear error message.

### Exclusions

During migration, known gaps are tracked in `embedded-validation-exclusions.json`. Recipes in this file are excluded from CI validation until their dependencies are properly embedded.

### Multi-Source Recipes and Golden Files

An embedded recipe can declare `fallback_urls` alongside its `url` so a single
host outage does not fail plan generation for every recipe that depends on it.
`zig.toml` is the live example, and its comment block records why.

The generated plan carries `fallback_urls` only when the recipe declared it, so
a single-source recipe's golden is byte-identical to what it was before the
parameter existed. Two consequences when regenerating:

- A golden that grows a `fallback_urls` array grew it because its recipe (or a
  recipe it depends on) gained a source list. If one appears in a golden whose
  recipe you did not touch, stop and find out why.
- zig is an implicit dependency of `cmake_build`, `configure_make` and
  `meson_build`, so its download step is embedded in other recipes' goldens.
  `grep -rl zig- testdata/golden/plans/embedded/` finds the current set. ninja
  is on the code-validation exclusion list, so its goldens take a URL-only edit
  rather than a full regeneration, which would sweep in unrelated format drift.

See [GUIDE-actions-and-primitives.md](guides/GUIDE-actions-and-primitives.md)
for the recipe-authoring side.

### Adding a New Embedded Recipe

To add a recipe to the embedded list:

1. Add the recipe file to `internal/recipe/recipes/<name>.toml`
2. Run `go generate ./...` to rebuild the embedded filesystem
3. Update this document to include the recipe with its rationale
4. Ensure all transitive dependencies are also embedded

### Design Reference

See [DESIGN-embedded-recipe-list.md](designs/DESIGN-embedded-recipe-list.md) for the complete design rationale and validation approach.
