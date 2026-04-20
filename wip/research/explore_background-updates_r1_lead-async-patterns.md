# Lead: What background/async patterns exist in the codebase?

## Findings

### Pattern 1: Detached subprocess via `cmd.Start()` (updates/trigger.go)

The primary async pattern for update checks is a detached subprocess. `CheckAndSpawnUpdateCheck` in `internal/updates/trigger.go` re-executes the tsuku binary as a detached child process running `check-updates`:

```go
cmd := exec.Command(binary, "check-updates")
cmd.Stdin = nil
cmd.Stdout = nil
cmd.Stderr = nil
if err := cmd.Start(); err != nil { ... }
// Don't Wait() -- the process runs independently
```

No `Wait()` is called, so the child outlives the parent. The parent returns immediately after `Start()`. There is no `SysProcAttr` with `Setpgid` or `Setsid` to formally detach the process from the terminal session, but because stdin/stdout/stderr are all nil, the process runs silently in the background.

This pattern uses an advisory file lock (`install.NewFileLock`) with a non-blocking `TryLockExclusive` to prevent duplicate spawns. The lock is probed, then immediately released before spawning -- the background process acquires its own lock via `RunUpdateCheck` in `checker.go`.

This is the **established pattern** for background work in tsuku's update system.

### Pattern 2: Goroutine-based fan-out with `sync.WaitGroup` + channel (index/rebuild.go)

`internal/index/rebuild.go` uses a bounded goroutine pool (10 workers) for concurrent recipe fetching during index rebuild:

```go
sem := make(chan struct{}, 10)
var mu sync.Mutex
var wg sync.WaitGroup

for _, name := range names {
    wg.Add(1)
    go func(n string) {
        defer wg.Done()
        sem <- struct{}{}   // acquire
        defer func() { <-sem }()
        // fetch and append to shared results
        mu.Lock()
        results = append(results, result{n, data})
        mu.Unlock()
    }(name)
}
wg.Wait()
```

This is synchronous from the caller's perspective (the `wg.Wait()` blocks), but it uses goroutines for bounded parallelism within a single operation. The `sync.Mutex` guards the shared slice.

### Pattern 3: Parallel ecosystem probing with WaitGroup + channel (discover/ecosystem_probe.go)

`internal/discover/ecosystem_probe.go` launches all probers in parallel, collects results through a buffered channel, and closes it with a monitor goroutine:

```go
ch := make(chan probeOutcome, len(p.probers))
var wg sync.WaitGroup
for _, prober := range p.probers {
    wg.Add(1)
    go func(pr builders.EcosystemProber) {
        defer wg.Done()
        result, err := pr.Probe(ctx, toolName)
        ch <- probeOutcome{...}
    }(prober)
}
go func() {
    wg.Wait()
    close(ch)
}()
for outcome := range ch { ... }
```

The caller blocks on the `range ch` loop until all probers complete. A context with timeout bounds total wait time.

### Pattern 4: Fire-and-forget goroutines for telemetry (internal/telemetry/client.go)

All five `Send*` methods in the telemetry client use fire-and-forget goroutines:

```go
// Fire-and-forget: spawn goroutine, no waiting
go c.send(event)
```

No WaitGroup or synchronization. The goroutine is abandoned when the process exits, which is acceptable because telemetry is non-critical. This is the simplest async pattern in the codebase.

### Pattern 5: cmd.Start() for LLM daemon lifecycle (internal/llm/lifecycle.go)

`internal/llm/lifecycle.go` starts the `tsuku-llm` addon binary with `cmd.Start()` (non-blocking), then monitors process exit in a background goroutine:

```go
if err := cmd.Start(); err != nil { ... }
go func() {
    _ = cmd.Wait()
    s.mu.Lock()
    s.process = nil
    s.lockFd.Close()
    s.mu.Unlock()
}()
return s.waitForReady(ctx)
```

Unlike the update check subprocess, this one is actively managed: the lifecycle manager holds a reference to the process, polls for readiness via Unix socket connection attempts, and can stop it via SIGTERM or gRPC. A `sync.Mutex` protects lifecycle state. This is a more sophisticated pattern than the fire-and-forget subprocess.

### Pattern 6: Signal-handling goroutine in main (cmd/tsuku/main.go)

A single goroutine handles SIGINT/SIGTERM to cancel the global context:

```go
go func() {
    sig := <-sigChan
    globalCancel()
    <-sigChan // second signal forces exit
    exitWithCode(ExitCancelled)
}()
```

This is infrastructure, not a worker pattern, but it establishes that goroutines are used in main for lifecycle control.

### Where MaybeAutoApply and CheckAndSpawnUpdateCheck are called

Both are invoked synchronously in `PersistentPreRun` (before every command except those in the skip list):

```go
rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
    initLogger(cmd, args)
    // ...
    updates.CheckAndSpawnUpdateCheck(cfg, userCfg)     // fast: stat + optional fork
    results := updates.MaybeAutoApply(...)              // potentially slow: install loop
    updates.DisplayNotifications(...)
}
```

