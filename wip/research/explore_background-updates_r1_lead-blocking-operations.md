# Lead: Which commands trigger blocking update or cache-refresh operations?

## Findings

### The PersistentPreRun hook: universal trigger

Every tsuku command except a small skip-list runs two operations in `PersistentPreRun` before the command itself executes (`cmd/tsuku/main.go` lines 60–86):

1. `updates.CheckAndSpawnUpdateCheck(cfg, userCfg)` — checks a sentinel file and spawns a detached background process if the check is stale.
2. `updates.MaybeAutoApply(cfg, userCfg, projCfg, installFn, tc)` — reads cached update entries and, if auto-apply is enabled, calls `runInstallWithTelemetry` for each pending update before the user's command proceeds.

Commands excluded from this hook:

```
check-updates, hook-env, run, help, version, completion, self-update
```

All other commands — `install`, `update`, `remove`, `list`, `outdated`, `search`, `info`, `verify`, `update-registry`, `recipes`, `versions`, `config`, `doctor`, `shellenv`, `create`, `validate`, `eval`, `plan`, `activate`, `cache`, `registry`, `which`, `suggest`, `hook`, `shell`, `init`, `llm` — go through `PersistentPreRun`.

### What blocks and what doesn't

**Non-blocking (fast):**

`CheckAndSpawnUpdateCheck` is designed to be non-blocking. It checks a sentinel file mtime (<0.5 ms per the code comment), acquires a non-blocking flock to prevent duplicate spawns, releases the lock, and calls `exec.Command(binary, "check-updates").Start()` without waiting. The spawned process runs independently and survives parent exit. Errors at every step are swallowed. This is documented as `<1ms` in `cmd/tsuku/hook_env.go`.

**Blocking: MaybeAutoApply**

`MaybeAutoApply` (`internal/updates/apply.go`) is the problematic call. It runs synchronously in `PersistentPreRun` before the user's command. Its execution path:

1. Reads all cached update entries from `$TSUKU_HOME/cache/updates/*.json`.
2. Filters for actionable entries (`LatestWithinPin != ActiveVersion`).
3. Probes `state.json.lock` with a non-blocking try-lock. If held by another process, skips silently.
4. For each pending update, calls `installFn(toolName, version, constraint)` which is `runInstallWithTelemetry` — the full install pipeline (download, extract, install binaries, activate symlinks, shell.d file creation, GC).

A user with three outdated tools and auto-apply enabled will run three full tool installs before their `tsuku list` command completes. Each install involves at minimum one HTTP download.

The check-updates background process (`cmd/tsuku/cmd_check_updates.go`) has a 10-second context timeout. But this process only populates the cache; the actual installs happen synchronously in `MaybeAutoApply` on the next command invocation.

**Blocking: Distributed registry initialization**

In `main.go` `init()` (lines 133–150), when the user has distributed registries configured, `NewDistributedRegistryProvider` is called for each one at startup. This function calls `DiscoverManifest` synchronously (`internal/distributed/provider.go` line 107), which probes a GitHub repository for a manifest file via HTTP. With no timeout on the `initCtx := context.Background()` context, a slow GitHub API response blocks binary startup entirely. For a user with two configured distributed registries, this runs two HTTP roundtrips before any command runs.

**Blocking: update-registry command**

`tsuku update-registry` (`cmd/tsuku/update_registry.go`) is fully synchronous by design — it fetches all cached recipes, refreshes the manifest, rebuilds the binary index (SQLite), and refreshes all distributed sources. This is an explicit user-invoked operation, but it also triggers `PersistentPreRun`, meaning auto-apply installs run before the registry refresh begins.

**Blocking: first-use recipe fetch**

When a recipe is not in local cache, `CachedRegistry.GetRecipe` (`internal/registry/cached_registry.go` lines 92–135) makes a synchronous HTTP fetch to `raw.githubusercontent.com`. This happens inline during `tsuku install <tool>` as part of recipe loading.

### Timing context

- `CheckAndSpawnUpdateCheck`: ~0.5 ms (file stat + flock + process spawn)
- `MaybeAutoApply` with pending updates: varies by number of tools and download speed. Each tool install requires at minimum one download. On a fast connection, a binary install could complete in 2–5 seconds; with large tools (100+ MB), this could be 30–60 seconds per tool.
- Distributed registry initialization at startup: one or two GitHub HTTP roundtrips (each typically 200–500 ms on good network, potentially several seconds on slow connections or GitHub API slowness)
- Background `check-updates` subprocess: capped at 10 seconds total; queries all installed tools' version providers

### The notification system context

