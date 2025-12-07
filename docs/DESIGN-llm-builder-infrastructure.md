# Design Document: LLM Builder Infrastructure

**Status**: Proposed

## Context and Problem Statement

Tsuku currently has ecosystem-specific builders (Cargo, Gem, npm, PyPI, Go) that generate recipes by parsing package registry APIs. These deterministic builders work well for packages with standardized metadata, but fail to cover tools distributed through:

- **GitHub Releases**: Binaries published as release assets with non-standard naming conventions
- **Documentation-only sources**: Tools described in READMEs without structured metadata
- **Complex ecosystems**: Homebrew formulas requiring Ruby DSL interpretation
- **Non-standard registries**: Aqua, system packages (apt/yum), and proprietary sources

The common thread is that these sources require interpretation rather than mechanical parsing. An LLM can examine release asset names, documentation, or formula definitions and infer the correct recipe structure.

### The Opportunity

Modern LLMs with tool use capabilities can:

1. **Analyze release assets**: Given GitHub Release JSON, match asset filenames to platform/architecture combinations (e.g., `ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz` maps to `linux/amd64`)

2. **Extract from documentation**: Parse README installation instructions to identify download URLs, extraction steps, and binary locations

3. **Interpret formula DSLs**: Understand Homebrew Ruby formulas to extract version sources, dependencies, and installation steps

4. **Handle ambiguity**: Make reasonable decisions when metadata is incomplete, with confidence signals for validation

### Why Now

1. **Infrastructure exists**: The existing Builder interface, actions, and version providers provide the foundation
2. **LLM APIs are mature**: Claude and Gemini both support tool use with structured outputs
3. **Validation is possible**: Container-based validation can verify generated recipes before user installation
4. **Cost is manageable**: Per-recipe generation costs ~$0.05-0.10, acceptable for occasional use

### Scope

**In scope (this design):**
- LLM client abstraction supporting Claude (tool use) and Gemini (function calling) - both required for MVP
- Infrastructure components: web fetcher with caching, secrets management, cost tracking
- Container-based recipe validation with network isolation (Docker and Podman support)
- Repair loop for iterative recipe improvement on validation failure
- GitHub Release Builder as the first LLM builder implementation
- CLI integration via `tsuku create <tool> --from github`
- Recipe preview requirement before installation of LLM-generated recipes

**Out of scope (future milestones):**
- Aqua Registry Builder
- System Package Builder (apt/yum)
- Homebrew Builder
- Documentation Builder
- Automatic registry contribution workflow
- LLM-based orchestration for builder selection

### Success Criteria

- **Recipe success rate**: 80% of generated recipes produce working installations
- **Latency**: Under 60 seconds per generation (including validation)
- **Cost**: Under $0.10 per recipe average
- **Repair effectiveness**: Repair loops improve success by 20%+ over single-shot
- **Extensibility**: Adding a new LLM builder requires only implementing the builder logic, not infrastructure

### Non-Goals and Acceptable Failures

**Non-goals for this milestone:**
- Multi-platform validation (validating on both x86_64 and ARM)
- Recipe caching across users or machines
- Automatic registry contribution
- 100% success rate (80% is the target)

**Acceptable failure modes:**
- Generation fails with clear error message (user can retry or write recipe manually)
- Recipe passes validation but fails on user's different architecture (documented limitation)
- LLM produces slightly different recipes on repeated runs (non-determinism is expected)

**Unacceptable failures:**
- Silent corruption of user's system
- Security vulnerabilities in generated recipes passing validation
- Unbounded cost accumulation without user visibility

### Expected Usage Scale

This feature targets occasional, on-demand recipe generation:
- Estimated usage: 10-100 recipes/month per active user
- Concurrent users: Low (single-user CLI tool)
- No multi-tenant concerns; each user has their own API keys and local recipes

## Decision Drivers

- **Leverage existing infrastructure**: LLM builders must use the same Builder interface, actions, and version providers as deterministic builders
- **Multi-provider support**: Architecture must support multiple LLM providers (Claude, Gemini) from the start, not as an afterthought
- **Validation before execution**: Generated recipes must be validated in containers before user installation
- **Cost visibility**: LLM usage costs must be tracked and visible to users
- **Security-first**: Generated recipes must pass the same validation as registry recipes; no execution of arbitrary LLM output
- **Graceful degradation**: When LLM generation fails, provide actionable error messages

## External Research

### LLM Code Sandboxing Approaches

#### Container-Based Isolation

