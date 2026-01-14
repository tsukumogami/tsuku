# Implementation Context: Issue #872

**Source**: docs/designs/DESIGN-tap-support.md

## Key Design Decisions

1. **Dedicated `tap` version provider** - Follows cask design pattern, separate from core homebrew provider
2. **GitHub raw content fetching** - Fetch formula files from `raw.githubusercontent.com`
3. **Ruby regex parsing** - Target stable patterns: `version "x.y.z"`, `bottle do...end`, `root_url`, `sha256`
4. **Platform tag mapping** - Map Go runtime to Homebrew platform tags (arm64_sonoma, sonoma, x86_64_linux, etc.)

## Files to Create

- `internal/version/tap.go` - TapProvider implementing VersionResolver
- `internal/version/tap_parser.go` - Ruby formula parsing via regex
- `internal/version/tap_test.go` - Unit tests with mocked HTTP responses

## Integration Points

- Implements `VersionResolver` interface (existing)
- Returns `TapVersionInfo` for template variable substitution
- Works with existing `homebrew` action for bottle downloading

## Downstream Dependencies

This issue blocks:
- #873 (tap cache) - needs TapProvider and TapVersionInfo types
- #874 (factory integration) - needs TapProvider implementing VersionResolver  
- #875 (GitHub token support) - needs HTTP fetching logic to extend
