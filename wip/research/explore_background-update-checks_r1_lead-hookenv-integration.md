# Lead: What's the concrete hook-env modification needed?

## Findings

### Current hook-env Flow

The `hook-env` command (cmd/tsuku/hook_env.go) runs on every prompt via shell hooks (internal/hooks/tsuku-activate.bash/zsh/fish). The entire flow is:

1. Shell calls: `eval "$(tsuku hook-env bash)"`
2. hook-env.go reads cwd, loads config, calls `shellenv.ComputeActivation(cwd, prevPath, curDir, cfg)`
3. ComputeActivation returns nil (no-op) when cwd == curDir, otherwise loads .tsuku.toml and builds new PATH
4. FormatExports renders export statements or returns empty string
5. Shell evals the output

**Key property:** ComputeActivation already has the cwd change detection (lines 44-45 of activate.go). It returns nil when directory hasn't changed, producing zero output. The function returns quickly on the hot path.

### Stat Check Integration Point

The staleness check can be **embedded in hook-env.go immediately after the cwd == curDir early exit**. Since hook-env already runs on every prompt and already has access to the activated directory, the cost is minimal:

```go
// After ComputeActivation call, before FormatExports
result, err := shellenv.ComputeActivation(cwd, prevPath, curDir, cfg)

// NEW: Check update cache staleness
if shouldCheckUpdates(cfg, cwd) {
    spawnUpdateCheck(cfg)  // non-blocking spawn
}

// Continue as before
if result == nil {
    return nil
}
fmt.Print(shellenv.FormatExports(result, shell))
```

### Staleness Detection Mechanism

A single stat check on the cache file can determine staleness:

```go
func shouldCheckUpdates(cfg *config.Config, cwd string) bool {
    cachePath := filepath.Join(cfg.CacheDir, "update-check.json")
    info, err := os.Stat(cachePath)
    if err != nil {
        return true  // Cache doesn't exist or is unreadable -> check now
    }
    
    age := time.Since(info.ModTime())
    interval := 24 * time.Hour  // configurable
    return age > interval
}
```

This is **pure stat I/O** with no JSON parsing on the hot path. Under 1ms on local filesystems. Fully satisfies <5ms requirement.

### Detached Process Spawning

Reference implementation exists in internal/llm/lifecycle.go (lines 167-191). The pattern:

```go
func spawnUpdateCheck(cfg *config.Config) error {
    cmd := exec.Command(os.Args[0], "check-updates")
    cmd.Stdin = nil
    cmd.Stdout = nil
    cmd.Stderr = nil
    // SysProcAttr not needed - shell is parent, child becomes orphan
    // when shell exits. Go runtime handles cleanup.
    return cmd.Start()  // Non-blocking; don't Wait()
}
```

The key: **call Start(), not Run()**. Start() spawns the process and returns immediately. The goroutine in lifecycle.go (lines 182-191) that monitors the process is optional for long-lived daemons; a background update check can just Start() and exit hook-env.

**Important detail:** No SysProcAttr{Setsid: true} is needed. The shell is the parent; when shell exits, the spawned check process becomes an orphan under init/systemd and continues independently. This is the standard Unix pattern for background tasks.

### Separate Code Path vs. Embedded

**Integration within ComputeActivation is NOT recommended** because:

1. ComputeActivation's job is to compute PATH activation for a specific directory. Update checking is orthogonal infrastructure.
2. ComputeActivation is used by other code paths (e.g., direct `tsuku install` commands). Embedding the stat check there would cause checks on every command.
3. Layering (R5 of PRD) requires independent triggers: shell hook, shim, direct command. Each needs its own integration point.

**Better approach:** New function `CheckUpdateStaleness(cfg)` in a new package `internal/updates` that hook-env calls. Shims and direct commands call the same function independently.

### New Subcommand Required

A new `tsuku check-updates` subcommand is needed:

```go
var checkUpdatesCmd = &cobra.Command{
    Use:    "check-updates",
    Short:  "Check for updates in background",
    Hidden: true,
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, _ := config.DefaultConfig()
        // Use ProviderFactory to check all tools
        // Write results to $TSUKU_HOME/cache/update-check.json
        // Timeout: 10 seconds (R19)
        return performUpdateCheck(cfg)
    },
}
```

