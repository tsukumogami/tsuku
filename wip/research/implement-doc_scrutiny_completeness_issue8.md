# Scrutiny Review: Completeness -- Issue 8

**Issue:** #8 feat(cli): add source-directed loading to update, outdated, and verify
**Focus:** completeness
**Files changed:** cmd/tsuku/update.go, cmd/tsuku/outdated.go, cmd/tsuku/verify.go, cmd/tsuku/source_directed_test.go, internal/recipe/loader.go

## Independent Assessment of the Diff

The implementation adds source-directed recipe loading to three commands:

1. **update.go**: Adds `ensureSourceProvider()` call before the install flow. This registers a distributed provider in the loader chain if the tool's state has a distributed source. The actual recipe resolution still goes through `loader.Get()` via `runInstallWithTelemetry`/`runDryRun`. Also adds `ensureSourceProvider()` and `isDistributedSource()` helper functions.

2. **outdated.go**: Loads state, then calls `loadRecipeForTool()` for each installed tool. `loadRecipeForTool` checks state for a distributed source, calls `addDistributedProvider` + `loader.GetFromSource()` if distributed, falls back to `loader.Get()` on error or for non-distributed sources.

3. **verify.go**: Replaces `loader.Get()` with `loadRecipeForTool()`, using the existing `state` and `cfg` variables already in scope.

4. **loader.go**: Adds `ParseAndCache()` method that parses raw TOML bytes and caches the result.

5. **source_directed_test.go**: 12 test functions covering `isDistributedSource`, `loadRecipeForTool` with central/empty/distributed/unreachable/nil-state/not-in-state scenarios, `ensureSourceProvider` with empty/central/not-installed scenarios, and `ParseAndCache` with valid and invalid TOML.

## Requirements Mapping Evaluation

--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
1. ac: "update reads ToolState.Source and uses GetFromSource", status: "implemented", evidence: "update.go:ensureSourceProvider()"
2. ac: "outdated iterates installed tools checking each against recorded source", status: "implemented", evidence: "outdated.go:loadRecipeForTool()"
3. ac: "verify uses cached recipe from recorded source", status: "implemented", evidence: "verify.go calls loadRecipeForTool()"
4. ac: "Unit tests for each command covering central, embedded, and distributed source paths", status: "implemented", evidence: "source_directed_test.go: 12 test functions"
5. ac: "Unit tests for fallback behavior when source is empty", status: "implemented", evidence: "TestLoadRecipeForTool_EmptySourceDefaultsToCentral, TestEnsureSourceProvider_EmptySource"
6. ac: "No changes to CLI output format or exit codes", status: "implemented", evidence: "All changes are internal routing; same functions produce output"
--- END UNTRUSTED REQUIREMENTS MAPPING ---

### AC 1: update reads ToolState.Source, calls GetFromSource

**Claimed status:** implemented
**Claimed evidence:** update.go:ensureSourceProvider()
**Assessment:** ADVISORY -- partial implementation diverges from AC text

The AC says: "reads `ToolState.Source`, calls `GetFromSource` for fresh recipe from the recorded source." The implementation reads `ToolState.Source` (via `ensureSourceProvider`), but does NOT call `GetFromSource`. Instead, it registers the distributed provider into the loader chain and lets `runInstallWithTelemetry` -> `installWithDependencies` -> `loader.Get()` resolve the recipe through the normal chain walk.

This is functionally equivalent for the happy path: the distributed provider is in the chain, and `loader.Get()` will find the recipe from it. However, it differs from `outdated` and `verify` which both use `loadRecipeForTool` -> `GetFromSource` for targeted source-directed loading. The AC explicitly says "calls `GetFromSource`" which `update` does not do.

The pragmatic reason this works is that `update` delegates to the install flow which already handles recipe resolution. The approach avoids duplicating install logic just to use `GetFromSource`. This is a reasonable implementation choice but the evidence claim ("update.go:ensureSourceProvider()") understates the divergence from the AC text.

**Severity:** advisory

### AC 2: outdated iterates installed tools checking each against recorded source

**Claimed status:** implemented
**Claimed evidence:** outdated.go:loadRecipeForTool()
**Assessment:** CONFIRMED

