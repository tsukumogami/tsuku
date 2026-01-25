---
summary:
  constraints:
    - Write tokens require protected environment (registry-write) with main branch only
    - All GitHub Actions must be pinned to commit SHAs, not version tags
    - Must use r2-upload.sh script from #1094 for actual uploads
    - Cross-platform matrix: ubuntu-latest, macos-14, macos-15-intel
    - Object key pattern: plans/{letter}/{recipe}/v{version}/{platform}.json
  integration_points:
    - scripts/r2-upload.sh (upload with metadata and verification)
    - scripts/regenerate-golden.sh (existing golden file generation)
    - GitHub Secrets: R2_BUCKET_URL, R2_ACCOUNT_ID, R2_ACCESS_KEY_ID_WRITE, R2_SECRET_ACCESS_KEY_WRITE
    - GitHub Environment: registry-write (protected, main branch only)
  risks:
    - macOS runners may be unavailable (design says defer to next run)
    - Concurrent merges affecting same recipe (last-write-wins is acceptable)
    - Partial failures during generation (need graceful handling)
  approach_notes: |
    Create publish-golden-to-r2.yml workflow with:
    1. Dual triggers: push to main (recipes/**/*.toml) + workflow_dispatch
    2. Detect changed recipes from git diff (push trigger) or input (manual)
    3. Matrix job for 3 platforms generating golden files
    4. Upload job collecting artifacts and calling r2-upload.sh
    5. Manifest update (create if missing, append otherwise)
---

# Implementation Context: Issue #1095

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 2: Post-Merge Generation Workflow)

## Key Design Excerpts

### Post-Merge Generation Flow

```
1. Recipe merged to main
   |
   v
2. Post-merge workflow triggered
   |
   +---> [linux runner] Generate linux-amd64, linux-family variants
   +---> [macos-14] Generate darwin-arm64
   +---> [macos-15-intel] Generate darwin-amd64
   |
   v
3. Collect artifacts, upload to R2
   |
   v
4. Verify uploads with read-back check
   |
   v
5. Update manifest.json
```

### R2 Object Key Convention

```
plans/{category}/{recipe}/v{version}/{platform}.json

Examples:
- plans/a/ack/v3.9.0/darwin-arm64.json
- plans/embedded/go/v1.25.5/linux-amd64.json
- plans/f/fzf/v0.60.0/linux-debian-amd64.json
```

### Object Metadata

```json
{
  "x-tsuku-recipe-hash": "sha256:abc123...",
  "x-tsuku-generated-at": "2026-01-24T10:00:00Z",
  "x-tsuku-format-version": "3",
  "x-tsuku-generator-version": "0.15.0"
}
```

### Manual Trigger Usage

```bash
# Single recipe
gh workflow run publish-golden-to-r2.yml -f recipes=fzf

# Multiple recipes
gh workflow run publish-golden-to-r2.yml -f recipes="fzf,ripgrep,bat"

# Force regeneration
gh workflow run publish-golden-to-r2.yml -f recipes=fzf -f force=true
```

### Security Requirements

- Write tokens protected by GitHub Environment `registry-write` requiring main branch only
- All GitHub Actions must be pinned to specific commit SHAs
- Golden file content must never be interpolated into shell commands
