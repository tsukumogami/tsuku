# Exploration Findings: update-warnings-notifications

## Core Question

How should tsuku implement a context-aware notification routing system where the execution channel (interactive vs auto-update background) determines the sink (terminal output vs inbox persistence), with appropriate lifecycle semantics per notice type?

## Round 1

### Key Insights

- **The Reporter interface is already the right seam** *(reporter-sink)*: `progress.Reporter` is dependency-injected through the entire install stack — actions, executor, install.Manager, everything. A new `InboxReporter` implementation of that interface is all that's needed for the background path. The single change point is `cmd_apply_updates.go`, which already has `runInstallWithExternalReporter` as an entry point. Zero call-site changes in the install engine.

- **The background path deliberately discards output** *(reporter-sink, auto-apply)*: `cmd_apply_updates.go` redirects stderr to `/dev/null` before constructing the reporter, then constructs `NewTTYReporter(devnull)`. The silent loss is intentional. The fix is to swap the reporter type to `InboxReporter`, not remove the redirect.

- **Success notices are half-implemented** *(auto-apply)*: `renderUnshownNotices` in `notify.go` already handles `n.Error == ""` tool notices — it prints "X has been updated to Y". But `MaybeAutoApply` never writes a success notice. Display is wired; the write is missing. Smallest, lowest-risk gap to close.

- **Notice taxonomy is by convention, not code** *(taxonomy)*: Persistent vs. single-view is encoded by `Error != ""` convention. The `Kind` field exists (`KindUpdateResult`, `KindAutoApplyResult`) but drives no display or deletion behavior. To support the platform model, `Kind` must become the lifecycle routing key rather than inert metadata.

- **Version fallback doesn't exist** *(version-fallback)*: No "skip release with no asset, try previous version" logic exists. The current resolver errors out. The natural insertion point is inside `GitHubArchiveAction.Decompose`, where `FetchReleaseAssets` is already called for wildcard patterns and `ctx.Resolver` (`ListGitHubVersions`) is available. But `Decompose` returns `([]Step, error)` — a successful fallback needs an out-of-band mechanism to signal "I used a different version" (existing `PlanConfig.OnWarning` callback is the candidate).

- **Ecosystem consensus** *(external-patterns)*: npm, dnf-automatic, Homebrew, and rustup's proposed design all converge on the same pattern: file-based inbox for background results, TTY detection as routing gate, reporter interface as the seam. tsuku already has all three pieces — notices package, `ShouldSuppressNotifications()`, `Reporter` interface — just not wired together.

### Tensions

- **Where fallback lives**: Decompose time (inside `GitHubArchiveAction`) vs. checker.go time (before the version is cached). Decompose time is lower-cost but leaves `UpdateCheckEntry.LatestWithinPin` stale (checker cached X, but X-1 was installed). Checker.go time keeps the cache correct but requires fetching asset lists for every version candidate during background checks.

- **Static vs. wildcard pattern handling**: Wildcard patterns get `FetchReleaseAssets` called at plan-generation time (Decompose). Static patterns construct the URL directly and get a 404 at actual download time. A complete fallback solution needs to handle both cases.

### Gaps

- `ConsecutiveFailures` field is documented with a "suppress below 3 failures" contract but is never incremented or read — dead code.
- Self-update success notices are written (`Shown=false`) but never deleted — `renderUnshownNotices` marks `Shown=true` but nothing calls `RemoveNotice`. They accumulate on disk.
- Background `apply-updates` passes `nil` for `projectCfg`, silently ignoring any `.tsuku.toml` pin the user may have set.

### Decisions

- **Version-fallback notices are single-view**: A successful fallback (tool installed at X-1 because X had no asset) is not an ongoing error. The user sees it once in `tsuku notices` then it clears — same semantics as a success notice.
- **`InboxReporter` is the right abstraction**: A new `Reporter` implementation that routes `Warn()`/`DeferWarn()` calls to `notices.WriteNotice()` instead of stderr. No call-site changes required.
- **`Kind` must become load-bearing**: The new notice types (`KindVersionFallback`, `KindShellInitChange`, etc.) should route display and deletion behavior through `Kind`, not the `Error != ""` convention.
- **Fallback belongs in `Decompose`**: Version provider has no knowledge of asset patterns; `Decompose` is where asset existence is already checked and `ctx.Resolver` is available. The staleness issue with `LatestWithinPin` is an acceptable tradeoff for a targeted fix.

### User Focus

User's primary concern: no duplicate logic between paths — same `reporter.Warn(...)` call, different sink based on execution context. Version-fallback notices are single-view.

## Accumulated Understanding

tsuku has three components that need to be wired together: (1) the `progress.Reporter` interface, already injected everywhere; (2) the `notices/` package, already used for auto-apply failures; (3) TTY detection, already in `ShouldSuppressNotifications()`. The gap is that the background subprocess constructs a `NoopReporter`-equivalent (TTY reporter against devnull) instead of an `InboxReporter` that persists to the notices inbox.

The design has four parts:
1. **`InboxReporter`**: new `progress.Reporter` implementation that routes `Warn`/`DeferWarn` to `notices.WriteNotice`. Used by background path via `runInstallWithExternalReporter`. Interactive path continues to use `ttyReporter`; for events worth persisting interactively, a `fanoutReporter` wrapping both can be used later.
2. **Success notice write**: `MaybeAutoApply` writes `Notice{Error: "", Kind: KindAutoApplyResult, Shown: false}` on the success branch, activating the existing display logic in `renderUnshownNotices`.
3. **Notice taxonomy formalization**: `Kind` becomes the lifecycle discriminator. New Kind values (`KindVersionFallback`, optionally `KindShellInitChange`) with defined lifecycle rules (single-view vs. persistent) per Kind. The `Error != ""` convention is preserved for backward compatibility but `Kind` takes precedence where set.
4. **Version fallback logic**: `GitHubArchiveAction.Decompose` catches "no asset" errors, retries with previous versions via `ctx.Resolver.ListGitHubVersions`, and surfaces the fallback via `PlanConfig.OnWarning`. The notice is written as `KindVersionFallback`, single-view lifecycle.

The `ConsecutiveFailures` gap and self-update notice leak are in scope but secondary — they can be fixed as part of the same design since the fix is a small extension of the lifecycle model.

## Decision: Crystallize
