# Decision 3: Failure notice system

## Question

How are failed auto-update notices written, stored at `$TSUKU_HOME/notices/`, displayed via `tsuku notices`, and cleared after display?

## Options Considered

### Option A: Per-event JSON files (`$TSUKU_HOME/notices/<toolname>-<timestamp>.json`)

Each failure writes a new file with a unique name derived from the tool name and a timestamp (e.g., `ripgrep-1711846800.json`).

**Write protocol:** After rollback completes, marshal a notice struct to JSON, write to a temp file, rename into `notices/` with the timestamped name. Uses the same atomic write pattern as `updates/cache.go:WriteEntry`.

**Display logic:** On next tsuku command, `ReadDir` the notices directory, filter for files where `shown == false`, print a summary to stderr, then rewrite each file with `shown: true`. `tsuku notices` reads all `.json` files, sorts by timestamp, and displays them regardless of `shown` state.

**Clearing:** Files older than a retention period (e.g., 30 days) are pruned by `tsuku notices --clear` or a GC pass during commands. Alternatively, shown notices are deleted after retention expires.

**Pros:**
- No contention between concurrent auto-updates for different tools -- each writes its own file.
- History is naturally preserved; every failure is a separate record.
- Easy to extend for Feature 7 (consecutive-failure counting is just counting files per tool).
- Matches the per-file pattern already used in `cache/updates/`.

**Cons:**
- Directory can accumulate many files for repeatedly failing tools (until Feature 7 adds suppression).
- "Mark as shown" requires rewriting each file or maintaining a separate shown-state file.
- Listing requires reading every file in the directory.

### Option B: Single append-only `notices.json` file

All failure notices live in one JSON file as an array.

**Write protocol:** Acquire exclusive file lock, read existing array, append new notice, atomic write back. Uses the same lock pattern as `state.go:saveWithLock`.

**Display logic:** Read the file, filter for `shown == false` entries, print summary. Rewrite the file with those entries marked `shown: true`. `tsuku notices` displays all entries.

**Clearing:** Prune entries older than retention on each write, or via explicit `tsuku notices --clear`.

**Pros:**
- Single file to read for display -- no directory scan.
- All history in one place; easy to serialize/deserialize.

**Cons:**
- Lock contention: concurrent auto-updates for different tools must serialize on the same file lock. This is the main problem -- the auto-apply background process and the foreground tsuku command could race.
- Append-read-write cycle is not atomic without locking. The atomic rename pattern doesn't help because two writers could read the same state and one's append gets lost.
- File grows without bound until pruned.
- Doesn't match existing per-file patterns in the codebase.

### Option C: Per-tool JSON files (`$TSUKU_HOME/notices/<toolname>.json`)

Each tool gets one file. A new failure overwrites the previous notice for that tool.

**Write protocol:** Same atomic write as `cache.go:WriteEntry` -- marshal, write temp, rename. The file contains only the most recent failure for that tool.

**Display logic:** On next tsuku command, scan directory for files with `shown == false`, display summary, rewrite with `shown: true`. `tsuku notices` reads all files.

**Clearing:** Files are removed after retention period or via `tsuku notices --clear`.

**Pros:**
- No contention between tools (same as Option A).
- Bounded file count (one per tool, so at most ~number of installed tools).
- Matches `cache/updates/` pattern exactly: per-tool JSON files in a dedicated directory, with `ReadEntry`/`WriteEntry`/`ReadAllEntries` functions.
- Natural fit for Feature 7: the file can later gain a `consecutive_failures` counter without schema migration.

**Cons:**
- Only the most recent failure per tool is preserved. Earlier failures for the same tool are lost.
- If a user wants to see the full failure history for a flaky tool, it's not available.

### Option D: Per-tool JSON files with history array

Like Option C, but each per-tool file contains an array of the last N failures (e.g., last 5) instead of just the most recent.

**Write protocol:** Read existing file (if any), prepend new failure to array, truncate to N entries, atomic write.

**Display logic:** Same as Option C for the "show once on stderr" case -- check a top-level `unshown` flag. `tsuku notices` shows all entries from all files for full detail.

**Clearing:** Same as Option C.

**Pros:**
- Preserves recent history per tool without unbounded growth.
- Same concurrency properties as Option C (per-tool files, no cross-tool contention).
- Feature 7 consecutive-failure counting is trivial: count entries in the array.

**Cons:**
- Read-modify-write cycle on each failure (vs. simple overwrite in Option C). But this is acceptable since failures are rare events.
- Slightly more complex schema than Option C.

## Chosen

**Option C: Per-tool JSON files at `$TSUKU_HOME/notices/<toolname>.json`**

## Rationale

Option C is the right choice because it directly mirrors the established per-tool-file pattern from `cache/updates/` and keeps things simple for what this feature needs to deliver.

The key arguments:

1. **Pattern consistency.** The codebase already has `ReadEntry`, `WriteEntry`, `ReadAllEntries`, and `RemoveEntry` functions for per-tool JSON files in `internal/updates/cache.go`. The notices package can follow the same structure almost line-for-line. This means less new code to review and a familiar shape for anyone working in the codebase.

2. **Concurrency without locks.** Per-tool files mean the auto-apply background process writing a notice for `ripgrep` doesn't contend with a foreground `tsuku install node` that might also write a notice. Each tool's file is independent. This eliminates Option B's main weakness.

3. **Bounded storage.** At most one file per installed tool. Even with 50 tools, that's 50 small JSON files. Option A's per-event files could accumulate rapidly for a tool that fails on every cycle (24h default) until Feature 7 adds suppression.

4. **Good enough history.** Losing earlier failures for the same tool is acceptable. The most recent failure is what matters for debugging. If the tool keeps failing, the notice gets overwritten with the latest error, which is the freshest and most useful. Feature 7 will later add a `consecutive_failures` integer to the schema, which captures "how long has this been broken" without needing the full history.

5. **Option D's history array is premature.** The read-modify-write cycle and array management add complexity for a capability that isn't required by R11a. Feature 7 can add the consecutive-failure counter as a simple integer field on the existing per-tool file without changing the storage model.

The "mark as shown" mechanism works as follows: the notice struct includes a `shown: bool` field. On first display (stderr during a tsuku command), the file is rewritten with `shown: true`. `tsuku notices` displays all files regardless of `shown` state, sorted by timestamp. This gives one-time stderr alerts for new failures while keeping history available via the explicit command.

## Assumptions

- Failure notices are rare events (tools that consistently build and verify correctly won't produce notices), so directory scan cost is negligible.
- One notice per tool is sufficient. Users who need full audit trails can check telemetry data (R22, Phase 2).
- The `shown` flag rewrite is safe without file locking because only the foreground tsuku process reads and marks notices. The background auto-apply process only writes new notices.
- Feature 7 will extend the per-tool file schema (adding `consecutive_failures`, `first_failure_at`) rather than changing the storage model. The per-tool file approach accommodates this without migration.
- The `notices/` directory namespace is shared with Feature 5 (notification throttle state), but notice files use `<toolname>.json` naming while throttle state will use a different naming convention (e.g., `.throttle-<toolname>` or a subdirectory).
