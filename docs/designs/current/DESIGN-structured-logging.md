---
status: Current
problem: tsuku lacks structured diagnostic logging, making it difficult for users to troubleshoot installation failures without examining code or using ad-hoc debug patterns scattered across the codebase.
decision: Implement a unified Logger interface backed by Go stdlib slog, with subsystems receiving the logger via functional options, enabling debug and verbose output controlled by command-line flags.
rationale: This approach provides zero external dependencies while maintaining testability and matching the existing functional options pattern already established in internal/validate. slog's performance is adequate for CLI workloads where I/O dominates, and the interface design allows gradual migration of ~200+ existing fmt.Printf calls without requiring a big-bang rewrite.
---

## Status

Current

# DESIGN: Structured Logging Framework

## Context and Problem Statement

tsuku currently uses raw `fmt.Print*` calls throughout the codebase (approximately 275 occurrences across 39 files). This approach works for basic CLI output but creates several limitations:

**Missing diagnostic capabilities:** When users encounter installation issues, they have no way to see what tsuku is doing internally. There's no `--verbose` or `--debug` flag to help troubleshoot problems like failed downloads, checksum mismatches, or version resolution failures.

**Inconsistent output patterns:** The codebase mixes user-facing output with internal progress messages. Some code uses `fmt.Printf` to stdout for user messages, some uses `fmt.Fprintf(os.Stderr, ...)` for errors, and there's no clear convention for what constitutes "output" versus "logging."

**No structured context:** When reporting errors or progress, messages are plain text. Adding contextual information (tool name, version, action being performed) requires string formatting scattered throughout the code. This makes it harder to programmatically parse output or correlate related messages.

**Existing ad-hoc abstractions:** The `internal/validate` package has introduced local logger interfaces (`CleanupLogger`, `ExecutorLogger`) for testing and optional debug output. This pattern works but creates fragmentation as different subsystems may define their own incompatible logger interfaces.

**Why now:** As tsuku adds more complex installation flows (nix-portable, builder actions, recipe validation), the need for consistent diagnostic output grows. Users troubleshooting failed installations currently have to rely on guesswork or code reading.

**Current baseline behavior:** During a normal install, users see step-by-step progress messages (e.g., "Step 1/3: download", "Step 2/3: extract"). Errors print to stderr with suggestions. Download progress shows a terminal-aware progress bar. There's no way to see more detail without modifying code.

### Scope

**In scope:**
- Evaluating structured logging frameworks (slog, zerolog, zap, or custom)
- Defining what constitutes "user output" versus "debug/verbose logging"
- Designing a sustainable process for maintaining log coverage as code evolves
- Global verbose/debug flag support
- Consolidating existing ad-hoc logger interfaces

**Out of scope:**
- Changing user-facing output format (colors, progress bars)
- Machine-readable output modes (JSON output for automation)
- Log rotation or file-based logging (tsuku is a CLI tool, not a daemon)
- Metrics or telemetry (separate concern, already has telemetry service)

### Output vs. Logging Boundary

This design distinguishes between two types of output:

**User output (stdout):** Messages users expect to see during normal operation.
- Command results: tool lists, search results, version info
- Progress indicators: "Step 1/3: download", progress bars
- Success/completion messages: "Installed tool@version"
- These continue to use `fmt.Print*` to stdout

**Diagnostic logging (stderr):** Messages for troubleshooting.
- Debug: internal state, cache hits, version resolution details
- Info: operational context ("Using cached asset", "Connecting to registry")
- Warn: recoverable issues ("Checksum mismatch, re-downloading")
- Error: failures (already go to stderr via `errmsg.Fprint`)
- These use the structured logger to stderr, controlled by verbosity flags

The boundary: if removing the message would break a user's understanding of what happened, it's user output. If it only helps debug why something happened, it's logging.

## Decision Drivers

- **No external dependencies preferred:** tsuku aims to minimize external dependencies; Go 1.21+ stdlib `slog` is available
- **Zero-allocation hot paths:** Download and extraction operations should not incur logging overhead unless verbose mode is enabled
- **Clear user output semantics:** Users should see clean output by default; debug/verbose output should be opt-in
- **Sustainable evolution:** New code paths should naturally gain log coverage through clear conventions and optional linting
- **Testability:** Logging behavior should be testable without capturing stdout/stderr
- **Existing pattern compatibility:** Should work with the existing `CleanupLogger`/`ExecutorLogger` interface pattern in `internal/validate`

