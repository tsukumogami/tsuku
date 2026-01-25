# Issue 1095 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-r2-golden-storage.md`
- Sibling issues reviewed: #1093, #1094
- Prior patterns identified:
  - `scripts/r2-upload.sh` establishes the upload pattern with metadata and verification
  - `scripts/r2-download.sh` establishes the download and checksum validation pattern
  - `scripts/r2-health-check.sh` establishes health check contract
  - `scripts/regenerate-golden.sh` is the existing generation script
  - `generate-golden-files.yml` is the existing cross-platform generation workflow (not R2-aware)

## Gap Analysis

### Minor Gaps

1. **Reuse generate-golden-files.yml pattern**: The issue specifies creating `publish-golden-to-r2.yml`, but there's already a `generate-golden-files.yml` workflow that handles cross-platform generation with artifact collection. The new workflow should follow its established patterns:
   - Matrix strategy with same runners: `ubuntu-latest`, `macos-14`, `macos-15-intel`
   - Artifact upload/download pattern for cross-platform collection
   - Go setup with `go-version-file: 'go.mod'`

2. **Golden path computation pattern**: The existing `generate-golden-files.yml` computes paths using first letter extraction. The upload script follows the same pattern via `--category` auto-detection.

3. **Changed recipe detection**: The issue mentions automatic trigger "on push to main when `recipes/**/*.toml` changes" but doesn't specify how to identify which recipes changed. This should follow patterns from existing validation workflows that use `git diff` to detect changed files.

4. **Embedded recipe handling**: The upload script supports `--category embedded` but the issue focuses on registry recipes. For clarity, the workflow should handle both categories since the design shows `plans/embedded/go/v1.25.5/darwin-arm64.json` in the bucket structure.

### Moderate Gaps

1. **Manifest update logic**: The issue lists "Manifest handling creates `manifest.json` on first upload if missing, or appends to existing manifest" as acceptance criteria, but neither the existing scripts nor the design doc specify the exact manifest update algorithm. Questions:
   - Should manifest update be atomic (read-modify-write)?
   - How to handle concurrent uploads from parallel matrix jobs?
   - Should each platform job update manifest independently, or should there be a final consolidation step?

   **Proposed amendment**: Add a manifest update script or extend `r2-upload.sh` to optionally update manifest after upload. Final consolidation should happen in a post-generation step after all platforms complete.

2. **Batch vs. single recipe**: The issue mentions manual trigger with "comma-separated list of recipe names" but doesn't clarify behavior when automatic trigger detects multiple changed recipes. Should each recipe be processed sequentially, or should there be parallel handling?

   **Proposed amendment**: For automatic triggers, detect all changed recipes and process them in parallel (one workflow run generates all changed recipes across all platforms). For manual triggers, allow comma-separated list with same parallel processing.

### Major Gaps

None identified. The issue spec is largely complete and aligns well with what #1093 and #1094 delivered.

## Recommendation

**Clarify**

The issue is implementable, but two moderate gaps need user confirmation before proceeding:

1. Manifest update mechanism (atomic updates, concurrency handling)
2. Batch processing behavior for multiple changed recipes

## Proposed Amendments

If user approves, add the following clarifications to the issue:

1. **Manifest handling**:
   - Manifest update should occur in a separate job that runs after all platform generation jobs complete
   - Use read-modify-write with S3 conditional PUT (If-Match header) to handle concurrent updates
   - If manifest doesn't exist, create it from scratch listing all currently generated files

2. **Batch recipe processing**:
   - Automatic trigger: Use `git diff` to identify all changed `.toml` files in `recipes/`, generate for all in single workflow run
   - Manual trigger: Parse comma-separated input, generate all specified recipes in single workflow run
   - Platform matrix handles cross-platform generation; recipe list can be processed sequentially within each platform job

3. **Category handling**:
   - Support both `embedded` and `registry` categories in the workflow
   - Auto-detect category based on recipe location (same logic as `regenerate-golden.sh`)
