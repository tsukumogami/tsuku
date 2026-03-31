# Decision 1: Cache schema and staleness model

## Question

What is the per-tool cache file schema and staleness detection model? What fields does each `<toolname>.json` contain, how does the trigger layer determine global staleness without statting every file, and what is the write protocol for the background process?

## Options Considered

### Option A: Per-tool files with a sentinel file for global staleness

Each tool gets its own cache file at `$TSUKU_HOME/cache/updates/<toolname>.json`. A separate sentinel file at `$TSUKU_HOME/cache/updates/.last-check` is touched (or written with a timestamp) after every background check run completes. The trigger layer stats only the sentinel file to determine whether a new check is needed.

**Pros:**
- Single stat for the trigger layer -- meets the <5ms latency budget.
- Per-tool files avoid lock contention between concurrent writers updating different tools.
- Sentinel file is cheap to write (just a touch or a small JSON blob).
- Matches the existing version cache pattern of per-provider files with atomic writes.
- Feature 5 and Feature 6 can scan per-tool files at leisure (not on the hot path).

**Cons:**
- Two-phase write: the background process must write all per-tool files, then update the sentinel. If the process crashes between writing tools and touching the sentinel, the sentinel is stale but tool files are fresh -- this is safe (triggers a redundant re-check, not data loss).
- Sentinel mtime can drift from individual tool mtimes if partial updates occur.
- Adds one extra file to manage alongside the per-tool files.

### Option B: Per-tool files with directory mtime for global staleness

Same per-tool file layout, but instead of a sentinel file, the trigger layer stats the `$TSUKU_HOME/cache/updates/` directory itself. Directory mtime updates whenever a file inside it is created, renamed, or deleted.

**Pros:**
- No extra sentinel file -- the directory is the signal.
- Single stat call for staleness detection.
- Simple mental model: "if anything in the directory changed recently, we're fresh."

