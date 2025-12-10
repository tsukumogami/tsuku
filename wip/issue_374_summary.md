# Issue 374 Summary

## What Was Implemented

Added `--yes` flag to `tsuku create` command that skips recipe preview confirmation and displays a warning message to users when used.

## Changes Made

- `cmd/tsuku/create.go`: Added `createAutoApprove` variable, `--yes` flag registration, and warning message output

## Key Decisions

- Placed warning at start of `runCreate()`: Ensures warning is visible before any network calls or processing
- Used stderr for warning: Follows convention for warnings and keeps stdout clean for data output
- Minimal implementation: Flag infrastructure is in place for #375 (recipe preview flow) to use

## Trade-offs Accepted

- Warning displays even without preview flow: This is intentional - users see consistent behavior whether or not #375 has landed

## Test Coverage

- New tests added: 0 (flag parsing handled by cobra)
- Coverage change: N/A (trivial code path)

## Known Limitations

- The `--yes` flag currently only shows a warning; the actual preview skip behavior will be implemented in #375

## Future Improvements

- #375 will implement the recipe preview flow that checks `createAutoApprove` to skip the confirmation prompt
