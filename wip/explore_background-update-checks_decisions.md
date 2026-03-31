# Exploration Decisions: background-update-checks

## Round 1
- Per-tool cache files over single file: avoids lock contention on concurrent writes, matches existing version cache precedent at cache/versions/
- Advisory flock for spawn dedup: matches existing patterns in filelock.go and LLM lifecycle, kernel-managed auto-cleanup, <1ms non-blocking check
- Separate process over goroutine: hook-env exits immediately after printing shell code, cannot hold goroutines. exec.Command().Start() spawns detached process
- New hidden tsuku check-updates subcommand: dedicated entry point for background checks, not piggybacked on existing commands
- Shared CheckUpdateStaleness function in internal/updates/: single implementation called by all three trigger layers (hook-env, tsuku run, direct commands)
- Notification throttle state in $TSUKU_HOME/notices/: separated from check cache to keep cache focused on "what's available"
- Config follows LLMConfig pattern: pointer types, getter methods, env var overrides checked first