## Implementation Context

### Existing Patterns

**Logger interfaces in `internal/validate/`:**
- `CleanupLogger`: `Debug(msg string, args ...any)` - used for container cleanup
- `ExecutorLogger`: `Warn(msg string, args ...any)` + `Debug(msg string, args ...any)` - used for validation executor
- Both use noop implementations as defaults, injected via functional options

**Output helpers in `cmd/tsuku/helpers.go`:**
- `printInfo()` / `printInfof()`: Quiet-aware informational output
- `printError()`: Uses `errmsg.Fprint()` for formatted errors with suggestions
- `printJSON()`: Structured JSON output for `--json` flag
- Only `printInfo*` functions respect the `--quiet` flag

**Error formatting in `internal/errmsg/`:**
- `Suggester` interface allows errors to provide actionable hints
- Implemented by `version.ResolverError` and `registry.RegistryError`

**Progress tracking in `internal/progress/`:**
- Terminal-aware progress display with ETA
- Used by download and nix_portable actions

**Current limitations:**
- ~200+ direct `fmt.Printf()` calls bypass quiet mode
- No `--verbose` or `--debug` flags exist
- Logger interfaces not unified across packages
- Actions emit output unconditionally

### Go Logging Ecosystem (2024-2025)

| Library | Performance | Dependencies | Best For |
|---------|------------|--------------|----------|
| **slog** (stdlib) | ~650 ns/op | None (Go 1.21+) | Most CLIs |
| **zerolog** | ~174 ns/op, 0 alloc | External | High-throughput services |
| **zap** | ~300 ns/op | External | Highly customizable setups |

**Key insight:** For CLI tools, I/O latency dominates logging overhead. Zero-allocation matters for services processing millions of logs/second, not CLIs running a few hundred operations.

**CLI best practice:** Separate user output (stdout) from diagnostic logs (stderr). Popular CLIs like kubectl, docker, and gh follow this pattern.

### Applicable Conventions

**Unix CLI pattern:**
- User-facing output goes to stdout for piping: `tsuku list | grep tool`
- Diagnostic/debug logs go to stderr to avoid polluting pipes
- Verbosity levels: quiet → normal → verbose → debug

**slog structured logging:**
- Two-part design: Logger (frontend) + Handler (backend)
- TextHandler for human-readable, JSONHandler for machine-parseable
- HandlerOptions configure minimum level, source info, attribute transformation

### Industry CLI Patterns (gh, kubectl, docker)

Research into three prominent Go CLIs reveals distinct approaches:

| CLI | Logging Library | Debug Mechanism | Output Separation |
|-----|-----------------|-----------------|-------------------|
| **gh** | None (IOStreams pattern) | `GH_DEBUG` env var | stdout=data, stderr=messages |
| **kubectl** | klog (Kubernetes fork of glog) | `-v=0-9` numeric levels | stdout=user output, stderr=diagnostics |
| **docker** | logrus (structured logging) | `--log-level` flag | streams package (`Out`, `Err`) |

**Key insights:**

1. **gh uses no logging library** - Instead uses an IOStreams abstraction that wraps stdout/stderr with terminal capability detection. Debug output enabled via environment variable rather than flags.

2. **kubectl uses numeric verbosity** - Levels 0-9 with progressively more detail. Levels 6-9 show HTTP traffic. Uses klog's `V()` pattern for conditional logging that avoids evaluation overhead when disabled.

3. **docker uses logrus** - Structured field-based logging. Streams package provides testable I/O abstraction. Strong emphasis on `--password-stdin` pattern for credentials.

**Common patterns across all three:**
- User output goes to stdout for piping (`cmd | grep`)
- Diagnostic/debug logs go to stderr
- Verbosity controlled by flags or environment variables
- Streams abstraction for testable output
- Sensitive data protection (avoid logging credentials)

