# Issue 488 Implementation Plan

## Overview
Implement `HomebrewBuilder` that uses LLM conversation with tool-use protocol to generate tsuku recipes from Homebrew bottle formulas.

## Architecture

### Pattern: Follow GitHubReleaseBuilder
The implementation follows the established `GitHubReleaseBuilder` pattern in `internal/builders/github_release.go`:
- Struct with dependencies (httpClient, factory, executor, sanitizer, telemetryClient, progress)
- `CanBuild()` checks if package exists in Homebrew
- `Build()` orchestrates LLM conversation with tool-use
- `generateWithRepair()` implements retry loop (max 2 attempts)
- Tool definitions for LLM interaction

### Key Dependencies
- `internal/version/homebrew.go` - Homebrew API integration (reuse `homebrewFormulaInfo` struct)
- `internal/actions/homebrew_bottle.go` - Existing bottle download/extraction action
- `internal/llm` - LLM client and tool definitions
- `internal/validate/sanitize.go` - Input sanitization patterns

## Implementation Steps

### Step 1: Create HomebrewBuilder struct
File: `internal/builders/homebrew.go`

```go
type HomebrewBuilder struct {
    httpClient      *http.Client
    factory         llm.Factory
    executor        executor.Executor
    sanitizer       *validate.Sanitizer
    telemetryClient telemetry.Client
    progress        func(string)
}
```

Constructor: `NewHomebrewBuilder(factory, executor, sanitizer, telemetryClient, progress) *HomebrewBuilder`

### Step 2: Implement CanBuild()
- Query `https://formulae.brew.sh/api/formula/{name}.json`
- Return true if formula exists and has bottles
- Reuse validation logic from `internal/version/homebrew.go`

### Step 3: Implement Build()
- Fetch formula JSON from Homebrew API
- Call `generateWithRepair()` for LLM conversation
- Parse and validate generated recipe
- Return BuildResult with recipe content

### Step 4: Implement generateWithRepair()
- Max 2 repair attempts (following GitHubReleaseBuilder pattern)
- LLM conversation with tool-use protocol
- Validate output against expected schema
- Retry with error feedback on validation failure

### Step 5: Define Homebrew-specific tools

#### Tool 1: fetch_formula_json
- Fetches formula metadata from `formulae.brew.sh/api/formula/{name}.json`
- Returns sanitized JSON (remove sensitive fields)
- URL allowlist: only `formulae.brew.sh`

#### Tool 2: inspect_bottle
- Downloads bottle from GHCR, extracts to temp directory
- Lists directory structure and key files
- Returns file listing for LLM analysis
- URL allowlist: `ghcr.io`, `github.com`

#### Tool 3: extract_recipe
- Called by LLM to output the generated recipe
- Validates TOML syntax and required fields
- Schema enforcement: no checksums in LLM output

### Step 6: Implement input sanitization
- Sanitize formula metadata before sending to LLM
- Remove/escape potentially dangerous content
- Use patterns from `internal/validate/sanitize.go`

### Step 7: Implement URL allowlist
- Only allow requests to:
  - `formulae.brew.sh` (formula API)
  - `ghcr.io` (bottle downloads)
  - `github.com` (release assets)
- Reject any URLs outside allowlist

### Step 8: Generated recipe structure
Following design doc, recipes use `homebrew_bottle` action:

```toml
name = "ripgrep"
description = "Search tool like grep"
homepage = "https://github.com/BurntSushi/ripgrep"

[version]
provider = "homebrew"
formula = "ripgrep"

[[install]]
action = "homebrew_bottle"
formula = "ripgrep"
binaries = ["rg"]
```

Platform detection happens at runtime via the `homebrew_bottle` action.

## Testing Strategy

### Unit Tests
File: `internal/builders/homebrew_test.go`

1. `TestHomebrewBuilder_Name` - Returns "homebrew"
2. `TestHomebrewBuilder_CanBuild_Success` - Formula with bottles returns true
3. `TestHomebrewBuilder_CanBuild_NotFound` - Missing formula returns false
4. `TestHomebrewBuilder_CanBuild_NoBottles` - Formula without bottles returns false
5. `TestHomebrewBuilder_Build_Success` - Successful recipe generation
6. `TestHomebrewBuilder_Build_RepairLoop` - Tests retry on validation failure
7. `TestHomebrewBuilder_URLAllowlist` - Rejects disallowed URLs
8. `TestHomebrewBuilder_InputSanitization` - Sanitizes formula metadata

### Mock Strategy
- Mock HTTP client for Homebrew API responses
- Mock LLM factory for controlled tool-use conversations
- Use test fixtures for formula JSON samples

## Files to Create/Modify

### New Files
- `internal/builders/homebrew.go` - Main implementation
- `internal/builders/homebrew_test.go` - Unit tests

### Files to Reference (no modifications)
- `internal/builders/builder.go` - Builder interface
- `internal/builders/github_release.go` - Pattern reference
- `internal/version/homebrew.go` - Homebrew API types
- `internal/actions/homebrew_bottle.go` - Bottle action
- `internal/validate/sanitize.go` - Sanitization patterns

## Acceptance Criteria Mapping

| Criteria | Implementation |
|----------|---------------|
| Create `internal/builders/homebrew.go` | Step 1-7 |
| Implement `CanBuild()` querying Homebrew API | Step 2 |
| Implement `Build()` with LLM conversation | Steps 3-4 |
| Three tools: fetch_formula_json, inspect_bottle, extract_recipe | Step 5 |
| Platform-agnostic recipes using homebrew_bottle | Step 8 |
| Schema enforcement (no checksums) | Step 5 (extract_recipe) |
| Input sanitization | Step 6 |
| URL allowlist validation | Step 7 |
| Unit tests | Testing Strategy |

## Risk Mitigation

1. **LLM output validation**: Strict TOML parsing and schema validation
2. **URL security**: Allowlist prevents SSRF attacks
3. **Input sanitization**: Prevents prompt injection from formula metadata
4. **Repair loop limit**: Max 2 attempts prevents infinite loops
