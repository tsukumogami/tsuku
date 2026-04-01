# Self-Update Implementation: Maintainability Review

Reviewer perspective: can the next developer understand and change this code with confidence?

## Finding 1: Divergent `IsDevBuild` twins (Blocking)

**Files:** `internal/updates/self.go:48` (`IsDevBuild`) and `cmd/tsuku/helpers.go:85` (`isDevBuild`)

Two functions with the same name and stated purpose produce **different results for pre-release versions**:

- `helpers.go:isDevBuild("v1.0.0-rc.1")` returns **false** -- it only matches `"dev"`, `"unknown"`, or `"dev-*"` prefix.
- `updates/self.go:IsDevBuild("v1.0.0-rc.1")` returns **true** -- any version with a hyphen after stripping "v" is treated as a dev build.

The `cmd_self_update.go` command calls `updates.IsDevBuild`, which means `tsuku self-update` will refuse to run on any pre-release version (e.g., `v1.0.0-rc.1`). Meanwhile, the deprecation warning formatter in helpers.go uses the narrower `isDevBuild` and will treat `v1.0.0-rc.1` as a release build.

The next developer will see two functions named `isDevBuild`/`IsDevBuild`, assume they're equivalent, and use whichever is in scope. The behavioral difference is invisible without reading both implementations line-by-line.

**Recommendation:** Consolidate into one function. If the broader definition in `updates.IsDevBuild` is intentional (treating pre-releases and pseudo-versions as dev builds), update the helpers.go copy and its tests. If not, the `updates.IsDevBuild` hyphen check is too aggressive and will block self-update for legitimate pre-release tags.

The test in `helpers_test.go:380` explicitly asserts `isDevBuild("v1.0.0-rc.1") == false`, while the comment on `updates.IsDevBuild` says "Any version with a hyphen followed by digits is a pseudo-version or pre-release" -- these are contradictory design decisions hiding behind the same name.

## Finding 2: Duplicate semver comparison functions (Advisory)

**Files:** `internal/updates/self.go:284` (`CompareSemver`) and `internal/version/version_utils.go:61` (`CompareVersions`)

`CompareSemver` is a simple numeric-segment comparator that ignores pre-release suffixes entirely. `CompareVersions` in the version package handles normalization, pre-release ordering, and various tag formats. The self-update code uses the simpler one.

This is acceptable for now because `IsDevBuild` gates pre-release versions before `CompareSemver` is ever called. But the coupling is invisible -- if someone changes the `IsDevBuild` logic to allow pre-releases through, `CompareSemver` will silently produce wrong results (it would `strconv.Atoi` a segment like `"0-rc"` and get 0, potentially comparing `1.0.0-rc.1` as equal to `1.0.0`).

**Recommendation:** Add a comment on `CompareSemver` stating it only handles clean semver (no pre-release suffixes) and that callers must filter pre-releases beforehand. Or use `version.CompareVersions` and drop the duplicate.

## Finding 3: `CheckAndApplySelf` name understates its side effects (Advisory)

**File:** `internal/updates/self.go:65`

The function name suggests "check and maybe apply." Its actual side effects:
1. Resolves latest version via network
2. Writes a cache entry (always, even when no update is needed)
3. Acquires a file lock
4. Downloads a binary from GitHub
5. Replaces the running executable on disk
6. Writes a notice file for the next CLI invocation to display

The doc comment is accurate and the function is well-structured internally. But the name `CheckAndApply` undersells steps 4-6. A next developer calling this from a new code path might not expect it to replace the binary they're currently running.

**Recommendation:** The name is borderline acceptable given the doc comment. Consider `CheckAndApplySelfUpdate` (already has `Self` suffix, just adding `Update`) or adding a one-line warning comment at the call site in the background check code.

## Finding 4: `latestNorm` normalization asymmetry in `cmd_self_update.go` (Advisory)

**File:** `cmd/tsuku/cmd_self_update.go:39-40`

```go
currentNorm := strings.TrimPrefix(current, "v")
latestNorm := latest.Version
```

`currentNorm` has its "v" prefix explicitly stripped. `latestNorm` is assigned directly from `latest.Version`, which happens to already be normalized (no "v" prefix) per `VersionInfo` contract. The asymmetry looks like a bug on first read -- the next developer will wonder why only one side strips "v".

In `self.go:83-84`, the same pattern appears but both sides use `TrimPrefix`, which is clearer (even though redundant for `latest.Version`):

```go
normalizedCurrent := strings.TrimPrefix(currentVersion, "v")
normalizedLatest := strings.TrimPrefix(latest.Version, "v")
```

**Recommendation:** Use `strings.TrimPrefix` on both sides in `cmd_self_update.go` for consistency, matching the pattern in `self.go`. The redundant call is cheap and makes the intent obvious.

## Finding 5: Silent error suppression in `CheckAndApplySelf` (Advisory)

**File:** `internal/updates/self.go:97, 133-134`

```go
_ = WriteEntry(cacheDir, entry)     // line 97
// ...
if applyErr := ApplySelfUpdate(...); applyErr != nil {
    log.Default().Debug("self-update apply failed", "error", applyErr)
    return nil // Best-effort, don't propagate
}
```

The cache write error is silently discarded. The apply error is logged at debug level and swallowed. This is intentional (best-effort background update), and the comment on line 134 explains the apply case. But the cache write on line 97 has no comment -- the next developer might add error handling thinking it was accidentally ignored.

**Recommendation:** Add `// Best-effort: don't fail the foreground command for cache writes` or similar.

## Finding 6: No test for `IsDevBuild` in `self_test.go` (Advisory)

**File:** `internal/updates/self_test.go`

The exported `IsDevBuild` function has no tests in this file. The only test for a dev-build checker is `TestIsDevBuild` in `cmd/tsuku/helpers_test.go`, which tests the *different* `isDevBuild` function. A developer looking at `self_test.go` to understand `IsDevBuild`'s behavior will find nothing, and the test in helpers_test.go will mislead them about its behavior (see Finding 1).

**Recommendation:** Add `TestIsDevBuild` to `self_test.go` covering the pre-release/pseudo-version cases that differ from the helpers version.

## Finding 7: `ApplySelfUpdate` doesn't clean up `.old` backup on success (Nit)

**File:** `internal/updates/self.go:232-243`

After a successful rename, the `.old` backup file is left on disk. It's cleaned up on the *next* update attempt (line 232: `_ = os.Remove(exePath + ".old")`). This is fine for crash safety but means users will always have a stale binary sitting around. Not a maintainability issue per se, but a developer adding cleanup logic later needs to know this is intentional.

## Overall Assessment

The code is well-structured. `ApplySelfUpdate` is a clean download-verify-replace pipeline with proper rollback. Error messages are specific and actionable. The file organization is logical (pure functions in `self.go`, command wiring in `cmd_self_update.go`, cache integration in `outdated.go`).

The blocking issue is the divergent `IsDevBuild`/`isDevBuild` twins. Everything else is advisory polish.