**Implications for tsuku:**
- Our choice of slog (stdlib) is consistent with the trend toward structured logging
- IOStreams-style output abstraction is a proven pattern (gh, docker)
- Numeric verbosity (kubectl-style) vs named levels (docker-style) - both work; we're choosing named flags for simplicity
- Environment variable support (`TSUKU_DEBUG`) could complement flags

## Considered Options

This design addresses two related decisions: (1) which logging framework to adopt, and (2) how to ensure log coverage remains consistent as code evolves.

### Decision 1: Logging Framework

#### Option 1A: Go stdlib slog

Use Go 1.21+ standard library structured logging (`log/slog`).

**Pros:**
- Zero external dependencies (aligns with tsuku philosophy)
- Stable stdlib API with long-term support
- Built-in TextHandler and JSONHandler
- Compatible with existing logger interface pattern (can wrap slog.Logger)
- Sufficient performance for CLI workloads (~650 ns/op)

**Cons:**
- Slightly slower than zerolog/zap (3-4x, but CLI I/O dominates)
- Less flexible handler customization than third-party libraries
- Relatively new (Go 1.21, Oct 2023) - fewer community examples

#### Option 1B: OpenTelemetry Logging

Use OpenTelemetry Go SDK for structured logging with semantic conventions.

**Pros:**
- Industry-standard semantic conventions for log attributes
- Designed for correlation with traces/metrics if added later
- Strong ecosystem momentum and vendor support
- Built-in batching and export pipeline
- Consistent with observability best practices

**Cons:**
- Significant external dependency (`go.opentelemetry.io/otel` ecosystem)
- Heavier runtime footprint than slog
- Designed for services, not CLI tools (batching, async export)
- Log bridge API is relatively new (stable as of 2024)
- Overkill for a CLI that runs for seconds

#### Option 1C: zerolog

Use zerolog for zero-allocation structured logging.

**Pros:**
- Fastest Go logging library (~174 ns/op, 0 allocations)
- Fluent API enables compile-time optimization
- Popular in performance-critical Go services
- Good JSON output by default

**Cons:**
- External dependency
- Zero-allocation benefits don't matter for CLI tools (I/O dominates)
- JSON-first design less ideal for human-readable CLI output
- Fluent API style differs from slog/OTel conventions

### Decision 2: Sustainability Process

#### Option 2A: Documentation-Only Convention

Establish logging conventions through documentation and code review.

**Pros:**
- No tooling overhead
- Flexible - conventions can evolve easily
- Low barrier to adoption

**Cons:**
- Relies entirely on human discipline
- No automated enforcement
- New code paths may miss log coverage silently
- Inconsistencies accumulate over time

#### Option 2B: Logger Interface Pattern

Require all subsystems to accept a logger interface (like existing `CleanupLogger`).

**Pros:**
- Enforced through API design
- Testable - can inject mock loggers
- Consistent with existing pattern in `internal/validate`
- Makes missing logger obvious (function signature requires it)

**Cons:**
- Increases function parameter counts
- Some simple functions may not need logging
- Requires discipline to add logger parameter to new code

#### Option 2C: Context-Based Logger

Attach logger to `context.Context` and extract in functions that need it.

**Pros:**
- Logger available throughout call stack without explicit parameters
- Preserves clean function signatures
- Can carry request-scoped context (tool name, version being installed)
- Works well with slog (slog.Default() can be context-aware)

**Cons:**
- Implicit dependency (logger may or may not be in context)
- Requires discipline to propagate context correctly
- Context pollution concern (too much attached to ctx)
- Harder to see at a glance what functions log

#### Option 2D: Centralized Logger with Code Review Gate

Use a package-level logger that new code must use, enforced via code review checklist.

**Pros:**
- Simple to use (import package, call logger)
- Single point of configuration
- Code review catches missing logs

**Cons:**
- Still relies on human review
- Global mutable state (package-level logger)
- Testing requires careful setup/teardown
- No compile-time guarantees

#### Option 2E: Hybrid Interface + Concrete Logger

Define a unified logger interface with a slog-backed default implementation. Subsystems accept the interface via functional options (matching existing pattern in `internal/validate`).

