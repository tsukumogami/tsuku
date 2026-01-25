---
summary:
  constraints:
    - Must use the existing publish-golden-to-r2.yml workflow (from #1095)
    - R2 object key convention: plans/{letter}/{recipe}/v{version}/{platform}.json
    - Need verification to confirm all files uploaded with correct checksums
    - Must not disrupt current CI workflows during migration
  integration_points:
    - publish-golden-to-r2.yml workflow with manual dispatch support
    - scripts/r2-upload.sh for individual file uploads
    - scripts/r2-download.sh for verification
    - testdata/golden/plans/ for registry golden files source
  risks:
    - Large batch upload may hit rate limits or timeout
    - Partial failures could leave R2 in inconsistent state
    - Current golden files structure differs from R2 (version in filename vs directory)
  approach_notes: |
    This is primarily an operational task, not a code change. The publish-golden-to-r2.yml
    workflow supports manual dispatch with a `recipes` input. The migration can be done by:
    1. Identifying all recipes with golden files
    2. Triggering the publish workflow for batches of recipes
    3. Verifying uploads completed successfully
    4. Capturing audit logs

    Key insight: The validation script in the issue assumes a manifest.json at the root,
    but we need to verify if that's implemented in the publish workflow or if we need
    a different verification approach.
---

# Implementation Context: Issue #1097

**Source**: docs/designs/DESIGN-r2-golden-storage.md

## Design Excerpt

This issue is Phase 4 (Migration) of the R2 Golden Storage design. It's the initial bulk upload step - migrating existing 418 golden files from git to R2.

**Dependencies satisfied:**
- #1093: R2 infrastructure setup (done)
- #1094: Helper scripts (done)
- #1095: Publish workflow (done)
- #1096: Nightly validation integration (done)

**What this unblocks:**
- #1098: Parallel R2 and git operation (requires files to exist in R2)

## R2 Object Structure

```
plans/{letter}/{recipe}/v{version}/{platform}.json

Examples:
- plans/f/fzf/v0.60.0/darwin-arm64.json
- plans/r/ripgrep/v14.1.0/linux-amd64.json
```

## Current Golden Files Structure

```
testdata/golden/plans/{letter}/{recipe}/v{version}-{platform}.json

Examples:
- testdata/golden/plans/f/fzf/v0.60.0-darwin-arm64.json
```

Note the difference: git uses `v{version}-{platform}.json` while R2 uses `v{version}/{platform}.json`.

## Migration Approach

The publish-golden-to-r2.yml workflow handles the transformation. For bulk migration:
1. Get list of all recipes with golden files
2. Trigger workflow with `recipes` input (comma-separated list)
3. Verify uploads via r2-download.sh or direct API checks
