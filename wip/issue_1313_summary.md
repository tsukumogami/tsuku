# Issue 1313 Summary

## What Was Implemented

Added entry validation to `ParseRegistry` that rejects discovery registry entries with empty `builder` or `source` fields at parse time. Also fixed a pre-existing stdlib log import that was failing the project's lint test.

## Changes Made

- `internal/discover/registry.go`: Added `validateEntries()` method called from `ParseRegistry` after JSON unmarshal and schema version check
- `internal/discover/registry_test.go`: Added 5 tests: empty builder, missing builder, empty source, missing source, optional binary allowed
- `internal/discover/chain.go`: Replaced stdlib `log` import with `internal/log` package

## Key Decisions

- Validation fails the entire registry load rather than silently skipping bad entries
- Error messages include both tool name and which field is invalid

## Test Coverage

- 5 new tests added
- Pre-existing TestNoStdlibLog now passes (was failing due to chain.go stdlib log import)
