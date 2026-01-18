# Test Matrix Usage Research

## Summary

`test-matrix.json` defines an **integration test suite** that's separate from golden file validation. It ensures specific tsuku features (actions, version providers) work by testing real tool installations.

## Test Matrix Structure

```json
{
  "tiers": {
    "tier1": [...],    // 4 simple GitHub archive tests
    "tier2": [...],    // 5 download archive tests
    "tier3": [...],    // 1 Nix test
    "tier4": [...],    // 2 specialized tests (tap, cask)
    "tier5": [...]     // 10 package manager tests
  },
  "ci": {
    "linux": [...],    // 11 tests on every PR
    "macos": [...],    // 5 tests on every PR
    "scheduled": [...]  // 5 additional tests nightly only
  }
}
```

## Workflows Using test-matrix.json

| Workflow | Trigger | Tests Run |
|----------|---------|-----------|
| `test.yml` | PR/push to main | ci.linux + ci.macos |
| `scheduled-tests.yml` | Nightly 2 AM UTC | ci.linux + ci.macos + ci.scheduled |
| `sandbox-tests.yml` | PR/push to main | Plan generation validation |

## Relationship to Golden File Validation

**Completely separate systems:**

| System | Purpose | Command |
|--------|---------|---------|
| test-matrix (integration) | Real tool installation | `tsuku install <tool>` |
| Golden files (plan validation) | Deterministic plan checking | `tsuku install --plan <file.json>` |

## Feature Coverage by Tier

**Tier 1-2 (toolchains):** `github_archive`, `github_file`, `download_archive`
- Recipes: actionlint, btop, argo-cd, bombardier, golang, zig, rust, nodejs, perl

**Tier 3 (nix):** `nix_install`
- Recipes: hello-nix

**Tier 4 (macOS):** `tap`, `cask`
- Recipes: waypoint-tap (testdata), iterm2

**Tier 5 (package managers):** `npm_install`, `pipx_install`, `cargo_install`, `gem_install`, `cpan_install`, `go_install`
- Recipes: netlify-cli, serve, ruff, black, cargo-audit, cargo-watch, bundler, jekyll, fpm, ack, gofumpt

## Problem After Separation

Tier 5 tests are the issue:
- They test **action implementations** (npm_install.go, cargo_install.go, etc.)
- The recipes used (netlify-cli, cargo-audit, bundler) are NOT action dependencies
- If these recipes become community recipes:
  - They're not embedded in the binary
  - But they need to be available for integration tests
  - Code changes to actions wouldn't trigger community recipe testing

**Example failure scenario:**
1. Developer changes `internal/actions/npm_install.go`
2. Integration test uses `netlify-cli` recipe to test npm_install action
3. If `netlify-cli` is a community recipe, it needs to be fetched
4. If fetching fails or is slow, integration tests break

## Test Matrix Trigger Path

Unlike golden file tests, `test.yml` doesn't use fine-grained path triggers for recipe locations. It runs on:
- Any push to main
- Any PR to main
- Skipped only for documentation-only changes

This means integration tests run regardless of what changed, but they need the recipes to be available.
