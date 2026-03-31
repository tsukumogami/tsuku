# Lead: How should duplicate background spawns be prevented?

## Findings

### 1. Existing Lock Patterns in Tsuku

**LLM Lifecycle Manager** (`internal/llm/lifecycle.go`): Uses **advisory file locking with fcntl/flock**. The `ServerLifecycle` manages a daemon via a lock file (`socketPath + ".lock"`). The `IsRunning()` method attempts a non-blocking exclusive lock (`syscall.FLOCK_EX | LOCK_NB`):
- If lock succeeds → daemon not running
- If lock fails (EWOULDBLOCK) → daemon is running
- Lock is held by the daemon process; automatically released when process exits

This is **fast** (sub-millisecond lock check) and **robust** (kernel-managed, auto-cleanup on process death).

**State File Manager** (`internal/install/state.go`): Uses **shared/exclusive file locking via flock** for concurrent access to `state.json`. The `FileLock` abstraction:
- `LockShared()` for reads (multiple readers allowed)
- `LockExclusive()` for writes (exclusive)
- Built on `syscall.Flock` (non-POSIX but available on Unix)

Both use **advisory locks**, not PID files. Advisory locks are automatically released on process termination (critical for crash safety).

### 2. FileLock Implementation Details

**File**: `internal/install/filelock.go` and `filelock_unix.go`

Characteristics:
- Creates lock file with `os.O_CREATE | O_RDWR, 0644`
- Non-blocking check: `syscall.Flock(fd, LOCK_EX | LOCK_NB)` returns immediately
- Blocking acquire: `syscall.Flock(fd, LOCK_EX)` waits indefinitely
- Unlock: `syscall.Flock(fd, LOCK_UN)` or implicit on close

**Critical property**: If the process holding the lock crashes, the kernel automatically releases it. No stale lock files.

### 3. Shell Hook Execution Context

**File**: `cmd/tsuku/hook_env.go`

Current implementation:
- Runs on every prompt (PROMPT_COMMAND, precmd, fish_prompt)
- Calls `ComputeActivation(cwd, prevPath, curDir, cfg)`
- Implements early-exit: if `cwd == curDir` (directory unchanged), returns nil (no output)
- Design requires <5ms latency when directory hasn't changed

The hook runs as a subprocess fork+exec (~2-4ms per invocation). Adding a background check spawn on top of this context means:
- Hook runs 5-20+ times per minute (typical shell usage)
- Cache must be checked fast
- Background process dedup is essential to avoid spawning 100+ processes

### 4. Background Process Dedup Mechanisms

Three candidates emerged from analysis:

**A) PID Files**
```go
pidFile := "$TSUKU_HOME/cache/update-check.pid"
pid := ReadPIDFile()
if pid != nil {
    // Check if process exists: os.FindProcess(pid)
    // On Unix, this succeeds even if process is dead
    if ProcessRunning(pid) {
        return  // already running
    } else {
        DeleteStaleFile()  // cleanup
    }
}
// Spawn new background process, write PID file
```

Problems:
- `os.FindProcess` on Unix succeeds even if process is dead (must use signal 0 to verify)
- Race condition: process exits between check and comparison
- Stale files accumulate if process crashes before cleanup
- PID reuse: a new process could have the same PID as the old one
- Not atomic: write-PID-then-spawn leaves window where file exists but process doesn't

**B) Lock Files (Advisory Flock)**
```go
lockFile := "$TSUKU_HOME/cache/update-check.lock"
lock := NewFileLock(lockFile)
if err := lock.LockExclusive() /* non-blocking */; err == nil {
    // We got the lock, no check running
    // Spawn background process
    lock.Unlock()  // or hold it -- design choice
} else {
    // Lock held by another process, check is running
    return  // skip spawn
}
```

Advantages (matching existing LLM code):
- Kernel-managed, atomic on Unix
- Non-blocking check is O(1), <1ms
- Automatic cleanup on process death
- No PID reuse concerns
- No stale file cleanup needed

**C) Mtime-Only Check**
```go
cacheFile := "$TSUKU_HOME/cache/update-check.json"
stat, _ := os.Stat(cacheFile)
if time.Since(stat.ModTime()) < interval {
    return  // cache fresh, skip
}
// Spawn check, but don't wait for completion
// Background process updates mtime when *starting* (not finishing)
```

Advantages:
- Requires only stat (no lock syscall)
- No process lookup
- No dedup file to manage

Problems:
- Doesn't prevent concurrent spawns
- If hook fires 10 times in 100ms, all 10 spawn checks
- Mtime-based staleness only works if process updates timestamp on start (race: process starts, updates mtime, then hangs)

### 5. Dedup Under Load

**Test scenario**: user in active shell session, typing commands every 1-3 seconds, hook fires on every prompt.

- **PID file approach**: 50-100 spawns before first check finishes (unreliable checks, CPU spike)
- **Advisory lock**: 1 spawn (lock held), 49-99 hook invocations skip (kernel rejects non-blocking lock acquisition in <1ms)
- **Mtime approach**: 50-100 spawns, all racing (worst case)

The advisory lock is dramatically more efficient at scale.

### 6. Crash Resilience

**Process dies mid-check**:

- **PID file**: Stale PID file persists. Next hook run checks and detects dead process (requires `kill -0`), deletes file, spawns new check. Delay to recovery: next prompt (could be 5-10s later).
- **Advisory lock**: Kernel releases lock immediately when process dies. Next non-blocking check succeeds instantly. No cleanup needed.
- **Mtime**: No state to clean up, but unbounded spawns continue.