**Pros:**
- Testable - inject mock logger in tests
- Matches established pattern in `internal/validate`
- API design enforces logger availability where needed
- Concrete default means callers don't need to provide logger
- Gradual migration - can update one subsystem at a time

**Cons:**
- Adds interface definition and implementation code
- Functions that log must accept options or interface parameter
- Need to define which subsystems "need" logging

### Evaluation Against Decision Drivers

| Driver | 1A: slog | 1B: OTel | 1C: zerolog |
|--------|----------|----------|-------------|
| No external deps | **Good** | Poor | Fair |
| Zero-alloc hot paths | Fair | Fair | **Good** |
| Clear output semantics | **Good** | **Good** | Fair |
| Testability | **Good** | **Good** | **Good** |
| Existing pattern compat | **Good** | Fair | Fair |

| Driver | 2A: Docs | 2B: Interface | 2C: Context | 2D: Centralized | 2E: Hybrid |
|--------|----------|---------------|-------------|-----------------|------------|
| Sustainable evolution | Poor | **Good** | Fair | Fair | **Good** |
| Testability | Fair | **Good** | Fair | Fair | **Good** |
| Pattern compatibility | **Good** | **Good** | Fair | Fair | **Good** |

### Assumptions

- **Backward compatibility:** All changes are internal. Users see the same output unless they opt into verbose/debug mode.
- **Gradual migration:** We will migrate one subsystem at a time rather than a big-bang rewrite. The ~200+ `fmt.Printf` calls will be addressed incrementally.
- **I/O dominance:** Logging performance is not a concern because network/disk I/O dominates CLI execution time. The ~650 ns/op of slog vs ~174 ns/op of zerolog is negligible.
- **Flag semantics:** Verbosity flags will follow the pattern: `--quiet` (errors only), default (user output), `--verbose` (+ info logs), `--debug` (+ debug logs).

### Uncertainties

- **OTel log bridge maturity:** The OpenTelemetry logs API became stable in 2024, but CLI-specific usage patterns are less documented than service patterns.
- **slog adoption in CLI tools:** While slog is well-suited for CLIs, most examples focus on web services. We haven't found extensive CLI-specific patterns.
- **Interface proliferation:** Adding logger interfaces to all functions may feel heavy-handed for a CLI codebase of this size.
- **Context propagation:** We don't currently use `context.Context` consistently throughout the codebase; adopting context-based logging would require broader changes.

## Decision Outcome

**Chosen: 1A (slog) + 2E (Hybrid Interface + Concrete Logger)**

### Summary

Use Go stdlib `slog` for structured logging with a unified logger interface backed by slog. Subsystems receive the logger via functional options (matching the established pattern in `internal/validate`). This gives us zero external dependencies, testable code, and a gradual migration path.

### Rationale

**Why slog over OTel (1B):**
- tsuku is a CLI tool that runs for seconds, not a long-running service. OTel's design for batching, async export, and service observability adds complexity without benefit.
- The dependency footprint of `go.opentelemetry.io/otel` is substantial for a tool that aims to minimize dependencies.
- If tsuku ever needs distributed tracing or metrics correlation, slog can be bridged to OTel via a custom handler later. The abstraction layer we're building allows this evolution.
- slog is stdlib: long-term stability, no version conflicts, no supply chain concerns.

**Why slog over zerolog (1C):**
- Zero-allocation benefits are irrelevant when I/O dominates execution time. The ~4x performance difference disappears into network/disk latency.
- zerolog's fluent API and JSON-first design don't match slog's conventions or the existing interface patterns.
- External dependency with no meaningful benefit for CLI workloads.

**Why hybrid interface (2E) over other sustainability options:**
- Matches the established pattern in `internal/validate` (CleanupLogger, ExecutorLogger).
- API design enforces logger availability - missing logger is a compile error, not a runtime surprise.
- Functional options keep function signatures clean while allowing injection.
- Concrete default means most callers don't need to think about logging setup.
- Gradual migration: we can update one package at a time.

### Trade-offs Accepted

By choosing slog + hybrid interface:

