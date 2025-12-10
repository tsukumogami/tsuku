# Issue 373 Summary

## What Was Implemented

Added `--skip-validation` flag to `tsuku create` that allows users without Docker to generate LLM-based recipes from GitHub releases. The flag requires explicit user consent via an interactive prompt before proceeding.

## Changes Made

- `cmd/tsuku/create.go`:
  - Added `createSkipValidation` flag variable
  - Added `confirmSkipValidation()` function for consent prompt
  - Modified GitHub builder creation to conditionally set up validation executor
  - Added warning output when validation was skipped
  - Set `llm_validation` metadata on recipes when validation skipped

- `internal/recipe/types.go`:
  - Added `LLMValidation` field to `MetadataSection` struct

## Key Decisions

- **Consent prompt required**: The flag requires explicit y/N consent, matching the design spec. Non-interactive mode refuses the flag entirely to prevent accidental use.

- **Reused existing patterns**: The confirmation logic follows the same pattern as `confirmInstall()` in install.go, using `bufio.Reader` and `isInteractive()`.

- **Metadata field vs comment**: Added `llm_validation` as a proper struct field with TOML tag rather than a comment, making it machine-readable and queryable.

## Trade-offs Accepted

- **No unit test for consent prompt**: The function reads from stdin and checks terminal mode, making unit testing complex. The logic is simple and follows an existing pattern.

- **Warning shown even when runtime unavailable**: If validation is skipped because no container runtime is available (not because of the flag), the warning still shows `ValidationSkipped = true`. This is existing behavior from the builder.

## Test Coverage

- New tests added: 0
- Coverage change: No change (no testable logic added beyond I/O interaction)

## Known Limitations

- The consent prompt only works in interactive mode. Non-interactive usage must use a different approach (future `--yes` flag from #374 could combine with this).

## Future Improvements

- Issue #374 will add `--yes` flag which could potentially bypass the consent prompt with a warning
- Issue #375 will add recipe preview flow before installation
