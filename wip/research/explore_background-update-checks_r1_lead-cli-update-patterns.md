# Lead: How do other CLI tools implement background update checks?

## Findings

### Homebrew Pattern
- **Mechanism**: Uses launchd on macOS to manage background processes, not self-spawned detached processes
- **Schedule**: Default interval is 86400 seconds (24 hours); can be configured as `StartInterval` or `StartCalendarInterval` in the plist
- **Configuration**: Created via user-installed tap (`brew autoupdate start [interval]`)
- **Dedup**: launchd manages single process execution; no PID file or lock file needed at tool level
- **Cache**: Relies on implicit staleness via launchd scheduling, not explicit mtime checks
- **Limitation**: Heavily macOS-specific (launchd only); not portable to Linux/Windows without third-party solutions
- **Source**: [DomT4/homebrew-autoupdate](https://github.com/DomT4/homebrew-autoupdate), [homebrew-autoupdate README](https://github.com/DomT4/homebrew-autoupdate/blob/master/README.md)

### Rustup Pattern
- **Mechanism**: Checks for updates in a background process when `cargo` or `rustup` commands are invoked
- **Non-blocking**: Fire-and-forget; check is spawned without waiting for results
- **Configurability**: Weekly check-and-download by default; users can choose frequency/download combinations
- **Recent feature** (rustup 1.29.0): Concurrent update checking enabled via background goroutine spawning
- **Exit codes**: New feature uses different exit codes (100 for updates, 0 for no updates) for automation
- **Source**: [Rustup basics](https://rust-lang.github.io/rustup/basics.html), [rustup 1.29.0 announcement](https://blog.rust-lang.org/2026/03/12/Rustup-1.29.0/)

### GitHub CLI (gh) Pattern
- **Mechanism**: Architecture uses factory-based dependency injection with lazy-loaded subsystems
- **Background check**: Limited public documentation on background update mechanisms
- **Architecture note**: Cobra-based command system with decoupled API, git, and config subsystems
- **Source**: [GitHub CLI repository](https://github.com/cli/cli)

### Mise Version Manager Pattern
- **Caching strategy**: Caches version lists daily (24-hour default); uses centralized https://mise-versions.jdx.dev service
- **Commands**: `mise outdated` (check), `mise upgrade` (apply), `mise self-update` (self-update)
- **Cache cleanup**: `mise cache clean` explicitly invalidates cache
- **Performance**: ~10ms hook-env execution via Rust implementation (vs asdf's ~120ms with shell scripts)
- **No explicit background spawn**: Updates are explicitly triggered, not run in background
- **Source**: [mise documentation](https://mise.jdx.dev/), [mise vs asdf comparison](https://betterstack.com/community/guides/scaling-nodejs/mise-vs-asdf/)

### asdf Version Manager Pattern
- **Performance penalty**: Adds ~120ms overhead per shim invocation due to shell script overhead
- **Plugin model**: Shell script-based plugins; slower than native implementations
- **No background check**: No built-in background update mechanism documented
- **Limitation**: Performance makes real-time checks prohibitive
- **Source**: [asdf documentation](https://asdf-vm.com/), [mise vs asdf comparison](https://betterstack.com/community/guides/scaling-nodejs/mise-vs-asdf/)

### NVM (Node Version Manager) Pattern
- **Background load issue**: `nvm use lts` background process may not complete before subsequent commands
- **Race condition**: Known issue where shell completion uses background loading
- **Timeout issues**: Installation can hang indefinitely with no output
- **Workaround**: Initial loading can be done in background via env var hooks, but not blocking-safe
- **Source**: [NVM issues](https://github.com/nvm-sh/nvm/issues)

### NPM/Yarn/pnpm Package Managers Pattern
- **pnpm background server**: pnpm has an optional store server that runs in background after installation
- **Isolation model**: pnpm's strict resolution and symlink-based isolation differs from npm/yarn
- **No explicit update check**: Version checking is part of install phase, not separated into background process
- **Source**: [pnpm documentation](https://pnpm.io/), [npm/yarn/pnpm comparison](https://www.deployhq.com/blog/choosing-the-right-package-manager-npm-vs-yarn-vs-pnpm-vs-bun)

## OS-Level Patterns for Detached Process Spawning

### Linux/Unix Double Fork Pattern
- **Standard approach**: Fork, setsid() to create new session, then second fork to prevent terminal reacquisition
- **Result**: Grandchild process survives parent termination and is adopted by init
- **Rationale**: First fork creates session leader; second fork ensures process is not a session leader (can't acquire tty)
- **Trade-off**: Adds complexity; not needed if parent process survives or uses systemd socket activation
- **Source**: [UNIX daemonization](https://0xjet.github.io/3OHA/2022/04/11/post.html), [Double fork and setsid](https://www.oreilly.com/library/view/mastering-bash/9781784396879/74b3ea0c-a21e-4987-8e74-6616f32007ce.xhtml)

### Go stdlib Pattern
- **cmd.Start() vs cmd.Run()**: Start() spawns and returns immediately; Run() waits for completion
- **Detachment in Go**: Set `SysProcAttr.Setpgid=true` to create separate process group, or `Setpgid=true` + `Setsid=true` for full detachment
- **Process inheritance**: Child processes inherit parent's file descriptors; closing parent doesn't kill child if detached
- **Recommended**: Goroutines for concurrent work (cheaper than processes); detached processes only for long-lived daemons
- **Source**: [Go os/exec patterns](https://www.dolthub.com/blog/2022-11-28-go-os-exec-patterns/), [Starting detached processes in Go](https://linuxvox.com/blog/start-a-process-in-go-and-detach-from-it/)

### Python subprocess Pattern
- **subprocess.Popen()**: Non-blocking; returns immediately, parent continues
- **subprocess.run()/call()**: Blocking; waits for completion
- **Async approach**: asyncio.create_subprocess_exec() for async/await integration
- **Redirect output**: stdout/stderr can be redirected to log file
- **Source**: [Python subprocess tutorial](https://www.codecademy.com/article/python-subprocess-tutorial-master-run-and-popen-commands-with-examples), [Python non-blocking processes](https://sqlpey.com/python/python-non-blocking-process-spawning/)

## Cache Staleness Detection Patterns

### Timestamp-based (mtime)
- **Mechanism**: Store Unix timestamp or ISO8601 in JSON cache file; compare against current time on read
- **Example**: Claude Code plugins cache `.last-update-check` as Unix timestamp, skip if elapsed < configured interval
- **Precision**: Millisecond-level precision available; typically 1-day intervals (86400 seconds)
- **File format**: JSON with version, tools array, each tool with {version, timestamp, pin}
- **Source**: [Claude Code plugin update check](https://github.com/anthropics/claude-code/issues/31462), [JSON schema versioning](https://json-schema.org/specification)

### Lock/PID File Dedup
- **Purpose**: Prevent multiple simultaneous checks if shell hook fires while previous check running
- **Pattern**: Create `.lock` or `.pid` file before starting check; remove after completion
- **Staleness detection**: Use `kill -0 $pid` to check if process still running; if not, file is stale
- **Advantage**: Works across shell invocations; doesn't require parent-child relationship
- **Disadvantage**: Vulnerable to unclean shutdown leaving stale locks
- **Recommended storage**: /run or /tmp with automatic cleanup on reboot
- **Source**: [PID file stale detection](https://github.com/trbs/pid), [Handling stale PID files](https://forums.ni.com/t5/NI-Linux-Real-Time-Discussions/Handling-stale-PID-files/td-p/3451761)

## Timeout Handling Patterns

### Go context.WithTimeout
- **Idiomatic**: signal.NotifyContext() for OS signals; context.WithTimeout() for operation limits
- **Pattern**: Pass context to all blocking operations; cancel propagates to all goroutines
- **Graceful shutdown**: Drain in-flight requests before timeout expires
- **Source**: [Graceful shutdown in Go](https://victoriametrics.com/blog/go-graceful-shutdown/)

### Shell Hook Timeout
- **Claude Code pattern**: SessionEnd hooks have 1.5s default timeout; configurable via CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS
- **Watchtower pattern**: 60s default for all lifecycle commands; overridable per-command
- **Non-blocking requirement**: Hook must return immediately; detached process runs async
- **Source**: [Claude Code hooks](https://code.claude.com/docs/en/hooks)

## Tsuku-Relevant Patterns

### Fire-and-Forget Goroutine Model (Already Implemented)
Tsuku's telemetry client demonstrates the pattern tsuku should reuse:
```go
// Fire-and-forget: spawn goroutine, no waiting
go c.sendJSON(event)
```
- **Timeout**: Uses context.WithTimeout(context.Background(), 2s) for HTTP requests
- **Silent failure**: Goroutine silently ignores errors; no retry
- **Non-blocking**: Spawn returns immediately; parent continues
- **Source**: [tsuku telemetry/client.go](internal/telemetry/client.go:95-111)

### No Explicit Background Spawn Infrastructure
- **Current state**: Only telemetry uses fire-and-forget goroutines
- **No daemon/detach infrastructure**: No double-fork, SysProcAttr config, or process group isolation
- **No cache file pattern**: No mtime-based staleness detection implemented
- **No config section**: No [updates] config; only [telemetry] and [llm] sections exist
- **Source**: [tsuku/internal/userconfig/userconfig.go](internal/userconfig/userconfig.go:30-95)

## Implications

### 1. Avoid OS-specific solutions
Homebrew's launchd approach is elegant but not portable. tsuku should use in-process goroutine spawning (like rustup) rather than relying on systemd/launchd. This keeps the codebase unified and works on all platforms.

### 2. Use fire-and-forget spawning with timeouts
The tsuku telemetry pattern is reusable: spawn a goroutine with context.WithTimeout(). This ensures:
- Zero blocking latency (hook-env requirement < 5ms for staleness check)
- Natural timeout handling (2-10s timeout allows work to complete off-critical path)
- Silent failures (no retry, no blocking on error)

### 3. Cache file format should be simple
JSON with Unix timestamps is the pattern (Claude Code, mise). Propose:
```json
{
  "version": 1,
  "tools": [
    {
      "name": "go",
      "latest_version": "1.21.0",
      "current_version": "1.20.5",
      "checked_at": 1704067200,
      "pin_boundary": "1.20.*"
    }
  ],
  "checked_at": 1704067200
}
```

### 4. Staleness detection with minimal overhead
Don't require double-fork or systemd. Instead:
- **In hook-env** (runs every prompt): Stat the cache file mtime; if > 24h old, spawn check
- **In tsuku run** (also runs frequently): Same check; both paths can fire
- **Dedup via lock file**: Create .lock before check; remove after. Skip if lock exists and process is alive
- This adds ~1ms to hook-env (single stat call) and stays under 5ms requirement

### 5. Shell hook trigger is the critical path
R5 requires "layered triggers" but R19 requires "zero added latency." The solution:
- **hook-env**: Simple mtime check only (1ms); if stale, spawn detached check
- **shim (tsuku run)**: Same check; skip if lock exists
- **explicit `tsuku check-updates` command**: Can be called manually
- Don't add staleness check to every shim invocation (would violate R19)

### 6. Config surface is minimal
Add [updates] section to userconfig:
```toml
[updates]
enabled = true
interval_hours = 24
timeout_seconds = 10
check_on_hook = true
check_on_run = true
```

### 7. Timeout handling follows Claude Code pattern
Use context.WithTimeout(context.Background(), duration) in the spawned goroutine. If check takes > 10s (R19), it's silently abandoned. Parent never waits.

## Surprises

1. **No tool implements real background daemon infrastructure**: Rustup, Homebrew (via launchd), and others rely on OS schedulers or trigger on tool invocation. None spawn true detached daemons with process group isolation. tsuku could be simpler by not trying.

2. **Performance difference between Rust and shell is massive**: mise's 10ms vs asdf's 120ms overhead makes asdf unsuitable for frequent checks. tsuku being written in Rust means hook-env can be called frequently (every prompt) without guilt; asdf can't do this.

3. **Lock file dedup is more robust than timestamps**: mtime-based staleness works but can race. Lock files (check if process alive via kill -0) are the pattern Unix daemons use. Claude Code's timestamp approach is simpler but lacks dedup.

4. **Goroutines are cheaper than processes**: Go's concurrency model means spawning goroutines for checks (à la rustup 1.29.0) is safer than forking processes. Tsuku should prefer goroutines over detached processes for update checks.

5. **Cache file doesn't need to be per-tool**: Single `update-check.json` with array of tools is cleaner than individual files. Mise uses centralized service; Claude Code uses single timestamp file.

## Open Questions

1. **Should the check include self-update (Feature 4) or only tool updates?** Rustup treats both the same way (single background check). Tsuku's scope unclear from PRD.

2. **Does the spawned check process need to survive shell logout?** If hook-env spawns a goroutine (not a subprocess), it's tied to tsuku's process. If spawning a subprocess, it can survive. Current requirements don't clarify.

3. **What's the exact cache file location?** Suggest `~/.tsuku/cache/update-check.json`. Should it be git-ignored in project .tsuku.toml files?

4. **How do notifications integrate?** Feature 5 (notification display) consumes check results but format unclear. Does it read the cache file or subscribe to a channel?

5. **Should checks be rate-limited per-tool or global?** A slow tool shouldn't block checks of fast tools. Consider per-tool timeout with global overall timeout.

6. **What's the fallback if background check fails?** Should hook-env/run warn the user, or silently skip? Prefer silent skip (non-blocking contract).

## Summary

Cli tools implement background update checks via fire-and-forget spawning (rustup uses goroutines; Homebrew uses launchd) with simple JSON timestamp caches and optional lock-file dedup; tsuku should adopt rustup's fire-and-forget goroutine pattern with a context.WithTimeout to honor the 10s timeout in R19, use a single update-check.json cache file with Unix timestamps for staleness detection, and add a <5ms mtime check to hook-env to trigger spawning only when stale, avoiding any daemon infrastructure or OS-specific scheduling like launchd.

