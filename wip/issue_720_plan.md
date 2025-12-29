# Issue 720 Implementation Plan

## Overview

Create a workflow_dispatch workflow at `.github/workflows/generate-golden-files.yml` that generates golden files for specified recipes across all supported platforms (ubuntu-latest, macos-latest, macos-13).

## Workflow Structure

### Trigger and Inputs

```yaml
on:
  workflow_dispatch:
    inputs:
      recipe:
        description: 'Recipe name to regenerate (required)'
        required: true
        type: string
      commit_back:
        description: 'Commit results back to current branch'
        type: boolean
        default: false
      branch:
        description: 'Branch to run on (defaults to current ref)'
        required: false
        type: string
```

### Jobs

1. **generate** (matrix job)
   - Matrix: ubuntu-latest (linux-amd64), macos-latest (darwin-arm64), macos-13 (darwin-amd64)
   - Each runner generates golden files for its platform only using `regenerate-golden.sh`
   - Uploads artifacts per-platform for download

2. **commit** (conditional, runs only if `commit_back` is true)
   - Depends on `generate` job completion
   - Downloads all platform artifacts
   - Merges golden files from all platforms
   - Commits and pushes to the branch with bot attribution

## Key Implementation Details

### Platform-to-Runner Mapping

| Runner | Platform | OS | Arch |
|--------|----------|----|----- |
| ubuntu-latest | linux-amd64 | linux | amd64 |
| macos-latest | darwin-arm64 | darwin | arm64 |
| macos-13 | darwin-amd64 | darwin | amd64 |

### Script Invocation

Each platform runner calls:
```bash
./scripts/regenerate-golden.sh "$RECIPE" --os "$OS" --arch "$ARCH"
```

### Artifact Handling

- Each generate job uploads `testdata/golden/plans/` with platform-specific name
- Commit job downloads all artifacts and merges into single directory
- Artifacts remain available for download even if `commit_back` is false

### Git Commit (when commit_back is true)

```yaml
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add testdata/golden/plans/
git commit -m "chore(golden): regenerate ${{ inputs.recipe }} golden files"
git push
```

### Permissions

```yaml
permissions:
  contents: write  # Required for pushing commits
```

## Implementation Steps

1. Create `.github/workflows/generate-golden-files.yml` with:
   - workflow_dispatch trigger with recipe, commit_back, and branch inputs
   - Platform matrix (3 runners)
   - Checkout with branch input support
   - Go setup and tsuku build
   - Platform-specific golden file generation
   - Artifact upload per platform
   - Conditional commit job that merges artifacts and pushes

## Acceptance Criteria Mapping

| Criteria | Implementation |
|----------|----------------|
| workflow_dispatch with recipe input | `inputs.recipe` (required) |
| Input for commit results back | `inputs.commit_back` (boolean, default false) |
| Matrix: ubuntu-latest, macos-latest, macos-13 | matrix.include with os and platform |
| Each runner generates for its platform only | `--os` and `--arch` flags to script |
| Artifacts uploaded if not committing | Always upload, artifacts available regardless |
| Optional commit back with bot attribution | Conditional commit job with github-actions[bot] |
| Works from PR branches | `inputs.branch` or `github.ref` for checkout |
