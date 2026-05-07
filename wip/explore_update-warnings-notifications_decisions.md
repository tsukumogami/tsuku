# Exploration Decisions: update-warnings-notifications

## Round 1

- **Version-fallback notices are single-view**: A successful fallback installs the tool at X-1; it's not an ongoing error. Cleared after first view, same as success notices.
- **`InboxReporter` is the right abstraction**: New `progress.Reporter` implementation routing `Warn()`/`DeferWarn()` to `notices.WriteNotice()`. Keeps call sites unchanged; routing decision lives at construction time.
- **`Kind` must become the lifecycle routing key**: Replace `Error != ""` convention with explicit `Kind`-based dispatch for display and deletion behavior. Backward-compatible (zero value preserved).
- **Version fallback belongs in `Decompose`, not the version provider or checker.go**: Version provider has no asset pattern knowledge; `Decompose` already calls `FetchReleaseAssets` and has `ctx.Resolver` available. Cache staleness in `LatestWithinPin` is accepted as a known tradeoff.
