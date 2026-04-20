# Lead: How does tsuku's notification system work?

## Findings

### Model: Pull-based, file-backed, per-command display

The notification system is pull-based, not push. There is no daemon listening or writing to a socket. Each command invocation reads notice files from disk during `PersistentPreRun` and renders any unshown ones before executing the requested command.

Notices are JSON files stored at `$TSUKU_HOME/notices/<toolname>.json`. Each file holds one `Notice` struct with fields: `tool`, `attempted_version`, `error`, `timestamp`, `shown`, and `consecutive_failures`. The `internal/notices/` package handles all I/O atomically (write-to-temp, rename).

### What notices currently surface

`DisplayNotifications` in `internal/updates/notify.go` renders three categories, in order, to stderr:

1. **Auto-apply results from this invocation** -- tool updated successfully (`Updated foo 1.0 -> 1.1`) or update failed with rollback instructions (`Run 'tsuku notices' for details`). These come from `MaybeAutoApply` return values, not from disk files.

2. **Unshown notices from prior runs** -- read from `$TSUKU_HOME/notices/*.json` where `shown == false`. These cover both self-update outcomes (success or failure) and tool auto-update failures that survived the consecutive-failure suppression threshold (3 failures before showing). After rendering, each notice is marked `shown = true`.

3. **Available-update summary** -- e.g., `2 updates available. Run 'tsuku update' to apply.` Only shown when `auto_apply` is disabled. Uses a sentinel file `.notified` to deduplicate across commands within the same check cycle (sentinel is stale when the cache directory mtime is newer).

A fourth type, out-of-channel notifications (newer version exists outside pin boundary), is shown once per 7 days per tool, throttled by `.ooc-<toolname>` dotfiles.

### When DisplayNotifications is called

`PersistentPreRun` in `cmd/tsuku/main.go` calls the full sequence synchronously, before the requested command runs:

```
CheckAndSpawnUpdateCheck(cfg, userCfg)  // non-blocking spawn (<1ms)
results := MaybeAutoApply(...)          // synchronous -- can block
DisplayNotifications(cfg, userCfg, quietFlag, results)
```

`MaybeAutoApply` iterates cached update entries and calls `runInstallWithTelemetry` for each pending update. This is a full install operation executed in the pre-run hook, not in a background process. If multiple tools have pending updates, they all run serially before the requested command gets any CPU.

`CheckAndSpawnUpdateCheck` is non-blocking: it stat-checks a sentinel file, tries a non-blocking flock, then calls `spawnChecker()`, which does `exec.Command(binary, "check-updates").Start()` without `Wait()`. The spawned `check-updates` process runs with a 10-second context timeout, produces no output (stdout/stderr redirected to `/dev/null`), and writes results to `$TSUKU_HOME/cache/updates/<toolname>.json`.

### Cache refresh is not automatic -- it is a manual command

`tsuku update-registry` is the only way to refresh recipe cache on demand. There is no automatic background registry cache refresh. The recipe cache in `$TSUKU_HOME/registry/` is populated lazily on first use (`CachedRegistry.GetRecipe` fetches on cache miss) and has a TTL after which stale-but-not-too-old entries are served with a warning. There is no spawn-in-background path for registry refresh.

### Suppression rules

`ShouldSuppressNotifications` suppresses all notification output when:
- `TSUKU_NO_UPDATE_CHECK=1` is set
- `CI=true` is set
- `--quiet` flag is passed
- stdout is not a TTY (scripted context)

Explicit opt-in `TSUKU_AUTO_UPDATE=1` overrides suppression.

### State files used for notification persistence

| File | Location | Purpose |
|------|----------|---------|
| `<toolname>.json` | `$TSUKU_HOME/notices/` | Per-tool failure/success notices with `shown` flag |
| `<toolname>.json` | `$TSUKU_HOME/cache/updates/` | Per-tool update check cache entries (version available, checked_at, expires_at) |
| `.last-check` | `$TSUKU_HOME/cache/updates/` | Global sentinel: mtime tracks when last `check-updates` completed |
| `.notified` | `$TSUKU_HOME/cache/updates/` | Dedup sentinel: prevents repeated available-update summary within one check cycle |
| `.ooc-<toolname>` | `$TSUKU_HOME/cache/updates/` | Per-tool throttle file for out-of-channel notifications (7-day interval) |
| `.lock` | `$TSUKU_HOME/cache/updates/` | Advisory flock to deduplicate concurrent `check-updates` spawns |
| `.self-update.lock` | `$TSUKU_HOME/cache/updates/` | Advisory flock for self-update dedup |

### The `tsuku notices` command

`cmd/tsuku/cmd_notices.go` provides a user-facing command that reads all notices from `$TSUKU_HOME/notices/` (both shown and unshown) and renders a table with tool, version, error, and timestamp. It shows all historical failures until the notice file is removed (which happens on successful update via `notices.RemoveNotice`).

## Implications

### The notice channel can carry background activity status

