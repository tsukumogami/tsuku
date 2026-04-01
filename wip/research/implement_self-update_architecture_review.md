# Architecture Review: Self-Update Implementation

## Scope

Design: `docs/designs/DESIGN-self-update.md`
Implementation files reviewed:
- `internal/updates/self.go`
- `internal/updates/self_test.go`
- `internal/updates/checker.go`
- `internal/updates/apply.go`
- `internal/userconfig/userconfig.go`
- `internal/userconfig/userconfig_test.go`
- `cmd/tsuku/cmd_self_update.go`
- `cmd/tsuku/main.go`
- `cmd/tsuku/outdated.go`

---

## 1. Design vs Implementation: Solution Architecture

### 1.1 Function signature deviation: `checkAndApplySelf`

**Design specifies:**
```go
func checkAndApplySelf(ctx context.Context, cacheDir string, resolver *version.Resolver, exePath string, autoApply bool)
```

**Implementation:**
```go
func CheckAndApplySelf(ctx context.Context, cfg *config.Config, userCfg *userconfig.Config, cacheDir string, resolver *version.Resolver) error
```

Differences:
- **Exported** (`CheckAndApplySelf`) vs unexported (`checkAndApplySelf`). The design explicitly uses lowercase. The implementation exports it, but it's only called from `checker.go` within the same package. Exporting a function intended for single-package use widens the API surface without need.
- **`exePath` removed from parameters**: The implementation resolves `exePath` internally via `os.Executable()` + `filepath.EvalSymlinks()`. This is fine -- it centralizes path resolution so the caller doesn't need to worry about it.
- **`autoApply bool` replaced with `userCfg *userconfig.Config`**: The design passes a pre-computed boolean; the implementation pulls `userCfg` and calls both `UpdatesSelfUpdate()` (line 66) and `UpdatesAutoApplyEnabled()` (line 105). This is a **double gate** -- the design says the caller gates on `UpdatesSelfUpdate()` and the function itself handles auto-apply. The implementation gates on `UpdatesSelfUpdate()` at entry AND `UpdatesAutoApplyEnabled()` at the apply step. The second gate (`UpdatesAutoApplyEnabled`) means auto-apply for self-updates can be disabled by the general `auto_apply` config, which the design doesn't mention. This might be intentional but diverges from the design's stated behavior.
- **`cfg *config.Config` added**: Needed for `cfg.HomeDir` to compute `noticesDir`. Pragmatic addition.
- **Returns `error`**: Design shows no return type. Implementation returns error, which is the Go convention.

### 1.2 Function signature deviation: `applySelfUpdate`

**Design specifies:**
```go
func applySelfUpdate(ctx context.Context, exePath string, tag string, assetName string) error
```

**Implementation:**
```go
func ApplySelfUpdate(ctx context.Context, exePath, tag, assetName string) error
```

Exported vs unexported. `ApplySelfUpdate` is called from `cmd/tsuku/cmd_self_update.go`, which is a different package, so exporting is **required**. The design's lowercase `applySelfUpdate` would not compile for the manual command use case. The design's Solution Architecture section says "Used by both the background checker and the manual self-update command" -- cross-package usage requires export.

### 1.3 `checkAndApplySelf` gating logic

**Design (D3, Decision Outcome):**
> The caller (`RunUpdateCheck`) gates this on `userCfg.UpdatesSelfUpdate()`.

**Implementation in `checker.go:88`:**
```go
if err := CheckAndApplySelf(ctx, cfg, userCfg, cacheDir, res); err != nil {
```
No gating at the call site. The gate moved inside `CheckAndApplySelf` itself (line 66). The caller just calls it unconditionally.

This means the gating is correct in effect but the responsibility shifted. Not a structural problem -- the function is self-contained.

### 1.4 Notice formatting for self-updates

**Design (D3):**
> The notice is formatted using `IsSelfUpdate()` to produce "tsuku has been updated from v0.5.0 to v0.6.0" rather than the standard tool update format.

**Implementation in `apply.go:130-141`:**
`displayUnshownNotices` does NOT check `IsSelfUpdate()`. It formats ALL notices identically:
```go
fmt.Fprintf(os.Stderr, "\nUpdate failed: %s -> %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details, 'tsuku rollback %s' to revert.\n", n.Tool)
```

Two problems:
1. Self-update success notices have `Error: ""`, but this display function only formats failure notices ("Update failed:"). A self-update success notice would render as "Update failed: tsuku -> 0.6.0: " with an empty error string.
2. The rollback suggestion ("tsuku rollback tsuku") doesn't make sense for self-updates. The `.old` backup is the rollback mechanism, not `tsuku rollback`.

The self-update success notice written at `self.go:139-148` will be picked up by `displayUnshownNotices` on the next `MaybeAutoApply` call, but formatted as a failure with empty error text. This is a functional bug with architectural implications -- the notice system doesn't distinguish success from failure notices for self-updates.

