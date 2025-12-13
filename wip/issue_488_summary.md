# Issue 488 Summary

## Changes Made

### New Files
- `internal/builders/homebrew.go` - HomebrewBuilder implementation
- `internal/builders/homebrew_test.go` - Unit tests for HomebrewBuilder

### Implementation Details

#### HomebrewBuilder struct
Implements the `Builder` interface following the `GitHubReleaseBuilder` pattern:
- `httpClient` - HTTP client for API requests
- `factory` - LLM provider factory with circuit breaker failover
- `executor` - Container validation executor (optional)
- `sanitizer` - Error sanitization for LLM repair messages
- `homebrewAPIURL` - Base URL for Homebrew API (configurable for testing)
- `telemetryClient` - Telemetry event emitter
- `progress` - Progress reporter for stage updates

#### CanBuild method
- Validates formula name (security: prevent injection)
- Queries `formulae.brew.sh/api/formula/{name}.json`
- Returns `true` if formula exists and has bottles
- Returns `false` for disabled formulas or formulas without bottles

#### Build method
- Fetches formula metadata from Homebrew JSON API
- Runs LLM conversation with tool-use protocol
- Three tools available:
  - `fetch_formula_json`: Get formula metadata
  - `inspect_bottle`: Inspect bottle contents (placeholder)
  - `extract_recipe`: Signal completion with recipe data
- Repair loop (max 2 attempts) for validation failures
- Generates platform-agnostic recipe using `homebrew_bottle` action

#### Security Controls
- Formula name validation (prevent path traversal, injection)
- Platform tag validation (whitelist of valid tags)
- URL allowlist enforcement (formulae.brew.sh only for API)
- Input sanitization (control characters removed)
- Schema enforcement (LLM output excludes checksums)

### Generated Recipe Structure
```toml
[metadata]
name = "ripgrep"
description = "Search tool like grep and The Silver Searcher"
homepage = "https://github.com/BurntSushi/ripgrep"
runtime_dependencies = ["pcre2"]

[version]
source = "homebrew"
formula = "ripgrep"

[[steps]]
action = "homebrew_bottle"
formula = "ripgrep"

[[steps]]
action = "install_binaries"
binaries = ["bin/rg"]

[verify]
command = "rg --version"
```

## Test Coverage
- `TestHomebrewBuilder_Name` - Returns "homebrew"
- `TestHomebrewBuilder_CanBuild_Success` - Formula with bottles
- `TestHomebrewBuilder_CanBuild_NotFound` - Missing formula
- `TestHomebrewBuilder_CanBuild_NoBottles` - Formula without bottles
- `TestHomebrewBuilder_CanBuild_Disabled` - Disabled formula
- `TestHomebrewBuilder_CanBuild_InvalidName` - Formula name validation
- `TestHomebrewBuilder_isValidPlatformTag` - Platform tag validation
- `TestHomebrewBuilder_fetchFormulaInfo` - API response parsing
- `TestHomebrewBuilder_generateRecipe` - Recipe generation
- `TestHomebrewBuilder_generateRecipe_NoExecutables` - Error handling
- `TestHomebrewBuilder_sanitizeFormulaJSON` - Input sanitization
- `TestHomebrewBuilder_buildSystemPrompt` - System prompt content
- `TestHomebrewBuilder_buildUserMessage` - User message construction
- `TestHomebrewBuilder_buildToolDefs` - Tool definitions
- `TestHomebrewBuilder_executeToolCall_ExtractRecipe` - Extract recipe tool
- `TestHomebrewBuilder_executeToolCall_FetchFormulaJSON` - Fetch formula tool
- `TestHomebrewBuilder_executeToolCall_InvalidFormula` - Security validation
- `TestHomebrewBuilder_executeToolCall_InspectBottle` - Inspect bottle tool
- `TestHomebrewBuilder_executeToolCall_InvalidPlatform` - Platform validation
- `TestHomebrewBuilder_executeToolCall_UnknownTool` - Error handling

## Limitations
- `inspect_bottle` tool returns placeholder (full implementation requires downloading bottles)
- Validation requires executor configuration (skipped by default)
- No dependency tree discovery (planned for issue #489)

## Acceptance Criteria Status
| Criteria | Status |
|----------|--------|
| Create `internal/builders/homebrew.go` | Done |
| Implement `CanBuild()` querying Homebrew API | Done |
| Implement `Build()` with LLM conversation | Done |
| Three tools: fetch_formula_json, inspect_bottle, extract_recipe | Done |
| Platform-agnostic recipes using homebrew_bottle | Done |
| Schema enforcement (no checksums) | Done |
| Input sanitization | Done |
| URL allowlist validation | Partial (API only) |
| Unit tests | Done |
