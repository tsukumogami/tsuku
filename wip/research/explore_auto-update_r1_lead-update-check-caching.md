# Lead: How should cached update checks work?

## Findings

### 1. Existing Caching Patterns in the Tsuku Codebase

Tsuku already has five distinct caching subsystems, each using file-based JSON storage with TTL expiration. These establish strong conventions that update check caching should follow.

#### Version Cache (`internal/version/cache.go`)
- **Storage**: Individual JSON files in `$TSUKU_HOME/cache/versions/`, named by SHA256 hash of source description.
- **Staleness**: TTL-based. `ExpiresAt` is stored in the entry, but freshness is checked against it on read. Default TTL: 1 hour (`DefaultVersionCacheTTL`).
- **Invalidation**: Automatic on expiry; explicit via `Refresh()` method that bypasses cache.
- **Corruption handling**: If `json.Unmarshal` fails, treated as cache miss -- silently re-fetches.
- **Atomic writes**: Yes -- write to `.tmp` then `os.Rename`.
- **Concurrency**: No file locking. Best-effort writes (errors ignored on cache write).

#### Recipe Cache (`internal/registry/cached_registry.go` + `cache_manager.go`)
- **Storage**: TOML files + `.meta.json` sidecars in `$TSUKU_HOME/registry/{letter}/`.
- **Staleness**: TTL-based (default 24h). `isFresh()` calculates from `CachedAt + TTL`, not from stored `ExpiresAt` -- this means TTL config changes take effect without cache invalidation.
- **Stale-if-error fallback**: If network fetch fails and cache age < `maxStale` (default 7 days), returns stale data with a stderr warning. Beyond `maxStale`, returns `ErrTypeCacheTooStale`.
- **Size management**: LRU eviction via `CacheManager` with high/low water marks (80%/60%).
- **Corruption**: Metadata parse failure triggers fresh fetch.

#### Distributed Source Cache (`internal/distributed/cache.go`)
- **Storage**: Per-repo directories under `$TSUKU_HOME/cache/distributed/{owner}/{repo}/`.
- **Staleness**: TTL-based (default 1h). Incomplete entries (rate-limit fallback) use a 5-minute TTL.
- **Size management**: 20MB limit, oldest eviction.

#### Tap Cache (`internal/version/tap_cache.go`)
- **Storage**: JSON files under `$TSUKU_HOME/cache/taps/{owner}/{repo}/{formula}.json`.
- **Staleness**: TTL-based, checked via `time.Since(entry.CachedAt) > c.ttl`.
- **Corruption**: Silent cache miss on parse error.
- **Atomic writes**: Yes.

#### Plan Cache (`internal/executor/plan_cache.go`)
- **Storage**: Content-addressed via SHA256 of plan content.
- **Staleness**: Content-hash validation rather than time-based.
- **Invalidation**: Format version mismatch, platform mismatch, or content hash mismatch.

### 2. State Management and File Locking

**`state.json`** (`internal/install/state.go`):
- Uses `sync.RWMutex` for in-process concurrency.
- Uses `FileLock` (`internal/install/filelock.go`) for cross-process synchronization via advisory file locking (flock).
- Shared locks for reads, exclusive locks for writes.
- Atomic writes via temp file + rename.
- Robust migration system for backward compatibility (multi-version migration, source tracking migration).

The file locking infrastructure already exists and is battle-tested. This is directly reusable for update check state.

### 3. Configuration System

**`config.toml`** (`internal/userconfig/userconfig.go`):
- TOML-based user configuration at `$TSUKU_HOME/config.toml`.
- Already has sections for telemetry, LLM, secrets, registries.
- Natural home for update check configuration (interval, enabled/disabled, channel preferences).

**Environment variables** (`internal/config/config.go`):
- Pattern: `TSUKU_*` env vars with sensible defaults and range validation.
- Duration parsing with human-friendly error messages.
- Update check interval should follow this pattern: `TSUKU_UPDATE_CHECK_INTERVAL` defaulting to `24h`.

### 4. How Other CLI Tools Handle Update Check Caching

#### GitHub CLI (`gh`)
- Stores last check timestamp in `~/.config/gh/state.yml` under `checked_for_update_at`.
- Checks once per 24 hours.
- Non-blocking: check runs in background, result displayed on next invocation.
- Corrupt/missing state: silently creates fresh state.

