# Pragmatic Review: Self-Update Implementation

## Files Reviewed

- `internal/updates/self.go`
- `internal/updates/self_test.go`
- `cmd/tsuku/cmd_self_update.go`
- `cmd/tsuku/outdated.go`

## Findings

### 1. Duplicated logic between CheckAndApplySelf and cmd_self_update.go -- Advisory

**Location**: `internal/updates/self.go:65-148` vs `cmd/tsuku/cmd_self_update.go:22-91`

Both paths independently:
- Call `IsDevBuild(current)`
- Create `version.NewGitHubProvider(res, SelfRepo)`
- Call `provider.ResolveLatest(ctx)`
- Normalize versions with `strings.TrimPrefix`
- Compare with `CompareSemver`
- Acquire file lock at `SelfUpdateLockFile`
- Resolve exe path with `os.Executable()` + `filepath.EvalSymlinks()`
- Build asset name with `runtime.GOOS`/`runtime.GOARCH`
- Call `ApplySelfUpdate`

This is nearly identical logic in two places. The command adds user-facing output and error propagation (appropriate), but the "resolve, compare, lock, find exe, apply" sequence is duplicated verbatim.

**Suggested fix**: Extract a shared struct/function that performs "resolve version, compare, acquire lock, find exe path, compute asset name" and returns either a ready-to-apply descriptor or a reason to skip. Both callers use that, adding their own output and error handling. Alternatively, have `cmd_self_update.go` call `CheckAndApplySelf` with a force flag (since it already does the same thing minus the config gating). Advisory because both paths are correct today, but a version-comparison bug would need fixing in two places.

### 2. IsSelfUpdate is a trivial one-liner with limited callers -- Advisory

**Location**: `internal/updates/self.go:41-43`

```go
func IsSelfUpdate(entry *UpdateCheckEntry) bool {
    return entry.Tool == SelfToolName
}
```

Called in `apply.go` and tested in `self_test.go`. A `== SelfToolName` comparison is self-documenting. The function adds no naming clarity over the inline check. That said, it's 3 lines and bounded -- not blocking.

**Suggested fix**: Could inline, but low priority.

### 3. IsDevBuild treats all pre-release semver as dev builds -- Advisory

**Location**: `internal/updates/self.go:48-60`

The function marks any version with a hyphen after stripping `v` as a dev build. This means legitimate pre-release tags like `v1.0.0-rc.1` or `v1.0.0-beta.1` would be treated as dev builds and skip self-update entirely. If tsuku ever publishes a pre-release via goreleaser (which the release process supports -- tags with hyphens are marked as pre-releases), users on that version can't self-update.

This may be intentional (pre-release users opt out of auto-update), but it's undocumented and untested for the `rc`/`beta` case specifically. The test in `helpers_test.go` only covers `dev-*` and pseudo-versions.

**Suggested fix**: Add a comment documenting this is intentional, or narrow the check to only match pseudo-version patterns (timestamps after hyphen) and `dev-*` prefixes.

### 4. No test for IsDevBuild in self_test.go -- Out of scope

Tests exist in `cmd/tsuku/helpers_test.go` but not in the package where the function lives. This is a test coverage concern, deferring to tester.

### 5. CompareSemver silently swallows parse errors -- Advisory

**Location**: `internal/updates/self.go:296`

`strconv.Atoi` errors are discarded, treating non-numeric segments as 0. If a version string is malformed (e.g., `1.2.beta`), `beta` becomes `0`. This is fine for the current callers (versions come from GitHub releases which are well-formed), but the silent coercion could mask bugs.

**Suggested fix**: Either validate inputs at the call site or return an error. Advisory because current callers guarantee valid input.

### 6. outdated.go self-update check uses cache only, no freshness check -- Not a finding

The outdated command reads from cache (written by background check). This is correct -- it avoids a blocking network call during `tsuku outdated`. Not over-engineered.

## Summary

One meaningful duplication (finding 1) that will cause maintenance pain if the update flow changes. The rest are advisory or out of scope. The core implementation (`ApplySelfUpdate`) is well-structured: download, verify checksum, atomic rename with backup. No over-engineering there.

**Blocking findings**: 0
**Advisory findings**: 4 (duplication, trivial helper, pre-release handling, silent parse errors)
