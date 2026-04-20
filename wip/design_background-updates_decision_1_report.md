<!-- decision:start id="auto-apply-lifecycle-decoupling" status="assumed" -->
### Decision: Auto-Apply Lifecycle Decoupling and Concurrency Model

**Context**

Every tsuku command passes through `PersistentPreRun` in `cmd/tsuku/main.go`, which
calls `MaybeAutoApply` before the user's command runs. `MaybeAutoApply` reads cached
update check entries and, when auto-apply is enabled, calls `runInstallWithTelemetry`
for each pending update synchronously. A user with three pending auto-updates waits
through three complete install operations — each potentially involving a download,
extraction, and state write — before `tsuku list` prints anything.

The update check itself (`CheckAndSpawnUpdateCheck`) is already non-blocking: it fires
a detached `tsuku check-updates` subprocess via `cmd.Start()` without `Wait()`, uses
sentinel file mtime for staleness detection, and uses a non-blocking `flock(2)` probe
to deduplicate concurrent spawns. This pattern is proven, in-production, and adds less
than 1ms to foreground startup.

The locking model uses `flock(2)` via `install.NewFileLock` with two modes:
`LockExclusive` (blocking) and `TryLockExclusive` (non-blocking, returns immediately
if held). `MaybeAutoApply` already uses `TryLockExclusive` on `state.json.lock` as a
concurrency probe — if any other tsuku process holds the lock, auto-apply skips
silently and the cache entries persist for the next invocation.

State writes during install go through `StateManager.saveWithLock`, which acquires
`LockExclusive` on `state.json.lock`. The install flow uses atomic staging + rename:
files are first copied to a staging directory, then `os.Rename`d to the final location.
A partial install (process crash) leaves no half-installed state — the staging
directory may remain, but it's cleaned up at the start of the next install attempt for
the same tool and version.

**Assumptions**

- Tool installs can take 10–120 seconds (download + extract + state write). The
  10-second context timeout for `check-updates` (per PRD R19, visible in
  `cmd_check_updates.go`) rules out embedding installs in that subprocess.
- Users primarily notice blocking when running fast read-only commands (`tsuku list`,
  `tsuku info`) — the worst case. Blocking before `tsuku install foo` is less
  objectionable but still wrong.
- The "one command late" tradeoff — background results appear on the next invocation —
  is acceptable. Peer tools (npm, gh) use this model; Homebrew's inline blocking is
  universally considered a flaw.
- `SysProcAttr{Setpgid: true}` is not currently set on spawned processes. Without it,
  closing the terminal sends SIGHUP to the background process group on some shell
  configurations. This is a correctness gap, not just a polish issue.

**Chosen: Option A — Spawn a separate apply subprocess from PersistentPreRun**

Mirror `CheckAndSpawnUpdateCheck` exactly: add a `MaybeSpawnAutoApply` function in
`internal/updates/trigger.go` (or a parallel file) that, when auto-apply is enabled
and pending entries exist, spawns a detached `tsuku apply-updates` subprocess via
`cmd.Start()` without `Wait()`. `PersistentPreRun` calls this function alongside
`CheckAndSpawnUpdateCheck`; both return in under 1ms.

The `apply-updates` subcommand (hidden, like `check-updates`):
1. Acquires `TryLockExclusive` on `state.json.lock`. If not acquired, exits silently.
2. Releases the probe lock immediately (same pattern as `MaybeAutoApply` today).
3. Reads pending cache entries and applies each update via the install flow, which
   re-acquires `state.json.lock` per write via `saveWithLock`.
4. Writes a notice file per tool (success or failure) to `$TSUKU_HOME/notices/`.
5. Removes the consumed cache entry after each tool.

