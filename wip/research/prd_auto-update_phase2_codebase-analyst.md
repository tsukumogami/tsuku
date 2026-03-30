# Phase 2 Research: Codebase Analyst

## Lead 3: Configuration Surface Design

### Findings

#### Current Configuration Landscape

Tsuku has three distinct configuration layers, each with different scope and persistence:

**Layer 1: Environment Variables (`internal/config/config.go`)**

All follow the `TSUKU_*` prefix convention. Currently defined:

| Variable | Type | Default | Purpose |
|----------|------|---------|---------|
| `TSUKU_HOME` | path | `~/.tsuku` | Override home directory |
| `TSUKU_API_TIMEOUT` | duration | 30s | API request timeout (1s-10m range) |
| `TSUKU_VERSION_CACHE_TTL` | duration | 1h | Version list cache lifetime (5m-7d) |
| `TSUKU_RECIPE_CACHE_TTL` | duration | 24h | Recipe cache lifetime (5m-7d) |
| `TSUKU_RECIPE_CACHE_SIZE_LIMIT` | bytes | 50MB | Recipe cache max size (1MB-10GB) |
| `TSUKU_RECIPE_CACHE_MAX_STALE` | duration | 7d | Max staleness for fallback (0 disables) |
| `TSUKU_RECIPE_CACHE_STALE_FALLBACK` | bool | true | Enable stale-if-error |
| `TSUKU_DEBUG` | bool | false | Debug logging |
| `TSUKU_VERBOSE` | bool | false | Verbose logging |
| `TSUKU_QUIET` | bool | false | Suppress info messages |
| `TSUKU_CEILING_PATHS` | path list | (none) | Additional traversal ceilings for .tsuku.toml |
| `TSUKU_NO_TELEMETRY` | bool | false | Disable telemetry |
| `TSUKU_TELEMETRY` | bool | true | Alias (0/false disables) |
| `TSUKU_LLM_IDLE_TIMEOUT` | duration | 5m | LLM addon idle timeout |

The pattern is consistent: each env var has a `Get*()` function with range validation, human-friendly parsing, and stderr warnings for invalid values. Duration values support Go `time.ParseDuration` format; byte sizes support human-readable suffixes (KB, MB, GB).

**Layer 2: User Config (`internal/userconfig/userconfig.go` -- `$TSUKU_HOME/config.toml`)**

TOML file with typed `Get`/`Set` API and `AvailableKeys()` for discoverability. Current keys:

| Key | Type | Default | Section |
|-----|------|---------|---------|
| `telemetry` | bool | true | root |
| `auto_install_mode` | enum | "confirm" | root |
| `llm.enabled` | bool | true | `[llm]` |
| `llm.local_enabled` | bool | true | `[llm]` |
| `llm.local_preemptive` | bool | true | `[llm]` |
| `llm.idle_timeout` | duration | 5m | `[llm]` |
| `llm.providers` | string list | (auto) | `[llm]` |
| `llm.daily_budget` | float | 5.0 | `[llm]` |
| `llm.hourly_rate_limit` | int | 10 | `[llm]` |
| `llm.backend` | enum | (auto) | `[llm]` |
| `secrets.*` | string | (none) | `[secrets]` |
| `registries.*` | table | (none) | `[registries]` |

Config uses `*bool` pointer types for opt-in/opt-out semantics (nil means "use default"), atomic saves via temp-file-then-rename, and 0600 permissions with group/other access warnings. Adding a new section follows a clear pattern: define struct fields with TOML tags, add Get/Set cases, add to AvailableKeys.

**Layer 3: Project Config (`internal/project/config.go` -- `.tsuku.toml`)**

Per-directory tool declarations discovered by walking parent directories up to `$HOME`. Current structure:

```toml
[tools]
node = "20.16.0"          # string shorthand
python = { version = "3.12" }  # inline table form
```

