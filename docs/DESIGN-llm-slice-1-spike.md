# Design Document: LLM Slice 1 - End-to-End Spike

**Status**: Proposed

**Parent Design**: [DESIGN-llm-builder-infrastructure.md](DESIGN-llm-builder-infrastructure.md)

**Issue**: [#266 - Slice 1: End-to-End Spike](https://github.com/tsukumogami/tsuku/issues/266)

## Context and Problem Statement

The LLM Builder Infrastructure milestone (M12) will enable tsuku to generate recipes from GitHub release data using LLMs. Before building supporting infrastructure (container validation, provider abstraction, repair loops), we implement the critical path end-to-end to discover real integration issues early.

### Purpose of This Slice

This is not an experiment to see if LLM recipe generation is possible - we know it works. The purpose is to:

1. **Discover integration issues** by building the real feature path
2. **Inform the design** of later slices with concrete experience
3. **Establish patterns** that will be extended, not thrown away

The code written in this slice is production code. It will evolve through later slices, not be rewritten.

### Why Critical Path First

The parent design outlines four vertical slices. Slice 1 implements the critical path to surface issues that would otherwise be discovered late:
- How does the LLM output integrate with existing recipe structures?
- What edge cases exist in GitHub release naming conventions?
- What prompt patterns produce reliable results?
- Where does the builder interface need extension?

Issues discovered here inform the designs for Slices 2-4.

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

### Exit Criteria

- [ ] GitHub Release Builder registered and callable via `tsuku create <tool> --from github`
- [ ] Generated recipes are syntactically valid and follow existing recipe patterns
- [ ] At least 3 test repos produce working installations
- [ ] Cost per generation tracked and displayed
- [ ] Issues discovered are documented for Slice 2-4 designs

### What This Slice Will Discover

Building the critical path will reveal:
1. **Integration patterns**: How LLM output maps to existing `recipe.Recipe` structures
2. **Edge cases**: GitHub release naming variations that need special handling
3. **Prompt engineering**: What instructions produce consistent, correct output
4. **Builder interface gaps**: Where the existing Builder interface needs extension for LLM builders
5. **Cost characteristics**: Actual token consumption for real-world repos

## Assumptions

- Claude API key available with sufficient quota for development and testing
- GitHub release asset naming contains sufficient signal for pattern extraction
- Test repos (cli/cli, BurntSushi/ripgrep, sharkdp/fd, hashicorp/terraform, jqlang/jq) cover common naming conventions
- Tool use pattern produces more reliable structured output than text parsing
- Manual validation acceptable for Slice 1; container validation added in Slice 2

## Decision Drivers

- **Follow existing patterns**: Use existing Builder interface, HTTP clients, recipe structures
- **Discover issues early**: Build the critical path first to surface problems before infrastructure
- **Production code from start**: Code written here will evolve, not be thrown away
- **Defer complexity**: Single provider, no repair loop, manual validation - add in later slices

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

## Implementation Approach

This is a tsuku feature. The code lives in `internal/` and integrates with the existing builder infrastructure.

### New Files

| File | Purpose |
|------|---------|
| `internal/llm/client.go` | Claude API client using Anthropic Go SDK |
| `internal/llm/cost.go` | Token usage and cost tracking |
| `internal/builders/github_release.go` | GitHub Release Builder implementing Builder interface |

### Integration Points

1. **Builder Registry**: Register `GitHubReleaseBuilder` alongside existing builders (Cargo, npm, PyPI, etc.)
2. **CLI**: Extend `tsuku create` with `--from github` flag
3. **Recipe Generation**: Use existing `recipe.Recipe` structures and TOML marshaling
4. **HTTP Client**: Use existing `version.NewHTTPClient()` patterns for GitHub API

### Why This Structure

- **Follows existing patterns**: Other builders live in `internal/builders/`
- **Leverages infrastructure**: Reuse HTTP clients, recipe validation, TOML serialization
- **Extensible**: Slice 3 will add Gemini provider to `internal/llm/`
- **No throwaway code**: Everything written here evolves through later slices

## Solution Architecture

### Overview

```
tsuku create gh --from github

cmd/tsuku/create.go
├── Parse --from github flag
├── Get GitHubReleaseBuilder from registry
└── builder.Build(ctx, "cli/cli", "")

internal/builders/github_release.go
├── Fetch release metadata from GitHub API
├── Call Claude via internal/llm/client.go
├── Parse tool response into asset pattern
├── Generate recipe.Recipe struct
└── Return BuildResult with recipe and cost info

internal/llm/client.go
├── Anthropic SDK wrapper
├── Tool use with extract_pattern
└── Usage tracking for cost calculation
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

#### 1. GitHubReleaseBuilder (`internal/builders/github_release.go`)

Implements the existing `Builder` interface:

```go
type GitHubReleaseBuilder struct {
    httpClient *http.Client
    llmClient  *llm.Client
}

func (b *GitHubReleaseBuilder) Name() string { return "github" }

func (b *GitHubReleaseBuilder) CanBuild(ctx context.Context, pkg string) (bool, error)

func (b *GitHubReleaseBuilder) Build(ctx context.Context, pkg, version string) (*BuildResult, error)
```

The `Build` method:
1. Parses `pkg` as `owner/repo` or GitHub URL
2. Fetches release data via GitHub API (reusing existing HTTP client patterns)
3. Fetches repo metadata (description, homepage)
4. Calls LLM client with assets and version tag
5. Constructs `recipe.Recipe` from LLM response
6. Returns `BuildResult` with recipe and warnings

#### 2. LLM Client (`internal/llm/client.go`)

Wraps Anthropic Go SDK with tsuku-specific patterns:

```go
type Client struct {
    anthropic *anthropic.Client
    model     string
}

func (c *Client) ExtractPattern(ctx context.Context, tag string, assets []string) (*AssetPattern, *Usage, error)
```

- Model: `claude-sonnet-4-5-20250929` (hardcoded for Slice 1, configurable in Slice 3)
- Uses tool use with `extract_pattern` tool
- Forces tool use via `tool_choice`
- Returns usage for cost tracking

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

#### 4. Recipe Generation

The builder constructs a `recipe.Recipe` directly (using existing types from `internal/recipe/types.go`):

```go
func (b *GitHubReleaseBuilder) buildRecipe(repo string, meta *RepoMeta, pattern *AssetPattern) *recipe.Recipe {
    return &recipe.Recipe{
        Metadata: recipe.MetadataSection{
            Name:        pattern.Executable,
            Description: meta.Description,
            Homepage:    meta.Homepage,
        },
        Version: recipe.VersionSection{
            Source:     "github_releases",
            GitHubRepo: repo,
        },
        Steps: []recipe.Step{{
            Action: "github_archive",
            Params: map[string]interface{}{
                "repo":           repo,
                "asset_pattern":  pattern.Pattern,
                "archive_format": pattern.Format,
                "strip_dirs":     1,
                "binaries":       []string{pattern.Executable},
                "os_mapping":     pattern.OSMapping,
                "arch_mapping":   pattern.ArchMapping,
            },
        }},
        Verify: recipe.VerifySection{
            Command: pattern.Verify,
        },
    }
}
```

#### 5. Cost Tracking (`internal/llm/cost.go`)

Track and display token usage:

```go
type Usage struct {
    InputTokens  int
    OutputTokens int
}

func (u Usage) Cost() float64 // Returns cost in USD
func (u Usage) String() string // Returns human-readable summary
```

Claude Sonnet pricing (as of 2025):
- Input: $3 per 1M tokens
- Output: $15 per 1M tokens

Cost is included in `BuildResult.Warnings` for display to user.

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

Test with 5 repositories covering different GitHub release naming conventions:

| Repository | Convention | Goal |
|------------|------------|------|
| cli/cli | Go (linux_amd64) | Baseline - common Go pattern |
| BurntSushi/ripgrep | Rust (x86_64-unknown-linux-musl) | Rust target triples |
| sharkdp/fd | Rust | Confirm Rust pattern consistency |
| hashicorp/terraform | Go (many platforms) | Many platform variants |
| jqlang/jq | Generic (linux-amd64) | Generic naming conventions |

**Validation steps:**
1. Run: `tsuku create <tool> --from github`
2. Inspect generated recipe for correctness
3. Install: `tsuku install <tool>`
4. Verify: Run the tool's verify command

**Exit criteria:**
- At least 3/5 repos produce working installations
- Generated recipes follow existing recipe patterns
- Issues discovered are documented

**Artifacts:**
- Generated recipes saved to `testdata/llm/` for regression testing
- Issues discovered recorded in design doc updates or new issues

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

1. **Early issue discovery**: Building the critical path surfaces integration issues before infrastructure investment
2. **Real cost data**: Actual token consumption measured against estimates
3. **Production-ready patterns**: Code structure and prompts evolve, not rewrite
4. **Informed later designs**: Slice 2-4 designs benefit from concrete experience

### Negative

1. **No automated validation**: Manual testing until Slice 2 adds container validation
2. **Single provider**: Claude only until Slice 3 adds Gemini
3. **Limited error handling**: Basic errors until Slice 4 adds production UX

### What We'll Document

After this slice:
1. **Issues discovered**: Any gaps in Builder interface, recipe structures, or LLM behavior
2. **Prompt evolution**: What changes were needed to get consistent output
3. **Cost actuals**: Real token consumption for test repos
4. **Edge cases**: GitHub release naming patterns that need special handling

## Implementation Checklist

- [ ] Create `internal/llm/client.go` - Claude API client with tool use
- [ ] Create `internal/llm/cost.go` - Usage tracking and cost calculation
- [ ] Create `internal/builders/github_release.go` - Builder implementation
- [ ] Register builder in `cmd/tsuku/create.go`
- [ ] Add `--from github` flag to `tsuku create`
- [ ] Write `extract_pattern` tool schema and system prompt
- [ ] Test with cli/cli (baseline, Go naming)
- [ ] Test with BurntSushi/ripgrep (Rust naming)
- [ ] Test with sharkdp/fd (confirm Rust pattern)
- [ ] Test with hashicorp/terraform (many variants)
- [ ] Test with jqlang/jq (generic naming)
- [ ] Save generated recipes to testdata for regression testing
- [ ] Document issues discovered for Slice 2-4 designs