- **Performance gap vs zerolog:** We accept ~4x slower logging operations (~650 ns vs ~174 ns). This is acceptable because CLI I/O dominates; logging overhead is noise.
- **No OTel semantic conventions:** We won't have industry-standard attribute names out of the box. This is acceptable because tsuku is a local CLI tool, not a distributed service requiring cross-system correlation.
- **Interface maintenance:** We commit to maintaining a logger interface and slog adapter. This is acceptable because the interface is small (Debug, Info, Warn) and stable.
- **Gradual migration effort:** We'll migrate ~200+ `fmt.Printf` calls incrementally. This is acceptable because it reduces risk and allows learning.

## Solution Architecture

### Overview

The solution introduces a unified `Logger` interface in `internal/log` backed by Go's `slog` package. Subsystems opt into logging by accepting a `Logger` via functional options. A global logger configured at startup handles the common case; tests inject mock loggers.

```
┌─────────────────┐     ┌─────────────────┐
│  cmd/tsuku/     │     │  internal/log   │
│  main.go        │────▶│  Logger iface   │
│  (configures    │     │  + slog impl    │
│   log level)    │     └────────┬────────┘
└─────────────────┘              │
                                 │ injected via
                                 │ functional options
                    ┌────────────┴────────────┐
                    ▼                         ▼
          ┌─────────────────┐       ┌─────────────────┐
          │ internal/actions│       │ internal/install│
          │ (uses logger)   │       │ (uses logger)   │
          └─────────────────┘       └─────────────────┘
```

### Components

**`internal/log/logger.go`** - Logger interface and slog implementation:
```go
// Logger is the interface for structured logging.
// Methods match slog's signature for easy integration.
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger  // Returns logger with additional context
}

// New creates a Logger backed by slog with the given handler.
func New(h slog.Handler) Logger

// NewNoop returns a logger that discards all output (for testing).
func NewNoop() Logger

// Default returns the global logger configured at startup.
func Default() Logger

// SetDefault sets the global logger (called once in main).
func SetDefault(l Logger)
```

**`internal/log/handler.go`** - CLI-friendly slog handler:
```go
// NewCLIHandler creates a slog.Handler for CLI output.
// - Writes to stderr
// - Human-readable format (not JSON)
// - Includes timestamp only in debug mode
// - Omits source location unless debug
func NewCLIHandler(level slog.Level, opts ...HandlerOption) slog.Handler
```

**Subsystem integration pattern** (example from actions):
```go
// ActionOption configures action execution.
type ActionOption func(*actionConfig)

// WithLogger sets the logger for action execution.
func WithLogger(l log.Logger) ActionOption {
    return func(c *actionConfig) { c.logger = l }
}

// Execute runs the action with configured options.
func (a *DownloadAction) Execute(ctx context.Context, opts ...ActionOption) error {
    cfg := defaultConfig() // includes log.Default()
    for _, opt := range opts {
        opt(&cfg)
    }
    cfg.logger.Debug("starting download", "url", a.URL)
    // ...
}
```

**ExecutionContext integration:**
The existing `ExecutionContext` struct (used by 80+ actions) will gain a `Logger` field:
```go
type ExecutionContext struct {
    // existing fields...
    Logger log.Logger  // Falls back to log.Default() if nil
}
```
This makes the logger available to all actions without changing their function signatures.

### Key Interfaces

**Logger interface alignment:**
The interface mirrors slog's method signatures so the implementation is a thin wrapper:
- `Debug(msg string, args ...any)` - maps to `slog.Logger.Debug`
- `Info(msg string, args ...any)` - maps to `slog.Logger.Info`
- `Warn(msg string, args ...any)` - maps to `slog.Logger.Warn`
- `Error(msg string, args ...any)` - maps to `slog.Logger.Error`
- `With(args ...any) Logger` - creates a child logger with context

**Verbosity levels:**
```
Level     Flag              Env Var            Shows
──────────────────────────────────────────────────────────────
ERROR     --quiet           TSUKU_QUIET=1      Errors only
WARN      (default)         -                  + Warnings + user output
INFO      --verbose         TSUKU_VERBOSE=1    + Info messages
DEBUG     --debug           TSUKU_DEBUG=1      + Debug details
```

