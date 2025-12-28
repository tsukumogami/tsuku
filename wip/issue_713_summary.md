# Issue 713 Summary

## What Was Implemented

Added `--version` flag to `tsuku eval` command that works with `--recipe` mode to specify the version when generating plans from local recipe files.

## Changes Made

- `cmd/tsuku/eval.go`:
  - Added `evalVersion` variable
  - Added `--version` flag registration in `init()`
  - Added validation that `--version` requires `--recipe`
  - Set `reqVersion` from `evalVersion` in recipe mode
  - Updated Long description and examples to document the new flag

## Key Decisions

- **Flag vs @syntax**: Used a separate `--version` flag rather than trying to reuse the `@version` syntax (which doesn't make sense for file paths). This keeps the UX clean and consistent with other flags like `--os` and `--arch`.

- **Mutual exclusivity**: `--version` requires `--recipe` - using `--version` without `--recipe` produces a clear error. Registry mode continues to use the existing `tool@version` syntax.

## Trade-offs Accepted

- **No additional unit tests**: The validation logic is simple (one conditional), and the version propagation is tested through existing executor tests. Manual testing confirmed the behavior.

## Test Coverage

- New tests added: 0
- Existing tests: All pass
- Manual verification: Confirmed all acceptance criteria met

## Known Limitations

None. The implementation is straightforward and follows existing patterns.