This is **not `tsuku run` delegation** but a dedicated entry point. It runs in background (spawned from hook-env) and has no user-facing output. Auto-apply logic (Feature 3) will read the cache later and install pending updates during subsequent tsuku commands.

### Cache File Location and Schema

The check process writes to: `$TSUKU_HOME/cache/update-check.json`

Schema:
```json
{
  "checked_at": "2026-03-31T10:22:00Z",
  "tools": {
    "node": {
      "current": "18.19.0",
      "available": "18.20.1",
      "within_boundary": true,
      "pin_level": "major"
    },
    "ripgrep": {
      "current": "13.0.0",
      "available": "14.0.1",
      "within_boundary": true,
      "pin_level": "minor"
    }
  }
}
```

The mtime of this file is the staleness marker. Consumers (auto-apply, notifications) read the entire JSON. Only the check trigger (hook-env, shims, commands) cares about mtime.

### Configuration Integration

The [updates] section in config.toml:

```toml
[updates]
enabled = true
auto_apply = true
check_interval = "24h"
```

Parsed by new `internal/userconfig` extension. Environment variable override: `TSUKU_UPDATE_CHECK_INTERVAL`.

## Implications

1. **Two-phase design is correct:** The stat check (hook-env) is separate from auto-apply (feature 3). This keeps prompt latency minimal and allows independent phasing.

2. **No ComputeActivation changes needed:** The PATH activation logic is complete as-is. Update checking is purely additive.

3. **Background spawning is proven safe:** The LLM lifecycle code demonstrates the pattern. Go's exec.Start() is well-understood and reliable. No locks or dedup needed—each prompt can spawn a check; the 24h cache ensures only one runs per day anyway.

4. **Hook-env footprint:** Adding the stat check adds ~3 lines of code and ~1ms latency (stat call). Well under the 5ms budget. The overhead is justified by reliable daily checks.

5. **Shims need the same logic:** The pattern in hook-env (stat + spawn) repeats in tsuku run. Extracting to a shared function is essential for consistency.

## Surprises

1. **No SysProcAttr needed.** This was my initial assumption but reviewing lifecycle.go clarified that Go's exec.Start() on Unix naturally produces an orphan when the parent exits. The shell hooks are the parent; the check process becomes init's child. No deamonization tricks required.

2. **ComputeActivation already has the fast path.** The cwd == curDir check (activate.go:44-45) means most prompts produce zero output. The stat check doesn't add latency to the common case; it's additional only when directory changes. This is actually better than expected.

3. **Cache schema is output-focused.** Downstream consumers (auto-apply, notifications) need structured data about all tools and their availability. The cache can't be just a timestamp; it must hold results. This means the check process does non-trivial work (queries providers), but the spawning from hook-env is still <5ms because it only stats the file.

## Open Questions

1. **Deduplication on concurrent prompts.** If a user opens two terminal windows at the same time, will both spawn a check? The 24h cache makes this benign (the second check will hit the same results), but multiple concurrent checks are wasteful. The LLM lifecycle uses a lock file to dedup; should update checks do the same?

2. **What's the exact error handling in performUpdateCheck?** If the check process times out (10s limit per R19), dies, or fails, what gets written to the cache? If the cache is missing/corrupt, auto-apply must handle gracefully.

3. **How do .tsuku.toml project pins influence the check results?** If the current directory has a .tsuku.toml with exact pins, those tools shouldn't be in the "available" list. Does the check process read .tsuku.toml, or does auto-apply filter later?

4. **Does the check process require terminal/TTY?** All provider queries should be network only (no TTY needed), but error handling and logging might expect a terminal. Should check-updates use a nohup pattern or redirect stderr/stdout to /dev/null?

## Summary

The stat check belongs in hook-env.go right after ComputeActivation, as a <1ms call to a new `CheckUpdateStaleness(cfg)` function that returns a bool. When true, hook-env spawns a detached `tsuku check-updates` process via `exec.Command().Start()` (no SysProcAttr needed). The check process runs independently, queries all providers in parallel, and writes structured JSON to $TSUKU_HOME/cache/update-check.json within a 10s timeout. This design keeps prompt latency under 5ms, separates concerns cleanly, and enables the layered trigger architecture (hook > shim > command) described in R5. No ComputeActivation changes are required; the update infrastructure is purely additive.

