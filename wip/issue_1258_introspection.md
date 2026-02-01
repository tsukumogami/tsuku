# Issue 1258 Introspection

## Context Reviewed
- Design doc: docs/designs/DESIGN-batch-recipe-generation.md
- Sibling issues reviewed: #1256 (closed), #1271 (merged)
- Prior patterns identified: workflow uses `--recipe` flag for local paths (from #1271)

## Gap Analysis

### Minor Gaps

- **Workflow uses `--recipe <path>` not tool names**: The issue's AC says to use `tsuku info --json --metadata-only <recipe>` but the workflow now passes recipe file paths, not tool names. The `tsuku info` command takes a tool name (e.g., `ripgrep`), not a path. The implementation will need to use the `tool` field from the matrix JSON (already extracted as `basename "$path" .toml`), which is fine since `tsuku info` resolves by name.

- **macOS job passes only tool names, not paths**: The macOS job receives `macos_recipes` as a simple string array of tool names (not objects with path/tool fields). The issue's platform filtering logic will need to handle both the Linux matrix (objects) and macOS list (strings) differently, or unify the format.

- **#1271 changed `--recipe` usage**: The workflow was modified after issue creation to use `--recipe` flag for local recipe files. This doesn't conflict with the issue but the implementation should be aware of this pattern.

- **`tsuku info` needs `--recipe` for local paths too**: In CI, recipes may only exist locally (not in the registry). The matrix setup step would need to run `tsuku info --json --metadata-only --recipe <path>` rather than `tsuku info --json --metadata-only <tool>` to handle recipes not yet in the registry. Need to verify `--recipe` flag works with `info` subcommand (it may only be on `install`).

### Moderate Gaps

None.

### Major Gaps

None. Verified that `tsuku info --recipe <path> --json --metadata-only` is supported (cmd/tsuku/info.go line 16: `Use: "info <tool> | --recipe <path>"`).

## Recommendation
Proceed

## Notes

- #1256 (dependency) is closed -- no blocker.
- `tsuku info` supports both `--recipe <path>` and `--metadata-only` flags, so the approach in the issue is viable.
- The workflow was modified by #1271 to use `--recipe` for install; the same pattern applies to `info`.
- No ACs are already satisfied -- the workflow still uses simple grep-based `supported_os` / `os_mapping` checks (lines 98-110).
