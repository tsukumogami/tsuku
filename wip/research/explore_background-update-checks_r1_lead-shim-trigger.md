# Lead: What's the shim trigger path?

## Findings

### How tsuku's Shim System Works

Shims are minimal shell scripts at `$TSUKU_HOME/bin/`:
```sh
#!/bin/sh
exec tsuku run "$(basename "$0")" -- "$@"
```

Managed by `internal/shim/manager.go`. Shim metadata tracked in `$TSUKU_HOME/bin/.tsuku-shims.json`.

### Shims Are Secondary, Not Primary

Most tsuku tools use **symlinks** to `$TSUKU_HOME/tools/current/<toolname>`, not shims. Shims are opt-in via `tsuku shim install` and primarily for `.tsuku.toml`-declared tools where version resolution happens at runtime.

This means the shim trigger path is genuinely secondary -- most tool invocations bypass it entirely.

### The tsuku run Path

When a shim is invoked:
1. Shell calls shim -> execs `tsuku run <toolname> -- <args>`
2. `cmd/tsuku/cmd_run.go` handles the command
3. `internal/autoinstall/autoinstall.go` Runner.Run does:
   - Security gates (root guard, verify mode, conflict check)
   - Version resolution (project-pinned versions take precedence)
   - Installation if needed
   - `syscall.Exec` to replace process with tool binary

### Where the Staleness Check Fits

The staleness check should go inside `tsuku run` before the `syscall.Exec`:
1. Stat the cache file for this tool
2. If stale: attempt non-blocking flock on lock file
3. If lock acquired: spawn detached `tsuku check-updates` process
4. Proceed with exec regardless

This adds ~1ms to the shim path (one stat call + possible flock attempt).

### Prior Art

- **asdf**: No background checks. Shims have 150ms overhead due to per-plugin directory traversal. No update infrastructure at all.
- **mise**: Chose not to use shims in the execution path. Uses PATH manipulation via shell hooks instead. Shims exist only as metadata holders.
- **rustup**: Spawns detached background process during command invocation. Next invocation checks if download finished and displays notification.

### Performance Budget

The shim path timeline:
- Shell calls shim: ~0ms
- Shim execs tsuku run: ~1-2ms fork overhead
- Config load: ~1-3ms
- Staleness stat: ~0.5ms
- Version resolution: ~1-2ms
- Exec replacement: <1ms

Total: ~5-8ms before tool execution. The staleness check adds <1ms. Acceptable per PRD R19.

## Implications

The shim trigger is cheap because `tsuku run` already starts a Go process. Adding a stat call is negligible. The bigger question is whether shims are common enough to matter as a trigger layer. Since most tools use symlinks, the shell hook is the primary trigger and shims are a bonus for the subset of tools that use them.

The update check should happen at the `tsuku run` level (which all tsuku commands share), not at the shim level specifically. This means "tsuku commands" and "shim invocations" are really the same trigger path -- both route through the Go binary.

## Surprises

1. Most tsuku tools use symlinks, not shims. The shim trigger covers a minority of tool invocations.
2. mise explicitly chose not to use shims at all in the execution path -- PATH manipulation via shell hooks is faster.
3. The shim and direct-tsuku-command triggers are essentially the same code path (both invoke the tsuku binary).

## Open Questions

1. Should the staleness check in `tsuku run` be global (check all tools) or per-tool (check only the invoked tool)?
2. Should symlink-based tools get any update check trigger? (Only via shell hook or direct tsuku commands.)
3. Should the shim trigger spawn a full check for all tools, or just flag staleness for the shell hook to pick up?

## Summary

Shims exec `tsuku run`, so the shim trigger and the tsuku-command trigger share the same code path. Most tools use symlinks, not shims, making the shell hook the primary trigger for update checks. Adding a staleness stat to `tsuku run` adds <1ms and covers both shim and direct command invocations. The real design question is whether the check should be per-tool or global.
