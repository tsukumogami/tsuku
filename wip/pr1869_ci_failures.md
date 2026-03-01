# PR #1869 CI Failure Groups

## Group 1: Suse systematic failure (ALL 21 recipes) -- FIXED

Root cause: opensuse/leap:15.6 base image doesn't include `tar` or `gzip`. The `cargo_install` action needs both to extract crate tarballs. Rust itself installs fine on suse.

Fix: added `tar gzip` to the suse package list in test-recipe.yml (both x86_64 and arm64 jobs).

Verified locally: cargo-sweep installs successfully on suse with the fix.

## Group 2: Alpine systematic failure (ALL 21 recipes) -- FIXED

Root cause: Rust embedded recipe used `apk_install` for Alpine, which only verifies packages are present (doesn't install). Alpine's repo Rust was also too old (1.83 vs 1.93).

Fix: Replaced `apk_install` with download of official musl-linked Rust binaries from static.rust-lang.org. Uses `linux_family = "alpine"` when clauses (not `libc = ["musl"]`) to work with the existing golden file infrastructure.

Changes:
- `internal/recipe/recipes/rust.toml` — musl download path instead of apk_install
- `internal/recipe/coverage.go` — recognize `linux_family = "alpine"` as musl fallback
- `internal/recipe/coverage_test.go` — updated warning message strings
- `testdata/golden/plans/embedded/rust/` — regenerated with 7 family-specific files

Verified locally: cargo-sweep installs on Alpine with the musl Rust toolchain.

## Group 3: macOS timeouts (maturin, probe-rs-tools, cargo-nextest)

- **maturin**: timeout on both macOS arm64 + x86_64
- **probe-rs-tools**: timeout on both macOS arm64 + x86_64
- **cargo-nextest**: timeout on macOS arm64 only

Action: increase timeout, remove recipes, or accept as known slow builds.

## Group 4: Broken recipes (komac, cargo-release)

- **komac**: fails ALL Linux families + macOS timeout. Broken.
- **cargo-release**: fails rhel, arch, suse, alpine + macOS timeout. Only passes debian.

Action: remove both recipes.