#### Homebrew
- Stores `HOMEBREW_UPDATE_AUTO_LAST` timestamp in a file.
- Default interval: 24 hours (configurable via `HOMEBREW_AUTO_UPDATE_SECS`).
- Runs `brew update --auto-update` in background before operations.
- `HOMEBREW_NO_AUTO_UPDATE=1` disables entirely.

#### mise (formerly rtx)
- Checks every 7 days by default.
- Stores check result in `~/.local/state/mise/self_update_check`.
- Configurable via `settings.auto_update_check_interval` or `MISE_AUTO_UPDATE_CHECK_INTERVAL`.

#### rustup
- Checks on `rustup update` only -- no automatic background checks.
- Version info fetched from a manifest URL and compared to local.

#### Volta
- No automatic update checks for the binary itself.
- Tool version resolution uses cached manifests with TTL.

### 5. Recommended Design: Separate Cache File

**Do NOT put update check timestamps in `state.json`.** Reasons:

1. **Contention**: `state.json` is locked for writes during installs. An update check writing to the same file would contend with install operations.
2. **Separation of concerns**: `state.json` tracks installed tools. Update check metadata is operational state, not tool state.
3. **Frequency mismatch**: `state.json` changes on install/remove. Update checks happen on every command invocation (when stale). Different write frequencies suggest different files.

**Recommended: `$TSUKU_HOME/cache/update-check.json`**

```json
{
  "last_check_at": "2026-03-29T10:00:00Z",
  "tsuku": {
    "current_version": "0.7.0",
    "latest_version": "0.8.0",
    "channel": "stable",
    "checked_at": "2026-03-29T10:00:00Z"
  },
  "tools": {
    "ripgrep": {
      "installed_version": "14.1.0",
      "latest_version": "14.2.0",
      "checked_at": "2026-03-29T10:00:00Z"
    }
  }
}
```

This follows the same pattern as the version cache (individual JSON file in `$TSUKU_HOME/cache/`) but consolidates all update check results into a single file, since:
- The data is small (one entry per installed tool).
- It avoids filesystem overhead of per-tool files.
- A single atomic write covers all tools.

### 6. Staleness Policy

Based on conventions in the codebase and patterns in other tools:

| Parameter | Default | Env Override | Range |
|-----------|---------|-------------|-------|
| Check interval | 24h | `TSUKU_UPDATE_CHECK_INTERVAL` | 1h - 30d |
| Force check | `--check-updates` flag | n/a | n/a |
| Disable entirely | `false` | `TSUKU_NO_UPDATE_CHECK=1` | bool |

The check should happen **at most once per interval**. The `last_check_at` timestamp is the single gate.

### 7. Invalidation Triggers

1. **Time-based expiry**: `time.Since(last_check_at) > interval` -- standard TTL pattern.
2. **Force flag**: `tsuku update --check` or `tsuku outdated` always bypass cache.
3. **Install/remove/update**: After modifying the tool set, invalidate the `tools` section (the installed version may have changed).
4. **Binary upgrade**: After tsuku self-updates, invalidate the `tsuku` section.
5. **Manual**: `tsuku cache clear` should clear this file too.

### 8. Corruption and Missing Cache Handling

Following established tsuku conventions:

- **Missing file**: Treat as "never checked" -- trigger check on next eligible command.
- **Corrupt JSON**: Log warning to stderr, delete file, treat as "never checked."
- **Partial data**: If `tools` section is present but `tsuku` is missing, check only the missing part.
- **Clock skew**: If `checked_at` is in the future, treat as stale (same as `time.Since` returning negative).

### 9. Blocking vs. Background Checks

**Recommended: Non-blocking with deferred display.**

The check should run as a background goroutine that starts early in command execution and writes results to the cache file. If the check completes before command output finishes, append a notice. If not, the result is available for the next invocation.

Pattern:
```go
// In main command initialization
updateCh := make(chan *UpdateCheckResult, 1)
go func() {
    result, err := checkForUpdates(ctx, cache)
    if err == nil {
        updateCh <- result
    }
    close(updateCh)
}()

// After main command execution
select {
case result := <-updateCh:
    if result != nil && result.HasUpdates() {
        fmt.Fprintf(os.Stderr, "Updates available: ...")
    }
case <-time.After(50 * time.Millisecond):
    // Don't wait -- result will be cached for next run
}
```

This avoids adding latency to any command while still showing timely notifications.

### 10. Concurrent Instances

Two tsuku processes running simultaneously could both decide to check:

