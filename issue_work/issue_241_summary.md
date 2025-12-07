# Issue 241 Summary

## What Changed

Enhanced `tsuku remove` to warn (not error) when removing tools with dependents and allow `--force` to proceed.

## Files Modified

- `cmd/tsuku/remove.go` - Changed error to warning, added --force flag

## Key Decisions

1. **Warning with --force**: Changed from hard error to warning with bypass option
2. **Backward compatible exit codes**: Still exits non-zero without --force
3. **Uses existing RequiredBy**: Leverages existing state tracking, no new data needed

## Testing

- All 17 test packages pass
- Manual verification: `./tsuku remove --help` shows new -f/--force flag

## Acceptance Criteria

- [x] Uninstall checks if any installed tool depends on target (via RequiredBy)
- [x] Warning lists dependent tools
- [x] User can proceed with `--force`
- [x] Hidden deps can be removed with --force
