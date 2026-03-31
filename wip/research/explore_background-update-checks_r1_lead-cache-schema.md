# Lead: What's the right cache file schema for update check results?

## Findings

### Existing cache patterns in tsuku

The version cache at `internal/version/cache.go` implements a per-provider caching strategy using SHA256 hashes of source descriptions as keys. The `cacheEntry` struct contains:
- `Versions[]string`: list of versions (newest first)
- `CachedAt time.Time`: when the entry was cached
- `Source string`: human-readable source (e.g., "GitHub:rust-lang/rust")
- `ExpiresAt time.Time`: expiration timestamp for staleness detection

Each provider gets its own file at `cache/versions/<hash>.json` (8 bytes of SHA256 hex = 16 chars). This per-provider approach provides:
1. **Atomic writes**: each provider's list is independent, no multi-writer conflicts
2. **Staleness detection**: single mtime stat on individual cache files
3. **Separate TTLs**: each provider can have different cache intervals if needed

### What downstream consumers (Feature 3, Feature 5) need

From the PRD and roadmap:

**Feature 3 (auto-apply with rollback, R3, R9, R10, R11a)**:
- Which tools have newer versions available within their pin boundary
- The resolved version to install
- Previous version (for rollback linkage)
- Install status (success/failure)
- Failure reason (for rollback decision)

**Feature 5 (notification system, R12, R16)**:
- What updates are available (tool name, current version, new version)
- Whether the update was auto-applied or just available
- Whether it's in-channel (within pin) or out-of-channel (newer major version)
- Timestamp when the check ran (for de-duping notifications)

**Feature 6 (update polish, R13, R15b)**:
- Latest version within pin boundary
- Latest version overall (for dual-column display in `tsuku outdated`)
- Last notification timestamp per tool (for weekly throttle on out-of-channel)

### Tool state context

The state.json schema (ToolState, VersionState in `internal/install/state.go`) provides:
- `ActiveVersion string`: current active version per tool
- `Requested string`: what the user typed at install (determines pin level)
- `Source string`: where the tool's recipe comes from ("central", "local", "owner/repo")
- Per-version `InstalledAt time.Time`: installation timestamp

For update checks, the system already has per-tool install state with pin boundaries encoded in the `Requested` field. The update check cache needs to bridge this: it captures what's available, and Feature 3 reads both the cache and state.json to decide what to install.

### Cache file schema options

**Option A: Single file at `$TSUKU_HOME/cache/update-check.json`** with per-tool entries:
```json
{
  "checked_at": "2026-03-31T10:00:00Z",
  "expires_at": "2026-04-01T10:00:00Z",
  "tools": {
    "node": {
      "current_active": "20.11.0",
      "pin_level": "major",
      "latest_within_pin": "20.12.0",
      "latest_overall": "22.0.0",
      "last_checked_at": "2026-03-31T10:00:00Z",
      "available_version": "20.12.0",
      "available_tag": "v20.12.0",
      "source": "GitHub:nodejs/node",
      "last_notification_timestamp": "2026-03-24T09:30:00Z"
    },
    "ripgrep": { ... }
  }
}
```

Pros:
- Single atomic write covers all tools
- One mtime stat for staleness detection
- Easy to display "available updates" in CLI
- Out-of-channel throttle state co-located with check result

Cons:
- Lock contention if multiple processes try to write simultaneously (e.g., shell hook + tsuku command)
- Reads the entire file even when checking one tool
- Schema couples all tools together

**Option B: Per-tool files at `$TSUKU_HOME/cache/updates/<toolname>.json`** (mirrors versions/ structure):
```json
{
  "tool": "node",
  "active_version": "20.11.0",
  "pin_level": "major",
  "latest_within_pin": "20.12.0",
  "latest_overall": "22.0.0",
  "checked_at": "2026-03-31T10:00:00Z",
  "expires_at": "2026-04-01T10:00:00Z",
  "source": "GitHub:nodejs/node",
  "last_notification_timestamp": "2026-03-24T09:30:00Z"
}
```

