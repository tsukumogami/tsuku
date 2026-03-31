# Decision 2: Trigger integration and spawn protocol

## Question

How do the three trigger layers (hook-env, tsuku run, direct commands) integrate with CheckUpdateStaleness, and what is the exact flock-based spawn dedup protocol for launching the detached background process?

## Options Considered

### Option A: Package-level function with inline flock+spawn

A single `CheckAndSpawnUpdateCheck(cfg *config.Config)` function in `internal/updates/` that performs staleness check, flock attempt, and spawn all in one call. Each trigger site calls it as a one-liner.

**API surface:**
```go
// internal/updates/trigger.go
func CheckAndSpawnUpdateCheck(cfg *config.Config) error
```

**Integration points:**
- hook-env.go: after `ComputeActivation()`, before `FormatExports()`
- cmd_run.go: after config load, before `runner.Run()`
- main.go PersistentPreRun: after `initLogger`, for all direct commands

**Pros:**
- Minimal caller-side code (one line per site)
- All logic encapsulated; callers can't forget flock or misorder steps
- Easy to test the function in isolation

**Cons:**
- No way to customize behavior per trigger (e.g., skip spawn for certain commands)
- Caller has no visibility into what happened (stale? spawned? skipped?)
- Mixes concerns: staleness detection and process spawning in one function

### Option B: UpdateTrigger struct with methods

An `UpdateTrigger` struct that encapsulates config, lock path, cache path, and provides `ShouldCheck()` and `SpawnIfNeeded()` methods. Constructed once, called at each site.

**API surface:**
```go
// internal/updates/trigger.go
type UpdateTrigger struct { ... }

func NewUpdateTrigger(cfg *config.Config) *UpdateTrigger
func (t *UpdateTrigger) ShouldCheck() bool       // stat-only, <1ms
func (t *UpdateTrigger) SpawnIfNeeded() error     // flock + spawn
func (t *UpdateTrigger) CheckAndSpawn() error     // convenience: ShouldCheck + SpawnIfNeeded
```

**Integration points:**
- hook-env.go: construct trigger, call `CheckAndSpawn()` after `ComputeActivation()`
- cmd_run.go: construct trigger, call `CheckAndSpawn()` before `runner.Run()`
- main.go PersistentPreRun: construct trigger, call `CheckAndSpawn()`

**Pros:**
- Separation of stat check and spawn allows callers to inspect intermediate state
- Struct holds derived paths (lock file, cache file) computed once
- `CheckAndSpawn()` convenience preserves the one-liner ergonomics of Option A
- Testable: mock the struct or test methods independently

**Cons:**
- More API surface than strictly needed for three call sites
- Struct allocation on every prompt (negligible, but non-zero)
- Possible over-engineering for what is fundamentally a check-then-act

### Option C: CheckUpdateStaleness returns bool, caller handles spawn

`CheckUpdateStaleness` returns a boolean. Each caller decides whether to spawn. A separate `SpawnUpdateCheck` function handles flock+detach.

**API surface:**
```go
// internal/updates/staleness.go
func CheckUpdateStaleness(cfg *config.Config) bool

// internal/updates/spawn.go
func SpawnUpdateCheck(cfg *config.Config) error
```

**Integration points:**
- hook-env.go: `if updates.CheckUpdateStaleness(cfg) { updates.SpawnUpdateCheck(cfg) }`
- cmd_run.go: same two-line pattern
- main.go PersistentPreRun: same two-line pattern

**Pros:**
- Clear separation: staleness is a question, spawn is an action
- Caller has full control over whether to spawn
- Functions are independently testable
- Minimal API -- two functions, no types

**Cons:**
- Callers must always pair the two calls; forgetting `SpawnUpdateCheck` silently drops updates
- Flock dedup is hidden inside `SpawnUpdateCheck` -- caller doesn't know if spawn was skipped
- Two function calls instead of one at every site (minor ceremony)

### Option D: Package-level function, PersistentPreRun only

