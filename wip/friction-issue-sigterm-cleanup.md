# Issue Draft: Socket Not Cleaned Up After SIGTERM

## Title

fix(tsuku-llm): ensure socket cleanup runs after SIGTERM signal

## Problem Description

When the tsuku-llm daemon receives SIGTERM, the socket file is not being cleaned up despite `cleanup_files()` being called in the code path.

### Current Behavior

Looking at `tsuku-llm/src/main.rs`, the shutdown sequence has a flaw:

```rust
// Lines 373-395
let shutdown_reason = tokio::select! {
    result = server_future => {
        result.context("Server error")?;
        "server stopped"
    }
    _ = tokio::time::sleep(idle_timeout) => {
        info!("Idle timeout reached, initiating shutdown");
        "idle timeout"
    }
    _ = sigterm.recv() => {
        info!("SIGTERM received, initiating graceful shutdown");
        "SIGTERM"
    }
};

// ... cleanup code follows
cleanup_files(&socket, &lock);
```

The problem is that when SIGTERM is received:
1. The `tokio::select!` exits, dropping the `server_future`
2. The server_future uses `serve_with_incoming_shutdown` which waits for `shutdown_rx.recv()`
3. But SIGTERM never sends anything to `shutdown_tx` - it just exits the select
4. When the server_future is dropped, the `UnixListener` may not properly unbind
5. Even if cleanup_files() runs, the OS may not have released the socket yet

Additionally, if the process is killed before reaching `cleanup_files()` (e.g., second SIGTERM or SIGKILL), the socket remains.

### Expected Behavior

After SIGTERM:
1. The socket file should be removed
2. Subsequent daemon starts should succeed without "address already in use" errors
3. The cleanup should happen even if graceful shutdown is interrupted

### Evidence from Friction Log

Integration tests observed that after sending SIGTERM to the daemon, the socket file still existed, preventing a new daemon from starting cleanly.

## Recommended Fix

1. **Send shutdown signal when SIGTERM received**:

```rust
_ = sigterm.recv() => {
    info!("SIGTERM received, initiating graceful shutdown");
    // Notify the server to stop accepting connections
    let _ = shutdown_tx.send(()).await;
    "SIGTERM"
}
```

2. **Use Drop guard for cleanup** to ensure cleanup even on panic or early exit:

```rust
struct CleanupGuard {
    socket: PathBuf,
    lock: PathBuf,
}

impl Drop for CleanupGuard {
    fn drop(&mut self) {
        cleanup_files(&self.socket, &self.lock);
    }
}

// In main():
let _cleanup = CleanupGuard { socket: socket.clone(), lock: lock.clone() };
```

3. **Handle second SIGTERM** by installing a signal handler that forces immediate exit after cleanup:

```rust
// After first SIGTERM, set up handler for second
tokio::spawn(async move {
    sigterm.recv().await;
    warn!("Second SIGTERM received, forcing exit");
    std::process::exit(1);
});
```

## Related Files

- `tsuku-llm/src/main.rs`: Lines 359-395 (signal handling and shutdown sequence)
