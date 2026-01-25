# Issue 1097 Implementation Plan

## Overview

Migrate existing golden files to R2 using the publish workflow. This is primarily an operational task - the infrastructure (workflow, scripts) is already in place.

## Current State

- **Registry recipes with golden files**: ~117
- **Total golden files**: 418
- **Infrastructure ready**: publish-golden-to-r2.yml workflow supports manual dispatch

## Approach

The publish workflow regenerates golden files fresh (doesn't upload existing files). This is correct because:
1. Ensures R2 has correctly-formatted files matching the R2 structure
2. Validates that generation still works for all recipes
3. Adds proper metadata during upload

### Batch Size Considerations

The workflow runs generation on 3 platforms (linux, darwin-arm64, darwin-amd64) concurrently. Processing too many recipes at once may hit:
- GitHub Actions job duration limits (6 hours)
- Artifact size limits
- API rate limits

Recommend batches of ~20-30 recipes per workflow run.

## Migration Steps

### Step 1: Prepare Recipe List

Split recipes into batches for workflow runs:
- Batch 1: a-c (~15 recipes)
- Batch 2: d-g (~20 recipes)
- Batch 3: h-m (~15 recipes)
- Batch 4: n-s (~35 recipes)
- Batch 5: t-z (~32 recipes)

### Step 2: Execute Migration

For each batch:
```bash
gh workflow run publish-golden-to-r2.yml -f recipes="<batch>" -f force=true
```

Wait for completion before starting next batch.

### Step 3: Verify Uploads

After all batches complete:
1. Use r2-download.sh to spot-check sample files
2. Count files in R2 vs expected
3. Verify checksums match

### Step 4: Document Results

Create migration log with:
- Workflow run URLs
- Success/failure counts
- Any recipes that failed and need retry

## Verification Script

The issue's validation script assumes a manifest.json endpoint, which doesn't exist yet. Use alternative verification:

```bash
# Spot check specific recipes
for recipe in fzf ripgrep bat; do
  ./scripts/r2-download.sh "$recipe" latest linux-amd64 /tmp/test.json
  echo "$recipe: $(cat /tmp/test.json | jq -r '.version')"
done
```

## Risk Mitigation

1. **Partial failures**: The force flag allows re-running individual batches
2. **Workflow failures**: Monitor each run before proceeding to next batch
3. **Rate limits**: Space out batch submissions if needed

## Files Changed

This issue requires no code changes. It's purely operational - triggering existing workflows to populate R2.
