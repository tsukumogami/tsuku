# Issue #1026 Baseline

## Branch
`chore/1026-goreleaser-config`

## Test Results
All tests pass (go test ./...)

## Current State
- .goreleaser.yaml has draft: false
- ldflags inject buildinfo.version and buildinfo.commit
- No pinnedDltestVersion variable exists

## Task
1. Create internal/verify/version.go with pinnedDltestVersion variable
2. Update .goreleaser.yaml:
   - Change draft: false to draft: true
   - Add pinnedDltestVersion ldflag
