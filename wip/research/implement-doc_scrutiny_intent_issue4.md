# Intent Scrutiny: Issue 4 -- tsuku registry subcommands

## Acceptance Criteria Mapping

### AC1: `registry list` displays sources with URL, auto-registered annotation, strict_registries status
**Status: Implemented.** `runRegistryList()` at `registry.go:81-123` shows sorted registries with URL, `(auto-registered)` annotation, and strict_registries line.

### AC2: `registry add` validates format, saves config, sets AutoRegistered=false, idempotent
**Status: Implemented.** `runRegistryAdd()` at `registry.go:125-161`. Calls `discover.ValidateGitHubURL()`, checks for existing entry, sets `AutoRegistered: false`, saves.

### AC3: `registry remove` removes entry, does NOT remove tools (R13), prints tools from source, handles non-existent gracefully
**Status: Implemented.** `runRegistryRemove()` at `registry.go:163-195` with `printToolsFromSource()` at `registry.go:200-229`. Non-existent handled at line 173-176.

### AC4: `registry` with no subcommand prints help
**Status: Implemented.** `registryCmd.Run` at `registry.go:31-33` calls `cmd.Help()`.

### AC5: Consistent exit codes and error formatting
**Status: Implemented.** Uses `exitWithCode(ExitUsage)` for validation errors, `exitWithCode(ExitGeneral)` for runtime errors, matching codebase conventions.

### AC6: Clean output when no registries configured
**Status: Implemented.** `runRegistryList()` at `registry.go:88-94` prints "No registries configured." and shows strict_registries if enabled.

## Findings

### 1. Divergent validation twin (Blocking)

`registry_test.go:229-251` defines a local `validateRegistrySource()` that reimplements validation logic instead of calling `discover.ValidateGitHubURL()`. The test comment says it "mirrors the validation in runRegistryAdd" -- but the implementations differ in material ways:

- The test function rejects `@` anywhere in the string. `ValidateGitHubURL` only rejects credentials in the URL-parsing branch (when `://` is present); in the owner/repo branch it delegates to `validateOwnerRepo` which uses a regex allowing only `[a-zA-Z0-9._-]`, which also rejects `@` but for a different reason and with a different error type.
- The test function allows owner names starting with `.` or `_`. `ValidateGitHubURL` -> `validateOwnerRepo` -> `ownerRepoRegex` requires the first character to be `[a-zA-Z0-9]`, so `.hidden/repo` passes the test helper but fails the real validator.
- The test for `"a/b/c"` (triple path) passes the test helper (rejects via `SplitN` limit 3 producing 3 parts) but `ValidateGitHubURL` -> `validateOwnerRepo` also uses `SplitN(path, "/", 3)` and only checks `parts[0]` and `parts[1]`, so `"a/b/c"` actually *passes* `ValidateGitHubURL`. The test asserts `wantErr: true` but never calls the real function.

The next developer looking at `TestRegistryAdd_ValidatesFormat` will believe those inputs are rejected at the command level. Some of them are not. The test is not exercising the actual code path. Either call `discover.ValidateGitHubURL` directly in the test or delete the duplicate helper.

### 2. Test name lie: TestRegistryList_NoRegistries (Advisory)

`registry_test.go:24-31` is named `TestRegistryList_NoRegistries` but it only checks `DefaultConfig()` has zero registries. It never calls `runRegistryList` or captures output. The next developer will think list output for empty registries is tested. The actual output format ("No registries configured." and conditional strict_registries line) has no test coverage. Rename to `TestDefaultConfig_NoRegistries` or extend to actually test the list command output.

### 3. printToolsFromSource silently swallows errors (Advisory)

`registry.go:200-229`: Both `DefaultConfig()` and `mgr.GetState().Load()` errors are silently swallowed with bare `return`. This is fine for "best-effort" as the comment says, but if `state.json` is corrupted, the user gets no indication that the tool-listing was skipped. A debug-level log or a brief "could not list installed tools" would prevent a debugging detour when someone wonders why no tools were listed after removing a registry that definitely had tools.

### 4. `a/b/c` passes ValidateGitHubURL but test claims rejection (Blocking)

This is a consequence of finding 1 but worth calling out explicitly for Issue 7 (install flow). `ValidateGitHubURL("a/b/c")` takes the owner/repo path, calls `validateOwnerRepo("a/b/c")`, which does `SplitN("a/b/c", "/", 3)` -> `["a", "b", "c"]`, checks `len(parts) < 2` (false, len is 3), then validates `parts[0]="a"` and `parts[1]="b"` -- both pass. The third segment `"c"` is silently ignored. This means `tsuku registry add a/b/c` would succeed and register `"a/b/c"` as a registry name, but the URL would be `https://github.com/a/b/c` which is not an owner/repo URL. Issue 5's HTTP fetching will hit this path. Either reject inputs with more than one slash or strip trailing segments.
