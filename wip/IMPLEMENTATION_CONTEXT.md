---
summary:
  constraints:
    - This issue only adds workflow structure, not goreleaser changes or Rust builds
    - Existing goreleaser release job must continue working unchanged for now
    - Job dependency chain must be correct: create-draft -> builds -> finalize
    - Outputs (release_id, upload_url) must be properly exposed for downstream jobs
  integration_points:
    - .github/workflows/release.yml - the single file being modified
    - goreleaser job receives release_id from create-draft-release
    - finalize-release depends on release job (will expand to all build jobs later)
  risks:
    - Draft release API syntax may differ from expected (test with dry-run)
    - GITHUB_TOKEN permissions must include write access to releases
    - If create-draft-release fails, all downstream jobs should be skipped
  approach_notes: |
    Add two new jobs around the existing release job:
    1. create-draft-release: Creates draft, outputs release_id and upload_url
    2. finalize-release: Publishes the draft after builds complete

    The existing release job becomes: needs: [create-draft-release]
    The finalize-release job depends on release (and will expand to all build jobs).

    Use gh release create --draft to create, gh release edit --draft=false to publish.
---

# Implementation Context: Issue #1025

**Source**: docs/designs/DESIGN-release-workflow-native.md

## Key Design Points

1. **Draft-then-publish pattern**: Create draft release first, upload artifacts, then publish atomically
2. **Outputs for downstream**: create-draft-release must output `release_id` and `upload_url`
3. **Correct dependencies**: create-draft -> builds -> finalize
4. **Preserve existing behavior**: The goreleaser job should continue working

## Validation Script Key Checks

- create-draft-release job exists
- finalize-release job exists
- create-draft-release outputs release_id
- finalize-release has needs dependency
