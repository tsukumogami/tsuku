---
summary:
  constraints:
    - Must use workflow_dispatch with recipe name input
    - Must support pull_request trigger detecting changed recipes
    - Pin all actions to commit SHAs (repo convention)
    - Must NOT skip library recipes (unlike test-changed-recipes.yml)
    - Platform failures use continue-on-error, not PR blocking
    - Must be merged to main before library recipe PRs (blocking prerequisite)
  integration_points:
    - batch-generate.yml for Docker container patterns, cross-compilation, image set
    - validate-golden-execution.yml for container family mapping
    - test-changed-recipes.yml for recipe path detection from PR diff
    - tsuku install --recipe <path> --force as the install command
  risks:
    - arm64 runner availability (ubuntu-24.04-arm)
    - macOS runner minute costs
    - Recipe path resolution edge cases (names with special chars)
  approach_notes: |
    Create .github/workflows/test-recipe.yml following patterns from
    batch-generate.yml. Build job cross-compiles tsuku, uploads artifacts.
    Platform jobs download binary and run tsuku install in containers (Linux)
    or natively (macOS). Job summary reports per-platform results table.
---

# Implementation Context: Issue #1864

Source: docs/designs/DESIGN-system-lib-backfill.md (Decision 7)
