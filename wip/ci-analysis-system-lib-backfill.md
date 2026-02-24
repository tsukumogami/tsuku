# CI Failure Analysis: docs/system-lib-backfill

Branch: `docs/system-lib-backfill` (rebased onto main, force-pushed 2026-02-24)
Commit: 482f27a2

## Summary

15 workflows triggered. 10 passed, 5 failed. No failures are caused by changes in this branch.

| Workflow | Result | Root Cause |
|----------|--------|------------|
| Check Artifacts | PASS | |
| Deploy Website | PASS | |
| Validate Closing Issues in Design Docs | PASS | |
| Validate Design Docs | PASS | |
| Validate Diagram Status for Closing Issues | PASS | |
| Validate Golden Files (Execution) | PASS | |
| Validate Golden Files (Recipes) | PASS | |
| Validate Recipe Structure (x2) | PASS | |
| **Validate Diagram Classes for Changed Docs** | **FAIL** | Issue #1191 marked done in diagram but still open on GitHub |
| **Validate Embedded Dependencies** (run 1) | **FAIL** | `python-build-standalone` release tag `dev` not found |
| **Validate Embedded Dependencies** (run 2) | **FAIL** | `bundler (gem_install)` failed (upstream flake) |
| **Platform Integration Tests** | **FAIL** | `ghcr.io` network timeout fetching libyaml manifest |
| **Test Recipe** | **FAIL** | macOS jobs cancelled (timeout with 1,331 recipes) |

## Failure Details

### 1. Validate Diagram Classes for Changed Docs

**Error**: `Node I1191 has class 'done' but issue #1191 is open` in `DESIGN-registry-scale-strategy.md`

**Root cause**: The Mermaid diagram in the parent design doc marks issue #1191 with the `done` CSS class, but #1191 is still open on GitHub. The diagram class validation (MM15) requires diagram node classes to match GitHub issue status.

**Fix**: Close issue #1191 on GitHub, or change the node class in the diagram to match the current issue state. This predates the current branch -- the issue was marked done in the design doc when the child design (DESIGN-system-lib-backfill.md) reached Current status.

**Scope**: Design doc housekeeping. Not related to recipe changes.

### 2. Validate Embedded Dependencies (both runs)

**Run 1 error**: `ruff (pipx_install)` failed — `release 'dev' not found in 'indygreg/python-build-standalone'`

The embedded `python-standalone` recipe references a GitHub release tagged `dev` in the `indygreg/python-build-standalone` repo. This tag no longer exists or was renamed upstream. The `pipx_install` action depends on `python-standalone` to provide a Python runtime.

**Run 2 error**: `bundler (gem_install)` failed (different flake)

The `gem_install` action's embedded Ruby dependency hit an upstream issue.

**Root cause**: Upstream release tag changes in `indygreg/python-build-standalone`. The embedded recipe at `internal/recipe/recipes/python-standalone.toml` needs its version provider or release tag updated.

**Fix**: Update the embedded `python-standalone` recipe to point to the current release tag. This is a pre-existing issue not introduced by this branch.

**Scope**: Embedded recipe maintenance. Affects all PRs, not specific to this branch.

### 3. Platform Integration Tests (rhel)

**Error**: `context deadline exceeded` fetching `ghcr.io/v2/homebrew/core/libyaml/manifests/0.2.5`

**Root cause**: Transient network timeout on the GitHub Actions runner when fetching the Homebrew bottle manifest from GHCR. The rhel (glibc, amd64) job failed; all other jobs (debian amd64/arm64, dltest builds) passed.

**Fix**: Re-run the failed job. This is a transient infrastructure issue.

**Scope**: GitHub Actions infrastructure flake. Not related to branch changes.

### 4. Test Recipe (macOS cancelled)

**Error**: Both `test-darwin-arm64` and `test-darwin-x86_64` jobs were cancelled (timeout).

**Root cause**: This PR changes 1,331 recipe files vs main. The macOS jobs attempt to test all changed recipes sequentially. With this volume, the jobs exceed the GHA job timeout limit. Linux batch jobs were never created (the workflow splits Linux testing into batches but macOS runs serially).

Additionally, no Linux batch jobs appeared in the run. The batch splitting logic may have hit a limit with 1,331 recipes, or the `has_linux_batches` output wasn't set to `true`. This needs investigation in the workflow's batch step.

**Fix**: This is a workflow scaling issue. With 1,331 changed recipes, the Test Recipe workflow isn't designed to handle this volume in a single PR. For normal PRs with a few recipe changes, it works fine. Options:
- Accept that this workflow will timeout on bulk recipe PRs
- Add batching to macOS jobs (similar to Linux)
- Cap the number of recipes tested per run
- Skip Test Recipe for PRs that change more than N recipes (they've already been tested individually)

**Scope**: Workflow design limitation for bulk PRs. Not related to branch changes.

## Pre-existing Issues (not introduced by this branch)

1. **Issue #1191 still open**: Needs to be closed on GitHub to match the design doc status.
2. **python-build-standalone release tag changed**: Embedded recipe needs update.
3. **ghcr.io transient timeouts**: Persistent infrastructure flakiness in GitHub Actions.
4. **Test Recipe workflow doesn't scale to 1000+ recipe PRs**: Needs batching for macOS jobs.

## Actionable Items

| Item | Priority | Type |
|------|----------|------|
| Close #1191 on GitHub | High | Housekeeping (fixes CI for this PR) |
| Update python-standalone embedded recipe | Medium | Bug fix (affects all PRs) |
| Re-run Platform Integration Tests (rhel) | Low | Transient (will likely pass) |
| Investigate missing Linux batch jobs in Test Recipe | Medium | Workflow bug |
| Add macOS batching to test-recipe.yml | Low | Enhancement (only affects bulk PRs) |
