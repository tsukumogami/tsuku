# Issue 240 Summary

## What Changed

Enhanced `tsuku info <tool>` to display the dependency tree.

## Files Modified

- `cmd/tsuku/info.go` - Added dependency tree display

## Key Decisions

1. **Data source by status**: Installed tools read from state.json, uninstalled tools resolve from recipe
2. **Transitive resolution**: Full transitive deps resolved for uninstalled tools using `actions.ResolveTransitive()`
3. **Output format**: Dependencies shown with indentation under separate sections

## Testing

- All 17 test packages pass
- Manual verification:
  - `tsuku info turbo` shows nodejs as install/runtime dep
  - `tsuku info poetry` shows transitive deps (pipx, python-standalone)
  - JSON output includes dependency arrays

## Acceptance Criteria

- [x] `tsuku info` shows install dependencies with indentation
- [x] `tsuku info` shows runtime dependencies separately
- [x] Transitive deps shown (for uninstalled tools via resolution)
- [x] Works for installed tools (reads state.json)
- [x] Works for uninstalled tools (resolves from recipe)
