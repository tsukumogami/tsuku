# Design Document: LLM Slice 1 - End-to-End Spike

**Status**: Current

**Parent Design**: [DESIGN-llm-builder-infrastructure.md](DESIGN-llm-builder-infrastructure.md)

**Issue**: [#266 - Slice 1: End-to-End Spike](https://github.com/tsukumogami/tsuku/issues/266)

<a id="implementation-issues"></a>
**Implementation Issues**:

| Issue | Title | Design Section | Dependencies |
|-------|-------|----------------|--------------|
| [#277](https://github.com/tsukumogami/tsuku/issues/277) | Extend Builder interface with BuildRequest struct | [Builder Interface Extension](#1-builder-interface-extension) | None |
| [#278](https://github.com/tsukumogami/tsuku/issues/278) | Implement LLM client package with multi-turn tool use | [LLM Client](#3-llm-client-internalllmclientgo) | None |
| [#279](https://github.com/tsukumogami/tsuku/issues/279) | Implement fetch_file tool handler for LLM | [Tool Definitions](#4-tool-definitions) | #278 |
| [#280](https://github.com/tsukumogami/tsuku/issues/280) | Implement inspect_archive tool handler for LLM | [Tool Definitions](#4-tool-definitions) | #278 |
| [#281](https://github.com/tsukumogami/tsuku/issues/281) | Implement GitHub Release Builder | [GitHubReleaseBuilder](#2-githubreleasebuilder-internalbuildsgithub_releasego) | #277, #278 |
| [#282](https://github.com/tsukumogami/tsuku/issues/282) | Add --from flag to tsuku create command | [CLI Syntax](#cli-syntax) | #277, #281 |
| [#283](https://github.com/tsukumogami/tsuku/issues/283) | Ground truth validation tests for LLM recipe generation | [Testing Approach](#testing-approach) | #281, #282 |

```
Dependency Graph:

#277 (Builder interface) ───┐
                            ├──> #281 (GitHub Release Builder) ──> #282 (CLI --from) ──> #283 (Validation)
#278 (LLM client) ──────────┤         ↑ (optional)
        │                   │         │
        ├──> #279 (fetch_file) ───────┘
        │                             │
        └──> #280 (inspect_archive) ──┘
```

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

- [ ] GitHub Release Builder registered and callable via `tsuku create <tool> --from github:owner/repo`
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
2. **CLI**: Extend `tsuku create` with `--from github:owner/repo` syntax
3. **Builder Interface**: Extend with `BuildRequest` struct for builder-specific arguments
4. **Recipe Generation**: Use existing `recipe.Recipe` structures and TOML marshaling
5. **HTTP Client**: Use existing `version.NewHTTPClient()` patterns for GitHub API

### CLI Syntax

The `--from` flag uses a colon-separated format: `builder:source-arg`

```bash
tsuku create age --from github:FiloSottile/age
```

- `age` - The tool name (goes in recipe metadata, used for `tsuku install age`)
- `github` - The builder to use
- `FiloSottile/age` - Builder-specific source argument (the GitHub repo)

This separation is necessary because:
1. Tool name and repo name often differ (e.g., `gh` from `cli/cli`)
2. Repos can be monorepos containing multiple tools
3. Each builder needs different source information

### Why This Structure

- **Follows existing patterns**: Other builders live in `internal/builders/`
- **Leverages infrastructure**: Reuse HTTP clients, recipe validation, TOML serialization
- **Extensible**: Slice 3 will add Gemini provider to `internal/llm/`
- **No throwaway code**: Everything written here evolves through later slices

## Solution Architecture

### Overview

```
tsuku create gh --from github:cli/cli

cmd/tsuku/create.go
├── Parse --from github:cli/cli
├── Get GitHubReleaseBuilder from registry
└── builder.Build(ctx, BuildRequest{Package: "gh", SourceArg: "cli/cli"})

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
    AssetType   string            `json:"asset_type"`   // "archive" or "binary"
    Pattern     string            `json:"pattern"`      // e.g., "gh_{version}_{os}_{arch}.tar.gz"
    OSMapping   map[string]string `json:"os_mapping"`   // e.g., {"linux": "linux", "darwin": "darwin"}
    ArchMapping map[string]string `json:"arch_mapping"` // e.g., {"amd64": "amd64", "arm64": "arm64"}
    Format      string            `json:"format"`       // e.g., "tar.gz" (only for archives)
    Binaries    []string          `json:"binaries"`     // e.g., ["gh"] or ["age", "age-keygen"]
    Verify      string            `json:"verify"`       // e.g., "gh --version"
}
```

### Components

#### 1. Builder Interface Extension

The existing Builder interface needs extension to support builder-specific arguments:

```go
// BuildRequest contains all parameters for recipe generation
type BuildRequest struct {
    Package   string // Tool name for recipe metadata (e.g., "gh")
    Version   string // Optional version constraint
    SourceArg string // Builder-specific source (e.g., "cli/cli" for github)
}

// Builder interface (extended)
type Builder interface {
    Name() string
    CanBuild(ctx context.Context, req BuildRequest) (bool, error)
    Build(ctx context.Context, req BuildRequest) (*BuildResult, error)
}
```

#### 2. GitHubReleaseBuilder (`internal/builders/github_release.go`)

Implements the Builder interface:

```go
type GitHubReleaseBuilder struct {
    httpClient *http.Client
    llmClient  *llm.Client
}

func (b *GitHubReleaseBuilder) Name() string { return "github" }

func (b *GitHubReleaseBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
    // SourceArg must be owner/repo format
    if req.SourceArg == "" {
        return false, fmt.Errorf("github builder requires repo: --from github:owner/repo")
    }
    return strings.Contains(req.SourceArg, "/"), nil
}

func (b *GitHubReleaseBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error)
```

The `Build` method:
1. Uses `req.SourceArg` as `owner/repo`
2. Uses `req.Package` as the tool name for recipe metadata
3. Fetches release data via GitHub API (reusing existing HTTP client patterns)
4. Fetches repo metadata (description, homepage)
5. Calls LLM client with assets and version tag
6. Constructs `recipe.Recipe` from LLM response
7. Returns `BuildResult` with recipe and warnings

#### 3. LLM Client (`internal/llm/client.go`)

Wraps Anthropic Go SDK with multi-turn tool use:

```go
type Client struct {
    anthropic  *anthropic.Client
    model      string
    httpClient *http.Client  // For fetch_file
}

// GenerateRecipe runs multi-turn conversation until extract_pattern is called
func (c *Client) GenerateRecipe(ctx context.Context, req *GenerateRequest) (*AssetPattern, *Usage, error)

type GenerateRequest struct {
    Repo        string    // "FiloSottile/age"
    Releases    []Release // Last 3-5 releases for pattern inference
    Description string    // Repo description
    README      string    // README.md content (fetched proactively)
}

type Release struct {
    Tag    string   // "v1.2.1"
    Assets []string // Asset filenames for this release
}
```

**Multi-turn flow:**
1. Send initial prompt with: last 3-5 releases (tag + assets each), repo metadata, README.md
2. Loop: handle tool calls (`fetch_file`, `inspect_archive`)
3. Exit when `extract_pattern` is called

**Why multiple releases:**
- LLM sees how version appears across releases: `v1.2.0` → `age-v1.2.0-...`, `v1.2.1` → `age-v1.2.1-...`
- Validates pattern consistency (detect if a release is anomalous)
- Makes `{version}` placeholder extraction obvious

**Tool handlers:**
- `fetch_file`: GET `https://raw.githubusercontent.com/{repo}/{tag}/{path}` - for any file (INSTALL.md, docs/, etc.)
- `inspect_archive`: Download asset, extract in container, return file list with executable detection
- `extract_pattern`: Parse response, return `AssetPattern`

- Model: `claude-sonnet-4-5-20250929` (hardcoded for Slice 1)
- Accumulates usage across all turns
- Max turns: 5 (prevent infinite loops)

#### 4. Tool Definitions

The LLM has access to multiple tools for gathering information before producing the final pattern. This enables multi-turn reasoning when the LLM needs more context.

**Available tools:**

| Tool | Purpose | Required |
|------|---------|----------|
| `fetch_file` | Fetch README.md or other repo files | Optional |
| `inspect_archive` | Download & list archive contents in container | Optional |
| `extract_pattern` | Final output - the asset pattern | Required (forced via `tool_choice`) |

**Why multi-turn:**
- README often documents binary names explicitly
- Archive inspection reveals actual binaries when names don't match tool name
- LLM can ask for what it needs rather than guessing

**Typical flows:**

Simple case (e.g., age):
```
LLM sees:
  - v1.2.0: age-v1.2.0-linux-amd64.tar.gz, age-v1.2.0-darwin-arm64.tar.gz, ...
  - v1.2.1: age-v1.2.1-linux-amd64.tar.gz, age-v1.2.1-darwin-arm64.tar.gz, ...
  - README.md
LLM infers: pattern is "age-v{version}-{os}-{arch}.tar.gz"
LLM calls: extract_pattern with binaries=["age"]
```

Binary name in README (e.g., ripgrep → rg):
```
LLM sees:
  - v14.1.0: ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz, ...
  - v14.0.3: ripgrep-14.0.3-x86_64-unknown-linux-musl.tar.gz, ...
  - README says: "Install the rg binary to your PATH"
LLM calls: extract_pattern with binaries=["rg"]
```

Multi-binary discovery:
```
LLM sees: assets, README mentions multiple commands
LLM calls: inspect_archive("age-v1.2.1-linux-amd64.tar.gz")
Container returns: ["age/age", "age/age-keygen"] (both executable)
LLM calls: extract_pattern with binaries=["age", "age-keygen"]
```

Need more docs:
```
LLM sees: README unclear about installation
LLM calls: fetch_file("INSTALL.md")
INSTALL.md explains binary structure
LLM calls: extract_pattern
```

**Tool: fetch_file** (optional, for additional docs)
```json
{
  "name": "fetch_file",
  "description": "Fetch a file from the repository. README.md is already provided - use this for other files like INSTALL.md, docs/usage.md, etc.",
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "File path in repo (e.g., 'INSTALL.md', 'docs/install.md', 'Makefile')"
      }
    },
    "required": ["path"]
  }
}
```

**Tool: inspect_archive** (optional, for binary discovery)
```json
{
  "name": "inspect_archive",
  "description": "Download and list contents of a release archive to discover binaries. Runs in isolated container.",
  "input_schema": {
    "type": "object",
    "properties": {
      "asset_name": {
        "type": "string",
        "description": "Exact asset filename to inspect (e.g., 'age-v1.2.1-linux-amd64.tar.gz')"
      }
    },
    "required": ["asset_name"]
  }
}
```

Returns file listing with types:
```json
{
  "files": [
    {"path": "age/age", "type": "executable", "size": 4521984},
    {"path": "age/age-keygen", "type": "executable", "size": 3201024},
    {"path": "age/LICENSE", "type": "file", "size": 1523}
  ]
}
```

**Tool: extract_pattern** (required, final output)
```json
{
  "name": "extract_pattern",
  "description": "Final output: the asset naming pattern for a tsuku recipe",
  "input_schema": {
    "type": "object",
    "properties": {
      "asset_type": {
        "type": "string",
        "enum": ["archive", "binary"],
        "description": "Whether assets are archives (tar.gz, zip) or standalone binaries"
      },
      "asset_pattern": {
        "type": "string",
        "description": "Pattern with placeholders. Use {version}, {os}, {arch} as needed."
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
        "description": "Archive format (only if asset_type is 'archive')"
      },
      "binaries": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Binary paths to install (e.g., ['age', 'age-keygen'] or ['bin/node', 'bin/npm'])"
      },
      "verify_command": {
        "type": "string",
        "description": "Command to verify installation (e.g., 'gh --version')"
      }
    },
    "required": ["asset_type", "asset_pattern", "os_mapping", "arch_mapping", "binaries", "verify_command"]
  }
}
```

Example response for cli/cli (archive, single binary):
```json
{
  "asset_type": "archive",
  "asset_pattern": "gh_{version}_{os}_{arch}.tar.gz",
  "os_mapping": { "linux": "linux", "darwin": "macOS" },
  "arch_mapping": { "amd64": "amd64", "arm64": "arm64" },
  "archive_format": "tar.gz",
  "binaries": ["gh"],
  "verify_command": "gh --version"
}
```

Example response for k3d-io/k3d (standalone binary):
```json
{
  "asset_type": "binary",
  "asset_pattern": "k3d-{os}-{arch}",
  "os_mapping": { "linux": "linux", "darwin": "darwin" },
  "arch_mapping": { "amd64": "amd64", "arm64": "arm64" },
  "binaries": ["k3d"],
  "verify_command": "k3d version"
}
```

Example response for FiloSottile/age (archive, multiple binaries after inspect_archive):
```json
{
  "asset_type": "archive",
  "asset_pattern": "age-v{version}-{os}-{arch}.tar.gz",
  "os_mapping": { "linux": "linux", "darwin": "darwin" },
  "arch_mapping": { "amd64": "amd64", "arm64": "arm64" },
  "archive_format": "tar.gz",
  "binaries": ["age", "age-keygen"],
  "verify_command": "age --version"
}
```

#### 5. Recipe Generation

The builder constructs a `recipe.Recipe` directly (using existing types from `internal/recipe/types.go`):

```go
func (b *GitHubReleaseBuilder) buildRecipe(req BuildRequest, repo string, meta *RepoMeta, pattern *AssetPattern) *recipe.Recipe {
    r := &recipe.Recipe{
        Metadata: recipe.MetadataSection{
            Name:        req.Package,  // Use requested package name, not executable
            Description: meta.Description,
            Homepage:    meta.Homepage,
        },
        Version: recipe.VersionSection{
            Source:     "github_releases",
            GitHubRepo: repo,
        },
        Verify: recipe.VerifySection{
            Command: pattern.Verify,
        },
    }

    if pattern.AssetType == "archive" {
        r.Steps = []recipe.Step{{
            Action: "github_archive",
            Params: map[string]interface{}{
                "repo":           repo,
                "asset_pattern":  pattern.Pattern,
                "archive_format": pattern.Format,
                "strip_dirs":     1,
                "binaries":       pattern.Binaries,
                "os_mapping":     pattern.OSMapping,
                "arch_mapping":   pattern.ArchMapping,
            },
        }}
    } else {
        // github_file for standalone binaries (single binary only)
        r.Steps = []recipe.Step{{
            Action: "github_file",
            Params: map[string]interface{}{
                "repo":          repo,
                "asset_pattern": pattern.Pattern,
                "binary":        pattern.Binaries[0],
                "os_mapping":    pattern.OSMapping,
                "arch_mapping":  pattern.ArchMapping,
            },
        }}
    }

    return r
}
```

#### 6. Cost Tracking (`internal/llm/cost.go`)

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

You will receive:
- Multiple releases with their tags and asset filenames (use these to identify how version appears in patterns)
- Repository metadata (description, homepage)
- README.md content

Your task:
1. Identify the asset pattern with {version}, {os}, {arch} placeholders
2. Map standard OS names (linux, darwin) to strings used in filenames
3. Map standard arch names (amd64, arm64) to strings used in filenames
4. Determine if assets are archives or standalone binaries
5. Identify the executable binary name(s) - check README or use inspect_archive if unclear
6. Provide a verify command

Use the multiple releases to see how version numbers appear in filenames. For example:
- v1.2.0 → age-v1.2.0-linux-amd64.tar.gz
- v1.2.1 → age-v1.2.1-linux-amd64.tar.gz
- Pattern: age-v{version}-{os}-{arch}.tar.gz

Common naming conventions:
- Rust targets: x86_64-unknown-linux-musl, aarch64-apple-darwin
- Go conventions: linux_amd64, darwin_arm64, macOS_arm64
- Generic: linux-x64, macos-arm64

Preferences:
- musl over glibc for Linux (more portable)
- tar.gz or tar.xz over zip for Unix platforms

Tools available:
- fetch_file: Get additional files from repo (INSTALL.md, docs/, etc.)
- inspect_archive: Download and list archive contents to discover binaries
- extract_pattern: Final output (required)
```

### Data Flow

```
1. User runs: tsuku create gh --from github:cli/cli

   CLI parses:
   - Package: "gh"
   - Builder: "github"
   - SourceArg: "cli/cli"

   Calls: builder.Build(ctx, BuildRequest{Package: "gh", SourceArg: "cli/cli"})

2. Fetch context (parallel):
   GET https://api.github.com/repos/cli/cli/releases?per_page=5
   → Last 5 releases with tags and asset names:
     - v2.42.0: ["gh_2.42.0_linux_amd64.tar.gz", "gh_2.42.0_darwin_arm64.tar.gz", ...]
     - v2.41.0: ["gh_2.41.0_linux_amd64.tar.gz", "gh_2.41.0_darwin_arm64.tar.gz", ...]
     - v2.40.1: ["gh_2.40.1_linux_amd64.tar.gz", ...]
     - ...

   GET https://api.github.com/repos/cli/cli
   → description: "GitHub CLI", homepage: "https://cli.github.com"

   GET https://raw.githubusercontent.com/cli/cli/v2.42.0/README.md
   → README content

3. Call Claude (multi-turn):
   POST https://api.anthropic.com/v1/messages
   - System prompt (pattern extraction instructions)
   - User message with releases, repo metadata, README
   - Tools: [fetch_file, inspect_archive, extract_pattern]

   (LLM may call fetch_file or inspect_archive, loop until extract_pattern)

4. Parse extract_pattern response:
   → tool_use block:
     asset_type: "archive"
     asset_pattern: "gh_{version}_{os}_{arch}.tar.gz"
     os_mapping: {"linux": "linux", "darwin": "macOS"}
     arch_mapping: {"amd64": "amd64", "arm64": "arm64"}
     binaries: ["gh"]
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

We have ground truth: existing recipes in `internal/recipe/recipes/`. Tests validate that the builder produces equivalent recipes.

**Test matrix** covering `github_archive`, `github_file`, and versionless patterns:

| Repository | Action | Convention | Existing Recipe |
|------------|--------|------------|-----------------|
| stern/stern | archive | Go standard | `s/stern.toml` |
| ast-grep/ast-grep | archive | Rust target triples | `a/ast-grep.toml` |
| aquasecurity/trivy | archive | Non-standard mappings | `t/trivy.toml` |
| FiloSottile/age | archive | Multiple binaries | `a/age.toml` |
| k3d-io/k3d | binary | Standalone binary | `k/k3d.toml` |
| kubernetes-sigs/kind | binary | Standalone binary | `k/kind.toml` |
| derailed/k9s | archive | **No {version}** in pattern | `k/k9s.toml` |
| terraform-linters/tflint | archive | **No {version}**, zip | `t/tflint.toml` |
| runatlantis/atlantis | archive | **No {version}**, zip | `a/atlantis.toml` |

**Validation approach:**
1. Run: `tsuku create <tool> --from github:owner/repo`
2. Compare generated recipe against existing recipe in `internal/recipe/recipes/`
3. Key fields must match: `action`, `asset_pattern`, `os_mapping`, `arch_mapping`
4. Install: `tsuku install <tool>` (validates the recipe actually works)

**Example commands:**
```bash
# Archives (with version in pattern)
tsuku create stern --from github:stern/stern
tsuku create ast-grep --from github:ast-grep/ast-grep
tsuku create age --from github:FiloSottile/age

# Archives (versionless patterns)
tsuku create k9s --from github:derailed/k9s
tsuku create tflint --from github:terraform-linters/tflint
tsuku create atlantis --from github:runatlantis/atlantis

# Binaries (standalone executables)
tsuku create k3d --from github:k3d-io/k3d
tsuku create kind --from github:kubernetes-sigs/kind
```

**Exit criteria:**
- At least 7/9 repos produce recipes matching existing ground truth
- All three categories work: archive+version, archive+versionless, binary
- Generated recipes install successfully

**Test artifacts:**
- Generated recipes saved to `testdata/llm/` for regression testing
- Comparison diffs saved when generated != expected
- Issues discovered recorded for Slice 2-4 designs

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

Each section maps to implementation issues. See the [Implementation Issues](#implementation-issues) table above for dependencies.

### #277: Builder Interface Extension

- [ ] Define `BuildRequest` struct with `Package`, `Version`, `SourceArg` fields
- [ ] Update `Builder` interface to accept `BuildRequest`
- [ ] Define `BuildResult` struct with recipe and warnings

### #278: LLM Client Package

- [ ] Create `internal/llm/client.go` - Multi-turn Claude client with tool handling
- [ ] Create `internal/llm/cost.go` - Usage tracking and cost calculation
- [ ] Create `internal/llm/tools.go` - Tool schemas (`fetch_file`, `inspect_archive`, `extract_pattern`)
- [ ] Implement multi-turn conversation loop (max 5 turns)
- [ ] Accumulate token usage across turns

### #279: fetch_file Tool Handler

- [ ] Implement handler: `GET https://raw.githubusercontent.com/{repo}/{tag}/{path}`
- [ ] Return file content or helpful error message for 404s
- [ ] 60 second timeout, reject binary content types

### #280: inspect_archive Tool Handler

- [ ] Download archive to temp directory
- [ ] Extract based on format (tar.gz, tar.xz, zip)
- [ ] Return file listing with executable detection
- [ ] Clean up temp files after inspection

### #281: GitHub Release Builder

- [ ] Create `internal/builders/github_release.go`
- [ ] Fetch last 3-5 releases from GitHub API
- [ ] Fetch repo metadata (description, homepage)
- [ ] Fetch README.md proactively
- [ ] Call LLM client and handle response
- [ ] Generate `recipe.Recipe` from `AssetPattern`
- [ ] Register in builder registry

### #282: CLI --from Flag

- [ ] Add `--from` flag to `tsuku create` command
- [ ] Parse `builder:sourceArg` format
- [ ] Look up builder by name from registry
- [ ] Print recipe TOML to stdout
- [ ] Print cost/warnings to stderr

### #283: Ground Truth Validation Tests

**Test: github_archive (with version):**
- [ ] `tsuku create stern --from github:stern/stern` → compare to `s/stern.toml`
- [ ] `tsuku create ast-grep --from github:ast-grep/ast-grep` → compare to `a/ast-grep.toml`
- [ ] `tsuku create trivy --from github:aquasecurity/trivy` → compare to `t/trivy.toml`
- [ ] `tsuku create age --from github:FiloSottile/age` → compare to `a/age.toml`

**Test: github_archive (versionless patterns):**
- [ ] `tsuku create k9s --from github:derailed/k9s` → compare to `k/k9s.toml`
- [ ] `tsuku create tflint --from github:terraform-linters/tflint` → compare to `t/tflint.toml`
- [ ] `tsuku create atlantis --from github:runatlantis/atlantis` → compare to `a/atlantis.toml`

**Test: github_file (standalone binaries):**
- [ ] `tsuku create k3d --from github:k3d-io/k3d` → compare to `k/k3d.toml`
- [ ] `tsuku create kind --from github:kubernetes-sigs/kind` → compare to `k/kind.toml`

**Validation:**
- [ ] At least 7/9 repos produce recipes matching ground truth
- [ ] All three categories work: archive+version, archive+versionless, binary
- [ ] Generated recipes install successfully
- [ ] Save generated recipes to `testdata/llm/` for regression
- [ ] Document issues discovered for Slice 2-4 designs
