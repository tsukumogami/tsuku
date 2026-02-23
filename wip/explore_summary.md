# Exploration Summary: Deterministic Recipe Repair

## Problem (Phase 1)

The Cargo builder's `discoverExecutables()` fetches the root Cargo.toml from GitHub to find `[[bin]]` sections, but this fails for workspace monorepos where binaries are defined in member crates. The fallback is the crate name, which is often wrong (e.g., `sqlx-cli` produces binary `sqlx`, `probe-rs-tools` produces `probe-rs`, `cargo-flash`, `cargo-embed`). These mismatches were caught manually during PR #1869 testing. The issue proposes a deterministic validation step that queries registry APIs for actual binary names and auto-corrects mismatches.

## Decision Drivers (Phase 1)

- Must work for workspace monorepos (the primary failure case)
- Should not require network calls that aren't already happening
- Must integrate into the existing builder pipeline without breaking the generate-validate-repair cycle
- Accuracy matters more than speed -- wrong binaries cause install failures
- Should generalize beyond crates.io to other registries (npm, PyPI, etc.)
- The crates.io API does NOT expose a `bin_names` field (contrary to the issue's assumption)
- Crate tarballs on crates.io contain the correct sub-crate Cargo.toml
- Start with crates.io since that's where most mismatches occur

## Research Findings (Phase 2)

**crates.io**: API has `bin_names` field on version objects. This is the authoritative source -- no need to parse Cargo.toml at all. The current code fetches root Cargo.toml from GitHub, which fails for workspace monorepos.

**npm**: API has `bin` field in standard metadata response. Already used by npm builder, but `parseBinField()` returns nil for string-type bin values (single executable).

**PyPI**: No API for executables. Must download `.whl` and extract `entry_points.txt`. Current builder fetches pyproject.toml from GitHub (same monorepo risk as Cargo).

**RubyGems**: No API for executables. Must download `.gem` and extract `metadata.gz`. Current builder fetches gemspec from GitHub.

**Go**: No metadata at all. Uses heuristic (last path segment of module path). No repair possible without source analysis.

**Orchestrator gap**: Self-repair only handles verification failures (help text detection). Exit code 127 (command not found) is detected but not repaired -- the wrong binary name just fails.

**Key insight**: The fix for crates.io is trivial (use `bin_names` from API already being called). The fix for npm is a minor parser improvement. PyPI and RubyGems need artifact downloads. Go has no good solution.

## Current Status
**Phase:** 2 - Research
**Last Updated:** 2026-02-22
