---
summary:
  constraints:
    - Daemon must exit with status 0 on SIGTERM (not "signal: terminated")
    - Socket file (llm.sock) must be removed before process exit
    - Lock file (llm.sock.lock) must be released before process exit
    - Must handle multiple rapid SIGTERMs safely (idempotent shutdown)
    - 10-second grace period for in-flight requests before forced exit
  integration_points:
    - tsuku-llm/src/main.rs - SIGTERM handler and shutdown logic
    - tsuku-llm/src/server.rs - gRPC server shutdown coordination
    - internal/llm/lifecycle_integration_test.go - Tests to unskip after fix
  risks:
    - Race condition between signal handler and cleanup code
    - Async runtime may not flush cleanly on signal
    - Lock file descriptor may not be released if cleanup is interrupted
    - Socket removal may fail if server still has open connections
  approach_notes: |
    The issue is that SIGTERM triggers process exit before cleanup runs.
    Need to ensure the signal handler:
    1. Sets a shutdown flag (doesn't immediately exit)
    2. Initiates graceful gRPC server shutdown
    3. Waits for in-flight requests
    4. Explicitly removes socket and lock files
    5. Then exits with status 0 via std::process::exit(0)

    Key: The Rust runtime must NOT handle SIGTERM by default - we need
    to catch it and perform cleanup before exiting.
---

# Implementation Context: Issue #1675

**Source**: docs/designs/DESIGN-local-llm-runtime.md

## Relevant Design Sections

### Signal Handling Requirements (from design doc)

From "Decision 3: Addon Lifecycle":
> **Signal Handling**: The server handles SIGTERM for graceful shutdown, allowing in-flight requests to complete before exit. This makes the addon well-behaved in systemd, Docker, and orchestrator contexts where SIGTERM is the standard shutdown signal.

From "Phase 2.5: Daemon Lifecycle Management":
> **Signal handling (Rust server)**:
> - SIGTERM handler triggers graceful shutdown
> - Wait for in-flight requests (10-second grace period)
> - Clean up socket and lock files on exit

From "InferenceServer" section:
> On shutdown (idle timeout, SIGTERM, or gRPC Shutdown call):
> 1. Stops accepting new connections
> 2. Waits for in-flight requests to complete (with 10-second grace period)
> 3. Removes socket file
> 4. Releases lock file

### Expected Behavior

The tests that are failing expect:
1. `TestIntegration_SIGTERMTriggersGracefulShutdown`: Socket should be cleaned up after SIGTERM
2. `TestIntegration_MultipleSIGTERMIsSafe`: Daemon should exit cleanly (status 0) after multiple SIGTERMs

Currently, the daemon exits with "signal: terminated" status instead of 0, indicating the default signal handler is running instead of our custom cleanup.
