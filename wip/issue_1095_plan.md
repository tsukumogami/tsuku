# Issue 1095 Implementation Plan

## Overview

Create `publish-golden-to-r2.yml` workflow that generates golden files on merge and uploads to R2.

## Approach

Follow the patterns established by:
- `generate-golden-files.yml`: Cross-platform matrix, artifact collection, recipe detection
- `validate-golden-recipes.yml`: Changed recipe detection via git diff
- `r2-upload.sh`: Upload with metadata and verification

## Files to Create

1. `.github/workflows/publish-golden-to-r2.yml` - Main workflow

## Workflow Structure

```yaml
name: Publish Golden Files to R2

on:
  push:
    branches: [main]
    paths:
      - 'recipes/**/*.toml'
      - 'internal/recipe/recipes/**/*.toml'
  workflow_dispatch:
    inputs:
      recipes:
        description: 'Comma-separated list of recipe names'
        required: true
        type: string
      force:
        description: 'Force regenerate even if version exists'
        type: boolean
        default: false

jobs:
  detect-recipes:
    # For push trigger: detect changed recipes via git diff
    # For workflow_dispatch: parse input

  generate:
    # Matrix: ubuntu-latest, macos-14, macos-15-intel
    # For each platform:
    #   1. Build tsuku
    #   2. Generate golden files for detected recipes
    #   3. Upload artifacts

  upload-to-r2:
    needs: [detect-recipes, generate]
    # Protected environment: registry-write
    # 1. Download all artifacts
    # 2. For each golden file:
    #    - Extract version from filename
    #    - Call r2-upload.sh
    # 3. Update manifest (deferred to future issue)
```

## Implementation Steps

### Step 1: Create workflow file with triggers

- Push trigger on `recipes/**/*.toml` and `internal/recipe/recipes/**/*.toml`
- workflow_dispatch with `recipes` (string) and `force` (boolean) inputs
- Permissions: contents read (for checkout), no write needed

### Step 2: Add detect-recipes job

- For push: use git diff to find changed TOML files
- For workflow_dispatch: parse comma-separated input
- Output: JSON array of `{recipe, category}` objects
- Handle both embedded and registry categories

### Step 3: Add generate job (matrix)

- Matrix strategy following generate-golden-files.yml pattern
- Use `regenerate-golden.sh` to generate files
- Upload artifacts per platform

### Step 4: Add upload-to-r2 job

- Environment: `registry-write` (protected)
- Download artifacts from all platforms
- Parse golden files to extract recipe/version/platform
- Call `r2-upload.sh` for each file
- Handle failures gracefully (continue-on-error for individual uploads)

### Step 5: Add concurrency control

- Concurrency group to prevent overlapping runs
- Cancel-in-progress for workflow_dispatch (not for push)

## Security Notes

- Use pinned action versions (SHA)
- Protected environment for write credentials
- No shell interpolation of file content

## Testing Strategy

1. Syntax validation: workflow_dispatch dry-run
2. Manual test with single recipe
3. Verify R2 upload via r2-download.sh

## Manifest Update

Deferred - the issue mentions manifest handling but this adds complexity. The r2-upload.sh script already stores metadata per-object. A separate issue can add manifest aggregation.