The technical mechanism already exists: write a JSON file to `$TSUKU_HOME/notices/` with `shown = false`, and `DisplayNotifications` will render it on the next command invocation. The `Notice` struct currently only carries failure information (`Error` field), but the display logic in `notify.go` already has a branch for `n.Error == ""` (success notices from self-update and tool updates). A background process completing a cache refresh or update check could write a synthetic notice that gets displayed to the user on the next command.

However, the current `Notice` type is tightly scoped to update failures and successes for named tools. It has no `Kind` or `Category` field to distinguish "update failed" from "registry cache refreshed." Adding a new notice kind would require a schema extension and changes to the rendering logic.

### MaybeAutoApply is the actual blocking operation

The framing in the exploration context mentions "auto-update and cache-refresh operations block commands." For update checks, `CheckAndSpawnUpdateCheck` is already non-blocking -- it spawns a detached child. The blocking operation is `MaybeAutoApply`, which runs full tool installs in `PersistentPreRun`. This is synchronous and can take as long as a complete `tsuku install` run for each pending update.

Registry cache refresh has no automatic trigger during normal commands. Users only block on it if they run `tsuku update-registry` explicitly, or if a recipe cache miss forces a network fetch during install (which is per-recipe, not a bulk operation).

### Deferred display requires no new IPC

Because notification state is entirely file-based and polled per command, a background process can signal completion by writing a notice file. There is no socket, pipe, or shared memory involved. The `DisplayNotifications` call in `PersistentPreRun` already picks this up without modification -- as long as the background process writes a notice file with `shown = false` before the next command runs.

### Suppression is already TTY-aware

Scripted contexts (non-TTY stdout, `CI=true`, `--quiet`) already suppress all notification output. Any new background-activity notices would automatically respect these same rules if routed through `ShouldSuppressNotifications`.

## Surprises

### MaybeAutoApply blocks the requested command, not just notifications

The exploration context notes that "tsuku already has a notification mechanism." But the problem isn't notification display -- that's lightweight. The problem is that `MaybeAutoApply` is called in `PersistentPreRun` and performs actual installs before the requested command runs. A user running `tsuku install foo` could wait through one or more background auto-updates before their own command starts. Moving auto-apply to a background process (similar to the check-updates pattern) would address the blocking without any changes to the notification system.

### Registry cache refresh is not the problem described

The exploration context mentions "cache refreshes" as a source of blocking. Based on the code, recipe cache refreshes do not happen automatically -- only `tsuku update-registry` triggers them. The blocking observed during `tsuku install` is more likely from `MaybeAutoApply` running installs synchronously, or from network fetches during per-recipe cache misses on first install. This is a narrower problem than "cache refresh."

### The Notice struct would need extension for new notice kinds

If background processes (update check, auto-apply, registry refresh) want to write status notices, the current `Notice` type covers only tool name, version, and error string. Reporting "registry refreshed 42 recipes" or "update check completed" would require either repurposing the `error` field as a generic message field or adding a new `Kind` field. Either change would need backward-compatible deserialization handling since old notice files exist on disk.

## Open Questions

1. **Is MaybeAutoApply the primary latency source?** The code shows it runs synchronously in `PersistentPreRun`, but we don't know how often users have pending updates in practice or whether this is the dominant wait observed.

2. **What does "cache refresh" specifically mean to users who reported blocking?** The exploration context says refreshes cause up to a minute of waiting. Recipe-by-recipe lazy fetches shouldn't take a minute unless many recipes are stale simultaneously. Is the block from `MaybeAutoApply` (installs), from `tsuku update-registry` (explicit), or from something else?

3. **Should MaybeAutoApply move to a background process?** The current design applies updates inline before commands run. Moving it to the `check-updates` subprocess (run apply after check, write notice on completion) would eliminate the blocking. The trade-off: users see "update applied" notices on the next command rather than inline. Is that acceptable?

4. **Should the Notice struct gain a Kind field?** To use the notice channel for non-failure status (e.g., "auto-update applied in background"), the struct needs extension. A `Kind` field (e.g., `update_success`, `update_failure`, `self_update_success`, `cache_refresh`) would make the schema explicit. The display logic would need to handle new kinds.

5. **Is there a user-visible race between background check-updates and MaybeAutoApply?** `check-updates` writes cache entries; `MaybeAutoApply` reads them. If `check-updates` is still running when the next command starts, `MaybeAutoApply` reads whatever partial results exist. This is likely benign (entries are written atomically) but worth confirming.

## Summary

The notice system is a pull-based, file-backed mechanism where background processes write JSON files to `$TSUKU_HOME/notices/` and `DisplayNotifications` reads and clears them on the next command invocation -- it already supports background activity reporting without new IPC. The blocking problem is not in the notification display itself but in `MaybeAutoApply`, which runs full tool installs synchronously in `PersistentPreRun` before the requested command executes; moving auto-apply into the background (write a notice on completion, as self-update already does) would fix the wait without any notification infrastructure changes. The biggest open question is whether `MaybeAutoApply` is the actual source of the blocking users experience, or whether something else -- recipe network fetches, explicit `update-registry` runs -- accounts for the reported minute-long waits.