`CheckAndSpawnUpdateCheck` is fast by design (a stat call plus one non-blocking flock attempt), but `MaybeAutoApply` is blocking and synchronous -- if there are pending updates, it calls `installFn` for each one before the user's command runs.

### Cache refresh on install

Registry cache refresh is **not** automatic during `tsuku install`. The `CachedRegistry.GetRecipe` method checks freshness inline when a recipe is loaded -- if expired and the network succeeds, it refreshes synchronously. If the cache is stale and the network fails, it falls back to stale content. There is no background proactive refresh; the recipe is fetched on demand when the install command needs it.

`tsuku update-registry` is the explicit, manual command for refreshing all cached recipes. It runs synchronously and serially (no goroutines, just a loop calling `cachedReg.Refresh`).

### Lock infrastructure (install/state.go)

`install.NewFileLock` provides `LockExclusive` (blocking) and `TryLockExclusive` (non-blocking) using `flock(2)`. This is used by both the update checker (non-blocking probe to avoid duplicate spawns) and the background check process itself (blocking acquire while running). The same lock gates `MaybeAutoApply` via a non-blocking try-lock on `state.json.lock`.

## Implications

1. **The detached-subprocess pattern is established and working** for the update checker. Any new background work (e.g., registry refresh) would naturally fit this pattern: spawn `tsuku <internal-cmd>` via `cmd.Start()`, suppress output, don't `Wait()`.

2. **MaybeAutoApply is the actual blocking path for most users**, not `CheckAndSpawnUpdateCheck`. The spawn is fast; applying cached updates inline before the command runs is what produces visible latency. If auto-apply were also moved to a background process (or deferred post-exit), the wait would disappear.

3. **Goroutines with WaitGroup are used for parallelism within a synchronous operation** (index rebuild, ecosystem probing), not for deferring work past command exit. These patterns don't apply directly to the background-updates problem.

4. **No persistent daemon exists** for tsuku itself (only the optional `tsuku-llm` addon uses a persistent daemon). Any new background mechanism would need to fit the fire-and-forget subprocess model already established in `trigger.go`.

5. **The file lock system already handles deduplication and concurrency** between parent and child processes. Extending the same lock pattern to a registry-refresh subprocess would require no new primitives.

6. **`MaybeAutoApply` does have a non-blocking probe**: it calls `TryLockExclusive` on `state.json.lock` and silently skips if another tsuku is running. So the install loop itself is concurrent-safe, but still synchronous from the user-command perspective.

## Surprises

- `MaybeAutoApply` runs in `PersistentPreRun`, meaning it runs **before** the user's actual command starts printing output. For users with several pending auto-updates, this is an invisible installation phase of potentially significant duration.

- The detached subprocess in `trigger.go` does **not** use `SysProcAttr.Setsid` or `Setpgid`. On Linux/macOS, this means the background process is still in the same process group and would receive terminal signals (SIGINT) if the user Ctrl-C's within a very short window after spawning. Most process managers would use `Setsid` to fully orphan it.

- `update-registry` (registry cache refresh) is entirely manual and synchronous -- there is no equivalent of `CheckAndSpawnUpdateCheck` for registry cache staleness. If a recipe cache is expired at install time, the network fetch blocks inline.

- The `hook-env` command also calls `CheckAndSpawnUpdateCheck`, meaning every shell prompt evaluation (if using the shell hook) can trigger the background check. This is intentional and is already fast due to the sentinel file stat check.

## Open Questions

1. **What is the measured latency of `MaybeAutoApply`?** The code iterates all pending updates and installs each one. For N pending updates, this is N sequential install operations before the user's command runs. Is this the source of the reported minute-long waits, or is it something else?

2. **Should `MaybeAutoApply` also be moved to a background subprocess?** The fire-and-forget subprocess pattern is proven. The main reason it was kept synchronous may be UX (show update results inline), but this could be solved via the notices system (already used for async results from prior runs).

3. **Is the missing `Setsid` in `spawnChecker` a known gap?** Formally orphaning the subprocess would make the background check resilient to terminal signals. On most workloads this doesn't matter (the Ctrl-C window is tiny), but it's worth documenting.

4. **Is registry cache staleness a source of latency on first-install for new users?** If no cache exists, the install command fetches the recipe from the network inline. Is this the blocking scenario described in the exploration question, or is it auto-apply latency?

5. **Can `MaybeAutoApply` be made opt-in-async?** Users who want zero pre-command latency could get it by moving auto-apply to a background subprocess that writes results to the notices system for display on the next command. What are the failure modes (e.g., a broken update installing while the user is in the middle of something)?

## Summary

Tsuku has two distinct async patterns: a fire-and-forget detached subprocess (used by the update checker via `cmd.Start()` without `Wait()`) and goroutine fan-out with WaitGroup (used for parallelism within single operations like index rebuild). The primary source of blocking latency is `MaybeAutoApply`, which runs auto-updates synchronously in `PersistentPreRun` before the user's command executes, using the same process as the main command rather than any background mechanism. The biggest open question is whether moving `MaybeAutoApply` to the same detached-subprocess pattern as the update check would eliminate the latency without sacrificing reliability.