`ToolRequirement` has a single `Version` field. No fields for pin level, update policy, or channel selection. The `UnmarshalTOML` method handles both string and table forms, so extending with new fields (e.g., `pin`, `auto_update`) is straightforward -- the table form already exists as an extension point.

Max 256 tools per config file (DoS protection). Traversal stops at `$HOME` unconditionally plus any `TSUKU_CEILING_PATHS`.

#### Precedence Model

There is no formal precedence system today. Each layer operates independently:

- Env vars override specific config.toml values only where explicitly coded (e.g., `LLMIdleTimeout()` checks env var first, then config file).
- `.tsuku.toml` is purely additive -- it declares tool requirements, not configuration overrides.
- CLI flags (`--quiet`, `--json`, `--dry-run`) are command-scoped and don't interact with config.toml.

There is no generic "precedence chain" infrastructure. Each config value implements its own override logic. For the LLM idle timeout: env var > config.toml > default. For version cache TTL: env var > default (no config.toml option exists). For telemetry: config.toml > default (no env var override for the boolean, though `TSUKU_NO_TELEMETRY` disables independently).

#### CLI Flags on Relevant Commands

| Command | Relevant Flags |
|---------|---------------|
| `install` | `--dry-run`, `--force`, `--fresh`, `--from`, `--yes`, `--plan`, `--recipe`, `--sandbox`, `--skip-security` |
| `update` | `--dry-run` |
| `outdated` | `--json` |
| `activate` | (version arg only) |

No existing flags relate to version pinning, update channels, or auto-update behavior.

### Implications for Requirements

**1. The `[updates]` section in config.toml is the natural home for global update preferences.**

The pattern is established: `[llm]` groups LLM settings, `[registries]` groups registry settings. An `[updates]` section would group:
- `enabled` (bool, default true) -- master switch for auto-update
- `check_interval` (duration string, default "24h")
- `notification` (enum: "off", "pinned", "all")
- `self_update` (bool or enum)
- `auto_apply` (bool, default false) -- whether to auto-install found updates

**2. Per-tool update configuration belongs in `.tsuku.toml`, not config.toml.**

`.tsuku.toml` already declares per-tool versions. Extending `ToolRequirement` to include `pin` and `auto_update` fields maps naturally:
```toml
[tools]
node = { version = "20.16.0", pin = "20" }
kubectl = { version = "1.29.3", auto_update = false }
terraform = "latest"
```

The table form already exists. String shorthand (`node = "20.16.0"`) would remain as exact-pin-with-no-auto-update for backward compatibility.

**3. Env vars for CI/scripting should follow existing patterns.**

Candidates:
- `TSUKU_NO_UPDATE_CHECK` (bool) -- suppress all update checking
- `TSUKU_UPDATE_CHECK_INTERVAL` (duration) -- override check interval
- `TSUKU_AUTO_UPDATE` (bool) -- enable/disable auto-apply

These follow the established pattern: `TSUKU_` prefix, human-readable values, range validation, stderr warnings.

**4. Precedence must be explicitly defined and documented.**

The PRD should specify: CLI flag > env var > `.tsuku.toml` (project) > `config.toml` (global) > default. This matches the conventional layered config pattern and is consistent with how `TSUKU_LLM_IDLE_TIMEOUT` already overrides the config file value. The codebase doesn't have a generic precedence resolver, so each setting will need to implement its own chain (matching the current approach).

**5. config.toml has no existing TTL settings -- adding update interval creates a precedent.**

All current cache TTLs (version, recipe) are env-var-only. Adding `updates.check_interval` to config.toml would be the first TTL setting there. The PRD should decide whether to also promote existing TTLs into config.toml for consistency, or accept the asymmetry.

**6. The `Requested` field in state already serves as implicit pin level.**

`VersionState.Requested` stores what the user typed at install time ("17", "1.29", "@lts", ""). The PRD should specify whether pin level is derived from `Requested` (zero new config) or stored as an explicit field (more flexible, but requires migration).

### Open Questions

