# Test Recipes

This directory contains non-production recipes used for testing and demonstration purposes.

## Purpose

These recipes exist to exercise and verify specific tsuku features that may not be well-represented in the production registry. They are **not intended for end-user consumption** and may duplicate tools that already exist in the production registry (`internal/recipe/recipes/`).

## Why Separate from Production?

Production recipes prioritize reliability, simplicity, and user experience. Test recipes prioritize feature coverage and edge case testing. These goals sometimes conflict:

| Aspect | Production Recipes | Test Recipes |
|--------|-------------------|--------------|
| Goal | Reliable tool delivery | Feature verification |
| Installation method | Simplest reliable approach | Demonstrates specific capability |
| Maintenance | Actively maintained | Updated as needed for tests |
| User-facing | Yes | No |

## Common Patterns

### Source Recipes (`*-source.toml`)

Recipes like `python-source.toml`, `git-source.toml`, and `sqlite-source.toml` demonstrate tsuku's build-from-source capabilities using well-known Homebrew build instructions. These allow verification of:

- Build essentials integration
- Dependency resolution for source builds
- Configure/make/install workflows
- Platform-specific build flags

In production, these tools use pre-built bottles or binaries because:
- Faster installation (no compilation)
- More reliable (no build tool dependencies)
- Consistent across environments

### System Recipes (`*-system.toml`)

Recipes like `build-tools-system.toml` and `ssl-libs-system.toml` demonstrate integration with system package managers. These verify:

- Platform detection (apt, dnf, pacman, etc.)
- Package manager invocation
- System library discovery

### Feature Test Recipes

Some recipes exist solely to test specific features:

- `iterm2-test.toml` - Tests the cask version provider and app_bundle action
- `tool-a.toml`, `tool-b.toml` - Minimal recipes for dependency testing

## Adding Test Recipes

When adding a test recipe:

1. Use a descriptive name indicating its purpose (e.g., `foo-source.toml`, `bar-test.toml`)
2. Add comments explaining what feature the recipe tests
3. Consider whether the feature could instead be tested via the production recipe
4. Update relevant test matrix entries if the recipe is part of CI

## Feature Coverage Test Recipes

These recipes test specific package manager action implementations and are used in `test-matrix.json` for CI feature coverage tests:

| Recipe | Action | Purpose |
|--------|--------|---------|
| `netlify-cli.toml` | npm_install | Tests npm package installation with multiple executables |
| `ruff.toml` | pipx_install | Tests pipx package installation |
| `cargo-audit.toml` | cargo_install | Tests cargo crate installation |
| `bundler.toml` | gem_install | Tests gem installation with multiple executables |
| `ack.toml` | cpan_install | Tests CPAN module installation |
| `gofumpt.toml` | go_install | Tests Go module installation |

These recipes mirror their production counterparts (in `recipes/`) but exist separately to:
1. Ensure CI tests use stable, embedded recipes rather than registry recipes that may move to external storage
2. Allow test-specific customization if needed
3. Provide clear documentation of what action each recipe tests

Each test recipe includes a header comment explaining the action being tested and a reference to the related production recipe.

## Relationship to Production Registry

The production registry at `recipes/` contains the canonical recipes for end users. Embedded recipes at `internal/recipe/recipes/` are action dependencies required for the CLI to function. If a tool exists in both locations:

- **Production**: Optimized for reliability and user experience
- **Testdata**: Optimized for testing specific tsuku capabilities

Always prefer improving production recipes over creating test duplicates when possible.
