# Phase 3 Research: LLM Pinning Code Path

## Questions Investigated
- Full code path from CLI to llm binary discovery
- Where version checking fits in the addon manager
- How auto-reinstall works
- Handling of already-running wrong-version daemon

## Findings

### LLM Code Path
1. CLI command (e.g., `tsuku llm complete`) creates `AddonManager`
2. `AddonManager.EnsureAddon(ctx)` called to get binary path
3. `lifecycle.EnsureRunning()` starts daemon if not running
4. `local.LocalProvider` connects via gRPC over Unix socket
5. `LocalProvider.Complete(ctx, prompt)` sends inference request

### EnsureAddon Flow (manager.go:95-149)
1. Check `TSUKU_LLM_BINARY` env var (skip all checks if set)
2. Return cached path if available
3. Call `findInstalledBinary()` -- scans `$TSUKU_HOME/tools/tsuku-llm-*` directories
4. If not found: prompt user, call `installViaRecipe(ctx)`, re-scan
5. Clean up legacy paths
6. Cache and return binary path

### findInstalledBinary (manager.go:154-186)
Scans `$TSUKU_HOME/tools/` for directories matching `tsuku-llm-*`. Accepts ANY version -- first match wins. Checks both `bin/tsuku-llm` and root `tsuku-llm` layouts.

**Version check insertion point**: After `findInstalledBinary()` returns a path, extract the version from the directory name (`tsuku-llm-0.5.0` → `0.5.0`) and compare against `pinnedLlmVersion`. If mismatch in release mode, call `installViaRecipe` with the pinned version.

### installViaRecipe (manager.go:189-195)
Delegates to `m.installer.InstallRecipe(ctx, "tsuku-llm", "")`. The `Installer` interface calls the recipe system. Currently installs latest version. For pinned version, the installer would need to accept a version parameter OR the recipe's version resolution would use the pinned version via `@version` syntax.

### Comparison with dltest Pattern
dltest uses subprocess `tsuku install tsuku-dltest@{version}` for auto-reinstall. The llm addon manager uses `Installer.InstallRecipe()` which goes through the recipe system directly. The approaches are equivalent -- both use the recipe system, just via different entry points.

**Key difference**: dltest's `installDltest(version)` accepts a version parameter and passes it as `tsuku-dltest@{version}`. The llm `installViaRecipe` doesn't accept a version. It would need to be extended to `InstallRecipe(ctx, "tsuku-llm@0.5.0", "")` or similar.

### Wrong-Version Daemon Handling
If a wrong-version daemon is already running (started by a previous tsuku version):
1. `EnsureRunning()` checks the lock file -- daemon is running, returns immediately
2. The wrong-version daemon serves requests
3. Version mismatch is invisible without gRPC handshake

**Solution**: Version check should happen in `EnsureAddon` BEFORE `EnsureRunning`. If the installed binary version doesn't match the pinned version:
1. Shut down the running daemon (if any) via `shutdownViaGRPC` or SIGTERM
2. Install the correct version via recipe system
3. Let `EnsureRunning` start the new version

This is more complex than dltest (which is stateless subprocess) because llm has daemon lifecycle to manage.

### Version Extraction from Path
The installed directory name is `tsuku-llm-{version}`. Extracting version: `strings.TrimPrefix(dirName, "tsuku-llm-")`. This works for both `tsuku-llm-0.5.0` and `tsuku-llm-0.5.0-rc.1`.

## Implications for Design
The version check fits naturally after `findInstalledBinary()` in `EnsureAddon`. The main complexity is daemon lifecycle -- if a wrong-version daemon is running, it must be stopped before reinstalling. The `Installer` interface needs a version parameter for pinned installation.

## Surprises
The `Installer` interface doesn't accept a version parameter -- `InstallRecipe(ctx, recipeName, gpuOverride)`. Adding version support requires either extending the interface or passing version via the recipe name (e.g., `tsuku-llm@0.5.0`). This is a minor interface change but affects all `Installer` implementations.

## Summary
Version pinning fits into `EnsureAddon` after `findInstalledBinary()`. Extract version from directory name, compare against `pinnedLlmVersion`. On mismatch: shut down running daemon, install correct version, restart. The `Installer` interface needs a version parameter. Main complexity vs dltest is daemon lifecycle management.