Pros:
- Each tool's cache is independent (matches version cache precedent)
- No lock contention between tools
- Scales to many tools
- Simple schema per file

Cons:
- Many files to stat when checking staleness (N tools = N stats)
- Aggregate "what's available" requires reading multiple files
- Multiple writes during check could partially succeed

### Feature 5 (notifications) requirements inform the schema

From R12 (R13 out-of-channel, R11a failure reporting):
- Notifications are per-tool
- Out-of-channel notifications throttle weekly per tool (R13)
- Failure notices store per-tool state (R11)

The notification throttle state (last_notification_timestamp per tool) needs to live in the check cache so Feature 3 can update it after displaying a notification. This argues for per-tool tracking.

However, Feature 5's behavior is "read check results, display notification", not "write to cache". The throttle state could also live in a separate per-tool file at `$TSUKU_HOME/notices/<toolname>.last_notification` (similar to how failure notices will be stored in `$TSUKU_HOME/notices/`).

### Atomic write and concurrent access implications (R21)

The PRD requires atomic writes (temp-file-then-rename). The version cache already does this:
```go
tempFile := path + ".tmp"
os.WriteFile(tempFile, data, 0644)
os.Rename(tempFile, path)  // atomic on POSIX
```

For Option A (single file): If a shell hook and a tsuku command both try to write simultaneously, one rename wins and the other loses its update. This requires explicit locking (file lock or advisory lock), adding complexity.

For Option B (per-tool): Each tool's write is independent. Two processes updating node and ripgrep simultaneously succeed. Two processes updating node simultaneously: one succeeds, the other fails with rename error (safe, no partial state).

The version cache already accepts write failures gracefully with `_ = c.writeCache(cacheFile, versions)` (best effort).

### What VersionInfo provides and what's missing

The VersionInfo struct has:
- `Tag string`: original tag (e.g., "v1.2.3")
- `Version string`: normalized version (e.g., "1.2.3")
- `Metadata map[string]string`: provider-specific (checksums, URLs)

The VersionResolver interface provides ResolveLatest() and ResolveVersion(), which return VersionInfo. A background check would call ResolveWithinBoundary(provider, requested) to get the latest within the pin, and ResolveLatest(provider) for the latest overall.

The check cache must store the resolved version and the pin level to answer "is there an update?" later. Storing the `Requested` field (pin constraint) directly is simpler than storing the resolved pin level, since state.json already has `Requested` per tool.

### Storage of pin boundary in cache

The cache needs to know the pin level to determine staleness and to help Feature 3 decide what to install. Options:
1. Store the `Requested` field directly (e.g., "20", "20.11.0", "")
2. Store the computed pin level (e.g., "major", "minor", "exact", "latest")
3. Store only the latest-within-pin and latest-overall versions; Feature 3 re-derives pin level from state.json