1. **Should `auto_update` in `.tsuku.toml` override the global `config.toml` setting?** If config.toml says `auto_apply = true` but a project's `.tsuku.toml` says `auto_update = false` for a specific tool, which wins? The project-level config is more specific, but it's set by a different person (project maintainer vs. user).

2. **How does config.toml interact with `.tsuku.toml` for the same tool?** If config.toml has a `[tool-overrides.kubectl]` section and `.tsuku.toml` also declares kubectl, the precedence needs to be clear. Today these layers don't overlap because config.toml doesn't have per-tool settings.

3. **Should there be a `[tool-overrides]` section in config.toml at all?** Per-tool global preferences (like "always pin kubectl to minor") could live in config.toml, but this creates two places to configure per-tool behavior (config.toml and .tsuku.toml). The simpler model is: config.toml for global defaults, .tsuku.toml for per-tool overrides, no per-tool section in config.toml.

4. **Is the `Get`/`Set` CLI interface (`tsuku config set updates.check_interval 12h`) sufficient, or do update settings need a dedicated subcommand?** The existing pattern works for simple values but gets awkward for nested per-tool configs.

5. **Should `tsuku update` gain `--pin` and `--channel` flags?** E.g., `tsuku update kubectl --pin minor` to change pin level, `tsuku update --all` to update everything within constraints.

---

## Lead 4: Edge Cases from Implementation

### Findings

#### Concurrent Access Model

**In-process locking:** `StateManager` uses `sync.RWMutex` for goroutine safety within a single tsuku process. Read operations acquire `RLock`, write operations acquire full `Lock`.

**Cross-process locking:** `FileLock` (`internal/install/filelock.go` + `filelock_unix.go`) wraps `flock(2)` syscall for advisory file locking. Shared locks for reads, exclusive locks for writes. The lock file is `$TSUKU_HOME/state.json.lock`. Locking is blocking (no timeout, no `LOCK_NB`).

**The `UpdateTool` pattern** (`state_tool.go:7-35`) is the gold standard for atomic state mutation: acquire mutex, acquire file lock, load state, apply mutation function, save state, release locks. This read-modify-write cycle is fully serialized across processes.

**Gap: Tool directory operations are not covered by locks.** The file lock only protects `state.json`. Two concurrent processes installing the same tool could race on:
1. Staging directory creation (both create `.name-version.staging`)
2. Staging-to-final rename (both try `os.Rename(staging, toolDir)`)
3. Symlink creation (both try to update the same symlink in `current/`)

In practice, the staging directory name includes the version, so different-version installs don't collide. Same-version reinstalls could race, but the atomic rename means one wins and the other fails with a rename error. The state update is serialized, so both processes don't write conflicting state.

For auto-update, this gap matters more: a background auto-update and a manual `tsuku update` of the same tool could race. The PRD should specify whether per-tool locking is needed or if last-writer-wins is acceptable.

#### Atomic Write Patterns

Three separate atomic write implementations exist:

1. **State file** (`state.go:233-246`): Write to `.tmp`, rename. Under exclusive file lock.
2. **User config** (`userconfig.go:192-231`): Write to temp file in same dir, `chmod 0600`, rename. No file lock.
3. **Version cache** (`cache.go:126-156`): Write to `.tmp`, rename. No file lock (best-effort).

All follow the same temp-then-rename pattern. None use `fsync` before rename, which means on power failure the temp file content might not be durable. For state.json this is a minor risk; for update check cache it's irrelevant (cache miss is the fallback).

#### Signal Handling

`cmd/tsuku/main.go` sets up signal handling:
- `SIGINT` and `SIGTERM` are caught via `signal.Notify`.
- First signal: cancels `globalCtx` (a `context.WithCancel` context), prints "canceling operation..." to stderr.
- Second signal: force exits.
- Commands that use `globalCtx` get cooperative cancellation.

**Gap: No cleanup on cancellation.** When the context is canceled, the executor stops running actions, but there's no cleanup of:
- Staging directories (the `.staging` dirs in `$TSUKU_HOME/tools/`)
- Downloaded temp files in the work directory
- Partially written state (though atomic writes prevent corruption)

