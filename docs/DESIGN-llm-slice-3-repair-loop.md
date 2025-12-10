# Design Document: LLM Slice 3 - Repair Loop + Second Provider

**Status**: Current

**Parent Design**: [DESIGN-llm-builder-infrastructure.md](DESIGN-llm-builder-infrastructure.md)

**Issue**: [#269 - Slice 3: Repair Loop + Second Provider](https://github.com/tsukumogami/tsuku/issues/269)

## Context and Problem Statement

Slice 1 established the critical path for LLM-based recipe generation with Claude. Slice 2 added container-based validation to catch broken recipes before user installation. This slice addresses two key gaps:

1. **Single-shot generation is insufficient**: When validation fails, users must manually iterate or abandon the attempt. A repair loop that feeds validation errors back to the LLM can recover from many failures automatically.

2. **Single provider creates risk**: Relying solely on Claude means API outages or rate limits block all recipe generation. Adding Gemini as a second provider enables fallback and lets users choose based on preference or cost.

### Why These Two Together

These features are bundled because:

1. **Provider abstraction enables clean repair**: The repair loop needs to call the LLM multiple times. Having a clean provider interface ensures repair logic doesn't couple to Claude-specific code.

2. **Circuit breaker validates abstraction**: Adding a second provider with circuit breaker failover proves the abstraction works correctly under failure conditions.

3. **Error sanitization is shared**: Both repair loops and provider switching benefit from sanitized error messages that don't leak sensitive data.

### Current State

From Slice 1 implementation:
- `internal/llm/client.go` - Direct Claude integration using anthropic-sdk-go
- `internal/llm/tools.go` - Tool definitions (fetch_file, inspect_archive, extract_pattern)
- `internal/llm/cost.go` - Token usage and cost tracking
- Multi-turn conversation loop with max 5 turns

From Slice 2 implementation:
- `internal/validate/runtime.go` - Container runtime abstraction (Docker/Podman)
- `internal/validate/predownload.go` - Asset pre-download with checksum capture
- `internal/validate/executor.go` - Orchestrates container-based validation
- `internal/validate/cleanup.go` - Startup cleanup for orphaned containers
- `internal/validate/lock.go` - Lock file mechanism for parallel safety

### Problems to Solve

**Problem 1: No recovery from validation failures**

Current flow:
```
Generate recipe -> Validate in container -> Fail -> User must retry manually
```

Desired flow:
```
Generate recipe -> Validate -> Fail -> Parse errors -> Retry with feedback -> Success
```

**Problem 2: Single point of failure**

Current: Claude API unavailable = no recipe generation

Desired: Claude fails -> Gemini fallback (and vice versa)

**Problem 3: Sensitive data in error messages**

Container validation produces stderr/stdout that may contain:
- Absolute paths revealing username (`/home/username/...`)
- Environment variables that leaked into output
- IP addresses from network diagnostics
- Credential patterns from failed auth attempts

These must be sanitized before sending to LLM for repair.

### Hypotheses to Validate

1. **Repair loops improve success rate by 20%+** - Error feedback enables LLM to fix common issues (wrong binary path, missing extraction step, incorrect verification command).

2. **Provider abstraction works** - Both Claude and Gemini can produce equivalent recipes through the same interface.

3. **Circuit breaker prevents cascade failures** - When one provider fails, traffic shifts to the other without blocking.

## Decision Drivers

- **Extend, don't rewrite**: Build on Slice 1's client.go patterns, don't replace them
- **Provider parity**: Both providers must support the same tool use patterns
- **Security first**: Error sanitization before any external transmission
- **Cost awareness**: Repair loops add cost; limit retries to control spend
- **Testability**: Provider interface enables mock implementations for testing

## External Research

### Provider Abstraction Patterns

Research into LLM frameworks (LangChain, Semantic Kernel, LlamaIndex), Go libraries (langchaingo, go-openai), and multi-provider coding agents (Aider, Open Interpreter, goose, Continue) shows strong consensus:

**Conversation loops belong in the orchestration layer, not provider implementations.**

Across all surveyed tools, providers handle single request/response while the multi-turn loop lives in application/business logic code. This pattern keeps providers simple (~50 lines for type conversion) and makes the loop testable and customizable per use case.

**Anthropic vs Google SDK differences** also favor thin abstraction:
- Anthropic: Stateless (send full history each turn)
- Google GenAI: Stateful sessions (history managed internally)

These fundamental differences mean a thick shared abstraction would leak. A thin single-turn interface lets each provider use its SDK naturally while the builder owns conversation state.

### Gemini Function Calling

From [Google AI Function Calling documentation](https://ai.google.dev/gemini-api/docs/function-calling):

Gemini supports function calling with a similar pattern to Claude's tool use:
- Function declarations with JSON Schema parameters
- Model returns `functionCall` in response when it wants to use a function
- Application executes function and sends result back
- Model can make multiple function calls in a conversation

Key differences from Claude:
- Uses `functionCall` / `functionResponse` instead of `tool_use` / `tool_result`
- `tool_config` controls whether model must call functions
- Function results are wrapped in `Part` objects

Go SDK: [github.com/google/generative-ai-go](https://github.com/google/generative-ai-go)

### Circuit Breaker Patterns

The circuit breaker pattern prevents cascade failures by:
1. **Closed**: Normal operation, requests pass through
2. **Open**: After N failures, reject immediately (fail fast)
3. **Half-Open**: After timeout, allow one request to test recovery

Key parameters:
- **Failure threshold**: Consecutive failures to trip (3 is common)
- **Recovery timeout**: Time to wait before trying again (30-60s typical)
- **Success threshold**: Successes needed in half-open to close (1-2)

Go libraries: sony/gobreaker, afex/hystrix-go, or simple custom implementation.

For tsuku's use case:
- Low request volume (1-10 recipe generations per session)
- Two providers means simple failover, not load balancing
- Simple custom implementation preferred over external dependency

### Error Sanitization Patterns

Common patterns to redact:
- Home directory paths: `/home/username/`, `/Users/username/`
- Windows user paths: `C:\Users\username\`
- IP addresses: IPv4 and IPv6 patterns
- Credentials: `api_key=`, `token=`, `password=`, `secret=`
- Environment variables: `$VAR` or `${VAR}` patterns in error context
- Database connection strings

Truncation is also important:
- LLM context has limits
- Long stack traces don't add value
- 2000 characters is reasonable for error context

## Considered Options

### Decision 1: Provider Interface Design

How should the provider abstraction be structured?

#### Option 1A: Thin Interface with Loop in Builder

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}
```

Provider handles single request/response. Multi-turn loop lives in `GitHubReleaseBuilder`.

**Pros:**
- Single-turn providers are simple to implement and test
- Multi-turn logic written once in builder
- Each provider uses native SDK patterns
- Easy to mock for unit tests (just mock `Complete()`)
- Matches industry patterns for multi-provider LLM tools

**Cons:**
- Must define common Message/Response types
- Tool definitions need conversion per provider at call boundary

#### Option 1B: Layered Interface with Shared Conversation Logic

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
}
```

Shared code handles multi-turn loop, calling provider.Complete() at each turn.

**Pros:**
- Multi-turn logic written once
- Easier to ensure consistent behavior
- Provider implementations are simpler

**Cons:**
- Must find common denominator across APIs
- Some provider-specific optimizations may not fit
- More abstraction layers

#### Option 1C: Hybrid with Shared Tools

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error)
}
```

Conversation loop is shared, but providers handle their own tool result formatting.

**Pros:**
- Balance between consistency and flexibility
- Tools can be provider-agnostic
- Providers control response parsing

**Cons:**
- Still need to standardize message/response types
- Tool definitions need conversion per provider

### Decision 2: Circuit Breaker Scope

How should circuit breaker state be managed?

#### Option 2A: Per-Provider Circuit Breakers

Each provider has its own circuit breaker. Failures on Claude don't affect Gemini's circuit state.

**Pros:**
- Independent failure domains
- One provider's outage doesn't block the other
- Clear failure attribution

**Cons:**
- More state to manage
- Need logic to select next provider when one trips

#### Option 2B: Global Circuit Breaker

Single circuit breaker for all LLM operations. Any provider failure contributes to the trip threshold.

**Pros:**
- Simpler state management
- Protects against broader API issues

**Cons:**
- Transient failure on one provider blocks both
- Doesn't leverage having multiple providers

#### Option 2C: No Circuit Breaker (Simple Failover)

Try primary provider, on failure try secondary. No state tracking.

**Pros:**
- Simplest implementation
- No state to corrupt or leak

**Cons:**
- Every request to a dead provider incurs timeout
- No protection against repeated failures
- Slower failure detection

### Decision 3: Repair Loop Strategy

How should validation failures trigger repair attempts?

#### Option 3A: Immediate Retry with Error Context

On validation failure, immediately start new generation with error context appended to prompt.

**Pros:**
- Simple flow
- LLM gets full fresh context each attempt
- Easy to implement

**Cons:**
- Full cost per retry (no context reuse)
- LLM may repeat same mistakes
- No incremental improvement tracking

#### Option 3B: Continue Conversation with Error Feedback

Keep the conversation going - send validation error as a new user message.

**Pros:**
- LLM has context of its previous attempt
- Can learn from specific failure
- Lower token cost (incremental context)

**Cons:**
- Conversation may get long with multiple failures
- More complex state management
- Provider switch requires starting fresh

#### Option 3C: Structured Error Analysis + Targeted Retry

Parse validation errors into categories, add specific guidance based on error type.

**Pros:**
- Targeted feedback more likely to fix issue
- Can skip retry for unfixable errors
- Better telemetry on failure modes

**Cons:**
- More complex error parsing
- Must maintain error category mapping
- May miss novel error patterns

### Evaluation Against Decision Drivers

| Decision | Option A | Option B | Option C |
|----------|----------|----------|----------|
| **D1: Provider Interface** | | | |
| - Extend, don't rewrite | Good (client.go -> claude.go) | Fair (significant refactor) | Good |
| - Provider parity | Fair | Good | Good |
| - Testability | Fair | Good | Good |
| **D2: Circuit Breaker** | | | |
| - Provider parity | Good | Poor | Poor |
| - Cost awareness | Good | Fair | Poor |
| **D3: Repair Loop** | | | |
| - Cost awareness | Poor | Good | Good |
| - Provider parity | Good | Poor | Good |
| - Testability | Good | Fair | Fair |

## Decision Outcome

**Chosen: 1A (Thin Interface with Loop in Builder) + 2A (Per-Provider Breakers) + 3B (Continue Conversation)**

### Summary

Providers implement single-turn request/response only. The multi-turn conversation loop lives in `GitHubReleaseBuilder`, written once and shared across providers. Per-provider circuit breakers ensure independent failure domains. Repair loops continue the existing conversation with error feedback, keeping token costs lower and maintaining LLM context.

### Rationale

1. **Thin Interface (1A)**: Industry patterns show conversation loops belong in the orchestration layer, not provider implementations. Providers should handle single-turn only, making them simple to implement, test, and maintain. The loop in `GitHubReleaseBuilder` can optimize for the specific recipe generation use case.

2. **Per-Provider Breakers (2A)**: The whole point of having two providers is resilience. Coupling their failure states defeats that purpose. Per-provider breakers let Claude outages gracefully failover to Gemini.

3. **Continue Conversation (3B)**: LLMs perform better with context. Continuing the conversation means the LLM remembers what it tried and why it failed. Token cost is incremental rather than full restart. Provider switching still works - it just means starting a fresh conversation on the new provider.

### Trade-offs Accepted

1. **Common message types**: Must define `Message` and `Response` types that work for both providers. Mitigation: Types are simple (role, content, tool calls) and well-established.

2. **Provider switch loses context**: Switching providers mid-repair starts fresh. Mitigation: Include original error context in new conversation.

3. **Conversation length limits**: Long repair chains may hit context limits. Mitigation: Max 2 repair attempts bounds conversation size.

## Solution Architecture

### Overview

```
GitHubReleaseBuilder.Build()
│
├── Select Provider (factory with circuit breakers)
│   ├── Claude (primary if available)
│   └── Gemini (fallback)
│
├── Generate Recipe (provider.GenerateRecipe)
│   └── Multi-turn conversation
│
├── Validate in Container (executor.Validate)
│   ├── Pass → Return recipe
│   └── Fail → Continue to repair
│
└── Repair Loop (max 2 attempts)
    ├── Sanitize error output
    ├── Continue conversation with error feedback
    ├── Validate new recipe
    └── Repeat or fail
```

### New Files

| File | Purpose |
|------|---------|
| `internal/llm/provider.go` | Provider interface and types |
| `internal/llm/factory.go` | Provider factory with circuit breaker |
| `internal/llm/claude.go` | Claude provider (refactored from client.go) |
| `internal/llm/gemini.go` | Gemini provider |
| `internal/llm/breaker.go` | Circuit breaker implementation |
| `internal/validate/sanitize.go` | Error message sanitization |
| `internal/validate/errors.go` | Error parser for structured feedback |

### Modified Files

| File | Changes |
|------|---------|
| `internal/llm/client.go` | Deprecated, code moves to claude.go |
| `internal/builders/github_release.go` | Use factory, add repair loop |

### Components

#### 1. Provider Interface (`internal/llm/provider.go`)

```go
// Provider defines the interface for single-turn LLM completion.
// Multi-turn conversation loops live in GitHubReleaseBuilder, not here.
type Provider interface {
    // Name returns the provider identifier (e.g., "claude", "gemini").
    Name() string

    // Complete sends messages to the LLM and returns a single response.
    // Tool calls in the response must be handled by the caller (builder).
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
}

// CompletionRequest contains input for a single LLM turn.
type CompletionRequest struct {
    SystemPrompt string
    Messages     []Message
    Tools        []ToolDef
    MaxTokens    int
}

// Message represents a conversation message.
type Message struct {
    Role       Role        // user, assistant
    Content    string      // Text content
    ToolCalls  []ToolCall  // Tool calls (assistant only)
    ToolResult *ToolResult // Tool result (user only)
}

type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

// ToolCall represents an LLM request to call a tool.
type ToolCall struct {
    ID        string         // Unique identifier for correlation
    Name      string         // Tool name
    Arguments map[string]any // Parsed arguments
}

// ToolResult contains the output from executing a tool.
type ToolResult struct {
    CallID  string // Correlates to ToolCall.ID
    Content string // Tool execution output
    IsError bool   // True if tool execution failed
}

// CompletionResponse contains the LLM's response for a single turn.
type CompletionResponse struct {
    Content    string      // Text response (may be empty if tool calls)
    ToolCalls  []ToolCall  // Requested tool calls
    StopReason string      // "end_turn", "tool_use", "max_tokens"
    Usage      Usage       // Token counts for this turn
}

// ToolDef defines a tool the LLM can call.
// Providers convert to native format (Claude tool_use, Gemini functionCall).
type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any // JSON Schema
}
```

#### 2. Provider Factory (`internal/llm/factory.go`)

```go
// Factory creates and manages LLM providers with circuit breakers.
type Factory struct {
    providers map[string]Provider
    breakers  map[string]*CircuitBreaker
    primary   string // Preferred provider name
    logger    FactoryLogger
}

// FactoryOption configures a Factory.
type FactoryOption func(*Factory)

// WithPrimaryProvider sets the preferred provider.
func WithPrimaryProvider(name string) FactoryOption

// WithLogger sets the logger for circuit breaker events.
func WithLogger(logger FactoryLogger) FactoryOption

// NewFactory creates a factory with available providers.
// It auto-detects available providers based on environment variables.
func NewFactory(opts ...FactoryOption) (*Factory, error)

// GetProvider returns an available provider, respecting circuit breaker state.
// Returns the primary provider if available, otherwise falls back.
func (f *Factory) GetProvider(ctx context.Context) (Provider, error)

// ReportSuccess records a successful operation for circuit breaker.
func (f *Factory) ReportSuccess(providerName string)

// ReportFailure records a failed operation for circuit breaker.
func (f *Factory) ReportFailure(providerName string)

// AvailableProviders returns names of providers with closed/half-open breakers.
func (f *Factory) AvailableProviders() []string
```

#### 3. Circuit Breaker (`internal/llm/breaker.go`)

```go
// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
    name            string
    state           State
    failures        int
    lastFailure     time.Time
    failureThreshold int           // Failures to trip (default: 3)
    recoveryTimeout  time.Duration // Time before half-open (default: 60s)
    mu              sync.Mutex
}

type State int

const (
    StateClosed   State = iota // Normal operation
    StateOpen                  // Failing, reject requests
    StateHalfOpen              // Testing recovery
)

// NewCircuitBreaker creates a breaker with default settings.
func NewCircuitBreaker(name string) *CircuitBreaker

// Allow checks if a request should proceed.
// Returns false if breaker is open.
func (cb *CircuitBreaker) Allow() bool

// RecordSuccess resets failure count and closes breaker.
func (cb *CircuitBreaker) RecordSuccess()

// RecordFailure increments failure count and may trip breaker.
func (cb *CircuitBreaker) RecordFailure()

// State returns the current breaker state.
func (cb *CircuitBreaker) State() State
```

#### 4. Claude Provider (`internal/llm/claude.go`)

Simplified single-turn implementation:

```go
// ClaudeProvider implements Provider using the Anthropic API.
type ClaudeProvider struct {
    client *anthropic.Client
    model  anthropic.Model
}

// NewClaudeProvider creates a provider using ANTHROPIC_API_KEY.
func NewClaudeProvider() (*ClaudeProvider, error)

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    // Convert ToolDef to anthropic.ToolParam
    tools := p.convertTools(req.Tools)

    // Convert Message to anthropic.MessageParam
    messages := p.convertMessages(req.Messages)

    // Make single API call
    resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     p.model,
        System:    anthropic.F([]anthropic.TextBlockParam{{Text: anthropic.F(req.SystemPrompt)}}),
        Messages:  anthropic.F(messages),
        Tools:     anthropic.F(tools),
        MaxTokens: anthropic.F(int64(req.MaxTokens)),
    })

    // Convert response to CompletionResponse
    return p.convertResponse(resp), err
}
```

Key responsibilities:
- Convert common types to Anthropic SDK types
- Make single API call
- Convert response back to common types

#### 5. Gemini Provider (`internal/llm/gemini.go`)

Simplified single-turn implementation:

```go
// GeminiProvider implements Provider using the Google AI API.
type GeminiProvider struct {
    client *genai.Client
    model  string
}

// NewGeminiProvider creates a provider using GOOGLE_API_KEY.
func NewGeminiProvider() (*GeminiProvider, error)

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    // Convert ToolDef to genai.FunctionDeclaration
    tools := p.convertTools(req.Tools)

    // Convert Message to genai.Content
    contents := p.convertMessages(req.Messages)

    // Configure model with system prompt and tools
    model := p.client.GenerativeModel(p.model)
    model.SystemInstruction = &genai.Content{Parts: []genai.Part{genai.Text(req.SystemPrompt)}}
    model.Tools = []*genai.Tool{{FunctionDeclarations: tools}}

    // Make single API call
    resp, err := model.GenerateContent(ctx, contents...)

    // Convert response to CompletionResponse
    return p.convertResponse(resp), err
}
```

Key implementation notes:
- Model: `gemini-2.0-flash` (cost-effective, supports function calling)
- Convert tool definitions to Gemini function declarations
- Map `functionCall` responses to common ToolCall type
- Track token usage from response metadata

#### 6. Error Sanitization (`internal/validate/sanitize.go`)

```go
// Sanitizer removes sensitive information from error messages.
type Sanitizer struct {
    maxLength      int
    homePatterns   []*regexp.Regexp
    ipPatterns     []*regexp.Regexp
    credPatterns   []*regexp.Regexp
}

// NewSanitizer creates a sanitizer with default patterns.
func NewSanitizer() *Sanitizer

// SanitizerOption configures a Sanitizer.
type SanitizerOption func(*Sanitizer)

// WithMaxLength sets the maximum output length.
func WithMaxLength(n int) SanitizerOption

// Sanitize cleans sensitive data from the input string.
func (s *Sanitizer) Sanitize(input string) string
```

Default patterns to redact:
- `/home/[^/\s]+` → `$HOME`
- `/Users/[^/\s]+` → `$HOME`
- `C:\\Users\\[^\\s]+` → `%USERPROFILE%`
- IPv4: `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}` → `[IP]`
- IPv6: common patterns → `[IP]`
- Credentials: `(api_key|token|password|secret|credential)s?[=:]\s*\S+` → `[REDACTED]`

#### 7. Error Parser (`internal/validate/errors.go`)

```go
// ErrorCategory classifies validation errors for targeted repair.
type ErrorCategory string

const (
    ErrorBinaryNotFound    ErrorCategory = "binary_not_found"
    ErrorExtractionFailed  ErrorCategory = "extraction_failed"
    ErrorVerifyFailed      ErrorCategory = "verify_failed"
    ErrorPermissionDenied  ErrorCategory = "permission_denied"
    ErrorDownloadFailed    ErrorCategory = "download_failed"
    ErrorUnknown           ErrorCategory = "unknown"
)

// ParsedError contains structured information about a validation failure.
type ParsedError struct {
    Category    ErrorCategory
    Message     string            // Sanitized error message
    Details     map[string]string // Extracted details (e.g., expected/actual binary name)
    Suggestions []string          // Potential fixes
}

// ParseValidationError analyzes validation output to categorize the failure.
func ParseValidationError(stdout, stderr string, exitCode int) *ParsedError
```

Example parsing:
- `command not found: mytool` → `ErrorBinaryNotFound` with suggestion "Check binary name in recipe"
- `tar: Error opening archive` → `ErrorExtractionFailed` with suggestion "Verify archive format"
- `permission denied` → `ErrorPermissionDenied` with suggestion "Check file permissions"

### Data Flow

#### Initial Generation (Happy Path)

```
1. GitHubReleaseBuilder.Build(ctx, req)

2. factory.GetProvider(ctx)
   → Claude breaker closed, return ClaudeProvider

3. builder.runConversationLoop(ctx, provider)  // Loop in builder
   │
   ├─ turn 1: provider.Complete(ctx, req)
   │  → LLM returns tool call (fetch_file)
   │  → builder executes tool, appends result
   │
   ├─ turn 2: provider.Complete(ctx, req)
   │  → LLM returns tool call (inspect_archive)
   │  → builder executes tool, appends result
   │
   └─ turn 3: provider.Complete(ctx, req)
      → LLM returns extract_pattern result
      → builder parses AssetPattern

4. executor.Validate(ctx, recipe, assetURL)
   → Container runs recipe
   → Return ValidationResult{Passed: true}

5. Return BuildResult with recipe
```

#### Repair Loop (Validation Failure)

```
1. Initial generation (steps 1-3 above)

2. executor.Validate(ctx, recipe, assetURL)
   → Return ValidationResult{Passed: false, Stdout: "...", Stderr: "..."}

3. Parse and sanitize error
   sanitizer.Sanitize(result.Stderr + result.Stdout)
   parseError := ParseValidationError(...)

4. Repair attempt 1 (continue conversation in builder)
   builder.appendErrorMessage(messages, errMsg)
   │
   └─ turn N: provider.Complete(ctx, req)  // Same provider, same interface
      → LLM receives: "Validation failed: [sanitized error]. Please fix..."
      → builder continues loop until extract_pattern
      → Return new AssetPattern

5. executor.Validate(ctx, newRecipe, assetURL)
   → Return ValidationResult{Passed: true}

6. Return BuildResult with repaired recipe
```

Note: The provider doesn't know about repairs - it just sees another Complete() call.
The builder owns the message history and adds error context as a user message.

#### Provider Failover

```
1. factory.GetProvider(ctx)
   → Claude breaker open (recent failures)
   → Gemini breaker closed
   → Return GeminiProvider

2. provider.GenerateRecipe(ctx, genReq)
   → Success

3. factory.ReportSuccess("gemini")

4. (Later) Claude breaker half-open after 60s
   → Next request tries Claude
   → Success → breaker closes
```

### Repair Prompt Template

When continuing conversation with error feedback:

```
The recipe you generated failed validation. Here is the error:

---
{sanitized_error}
---

Error analysis:
- Category: {error_category}
- Details: {parsed_details}

Please analyze what went wrong and call extract_pattern again with a corrected recipe.

Common fixes for {error_category}:
{suggestions}
```

### Telemetry Events

| Event | Fields | When |
|-------|--------|------|
| `llm_generation_started` | provider, tool_name, repo | Generation begins |
| `llm_generation_completed` | provider, success, cost, duration, attempts | Generation ends |
| `llm_repair_attempt` | provider, attempt_number, error_category | Repair starts |
| `llm_validation_result` | passed, error_category, attempt | Validation completes |
| `llm_provider_failover` | from_provider, to_provider, reason | Failover occurs |
| `llm_circuit_breaker_trip` | provider, failures | Breaker opens |

### Configuration

The provider factory auto-detects available providers:
- Claude: Available if `ANTHROPIC_API_KEY` is set
- Gemini: Available if `GOOGLE_API_KEY` is set

Provider preference (from parent design):
- Config: `[llm] provider = "claude"` (default)
- Env: `TSUKU_LLM_PROVIDER=gemini`
- Flag: `--provider gemini`

Repair settings:
- Max repair attempts: 2 (hardcoded for Slice 3)
- Move to config in Slice 4

## Security Considerations

### Error Sanitization Requirements

**Must redact before LLM transmission:**
- User home directory paths
- IP addresses (could reveal infrastructure)
- Credential patterns (api_key, token, password, secret)
- Environment variable values in error output

**Sanitization is defense in depth:**
- Container validation uses `--network=none` (no outbound)
- Sanitization protects against future changes
- LLM providers see only necessary debugging info

### Provider API Key Handling

- Each provider reads its API key from environment
- Keys never logged or included in error messages
- Factory doesn't store keys, only references to providers
- Circuit breaker state doesn't include sensitive data

### Conversation Privacy

Conversation objects contain:
- Message history (may include repo names, asset names)
- Token counts
- Provider identifier

Not included:
- API keys
- User identifying information
- Local file paths (sanitized in errors)

## Testing Strategy

### Unit Tests

**Provider Interface:**
- Mock provider implementation for builder tests
- Each provider has isolated unit tests
- No API calls in unit tests

**Circuit Breaker:**
- State transitions (closed -> open -> half-open -> closed)
- Failure counting and threshold behavior
- Recovery timeout behavior
- Concurrent access safety

**Error Sanitization:**
- Path redaction (Unix, Windows, macOS)
- IP address patterns (IPv4, IPv6)
- Credential patterns (various formats)
- Length truncation
- Combined patterns

**Error Parser:**
- Each error category has test cases
- Unknown errors handled gracefully
- Suggestions match categories

### Integration Tests

**Provider Parity:**
- Same inputs produce structurally equivalent outputs
- Both providers handle all tool types
- Both providers handle multi-turn conversations

**Repair Loop:**
- Intentionally broken recipe repairs successfully
- Max retries respected
- Sanitization verified in repair prompts

**Failover:**
- Mock provider failures trigger circuit breaker
- Failover to second provider succeeds
- Recovery after timeout

### Test Fixtures

Record/replay pattern from parent design:
- Record real API responses during development
- Replay in CI to avoid API costs
- Re-record periodically for API changes

## Exit Criteria

- [ ] Both Claude and Gemini produce working recipes through unified interface
- [ ] Circuit breaker correctly handles API failures and recovers
- [ ] Repair loops improve success rate (measure against Slice 2 baseline)
- [ ] Error sanitization removes all sensitive patterns before LLM calls
- [ ] Telemetry events emitted for all LLM operations
- [ ] Provider failover works correctly when primary is unavailable

## Consequences

### Positive

1. **Resilience**: Two providers means no single point of failure
2. **Better success rate**: Repair loops recover from many common failures
3. **Cost efficiency**: Conversation continuation cheaper than full restart
4. **Security**: Error sanitization prevents accidental data exposure
5. **Observability**: Telemetry enables data-driven improvement

### Negative

1. **Two provider implementations**: Must maintain Claude and Gemini adapters
2. **Testing burden**: Must verify parity across providers
3. **Conversation state**: Provider switching loses context
4. **Dependency**: Adds google/generative-ai-go dependency

### Mitigations

1. **Simplified providers**: Single-turn interface means providers are ~50 lines each (just type conversion)
2. **Parity testing**: Integration tests verify both providers produce equivalent results via same loop
3. **Fresh start acceptable**: Provider switch with full context still works (just costs more)
4. **Minimal dependency**: Only SDK needed, no additional abstractions

## Implementation Issues

### Milestone: LLM Builder Infrastructure

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#323](https://github.com/tsukumogami/tsuku/issues/323) | feat(llm): define provider interface and types | None |
| [#324](https://github.com/tsukumogami/tsuku/issues/324) | feat(llm): implement circuit breaker | None |
| [#325](https://github.com/tsukumogami/tsuku/issues/325) | feat(validate): implement error sanitizer | None |
| [#326](https://github.com/tsukumogami/tsuku/issues/326) | feat(validate): implement error parser | None |
| [#327](https://github.com/tsukumogami/tsuku/issues/327) | refactor(llm): extract Claude provider from client.go | #323 |
| [#328](https://github.com/tsukumogami/tsuku/issues/328) | feat(llm): implement Gemini provider | #323 |
| [#329](https://github.com/tsukumogami/tsuku/issues/329) | feat(llm): implement provider factory with failover | #323, #324 |
| [#330](https://github.com/tsukumogami/tsuku/issues/330) | feat(llm): add repair loop to GitHub Release Builder | #327, #329, #325, #326 |
| [#331](https://github.com/tsukumogami/tsuku/issues/331) | feat(telemetry): add LLM generation events | #330 |
| [#332](https://github.com/tsukumogami/tsuku/issues/332) | test(llm): add integration tests for provider parity and repair loop | #328, #330 |

### Dependency Graph

```
#323 Provider Interface ──────────────────────────────────────────┐
                                                                   │
#324 Circuit Breaker ─────────────────────────────────────────────┤
                                                                   │
#325 Error Sanitizer ─────────────────────────────────────────────┤
                                                                   │
#326 Error Parser ────────────────────────────────────────────────┤
                                                                   │
├── #327 Claude Provider (← #323) ────────────────────────────────┤
│                                                                  │
├── #328 Gemini Provider (← #323) ────────────────────────────────┤
│                                                                  │
├── #329 Factory (← #323, #324) ──────────────────────────────────┤
│                                                                  │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          v
        #330 Repair Loop (← #327, #329, #325, #326)
                          │
                          v
        #331 Telemetry (← #330)
        #332 Integration Tests (← #328, #330)
```