### 1.5 Background check flow: notice display timing

**Design data flow:**
> Next tsuku invocation (PersistentPreRun) -> displayUnshownNotices (existing flow)

**Implementation:**
`displayUnshownNotices` is called at the end of `MaybeAutoApply` (`apply.go:111`), which runs in `PersistentPreRun` (`main.go:76`). However, the self-update notice is written by the background `check-updates` process, not the foreground process. The foreground's `MaybeAutoApply` calls `displayUnshownNotices`, which reads the notices directory. This flow is correct -- the background process writes, the next foreground invocation reads.

But `displayUnshownNotices` is only called inside `MaybeAutoApply`, which returns early if `UpdatesAutoApplyEnabled()` is false. A user who has `auto_apply = false` but `self_update = true` would never see self-update notices because `MaybeAutoApply` bails before reaching the display call. The design says the display happens in `PersistentPreRun` generically, but the implementation couples it to `MaybeAutoApply`'s auto-apply gate.

---

## 2. Decision Compliance

### D1: Release asset resolution and download

Fully compliant. The implementation:
- Constructs asset name from `runtime.GOOS`/`runtime.GOARCH` (self.go:130)
- Constructs download URLs from known convention (self.go:154-155)
- Downloads and parses `checksums.txt` with validation (self.go:159-183)
- Verifies SHA256 before replacement (self.go:214-218)
- Uses `httputil.NewSecureClient` (self.go:157) rather than the design's `httputil.NewClient` -- this is likely the correct current API name

Missing from design: the `io.LimitReader(checksumsResp.Body, 1<<20)` (1MB limit on checksums.txt). This is a good defensive addition not in the design.

### D2: Binary replacement strategy

Fully compliant. The 9-step sequence from the design matches the implementation:
1. `os.Executable()` + `filepath.EvalSymlinks()` (self.go:121-128)
2. `os.Stat(exePath)` for permissions (self.go:221-224)
3. `os.CreateTemp(filepath.Dir(exePath), ...)` (self.go:199)
4. Write downloaded binary (self.go:207)
5. `os.Chmod(tmp, info.Mode())` (self.go:226-228)
6. `os.Remove(exePath + ".old")` (self.go:232)
7. `os.Rename(exePath, exePath + ".old")` (self.go:233-236)
8. `os.Rename(tmpPath, exePath)` (self.go:237-242)
9. Rollback on step 8 failure (self.go:239)

Note: In `CheckAndApplySelf`, steps 1-2 (path resolution) happen before `ApplySelfUpdate` is called, and the resolved `exePath` is passed in. In the manual command, the same resolution happens in `cmd_self_update.go:73-79`. This is correct -- the design says `ApplySelfUpdate` receives the resolved path.

### D3: Update cache integration

Mostly compliant with deviations noted above (notice formatting, display timing).

The `SelfToolName` constant and `MaybeAutoApply` skip guard are implemented exactly as designed. The cache entry is written with `Tool: SelfToolName`, `ActiveVersion`, `LatestOverall`, matching the design.

**Deviation**: The design says to write the cache entry with `LatestOverall` set even when versions are equal. The implementation does this (self.go:87-96 runs before the comparison at line 99). Compliant.

**Deviation**: The design says the lock file is `$TSUKU_HOME/cache/updates/.self-update.lock`. Implementation uses `filepath.Join(cacheDir, SelfUpdateLockFile)` where `cacheDir = CacheDir(cfg.HomeDir)`. Assuming `CacheDir` returns `$TSUKU_HOME/cache/updates/`, this is compliant.

---

## 3. Phasing Compliance

**Phase 1 deliverables** (design):
- `internal/updates/self.go` -- delivered
- `internal/updates/self_test.go` -- delivered
- `internal/updates/checker.go` modification -- delivered
- `internal/updates/apply.go` modification -- delivered
- `internal/userconfig/userconfig.go` modification -- delivered

**Phase 2 deliverables** (design):
- `cmd/tsuku/cmd_self_update.go` -- delivered
- `cmd/tsuku/cmd_self_update_test.go` -- NOT delivered (no test file found)
- `cmd/tsuku/main.go` modification -- delivered

**Phase 3 deliverables** (design):
- `cmd/tsuku/outdated.go` modification -- delivered

All three phases appear in the same changeset, which is fine -- the phasing is an implementation ordering guide, not a delivery boundary. Phase 2 correctly depends on Phase 1's `ApplySelfUpdate`.

**Missing**: `cmd/tsuku/cmd_self_update_test.go` listed in Phase 2 deliverables is absent.

---

## 4. Structural Issues

### 4.1 `IsDevBuild` pre-release handling

`IsDevBuild` at self.go:48-60 rejects any version with a hyphen after the semver core. This means legitimate pre-release tags like `v1.0.0-rc.1` are treated as dev builds and skip self-update. The design says "Dev builds return 'dev-...' which won't match any release" but doesn't address pre-releases. GoReleaser marks hyphenated tags as pre-releases, so this behavior is probably correct (don't auto-update to or from pre-releases), but it should be documented.

