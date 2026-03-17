---
role: pragmatic-reviewer
issue: 4
---

# Pragmatic Review: Issue 4 (registry subcommands)

## Findings

### 1. Duplicated validation logic between production and test code

**File:** `cmd/tsuku/registry_test.go:232-240`
**Severity:** Advisory

`validateRegistrySourceForTest()` duplicates the two-step validation from `runRegistryAdd()` (lines 130-137 in registry.go). If validation logic changes in one place, the test helper silently diverges. Extract a shared `validateRegistrySource(string) error` function called by both `runRegistryAdd` and the test table. This is small enough that it's advisory, not blocking -- the duplication is only two calls.

### 2. Weak test for no-subcommand help path

**File:** `cmd/tsuku/registry_test.go:15-24`
**Severity:** Advisory

`TestRegistryCmd_NoSubcommand` calls `cmd.Run(cmd, []string{})` directly, bypassing cobra's dispatch. It asserts nothing beyond "no panic." The help output goes to a buffer that's never checked. This test provides false confidence -- it wouldn't catch a regression where the help text disappears or `Run` is changed to `RunE` returning an error. Either assert the buffer contains "registry" or delete the test.

### 3. `printToolsFromSource` is a single-caller helper but justified

**File:** `cmd/tsuku/registry.go:205-234`
**Severity:** Not blocking

Called only from `runRegistryRemove`. Normally I'd flag this, but it has a clear name and encapsulates the best-effort state loading pattern (two silent returns on error). Inlining would make `runRegistryRemove` harder to read. No action needed.

## No blocking findings.

The implementation is straightforward, matches the acceptance criteria, and doesn't over-engineer. The validation split between `ValidateGitHubURL` and the slash-count check is necessary since `validateOwnerRepo` accepts extra path segments. The `Registries` map cleanup (nil on empty) keeps config.toml tidy without introducing complexity. The `strict_registries` field isn't wired into `Get`/`Set`/`AvailableKeys()` but that's out of scope for this issue.