Option 3 is simplest: the cache stores what's available, Feature 3 reads both cache and state.json to decide. The cache doesn't need to store pin level; it just stores "latest_within_pin" (which was computed from the user's pin at check time) and "latest_overall".

But this creates a timing issue: if the user changes their pin (e.g., `tsuku install node@22`), the cached "latest_within_pin" value becomes stale even if the cache mtime is fresh. Feature 3 must re-check whenever the pin changes in state.json.

Storing the `Requested` value at check time (Option 1) solves this: Feature 3 compares state.json's current `Requested` with the cache's `Requested`. If they differ, the cache is logically stale and needs a refresh.

## Implications

**Single-file schema (Option A) is simpler for display but adds locking complexity.** Feature 5 (notifications) wants to list all available updates in one pass; a single file makes this cheap. But the lock contention issue is real when the shell hook fires while a tsuku command is also writing. This requires either advisory file locking or a dedup mechanism (e.g., checking if a check is already running before spawning).

**Per-tool schema (Option B) matches the existing version cache precedent and avoids lock contention.** Feature 3 reads per-tool caches when applying updates (one file per tool being updated), and Feature 5 aggregates across tools (requires reading multiple files). The scan cost for Feature 5 is O(N tools), which is acceptable since notifications aren't on the hot path.

**Storing `Requested` in the cache enables pin-change detection.** If the user manually edits their pin after a check, the cache's `Requested` field will differ from state.json's. Feature 3 can detect this and invalidate the cache entry without relying on mtime alone. This handles the edge case where mtime is fresh but the pin has changed.

**Failure notices and out-of-channel throttle state belong elsewhere.** Feature 3 writes failure notices to `$TSUKU_HOME/notices/<toolname>.json` (R11a). Feature 5's out-of-channel throttle (R13, weekly per tool) could also live in notices/ as `<toolname>.last_notification` or a separate `update-throttle.json`. Keeping the update check cache focused on "what's available" and moving state-of-notifications to notices/ separates concerns.

## Surprises

1. **The version cache already solves the hard problems.** Per-provider files with SHA256 hashing, atomic writes, and TTL-based expiration are the precedent to follow. The update check cache should be similarly structured.

2. **Feature 5's throttle state (last_notification_timestamp per tool) is currently not modeled anywhere.** It's mentioned in R13 but there's no struct for it yet. This might belong in a separate `update-throttle.json` file or integrated into the check cache. The roadmap doesn't detail where this state lives.

3. **Lock contention on a single file isn't addressed in the PRD or scope document.** R5 (layered triggers) describes shell hook, shim, and command spawning checks concurrently, but doesn't say how they coordinate writes. The assumption seems to be "best effort writes" like the version cache, but that loses updates silently.

4. **Atomic per-tool writes are safer than a single atomic file for the concurrent access scenario.** The version cache's pattern (per-provider) is actually better for concurrency than Option A would be.

## Open Questions

1. **Should the update check cache be per-tool (like versions/) or single-file (like state.json)?** The PRD specifies `$TSUKU_HOME/cache/update-check.json` (single file), but per-tool files match the version cache precedent and avoid locking. Is the single-file choice a firm requirement or a default that can be reconsidered?

2. **What happens when a shell hook and a tsuku command both try to write the cache simultaneously?** With Option A (single file), a lock is needed. With Option B (per-tool), the concurrent writes are independent but one tool's result may be lost. Is "best effort, one writer wins" acceptable, or does every check must succeed?

3. **Where does the out-of-channel throttle state (last_notification_timestamp per tool) live?** In the check cache, in a separate update-throttle.json, or in notices/? This affects the schema.

4. **Does the check cache store the full VersionInfo metadata (checksums, URLs), or just the resolved version string?** Feature 3 needs to download and install, so it needs URLs and checksums. Are those in the check cache, or does Feature 3 re-resolve from the provider?

5. **What fields are strictly necessary vs. "nice to have" for display?** The minimal set is: tool name, current version, latest-within-pin version, latest-overall version, checked-at timestamp, expires-at timestamp. Are pin_level, source, and metadata required in the cache, or can they be derived?

6. **How should the schema handle tools that are not yet installed?** Can a check result exist for a tool not in state.json (e.g., user is considering installing node), or do checks only run for already-installed tools?

## Summary

The update check cache should use a **per-tool file schema at `$TSUKU_HOME/cache/updates/<toolname>.json`** to match the version cache precedent and avoid lock contention on concurrent writes. Each file stores: tool name, active version, `Requested` pin constraint (for change detection), latest-within-pin and latest-overall versions, source description, check timestamp, and expiration time. Failure notices and out-of-channel notification throttle state belong in a separate `$TSUKU_HOME/notices/` directory, not in the check cache, to keep the cache focused on "what's available." Feature 3 (auto-apply) reads per-tool cache entries when installing, and Feature 5 (notifications) scans all files to display available updates; the O(N) scan cost is acceptable since display is not on the hot path.

