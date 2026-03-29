# Implementation Plan: Adopt Shirabe Reusable Release Workflows

## Files to Create

1. `.github/workflows/prepare-release.yml` — caller workflow for shirabe's release.yml
2. `.release/set-version.sh` — hook script to stamp Cargo.toml version fields
3. `.github/workflows/finalize.yml` — bridge triggered after existing Release workflow

## Files Unchanged

- `.github/workflows/release.yml` — existing tag-triggered build stays as-is

## Steps

### 1. Create `.release/set-version.sh`

Hook script that receives a version argument (no `v` prefix). Updates:
- `cmd/tsuku-dltest/Cargo.toml` version field
- `tsuku-llm/Cargo.toml` version field

Must handle both release versions (`0.6.2`) and dev versions (`0.6.3-dev`).
Use `sed` with portable patterns. Make executable.

### 2. Create `.github/workflows/prepare-release.yml`

Workflow dispatch with inputs: version, tag, ref (default: main).
Calls `tsukumogami/shirabe/.github/workflows/release.yml@v0.2.0`.
Passes `RELEASE_PAT` secret.

### 3. Create `.github/workflows/finalize.yml`

Triggered by `workflow_run` on the "Release" workflow (completed).
Extracts tag from `workflow_run.head_branch`.
Calls `tsukumogami/shirabe/.github/workflows/finalize-release.yml@v0.2.0`.
Passes `expected-assets: 16` and `RELEASE_PAT` secret.

### 4. Verify

- Confirm existing release.yml is untouched (git diff)
- Validate YAML syntax
- Check set-version.sh handles both version formats