**Advisory lock wins for robustness**.

### 7. Integration with Existing LLM Pattern

The LLM lifecycle manager already uses the advisory lock pattern at `internal/llm/lifecycle.go:85-113`:

```go
fd, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0600)
err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
if err != nil {
    // Lock failed - daemon is running
    return true
}
// Lock succeeded - daemon not running
_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
```

This pattern is battle-tested within tsuku. **Reusing it for background update checks ensures consistency and reduces risk**.

### 8. What Happens When Prompt Fires During Check?

**Advisory lock scenario** (recommended):

1. First prompt: lock acquired, background check spawned, lock released (or held depending on design)
2. Prompt fires while check running: non-blocking lock attempt fails immediately, hook skips spawn
3. Multiple concurrent prompts: kernel serializes lock attempts, all but one skip

**Key insight**: With advisory locks, concurrent prompt hooks don't queue. They fail fast and move on. This is the desired behavior -- no latency buildup.

### 9. Atomic Mtime Update in Background Process

If using advisory lock + background process, the background process should:
```go
// In background process
lock := NewFileLock(lockPath)
lock.LockExclusive()  // Blocking, we have time

// Immediately update cache file mtime to mark "check in progress"
// This signals to hook: don't spawn another check yet
stat, _ := os.Stat(cacheFile)
now := time.Now()
if stat == nil {
    os.Chtimes(cacheFile, now, now)  // Create if missing, set to now
}

// Do actual check (network, etc.), may take 5-30s
// Write results to temp file, atomic rename
// ...

lock.Unlock()
```

This is a **hybrid approach**: lock prevents concurrent spawns, mtime marks staleness for the main command path (not the hook). The hook only looks at the lock, not mtime.

## Implications

**Recommendation: Use advisory file locks with non-blocking check + background spawn.**

This design:
1. **Prevents unbounded spawns**: Hook's non-blocking lock check fails on contention, ~1ms cost
2. **Matches existing code**: Reuses `internal/install/filelock.go` and LLM patterns
3. **Handles crashes**: Kernel auto-releases locks on process death
4. **Adds zero latency**: Lock attempt is async, fails instantly on contention
5. **Integrates cleanly**: Background process and hook share lock file, no coordination needed

**Lock file path**: `$TSUKU_HOME/cache/update-check.lock`

**Background process spawning logic**:
```
hook-env (on every prompt):
  → stat cache file, compute staleness
  → if fresh, exit
  → if stale:
      → attempt non-blocking exclusive lock on update-check.lock
      → if lock fails: another check is running, exit (skip spawn)
      → if lock succeeds: release lock, spawn background process detached
```

**Background process**:
```
update-check (detached):
  → acquire exclusive lock on update-check.lock (blocking, we have time)
  → check network/versions
  → atomic write results to temp, rename to cache file
  → update mtime on cache file for next staleness check
  → release lock
  → exit
```

This avoids the race conditions of PID files and the unbounded spawning of mtime-only approaches.

## Surprises

1. **Advisory locks are better than I expected**: They're not just robust; they're also faster and simpler than PID file management. The kernel handles everything (atomicity, cleanup, blocking/non-blocking).

2. **Existing code already solved this**: The LLM lifecycle manager's pattern is exactly what's needed. No new abstraction required -- just reuse `FileLock` with non-blocking mode.

3. **"Mtime alone is insufficient" is a hard limit**: Even in supposedly "simple" approaches, concurrent shells invalidate the optimization. You end up spawning dozens of checks that could have been one.

## Open Questions

1. **Should the background process hold the lock for its entire lifetime?**
   - Holding it: Cleaner dedup, prevents concurrent checks, but extends lock duration to 30s+
   - Releasing after update: Faster lock release, but multiple checks could start if network is slow
   - **Recommendation**: Hold lock for entire check duration. Checks are rare (default 24h), and lock contention is in microseconds for 99% of prompts.

2. **Mtime semantics: when is it updated?**
   - On check start (marks "in progress"): Prevents duplicate spawns if staleness is based on mtime alone
   - On check finish (marks "completed"): Allows next check to be scheduled immediately after
   - **Recommendation**: Update on start. Makes the next staleness check simpler (just compare mtime).

3. **Should the hook wait for the lock, or fail fast?**
   - Non-blocking (recommended): <1ms cost, zero latency impact
   - Blocking with timeout: Guarantees eventual execution, but adds latency to slow prompts
   - **Recommendation**: Non-blocking. The shell hook is in the latency-critical path.

4. **What if the lock file is deleted externally?**
   - `FileLock.openFile()` creates it if missing
   - Next lock attempt succeeds
   - Could spawn duplicate checks if directory is rm'd
   - **Recommendation**: Accept edge case. Manually deleting `$TSUKU_HOME/cache/` is operator error.

## Summary

Advisory file locks (via `flock`) are the ideal dedup mechanism for background update checks: atomic, kernel-managed, zero latency on contention (non-blocking), and auto-cleanup on crash. This matches the existing pattern in `internal/llm/lifecycle.go` and `internal/install/filelock.go`. The hook path becomes: stat cache for staleness, attempt non-blocking lock (fails instantly if check running), spawn if lock succeeds. The background process holds the lock for its entire duration, preventing concurrent checks, and updates the cache file's mtime on completion for the main command path to detect staleness. This design adds <1ms to the hot path and prevents unbounded spawning under load.
