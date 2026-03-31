# Decision 1: Apply lifecycle and concurrency

## Question

When and where does auto-apply run during tsuku commands, how does it read cached check results to determine what to install, and how is concurrency with other state-mutating commands prevented?

## Options Considered

### Option A: PersistentPreRun, after the update check trigger

Auto-apply runs in `rootCmd.PersistentPreRun` immediately after `CheckAndSpawnUpdateCheck`. It calls `ReadAllEntries` on the cache directory, filters for entries where `LatestWithinPin` is non-empty and `Error` is empty, checks `UpdatesAutoApplyEnabled()`, then calls `runInstallWithTelemetry` for each pending update sequentially.

**Commands that trigger it:** All commands except those in the skip list (`check-updates`, `hook-env`, `run`, `help`, `version`, `completion`).

**Concurrency safety:** The install flow already acquires exclusive flock on `state.json.lock` via `UpdateTool`. If another tsuku process holds the lock, `LockExclusive` blocks until it's released. This is safe but means auto-apply could delay command startup if another process is mid-install. The blocking duration is bounded by a single install operation.

**Pros:**
- Single integration point -- same skip list already vetted for the update check trigger.
- Runs before the user's command, so the command operates against up-to-date tool versions.
- Natural extension of the existing pattern: check fires, then apply fires.

**Cons:**
- Adds latency to command startup (the install itself takes time). PRD R19 says "zero added latency" for checks, but apply inherently takes time when updates exist.
- Running before every non-skipped command means even read-only commands like `list` or `search` could trigger installs.
- If auto-apply fails partway through multiple tools, the user's actual command hasn't started yet -- clean error reporting is straightforward but the user sees a wall of output before their command.

### Option B: Specific command allowlist

A dedicated `MaybeAutoApply(cfg, userCfg)` function is called at the start of specific commands: `install`, `update`, `list`, `outdated`, `info`. Each command's `Run` function calls it explicitly.

**Commands that trigger it:** Only the allowlisted commands.

**Concurrency safety:** Same as Option A -- relies on the existing flock in the install path.

**Pros:**
- Finer control over which commands trigger installs.
- Doesn't surprise users with installs during `search` or `recipes`.

**Cons:**
- Scattered integration points -- each command must remember to call the function.
- New commands need to opt in manually, easy to forget.
- Harder to reason about when auto-apply fires; no single source of truth.

### Option C: Post-command hook (PersistentPostRun)

Auto-apply runs in `rootCmd.PersistentPostRun`, after the user's command completes.

**Commands that trigger it:** All commands except the skip list (same as Option A).

**Concurrency safety:** Same flock mechanism. However, there's a subtlety: the user's command may have just mutated state (e.g., `install`), and auto-apply would immediately re-read state and mutate it again. This is safe due to the atomic load/save pattern but creates a longer lock-holding window.

**Pros:**
- The user's command runs first with zero added latency, then auto-apply happens afterward.
- Closer to PRD R19 intent -- the user's requested operation isn't delayed.

**Cons:**
- Tool versions used during the command may be stale (the command ran before updates applied).
- If the user runs `tsuku list`, they see old versions, then auto-apply installs new ones -- confusing.
- If auto-apply fails, the user's command already succeeded, making rollback messaging awkward.
- Post-run hooks in cobra don't fire if the command returns an error, so auto-apply would be skipped on failed commands.

### Option D: PersistentPreRun with non-blocking TryLock gate

Same as Option A (PersistentPreRun), but auto-apply first attempts `TryLockExclusive` on `state.json.lock`. If another tsuku process holds the lock, auto-apply silently skips rather than blocking. If the lock is acquired, it proceeds with installs while holding the lock for the entire batch, then releases before the user's command runs.

**Commands that trigger it:** Same as Option A (all except skip list).