**Environment variable support:** Following the gh CLI pattern, verbosity can also be controlled via environment variables. This is useful in CI environments where modifying command-line flags may be inconvenient. Flags take precedence over environment variables.

### Data Flow

1. **Startup:** `cmd/tsuku/main.go` parses `--quiet`/`--verbose`/`--debug` flags
2. **Configuration:** Determines log level, creates handler, calls `log.SetDefault()`
3. **Execution:** Commands call into internal packages
4. **Logging:** Internal code calls `log.Default().Info(...)` or uses injected logger
5. **Output:** Handler writes to stderr in human-readable format

### Migration Compatibility

**Existing logger interfaces** (`CleanupLogger`, `ExecutorLogger`) will be updated to embed the new `Logger` interface. Existing code continues to work with a simple adapter:

```go
// ExecutorLogger extends Logger with validation-specific methods.
type ExecutorLogger interface {
    log.Logger
    // Any validation-specific methods if needed
}
```

## Implementation Approach

### Phase 1: Core logging infrastructure

Create `internal/log` package with Logger interface and slog implementation.
- Define Logger interface
- Implement slog-backed logger
- Create CLI-friendly handler
- Add global logger (Default/SetDefault)
- Wire up `--quiet`, `--verbose`, `--debug` flags in main

**Deliverable:** Logging infrastructure works end-to-end; no existing code changed yet.

### Phase 2: Migrate internal/validate

Update `CleanupLogger` and `ExecutorLogger` to use the new Logger interface.
- Replace custom interfaces with `log.Logger`
- Update functional options to use `log.WithLogger`
- Add debug logging to validation flow

**Deliverable:** Existing logger interface pattern migrated; validates the approach.

### Phase 3: Migrate internal/actions

Add logging to action execution. Focus on high-value actions first:
- `download.go`: Log URL, cache status, checksum validation
- `extract.go`: Log archive type, destination
- `install_binaries.go`: Log binary paths, symlink creation

**Deliverable:** Installation troubleshooting significantly improved.

### Phase 4: Migrate remaining subsystems

Extend to other internal packages as needed:
- `internal/install`: Installation manager
- `internal/executor`: Recipe execution
- `internal/version`: Version resolution

**Deliverable:** Comprehensive debug output available throughout.

### Sustainability Process

**For new code:**
1. Functions that perform I/O or complex operations should accept `log.Logger` via options
2. Use `log.Default()` as the default when no logger is injected
3. Log at appropriate level: Debug for internal state, Info for operations, Warn for recoverable issues

**For code review:**
- Reviewer asks: "Does this code path benefit from debug logging?"
- If yes, ensure logger is available and used

**No automated linting:** The interface pattern makes missing loggers visible in function signatures. We rely on code review rather than linters.

## Security Considerations

### Download Verification

**Not applicable.** This feature does not download external artifacts. The logging framework is implemented entirely using Go's stdlib (`log/slog`), which is part of the Go distribution already present on the system. No network requests are made to fetch logging-related code or dependencies.

### Execution Isolation

**No new permissions required.** The logging feature:
- Writes to stderr (already permitted)
- Does not access additional filesystem locations
- Does not make network requests
- Does not require elevated privileges
- Does not spawn processes

The logging infrastructure runs in the same security context as existing tsuku code with no privilege escalation.

### Supply Chain Risks

**Minimal risk - stdlib only.** By choosing Go's stdlib `slog` over external dependencies (zerolog, zap, OTel), we eliminate supply chain risk for the logging framework itself:
- No external packages to audit
- No external repositories that could be compromised
- No version pinning concerns
- slog is maintained by the Go team with the same security standards as the language

This was a key driver for choosing slog over alternatives.

### User Data Exposure

**Potential risk: sensitive data in debug logs.**

Debug logs may inadvertently capture sensitive information:
- URLs (could contain tokens in query parameters, path segments, or Basic Auth)
- File paths (could reveal user directory structure)
- Environment variables (could contain secrets if logged)
- Recipe contents (could contain private registry URLs with credentials)