### 4.2 `CompareSemver` ignores pre-release semantics

`CompareSemver` (self.go:284-309) only compares numeric dot-separated segments. It would incorrectly compare "1.0.0-rc.1" by ignoring everything after the hyphen (since `strconv.Atoi("0-rc")` returns 0 with an error that's silently dropped). This is safe because `IsDevBuild` filters pre-releases before `CompareSemver` is called. But `CompareSemver` is exported and could be called from other contexts where this assumption doesn't hold.

### 4.3 `CheckAndApplySelf` double gate on auto-apply

The function checks `userCfg.UpdatesSelfUpdate()` at entry (line 66) and `userCfg.UpdatesAutoApplyEnabled()` before applying (line 105). This means:
- `self_update = true, auto_apply = false` -> check runs, cache entry written, but no apply
- `self_update = false` -> nothing happens, not even cache entry

The design says `checkAndApplySelf` writes the cache entry regardless and gates apply on "auto-apply is enabled (not CI, not suppressed by env var)". The implementation's second gate on `UpdatesAutoApplyEnabled()` adds coupling between the general auto-apply setting and the self-update feature that the design doesn't describe. A user who sets `auto_apply = false` (to prevent tool auto-updates) but keeps `self_update = true` (to allow tsuku auto-updates) will get no self-update.

### 4.4 Package placement is correct

All self-update logic lives in `internal/updates/`, consistent with the existing update infrastructure. The manual command lives in `cmd/tsuku/`, consistent with other commands. No dependency direction violations -- `internal/updates/` doesn't import `cmd/`, and `cmd/` imports `internal/updates/`.

### 4.5 Command registration

`selfUpdateCmd` is registered via `init()` in `cmd_self_update.go:94-96`, calling `rootCmd.AddCommand(selfUpdateCmd)`. This is **also** where other commands are registered in `main.go:136-167`. The `selfUpdateCmd` registration happens in its own `init()`, which differs from the pattern where all other commands are registered in `main.go`'s `init()`. While Go's `init()` ordering within a package is by filename, this creates a subtle inconsistency: `selfUpdateCmd` is the only command registered in its own file's `init()` rather than in `main.go`.

Looking more carefully at `main.go`, I see the `selfUpdateCmd` is NOT in the explicit `rootCmd.AddCommand(...)` block at lines 136-167. It registers itself via its own `init()`. This means there are two registration patterns coexisting.

### 4.6 Skip list registration

`"self-update"` is correctly added to the PersistentPreRun skip list in `main.go:69`, preventing the background check from spawning during manual self-update.

---

## 5. Summary of Findings

### Blocking

1. **Notice display is broken for self-update success** (`apply.go:130-141`). `displayUnshownNotices` formats all notices as failures ("Update failed:") and suggests `tsuku rollback`, which doesn't apply to self-updates. Self-update writes a success notice with empty error. The design explicitly says `IsSelfUpdate()` should be used to format self-update notices differently. Either modify `displayUnshownNotices` to handle self-update notices, or add a separate display path in `PersistentPreRun`.

2. **Notice display gated behind `UpdatesAutoApplyEnabled`** (`apply.go:29,111`). `displayUnshownNotices` only runs inside `MaybeAutoApply`, which bails at line 29 if auto-apply is disabled. Users with `auto_apply=false, self_update=true` will never see self-update notifications. The design says notification display is in `PersistentPreRun`, not gated by auto-apply.

3. **Double gate on auto-apply creates undocumented interaction** (`self.go:105`). `CheckAndApplySelf` checks `UpdatesAutoApplyEnabled()` in addition to `UpdatesSelfUpdate()`. The design says `self_update` is the control for self-updates and doesn't mention dependency on the general `auto_apply` flag. A user who disables `auto_apply` (to prevent tool auto-updates) but enables `self_update` won't get self-updates. Either remove the `UpdatesAutoApplyEnabled` gate or document this as intentional.

### Advisory

4. **`CheckAndApplySelf` is exported but only called within-package** (`self.go:65`). The design uses lowercase. Unless there's a planned external caller, keep it unexported to limit API surface. No structural damage if left as-is since it's in `internal/`.

5. **`CompareSemver` is exported with incomplete semantics** (`self.go:284`). It silently drops non-numeric segments. Safe today because `IsDevBuild` filters pre-releases, but future callers might not know this constraint. Consider documenting the limitation in the function's doc comment.

6. **Command registration pattern inconsistency** (`cmd_self_update.go:94-96`). `selfUpdateCmd` registers itself via its own `init()` while all other commands are registered in `main.go`'s `init()` block. This creates a parallel registration pattern. Move the `AddCommand` call to `main.go`'s command list for consistency.

7. **Missing `cmd_self_update_test.go`**. Phase 2 deliverables list this file. Not present in the changeset.