`DisplayNotifications` (`internal/updates/notify.go`) is called in `PersistentPreRun` after `MaybeAutoApply`, and `DisplayAvailableSummary` is called in `PersistentPostRun`. Both are fast (file reads only) and non-blocking. They suppress output in CI, quiet mode, and non-TTY contexts. Auto-apply results are printed inline to stderr.

### The hook-env path

`cmd/tsuku/hook_env.go` is excluded from `PersistentPreRun` and calls `CheckAndSpawnUpdateCheck` directly. This is the path used by shell prompt hooks (`eval $(tsuku shellenv)` → shell.d hook → `tsuku hook-env <shell>` on each prompt). It intentionally skips `MaybeAutoApply` to avoid blocking shell prompts.

### Configuration knobs

Users and CI can control behavior via:
- `TSUKU_NO_UPDATE_CHECK=1`: disables `CheckAndSpawnUpdateCheck` entirely
- `CI=true`: suppresses auto-apply (but `CheckAndSpawnUpdateCheck` still runs)
- `TSUKU_AUTO_UPDATE=1`: explicit opt-in override for CI
- `updates.auto_apply = false` in `config.toml`: disables `MaybeAutoApply` blocking installs
- `updates.enabled = false`: disables the entire check pipeline
- `TSUKU_UPDATE_CHECK_INTERVAL`: configures check frequency (default 24h, min 1h, max 30d)

## Implications

The primary blocking problem is `MaybeAutoApply` executing full tool installs in `PersistentPreRun`. This is the most likely source of the "waits up to a minute" user experience. The design already treats update *checking* as non-blocking (spawned subprocess), but auto-apply installs remain synchronous.

Secondary blocking issue: distributed registry initialization at startup makes synchronous HTTP calls when distributed sources are configured. Even users without distributed registries could observe latency if they have `userconfig.Registries` populated.

The `hook-env` exclusion from auto-apply is a deliberate design choice that correctly avoids blocking shell prompts — this pattern could inform how auto-apply is re-routed.

The 10-second cap on `check-updates` bounds background check time, but doesn't address the foreground install time.

## Surprises

**Auto-apply happens before the user's command, not after.** The structure of `PersistentPreRun` means even a fast command like `tsuku list` can silently install multiple tools before listing anything. This is architecturally surprising — the installs are not triggered by the command the user asked for.

**The distributed registry initialization in `init()` has no timeout.** The `initCtx := context.Background()` context is passed to `NewDistributedRegistryProvider`, which calls `DiscoverManifest`. This is a potential hang source distinct from the update machinery.

**`update-registry` itself triggers PersistentPreRun.** A user running `tsuku update-registry` to refresh stale recipes first has auto-apply installs run, then the refresh. If auto-apply fails for one tool, the command still proceeds, but the ordering might surprise users.

**Auto-apply runs on every command including `tsuku install`.** A user explicitly running `tsuku install foo` will first have pending auto-apply installs run for other tools, then install foo.

## Open Questions

1. What is the actual observed latency distribution for `MaybeAutoApply`? The code makes it feel like a single-digit second operation for a single tool on fast networks, but real-world numbers across tool sizes and network conditions are unknown.

2. Should auto-apply installs ever block the foreground command at all? The check subprocess is non-blocking; there's no obvious reason apply must be synchronous. A design where apply runs as another detached subprocess and results are shown at the next command (via the existing notice system) would mirror how check works.

3. What is the right behavior when a pending update exists for the tool the user just asked to install? Skipping auto-apply for that tool and letting the explicit `install` handle it seems correct, but the current code doesn't appear to special-case this.

4. How many users actually have distributed registries configured? The startup HTTP cost is paid by all such users on every command. If adoption is low, this might not be a priority; if high, the no-timeout path is a reliability risk.

5. Does the `check-updates` subprocess's 10-second timeout match real-world version resolution time? Some version providers (e.g., PyPI, npm) may be slower than GitHub. It's unclear whether 10 seconds is sufficient or too tight for users with many installed tools.

## Summary

The primary blocker is `MaybeAutoApply`, which runs full tool installs synchronously in `PersistentPreRun` before every non-excluded command — meaning a `tsuku list` can silently install multiple tools before listing anything, with potential waits proportional to download time. A secondary blocker is distributed registry initialization in `main.go init()`, which makes synchronous HTTP calls with no timeout for each configured distributed source. The update check machinery itself (`CheckAndSpawnUpdateCheck`) is already non-blocking via detached subprocess, establishing a proven pattern; the most impactful change would be moving auto-apply installs out of `PersistentPreRun` and into the same detached-process model, surfacing results through the existing notice system on subsequent command invocations. The key open question is whether pending apply for the exact tool a user just asked to install should be handled differently than auto-apply for other tools.
