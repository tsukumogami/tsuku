# Maintainer Review: Issue 4 -- tsuku registry subcommands

**Reviewer:** maintainer-reviewer
**Files:** `cmd/tsuku/registry.go`, `cmd/tsuku/registry_test.go`
**Verdict:** Mostly clear. Two findings, one blocking.

---

## Finding 1: Duplicated validation logic between production and test code

**File:** `cmd/tsuku/registry.go:130-137` and `cmd/tsuku/registry_test.go:232-240`
**Severity:** Blocking

`runRegistryAdd` performs a two-step validation: `discover.ValidateGitHubURL()` then a manual `strings.Count(source, "/") != 1` check. The test file duplicates this exact sequence in `validateRegistrySourceForTest()` with a comment saying it "mirrors" the production code.

The next developer who changes the validation in `runRegistryAdd` (say, adding a max-length check) will not see or update the test helper. The tests will keep passing against stale validation logic, silently testing the wrong thing.

Fix: extract the two-step validation into a single exported or package-level function (e.g., `validateRegistrySource`) that both `runRegistryAdd` and the test call directly. The test should exercise the real validation path, not a copy of it.

Additionally, the slash-count check on line 134 is partially redundant with `validateOwnerRepo` inside `ValidateGitHubURL`, which already uses `strings.SplitN(path, "/", 3)` and requires exactly 2 parts. The difference is that `ValidateGitHubURL` accepts `a/b/c` (it splits with limit 3 and only validates the first two segments), so the slash-count check catches that case. This subtlety is not documented. A comment like `// ValidateGitHubURL accepts owner/repo/extra paths; we require exactly owner/repo` would prevent the next developer from removing the "redundant" check.

---

## Finding 2: `printToolsFromSource` silently depends on `Source` field format

**File:** `cmd/tsuku/registry.go:218-219`
**Severity:** Advisory

The function compares `tool.Source == source` where `source` is user input (e.g., `"myorg/recipes"`) and `tool.Source` is a value from `state.json`. The `Source` field documentation (`internal/install/state.go:87-88`) says values are `"central"`, `"local"`, or `"owner/repo"` -- matching the expected format. This works today, but the implicit contract between the CLI layer and the state format is fragile. If the state ever stores full URLs (e.g., `"https://github.com/myorg/recipes"`), this comparison silently finds zero matches and the user gets no warning about orphaned tools.

This is best-effort code with silent failures, so the blast radius is small. No action required, but a brief comment noting the assumed format would help.

---

## Finding 3: Tests don't exercise the actual command handlers

**File:** `cmd/tsuku/registry_test.go`
**Severity:** Advisory

The tests verify config serialization round-trips and data structure manipulation, but none of them invoke `runRegistryList`, `runRegistryAdd`, or `runRegistryRemove`. For example, `TestRegistryAdd_Idempotent` just checks map membership -- it doesn't test that the command prints the expected idempotent message or avoids calling `Save()`. `TestRegistryList_NoRegistries` checks a default config has zero entries but doesn't run the list command to verify the "No registries configured." output.

This is valid unit testing of the config layer, but the test names (`TestRegistryAdd_*`, `TestRegistryList_*`) imply they test the commands. The next developer will assume the command output formatting and error paths are covered when they aren't. Either rename to reflect what they actually test (e.g., `TestConfigRegistryRoundTrip`) or add tests that capture stdout from the actual command runners.

---

## What reads well

- The `printToolsFromSource` function is properly scoped as best-effort and won't fail the remove operation. Good separation.
- Idempotent add and graceful non-existent remove match the acceptance criteria cleanly.
- Sorting registry names for deterministic output is a small detail that prevents flaky comparisons and debugging confusion.
- The `registryCmd` root printing help when invoked without subcommands follows cobra conventions correctly.