**Cons:**
- Directory mtime semantics vary across filesystems and operating systems. On some systems, renaming a file within a directory (the atomic write pattern) may not update the directory mtime. This makes the approach unreliable.
- Creating temp files inside the directory (for atomic writes) would update the directory mtime prematurely, before the rename completes.
- If temp files are written to a sibling directory to avoid this, the rename from a different directory is not guaranteed atomic on all POSIX systems (must be same filesystem, and some implementations still don't update target dir mtime on rename-into).
- Fundamentally fragile -- ties correctness to filesystem implementation details.

### Option C: Per-tool files with a metadata file tracking last-check timestamp

A metadata file at `$TSUKU_HOME/cache/updates/.meta.json` stores structured data: the timestamp of the last completed check run, the list of tools checked, and optionally the check duration. The trigger layer reads (not just stats) this file.

**Pros:**
- Richer than a sentinel: can store check run metadata (tool count, duration, errors).
- Single file read for staleness detection.
- Can encode "which tools were checked" to detect newly installed tools that haven't been checked yet.

**Cons:**
- Reading and parsing JSON is slower than a single stat call. Even a small JSON file adds ~0.5-1ms for read + unmarshal, eating into the 5ms budget.
- The metadata file itself needs atomic writes, adding another temp-file-then-rename cycle.
- The extra fields (tool list, duration) aren't needed by the trigger layer -- they're optimization data that can live elsewhere.
- Over-engineers what is fundamentally a "has enough time passed?" check.

### Option D: Per-tool files with sentinel, background process writes sentinel first

Variant of Option A where the sentinel is written at the *start* of the check run (with an "in-progress" marker), then updated at completion. This lets the trigger layer skip spawning a check if one is already running.

**Pros:**
- Dedup without flock: trigger can detect in-progress checks by reading sentinel status.
- Same single-stat benefit as Option A for the common "is it fresh?" case.

**Cons:**
- Sentinel now has two states (in-progress, complete), adding parse complexity to the trigger.
- If the background process crashes, the sentinel is stuck in "in-progress" state. Requires a timeout-based fallback (e.g., treat in-progress as stale after 5 minutes).
- The exploration already decided on advisory flock for dedup, making the in-progress sentinel redundant.
- More complex than Option A with no clear benefit given flock-based dedup.

## Chosen

**Option A: Per-tool files with a sentinel file for global staleness.**

## Rationale

The trigger layer's <5ms latency budget is the binding constraint. Option A satisfies it with a single `os.Stat` on the sentinel file -- no file reads, no JSON parsing, no directory traversal. The trigger compares the sentinel's mtime against the configured check interval (e.g., 24 hours) and spawns a background check only if it's stale.

Option B fails on reliability -- directory mtime semantics are too filesystem-dependent to trust for correctness. Option C adds unnecessary parsing overhead. Option D duplicates the dedup responsibility already assigned to advisory flock.

Option A also aligns with the existing version cache pattern in `internal/version/cache.go`: independent per-file atomic writes with best-effort semantics. The sentinel is a minimal addition (one extra file) that solves the O(N) stat problem cleanly.

The crash-between-tools-and-sentinel edge case is safe: the worst outcome is a redundant re-check on the next trigger, which costs network calls but causes no data corruption or lost state.

For downstream consumers:
- **Feature 3 (auto-apply)**: reads individual `<toolname>.json` files for tools it wants to update. O(1) per tool.
- **Feature 5 (notifications)**: scans all files in the directory. O(N) but not latency-sensitive.
- **Feature 6 (outdated)**: same scan as Feature 5. Also not latency-sensitive.

## Assumptions

1. **Sentinel mtime resolution is sufficient.** Modern filesystems (ext4, APFS, NTFS) have sub-second mtime resolution. The check interval is measured in hours, so even second-granularity mtimes work.
2. **Advisory flock handles dedup.** The background process acquires an advisory lock before starting checks, so two concurrent triggers don't produce two concurrent check runs. The sentinel doesn't need to encode "in-progress" state.
3. **Notification throttle state lives in `$TSUKU_HOME/notices/`.** The check cache stores only "what's available," not "when was the user last notified." This separation is already decided.
4. **Tool names are filesystem-safe.** Recipe names use kebab-case (per conventions), so `<toolname>.json` filenames don't need escaping or hashing. This differs from the version cache's SHA256 approach because tool names are already constrained, while provider source descriptions are freeform.
5. **The background process checks all installed tools in one run.** Individual tool cache files may have different ages if the process is interrupted, but the sentinel reflects the last *complete* run.
6. **Feature 3 re-resolves download URLs at install time.** The check cache stores resolved versions but not download URLs or checksums. Feature 3 calls `ResolveVersion()` to get full `VersionInfo` (with metadata) when it actually installs.

## Schema Definition

### Sentinel file

Path: `$TSUKU_HOME/cache/updates/.last-check`

The sentinel is an empty file. Only its mtime matters. The trigger layer calls `os.Stat` and compares `ModTime()` against `time.Now().Add(-checkInterval)`.

Write protocol: after the background process finishes writing all per-tool cache files, it calls `os.Chtimes(sentinelPath, now, now)` to update the mtime. If the file doesn't exist, it creates it with `os.WriteFile(sentinelPath, nil, 0644)`.

### Per-tool cache file

Path: `$TSUKU_HOME/cache/updates/<toolname>.json`

```go
// UpdateCheckEntry represents the cached result of a background update check
// for a single installed tool. Stored at $TSUKU_HOME/cache/updates/<toolname>.json.
type UpdateCheckEntry struct {
    // Tool is the recipe name (e.g., "node", "ripgrep").
    Tool string `json:"tool"`

    // ActiveVersion is the installed version at check time (e.g., "20.11.0").
    // Copied from state.json ToolState.ActiveVersion.
    ActiveVersion string `json:"active_version"`

    // Requested is the user's pin constraint at check time (e.g., "20", "20.11.0", "").
    // Copied from state.json VersionState.Requested for the active version.
    // Used for pin-change detection: if state.json's Requested differs from this
    // value, the cache entry is logically stale regardless of mtime.
    Requested string `json:"requested"`

    // LatestWithinPin is the newest version that satisfies the pin constraint,
    // or empty if the active version is already the latest within the pin.
    LatestWithinPin string `json:"latest_within_pin,omitempty"`

    // LatestOverall is the newest version available regardless of pin constraints.
    // Used by Feature 6 (tsuku outdated) for the "overall" column and by
    // Feature 5 for out-of-channel notifications.
    LatestOverall string `json:"latest_overall"`

    // Source is the version provider description (e.g., "GitHub:nodejs/node").
    // Informational; helps with debugging and display.
    Source string `json:"source"`

    // CheckedAt is when the background process performed this check.
    CheckedAt time.Time `json:"checked_at"`

    // ExpiresAt is when this entry should be considered stale for per-tool reads.
    // Feature 3 uses this to decide whether to trust the cached result or re-check.
    // Distinct from the sentinel-based global staleness used by the trigger layer.
    ExpiresAt time.Time `json:"expires_at"`

    // Error records a non-empty string if the check failed (e.g., network error,
    // provider returned 404). Allows Feature 5 to skip tools with failed checks
    // rather than displaying stale data.
    Error string `json:"error,omitempty"`
}
```

### JSON example

```json
{
  "tool": "node",
  "active_version": "20.11.0",
  "requested": "20",
  "latest_within_pin": "20.18.2",
  "latest_overall": "23.1.0",
  "source": "GitHub:nodejs/node",
  "checked_at": "2026-03-31T10:00:00Z",
  "expires_at": "2026-04-01T10:00:00Z"
}
```

### Write protocol

1. Background process acquires advisory flock on `$TSUKU_HOME/cache/updates/.lock`.
2. For each installed tool, resolve versions and write the result:
   - Marshal `UpdateCheckEntry` to JSON.
   - Write to `<toolname>.json.tmp`.
   - Rename `<toolname>.json.tmp` to `<toolname>.json` (atomic on POSIX).
   - On write failure, log and continue to next tool (best effort, matching version cache pattern).
3. After all tools are processed, touch the sentinel: `os.Chtimes(".last-check", now, now)`.
4. Release flock.

### Staleness detection (trigger layer)

```go
func isCheckStale(cacheDir string, interval time.Duration) bool {
    sentinel := filepath.Join(cacheDir, ".last-check")
    info, err := os.Stat(sentinel)
    if err != nil {
        return true // No sentinel = never checked
    }
    return time.Since(info.ModTime()) > interval
}
```

Single `os.Stat` call. Well under 5ms on any filesystem.

### Pin-change detection (Feature 3)

```go
func isPinChanged(entry UpdateCheckEntry, currentRequested string) bool {
    return entry.Requested != currentRequested
}
```

Feature 3 reads both the cache entry and state.json. If `Requested` fields differ, the cache entry is logically stale and Feature 3 triggers a targeted re-check for that tool, even if the sentinel and entry mtime are fresh.