- **Race on read**: Both read stale cache, both trigger network check. This is harmless -- redundant network calls, not data corruption.
- **Race on write**: Both write results. With atomic writes (temp file + rename), the last writer wins. Both writes contain valid data, so this is also harmless.
- **No file locking needed**: Unlike `state.json` where lost writes mean lost tool state, update check cache is purely advisory. Last-writer-wins is acceptable.

This is the same approach used by the version cache, tap cache, and distributed cache -- none of them use file locking.

## Implications

1. **Reuse existing patterns**: The file-based JSON cache with TTL + atomic writes + corruption-as-miss pattern is well-established in at least four places in the codebase. The update check cache should be a fifth instance of this pattern, not something novel.

2. **Separate from state.json**: Update check metadata belongs in `$TSUKU_HOME/cache/`, not in `state.json`. This avoids lock contention and keeps concerns separated.

3. **Configuration via env vars + config.toml**: The interval and disable flag should be configurable through both `TSUKU_*` environment variables and `config.toml` settings, following the existing dual-config pattern.

4. **Background goroutine, not background process**: The check should be a goroutine within the running tsuku process, not a spawned subprocess. This keeps the implementation simple and avoids orphan process concerns.

5. **No per-tool granularity needed initially**: The first implementation can check all tools in a single pass (like `tsuku outdated` does today). Per-tool check intervals add complexity without clear benefit.

6. **The `outdated` command already does the hard work**: The current `outdated.go` iterates tools, loads recipes, resolves versions. The update check cache wraps this logic with time-gating. The version resolution itself is already cached by the version cache (1h TTL).

## Surprises

1. **No existing update check mechanism at all**: Despite having five caching subsystems, tsuku has zero infrastructure for proactive update notifications. The `outdated` command exists but only runs on explicit invocation -- there is no background or periodic check.

2. **Version cache TTL (1h) already covers most of the work**: Since version resolution results are cached for 1 hour, a 24-hour update check interval means the actual network cost of an update check is low -- most version lookups will be served from the version cache if the user has been active.

3. **The `outdated` command only checks GitHub-sourced tools**: It skips tools without a `repo` parameter in their recipe steps. A proper update check needs to handle all version providers (PyPI, npm, crates.io, etc.), not just GitHub. This is a gap in the current implementation.

4. **stale-if-error pattern exists but only in registry cache**: The recipe cache's stale-if-error fallback with `maxStale` is sophisticated but isolated to one subsystem. The update check cache could benefit from a similar pattern -- if the check fails, show the last known results with a note about when they were obtained.

5. **No config.toml support for existing cache TTLs**: All current TTL values are configured only via environment variables, not via `config.toml`. Adding update check config to `config.toml` would be the first TTL setting there, which may prompt unifying the other TTLs into `config.toml` as well.

## Open Questions

1. **Should tool update checks be per-tool or batch?** The current `outdated` command checks all tools in sequence. For a background check, should we check all tools (potentially slow with many tools) or only check a subset per invocation?

2. **What version providers should be supported?** The current `outdated` implementation only handles GitHub. Should the first update check implementation also support PyPI, npm, crates.io, etc., or is GitHub-only acceptable as a starting point?

3. **Should the check interval be configurable per-tool?** Some tools update frequently (nightly builds), others rarely. Is a global interval sufficient, or do power users need per-tool control?

4. **How should version channel pinning interact with the cache?** If a user pins to `14.x`, the "latest" field should reflect the latest 14.x version, not the absolute latest. This affects what gets stored in the cache and how checks are performed.

5. **Where does the notification appear?** Should it be stderr (like Homebrew), a banner line before output, or a separate `tsuku notices` command? The exploration context mentions "configurable notifications" but the UX isn't specified.

6. **Should `tsuku doctor` validate the update check cache?** It currently validates tool installations but not cache integrity. Adding a cache health check would be a natural extension.

## Summary

Tsuku should store update check results in a standalone `$TSUKU_HOME/cache/update-check.json` file using the same TTL + atomic-write + corruption-as-miss pattern already used by four other caching subsystems in the codebase, with a default 24-hour interval configurable via `TSUKU_UPDATE_CHECK_INTERVAL`. The check should run as a non-blocking background goroutine within the tsuku process, with results displayed if available before command exit or cached for the next invocation. The biggest open question is how version channel pinning (e.g., "only notify me about 14.x updates") should interact with the cache storage format and the version resolution layer.
