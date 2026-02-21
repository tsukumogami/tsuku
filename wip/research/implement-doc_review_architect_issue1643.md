# Architecture Review: Issue #1643

**Issue**: #1643 (feat(llm): implement tsuku llm download command)
**Focus**: architect (design patterns, separation of concerns)
**Files changed**: `cmd/tsuku/llm.go`, `cmd/tsuku/llm_test.go`, `cmd/tsuku/main.go`

## Review Scope

Changes introduce a `tsuku llm` command group with a `download` subcommand for pre-downloading the addon binary and model. Registration in `main.go` at line 99: `rootCmd.AddCommand(llmCmd)`.

## Findings

### ADVISORY: AddonManager instantiated without Installer, bypassing the recipe-based installation path

`cmd/tsuku/llm.go:74` creates an AddonManager with no Installer:

```go
addonManager := addon.NewAddonManager("", nil, "")
```

In `internal/llm/addon/manager.go:190-194`, `installViaRecipe` fails when `m.installer == nil`:

```go
func (m *AddonManager) installViaRecipe(ctx context.Context) error {
    if m.installer == nil {
        return fmt.Errorf("no installer configured")
    }
    return m.installer.InstallRecipe(ctx, "tsuku-llm", "")
}
```

If the addon is not already installed, `EnsureAddon` will hit the `installViaRecipe` path and fail with "no installer configured" wrapped in "installing tsuku-llm:". The error message then suggests `tsuku install tsuku-llm` (line 86), which is a valid fallback instruction.

This is the same pattern used in `NewLocalProvider()` at `internal/llm/local.go:54`:
```go
addonManager := addon.NewAddonManager("", nil, "")
```

So the `llm download` command follows the established pattern for code paths that don't own the installation pipeline. The `create` command wires the Installer via `NewLocalProviderWithInstaller`. The download command delegates installation to the user. This is consistent with the existing approach but worth noting: if a user's first interaction is `tsuku llm download` (not `tsuku create`), they get a slightly confusing error. The design doc says the CLI handles "Downloading the addon binary" directly, but the implementation requires the addon to already be installed or falls through to a manual install hint. This is a contained imperfection -- it doesn't affect other code paths.

**Severity**: Advisory. The pattern matches `NewLocalProvider()`. The error message provides a usable workaround.

### ADVISORY: ServerLifecycle created independently from the one inside LocalProvider

`cmd/tsuku/llm.go:94-96` creates a standalone `ServerLifecycle`:

```go
socketPath := llm.SocketPath()
lifecycle := llm.NewServerLifecycle(socketPath, addonPath)
lifecycle.SetIdleTimeout(llm.GetIdleTimeout())
```

Then at line 105, a separate `LocalProvider` is created:

```go
provider := llm.NewLocalProvider()
```

`NewLocalProvider()` internally creates its own `ServerLifecycle` (at `internal/llm/local.go:55`):
```go
lifecycle := NewServerLifecycle(socketPath, "")
```

The download command uses the standalone lifecycle to start the server (line 98), then uses the provider's internal lifecycle for `GetStatus` and `TriggerModelDownload`. This works because the provider calls `ensureConnection` (which connects to the already-running socket) rather than `lifecycle.EnsureRunning`. But there are now two `ServerLifecycle` instances managing the same socket -- the standalone one started the process, while the provider's internal one doesn't know about it.

This isn't a bug today because the provider only uses `ensureConnection` (raw gRPC connect), not `lifecycle.EnsureRunning`. But it's a fragile arrangement. If `LocalProvider.Complete()` is ever called from this code path, it would call `lifecycle.EnsureRunning` on the inner lifecycle which has `addonPath=""`, potentially failing or starting a duplicate server attempt. The standalone lifecycle holds the process reference; the provider's doesn't.

A cleaner approach: pass the already-resolved addonPath and lifecycle to the provider, or use the provider's own lifecycle for starting the server (feeding it the addonPath). This matches how `create.go` uses `NewLocalProviderWithInstaller` which sets up a single lifecycle internally.

**Severity**: Advisory. Both lifecycles point at the same socket path, and the current code path doesn't hit the conflict case. The duplication is contained to this command.

## Positive Observations

**CLI surface pattern is followed correctly.** The `llm` command group with `download` subcommand mirrors the established `config` command group pattern (`config.go`): parent command has no `Run`/`RunE`, subcommands have `RunE`, and `init()` wires subcommands. Registration in `main.go` follows the same `rootCmd.AddCommand` style.

**Prompter reuse is clean.** The `--yes` flag maps to `AutoApprovePrompter`, else `InteractivePrompter` -- the same pattern used in `create.go:575-577` for `createAutoApprove`. The prompter is set on both AddonManager and LocalProvider.

**No new package dependencies.** The command only imports `internal/llm` and `internal/llm/addon`, which are the expected packages for LLM addon management. No upward dependency violations.

**Exit code handling follows conventions.** Uses `exitWithCode(ExitGeneral)` consistently, matching other commands. The `RunE` pattern (returning nil after `exitWithCode`) is consistent with the codebase convention seen in other RunE-style commands.

**Design deviation handled cleanly.** The `--model` flag was removed rather than left as a non-functional display flag. The test explicitly documents why (`llm_test.go:49-56`). Model override is redirected to `config.toml local_model`, which the addon reads via its own configuration path. This avoids a parallel configuration mechanism.

**`TriggerModelDownload` uses the gRPC API correctly.** Rather than inventing a new RPC endpoint, it sends a minimal Complete request to force model download. This is the same path production inference takes, ensuring the download codepath is exercised exactly as it would be in real use.

## Overall Assessment

The implementation fits the existing architecture. The new CLI command follows established patterns for command groups, prompter wiring, and exit codes. The two advisory findings (nil installer and dual lifecycle instances) are contained imperfections that don't affect other code paths. No blocking architectural issues.
