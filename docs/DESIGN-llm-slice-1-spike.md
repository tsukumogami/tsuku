# Design Document: LLM Slice 1 - End-to-End Spike

**Status**: Proposed

**Parent Design**: [DESIGN-llm-builder-infrastructure.md](DESIGN-llm-builder-infrastructure.md)

**Issue**: [#266 - Slice 1: End-to-End Spike](https://github.com/tsukumogami/tsuku/issues/266)

## Context and Problem Statement

The LLM Builder Infrastructure milestone (M12) proposes using LLMs to generate tsuku recipes from GitHub release data. Before investing in infrastructure components like container validation, provider abstraction, or repair loops, we need to validate the core hypothesis:

**Can an LLM correctly match GitHub release assets to platform/architecture combinations and produce syntactically valid, working recipes?**

This is the riskiest assumption in the entire milestone. If the LLM cannot reliably produce working recipes from release data, the infrastructure built around it would be wasted effort.

### The Hypothesis

Given a GitHub repository with release assets, an LLM can:
1. Analyze asset filenames and identify platform/architecture patterns
2. Generate valid TOML recipe syntax following tsuku conventions
3. Produce recipes that actually install and work when manually tested

### Why This First

The parent design outlines four vertical slices. Slice 1 exists specifically to prove the LLM's capability before investing in:
- Container validation infrastructure (Slice 2)
- Repair loops and multi-provider abstraction (Slice 3)
- Production UX and error handling (Slice 4)

If the spike fails, we can abandon the approach early. If it succeeds, we have confidence to build the supporting infrastructure.

### Scope

**In scope:**
- Direct Claude API integration (single provider, hardcoded model)
- GitHub API integration to fetch release data
- Tool use to get structured asset mappings from Claude
- Recipe generation from asset mappings
- Basic cost tracking (tokens used, estimated cost displayed)
- Manual validation (user runs `tsuku install` from generated file)

**Out of scope (deferred to later slices):**
- Container-based validation
- Repair loops on validation failure
- Multi-provider abstraction (Gemini support)
- Configuration management (API keys via env vars only)
- Error handling beyond basic
- CLI integration (`tsuku create` command)
- Rate limiting, budgets, or confirmation prompts

### Success Criteria

- [ ] LLM produces syntactically valid TOML recipes
- [ ] Generated recipe can be manually installed (`tsuku install` from local file)
- [ ] Works for at least 3 test repos with different naming conventions
- [ ] Cost per generation is tracked and displayed

### Failure Criteria

The hypothesis is disproved if any of:
- Less than 80% of generated recipes are syntactically valid TOML
- Less than 50% of syntactically valid recipes successfully install
- Cannot generate working recipes for at least 3 mainstream CLI tools
- Cost per generation exceeds $0.15 (more than 2x estimated upper bound)

### What We'll Learn

Regardless of outcome, this spike will reveal:
1. **Asset matching accuracy**: How well does Claude understand release naming conventions?
2. **Recipe completeness**: Does it infer correct extraction steps, binary names, verify commands?
3. **Token consumption**: Actual cost per recipe (estimated ~$0.02-0.06 in parent design)
4. **Prompt sensitivity**: How much prompt engineering is needed for consistent results?

## Assumptions

- Claude API key available with sufficient quota for experimentation (~100 requests)
- GitHub release asset naming is sufficient signal (no README parsing needed for spike)
- Test repos (cli/cli, BurntSushi/ripgrep, sharkdp/fd) are representative of target tools
- Tool use pattern is appropriate for structured extraction (vs. text parsing)
- Manual validation is acceptable for initial spike (5-10 recipes tested)
- Cost measured in API tokens; developer time is not tracked as a cost metric

## Decision Drivers

- **Minimal investment**: Prove/disprove the hypothesis with minimal code
- **Fast feedback**: Quick iteration on prompts and tool definitions
- **Reusable learnings**: Prompts and tool schemas will carry forward to production
- **No abstractions yet**: Direct API calls, no interfaces or factories

## External Research

### Anthropic Go SDK

Anthropic provides an official Go SDK: [github.com/anthropics/anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go)

Key characteristics:
- Requires Go 1.22+
- API key via `ANTHROPIC_API_KEY` environment variable (default)
- Native tool use support with JSON Schema input definitions
- Usage tracking returned with each response (InputTokens, OutputTokens)

Basic usage pattern:
```go
client := anthropic.NewClient()  // Uses ANTHROPIC_API_KEY from env

message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
    Model:     anthropic.ModelClaudeSonnet4_5_20250929,
    MaxTokens: 1024,
    Messages:  []anthropic.MessageParam{...},
    Tools:     []anthropic.ToolParam{...},
})
```

### Tool Use Response Structure

When Claude uses a tool, the response contains:
- `stop_reason: "tool_use"` - indicates tool was called
- Content array with `tool_use` block containing:
  - `id`: Tool call identifier
  - `name`: Tool name
  - `input`: Structured JSON matching the tool's input_schema

For this spike, we use `tool_choice: {"type": "tool", "name": "match_assets"}` to force Claude to use our tool, ensuring structured output.

### GitHub Release API

GitHub's REST API provides release data at:
```
GET /repos/{owner}/{repo}/releases/latest
```

Response includes `assets` array with:
- `name`: Filename (e.g., "gh_2.42.0_linux_amd64.tar.gz")
- `browser_download_url`: Direct download URL
- `size`: File size in bytes
- `content_type`: MIME type

## Considered Options

### Option 1: Standalone Spike Binary

Create a separate `cmd/spike-llm/main.go` binary that:
- Takes a GitHub repo URL as argument
- Fetches release data
- Calls Claude API directly
- Prints generated TOML to stdout

**Pros:**
- Fully isolated from production code
- Can be deleted after spike with no trace
- No risk of coupling to spike decisions
- Can commit generated recipes to testdata/ for inspection
- Easy to benchmark in isolation

**Cons:**
- Prompts and schemas must be manually ported to production code
- Cannot leverage existing recipe validation logic
- Must duplicate HTTP client setup
- Learnings are not documented in production codebase

### Option 2: Internal Package with CLI Hook

Create `internal/llm/spike.go` with a public `GenerateRecipe` function. Add a hidden `--llm-spike` flag to `tsuku create`.

**Pros:**
- Code can evolve into production implementation
- Uses existing HTTP client patterns and recipe validation
- Integrated testing possible with existing fixtures
- Learnings about prompt engineering documented in code

**Cons:**
- Pollutes main binary with experimental code
- Risk of shipping spike code accidentally
- Harder to deprecate if approach changes (hidden flag becomes tech debt)
- May create premature coupling (hardcoded model/provider)

### Option 3: Integration Test as Spike

Write the spike as a test file (`internal/llm/spike_test.go`) that can be run with `go test -run TestSpike -v`.

**Pros:**
- Clearly experimental
- Built-in assertion framework for validating recipe structure
- Can use `t.Skip()` to disable in CI initially
- Can output to testdata/ for inspection

**Cons:**
- Semantic mismatch: tests assert correctness, spikes explore feasibility
- Cannot easily parameterize repo URL without env vars
- Running "tests" implies correctness checking, misleading for experimental code
- Go test framework expects pass/fail, not exploratory output

### Option 4: Python Notebook for Prompt Iteration

Use a Python Jupyter notebook or REPL to:
- Prototype prompts interactively
- Inspect API responses immediately
- Iterate on tool schemas visually
- Export final prompt/tool definitions to Go

**Pros:**
- Fastest iteration cycle (no compile step)
- Rich inspection of API responses (JSON pretty-print, diffs)
- Can compare prompt variants side-by-side
- Industry-standard approach for LLM experimentation
- Claude API has official Python SDK

**Cons:**
- Results must be manually ported to Go
- Different HTTP client than production
- Requires Python environment setup
- Not integrated with tsuku codebase
- Two-language context switching

### Evaluation

| Criterion | Option 1 | Option 2 | Option 3 | Option 4 |
|-----------|----------|----------|----------|----------|
| Isolation | Excellent | Poor | Good | Excellent |
| Reusability | Poor | Good | Fair | Poor |
| Iteration speed | Good | Good | Poor | Excellent |
| Risk of pollution | None | High | Low | None |
| Learning capture | Fair | Good | Fair | Poor |

## Decision Outcome

**Chosen: Option 1 (Standalone Spike Binary)**

Create `cmd/spike-llm/main.go` as a throwaway experiment. This maximizes isolation and makes the spike's temporary nature explicit.

### Rationale

1. **Clear boundaries**: A separate binary cannot accidentally ship or couple to production code
2. **Fast iteration**: Direct `go run` with immediate stdout output
3. **Explicit throwaway**: Named "spike" to signal temporary nature
4. **Learning capture**: Prompts and tool schemas are the reusable artifacts, not code structure
5. **Single language**: Stays in Go ecosystem, avoiding context switch to Python

### Why Not Option 4 (Python Notebook)?

Option 4 (Python) offers the fastest prompt iteration, but:
- Production code is Go; Python adds a translation step that may introduce bugs
- The Anthropic Go SDK is mature; no need to prototype in Python first
- Go compilation is fast enough (<1s) that iteration speed difference is marginal
- Keeping everything in Go means the spike code, while throwaway, serves as a direct reference for production implementation

For teams unfamiliar with Go or with more complex prompt engineering needs, Option 4 would be preferred. For this spike, Go is sufficient.

### Trade-offs Accepted

- Code will be rewritten for production (acceptable for a spike)
- HTTP client will be minimal (no SSRF protection for dev-only tool)
- Prompt iteration slightly slower than Python notebook approach

## Solution Architecture

### Overview

```
cmd/spike-llm/main.go
├── main()
│   ├── Parse args (owner/repo)
│   ├── Fetch GitHub release
│   ├── Call Claude with tool
│   ├── Parse tool response
│   ├── Generate recipe TOML
│   └── Print to stdout with cost
```

### Data Structures

```go
// Input: GitHub release data
type Release struct {
    TagName string  `json:"tag_name"`  // e.g., "v2.42.0"
    Name    string  `json:"name"`      // Release title
    Assets  []Asset `json:"assets"`
}

type Asset struct {
    Name               string `json:"name"`                 // e.g., "gh_2.42.0_linux_amd64.tar.gz"
    BrowserDownloadURL string `json:"browser_download_url"` // Direct download URL
}

// Output: LLM tool response
type AssetPattern struct {
    Pattern     string            `json:"pattern"`      // e.g., "gh_{version}_{os}_{arch}.tar.gz"
    OSMapping   map[string]string `json:"os_mapping"`   // e.g., {"linux": "linux", "darwin": "darwin"}
    ArchMapping map[string]string `json:"arch_mapping"` // e.g., {"amd64": "amd64", "arm64": "arm64"}
    Format      string            `json:"format"`       // e.g., "tar.gz"
    Executable  string            `json:"executable"`   // e.g., "gh"
    Verify      string            `json:"verify"`       // e.g., "gh --version"
}
```

### Components

#### 1. GitHub Release Fetcher

Minimal HTTP client to fetch release data:

```go
func fetchLatestRelease(owner, repo string) (*Release, error)
```

- Endpoint: `https://api.github.com/repos/{owner}/{repo}/releases/latest`
- Uses `GITHUB_TOKEN` if available (for rate limits)
- Returns release tag and asset list
- Also fetches repo metadata (description, homepage) via `/repos/{owner}/{repo}`

#### 2. Claude Client

Direct use of Anthropic Go SDK:

```go
func callClaude(ctx context.Context, tag string, assets []Asset) (*AssetPattern, *Usage, error)
```

Uses tool use to get structured output:
- Model: `claude-sonnet-4-5-20250929` (cost-effective, capable)
- Tool: `extract_pattern` with structured schema
- Force tool use via `tool_choice: { type: "tool", name: "extract_pattern" }`

#### 3. Tool Definition

The `extract_pattern` tool asks the LLM to directly produce the asset pattern template rather than specific asset matches. This is simpler because it eliminates the need for post-processing to extract patterns from specific filenames.

```json
{
  "name": "extract_pattern",
  "description": "Extract the asset naming pattern from GitHub release assets for a tsuku recipe",
  "input_schema": {
    "type": "object",
    "properties": {
      "asset_pattern": {
        "type": "string",
        "description": "Pattern with {version}, {os}, {arch} placeholders, e.g., 'tool-{version}-{os}-{arch}.tar.gz'"
      },
      "os_mapping": {
        "type": "object",
        "description": "Map standard OS names to the strings used in filenames",
        "additionalProperties": { "type": "string" }
      },
      "arch_mapping": {
        "type": "object",
        "description": "Map standard arch names to the strings used in filenames",
        "additionalProperties": { "type": "string" }
      },
      "archive_format": {
        "type": "string",
        "enum": ["tar.gz", "tar.xz", "zip"],
        "description": "Archive format for extraction"
      },
      "executable": {
        "type": "string",
        "description": "Name of the main executable binary"
      },
      "verify_command": {
        "type": "string",
        "description": "Command to verify installation (e.g., 'gh --version')"
      }
    },
    "required": ["asset_pattern", "os_mapping", "arch_mapping", "archive_format", "executable", "verify_command"]
  }
}
```

Example response for cli/cli:
```json
{
  "asset_pattern": "gh_{version}_{os}_{arch}.tar.gz",
  "os_mapping": { "linux": "linux", "darwin": "macOS" },
  "arch_mapping": { "amd64": "amd64", "arm64": "arm64" },
  "archive_format": "tar.gz",
  "executable": "gh",
  "verify_command": "gh --version"
}
```

#### 4. Recipe Generator

Convert tool response to TOML recipe:

```go
func generateRecipe(repo string, mappings *AssetMappings) string
```

Produces recipe with:
- `[metadata]`: name, description, homepage
- `[version]`: source = "github_releases", github_repo
- `[[steps]]`: download, extract, install_binaries
- `[verify]`: command from LLM

#### 5. Cost Calculator

Calculate and display cost from usage:

```go
func calculateCost(usage Usage) float64
```

Claude Sonnet pricing (as of 2025):
- Input: $3 per 1M tokens
- Output: $15 per 1M tokens

### System Prompt

```
You are extracting asset naming patterns from GitHub release assets for tsuku, a package manager for developer tools.

Given a list of release assets and the release version tag, identify:
1. The pattern used for asset filenames with {version}, {os}, {arch} placeholders
2. How standard OS names (linux, darwin, windows) appear in filenames
3. How standard arch names (amd64, arm64) appear in filenames
4. The archive format (tar.gz, tar.xz, or zip)
5. The main executable name
6. A command to verify the tool works

Common naming patterns:
- Rust targets: x86_64-unknown-linux-musl, aarch64-apple-darwin
- Go conventions: linux_amd64, darwin_arm64, macOS_arm64
- Generic: linux-x64, macos-arm64

Preferences:
- musl over glibc for Linux (more portable)
- tar.gz or tar.xz over zip for Unix platforms

Output the pattern with placeholders, NOT specific filenames. For example:
- "gh_{version}_{os}_{arch}.tar.gz" not "gh_2.42.0_linux_amd64.tar.gz"
- "ripgrep-{version}-{arch}-{os}.tar.gz" not "ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz"
```

### Data Flow

```
1. User runs: go run ./cmd/spike-llm cli/cli

2. Fetch release metadata:
   GET https://api.github.com/repos/cli/cli/releases/latest
   → tag_name: "v2.42.0", 20+ assets with names like "gh_2.42.0_linux_amd64.tar.gz"

   GET https://api.github.com/repos/cli/cli
   → description: "GitHub CLI", homepage: "https://cli.github.com"

3. Call Claude:
   POST https://api.anthropic.com/v1/messages
   - System prompt (pattern extraction instructions)
   - User message: {"tag": "v2.42.0", "assets": ["gh_2.42.0_linux_amd64.tar.gz", ...]}
   - Tool: extract_pattern
   - tool_choice: { type: "tool", name: "extract_pattern" }

4. Parse response:
   → tool_use block:
     asset_pattern: "gh_{version}_{os}_{arch}.tar.gz"
     os_mapping: {"linux": "linux", "darwin": "macOS"}
     arch_mapping: {"amd64": "amd64", "arm64": "arm64"}
     executable: "gh"
     verify_command: "gh --version"

5. Generate recipe:
   [metadata]
   name = "gh"
   description = "GitHub CLI"
   homepage = "https://cli.github.com"

   [version]
   source = "github_releases"
   github_repo = "cli/cli"

   [[steps]]
   action = "github_archive"
   [steps.params]
   repo = "cli/cli"
   asset_pattern = "gh_{version}_{os}_{arch}.tar.gz"
   archive_format = "tar.gz"
   strip_dirs = 1
   binaries = ["gh"]
   os_mapping = { linux = "linux", darwin = "macOS" }
   arch_mapping = { amd64 = "amd64", arm64 = "arm64" }

   [verify]
   command = "gh --version"

6. Output:
   - Print TOML to stdout
   - Print cost summary to stderr
```

### Error Handling

For this spike, errors are fatal with descriptive messages:

- Missing `ANTHROPIC_API_KEY`: "Set ANTHROPIC_API_KEY environment variable"
- GitHub 404: "Repository not found or has no releases: {repo}"
- Claude API error: "Claude API error: {status} {body}"
- No tool use in response: "Claude did not call match_assets tool"
- Invalid tool response: "Failed to parse tool response: {error}"

### Testing Approach

Manual testing with a structured test matrix:

| Repository | Convention | Expected Difficulty | Goal |
|------------|------------|---------------------|------|
| cli/cli | Go (linux_amd64) | Easy | Baseline validation |
| BurntSushi/ripgrep | Rust (x86_64-unknown-linux-musl) | Medium | Different naming |
| sharkdp/fd | Rust | Medium | Confirm Rust pattern |
| hashicorp/terraform | Go (many platforms) | Medium | Many variants |
| jqlang/jq | Generic (linux-amd64) | Medium | Generic naming |

**Validation steps for each:**
1. Generate recipe: `go run ./cmd/spike-llm owner/repo > /tmp/tool.toml`
2. Verify TOML syntax: `cat /tmp/tool.toml | toml-lint` (or manual inspection)
3. Copy to recipes: `cp /tmp/tool.toml ~/.tsuku/recipes/tool.toml`
4. Install: `tsuku install tool`
5. Verify: Run the verify command from recipe

**Success metrics:**
- 4/5 recipes syntactically valid (80%)
- 3/5 recipes install and run successfully (60%, with expectation to improve)
- Document failures for future prompt improvement

**Recording results:**
- Save all generated recipes to `testdata/spike-llm/` for reference
- Document actual cost per generation
- Note any prompt adjustments needed

## Security Considerations

### Threat Model

**In scope (spike must address):**
- API key exposure via logging
- Memory exhaustion from unbounded API responses
- Indefinite hangs from missing timeouts

**Out of scope (deferred to later slices):**
- Prompt injection attacks (Slice 4: asset sanitization)
- LLM hallucination validation (Slice 2: container validation)
- Cost explosion prevention (Slice 4: budgets/confirmations)
- SSRF attacks (Slice 4: URL allowlisting)

**Explicit non-threats:**
- Recipe execution risk: User manually reviews before install
- Multi-tenancy: Single-user dev tool
- Replay attacks: No authentication, stateless API calls

### API Key Handling

- API key read from `ANTHROPIC_API_KEY` environment variable
- Never logged or printed
- HTTP client constructed without request logging to prevent accidental key exposure
- No config file support (environment only for spike)
- Optional `GITHUB_TOKEN` follows same pattern (Authorization header only, never logged)

### HTTP Client Configuration

- GitHub API: 60 second timeout, 10MB response size limit via `io.LimitReader`
- Claude API: 120 second timeout, 5MB response size limit
- Context cancellation supported for graceful shutdown
- Content-Type validation: require `application/json` responses
- No automatic retries (fail fast for spike)

### LLM Output Trust

- Recipe is printed to stdout, NOT automatically installed
- User must manually copy to recipes directory
- User reviews TOML before installation
- No execution of LLM output during recipe generation
- Installation (existing `tsuku install`) is separate and user-initiated

### Data Sent to Anthropic

- GitHub release asset names (public data)
- Repository name (public)
- Release version tag (public)
- No user data, no local file contents, no credentials

### Known Limitations

**LLM hallucination risk:**
- LLM may generate syntactically valid but non-functional recipes
- Spike has NO automated validation (deferred to Slice 2)
- User must manually test generated recipes before use

**Cost controls:**
- No budget limits or confirmation prompts in spike
- Cost tracking is display-only, not preventative
- Recommendation: Set daily spend limit in Anthropic Console

**strip_dirs inference:**
- Cannot determine archive directory structure from asset names alone
- Default to `strip_dirs = 1` (works for most cases)
- Manual adjustment may be needed if binary not found after install

## Consequences

### Positive

1. **Fast validation**: Proves/disproves core hypothesis in days, not weeks
2. **Reusable artifacts**: Prompt and tool schema carry forward
3. **Cost data**: Real measurements instead of estimates
4. **Learning**: Understand LLM behavior with real release data

### Negative

1. **Throwaway code**: Spike code will be rewritten
2. **No automation**: Manual testing only
3. **Limited error handling**: Developer experience, not user experience

### Mitigations

1. **Document learnings**: Record prompt iterations, failure modes, costs
2. **Save successful recipes**: Build a test corpus for future validation
3. **Time-box**: If not working after reasonable effort, document blockers

## Implementation Checklist

- [ ] Create `cmd/spike-llm/main.go`
- [ ] Implement GitHub release fetcher (with repo metadata)
- [ ] Implement Claude client with tool use
- [ ] Define `extract_pattern` tool schema
- [ ] Write system prompt
- [ ] Implement recipe generator (using `github_archive` action)
- [ ] Add cost calculation
- [ ] Test with cli/cli (baseline, Go naming)
- [ ] Test with BurntSushi/ripgrep (Rust naming)
- [ ] Test with sharkdp/fd (confirm Rust pattern)
- [ ] Test with hashicorp/terraform (many variants)
- [ ] Test with jqlang/jq (generic naming)
- [ ] Save all generated recipes to testdata/spike-llm/
- [ ] Document results: success rate, cost per recipe, failure modes
- [ ] Decide: continue to Slice 2 or pivot approach
