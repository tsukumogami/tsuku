# Completeness Scrutiny: Issue 4 (registry subcommands)

## Requirements Mapping Audit

Source: PLAN-distributed-recipes.md, Issue 4 (lines 108-124)

### AC-by-AC Verification

| # | Acceptance Criterion | Claimed Status | Verified | Notes |
|---|---------------------|---------------|----------|-------|
| 1 | `registry list` displays each source with URL, `(auto-registered)` annotation, and `strict_registries` status | implemented | **YES** | `registryList()` lines 97-122: sorts names, prints URL with annotation, prints strict status |
| 2 | `registry add <owner/repo>` validates with `ValidateGitHubURL()`, adds entry, sets `AutoRegistered = false`, saves config; idempotent | implemented | **PARTIAL** | See finding F1 below |
| 3 | `registry remove <owner/repo>` removes entry, does NOT remove tools (R13), prints tools still installed from source, handles non-existent gracefully | implemented | **YES** | `registryRemove()` lines 163-195, `printToolsFromSource()` lines 200-229 |
| 4 | `registry` with no subcommand prints help | implemented | **YES** | Line 31-33: `cmd.Help()` |
| 5 | Subcommands registered in main command tree with consistent exit codes and error formatting | implemented | **YES** | `main.go:111` adds to root; uses `ExitGeneral`/`ExitUsage` from `exitcodes.go` |
| 6 | Clean output when no registries are configured | implemented | **YES** | Line 88-93: "No registries configured." with conditional strict status |

### Findings

**F1: `registry list` omits `strict_registries` status when disabled and registries are empty.** (Advisory)

AC1 says "displays ... `strict_registries` status." When `cfg.StrictRegistries` is false AND registries are empty, lines 88-93 print "No registries configured." and return without showing the strict status. The non-empty path (lines 118-122) always prints both "enabled" and "disabled." This asymmetry means `registry list` on a fresh config shows no strict status at all. Not blocking because the disabled state is the default and omitting it is defensible UX, but it's inconsistent with the non-empty code path.

**F2: Test uses local `validateRegistrySource()` instead of `discover.ValidateGitHubURL()`.** (Advisory)

`registry_test.go:229-251` defines a local `validateRegistrySource()` that reimplements a subset of the validation logic. The production code (`registry.go:129`) calls `discover.ValidateGitHubURL()`. The test helper doesn't exercise the actual validation function, so a divergence between the two won't be caught. The test validates the *pattern* but not the *implementation*. Not blocking because the production code does use the correct function, and the test is supplementary.

**F3: No gap in downstream contract for Issue 7.** (No finding)

Issue 7 depends on Issue 4 for: (a) registry config exists in `config.toml` so install can check registration status, (b) `tsuku registry add` exists so error messages can suggest it. Both are satisfied. The `RegistryEntry` struct, `StrictRegistries` field, and `Registries` map are all in `userconfig.go` and round-trip correctly through TOML.

### Summary

The requirements mapping is **accurate with one minor completeness gap** (F1) and one test-quality note (F2). No blocking issues. The implementation is structurally sound: it uses the existing exit code constants, follows the cobra subcommand pattern used by other commands, reads/writes config through `userconfig.Load()`/`Save()` (not a parallel config path), and correctly reuses `discover.ValidateGitHubURL()` for input validation.
