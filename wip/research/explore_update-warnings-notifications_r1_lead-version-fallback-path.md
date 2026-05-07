# Lead: Version fallback code path

## Findings

### Version resolution for `github_archive` recipes

Version resolution for `github_archive` recipes follows this path:

1. **`Executor.resolveVersionWith`** (`internal/executor/executor.go:115`) creates a `version.Resolver` and builds a `VersionProvider` via `version.NewProviderFactory().ProviderFromRecipeForPipx()`.

2. **`version.ResolveWithinBoundary`** (`internal/version/resolve.go:17`) routes the call. For an empty constraint (install latest) or a semver pin, it calls `provider.ResolveLatest(ctx)` or filters a `ListVersions` result. For `GitHubProvider`, `ResolveLatest` delegates to `resolver.ResolveGitHub(ctx, repo)`.

3. **`Resolver.ResolveGitHub`** (`internal/version/resolver.go:129`) calls the GitHub API for the latest release tag: `client.Repositories.GetLatestRelease(ctx, owner, repoName)`. It falls back to `resolveFromTags` on a 404. It returns a `VersionInfo{Tag, Version}` and commits to that version.

4. The version is **committed** at the point `resolveVersionWith` returns. After that, `EvalContext.Version` and `EvalContext.VersionTag` are set and passed to all downstream actions. There is no retry loop or fallback at this level.

**Key point:** The version resolver queries the release list/tag list from the GitHub API. It does not check whether a release has downloadable assets. A release can exist in the API (GetLatestRelease returns 200) while having no uploadable assets, or while having assets with different names than the pattern.

### Where the 404 surfaces

The 404 for a missing asset surfaces in two distinct places depending on execution path:

**Path A: Plan generation (`Decompose`)**

`GitHubArchiveAction.Decompose` (`internal/actions/composites.go:538`) is called during plan generation. It calls `resolveAssetName`, which:
- If `asset_pattern` contains wildcards: calls `ctx.Resolver.FetchReleaseAssets(apiCtx, repo, ctx.VersionTag)` (`internal/version/assets.go:45`). This calls `client.Repositories.GetReleaseByTag(...)`. A release with no assets returns the error `"release '%s' for '%s' has no assets"` at line 152. A release where the tag doesn't exist returns `"release '%s' not found in '%s'. It may be a draft..."` at line 132–133. **This error propagates up through `Decompose` → `resolveStep` → `GeneratePlan` → the top-level install caller.**
- If `asset_pattern` has no wildcards: the URL is constructed directly at `composites.go:582` without any API check. The 404 surfaces later, at actual download time.

**Path B: Plan execution (actual download)**

`DownloadAction.downloadFile` (`internal/actions/download.go:346`) performs the HTTP download. On a non-200 response, `doDownloadFile` returns `&httpStatusError{StatusCode: resp.StatusCode, ...}` at line 432. A 404 is not a retryable status code (only 403, 429, 5xx retry), so the download fails immediately. This error wraps up through `Execute` → executor step loop → install manager.

**The divergence:** For wildcard patterns, the 404 is detected during Decompose (plan time). For static patterns, it surfaces during execution. Fallback logic needs to handle both points.

### Where fallback logic would be inserted

**Option 1: Version provider level (before version is committed)**

A fallback loop could be added to `GitHubProvider.ResolveLatest` or a new wrapper around `ResolveWithinBoundary`. This would:
- Call `ListVersions` to get the ordered candidate list.
- For each candidate (starting from latest): check if the release has assets matching the expected pattern before committing.
- Return the first version that has a valid asset.

This is the cleanest architectural point but requires `FetchReleaseAssets` to be called during version resolution (adds one API call per resolution attempt). It also requires the asset pattern to be available at version resolution time, which currently it isn't—the provider factory only sees the recipe's `[version]` section.

**Option 2: `GitHubArchiveAction.resolveAssetName` / `Decompose` level**

When `FetchReleaseAssets` returns "no assets" or `MatchAssetPattern` returns "no asset matched", the action could catch the error and retry with the previous version. The challenge is that `Decompose` only has `ctx.Version` / `ctx.VersionTag` as fixed fields in `EvalContext`—there's no way to re-run version resolution from inside a Decompose call without a new `Resolver` call and a version list lookup.

A concrete insertion point would be in `GitHubArchiveAction.Decompose` (`composites.go:538`), calling `resolveAssetName`, wrapping it with a retry loop:
```go
// pseudo-code
assetName, err := a.resolveAssetName(ctx, params, assetPattern, repo)
if isAssetMissingError(err) {
    versions, _ := ctx.Resolver.ListGitHubVersions(ctx.Context, repo)
    for _, prevVersion := range versions {
        if prevVersion == currentVersion { continue }
        ctx2 := ctxWithVersion(ctx, prevVersion)
        assetName, err = a.resolveAssetName(ctx2, params, assetPattern, repo)
        if err == nil { fallbackVersion = prevVersion; break }
    }
}
```

