# Completeness Scrutiny: Issue 7

**Issue:** feat(install): integrate distributed sources into install flow
**Reviewed files:**
- `cmd/tsuku/install.go`
- `cmd/tsuku/install_distributed.go`
- `cmd/tsuku/install_distributed_test.go`
- `internal/install/state.go`
- `internal/recipe/loader.go`

---

## Name Parsing

### AC: Detect `/` in tool name to identify distributed source requests
**PASS**

`parseDistributedName()` returns nil when the name doesn't contain `/` (line 36-38 of `install_distributed.go`). The install command's main loop calls `parseDistributedName(arg)` and branches into the distributed path only when non-nil (line 188 of `install.go`).

Test coverage: `TestParseDistributedName` includes "bare tool name" and "tool with version" cases that verify nil return for non-distributed names.

### AC: Parse formats: `owner/repo`, `owner/repo:recipe`, `owner/repo@version`, `owner/repo:recipe@version`
**PASS**

All four formats are handled in `parseDistributedName()`:
- `owner/repo` -> source=owner/repo, recipe defaults to repo name (lines 57-62)
- `owner/repo:recipe` -> colon split at line 51
- `owner/repo@version` -> `@` split at line 43, then repo-name default
- `owner/repo:recipe@version` -> `@` split first, then colon split

Test coverage: `TestParseDistributedName` has explicit test cases for all four formats plus edge cases (hyphens in names, version with dots).

### AC: Reuse `ValidateGitHubURL()` for owner/repo validation
**PASS**

`ensureDistributedSource()` calls `validateRegistrySource()` (line 84 of `install_distributed.go`), which in turn calls `discover.ValidateGitHubURL()` (line 130 of `registry.go`).

Test coverage: `TestEnsureDistributedSource_InvalidSource` tests invalid patterns (empty, no repo, path traversal, triple path). `TestEnsureDistributedSource_AlreadyRegistered` tests that valid sources pass validation.

---

## Trust and Registration

### AC: Unregistered source with `strict_registries = false`: show confirmation prompt, `-y` skips it, auto-register on confirmation
**PASS**

`ensureDistributedSource()` implements the full flow:
1. Checks registration status (line 101)
2. Checks `StrictRegistries` (line 106)
3. If not strict and not `autoApprove`, calls `confirmWithUser()` (line 117)
4. On confirmation, calls `autoRegisterSource()` (line 123) which sets `AutoRegistered: true`

The `autoApprove` parameter is fed from `installYes || installForce` (line 197 of `install.go`), so both `-y` and `--force` skip the prompt.

Test coverage: `TestAutoRegisterSource` verifies auto-registration saves correctly with `AutoRegistered=true`. `TestInstallYesFlag` verifies the `--yes/-y` flag exists. No test for the full prompt-skip flow, but `confirmWithUser` returns false in non-interactive mode (CI), so `TestEnsureDistributedSource_AlreadyRegistered` exercises the registered path. The prompt logic is simple branching with adequate unit-level coverage of the components.

### AC: Unregistered source with `strict_registries = true`: error suggesting `tsuku registry add`
**PASS**

Lines 106-112 of `install_distributed.go`: when `StrictRegistries` is true, returns an error containing the suggestion string `"tsuku registry add"`.

Test coverage: **ADVISORY** -- No dedicated test for the strict mode error path. The logic is a simple conditional that returns a formatted error, so the risk is low, but a test asserting the error message contains `"tsuku registry add"` would add confidence.

### AC: Already-registered sources skip the confirmation prompt
**PASS**

Lines 99-103: if the source exists in `userCfg.Registries`, the function calls `addDistributedProvider()` and returns immediately without prompting.

Test coverage: `TestEnsureDistributedSource_AlreadyRegistered` validates the source format check succeeds. Due to architecture constraints (mocking `userconfig.Load()`), the full registered-skip path isn't end-to-end tested, but the logic path is clear.

---

## Source Collision Detection

### AC: Same-name tool from different source: prompt before replacing
**PASS**

`checkSourceCollision()` (lines 178-213 of `install_distributed.go`):
1. Loads existing tool state
2. Compares `existingSource` with `newSource`
3. If different and not force, calls `confirmWithUser()` (line 208)

Test coverage: `TestCheckSourceCollision_DifferentSourceNoForce` verifies that in non-interactive mode (CI), different-source collision returns an error.

### AC: `--force` flag skips the collision prompt; same-source reinstalls don't trigger it
**PASS**

- `force=true` returns nil at line 201
- Same source returns nil at line 197

Test coverage: `TestCheckSourceCollision_DifferentSourceWithForce` verifies force skips collision. `TestCheckSourceCollision_SameSource` verifies same-source doesn't trigger collision. `TestCheckSourceCollision_NotInstalled` verifies uninstalled tools don't trigger collision.

---

## State Recording

### AC: Record `Source: "owner/repo"` on `ToolState` after successful install
**PASS**

`recordDistributedSource()` (lines 217-228 of `install_distributed.go`) calls `UpdateTool()` and sets `ts.Source = source`. Called at line 243 of `install.go` after successful install.

The `ToolState` struct has a `Source` field (line 89 of `state.go`) with documentation of valid values.

Test coverage: `TestRecordDistributedSource` verifies Source is set and persisted in state.

### AC: Record `sha256(recipe_toml_bytes)` in state.json (audit trail)
**PASS**

`computeRecipeHash()` (lines 232-235 of `install_distributed.go`) uses `sha256.Sum256`. Recipe bytes are fetched via `fetchRecipeBytes()` before install (line 220 of `install.go`), and the hash is recorded in `recordDistributedSource()` via `ts.RecipeHash` (line 226 of `install_distributed.go`).

The `ToolState` struct has `RecipeHash string` field (line 93 of `state.go`).

Test coverage: `TestComputeRecipeHash` verifies SHA256 produces 64 hex chars and is deterministic. `TestRecordDistributedSource` verifies RecipeHash is persisted. `TestRecipeHashField_InState` verifies the hash survives a save/load cycle.

---

## Telemetry

### AC: Distributed installs use opaque `"distributed"` tag, not the full `owner/repo`
**PASS**

`distributedTelemetryTag()` returns the literal string `"distributed"` (line 246 of `install_distributed.go`). This value is passed as the `versionConstraint` parameter to `runInstallWithTelemetry()` (line 233 of `install.go`), which feeds into `telemetry.NewInstallEvent()` as the `versionConstraint` field. The `owner/repo` value never reaches the telemetry event.

Test coverage: `TestDistributedTelemetryTag` verifies the return value is exactly `"distributed"`.

---

## Summary

| Category | Criterion | Verdict |
|----------|-----------|---------|
| Name parsing | Detect `/` | PASS |
| Name parsing | 4 formats | PASS |
| Name parsing | Reuse ValidateGitHubURL | PASS |
| Trust | Non-strict: prompt, -y skip, auto-register | PASS |
| Trust | Strict: error with suggestion | PASS |
| Trust | Registered: skip prompt | PASS |
| Collision | Different source: prompt | PASS |
| Collision | --force + same-source | PASS |
| State | Record Source | PASS |
| State | Record RecipeHash | PASS |
| Telemetry | Opaque "distributed" tag | PASS |

**Overall: PASS**

One advisory note:

- **ADVISORY**: No unit test for the `strict_registries = true` error path. The logic is a two-line conditional returning a formatted error, so the risk is minimal. Adding a test would improve coverage but isn't blocking.
