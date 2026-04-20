# Lead: Platform implications of background process spawning

## Findings

### Platform targets

GoReleaser (`.goreleaser.yaml`) builds for `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64`. Windows is absent from release targets. The codebase carries Windows build tags (`internal/install/filelock_windows.go`, `internal/shellenv/filelock_windows.go`) and the project compiles cleanly on Windows, but no Windows binary is shipped. This narrows the primary concern to Linux and macOS, with Windows as a "works if you build it" case to keep from breaking.

### What tsuku already does

`internal/updates/trigger.go` implements the full background-spawn pattern:

1. Stat a sentinel file (`$TSUKU_HOME/cache/updates/.last-check`) to detect staleness in under 0.5 ms.
2. Attempt a non-blocking `flock(2)` (`LOCK_NB`) on a lock file to deduplicate concurrent spawns.
3. Call `exec.Command(binary, "check-updates")` with `Stdin/Stdout/Stderr = nil`, then `cmd.Start()` — never `cmd.Wait()`.

The spawned `check-updates` sub-command runs `updates.RunUpdateCheck`, which acquires its own exclusive lock, does the network I/O, writes per-tool JSON cache entries, and touches the sentinel. The parent exits without waiting.

### Goroutines with process exit

In Go, when `main()` returns (or `os.Exit` is called), the runtime tears down immediately. All goroutines that have not yet been scheduled are abandoned — there is no grace period. `sync.WaitGroup` and `defer` statements in goroutines do not run. This is consistent across Linux, macOS, and Windows. The practical implication: any background goroutine that starts network I/O but doesn't finish before the CLI command returns will be killed mid-flight. For operations like registry fetches (seconds to tens of seconds), goroutines alone cannot carry the work after the parent exits. This rules out a simple `go func() { doNetworkWork() }()` pattern unless the parent is held open with a `WaitGroup` — which reintroduces the blocking problem.

### Detached subprocess (`os/exec` + `cmd.Start()` without `Wait()`)

**Linux / macOS**: `cmd.Start()` followed by no `cmd.Wait()` leaves a child process running after the parent exits. On Linux and macOS the process persists as an orphan and is re-parented to PID 1 (or the subreaper, e.g., systemd). Without `SysProcAttr{Setpgid: true}` or `Setsid: true`, the child shares the parent's process group and will receive SIGHUP if the controlling terminal closes (relevant in interactive shells). For a CLI tool invoked from a terminal session, this means the background checker could be killed when the user closes their terminal. The existing `trigger.go` does not set `SysProcAttr`, so the spawned `check-updates` process is in the same process group. In practice, this is usually safe — most shells don't propagate SIGHUP to background processes that aren't job-controlled — but it is a latent edge case, particularly for tools like `nohup`-hostile setups or `fish` with aggressive process cleanup.

**Windows**: Without `SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}` or `DETACHED_PROCESS`, a child process launched via `cmd.Start()` is attached to the same console and job object as the parent. If the parent is attached to a console that closes, the child may receive a console detach event that terminates it. Additionally, on Windows, processes are not re-parented to a global init; they may show up as orphans under the `wininit.exe` subtree. The `golang.org/x/sys/windows` package (already in go.mod as a transitive dep) exposes the necessary constants to set `CREATE_NEW_PROCESS_GROUP` and `DETACHED_PROCESS`. The current code sets no `SysProcAttr` at all, which works for Linux/macOS but is fragile on Windows.

**Shell job control interaction**: On Linux/macOS, shell job control tracks process groups, not individual processes. A `cmd.Start()`-spawned child without `Setpgid` or `Setsid` stays in the parent's process group. The shell can therefore see it with `jobs` or send it SIGSTOP. Setting `Setpgid: true` breaks it out of the shell's job control, which is generally what you want for a transparent background operation. The existing trigger does not do this.

### File-based deferred work

Write a "work pending" marker file on the current invocation; the next invocation reads it and performs the work before or after the user's command. This is already partially how the cache layer works: `check-updates` writes JSON entries and a sentinel, and the next command reads them via `MaybeAutoApply`. The full deferred-work variant (where the marker triggers network I/O on the next command) shifts latency rather than eliminating it — the first invocation that picks up the marker still blocks. It adds no system footprint and survives reboots, but any stale state (machine offline, marker written two weeks ago) must be handled carefully. For infrequent operations (daily update checks) it is viable. For operations that must complete before the command runs (e.g., registry refresh needed to resolve a recipe), it doesn't help.

### OS scheduling (cron, launchd, systemd timers, Task Scheduler)

Using system schedulers entirely offloads background work from the CLI process. The work runs on a timer independent of any user invocation.

- **cron (Linux)**: Universal, minimal setup, but requires the tsuku installer to add a crontab entry. No easy way to detect whether the user's shell environment is sourced; network access works. Managing multiple users or sandboxed envs is awkward. Doesn't survive if the binary path changes.
- **systemd timers (Linux)**: More reliable than cron, integrates with `sd-notify`, supports per-user services (`~/.config/systemd/user/`). Requires `systemctl --user enable`. Not available on Alpine or systems without systemd.
- **launchd (macOS)**: Plist-based, per-user agents, reliable. Requires the installer to write a plist to `~/Library/LaunchAgents/` and call `launchctl load`. The installer already handles binary placement, so this is feasible but adds surface area.
- **Task Scheduler (Windows)**: XML-based job registration via `schtasks.exe` or the COM API. Manageable from Go using `os/exec` to call `schtasks`. Not particularly complex, but an install/uninstall lifecycle is needed.