Container isolation is the industry standard for running untrusted LLM-generated code. Key patterns from [Code Sandboxes for LLMs and AI Agents](https://amirmalik.net/2025/03/07/code-sandboxes-for-llm-ai-agents):

- **Namespace isolation**: Separate filesystem, network stack, and process space
- **Enhanced runtimes**: gVisor or Kata Containers for additional kernel isolation
- **Resource controls**: CPU, memory, and execution time limits

**Relevance to tsuku**: Container validation can run generated recipes in isolation to verify they work before user installation. Network isolation (`--network=none`) ensures validation can't access the internet, preventing exfiltration or supply chain attacks during testing.

#### LLM Sandbox Libraries

[llm-sandbox](https://github.com/vndee/llm-sandbox) provides a lightweight Python library for running LLM-generated code in containers. Key features:

- Multiple backend support (Docker, Kubernetes, local)
- Session pooling for performance
- Comprehensive language support

**Relevance to tsuku**: The session pooling pattern is interesting but likely overkill for tsuku's use case where recipe validation is infrequent.

### LLM Tool Use Patterns

#### Claude Tool Use

From [Anthropic's Advanced Tool Use](https://www.anthropic.com/engineering/advanced-tool-use):

- **Programmatic Tool Calling**: Claude can orchestrate tools through code rather than individual API round-trips, enabling loops, conditionals, and error handling in code
- **Tool Search Tool**: Allows Claude to access thousands of tools without consuming context window
- **Tool Use Examples**: Provides examples demonstrating effective tool usage

**Relevance to tsuku**: For the GitHub Release Builder, we need Claude to analyze release JSON and produce structured output (platform/arch mappings). This is a single-shot tool call, not orchestration, so basic tool use is sufficient.

#### Gemini Function Calling

From [Google's Function Calling documentation](https://ai.google.dev/gemini-api/docs/function-calling):

- **Mode control**: AUTO (model decides), ANY (force function call), NONE (no function calls)
- **Streaming arguments**: For Gemini 3 Pro+, function arguments can be streamed as generated
- **Built-in MCP support**: Automatic tool calling for MCP tools

**Relevance to tsuku**: Gemini's function calling maps well to Claude's tool use. The same prompts and tool definitions can work with both providers.

### GitHub Release Asset Patterns

Analyzing common GitHub release patterns for binary distribution:

**Standard patterns:**
- `{name}-{version}-{target}.{format}` (e.g., `ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz`)
- `{name}_{version}_{os}_{arch}.{format}` (e.g., `gh_2.42.0_linux_amd64.tar.gz`)
- `{name}-{os}-{arch}` (e.g., `fzf-linux-amd64`)

**Target string variations:**
- Rust targets: `x86_64-unknown-linux-musl`, `aarch64-apple-darwin`
- Go conventions: `linux_amd64`, `darwin_arm64`
- Generic: `linux-x64`, `macos-arm64`, `win64`

**Relevance to tsuku**: An LLM can learn these patterns from examples and generalize to new tools. The key insight is that asset matching requires understanding of both naming conventions AND the specific tool's release pattern.

### Research Summary

**Common patterns:**
1. Container isolation is standard for LLM code validation
2. Tool use APIs (Claude, Gemini) are mature and have similar capabilities
3. GitHub release asset naming follows common patterns but varies per project

**Key implications for tsuku:**
1. Container validation should use `--network=none` for isolation
2. LLM client abstraction can unify Claude and Gemini behind common interface
3. GitHub Release Builder should handle multiple naming conventions through examples

## Considered Options

### Decision 1: LLM Provider Architecture

How should we structure LLM provider support?

#### Option 1A: Single Provider with Fallback

Implement Claude as primary, Gemini as fallback when Claude fails or is unavailable.

**Pros:**
- Simpler implementation
- Clear preference reduces decision complexity
- Can optimize prompts for primary provider

**Cons:**
- Fallback provider may have different capabilities/behaviors
- Prompts optimized for one provider may not work well for fallback
- Hard to add new providers without changing architecture

#### Option 1B: Provider Factory with Unified Interface

Abstract LLM providers behind a common interface. Each provider implements the same operations. Selection based on user configuration.

```go
type LLMProvider interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    SupportsTools() bool
}
```

**Pros:**
- Clean separation of provider-specific logic
- Easy to add new providers
- User can choose preferred provider
- Enables A/B testing between providers

**Cons:**
- More abstraction layers
- Must find common denominator across providers
- Some provider-specific features may not fit interface

#### Option 1C: Direct Provider Integration

No abstraction; each builder calls provider APIs directly as needed.

**Pros:**
- Full access to provider-specific features
- No interface constraints
- Simplest initial implementation

**Cons:**
- Provider logic duplicated across builders
- Hard to switch providers
- No cost tracking consolidation

### Decision 2: Validation Strategy

How should generated recipes be validated before user installation?

#### Option 2A: Container Validation with Network Isolation

Run generated recipes in Docker containers. Use `--network=none` for validation of pre-downloaded assets, ensuring the container cannot make outbound connections during recipe execution.

**How it works:**
1. Pre-download required assets (binaries, archives) before container starts
2. Mount assets into container as read-only volumes
3. Run recipe with `--network=none` (no internet access during execution)
4. Verify binaries exist and verification command passes

**Pros:**
- Strong isolation from host system
- Mirrors production installation environment
- Network isolation prevents supply chain attacks during validation
- Can verify binary functionality
- Pre-download catches network issues early

**Cons:**
- Requires Docker installation
- Slower than no validation (~30-60s overhead)
- Two-phase process (download then validate) adds complexity

#### Option 2B: Static Recipe Validation Only

Extend existing recipe validator with LLM-specific checks. No execution.

**Pros:**
- Fast (~<1s)
- No Docker dependency
- Can catch syntax and schema errors

**Cons:**
- Cannot verify recipe actually works
- False positives (valid recipes that fail at runtime)
- Misses subtle issues (wrong binary paths, missing steps)

**Note:** This option is insufficient alone but valuable as a first stage in staged validation. All generated recipes should pass static validation before container validation.

#### Option 2C: Optional Container Validation

Container validation is available but optional via `--skip-validation` flag. Default to validating.

**Pros:**
- Best of both: security by default, flexibility when needed
- Users without Docker can still use LLM builders
- Fast iteration during development with skip flag

**Cons:**
- Users may disable validation habitually
- Inconsistent behavior based on flag
- Must handle both paths

### Decision 3: Repair Loop Strategy

When validation fails, how should we attempt repair?

#### Option 3A: Error-Driven Repair with Retry Limit

Parse validation errors, provide feedback to LLM, retry generation with error context. Limit retries (e.g., 3 attempts).

**Pros:**
- Addresses specific failures rather than blind retry
- LLM can learn from errors in context
- Retry limit prevents infinite loops and cost explosion

**Cons:**
- Error parsing may be fragile
- LLM may repeat same mistakes
- Each retry adds cost and latency

#### Option 3B: Validation Feedback in Initial Prompt

Instead of repair loop, include validation rules and examples in initial prompt to minimize failures.

**Pros:**
- Single LLM call (lower cost, lower latency)
- No complex error parsing
- Simpler implementation

**Cons:**
- Cannot address runtime-specific issues
- Larger prompt may degrade quality
- No learning from actual failures

#### Option 3C: Hybrid: Enhanced Prompt + Limited Repair

Combine comprehensive initial prompt with limited repair attempts (1-2 retries).

**Pros:**
- Best of both approaches
- Comprehensive prompt reduces retries
- Repair handles edge cases prompt didn't cover

**Cons:**
- Complexity of both approaches
- Must balance prompt size with retry capability

### Decision 4: Cost Tracking Approach

How should LLM usage costs be tracked and communicated?

#### Option 4A: Per-Request Tracking with User Display

Track tokens in/out per LLM request. Calculate cost based on provider pricing. Display to user after generation.

**Pros:**
- Transparent cost visibility
- Enables optimization decisions
- Can warn before expensive operations

**Cons:**
- Pricing may change; requires updates
- Complicates implementation
- May discourage valid use

#### Option 4B: Aggregate Tracking with Telemetry

Track costs but only report in aggregate via telemetry. No per-operation display.

**Pros:**
- Simpler user experience
- Global optimization insights
- No sticker shock per operation

**Cons:**
- Users unaware of costs
- Can't make informed decisions
- Less accountability

#### Option 4C: Monitoring Only (No Display)

Track internally for development insights. No user-facing cost information.

**Pros:**
- Simplest implementation for v1
- Focus on functionality first
- Can add display later

**Cons:**
- No cost awareness for users
- Harder to optimize without data

### Evaluation Against Decision Drivers

| Decision | Option A | Option B | Option C |
|----------|----------|----------|----------|
| **D1: Provider Architecture** | | | |
| - Multi-provider support | Poor (fallback only) | Good | Poor (no sharing) |
| - Leverage infrastructure | Fair | Good | Poor |
| **D2: Validation Strategy** | | | |
| - Security first | Good | Poor | Good |
| - Graceful degradation | Fair | Good | Good |
| **D3: Repair Loop** | | | |
| - Recipe success rate | Good | Fair | Good |
| - Cost visibility | Good | Good | Fair |
| **D4: Cost Tracking** | | | |
| - Cost visibility | Good | Fair | Poor |
| - Leverage infrastructure | Good | Fair | Good |

### Uncertainties

- **LLM reliability for asset matching**: We believe LLMs can correctly match GitHub release assets to platforms, but success rate is unknown. Initial testing suggests ~85-90% accuracy on common patterns.
- **Container validation overhead**: Docker startup and recipe execution time is estimated at 30-60s but not measured for our specific use case.
- **Repair loop effectiveness**: We hypothesize repair loops improve success by 20%+, but this requires validation with real recipes.
- **Provider cost parity**: Claude and Gemini have different pricing; we assume costs are comparable enough that provider choice shouldn't be cost-driven.

### Assumptions Requiring Validation

1. **API Stability**: LLM APIs (Claude, Gemini) remain stable. Mitigation: Version prompts and track which version generated each recipe.

2. **Recipe Schema Stability**: The tsuku recipe format is stable for LLM-generated content. Mitigation: Include schema version in generated recipes.

3. **Context Window Sufficiency**: Entire GitHub Release JSON + prompt fits in context window (~100KB typical). Mitigation: Truncate asset lists for projects with >100 assets.

4. **Single-Architecture Validation Sufficiency**: Validating on one architecture (host's architecture) is sufficient for 80% success. This is explicitly a non-goal to validate cross-platform.

### Cost Model Breakdown

Estimated per-recipe costs using Claude Sonnet ($3/$15 per 1M tokens input/output):

| Component | Input Tokens | Output Tokens | Cost |
|-----------|--------------|---------------|------|
| System prompt | ~1,500 | - | $0.0045 |
| Release assets JSON | ~2,000 | - | $0.006 |
| Tool response | - | ~500 | $0.0075 |
| **Single-shot total** | 3,500 | 500 | **~$0.02** |
| Repair attempt (×2 max) | +3,000 | +500 | +$0.02 each |
| **Worst case (3 attempts)** | 9,500 | 1,500 | **~$0.06** |

Gemini Pro is approximately 60% cheaper ($1.25/$5 per 1M tokens).

## Decision Outcome

**Chosen: 1B (Provider Factory) + 2C (Optional Container Validation) + 3C (Hybrid Repair) + 4A (Per-Request Tracking)**

### Summary

LLM providers implement a unified interface with factory-based selection. Container validation runs by default with `--skip-validation` escape hatch. Comprehensive prompts minimize LLM calls while limited repair loops (2 retries max) handle edge cases. Per-request cost tracking provides user transparency.

### Rationale

This combination was chosen because:

1. **Provider Factory (1B)** ensures we can add providers without architectural changes. The abstraction cost is low since tool use patterns are similar across providers.

2. **Optional Container Validation (2C)** balances security (validation by default) with accessibility (no Docker requirement). The `--skip-validation` flag is explicit opt-out.

3. **Hybrid Repair (3C)** maximizes success rate while controlling costs. Enhanced prompts reduce retry frequency; limited retries catch edge cases.

4. **Per-Request Tracking (4A)** enables informed decisions. Users can see costs per operation and optimize their usage patterns.

These choices reinforce each other: the unified interface enables consolidated cost tracking, repair loops benefit from provider abstraction, and optional validation allows cost-conscious users to skip slow validation when iterating.

### Alternatives Rejected

- **Single Provider (1A)**: Too limiting for a feature expected to grow
- **Static Validation Only (2B)**: Insufficient security for generated code
- **Validation Feedback Only (3B)**: Cannot handle runtime-specific issues
- **Monitoring Only (4C)**: Users need cost visibility for a paid API feature

### Trade-offs Accepted

1. **Docker dependency for full functionality**: Users without Docker can use `--skip-validation` but should be aware of risks
2. **Retry cost overhead**: Failed generations with repairs may cost 2-3x single-shot; acceptable given improved success rate
3. **Provider lowest common denominator**: Some provider-specific features won't be exposed through interface

## Solution Architecture

### Overview

The LLM builder infrastructure consists of four main components:

1. **LLM Client Layer**: Provider abstraction with Claude and Gemini implementations
2. **Infrastructure Services**: Web fetcher, secrets management, cost tracking
3. **Validation Pipeline**: Container-based recipe execution and verification
4. **Builder Framework**: Base types and utilities for LLM builders

```
User: tsuku create gh --from github

       +------------------+
       |  CLI (create)    |
       +--------+---------+
                |
                v
       +------------------+
       | Builder Registry |
       +--------+---------+
                |
                v
       +------------------+        +------------------+
       | GitHubReleaseBldr|------->|  LLM Client      |
       +--------+---------+        | (Claude/Gemini)  |
                |                  +--------+---------+
                |                           |
                v                           v
       +------------------+        +------------------+
       | BuildResult      |        | Cost Tracker     |
       | - Recipe         |        +------------------+
       | - Warnings       |
       +--------+---------+
                |
                v
       +------------------+
       | Container        |
       | Validator        |
       +--------+---------+
                |
                v
       +------------------+
       | Repair Loop      |  (if validation fails)
       | - Parse errors   |
       | - Retry LLM      |
       +--------+---------+
                |
                v
       ~/.tsuku/recipes/gh.toml
```

### Components

#### LLM Client Layer

**Provider Interface** (`internal/llm/provider.go`):
```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    CompleteWithTools(ctx context.Context, req *ToolRequest) (*ToolResponse, error)
}

type CompletionRequest struct {
    System   string
    Messages []Message
    MaxTokens int
}

type ToolRequest struct {
    CompletionRequest
    Tools []ToolDefinition
}

type ToolResponse struct {
    Content   string
    ToolCalls []ToolCall
    Usage     Usage
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}
```

**Provider Factory** (`internal/llm/factory.go`):
```go
type Factory struct {
    secrets  *secrets.Manager
    tracker  *CostTracker
}

func (f *Factory) Get(name string) (Provider, error)
func (f *Factory) Available() []string
```

**Claude Provider** (`internal/llm/claude.go`):
- Implements Provider interface using Anthropic API
- Tool use via `tools` parameter in messages API
- Handles tool results in conversation

**Gemini Provider** (`internal/llm/gemini.go`):
- Implements Provider interface using Google AI API
- Function calling via `tools` parameter
- Converts between Gemini function calls and generic ToolCall

**Builder Configuration** (`$TSUKU_HOME/config.toml`):
```toml
[llm]
provider = "claude"           # "claude" or "gemini"
daily_budget = 5.0            # USD, default $5
require_confirmation = 0.50   # USD, prompt before operations exceeding this

[llm.claude]
api_key_env = "ANTHROPIC_API_KEY"  # Environment variable name

[llm.gemini]
api_key_env = "GOOGLE_API_KEY"
```

**Note:** Model selection is NOT user-configurable. Each provider has a tested model version that tsuku uses internally. This ensures consistent behavior and allows prompts to be optimized for specific models.

Configuration resolution order:
1. Command-line flags (`--provider`)
2. Environment variables (`TSUKU_LLM_PROVIDER`)
3. Config file (`$TSUKU_HOME/config.toml`)
4. Defaults (Claude)

#### Infrastructure Services

**Web Fetcher** (`internal/fetch/fetcher.go`):
```go
type Fetcher struct {
    client      *http.Client
    cache       *Cache
    rateLimiter *rate.Limiter
}

func (f *Fetcher) Fetch(ctx context.Context, url string) ([]byte, error)
func (f *Fetcher) FetchJSON(ctx context.Context, url string, v interface{}) error
```

Features:
- Response caching with TTL (5 min default)
- Rate limiting per domain
- SSRF protection (private IP blocking)
- Size limits (10MB default)
- Timeout enforcement

**Secrets Manager** (`internal/secrets/manager.go`):
```go
type Manager struct {
    envPrefix  string
    configPath string
}

func (m *Manager) Get(key string) (string, error)
func (m *Manager) GetRequired(key string) (string, error)
```

Resolution order:
1. Environment variable (e.g., `ANTHROPIC_API_KEY`)
2. Config file (`$TSUKU_HOME/config.toml` section `[secrets]`)
3. Error if required and not found

**Cost Tracker** (`internal/llm/cost.go`):
```go
type CostTracker struct {
    mu       sync.Mutex
    requests []RequestCost
}

type RequestCost struct {
    Provider     string
    Model        string
    InputTokens  int
    OutputTokens int
    Cost         float64
    Timestamp    time.Time
}

func (t *CostTracker) Record(provider string, usage Usage)
func (t *CostTracker) Summary() CostSummary
func (t *CostTracker) Total() float64
```

Pricing (configurable):
- Claude Sonnet: $3/$15 per 1M tokens (input/output)
- Gemini Pro: $1.25/$5 per 1M tokens

#### Validation Pipeline

**Container Validator** (`internal/validate/container.go`):
```go
type ContainerValidator struct {
    dockerClient *client.Client
    baseImage    string
}

type ValidationResult struct {
    Success  bool
    ExitCode int
    Stdout   string
    Stderr   string
    Duration time.Duration
    Error    error
}

func (v *ContainerValidator) Validate(ctx context.Context, recipe *recipe.Recipe) (*ValidationResult, error)
```

Validation steps:
1. **Static validation** (always): Run recipe through existing validator + LLM-specific checks
2. **Pre-download**: Download required assets to temp directory based on recipe actions
3. **Checksum capture**: Compute SHA256 of all downloaded assets
4. **Container setup**: Create container with `--network=none --ipc=none`, mount pre-downloaded assets read-only
5. **Installation**: Run recipe with `TSUKU_ASSET_DIR=/assets TSUKU_SKIP_DOWNLOAD=true`
6. **Verification**: Check exit code, verify binaries exist, run verify command
7. **Checksum embedding**: Add computed checksums to recipe before writing
8. **Error collection**: Sanitize stdout/stderr (remove paths, env vars) for repair loop
9. **Cleanup**: Remove container and temp assets

**Container resource limits:**
- Memory: 2GB max
- CPU: 2 cores max
- Disk: 10GB max
- Timeout: 5 minutes

**Asset pre-download directory structure:**
```
/tmp/tsuku-validate-xxxxx/
├── assets/                  # Pre-downloaded assets (mounted read-only)
│   ├── gh_2.42.0_linux_amd64.tar.gz
│   └── checksums.txt
└── workspace/               # Recipe execution directory
```

**Error Parser** (`internal/validate/errors.go`):
```go
type ParsedError struct {
    Type     ErrorType
    Message  string
    Location string  // e.g., "step 2: extract"
    Hint     string  // Suggested fix
}

type ErrorType int

const (
    ErrDownloadFailed ErrorType = iota
    ErrExtractionFailed
    ErrBinaryNotFound
    ErrVerificationFailed
    ErrPermissionDenied
    ErrDependencyMissing
)

func ParseValidationOutput(stdout, stderr string) []ParsedError
```

#### Builder Framework

**LLM Builder Base** (`internal/builders/llm_base.go`):
```go
type LLMBuilder struct {
    provider    llm.Provider
    validator   *validate.ContainerValidator
    costTracker *llm.CostTracker
    maxRetries  int
}

func (b *LLMBuilder) BuildWithRetry(
    ctx context.Context,
    generate func() (*recipe.Recipe, []string, error),
) (*BuildResult, error)
```

The base handles:
- Retry loop with exponential backoff
- Error parsing and feedback formatting
- Cost tracking across retries
- Validation orchestration

**GitHub Release Builder** (`internal/builders/github_release.go`):
```go
type GitHubReleaseBuilder struct {
    *LLMBuilder
    fetcher  *fetch.Fetcher
    resolver *version.Resolver
}

func (b *GitHubReleaseBuilder) Name() string { return "github" }

func (b *GitHubReleaseBuilder) CanBuild(ctx context.Context, pkg string) (bool, error)

func (b *GitHubReleaseBuilder) Build(ctx context.Context, pkg, version string) (*BuildResult, error)
```

Build workflow:
1. Parse package as `owner/repo` or GitHub URL
2. Fetch release metadata from GitHub API
3. Call LLM with release assets JSON and tool definitions
4. LLM returns structured asset mappings
5. Generate recipe from mappings
6. Validate in container (if enabled)
7. Retry with error feedback if validation fails

### Data Flow

**Generation Flow:**
```
1. User: tsuku create gh --from github

2. CLI parses: tool="gh", ecosystem="github"

3. GitHubReleaseBuilder.Build(ctx, "cli/cli", ""):
   a. Fetch release from api.github.com/repos/cli/cli/releases/latest
   b. Extract assets list from response

4. LLM.CompleteWithTools(ctx, req):
   - System prompt: "You are matching GitHub release assets to platforms..."
   - User message: JSON list of assets
   - Tool: match_assets(mappings: [{asset, os, arch}, ...])

5. Parse tool call response into asset mappings

6. Generate recipe:
   Recipe{
     Metadata: {name: "gh", ...},
     Version: {source: "github:cli/cli", ...},
     Steps: [
       {action: "github_release", asset: "{{.asset}}", ...},
       {action: "extract", ...},
       {action: "install_binaries", files: ["gh"]},
     ],
     Verify: {command: "gh --version"},
   }

7. ContainerValidator.Validate(ctx, recipe):
   - Run in Docker with --network=none
   - Check exit code 0
   - Verify binaries exist

8. If validation fails and retries < 3:
   - Parse errors from stdout/stderr
   - Add error context to LLM prompt
   - Retry from step 4

9. Write to ~/.tsuku/recipes/gh.toml
```

### Key Interfaces

**Tool Definitions for LLM:**

Tool parameters use typed structs for compile-time safety:

```go
// Typed parameter structs ensure compile-time validation
type AssetMapping struct {
    Asset  string `json:"asset"`                    // Asset filename
    OS     string `json:"os" enum:"linux,darwin,windows"`
    Arch   string `json:"arch" enum:"amd64,arm64,386"`
    Format string `json:"format" enum:"tar.gz,zip,binary"`
}

type AssetMatchResult struct {
    Mappings      []AssetMapping `json:"mappings"`
    Executable    string         `json:"executable"`     // Name of the binary executable
    VerifyCommand string         `json:"verify_command"` // Command to verify installation
}

// ToolDefinition converts typed struct to JSON Schema at registration time
var AssetMatchingTool = llm.ToolDefinition{
    Name:        "match_assets",
    Description: "Match GitHub release assets to OS/architecture combinations",
    Parameters:  llm.SchemaFromType(AssetMatchResult{}),
}
```

**Error Feedback Format:**
```go
type RepairContext struct {
    PreviousRecipe string   // TOML of failed recipe
    Errors         []string // Parsed error messages
    Attempt        int      // Current retry number
}

func FormatRepairPrompt(ctx RepairContext) string {
    return fmt.Sprintf(`Your previous recipe failed validation.

Previous recipe:
%s

Errors:
%s

Please fix the issues and generate a corrected recipe.`,
        ctx.PreviousRecipe,
        strings.Join(ctx.Errors, "\n"))
}
```

## Implementation Approach

The implementation follows **vertical slices** that deliver end-to-end value at each stage, rather than building horizontal layers. Each slice proves a key hypothesis before moving to the next.

### Slice 1: End-to-End Spike

**Goal:** Prove an LLM can produce working recipes from GitHub release data.

**Deliverables:**
- Direct Claude API integration (no abstraction yet)
- Hardcoded test with a known GitHub repo (e.g., cli/cli)
- Print generated TOML to stdout
- Basic cost tracking

**Explicitly excluded:**
- Container validation
- Repair loops
- Configuration management
- Error handling beyond basic

**Validation:** Generated recipe can be manually installed and works.

**Exit criteria:** LLM produces syntactically valid recipes that work for at least one test repo.

### Slice 2: Add Container Validation

**Goal:** Prove container-based validation catches broken recipes.

**Deliverables:**
- `internal/validate/container.go` - Container runtime abstraction (Docker + Podman)
- `internal/validate/predownload.go` - Asset pre-download with checksum capture
- Container isolation (`--network=none --ipc=none`)
- Resource limits (2GB RAM, 2 CPU, 10GB disk, 5 min timeout)
- Lock file mechanism for parallel execution safety
- Startup cleanup for orphaned containers/temp directories

**Container Runtime Abstraction:**
```go
type ContainerRuntime interface {
    CreateContainer(ctx context.Context, config *ContainerConfig) (string, error)
    StartContainer(ctx context.Context, containerID string) error
    WaitContainer(ctx context.Context, containerID string) (int, error)
    RemoveContainer(ctx context.Context, containerID string) error
    GetLogs(ctx context.Context, containerID string) (string, string, error)
}

// Auto-detection: Try Podman first (rootless-friendly), fall back to Docker
func NewContainerRuntime(ctx context.Context) (ContainerRuntime, error)
```

**Validation:** Validate existing registry recipes in containers before adding LLM complexity.

**Exit criteria:** Container validation correctly identifies broken recipes and passes working ones.

### Slice 3: Add Repair Loop + Second Provider

**Goal:** Prove repair loops improve success rate and validate the provider abstraction with two real implementations.

**Deliverables:**
- `internal/llm/provider.go` - Provider interface
- `internal/llm/factory.go` - Provider factory with circuit breaker
- `internal/llm/claude.go` - Claude implementation
- `internal/llm/gemini.go` - Gemini implementation
- `internal/validate/sanitize.go` - Error message sanitization for repair loop
- `internal/validate/errors.go` - Error parser for structured feedback
- Repair loop with max 2 retries
- Telemetry events for success rates

**Circuit Breaker Pattern:**
```go
type CircuitBreaker struct {
    failures     int
    lastFailure  time.Time
    state        State // Closed, Open, HalfOpen
    threshold    int   // failures before opening (default: 3)
    timeout      time.Duration // time before half-open (default: 60s)
}
```

Per-provider circuit breakers ensure one provider's outage doesn't block the other.

**Error Sanitization (MVP):**
```go
func SanitizeErrorOutput(output string) string {
    // Truncate to reasonable length
    if len(output) > 2000 {
        output = output[:2000] + "\n[truncated]"
    }

    // Redact sensitive patterns
    patterns := []struct {
        regex   *regexp.Regexp
        replace string
    }{
        {regexp.MustCompile(`/home/[^/\s]+`), "[HOME]"},
        {regexp.MustCompile(`/Users/[^/\s]+`), "[HOME]"},
        {regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), "[IP]"},
        {regexp.MustCompile(`(api[_-]?key|token|secret|password)=\S+`), "$1=[REDACTED]"},
    }

    for _, p := range patterns {
        output = p.regex.ReplaceAllString(output, p.replace)
    }

    return output
}
```

**Telemetry Events:**
- `llm_generation_started` - provider, tool name
- `llm_generation_completed` - success/failure, cost, duration, attempt count
- `llm_validation_result` - pass/fail, error category

**Validation:** A/B test Claude vs Gemini on same repos. Measure repair loop improvement.

**Exit criteria:**
- Both providers work through unified interface
- Repair loops improve success rate by measurable amount
- Circuit breaker correctly handles API failures

### Slice 4: Productionize

**Goal:** Ship to users with proper UX, error handling, and safety features.

**Deliverables:**
- Update `tsuku create` to support `--from github`
- Register GitHub Release Builder in builder registry
- Configuration management (4-level: flags → env → file → defaults)
- `internal/secrets/manager.go` - API key resolution with 0600 permission enforcement
- Cost display after generation with confirmation for operations >$0.50
- `--skip-validation` flag
- Recipe preview before installation (mandatory for LLM-generated recipes)
- Actionable error messages
- Progress indicators during generation
- Rate limiting: max 10 LLM generations per hour
- Daily budget enforcement ($5 default)

**Recipe Preview Requirement:**

LLM-generated recipes must be shown to the user before installation:
```
$ tsuku create mytool --from github

Generated recipe for mytool:

  Downloads:
    - https://github.com/owner/mytool/releases/download/v1.0.0/mytool-linux-amd64.tar.gz

  Actions:
    1. Download release asset
    2. Extract tar.gz archive
    3. Install binary: mytool

  Verification: mytool --version

Install this recipe? [y/N]
```

**Error Message Examples:**

```
Recipe generation failed: no release artifacts found for "owner/repo"

This usually means:
  1. The repository has no GitHub releases
  2. Releases exist but contain no binary artifacts

Try:
  - Check https://github.com/owner/repo/releases
  - Use --version flag to target a specific release
```

```
Validation failed after 2 attempts:

Attempt 1: Binary not found after extraction
  Expected: tool-linux-amd64
  Found: tool_linux_amd64

Attempt 2: Same error (repair did not fix)

Suggestions:
  - Review generated recipe at ~/.tsuku/recipes/tool.toml
  - Edit the 'executables' field to match actual binary names
```

**Validation:** Full user flow testing with real users.

**Exit criteria:** Users can generate and install recipes with clear feedback at each step.

### Testing Strategy

LLM-dependent code uses a **record/replay** pattern:

1. **Development:** Record real API responses during development
2. **Storage:** Save responses to `testdata/llm_responses/` as fixtures
3. **CI:** Replay recorded responses to avoid API costs and ensure determinism
4. **Maintenance:** Re-record periodically to catch API changes

```go
// Test mode detection
func (c *Client) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    if replayPath := os.Getenv("TSUKU_LLM_REPLAY_DIR"); replayPath != "" {
        return c.replayFromFixture(replayPath, req)
    }

    resp, err := c.doAPICall(ctx, req)

    if recordPath := os.Getenv("TSUKU_LLM_RECORD_DIR"); recordPath != "" {
        c.recordFixture(recordPath, req, resp)
    }

    return resp, err
}
```

### Dependency Graph

```
Slice 1: End-to-End Spike
    │
    │ Proves: LLM can produce working recipes
    │
    v
Slice 2: Container Validation
    │
    │ Proves: Validation catches broken recipes
    │
    v
Slice 3: Repair + Second Provider
    │
    │ Proves: Repair loops help, abstraction works
    │
    v
Slice 4: Productionize
    │
    │ Result: Ship to users
    v
```

**Benefits of vertical slices:**
1. Each slice delivers end-to-end proof of a hypothesis
2. Problems are discovered early (e.g., if LLM can't produce good recipes, we know in Slice 1)
3. Abstraction is validated with real implementations (two providers in Slice 3)
4. No wasted infrastructure work if core approach doesn't work

## Security Considerations

### Download Verification

**How are downloaded artifacts validated?**

The LLM builder infrastructure downloads data from two sources:

1. **GitHub API metadata**: Release JSON and asset lists
   - HTTPS only with certificate verification
   - Response size limits (10MB)
   - JSON schema validation before parsing

2. **LLM API responses**: Generated recipe content
   - Responses are structured (tool calls), not arbitrary code
   - All content passes through recipe validator before use
   - No execution of LLM output; only recipe generation

Binaries are NOT downloaded during recipe generation. Binary downloads happen at install time through existing actions with established verification.

### Execution Isolation

**What permissions does this feature require?**

- **Network access**: Required to query GitHub API and LLM APIs
- **Docker socket access**: Required for container validation (optional with `--skip-validation`)
- **File system access**: Write to `$TSUKU_HOME/recipes/` and `$TSUKU_HOME/config.toml`
- **No elevated privileges**: All operations run as current user

**Container validation isolation:**
- Network disabled (`--network=none`)
- Read-only source mounts where possible
- No volume mounts to sensitive host directories
- Resource limits (CPU, memory, time)
- Unprivileged container execution

### Supply Chain Risks

**Where do artifacts come from?**

1. **LLM Providers (Anthropic, Google)**: Trusted API endpoints
   - API keys stored securely (env vars or config file)
   - HTTPS-only communication
   - No code execution from providers; only structured responses

2. **GitHub API**: Trusted source for release metadata
   - Only metadata is fetched during generation
   - Actual binaries come from GitHub releases at install time
   - Users can inspect generated recipes before installation

**What if the LLM is compromised or produces malicious output?**

1. **Recipe validation**: All generated recipes pass through the same validator as registry recipes
2. **Container isolation**: Recipes are tested in isolated containers before user installation
3. **User inspection**: Users can review recipes before running `tsuku install`
4. **No direct execution**: LLM output is structured data, not executable code

### User Data Exposure

**What user data does this feature access or transmit?**

**Data accessed locally:**
- Tool/package names (provided by user)
- API keys from environment or config

**Data transmitted to LLM providers:**
- GitHub release asset names and metadata
- Error messages during repair loops
- No user identifying information
- No local file contents

**Data transmitted to GitHub:**
- Repository names
- Standard GitHub API queries

**Privacy implications:**
- LLM providers see which tools users are creating recipes for
- Same privacy model as using GitHub and LLM APIs directly

### Prompt Injection Risks

**What if GitHub release assets contain malicious content?**

Asset names could theoretically contain prompt injection attempts. Mitigations:

1. **Structured output**: LLM is asked to return tool calls, not execute commands
2. **Schema validation**: Tool call outputs must match defined schemas
3. **No execution context**: LLM has no access to filesystem, network, or credentials beyond what's in prompt
4. **Asset name sanitization** (applied before LLM call):
   - Maximum length: 256 characters
   - Reject control characters (0x00-0x1F, 0x7F)
   - Reject Unicode homoglyphs
   - Reject template syntax (`{{`, `}}`)
   - Reject TOML syntax characters in suspicious positions
   - Wrap asset names in code blocks in prompts

### Time-of-Check-Time-of-Use (TOCTOU) Risks

**What if assets change between validation and installation?**

A validated recipe could pass with benign binaries, but malicious binaries could be served when the user later runs `tsuku install`.

Mitigations:

1. **Checksum generation during validation**: After successful container validation, compute SHA256 of all downloaded assets
2. **Checksum embedding**: Add computed checksums to generated recipe before writing to disk
3. **Mandatory verification**: All LLM-generated recipes include checksums; installation fails if checksums don't match
4. **Timestamp warning**: Display warning that recipe was validated at specific timestamp

**Recipe metadata for LLM-generated recipes:**
```toml
[metadata]
name = "gh"
generated_by = "llm:claude"
validated_at = "2025-01-15T10:30:00Z"
validation_platform = "linux/amd64"
```

### LLM-Specific Recipe Validation

Generated recipes undergo additional validation beyond standard registry recipes:

1. **Step count limit**: Maximum 20 steps per recipe
2. **Dependency limit**: Maximum 5 dependencies
3. **Privilege escalation detection**: Warn on actions that might require elevated privileges
4. **`run_command` review**: Flag recipes using `run_command` for explicit user approval
5. **Verify command safety**: Reject verify commands with shell operators (`;`, `&`, `|`)

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious LLM output | Recipe validator + container isolation + LLM-specific limits | Novel attacks not caught by validator |
| Prompt injection via assets | Structured tool use, schema validation, asset name sanitization | Sophisticated injection bypassing structure |
| API key exposure | Environment variables, secure config file (0600 perms) | Accidental logging, memory dumps |
| Container escape | Network isolation, IPC isolation, resource limits | Docker vulnerabilities |
| Cost explosion | Retry limits (3), hourly rate limit (10), daily budget ($5), confirmation >$0.50 | Users ignoring cost warnings |
| LLM hallucination | Container validation catches failures | Subtle issues passing validation |
| TOCTOU attacks | Checksum generation during validation, embedded in recipe | Assets changing to same checksum |
| Resource exhaustion | Container limits (2GB RAM, 2 CPU, 10GB disk, 5 min) | Attacks below limits |

## Consequences

### Positive

1. **Expanded coverage**: GitHub Release Builder enables recipes for thousands of tools distributed as GitHub releases
2. **Foundation for LLM builders**: Architecture supports future builders (Aqua, Homebrew, Documentation)
3. **Provider flexibility**: Users can choose their preferred LLM provider
4. **Security by default**: Container validation catches broken recipes before user installation
5. **Cost transparency**: Users see exactly what LLM operations cost

### Negative

1. **Docker dependency**: Container validation requires Docker; users without Docker must skip validation
2. **API key requirement**: Users must provide their own LLM API keys
3. **Latency**: Recipe generation takes 30-60s including validation (vs <1s for deterministic builders)
4. **Cost**: Each recipe generation costs ~$0.05-0.10 (vs free for deterministic builders)
5. **Non-deterministic**: Same tool may generate slightly different recipes on repeated runs

### Mitigations

1. **Docker dependency**: `--skip-validation` flag for users without Docker; static validation still runs
2. **API key requirement**: Clear error messages guide users to get API keys
3. **Latency**: Progress indicators during generation; caching for repeated tool attempts
4. **Cost**: Display costs after operation; users can track spending
5. **Non-determinism**: Once a recipe works, it's cached locally; no regeneration unless requested
