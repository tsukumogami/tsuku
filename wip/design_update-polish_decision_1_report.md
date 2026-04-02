<!-- decision:start id="ooc-throttle-persistence" status="assumed" -->
### Decision: Out-of-channel notification throttle persistence and clock injection

**Context**

When a tool is pinned to a version boundary (e.g., `node@20`), the background checker resolves both the latest version within the pin and the latest overall. If a newer major version exists outside the pin, tsuku should display an out-of-channel notification -- but at most once per week per tool (PRD R13) to avoid nagging.

This requires two things: (1) persistent per-tool state recording when the notification was last shown, and (2) an injectable clock so tests don't need real time delays.

The existing codebase already has several file-based persistence patterns in the `cache/updates/` directory: per-tool JSON files (`UpdateCheckEntry`), a `.last-check` sentinel using mtime, and a `.notified` sentinel using mtime. Clock injection doesn't exist yet in the updates package -- `time.Now()` is called directly throughout `cache.go`, `checker.go`, and `apply.go`.

**Assumptions**

- The number of pinned tools with out-of-channel versions will be small (under 20), so file I/O overhead per tool is not a concern.
- The `.notified` sentinel mtime pattern is considered a proven approach for dedup in this codebase, given it already works for available-update summary throttling.
- No concurrent writes to throttle state are expected since notifications are rendered in the foreground CLI process, not in the background checker.

**Chosen: Option B -- Per-tool throttle files with mtime**

One file per tool at `$TSUKU_HOME/cache/updates/.ooc-<tool>`, using the file's mtime as the "last notified" timestamp. To check if a notification should show: `stat` the file; if missing or mtime is older than 7 days, show the notification and touch the file. Clock injection uses a `now time.Time` parameter on the throttle-check function rather than a package-level variable.

Concretely:

- `IsOOCThrottled(cacheDir, toolName string, now time.Time) bool` -- returns true if `.ooc-<tool>` exists and its mtime is within 7 days of `now`.
- `TouchOOCThrottle(cacheDir, toolName string) error` -- creates or touches `.ooc-<tool>`.
- The notification rendering code passes `time.Now()` in production; tests pass an arbitrary `time.Time`.

**Rationale**

This matches the existing `.notified` sentinel pattern almost exactly. The codebase already uses mtime-based file sentinels for the same kind of "has enough time passed?" check (`IsCheckStale`, `isSentinelStale`). Per-tool files avoid JSON parsing and atomic-write concerns entirely -- `os.Stat` and `os.Create` are the only operations needed.

The `now time.Time` parameter approach for clock injection is already used elsewhere in the codebase (see `calculateNextRetryAt`, `ApplySourceChange`, `expireBackoff` in batch/seed packages). It's simpler than a package-level `var NowFunc` because it doesn't require resetting global state between tests and has no risk of leaking between parallel test cases.

The dotfile prefix (`.ooc-`) keeps throttle files out of `ReadAllEntries`, which already skips files starting with `.` (line 90 of cache.go). No changes to existing code are needed.

**Alternatives Considered**

- **Option A: Single JSON throttle file.** A single `.out-of-channel.json` mapping tool names to timestamps. This would work but introduces JSON serialization for something that's just a per-key timestamp. It also requires atomic read-modify-write (read map, update entry, write back), while the mtime approach needs only stat + touch. The package-level `var NowFunc` for clock injection creates shared mutable state that can leak between parallel tests. Rejected because it adds unnecessary complexity for no functional benefit.

- **Option C: Extend UpdateCheckEntry.** Adding an `OutOfChannelNotifiedAt` field to the existing per-tool cache JSON. This mixes concerns -- the cache entry represents background checker output (what versions exist), while the throttle represents foreground UI state (when did we last tell the user). The background checker would need to preserve a field it doesn't own when overwriting entries, and any cache invalidation or re-check would risk resetting the throttle. Rejected because it couples unrelated state and creates subtle data-loss scenarios during cache refresh.

**Consequences**

- Adding or removing a pinned tool doesn't need special cleanup -- uninstall can delete `.ooc-<tool>` alongside the tool's cache entry, or leave it as a harmless orphan.
- The `cache/updates/` directory gains up to N dotfiles (one per pinned tool with an OOC version). These are invisible to `ReadAllEntries` and `ls` by default.
- Tests for the throttle are straightforward: create a temp dir, optionally touch a file with a controlled mtime, call `IsOOCThrottled` with a chosen `now`.
<!-- decision:end -->
