# Platform Validation Gaps

Gaps identified during batch recipe generation design review. These need to be addressed in DESIGN-registry-scale-strategy.md before planning issues.

## Context

The batch pipeline generates platform-independent recipes, validates them progressively across platforms, and opens PRs with passing recipes. When a recipe fails on some platforms, it should merge with a `platforms` metadata field excluding failing targets. PR CI should respect this field so the PR passes.

The high-level flow is covered across the two designs, but the following gaps prevent it from working end-to-end.

## TODO

- [x] **Recipe file mutation mechanism**: The merge job is the single aggregation point — platform jobs produce result artifacts but never modify recipes. After all platform jobs complete, the merge job derives constraints from the set of passing platforms (not individual failures) and writes them using existing metadata fields (`supported_os`, `supported_arch`, `supported_libc`, `unsupported_platforms`). No new schema field needed. No fields written when all platforms pass. Decision recorded in DESIGN-registry-scale-strategy.md, Per-Environment Validation Results section.

- [x] **PR CI filtering by platform constraints**: `test-changed-recipes.yml` uses `tsuku info --json --recipe <path> --metadata-only` to read the computed `supported_platforms` during matrix setup. Recipes are only included in a platform's job list if `supported_platforms` includes that platform. No schema changes needed — the CLI already computes this from existing constraint fields. Decision recorded in DESIGN-registry-scale-strategy.md, Per-Environment Validation Results section.

- [x] **Auto-merge gate thresholds**: Merge if at least 1 platform passes. Partial coverage is acceptable — a constrained recipe is more useful than no recipe. Decision recorded in DESIGN-registry-scale-strategy.md, Per-Environment Validation Results section.

- [x] **Re-queue and retry for platform failures**: Platform validation jobs retry up to 3 times with exponential backoff, but only for exit code 5 (`ExitNetwork` — timeouts, rate limits, DNS, connection errors). All other failures are structural and fail immediately. No separate retry workflow needed. Decision recorded in DESIGN-registry-scale-strategy.md, Per-Environment Validation Results section.

- [x] **SLI reporting per environment**: `batch-runs.jsonl` extended with a `platforms` object containing per-environment tested/passed/failed counts. Merge job writes a markdown summary table to `$GITHUB_STEP_SUMMARY` for immediate visibility in GitHub Actions UI. Reporting script deferred until enough data accumulates. Decision recorded in DESIGN-batch-recipe-generation.md, SLI Collection section.

## Source

Identified during review of:
- `docs/designs/DESIGN-batch-recipe-generation.md` (PR #1240)
- `docs/designs/DESIGN-registry-scale-strategy.md`
- `.github/workflows/test-changed-recipes.yml`
