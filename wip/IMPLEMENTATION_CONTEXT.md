---
summary:
  constraints:
    - Must build on native runners (can't cross-compile macOS due to SDK licensing)
    - glibc variant uses ubuntu-22.04 (glibc 2.35), musl uses Alpine container
    - macOS requires ad-hoc code signing (codesign -s -)
    - Version must be injected from git tag into Cargo.toml before build
    - Job depends on create-draft-release and uploads to existing draft
  integration_points:
    - .github/workflows/release.yml (add build-rust job matrix)
    - Depends on create-draft-release job output (release_id)
    - Uploads artifacts to draft release created by #1025
    - Downstream: #1030 (integration-test) depends on this job
  risks:
    - ARM64 runner availability (ubuntu-24.04-arm is in preview)
    - Alpine container Rust version may differ from rust-toolchain.toml
    - musl builds in containers need careful working directory handling
  approach_notes: |
    Add build-rust job matrix to release.yml covering 6 platforms:
    - linux-amd64 (ubuntu-22.04)
    - linux-amd64-musl (Alpine container on ubuntu-22.04)
    - linux-arm64 (ubuntu-24.04-arm)
    - linux-arm64-musl (Alpine container on ubuntu-24.04-arm)
    - darwin-amd64 (macos-13)
    - darwin-arm64 (macos-latest)

    Each job: checkout, setup Rust, inject version, build, sign (macOS), upload.
    musl variants use docker run with Alpine container for consistent toolchain.
---

# Implementation Context: Issue #1028

**Source**: docs/designs/DESIGN-release-workflow-native.md (Step 4)

## Key Design Details

### Platform Matrix (from design)

| Platform | Runner | Build Type | Notes |
|----------|--------|------------|-------|
| linux-amd64 | ubuntu-22.04 | glibc | Direct build |
| linux-amd64-musl | ubuntu-22.04 | musl | Alpine container |
| linux-arm64 | ubuntu-24.04-arm | glibc | Direct build |
| linux-arm64-musl | ubuntu-24.04-arm | musl | Alpine container |
| darwin-amd64 | macos-13 | libSystem | Ad-hoc code signing |
| darwin-arm64 | macos-latest | libSystem | Ad-hoc code signing |

### Version Injection

```bash
VERSION="${GITHUB_REF_NAME#v}"
sed -i "s/^version = .*/version = \"$VERSION\"/" cmd/tsuku-dltest/Cargo.toml
```

### Artifact Naming Convention

- `tsuku-dltest-linux-amd64`
- `tsuku-dltest-linux-amd64-musl`
- `tsuku-dltest-linux-arm64`
- `tsuku-dltest-linux-arm64-musl`
- `tsuku-dltest-darwin-amd64`
- `tsuku-dltest-darwin-arm64`

### Job Dependency Chain

```
create-draft-release
  └─► build-rust (needs: create-draft-release, if: success)
        └─► integration-test (needs: build-*, if: all succeed)
              └─► finalize-release (needs: integration-test, if: success)
```
