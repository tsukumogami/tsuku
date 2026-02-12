# Issue Draft: Lock File Not Preventing Duplicate Daemons

## Title

fix(tsuku-llm): check lock file before binding to socket to prevent duplicate daemons

## Problem Description

The tsuku-llm daemon has a `lock_path()` function that generates a lock file path (`llm.sock.lock`), but the lock file is never actually used to prevent duplicate daemon instances from starting.

### Current Behavior

Looking at `tsuku-llm/src/main.rs`:

1. `lock_path()` (lines 255-261) generates the path but is only used in:
   - `cleanup_files()` during shutdown (line 395)

2. Before starting the server (lines 330-346), the code:
   - Removes stale socket if it exists
   - Creates the parent directory
   - Binds to the Unix socket

3. There is NO check for an existing lock file before binding.

This means:
- If a daemon is running and another instance starts, it will fail only when trying to bind to the socket (which may succeed if the first daemon hasn't bound yet)
- Race conditions can occur where two daemons start simultaneously
- The lock file mechanism exists but is completely non-functional

### Expected Behavior

Before binding to the socket, the daemon should:

1. Attempt to acquire an exclusive lock on the lock file
2. If the lock cannot be acquired, exit with a clear error message indicating another daemon is running
3. Hold the lock for the lifetime of the process
4. The lock file should be released automatically when the process exits (via file descriptor closure)

### Impact

- Integration tests that spawn multiple daemons may see race conditions or unclear failures
- Users accidentally running `tsuku-llm` twice could see confusing "address already in use" errors
- The current stale socket removal (line 335-336) could remove a socket actively in use by another daemon

## Recommended Fix

Use advisory file locking (`flock` on Unix) on the lock file:

```rust
use std::fs::{File, OpenOptions};
use std::os::unix::fs::OpenOptionsExt;

// Before binding to socket:
let lock_file = OpenOptions::new()
    .write(true)
    .create(true)
    .mode(0o600)
    .open(&lock)?;

// Try to acquire exclusive lock (non-blocking)
use std::os::unix::io::AsRawFd;
let result = unsafe { libc::flock(lock_file.as_raw_fd(), libc::LOCK_EX | libc::LOCK_NB) };
if result != 0 {
    anyhow::bail!("Another tsuku-llm instance is already running (lock held)");
}

// Keep lock_file alive for the lifetime of the server
```

Alternatively, use the `fs2` crate for a safer API:

```rust
use fs2::FileExt;

let lock_file = File::create(&lock)?;
lock_file.try_lock_exclusive()
    .context("Another tsuku-llm instance is already running")?;
```

## Related Files

- `tsuku-llm/src/main.rs`: Lines 255-261 (lock_path), lines 330-346 (startup sequence)
