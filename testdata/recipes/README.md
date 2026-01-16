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

## Relationship to Production Registry

The production registry at `internal/recipe/recipes/` contains the canonical recipes for end users. If a tool exists in both locations:

- **Production**: Optimized for reliability and user experience
- **Testdata**: Optimized for testing specific tsuku capabilities

Always prefer improving production recipes over creating test duplicates when possible.
