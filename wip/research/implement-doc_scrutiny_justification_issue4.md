---
phase: scrutiny
role: pragmatic-reviewer
focus: justification
issue: 4
---

# Scrutiny: Issue 4 Requirements Mapping

## Findings

### 1. Duplicated validation logic in test (Blocking)

`cmd/tsuku/registry_test.go:229-251` -- `validateRegistrySource()` is a hand-rolled reimplementation of `discover.ValidateGitHubURL()`. The test claims to validate the same logic as `runRegistryAdd` but tests a different function with potentially different behavior. The real validator handles full URLs, uses `url.Parse`, and has its own error types. This test proves nothing about the actual code path.

**Fix:** Import and call `discover.ValidateGitHubURL()` directly in the test table. Delete `validateRegistrySource()`.

### 2. Tests don't exercise command output (Advisory)

`cmd/tsuku/registry_test.go` -- Most tests verify config struct manipulation (map has entry, map is empty), not command behavior. `TestRegistryList_NoRegistries` checks `DefaultConfig()` returns zero registries -- that's a userconfig test, not a registry command test. `TestRegistryCmd_NoSubcommand` calls `cmd.Run` but doesn't assert output contains "Usage". The AC says "list displays sources" and "clean output when no registries" but no test captures stdout to verify display formatting.

**Fix:** Capture stdout in list/add/remove tests and assert on output strings. Move pure-config tests to `internal/userconfig/`.

### 3. AC gap: `registry remove` tool listing is untested (Advisory)

AC says `registry remove` "prints informational message listing tools still installed from the removed source." `printToolsFromSource()` at `registry.go:200` implements this, but no test covers it. The function silently swallows errors (lines 203, 209), which is fine for best-effort, but the happy path has zero coverage.

### 4. Requirements mapping accuracy

| Claimed AC | Verdict |
|---|---|
| registry list displays sources | Partially implemented. Code exists, no test verifies output format. |
| registry add validates and saves | Implemented. Round-trip test exists. But validation test uses wrong function (finding 1). |
| registry remove lists remaining tools | Implemented in code. Zero test coverage (finding 3). |
| registry no subcommand shows help | Implemented. Test doesn't assert output. |
| consistent exit codes | Implemented. Uses `exitWithCode(ExitGeneral)` and `exitWithCode(ExitUsage)`. |
| clean output no registries | Test exists but only checks config struct, not command output. |

## Summary

The mapping is directionally correct but overstates test coverage. The code itself is clean and appropriately scoped. Two issues need attention: the duplicated validator in tests (blocks correctness of test suite) and missing output assertions (reduces confidence in display ACs).