Same as Option A but called from a single location: `rootCmd.PersistentPreRun`. This covers all commands including `hook-env` and `run` since they're subcommands of root.

**API surface:**
```go
// internal/updates/trigger.go
func CheckAndSpawnUpdateCheck(cfg *config.Config) error
```

**Integration points:**
- main.go PersistentPreRun only (single call site)

**Pros:**
- Single integration point -- impossible to forget a trigger
- Lowest maintenance burden
- All future commands automatically get update checks

**Cons:**
- Runs on commands that shouldn't trigger checks (e.g., `tsuku check-updates` itself, `tsuku version`, `tsuku help`)
- PersistentPreRun fires before config is fully loaded in some code paths
- No per-command opt-out without adding exclusion logic
- hook-env already runs PersistentPreRun through cobra; adding config load there may affect the <5ms budget since initLogger is currently the only PersistentPreRun work

## Chosen

Option A: Package-level function with inline flock+spawn

## Rationale

The three trigger sites need identical behavior: check staleness, attempt flock, spawn if needed. There is no use case for checking staleness without spawning, or for spawning without checking staleness first. The split in Option C creates a protocol that callers must follow correctly with no compile-time enforcement. Option B's struct adds API surface that isn't justified by the access patterns -- nobody needs `ShouldCheck()` alone.

Option D (PersistentPreRun only) is tempting for its simplicity but creates problems: it runs on `tsuku check-updates` itself (the background process), on `tsuku help`, and on `tsuku version`. Adding exclusion lists is worse than having three explicit call sites. The three triggers also have slightly different timing requirements -- hook-env needs the check after `ComputeActivation` (to have config loaded), while `run` needs it after mode resolution.

Option A gives the right balance: one function, one call per site, all dedup logic hidden. The function returns error for logging but callers ignore it (update checks are best-effort). If future triggers need different behavior, the function can accept options -- but that's not needed today.

The existing `FileLock` in `internal/install/filelock.go` lacks a non-blocking `TryLockExclusive` method. Option A's implementation will add `TryLockExclusive() error` to `FileLock` (returning an error when the lock is held), or use raw `syscall.Flock` directly as the LLM lifecycle code does. Either way, the flock logic is internal to the single function.

## Integration Points

### hook-env.go

Insert after `ComputeActivation` returns, before the nil check. The update check runs on every prompt, independent of whether PATH activation changed.

```go
// cmd/tsuku/hook_env.go, inside RunE

result, err := shellenv.ComputeActivation(cwd, prevPath, curDir, cfg)
if err != nil {
    return err
}

// Trigger background update check (best-effort, <1ms).
// Runs on every prompt; staleness check + flock dedup ensures
// at most one background process per check interval.
_ = updates.CheckAndSpawnUpdateCheck(cfg)

if result == nil {
    return nil
}
fmt.Print(shellenv.FormatExports(result, shell))
```

**Why here, not before ComputeActivation:** Config (`cfg`) must be loaded first. Placing it after ComputeActivation means it runs even on the no-op path (cwd == curDir returns nil result), which is correct -- we want the check on every prompt regardless of directory change.

### cmd_run.go (shim trigger)

Insert after config and user config are loaded, before the TTY gate and runner setup. This covers both shim invocations (`exec tsuku run ...`) and direct `tsuku run` usage.

```go
// cmd/tsuku/cmd_run.go, inside Run func

userCfg, err := userconfig.Load()
if err != nil { ... }

// Trigger background update check (best-effort).
_ = updates.CheckAndSpawnUpdateCheck(cfg)

mode, err := resolveMode(runModeFlag, userCfg)
```

**Why here:** The cfg is available, and it's before the potentially-blocking TTY prompt and `runner.Run()`. The check adds <1ms to the shim path.

### Direct commands

Insert in `PersistentPreRun` chain or in a cobra `OnInitialize` callback. Since `hook-env` and `run` have their own explicit calls, the PersistentPreRun call covers commands like `install`, `list`, `outdated`, `update`, etc.

