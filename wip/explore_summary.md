# Exploration Summary: Cargo Subcommand Verification

## Problem (Phase 1)
The CargoBuilder generates `<executable> --version` as the verify command for all cargo crates, but cargo subcommands (crates named `cargo-*`) often require invocation through cargo itself (`cargo <subcommand> --version`). This causes verification failures for correctly installed tools.

## Decision Drivers (Phase 1)
- Must work in isolated $TSUKU_HOME where cargo may or may not be present
- Should handle both subcommands that work with direct invocation and those that don't
- The repair loop fallback (`--help`, `-h`) already handles some cases but doesn't try `cargo <subcommand> --version`
- Existing cargo-edit recipe shows the manual workaround pattern
- The batch generation system creates recipes at scale, so manual verify command tuning isn't viable

## Research Findings (Phase 2)
- Root cause: `cargo.go:160` - `fmt.Sprintf("%s --version", executables[0])` for all crates
- The repair loop (`orchestrator.go:340-470`) tries output analysis, `--help`, `-h` fallbacks but not `cargo <subcommand>` invocation
- cargo-edit recipe demonstrates manual workaround using `--help` + pattern matching
- All other ecosystem builders (npm, pypi, gem, go, cpan) have the same `<exe> --version` default pattern
- Cargo subcommand behavior is inconsistent across crates: some support standalone `--version`, others don't

## Options (Phase 3)
- Decision 1: Fix CargoBuilder + repair loop fallback (chosen) vs repair-only vs manual fixes
- Decision 2: Default to `cargo <subcommand>` for all subcommands (chosen) vs try direct first vs detect at build time

## Decision (Phase 5)

**Problem:**
The CargoBuilder generates `<executable> --version` as the verify command for all cargo crates. This fails for cargo subcommands (crates named `cargo-*`) that require invocation through cargo itself. The batch generation system creates these recipes at scale, so manual workarounds don't work. Users see verification failures on correctly installed tools.

**Decision:**
Change `CargoBuilder.Build()` to detect cargo subcommands (executables matching `cargo-*`) and generate `cargo <subcommand> --version` as the verify command. Add a repair loop fallback for the same pattern to handle existing recipes with the wrong command. Also set the `Pattern` field to `{version}` to enable version validation, which the current code omits.

**Rationale:**
Fixing the builder prevents bad verify commands from being generated. The repair loop fallback provides defense in depth for existing recipes. Using `cargo <subcommand> --version` for all subcommands is simpler than probing at build time, and the cargo PATH dependency is already satisfied by `cargo_install`'s own requirements.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-22
