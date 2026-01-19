# Issue 1028 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-release-workflow-native.md`
- Sibling issues reviewed: #1025 (closed), #1027 (closed)
- Linked PRs: #1042 (draft-finalize structure), #1050 (Rust project)
- Prior patterns identified:
  - Workflow uses `create-draft-release` job with `release_id` and `upload_url` outputs
  - `finalize-release` job depends on build jobs and publishes via `gh release edit --draft=false`
  - Rust toolchain pinned to `1.75` in `rust-toolchain.toml`
  - Rust project exists at `cmd/tsuku-dltest/` with release profile (LTO, strip, panic=abort)

## Gap Analysis

### Minor Gaps

1. **Job naming pattern established**: #1025 implemented the workflow with job names `create-draft-release`, `release` (for Go builds), and `finalize-release`. The new `build-rust` job should follow this naming convention and fit into this chain.

2. **Version injection method for Cargo.toml**: The issue spec shows `sed` to update Cargo.toml version, which is correct. The current Cargo.toml has `version = "0.0.0"` as a placeholder - the workflow step must update this from the git tag.

3. **macOS sed syntax**: Issue spec uses `sed -i` which works on Linux but macOS requires `sed -i ''`. The implementation should use platform-appropriate syntax or use a portable approach.

4. **Workflow `if:` condition pattern**: #1025 established the pattern `if: needs.create-draft-release.result == 'success'` for dependent jobs. The new build-rust job should follow this pattern.

5. **Artifact naming convention**: Design specifies `tsuku-dltest-{platform}` naming (e.g., `tsuku-dltest-linux-amd64`). The implementation should match this exactly for downstream verification in #1031.

### Moderate Gaps

None identified. The issue spec is well-aligned with the implemented foundation.

### Major Gaps

None identified. Both dependencies (#1025, #1027) have been completed and the implementations match what the design specified.

## Recommendation

**Proceed**

## Rationale

1. Both dependencies (#1025 and #1027) are closed and their implementations match the design specification.

2. The workflow structure from #1025 (PR #1042) provides exactly the foundation specified in the issue:
   - `create-draft-release` outputs `release_id` for artifact uploads
   - `finalize-release` job exists and uses the correct pattern
   - The job dependency chain supports adding parallel build jobs

3. The Rust project from #1027 (PR #1050) is minimal but buildable:
   - `cmd/tsuku-dltest/Cargo.toml` exists with release profile
   - `rust-toolchain.toml` pins Rust 1.75
   - Version is read from `CARGO_PKG_VERSION` at compile time

4. The issue spec already accounts for the correct patterns:
   - Uses `dtolnay/rust-toolchain@stable` action (standard approach)
   - Includes version injection via sed on Cargo.toml
   - Includes ad-hoc code signing for macOS
   - Artifact upload via `gh release upload` to draft release

## Implementation Notes

The following details from completed work should inform implementation:

1. **Current workflow structure** (`release.yml`):
   ```yaml
   jobs:
     create-draft-release: # outputs: release_id, upload_url
     release:              # goreleaser for Go builds
     finalize-release:     # depends on release, publishes draft
   ```
   The new `build-rust` job should be added after `release` in the file and included in `finalize-release`'s `needs:` array.

2. **Rust toolchain version**: Pinned to `1.75` in `rust-toolchain.toml`. The `dtolnay/rust-toolchain` action should be consistent with this.

3. **Binary output location**: After `cargo build --release`, binary is at `target/release/tsuku-dltest`.

4. **musl build approach**: Design specifies using Alpine container via Docker. Should use `alpine:3.19` as shown in design doc.