This option requires a mutable `EvalContext` or a local override mechanism. `EvalContext` is a struct (not a pointer receiver mutated in-place for these fields), so a local copy with the new version works.

**Option 3: Plan generation layer (`resolveStep` in executor)**

`plan_generator.go:resolveStep` calls `actions.DecomposeToPrimitives(evalCtx, ...)`. If the decomposition fails with an asset-missing error, the plan generator could detect that, decrement the version candidate, rebuild an `EvalContext` with the new version, and retry the decompose. This is the most invasive option and couples the executor to asset-existence semantics.

**Most natural insertion point:** Option 2 (inside `Decompose`) or a thin wrapper at the plan generator's `resolveStep` that intercepts asset errors. Option 2 has better locality but requires access to `ListGitHubVersions` from inside the `EvalContext`, which already has `ctx.Resolver` available. The information available at that point includes: tool name, repo (from params), current version tag, and the full resolver.

### Information available at the fallback point

At `GitHubArchiveAction.Decompose` call time, `EvalContext` carries:
- `ctx.Version` — resolved version string (e.g., "2.1.0")
- `ctx.VersionTag` — original tag (e.g., "v2.1.0")
- `ctx.Resolver` — `*version.Resolver` for API calls
- `ctx.Recipe` — the full recipe, including recipe name

This is sufficient to: (a) detect the missing-asset error, (b) list older versions via `ctx.Resolver.ListGitHubVersions`, and (c) emit a notification with tool name, skipped version, and fallback version.

### Comparison of explicit update path vs. auto-apply path at version resolution

Both paths converge on the same function: `version.ResolveWithinBoundary`. The differences are:

| Aspect | Explicit `tsuku update` | Background auto-apply |
|--------|------------------------|-----------------------|
| Entry point | `cmd/tsuku/update.go:runInstallWithExternalReporter` | `internal/updates/apply.go:applyUpdate` → `installFn(entry.Tool, entry.LatestWithinPin, entry.Requested)` |
| Version resolution | Happens inside `install` orchestration (executor path) at `GeneratePlan` time | **Already done** — `entry.LatestWithinPin` was resolved by the background checker (`checker.go:checkTool`). `applyUpdate` installs a pre-resolved version, skipping resolution. |
| Context at install | Has a `progress.Reporter` (TTY) | `installFn` wraps `runInstallWithTelemetry` which creates its own reporter; background context, no interactive terminal |
| Post-failure | Writes a `notices.Notice` with `Kind=""` | Writes a `notices.Notice` with `Kind=KindAutoApplyResult`, performs auto-rollback |

**Key asymmetry for fallback:** In background auto-apply, version resolution happened in `checker.go:checkTool` and the resolved version is cached in `UpdateCheckEntry.LatestWithinPin`. If the fallback needs to happen at install time (during asset download or decomposition), both paths encounter it the same way inside `Decompose`/download. But if the fallback is implemented at version resolution time (Option 1), only the background checker's path in `checkTool` would need updating—the auto-apply path would naturally receive the "safe" version in `LatestWithinPin`.

### Notification emit point

For the fallback warning notification:

- **Interactive mode (`tsuku install`, `tsuku update`):** The `EvalContext` does not carry a `Reporter`. The executor's `progress.Reporter` is in `Executor.reporter`, not in `EvalContext`. The `Decompose` method has no access to a reporter. The notification would need to be surfaced at the call site in `plan_generator.go:resolveStep` after the fallback resolves, using the `PlanConfig.OnWarning` callback or a new callback. Alternatively, a returned `FallbackInfo` alongside the steps could carry the warning up to the caller.

- **Notices inbox (both modes):** After the plan is generated successfully (with the fallback version), the notices write would naturally fit in the install orchestration layer in `cmd/tsuku/install.go` or the `install.Manager`, where both interactive and auto-apply paths converge. The pattern already exists: `notices.WriteNotice(noticesDir, &notices.Notice{...})` is called in both `update.go` and `apply.go` after successful installs.

A new `Notice.Kind` value (e.g., `KindVersionFallback`) would distinguish this from update/failure notices for rendering purposes.

## Implications

1. **Fallback logic belongs in `Decompose`, not in the version provider.** The version provider has no knowledge of asset patterns—it resolves tags, not files. The natural insertion point is `GitHubArchiveAction.resolveAssetName`, where the asset existence check already happens. The `EvalContext` already has the resolver available.

2. **The `Decompose` signature cannot return side-channel warnings today.** `Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error)` returns steps or an error—no middle ground. A fallback that succeeds would need to communicate the "I used a different version" fact out-of-band. Options: attach it to a new field on `EvalContext` (the caller reads it after), or use the `PlanConfig.OnWarning` callback already plumbed through `resolveStep`.

