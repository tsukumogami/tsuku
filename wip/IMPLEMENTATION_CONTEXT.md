---
summary:
  constraints:
    - Minimal implementation - only version printing, no dlopen logic
    - Version read from CARGO_PKG_VERSION at compile time
    - Binary size < 1MB after stripping
    - Pin Rust toolchain to stable version (1.75)
  integration_points:
    - cmd/tsuku-dltest/ - new Rust project directory
    - Will be built by workflow in #1028
    - Version injected via sed on Cargo.toml during release
  risks:
    - Rust toolchain version may affect binary compatibility
    - Release profile settings affect binary size
  approach_notes: |
    Create three files in cmd/tsuku-dltest/:
    - Cargo.toml with release profile (LTO, strip, panic=abort)
    - rust-toolchain.toml pinning Rust 1.75
    - src/main.rs with minimal version-printing CLI
---

# Implementation Context: Issue #1027

**Source**: docs/designs/DESIGN-release-workflow-native.md (Step 3)

## Key Design Points

1. **Minimal implementation**: Only print version, exit with correct codes
2. **Release profile**: LTO, strip, panic=abort for small binaries
3. **Pinned toolchain**: rust-toolchain.toml with stable version
4. **Version injection**: Via CARGO_PKG_VERSION at compile time

## Validation Script Key Checks

- Files exist (Cargo.toml, rust-toolchain.toml, src/main.rs)
- `cargo build --release` succeeds
- `--version` prints version and exits 0
- No arguments prints usage and exits 2
- Binary size < 1MB