In all cases: using OS schedulers adds an install/uninstall lifecycle, requires per-platform code paths, and creates a persistent footprint (a registered service or cron entry). The project philosophy strongly favors no system dependencies and no daemons. OS schedulers violate both.

### Cross-platform file locking

The existing `FileLock` in `internal/install/filelock_unix.go` and `filelock_windows.go` covers both POSIX `flock(2)` and Windows `LockFileEx`. The dedup mechanism in `trigger.go` uses this correctly — non-blocking probe lock, release, then spawn. This pattern is sound on both platforms.

### Go stdlib support for detachment

The Go standard library (`os/exec`) exposes `Cmd.SysProcAttr` as `*syscall.SysProcAttr`. On Linux/macOS you can set `Setpgid: true` or `Setsid: true`. On Windows you set `CreationFlags` with constants from `syscall` or `golang.org/x/sys/windows`. There is no cross-platform abstraction; callers must use build tags or `runtime.GOOS` switches. The `golang.org/x/sys` package (already a transitive dependency) provides the Windows-specific constants. No third-party library for cross-platform process detachment has meaningful adoption in the Go ecosystem; the standard pattern is build-tag-split files.

## Implications

**Detached subprocess is the most viable mechanism for background work that outlives the CLI process.** It avoids goroutine-lifetime problems, requires no system services, and matches the existing implementation in `trigger.go`. The pattern is already working on Linux and macOS.

The main gap is process group isolation. Without `Setpgid: true` (Linux/macOS) or `CREATE_NEW_PROCESS_GROUP` (Windows), the spawned checker can be killed by SIGHUP when a terminal closes, or can surface in shell job listings. This is fixable with two build-tagged files setting `SysProcAttr` appropriately.

**File-based deferred work** is viable as a complement: the current sentinel+cache-entry pattern already implements it for the result side. It's not viable as the sole mechanism for long-running network operations that need to complete promptly.

**Goroutines alone** are not viable for work that must survive parent exit. They are fine for work that must complete before the parent exits — but holding the parent open reintroduces blocking.

**OS scheduling** is ruled out by the project's no-daemon, no-system-deps philosophy.

**Windows** is not a release target today. The file locking infrastructure already handles Windows correctly. The only Windows gap in the spawner is missing `SysProcAttr` — if Windows support is ever added to releases, this will need a platform-specific file.

## Surprises

1. The existing `trigger.go` spawner is already the right architectural choice; the implementation gap is narrower than expected. The core mechanism (orphan sub-process, file-lock dedup, sentinel freshness check) is already correct. The missing piece is process group isolation, not the fundamental approach.

2. The `check-updates` command already redirects stdout/stderr to `/dev/null` inside its `RunE` handler — it silences itself without the parent needing to do anything special. This is clean.

3. `MaybeAutoApply` runs synchronously in `PersistentPreRun` and does actual installs before the user's command executes. If auto-apply is enabled and there are pending updates, this adds latency on every command invocation — not just when a check is due. This is separate from the spawner problem and is itself a blocking-path concern.

4. The `hook-env` command (called on every shell prompt via the shell hook) also calls `CheckAndSpawnUpdateCheck`. This means the update check trigger fires on every prompt change, not just explicit tsuku commands. The sentinel check keeps this cheap, but it's a slightly higher surface for the spawner to malfunction silently.

5. `autoinstall/run.go` uses `syscall.Exec` (process replacement) directly, which means on Windows this import will fail. This is already a latent Windows incompatibility in the codebase independent of the update spawner.

## Open Questions

1. **Process group isolation decision**: Should `trigger.go` be split into platform-specific files to set `SysProcAttr` with `Setpgid: true` on Linux/macOS and `CREATE_NEW_PROCESS_GROUP` on Windows? What is the acceptable risk of the current no-`SysProcAttr` state?

2. **`MaybeAutoApply` blocking**: The synchronous auto-apply in `PersistentPreRun` is a separate latency source. Should it be deferred to `PersistentPostRun`, run in a goroutine with a short timeout, or remain synchronous? (This is a design question, not a platform question, but it's closely related to the overall blocking problem.)

3. **Registry refresh blocking**: The stated user-visible wait ("up to a minute") isn't fully explained by `CheckAndSpawnUpdateCheck`, which is fast. Is the actual bottleneck the synchronous `MaybeAutoApply` install path, or is there a registry-refresh operation that also blocks `PersistentPreRun`? The `update-registry` command is explicit and user-invoked; it's not auto-triggered. Confirming the actual slow path would sharpen the fix.

4. **Windows release intent**: If Windows is added to GoReleaser targets, the `syscall.Exec` in `autoinstall/run.go` and the missing `SysProcAttr` in `trigger.go` both need addressing. Is Windows a future target?

5. **Orphan process cleanup**: If the spawned `check-updates` process is killed mid-run (machine suspend, SIGHUP), the lock file remains locked by a dead process. On Linux/macOS, `flock(2)` locks are automatically released on process death — this is correct and safe. On Windows, `LockFileEx` locks are also released on process exit. This is fine, but worth confirming explicitly in tests.

## Summary

The detached subprocess pattern already used in `internal/updates/trigger.go` is the correct mechanism for background work on Linux and macOS — the existing design handles goroutine-lifetime and system-footprint constraints correctly. Its key constraint is missing process group isolation: without `SysProcAttr{Setpgid: true}` (Linux/macOS) or `CREATE_NEW_PROCESS_GROUP` (Windows), the background checker can receive SIGHUP when a terminal closes, which is fixable with build-tagged platform files. The biggest open question is whether the user-visible blocking comes from the spawner path at all, or from `MaybeAutoApply` running synchronous installs in `PersistentPreRun` — confirming the actual slow path is prerequisite to knowing which change eliminates the wait.