```go
// cmd/tsuku/main.go, in init() or a wrapper around PersistentPreRun

rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
    initLogger(cmd, args)

    // Skip update check for commands that shouldn't trigger it.
    skip := map[string]bool{
        "check-updates": true,
        "hook-env":      true,  // has its own call
        "run":           true,  // has its own call
        "help":          true,
        "version":       true,
        "completion":    true,
    }
    if !skip[cmd.Name()] {
        cfg, err := config.DefaultConfig()
        if err == nil {
            _ = updates.CheckAndSpawnUpdateCheck(cfg)
        }
    }
}
```

**Alternative:** Instead of PersistentPreRun with a skip list, add the one-liner to each direct command's RunE. But the skip list approach is lower maintenance since new commands automatically get the check.

## Spawn Protocol

The complete sequence inside `CheckAndSpawnUpdateCheck`:

1. **Read config**: Determine check interval from `cfg` (default 24h). If updates are disabled (`updates.enabled = false`), return immediately.

2. **Stat cache file**: `os.Stat($TSUKU_HOME/cache/update-check.json)`. If the file exists and `time.Since(info.ModTime()) < interval`, return (cache is fresh). Cost: one stat syscall, <0.5ms.

3. **Open lock file**: `os.OpenFile($TSUKU_HOME/cache/update-check.lock, O_CREATE|O_RDWR, 0644)`. Cost: one open syscall.

4. **Try non-blocking flock**: `syscall.Flock(fd, LOCK_EX|LOCK_NB)`. If this returns `EWOULDBLOCK` (or any error), close fd and return -- another check is already running. Cost: one flock syscall, <0.1ms.

5. **Release probe lock**: `syscall.Flock(fd, LOCK_UN)` then `fd.Close()`. We only held the lock to confirm no background process is running. The background process will acquire its own lock.

6. **Spawn detached process**: `exec.Command(os.Args[0], "check-updates").Start()`. Set Stdin/Stdout/Stderr to nil. Do not call `Wait()`. Cost: fork+exec, <1ms for Start() to return.

7. **Return**: Total cost for the stale+spawn path: <2ms. Total cost for the fresh path (step 2 exits): <0.5ms.

**In the background process** (`tsuku check-updates`):

1. Acquire exclusive lock on `$TSUKU_HOME/cache/update-check.lock` (blocking -- we have time).
2. Re-check cache freshness (another process may have completed while we waited for the lock). If fresh, release lock, exit.
3. Set a 10-second context deadline.
4. Query version providers for all installed tools in parallel.
5. Write results to a temp file, then `os.Rename` to `$TSUKU_HOME/cache/update-check.json` (atomic on same filesystem).
6. Release lock, exit.

The lock is held for the entire duration of the check (steps 1-6). This prevents concurrent checks. The re-check in step 2 handles the race where two triggers both pass the staleness check before either acquires the lock.

## Assumptions

- `$TSUKU_HOME/cache/` directory exists or is created during tsuku initialization. The function should `os.MkdirAll` the cache dir if needed.
- `os.Args[0]` resolves to the tsuku binary. This holds for all three trigger paths (hook-env runs as `tsuku hook-env`, shim runs as `tsuku run`, direct commands run as `tsuku <cmd>`).
- The `internal/install.FileLock` type will gain a `TryLockExclusive() error` method, or the spawn function will use raw `syscall.Flock` directly (matching the LLM lifecycle pattern). Either approach works; raw syscall is simpler for a single use site.
- Advisory flock works correctly on all target platforms (Linux, macOS). The existing filelock_unix.go build tag confirms Unix-only support; Windows will need a separate implementation or a no-op stub (consistent with existing filelock_windows.go pattern).
- The `tsuku check-updates` hidden subcommand will be implemented as part of Feature 2. It does not exist yet.
- Config loading in PersistentPreRun for direct commands is acceptable because `config.DefaultConfig()` is already called in `init()` (main.go line 67). The second call returns cached state or is fast enough (<1ms).
