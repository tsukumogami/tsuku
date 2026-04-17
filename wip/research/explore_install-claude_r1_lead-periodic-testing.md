# Lead: Periodic testing model

## Findings

### What exists today

There are three distinct testing layers in the repository, each with different triggers:

**1. PR-gated recipe testing (`test-recipe.yml`)**
- Triggers on `pull_request` when any `recipes/**/*.toml` or `internal/recipe/recipes/**/*.toml` changes.
- Cross-compiles tsuku for linux/amd64, linux/arm64, darwin/arm64, darwin/amd64 and runs the changed recipes.
- Linux tests run inside sandbox containers across five OS families (debian, rhel, arch, suse, alpine).
- macOS tests run on native GitHub-hosted runners.
- Tests only changed recipes -- untouched recipes get no exercise.
- Also supports `workflow_dispatch` to test a single recipe by name on demand.

**2. Nightly golden-file validation (`nightly-registry-validation.yml`)**
- Runs on `cron: '0 2 * * *'` (daily at 2 AM UTC).
- Downloads all recipe plan golden files from Cloudflare R2, regenerates plans from the current recipe TOML, and diffs them.
- This is a *plan-generation* check, not an *installation* check. It detects recipe TOML drift (e.g., URL template changes, version provider failures) but does not actually execute installs.
- An "execute sample" job runs one recipe per letter directory (roughly 26 recipes alphabetically sampled) as actual installs via `tsuku install --plan`.
- Failures create GitHub issues automatically with labels `nightly-failure,recipes`.
- Degraded R2 availability creates a separate issue labeled `r2-unavailable`.

**3. Scheduled integration tests (`scheduled-tests.yml`)**
- Runs on `cron: '0 2 * * *'` (same daily schedule as nightly validation).
- Reads `test-matrix.json` and runs the union of `ci.linux`, `ci.macos`, and `ci.scheduled` test IDs.
- These are tool-specific functional tests (e.g., install `rust`, then presumably verify it works), not broad registry sweeps.
- The `ci.scheduled` set adds heavier tests excluded from PR CI: `archive_rust_custom_source`, `cargo_cargo-audit_basic`, `cpan_ack_with_dependency`, `archive_go_toolchain`, `go_gofumpt_with_dependency`.

**4. PR-gated sandbox tests (`sandbox-tests.yml`)**
- Triggers on `push`/`pull_request` to Go source files and embedded TOML recipes.
- Runs the `ci.linux` matrix from `test-matrix.json` using `tsuku install --plan ... --sandbox`.
- This tests the sandbox execution path, not the full cross-platform matrix.

**5. Platform integration tests (`platform-integration.yml`)**
- Triggers on push to `main` and on PRs changing `recipes/**`.
- Tests a fixed small set: zlib, libyaml (library installs + dlopen), and `just` (tool install).
- Covers debian, rhel, debian-arm64 only (arch/suse/alpine absent from this matrix; alpine disabled per issue #1570).

**6. Weekly discovery freshness (`discovery-freshness.yml`)**
- Validates that entries in `recipes/discovery/` still point to valid sources.
- This is metadata freshness, not recipe execution.

### No curated-recipe concept exists yet

There is no notion of a "curated" or "featured" recipe in the TOML schema, file structure, or workflow. All 1405 recipes in `recipes/` are treated identically by CI: tested only when changed. No recipe carries a tag that would route it into a dedicated periodic test track.

### What "periodically tested" currently means

- **Plan generation**: Nightly, for all registry recipes (via golden files from R2).
- **Actual installation**: Nightly, for ~26 recipes sampled alphabetically (one per letter), not curated by importance.
- **Specific tool installs**: Daily for the `ci.scheduled` matrix (~5 tools), weekly for nothing recipe-specific beyond the sample.

### The coverage gap

A recipe for a major tool like `kubectl`, `terraform`, or `gh` could have been authored and merged. If it passes the PR run and then neither its TOML nor the Go source changes, it receives no further installation test. The nightly golden-file diff will catch version-provider failures (broken URL templates) but not runtime installation failures on newer tool releases (e.g., changed archive structure, binary rename, moved download URL domain).

The nightly execute-sample covers one recipe per alphabetical letter, chosen arbitrarily (first non-excluded recipe alphabetically in each letter bucket). This is not curated and could easily miss high-value tools entirely.

## Implications

1. **Silent rot is possible.** A curated recipe for `terraform` could break on new Terraform release because HashiCorp changed download URL structure. The nightly plan diff would catch a version-provider failure but would not catch a download or extraction failure unless `terraform` happened to be the alphabetically first recipe in `t/`.

2. **The golden-file path is the right anchor.** The nightly `validate-plans` step already runs all recipes daily. Extending it to include actual installs for a curated subset is a natural upgrade rather than a new pipeline.

3. **`workflow_dispatch` provides a manual escape valve.** Maintainers can trigger `test-recipe.yml` for any recipe by name, but this requires manual action.

4. **Scheduled-tests.yml uses a hand-curated matrix (`test-matrix.json`).** This file already represents an opt-in model -- recipes are added by name and assigned features. A `ci.curated` array in this file would be a low-friction way to declare which recipes get periodic full-install testing.

5. **Telemetry exists but is not wired to recipe health.** The `internal/telemetry/` package sends usage events. Install failure rates could signal broken recipes without CI, but there is currently no pipeline that reads telemetry to gate or flag recipes.

## Surprises

- The nightly execution sample is alphabetically determined, not importance-weighted. This means the set of periodically-executed recipes is effectively random relative to user impact.
- The `platform-integration.yml` workflow is the closest thing to a curated test set (it tests `zlib`, `libyaml`, `just` specifically), but it only covers three recipes and runs on PR plus push-to-main, not on a schedule.
- There are 1405 recipes but the `test-matrix.json` has roughly 20 named tests. The gap between what is named and what exists in the registry is very large.
- The nightly validation has a two-tier degradation model for R2 unavailability, which is operationally mature, but the execution layer (sample installs) has no comparable fallback.
- The `ci.blocked` section in `test-matrix.json` shows that at least two recipe tests are suppressed due to known bugs, suggesting some recipes are already in a degraded state that periodic testing would surface.

## Open Questions

1. Should "curated" be a label in the recipe TOML schema (e.g., `featured = true`) or a separate manifest file (e.g., `recipes/curated.json`), or simply an array in `test-matrix.json`?

2. What is the right cadence for curated recipe install tests: daily (same as nightly validation), weekly, or triggered by upstream releases (e.g., watch the version provider)?

3. Should curated recipe tests run actual installs on the full platform matrix (all five Linux families plus macOS) or a representative subset (debian + macOS)?

4. How many recipes qualify as "curated"? If the answer is 20-50, a daily full-install matrix is feasible. If it is 200+, sampling or tiering within the curated set is needed.

5. Should failures in a curated recipe's periodic test gate the recipe from being served by the registry (i.e., mark it as broken in a status file), or only create an issue?

6. Is telemetry data accessible in a way that could inform which recipes have real user installs, and could that feed into the curated selection?

## Summary (3 sentences)

The current testing model gates only on recipe changes at PR time, with a nightly layer that validates plan generation for all recipes and runs actual installs for a random alphabetical sample (~26 recipes), not an importance-weighted set. No "curated" concept exists in the schema or CI -- there is no way to declare that a recipe should receive periodic full-install testing independent of code changes. The most natural implementation path adds a `ci.curated` array to `test-matrix.json` (mirroring the existing `ci.scheduled` pattern) and extends the nightly workflow to run actual installs for that named set across a representative platform matrix, with automatic issue creation on failure.