**Concurrency safety:** Never blocks on another process. The TryLock pattern is already used by the update check trigger for spawn dedup. If two concurrent tsuku commands race, exactly one performs auto-apply; the other skips it entirely. The skipped process will pick it up on its next invocation.

**Pros:**
- No blocking delay from concurrent processes -- worst case is a skip, not a wait.
- Single integration point in PersistentPreRun.
- Familiar pattern (already used by `CheckAndSpawnUpdateCheck`).
- Safe against deadlock and long waits.

**Cons:**
- Same startup latency concern as Option A when updates are actually applied (but only when no other process is running).
- A skipped auto-apply means updates are deferred to the next command invocation.

## Chosen

**Option D: PersistentPreRun with non-blocking TryLock gate**

## Rationale

Option D combines the best properties of the other options while avoiding their main weaknesses.

**Single integration point.** Like Option A, auto-apply lives in PersistentPreRun alongside the update check trigger. The same skip list applies. No scattered call sites to maintain.

**Pre-command timing is correct for user experience.** After auto-apply, the user's command (e.g., `list`, `outdated`) reflects the latest installed versions. Post-command apply (Option C) creates confusing output where the user sees stale state, and per-command allowlists (Option B) are fragile.

**Non-blocking TryLock eliminates the concurrency risk.** The existing install path uses blocking `LockExclusive`, which is fine for a single install operation. But PersistentPreRun is the wrong place to block waiting for another process -- it would make every command hang when `tsuku install` is running in another terminal. The TryLock gate means: if another process is doing work, skip auto-apply silently.

**The skip-and-retry semantic is acceptable.** Cached update entries persist until consumed. If auto-apply skips because the lock is held, the entries are still there for the next command invocation. The user loses nothing.

**Concurrency flow:**

1. PersistentPreRun fires.
2. `CheckAndSpawnUpdateCheck` runs (existing behavior).
3. `MaybeAutoApply` runs:
   a. Check `UpdatesAutoApplyEnabled()` -- bail if false.
   b. `TryLockExclusive` on `state.json.lock` -- bail if not acquired.
   c. Read all cache entries via `ReadAllEntries`.
   d. For each entry with `LatestWithinPin != ""` and no error, call the install flow.
   e. On success, delete the consumed cache entry. On failure, trigger rollback (separate decision).
   f. Release the lock.
4. The user's command runs.

The lock acquired in step 3b is held for the entire auto-apply batch, then released. The user's command then acquires its own lock as needed through the normal install/state paths. This prevents interleaving between auto-apply and the command itself.

**Why not Option A directly?** Option A uses the same integration point but relies on blocking `LockExclusive` inside `runInstallWithTelemetry`. If another tsuku process is mid-install, the current process blocks at step 3d indefinitely. With the TryLock gate, we fail fast and skip.

**Why not add `install`, `remove`, `update` to the skip list?** These commands mutate state themselves, so auto-applying before them could create confusion (e.g., auto-apply updates node to 20.1, then `tsuku install node@18` runs). However, this isn't actually a problem: auto-apply only acts on cached entries that match the current pin. If the user is about to change the pin via `install node@18`, the existing cache entry (which was checked against the old pin) won't match after install. The simpler approach is to not special-case these commands.

## Assumptions

- `runInstallWithTelemetry` can be called from PersistentPreRun with no issues (config and loader are already initialized in `init()`).
- The install flow's internal locking (per-operation `LockExclusive` in `UpdateTool`/`Save`) will need adjustment to work within an outer TryLock scope. The auto-apply function holds the exclusive lock and calls install using the `WithoutLock` variants, or the install flow is refactored to accept a pre-acquired lock. This is an implementation detail for the coding phase.
- Cache entries written by `check-updates` are stable during auto-apply reads. The background checker uses atomic rename (`WriteEntry`), so partial reads are impossible.
- The PRD's "zero added latency" (R19) refers to the background check, not the apply step. Apply inherently takes time when updates exist; this is expected and acceptable.