3. **The `EvalContext` version fields are read-only at the call site.** Changing `ctx.VersionTag` inside `Decompose` would silently affect the rest of plan generation. A local copy of `EvalContext` must be used for the retry, and the winning version must be propagated back to the plan (the `InstallationPlan.Version` field and the resolved step URLs).

4. **Auto-apply and interactive update share the same code path for asset download.** The notification routing (inline vs. inbox) maps naturally to: emit via `PlanConfig.OnWarning` → caller writes inline output; always write `notices.Notice` with the new kind. The background checker doesn't go through `Decompose` during check—only during install, which means the fallback fires at the same code point for both modes.

5. **`notices.Notice` already supports `Kind` classification.** Adding `KindVersionFallback` is a small, backward-compatible extension. The notice rendering in `notify.go` would need a new branch to display the right message.

## Surprises

1. **The version resolver never checks for assets.** `ResolveGitHub` calls `GetLatestRelease` and returns the tag name. There is no early-exit if the release has zero assets. This means tsuku can confidently commit to a version that will always 404 at download time, with no indication during version resolution.

2. **`Decompose` is called at plan generation time, not at execution time.** For wildcard patterns, `FetchReleaseAssets` is called eagerly during plan generation. This means the 404 for a missing asset surfaces before any download attempt—during `tsuku install` the failure happens at the "Generating plan..." stage, not the "Downloading..." stage. The fallback must also happen at plan time (inside `Decompose`), not during execution.

3. **Static asset patterns bypass the asset API entirely.** Only wildcard patterns trigger `FetchReleaseAssets`. A static pattern like `"tool-{version}-linux-amd64.tar.gz"` constructs the URL directly without verifying the asset exists. The 404 is deferred to actual download. This means a complete fallback solution needs to handle both cases: API-detected missing assets (wildcards) and HTTP 404 during download (static patterns).

4. **`EvalContext` has no `Reporter`.** The executor's reporter is stored in `Executor.reporter` and propagated to `ExecutionContext` (the runtime context), not to `EvalContext` (the plan-generation context). Surfacing a warning from inside `Decompose` requires a new mechanism—either a callback field on `EvalContext`, use of `PlanConfig.OnWarning`, or a return-value channel.

5. **`UpdateCheckEntry.LatestWithinPin` is populated before install.** The background checker resolves the version and stores it. When auto-apply calls `installFn(tool, entry.LatestWithinPin, ...)`, the version is already committed. If the fallback is applied at `Decompose` time (install time), the `entry.LatestWithinPin` value in the cache will no longer match what was actually installed, creating a stale cache entry. The fallback logic would need to ensure the cache is updated with the actual installed version.

## Open Questions

1. **Should fallback live at the version resolution stage (checker.go) or the plan generation stage (Decompose)?** Implementing it in `checker.go` keeps the pre-resolved version correct in the cache, but requires the checker to download and parse asset lists, significantly increasing its cost. Implementing it in `Decompose` is cheaper but creates the cache staleness issue.

2. **How many previous versions should the fallback search?** One version back? Some configurable limit? The version list from `ListGitHubVersions` can be long. An unbounded search is expensive and could delay installation significantly.

3. **How should the inline warning be surfaced when `EvalContext` has no reporter?** The `PlanConfig.OnWarning` callback is the existing mechanism for plan-time warnings. Is it appropriate for a "I fell back to version X" message, or does it need a distinct callback with richer data?

4. **What happens when the fallback version is also different from the currently installed version?** After a successful fallback install, should tsuku suppress the "update available" notice for the skipped version? Otherwise, the next `tsuku outdated` run would show the skipped (broken) version as an available update.

5. **Should the notices inbox entry for a successful fallback look like a success or a warning?** The current `Notice` struct conflates "attempted failed version" with a failure. A successful fallback is a partial success—the tool was installed, but not at the latest version. A new `Kind` and possibly new fields (e.g., `SkippedVersion`, `InstalledVersion`) may be needed.

## Summary

Version resolution for `github_archive` recipes commits to a version before any asset existence check—the GitHub provider resolves the latest release tag without verifying that downloadable assets exist. The 404 surfaces during plan generation (inside `GitHubArchiveAction.Decompose`) for wildcard patterns, where `FetchReleaseAssets` is called, and during actual download for static patterns. The most natural fallback insertion point is inside `Decompose` using the `ctx.Resolver` already present in `EvalContext`, with the warning surfaced via the existing `PlanConfig.OnWarning` callback and persisted via `notices.WriteNotice` with a new `Kind` value. The main open question is whether fallback belongs at plan-generation time (inside `Decompose`) or earlier at version-check time (inside `checker.go`), since the choice affects cache correctness for the background auto-apply pipeline.
