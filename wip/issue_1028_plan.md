# Issue 1028 Implementation Plan

## Summary

Add a `build-rust` job matrix to `.github/workflows/release.yml` that builds the `tsuku-dltest` binary on 6 platforms (linux-amd64, linux-amd64-musl, linux-arm64, linux-arm64-musl, darwin-amd64, darwin-arm64) using native runners and uploads artifacts to the draft release.

## Approach

Use a matrix strategy with include-based configuration to handle the platform-specific differences (runners, container images, build commands, code signing). The job will depend on `create-draft-release` and upload artifacts to the existing draft. Glibc builds run directly on runners; musl builds run in Alpine containers. macOS builds require ad-hoc code signing.

## Files to Modify

- `.github/workflows/release.yml` - Add `build-rust` job with 6-platform matrix between `create-draft-release` and `release` jobs; update `finalize-release` to depend on the new job

## Files to Create

None required.

## Implementation Steps

- [ ] Add `build-rust` job definition with matrix strategy
- [ ] Configure matrix include entries for all 6 platforms with runner, artifact name, and build type
- [ ] Add checkout step with sparse checkout for cmd/tsuku-dltest directory only
- [ ] Add version injection step using sed to update Cargo.toml
- [ ] Add Rust setup step using dtolnay/rust-toolchain action
- [ ] Add glibc build step for linux and darwin platforms (conditional)
- [ ] Add musl build step using Alpine container with docker run (conditional)
- [ ] Add macOS code signing step with codesign -s - (conditional)
- [ ] Add artifact upload step using gh release upload
- [ ] Update `release` job to also depend on `build-rust`
- [ ] Update `finalize-release` job to depend on `build-rust`

## Success Criteria

- [ ] Workflow syntax validates (no YAML errors)
- [ ] Matrix correctly defines all 6 platform combinations
- [ ] Version injection correctly strips 'v' prefix from tag
- [ ] Glibc builds use direct cargo build on native runners
- [ ] Musl builds use Alpine container with correct working directory
- [ ] macOS builds include ad-hoc code signing step
- [ ] Artifacts are named according to convention: `tsuku-dltest-<platform>`
- [ ] Job dependencies form correct chain: create-draft-release -> build-rust -> finalize-release

## Open Questions

None - the design document and implementation context provide sufficient detail.