The spawner in `trigger.go` should set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`
via build-tagged files (`spawn_unix.go`, `spawn_windows.go` stub), isolating the
background process from terminal signals.

**Rationale**

Option B (combine check and apply in a single subprocess) violates the 10-second
context timeout that `check-updates` already carries. Tool installs are not bounded by
that budget — a tool download can take 60+ seconds on a slow connection. Combining the
operations means either extending the timeout to cover installs (making the background
process long-lived and harder to reason about), or exceeding it silently (leaving
installs incomplete). The two operations have fundamentally different time budgets;
conflating them trades one correctness problem (foreground blocking) for another
(inconsistent apply behavior).

Option C (defer to `PersistentPostRun`) is a partial improvement — the user's command
runs first — but still blocks: `tsuku list` outputs results, then the terminal hangs
for up to 30 seconds before the prompt returns. `PersistentPostRun` also does not run
when the command exits with an error, creating inconsistent apply behavior. If the
apply times out and defers to the next run, the same 30-second hang recurs. Synchronous
PostRun still prevents Ctrl+C from returning the prompt immediately after a fast command.

Option A achieves zero foreground blocking. The install subprocess runs at OS scheduler
priority alongside the user's command, with no foreground involvement beyond the <1ms
spawn call. The pattern is proven, already trusted in production for `check-updates`,
and requires no new infrastructure — only a new subcommand and a spawn call.

**Sub-questions resolved**

1. **Lock contention between background auto-apply and foreground `tsuku install foo`:**
   The background subprocess uses `TryLockExclusive` as a probe at startup. If a
   foreground install already holds `state.json.lock`, the background subprocess exits
   immediately and silently. The cache entries persist; on the next command invocation,
   the probe will succeed (the foreground install has completed) and auto-apply fires.
   If the foreground install covered the same tool version that was pending, the cache
   entry's `LatestWithinPin` will now equal `ActiveVersion`, and `MaybeAutoApply`'s
   filter (`e.LatestWithinPin != e.ActiveVersion`) will skip it. No double-install occurs.

2. **Should auto-apply skip foo when an explicit install is in progress?**
   Yes, implicitly. The `TryLockExclusive` probe at startup is the gate: if any tsuku
   process holds `state.json.lock`, the entire apply batch skips. This is correct
   behavior — it avoids contending with a foreground install that may be installing the
   same version. The cache entry survives for retry. No per-tool detection is needed;
   the lock scope is sufficient.

3. **`SysProcAttr{Setpgid: true}` for spawned process:**
   Set it. The fix is two build-tagged files: `spawn_unix.go` sets
   `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` before `cmd.Start()`;
   `spawn_windows.go` is a no-op (Windows is not a release target but the file keeps
   the build clean). Without this, closing the terminal on some shell configurations
   delivers SIGHUP to the background process group, interrupting an in-progress
   install. This applies to both `check-updates` and `apply-updates` spawners; the fix
   should be shared via a `spawnDetached(cmd)` helper in `trigger.go`.

4. **Failure mode if the apply subprocess crashes partway through:**
   Partial-batch crash (e.g., SIGKILL after applying tool 1 of 3): `RemoveEntry` runs
   after each tool's apply attempt, so tool 1's cache entry is already removed.
   Tools 2 and 3 retain their entries and are retried on the next invocation. No data
   corruption: the install flow's staging + atomic rename means the partially-installed
   tool directory (if any) is cleaned at the start of the next install attempt.
   Single-tool crash mid-install: the cache entry was not yet removed; auto-apply retries
   on next invocation. The user finds out via the notice system — `WriteNotice` runs
   before `RemoveEntry`; if the process crashes before `WriteNotice`, the entry persists
   and the retry writes the notice on the next attempt. For persistent failures, the
   consecutive-failure threshold (3) gates visibility so transient network failures do
   not produce noise on every command.

**Consequences**

- `PersistentPreRun` returns in <1ms for all commands. `tsuku list`, `tsuku info`, and
  every other fast command are unblocked.
- A new hidden subcommand `apply-updates` is added, parallel to `check-updates`.
- `MaybeAutoApply` and its synchronous call in `PersistentPreRun` are removed.
- `DisplayNotifications` in `PersistentPreRun` no longer needs to display synchronous
  apply results (the `results []ApplyResult` parameter becomes unused); it reads
  background notices via `renderUnshownNotices` instead, which already handles this.
- Users see apply results one command after the apply completes, not the same command.
  This is the accepted tradeoff (peer tool precedent: npm, gh).
- The spawner helper gains `SysProcAttr{Setpgid: true}`, which makes both `check-updates`
  and `apply-updates` processes independent of the terminal session.
- The `apply-updates` subprocess inherits the `check-updates` pattern of redirecting
  stdout/stderr to `/dev/null` and setting `cmd.SilenceUsage = true`.
<!-- decision:end -->