The stale staging directory is cleaned up on the *next* install of the same tool (`manager.go:78`), which is a reasonable deferred cleanup. But if the user never reinstalls that tool, the staging dir persists.

#### State Corruption Recovery

**Corruption detection:** `json.Unmarshal` failure on `state.json` returns an error. The caller (any tsuku command) shows the error and exits. There is no automatic recovery, no backup, and no `tsuku doctor` repair for corrupt state.

**Migration system:** `migrateToMultiVersion()` and `migrateSourceTracking()` run on every state load, handling format evolution. Migrations are idempotent and non-destructive. This pattern is directly reusable for adding new fields needed by auto-update (e.g., `pin_level`, `last_update_check`).

**State-directory consistency:** If `InstallWithOptions` succeeds (tool dir created) but `UpdateTool` fails (state write fails), a tool directory exists without a state entry. The reverse (state entry without directory) is detected by `Activate()` which checks directory existence. `tsuku doctor` validates binary checksums but does not check for orphaned directories or missing state entries.

#### Version Cache Corruption

`CachedVersionLister.readCache()` (`cache.go:111-123`) treats JSON parse failure as a cache miss -- silently re-fetches. This is correct behavior. No file locking on cache files means concurrent writers could produce corrupt JSON, but the read-side handles it gracefully.

#### Installation Atomicity Analysis

The install flow has these critical sections:

1. **Clean staging** (`manager.go:78`): `os.RemoveAll` of stale staging. If interrupted, staging stays (cleaned next time).
2. **Copy to staging** (`manager.go:91-95`): If interrupted, partial staging stays (cleaned on next install or by step 1).
3. **Remove existing tool dir** (`manager.go:104-107`): `os.RemoveAll` of current installation. **This is a point of no return** -- if the process dies between removing old dir and renaming staging, the tool is gone with no state update. The old version directory is deleted before the new one is in place.
4. **Rename staging to final** (`manager.go:111`): Atomic on same filesystem.
5. **Create symlinks** (`manager.go:119-151`): If this fails, tool dir is rolled back (`os.RemoveAll(toolDir)`).
6. **Update state** (`manager.go:169-196`): Under file lock, atomic write.

