# Pragmatic Review: Issue 9 -- Source display in info, list, and recipes

## Scope

Files changed:
- `cmd/tsuku/list.go` -- source suffix in human-readable output, `source` field in JSON
- `cmd/tsuku/info.go` -- `Source:` line and JSON field, dual-path resolution (installed vs uninstalled)
- `cmd/tsuku/recipes.go` -- distributed source display, `--local` flag preserved, JSON source field
- `internal/install/list.go` -- `Source` field added to `InstalledTool` struct, populated from `ToolState.Source`
- `internal/install/list_test.go` -- tests for source field, migrated source, hidden tool filtering

## Findings

### No blocking issues found.

The implementation correctly matches all acceptance criteria:

1. **`list` command**: Human-readable shows `[owner/repo]` suffix only for distributed tools (contains `/`). JSON includes `"source"` field via `omitempty`, so central/empty sources still appear when set. Source is populated from `ToolState.Source` via `InstalledTool.Source`.

2. **`info` command**: For installed tools, reads `ToolState.Source`. For uninstalled tools, calls `GetWithSource` to determine provider. Maps `SourceLocal` -> `"local"`, `SourceRegistry`/`SourceEmbedded` -> `"central"`, and passes through distributed sources as-is. Human-readable shows `Source:` line when non-empty. JSON includes `"source,omitempty"`.

3. **`recipes` command**: Uses `ListAllWithSource()` which iterates all providers including distributed ones. Each entry carries its source. `--local` flag still uses `ListLocal()`. Source counting correctly handles the four source types.

### Advisory: info.go double-loads recipe (line 100, then line 116)

`info.go:100` calls `loader.Get()` and `info.go:116` calls `loader.GetWithSource()` for the same tool name. `GetWithSource` internally calls `Get` again. The in-memory cache makes this a map lookup on the second call, so there's no correctness or performance problem. Could simplify by using `GetWithSource` for both, but the current approach is harmless.

### Advisory: distributed recipes have empty Description in `recipes` output

`DistributedProvider.List()` (`internal/distributed/provider.go:52-66`) doesn't populate `Description` in `RecipeInfo`. This means distributed recipes in `tsuku recipes` output show blank descriptions. This is a pre-existing issue from Issue 6, not introduced by this change.

## Overall Assessment

Clean, minimal implementation. Each command integrates source display with the right approach: `list` reads from state (already loaded), `info` resolves from both state and provider, `recipes` delegates to the existing `ListAllWithSource` method. Test coverage is solid -- includes source field propagation, lazy migration to "central", and hidden tool filtering. No over-engineering detected.