**Mitigations:**

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Tokens in URLs | Programmatic URL sanitizer (see below) | User must not share debug logs publicly |
| Secrets in env vars | Programmatic enforcement: never log env var values | None |
| File path exposure | Acceptable - paths are necessary for troubleshooting | User shares debug logs intentionally |
| Debug logs shared publicly | Display warning banner when `--debug` is enabled | User awareness |
| Recipe credentials | Sanitize URL fields in recipe structs before logging | None |

**Required implementation: URL sanitizer**

The `internal/log` package must include a URL sanitizer that handles all credential patterns:
```go
// SanitizeURL removes credentials from URLs for safe logging.
// Handles: query params, Basic Auth, path tokens
func SanitizeURL(url string) string
```

Patterns to sanitize:
- Query parameters: `?token=abc` → `?token=REDACTED`
- Basic Auth: `https://user:pass@host` → `https://REDACTED@host`
- Path tokens: `https://api.example.com/download/token123/file` (heuristic detection)

**Required implementation: debug mode warning**

When `--debug` is enabled, display a banner before any output:
```
[DEBUG MODE] Output may contain file paths and URLs. Do not share publicly.
```

**Implementation guidelines:**
1. Never log environment variable values - use blocklist for known secret vars (`GITHUB_TOKEN`, `ANTHROPIC_API_KEY`, etc.)
2. Always use `SanitizeURL()` when logging URLs
3. Sanitize recipe URL fields before logging recipe contents
4. Escape special characters in logged strings to prevent log injection

### Summary

This feature has low security impact because it:
1. Uses only stdlib (no supply chain exposure)
2. Requires no new permissions
3. Downloads nothing

The main consideration is preventing sensitive data leakage in debug output, mitigated through logging hygiene guidelines and user awareness.

## Consequences

### Positive

- **Zero external dependencies:** stdlib slog means no version management or supply chain concerns
- **Improved troubleshooting:** Users can enable `--debug` to see what tsuku is doing
- **Testable logging:** Tests can inject mock logger and verify log calls
- **Matches existing pattern:** Builds on established functional options pattern in `internal/validate`
- **Gradual migration:** No big-bang rewrite; migrate one package at a time
- **Future-proof:** If we ever need OTel, a custom slog handler bridges the gap

### Negative

- **API surface increase:** New `internal/log` package and Logger interface to maintain
- **Migration effort:** ~200+ `fmt.Printf` calls need eventual review and migration
- **Not OTel-native:** If tsuku ever becomes a service or needs distributed tracing, we'll need additional work

### Mitigations

- **API surface:** Interface is intentionally small (4 methods + With). Keep it stable.
- **Migration:** Phase the work. High-value targets first (actions, install). Rest can be opportunistic.
- **OTel compatibility:** slog's handler abstraction allows bridging to OTel later via custom handler.

## Implementation Issues

### Milestone: [Structured Logging Framework](https://github.com/tsukumogami/tsuku/milestone/16)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [#417](https://github.com/tsukumogami/tsuku/issues/417) | feat(log): add Logger interface and slog implementation | None | critical |
| [#418](https://github.com/tsukumogami/tsuku/issues/418) | feat(log): add URL sanitizer for safe logging | None | testable |
| [#419](https://github.com/tsukumogami/tsuku/issues/419) | feat(log): add CLI handler with verbosity support | [#417](https://github.com/tsukumogami/tsuku/issues/417) | testable |
| [#420](https://github.com/tsukumogami/tsuku/issues/420) | refactor(validate): migrate to unified Logger interface | [#417](https://github.com/tsukumogami/tsuku/issues/417) | simple |
| [#421](https://github.com/tsukumogami/tsuku/issues/421) | feat(cli): add verbosity flags and environment variable support | [#417](https://github.com/tsukumogami/tsuku/issues/417), [#419](https://github.com/tsukumogami/tsuku/issues/419) | testable |
| [#422](https://github.com/tsukumogami/tsuku/issues/422) | feat(actions): add ExecutionContext logger and migrate high-value actions | [#417](https://github.com/tsukumogami/tsuku/issues/417), [#418](https://github.com/tsukumogami/tsuku/issues/418), [#421](https://github.com/tsukumogami/tsuku/issues/421) | milestone |
