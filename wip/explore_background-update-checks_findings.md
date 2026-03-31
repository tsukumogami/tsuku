# Exploration Findings: background-update-checks

## Core Question

How should tsuku's background update check infrastructure work? This covers the layered trigger model (shell hook > shim > command), time-cached check results, detached background process lifecycle, cache file format, and configuration surface.

## Round 1

### Key Insights

- Advisory file locks (flock) are the ideal dedup mechanism: atomic, kernel-managed, <1ms non-blocking check, auto-cleanup on crash. Matches existing patterns in internal/install/filelock.go and internal/llm/lifecycle.go. (spawn-dedup lead)
- Per-tool cache files at $TSUKU_HOME/cache/updates/<toolname>.json match the version cache precedent and avoid lock contention. Each file stores: tool name, active version, Requested pin constraint, latest-within-pin, latest-overall, check timestamp, expiration. (cache-schema lead)
- The stat check belongs in hook-env.go after ComputeActivation as a purely additive <1ms call. No changes to PATH activation logic. When stale: attempt non-blocking flock, spawn tsuku check-updates if lock acquired. (hook-env lead)
- Must be a separate process (not goroutine) because hook-env exits immediately after printing shell code. exec.Command().Start() spawns detached process; no SysProcAttr needed on Unix. (hook-env lead, CLI patterns lead)
- Shim and command triggers share the same code path -- both invoke the tsuku binary. Most tools use symlinks, not shims. Shell hook is the genuinely primary trigger. (shim-trigger lead)
- CLI tools (rustup, Homebrew, mise) use fire-and-forget spawning with JSON timestamp caches. No tool implements a true background daemon. (CLI patterns lead)
- Config section follows LLMConfig pattern: pointer types for optional values, getter methods checking env vars first. Five keys: enabled, auto_apply, check_interval, notify_out_of_channel, self_update. (config lead)

### Tensions

- Single file vs per-tool: PRD says $TSUKU_HOME/cache/update-check.json (single file), but per-tool files match version cache precedent and avoid lock contention. Per-tool wins on technical merits.
- Dedup necessity: Under rapid prompt fire (every 1-3s), mtime alone spawns dozens of duplicate checks. Advisory locks are essential despite 24h check interval making duplicates "benign" in isolation.

### Gaps

None significant. All 6 leads returned substantive findings with concrete codebase evidence.

### Decisions

- Per-tool cache files over single file
- Advisory flock for spawn dedup
- Separate process over goroutine
- New hidden tsuku check-updates subcommand
- Shared CheckUpdateStaleness in internal/updates/
- Notification throttle state in $TSUKU_HOME/notices/
- Config follows LLMConfig pattern

### User Focus

Auto-mode: research is comprehensive, design questions answered. Proceeding to crystallize.

## Decision: Crystallize

## Accumulated Understanding

The background update check infrastructure has three layers:

**Trigger layer** (hook-env, tsuku run, direct commands): Each trigger calls a shared `CheckUpdateStaleness(cfg)` function. This function stats the cache directory's newest file mtime, compares against the configured interval (default 24h), and returns a bool. If stale, the trigger attempts a non-blocking advisory flock on `$TSUKU_HOME/cache/updates/update-check.lock`. If the lock succeeds (no check running), it spawns a detached `tsuku check-updates` process and releases the lock. If the lock fails (check already running), it skips silently.

**Check process** (`tsuku check-updates`, hidden subcommand): Acquires the exclusive flock (blocking), iterates all installed tools, calls ResolveWithinBoundary and ResolveLatest via ProviderFactory for each, writes per-tool JSON results to `$TSUKU_HOME/cache/updates/<toolname>.json` (atomic temp+rename), and exits. Has a 10s context timeout per R19.

**Cache layer** (per-tool JSON files): Each file stores the tool name, active version at check time, Requested constraint (for pin-change detection), latest-within-pin version, latest-overall version, source description, check timestamp, and expiration. Feature 3 (auto-apply) reads individual files. Feature 5 (notifications) scans all files. Feature 6 (outdated polish) uses both within-pin and overall values.

**Configuration**: New `[updates]` section in config.toml with five keys. Env var overrides checked first. Precedence: CLI flag > env var > .tsuku.toml > config.toml > default.

The design is well-constrained by existing patterns (version cache, filelock, LLM lifecycle). No new infrastructure abstractions needed -- just composition of existing patterns applied to update checking.