The diff shows `outdated.go` loads state, then for each tool calls `loadRecipeForTool(ctx, tool.Name, state, cfg)`. The `loadRecipeForTool` function checks state for the source, defaults empty source to central (falls back to `loader.Get`), and for distributed sources calls `GetFromSource` with fallback. Unreachable sources produce warnings via `fmt.Fprintf(os.Stderr, "Warning: ...")` and fall back to the chain rather than failing fatally. All aspects of the AC are satisfied.

**Severity:** n/a (pass)

### AC 3: verify uses cached recipe from recorded source

**Claimed status:** implemented
**Claimed evidence:** verify.go calls loadRecipeForTool()
**Assessment:** CONFIRMED

The diff shows `verify.go` replaces `loader.Get(name, recipe.LoaderOptions{})` with `loadRecipeForTool(context.Background(), name, state, cfg)`. The `state` and `cfg` variables are already in scope from earlier in the function. The AC says "cached recipe from the recorded source" -- `loadRecipeForTool` calls `GetFromSource` which bypasses the loader's in-memory cache, then calls `ParseAndCache` which puts the result into the cache. This satisfies the AC.

**Severity:** n/a (pass)

### AC 4: Unit tests for each command covering central, embedded, and distributed source paths

**Claimed status:** implemented
**Claimed evidence:** source_directed_test.go: 12 test functions
**Assessment:** ADVISORY -- embedded path not tested

The tests cover central source (`TestLoadRecipeForTool_CentralSource`), distributed source (`TestLoadRecipeForTool_DistributedSource`), unreachable distributed (`TestLoadRecipeForTool_UnreachableDistributedFallsBack`), and various edge cases. However, there is no test for the embedded source path specifically. The AC explicitly lists "central, embedded, and distributed source paths."

The `loadRecipeForTool` function treats embedded the same as central (both fall through to `loader.Get`), so a test with `Source: "embedded"` would exercise the same code path as central. This makes it a minor gap since the code path is covered, but the AC explicitly asks for embedded coverage.

Additionally, the tests exercise `loadRecipeForTool` and `ensureSourceProvider` which are shared helpers, not command-specific tests. The AC says "for each command" -- there are no tests that exercise the `update`, `outdated`, or `verify` commands themselves. The tests validate the helper functions that these commands use, which is a reasonable approach but doesn't match the AC text literally.

**Severity:** advisory

### AC 5: Unit tests for fallback behavior when source is empty

**Claimed status:** implemented
**Claimed evidence:** TestLoadRecipeForTool_EmptySourceDefaultsToCentral, TestEnsureSourceProvider_EmptySource
**Assessment:** CONFIRMED

Both cited test functions exist in the diff. `TestLoadRecipeForTool_EmptySourceDefaultsToCentral` sets up a state with `Source: ""` and verifies the recipe loads from the central provider. `TestEnsureSourceProvider_EmptySource` verifies it's a no-op for empty source. These adequately test the migration path.

**Severity:** n/a (pass)

### AC 6: No changes to CLI output format or exit codes

**Claimed status:** implemented
**Claimed evidence:** All changes are internal routing; same functions produce output
**Assessment:** CONFIRMED

The diff shows only internal routing changes. `outdated.go` still calls the same output formatting code. `verify.go` still uses the same error/exit code paths. `update.go` still calls `runInstallWithTelemetry`/`runDryRun`. No output format or exit code constants were changed. The only new output is warning messages to stderr for unreachable distributed sources, which don't affect the normal output format.

**Severity:** n/a (pass)

### Missing/Phantom AC Check

All six ACs from the issue body are present in the mapping. No phantom ACs detected.

## Summary

| # | AC | Status | Severity |
|---|---|---|---|
| 1 | update reads ToolState.Source and uses GetFromSource | Advisory | `update` registers provider in chain rather than calling `GetFromSource` directly |
| 2 | outdated iterates installed tools checking each against recorded source | Pass | Confirmed in diff |
| 3 | verify uses cached recipe from recorded source | Pass | Confirmed in diff |
| 4 | Unit tests for each command covering central, embedded, and distributed | Advisory | Embedded path not explicitly tested; tests cover helpers not commands |
| 5 | Unit tests for fallback when source is empty | Pass | Confirmed in diff |
| 6 | No changes to CLI output format or exit codes | Pass | Confirmed in diff |

**Blocking findings:** 0
**Advisory findings:** 2
