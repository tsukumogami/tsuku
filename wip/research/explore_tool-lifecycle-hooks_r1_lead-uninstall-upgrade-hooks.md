# Lead: How should pre-uninstall and pre-upgrade hooks work?

## Findings

### Current Remove Flow

Tsuku has no pre-uninstall or pre-upgrade hooks. The `RemoveVersion`/`RemoveAllVersions`
methods simply delete directories and symlinks without executing any cleanup logic.
Shell hooks are registered via `tsuku hook install` (which modifies ~/.bashrc,
~/.zshrc, and ~/.config/fish/conf.d), but these are global tsuku hooks, not per-tool
lifecycle hooks.

Remove flow operations:
1. Delete tool directory recursively
2. Remove symlinks from current/
3. Remove state entry
4. No cleanup hooks, no recipe consultation

### Current Update Flow

The `update` command is implemented as a complete reinstall by calling
`runInstallWithTelemetry`. There is no explicit pre-upgrade phase. The flow is:
resolve version constraint -> install new version -> activate new version.

No hooks or customization points exist for update-time logic (migrate config files,
warn about breaking changes, run upgrade scripts).

### Recipe System Gaps

The recipe system currently has no hook declarations. types.go contains only
build/install actions. There is no mechanism to declare pre-remove, post-remove,
pre-update, or post-update behavior.

### Practical Scenarios

- Tool registered shell completions at install: remove doesn't know to clean them up
- Tool wrote an init script to shell.d/: remove needs to delete it
- Tool upgraded from v1 to v2 with config format change: no pre-upgrade migration
- If pre-uninstall hook fails: no policy on abort vs force

### Ordering Constraints

- Pre-uninstall must run BEFORE files are deleted
- Pre-upgrade must run BEFORE old version is replaced
- Post-upgrade might need access to both old and new state
- Uninstall infrastructure exists for shell hooks but is environment-scope, not tool-scope

## Implications

The remove and update flows would need significant refactoring to consult recipes
and execute lifecycle steps. Remove would need to load the recipe before deleting,
and update would need pre/post phases around the reinstall.

For cleanup to work reliably, tsuku needs to either:
1. Store cleanup instructions in state at install time (so remove doesn't need the recipe)
2. Or load the recipe at remove time (requires registry access)

Option 1 is more reliable -- if the recipe changes between install and remove,
the stored cleanup instructions match what was actually installed.

## Surprises

The remove flow doesn't even load the recipe -- it just deletes files. This means
any cleanup mechanism must either be stored in state or derived from the tool's
installed files.

## Open Questions

1. Should cleanup instructions be stored in state.db at install time?
2. What happens if pre-uninstall hook fails -- abort or force?
3. Should old version files be accessible during post-upgrade hooks?
4. How does multi-version support interact with lifecycle hooks?

## Summary

Tsuku has zero lifecycle awareness in remove and update flows. Remove just deletes
directories and symlinks without consulting recipes. Update is a full reinstall with
no pre/post phases. Adding lifecycle hooks requires either storing cleanup instructions
in state at install time or loading recipes at remove time.
