# Explore Scope: sandbox-configure-make-arm64

## Visibility

Public

## Core Question

Why do `configure_make` recipes fail plan generation on linux/arm64 + glibc, what is the right fix, and does this need upstream design work or can it be implemented directly?

## Context

Issue #2374 documents a failure mode in `tsuku install --sandbox --recipe <X> --target-family <Y> --json` on linux/arm64 + glibc (debian, rhel, suse). The CI output for PR #2373 shows `exit null` for install_exit_code on those families. Root cause is well-specified: `HomebrewAction.Decompose` fails for install-time dependencies that lack `arm64_linux` bottles in GHCR, and that error routes through `handleInstallError` which emits a JSON field named `exit_code` rather than `install_exit_code` (causing the null).

## In Scope

- Which install-time deps of `configure_make` lack arm64_linux homebrew bottles
- Whether `WhenClause` supports `arch` filtering (to add arm64-specific fallback steps)
- JSON field name mismatch between `installError` and `sandboxJSONOutput`
- CI workflow change to upload per-recipe failure logs
- Recipe fixes for pkg-config.toml and make.toml

## Out of Scope

- Auto-fallback in `HomebrewAction` itself (issue step 5 — more complex, not required to unblock)
- The pcre2 recipe itself (PR #2373 handles that)
- zig.toml (zig uses arch_mapping direct download, not homebrew — not affected)

## Research Leads

1. **Do pkgconf and make lack arm64_linux bottles in GHCR?**
   Confirmed via GHCR manifest query — both return 0 arm64_linux entries.

2. **Does WhenClause support arch filtering?**
   Confirmed — `WhenClause.Arch string` exists in `internal/recipe/types.go:285` with Matches() evaluation at line 338.

3. **What is the JSON field name mismatch?**
   `installError` has `ExitCode int \`json:"exit_code"\`` while `sandboxJSONOutput` uses `install_exit_code`. The CI jq filter checks `.install_exit_code`, which is absent from error responses.

4. **Does zig need fixing?**
   No — zig uses arch_mapping with direct download from ziglang.org, not homebrew.
