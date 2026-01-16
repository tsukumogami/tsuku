# Implementation Context: Issue #874

**Source**: docs/designs/DESIGN-tap-support.md

## Design Overview

Issue #874 implements **Slice 3: Template Integration** from the Homebrew Tap Support design. This integrates the TapProvider with the provider factory.

## Key Implementation Points

1. **TapSourceStrategy**: Implement `ProviderStrategy` interface (`CanHandle`, `Create`, `Priority`)
2. **Short form parsing**: Handle `tap:owner/repo/formula` syntax
3. **Registration**: Add to `NewProviderFactory()` at `PriorityKnownRegistry` (100)
4. **Template variables**: Populate `VersionInfo.Metadata` with bottle_url, checksum, tap, formula

## Dependencies (All Completed)

- #872: Tap provider core - provides `TapProvider`
- #862: Template infrastructure for dotted-path substitution

## Recipe Syntax Support

**Explicit form:**
```toml
[version]
source = "tap"
tap = "hashicorp/tap"
formula = "terraform"
```

**Short form:**
```toml
[version]
source = "tap:hashicorp/tap/terraform"
```

Short form parsing: `tap:{owner}/{repo}/{formula}`
- `owner` = GitHub organization or user
- `repo` = Tap name without `homebrew-` prefix
- `formula` = Formula name

## Acceptance Criteria

- TapSourceStrategy struct implementing ProviderStrategy interface
- Strategy registered in NewProviderFactory() at PriorityKnownRegistry (100)
- CanHandle returns true when r.Version.Source == "tap" or source starts with "tap:"
- Short form parsing extracts owner, repo, formula from tap:owner/repo/formula
- Short form parsing handles edge cases (missing parts, malformed input)
- Create method instantiates TapProvider with correct tap and formula parameters
- Unit tests for TapSourceStrategy.CanHandle() with explicit and short form sources
- Unit tests for short form parsing logic