**Critical gap for auto-update:** Step 3 removes the existing version directory. For same-version reinstalls (e.g., recipe fix), the old installation is gone before the new one is committed. For different-version updates, this isn't an issue because the new version goes to a different directory. But the `update` command today calls `runInstallWithTelemetry` which goes through the same install path, and if updating from v1.0 to v1.1, the v1.0 directory is **not** removed (different path). However, the symlink switch happens in step 5, and if that fails, the new version directory exists but isn't activated -- the old symlink target is gone (removed in step 3 of the old version's install path? No -- step 3 removes the *target* version dir, not the old version dir).

Actually, re-reading more carefully: step 3 removes `toolDir` which is `<name>-<version>` for the *new* version being installed. It removes a pre-existing installation of the same version, not the old version. So for v1.0 -> v1.1 updates, v1.0's directory is preserved. This is the key property that enables rollback.

#### Flock Blocking Behavior

`flock(2)` without `LOCK_NB` blocks indefinitely. If a tsuku process holds the exclusive lock on `state.json.lock` and hangs (e.g., stuck network request), all other tsuku processes block waiting for the lock. There's no timeout.

For auto-update running in the background (as a goroutine within the main process), this isn't an issue -- the goroutine shares the process. But if auto-update ever runs as a separate process, lock contention becomes a concern.

### Implications for Requirements

**1. Per-tool locking is needed for safe concurrent auto-update.**

The current file lock only covers `state.json`. Auto-update of tool X should not block auto-update of tool Y, but two updates of the same tool must be serialized. A per-tool lock file (`$TSUKU_HOME/tools/.name.lock`) would solve this. The PRD should specify whether this is required for v1 or can be deferred.

**2. The multi-version directory model makes rollback natural, but only for cross-version updates.**

Updating from v1.0 to v1.1 preserves the v1.0 directory. Rollback means calling `Activate("tool", "1.0")`. But same-version reinstalls (recipe fixes) overwrite the directory. The PRD should specify that auto-update only targets version changes, not same-version reinstalls.

**3. Staging directory cleanup should be formalized.**

Stale staging dirs from interrupted installs are cleaned up opportunistically. With auto-update adding more install operations, `tsuku doctor` should detect and clean orphaned staging dirs. The PRD should include this as a requirement.

**4. State corruption recovery should be specified.**

Today a corrupt `state.json` is a hard failure. For auto-update, the system should be more resilient: if state can't be loaded, auto-update should skip (not crash). The PRD should specify graceful degradation.

**5. The blocking flock is fine for single-process auto-update but blocks multi-process scenarios.**

If auto-update runs as a goroutine within the main tsuku process (the recommended approach from the update-check-caching research), the in-process mutex handles serialization without file lock contention. The PRD should specify that auto-update runs in-process, not as a separate daemon.

**6. Signal handling needs cleanup hooks for auto-update.**

If auto-update is mid-install when the user sends SIGINT, the staging directory should be cleaned up. The globalCtx cancellation propagates to the executor, but explicit cleanup of staging dirs should be added. The PRD should require cleanup-on-interrupt for auto-update operations.

**7. No `fsync` before rename means state can be lost on power failure.**

The atomic write pattern (temp + rename) protects against process crashes but not power failures. For auto-update state (last check time, update results), this is acceptable -- a cache miss just triggers a re-check. For tool state (`state.json`), this is a pre-existing risk that auto-update doesn't worsen.

### Open Questions

1. **Should auto-update acquire the state lock for the entire update cycle, or only for the state write?** Holding the lock for the entire download+install+state-write would prevent any other tsuku operation during auto-update. Holding it only for the state write allows concurrent reads but risks the state being stale when the write happens. The current `UpdateTool` pattern locks for the read-modify-write cycle, which is correct but assumes the operation is fast. Auto-update downloads are not fast.

2. **What happens if auto-update fails between steps 4 and 6 (rename succeeded but state write failed)?** The new version directory exists but state doesn't reflect it. On next run, the old version is still in state as active. The symlinks may point to the new version (if step 5 succeeded) or the old version (if step 5 failed). This inconsistency should be specified and handled.

3. **Should there be a "last known good" snapshot of state.json?** A periodic backup (e.g., `state.json.bak` updated daily) would allow manual recovery from corruption. The cost is minimal (one extra atomic write), and the benefit is significant for users who can't rebuild state from scratch.

4. **How should the notices/deferred-error subsystem handle concurrent writes?** If auto-update writes a failure notice while another process is also writing a notice, the notices dir approach (one file per notice) avoids collision. The state.json approach would require lock coordination.

5. **Should `tsuku doctor` be extended to validate update-related state (cache freshness, orphaned staging dirs, state-directory consistency)?** This seems like a natural fit but adds scope to the auto-update feature.

---

## Summary

The codebase imposes two major constraints on auto-update requirements: (1) configuration must flow through three existing layers -- env vars for CI/scripting (`TSUKU_NO_UPDATE_CHECK`), config.toml for global user preferences (new `[updates]` section), and `.tsuku.toml` for per-tool project-level pins (extending `ToolRequirement` with `pin` and `auto_update` fields) -- with explicit precedence (CLI flag > env var > project > global > default); and (2) the existing concurrency model protects `state.json` via flock but leaves tool directory operations unguarded, meaning auto-update needs either per-tool locking or a single-process-goroutine architecture to avoid races during concurrent updates. The multi-version directory model already supports rollback for cross-version updates (old version dir is preserved), and the atomic staging-then-rename pattern prevents partial installations, but same-version reinstalls and the state-write-after-install gap are edge cases the PRD must address.
