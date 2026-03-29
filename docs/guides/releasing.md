# Releasing

This guide describes how tsuku releases work, from version selection
through published release.

## Pipeline Overview

Releases flow through three workflow files:

```
release-prepare.yml    →  release.yml           →  release-finalize.yml
(stamp, tag, push)        (build + test)            (verify, publish, checksums)
     manual               triggered by tag          triggered by workflow_run
```

All version stamping happens once in the prepare step. The build step
reads versions from the committed files. The finalize step runs
automatically after builds complete.

## Using /shirabe:release (Recommended)

The `/shirabe:release` skill automates the full process:

```
/release           # analyzes commits, recommends version
/release 0.6.2     # uses specified version
/release --dry-run  # previews without side effects
```

The skill handles:

1. **Version selection** -- analyzes conventional commits since the last
   tag and recommends major/minor/patch based on commit prefixes
2. **Precondition checks** -- clean tree, CI green, no existing tag/draft,
   no release blockers
3. **Release notes** -- groups merged PRs by type, drafts user-facing notes
4. **Draft creation** -- creates a GitHub draft release with the notes
5. **Workflow dispatch** -- triggers `release-prepare.yml` with version,
   tag, and ref
6. **Monitoring** -- polls the workflow run and reports success/failure

After the skill dispatches, the three-step pipeline runs automatically.

## Manual Release

If you need to release without the skill:

### 1. Create a draft release

```bash
gh release create v0.6.2 --draft --title "v0.6.2" --notes "Release notes here"
```

### 2. Dispatch the prepare workflow

```bash
gh workflow run release-prepare.yml \
  -f version=0.6.2 \
  -f tag=v0.6.2 \
  -f ref=main
```

This triggers the full pipeline. To validate without side effects:

```bash
gh workflow run release-prepare.yml \
  -f version=0.6.2 \
  -f tag=v0.6.2 \
  -f ref=main \
  -f dry-run=true
```

### 3. Monitor

The prepare step pushes a tag, which triggers the build. After builds
complete, finalize runs automatically. Watch progress in GitHub Actions
or with:

```bash
gh run list --workflow=release-prepare.yml --limit=1
gh run list --workflow=release.yml --limit=1
gh run list --workflow=release-finalize.yml --limit=1
```

## What Each Step Does

### release-prepare.yml (step 1)

Calls shirabe's reusable release workflow, which:

1. Validates inputs and permissions
2. Runs `.release/set-version.sh 0.6.2` to stamp Cargo.toml files
3. Commits: `chore(release): set version to v0.6.2`
4. Creates annotated tag `v0.6.2` (with notes from draft release)
5. Runs `.release/set-version.sh 0.6.3-dev` for the next dev version
6. Commits: `chore(release): advance to 0.6.3-dev`
7. Pushes branch and tag

### release.yml (step 2)

Triggered by the tag push. Builds everything and uploads to the draft
release. If a draft already exists (created by `/release` in step 1),
assets are uploaded to it. Otherwise, a new draft is created from the
tag annotation.

- **Go binary** -- goreleaser builds 4 platform binaries (publish
  disabled; a separate step uploads assets to the draft)
- **tsuku-dltest** -- Rust binary built on native runners (4 glibc + 2 musl)
- **tsuku-llm** -- Rust binary with GPU backends (2 macOS + 4 Linux)
- **Integration tests** -- validates binaries on each platform

### release-finalize.yml (step 3)

Triggered automatically when the Release workflow completes. Calls
shirabe's reusable finalize workflow, which:

1. Verifies at least 16 binary assets are present
2. Promotes the draft release to published
3. Runs `.release/post-release.sh` to generate unified checksums.txt
   covering all binaries (replacing goreleaser's partial checksums)

## Version Surface

All version injection points:

| Component | How version is set |
|-----------|--------------------|
| Go binary (`tsuku`) | goreleaser ldflags from git tag |
| `cmd/tsuku-dltest/Cargo.toml` | `.release/set-version.sh` |
| `tsuku-llm/Cargo.toml` | `.release/set-version.sh` |

## Hook Scripts

| Script | Called by | Purpose |
|--------|----------|---------|
| `.release/set-version.sh` | shirabe release.yml (step 1) | Stamps Cargo.toml version fields |
| `.release/post-release.sh` | shirabe finalize-release.yml (step 3) | Generates unified checksums.txt |

## Expected Assets (16)

| Source | Count | Artifacts |
|--------|-------|-----------|
| goreleaser | 4 | tsuku-{linux,darwin}-{amd64,arm64} |
| build-rust | 4 | tsuku-dltest-{linux,darwin}-{amd64,arm64} |
| build-rust-musl | 2 | tsuku-dltest-linux-{amd64,arm64}-musl |
| build-llm | 6 | tsuku-llm-{darwin-arm64,darwin-amd64,linux-{amd64,arm64}-{cuda,vulkan}} |

## Prerequisites

- `RELEASE_PAT` secret configured with push and release edit permissions
- Draft release created before dispatch (the skill handles this)
- Clean main branch with passing CI

## Error Recovery

| Problem | Fix |
|---------|-----|
| Tag already exists | `git push --delete origin v0.6.2` and `git tag -d v0.6.2` |
| Draft already exists | `gh release delete v0.6.2 --yes` |
| Build failed | Fix and re-tag, or delete tag and re-dispatch |
| Finalize didn't trigger | Run manually: `gh workflow run release-finalize.yml` (not possible for workflow_run triggers -- re-run the Release workflow instead) |
| Wrong version stamped | Delete the tag, fix, re-dispatch |
