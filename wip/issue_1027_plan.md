# Issue 1027 Implementation Plan

## Summary
Create the minimal tsuku-dltest Rust project with three files as specified in the issue.

## Approach
Follow the exact file contents from the issue's Technical Approach section. The implementation is fully specified.

## Files to Create
- `cmd/tsuku-dltest/Cargo.toml` - Package manifest with release profile
- `cmd/tsuku-dltest/rust-toolchain.toml` - Pin Rust 1.75
- `cmd/tsuku-dltest/src/main.rs` - Version-printing CLI

## Implementation Steps
- [ ] Create cmd/tsuku-dltest/ directory
- [ ] Create Cargo.toml with release profile
- [ ] Create rust-toolchain.toml
- [ ] Create src/main.rs
- [ ] Build and test locally
- [ ] Run validation script from issue

## Success Criteria
- [ ] `cargo build --release` succeeds
- [ ] `--version` prints version and exits 0
- [ ] No arguments prints usage and exits 2
- [ ] Binary size < 1MB

## Open Questions
None - implementation is fully specified.
