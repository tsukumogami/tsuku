# Design Document: LLM Builder Productionization (Slice 4)

**Status**: Proposed

**Parent Design**: [DESIGN-llm-builder-infrastructure.md](docs/DESIGN-llm-builder-infrastructure.md)

**Issue**: [#270 - Slice 4: Productionize](https://github.com/tsukumogami/tsuku/issues/270)

## Context and Problem Statement

The LLM builder infrastructure is feature-complete through Slices 1-3:
- LLM client abstraction with Claude and Gemini support
- Container validation with repair loops
- GitHub Release Builder generating working recipes
- Basic CLI integration via `tsuku create --from github:owner/repo`

However, the feature is not production-ready. Users interacting with LLM-based generation face several gaps:

1. **No cost visibility**: Users cannot see how much LLM operations cost
2. **No cost controls**: No confirmation for expensive operations, no rate limiting, no budget enforcement
3. **No recipe preview**: LLM-generated recipes are written directly without user inspection
4. **No validation escape hatch**: Users without Docker cannot use the feature
5. **Poor error UX**: Error messages lack troubleshooting guidance
6. **No progress feedback**: Long-running generation shows no progress
7. **No checksum injection**: LLM-generated recipes lack download verification

This slice addresses these gaps to ship a production-ready LLM builder experience.

### Definition of Production Ready

For this feature to be considered production-ready:

| Criterion | Target |
|-----------|--------|
| Recipe success rate | 80% of generated recipes install successfully |
| Cost predictability | Actual cost within 2x of estimate (accounts for repair loops) |
| Maximum single-operation cost | $1.00 without explicit user confirmation |
| Daily cost cap | $5.00 default, configurable |
| User visibility | Users see cost, downloads, and verification status before approval |

**Scope note**: This ships as a GA feature, not experimental. Users should expect reliable operation within these parameters.

## Decision Drivers

- **User safety**: Cost and safety controls must be in place before exposing LLM features
- **Accessibility**: Users without Docker should still be able to use the feature (with warnings)
- **Transparency**: Users must see what will be installed before it happens
- **Leverage existing work**: Build on existing infrastructure (userconfig, telemetry, cost tracking)

## Upstream Design Reference

From [DESIGN-llm-builder-infrastructure.md](docs/DESIGN-llm-builder-infrastructure.md), Slice 4 specifies:

### Deliverables
- Update `tsuku create` to support `--from github`
- Register GitHub Release Builder in builder registry
- `--skip-validation` flag for users without Docker
- Configuration management (4-level: flags → env → file → defaults)
- `internal/secrets/manager.go` - API key resolution with 0600 permission enforcement
- Cost display after generation
- Confirmation prompt for operations >$0.50
- Rate limiting: max 10 LLM generations per hour
- Daily budget enforcement ($5 default)
- Recipe preview before installation (mandatory for LLM-generated recipes)
- Actionable error messages with troubleshooting guidance
- Progress indicators during generation

### Configuration Hierarchy
```
1. Command-line flags (--provider)
2. Environment variables (TSUKU_LLM_PROVIDER)
3. Config file ($TSUKU_HOME/config.toml)
4. Defaults (Claude)
```

## Current State Assessment

### Already Implemented (from Slices 1-3)

| Component | File | Notes |
|-----------|------|-------|
| LLM providers | `internal/llm/claude.go`, `gemini.go` | Both providers working |
| Provider factory | `internal/llm/factory.go` | Auto-detection from env vars |
| Circuit breaker | `internal/llm/breaker.go` | Per-provider failover |
| Cost tracking | `internal/llm/cost.go` | Token counting, USD calculation |
| GitHub builder | `internal/builders/github_release.go` | Full generation with repair loop |
| Container validation | `internal/validate/executor.go` | Docker/Podman abstraction |
| Error sanitization | `internal/validate/sanitize.go` | Safe error messages for LLM |
| User config | `internal/userconfig/userconfig.go` | TOML config, llm.enabled, llm.providers |
| Telemetry | `internal/telemetry/event.go` | LLM generation events |
| Create command | `cmd/tsuku/create.go` | Basic `--from github:` support |

### Gaps to Address

| Gap | Upstream Requirement | Implementation Needed |
|-----|---------------------|----------------------|
| Secrets manager | API key resolution with 0600 enforcement | New `internal/secrets/` package |
| Cost display | Show cost after generation | Extend create command output |
| Cost confirmation | Prompt for operations >$0.50 | New confirmation logic |
| Rate limiting | Max 10 generations/hour | State file + enforcement |
| Daily budget | $5 default, configurable | State tracking + config |
| Recipe preview | Mandatory before install | New preview flow |
| Skip validation | `--skip-validation` flag | Add flag to create command |
| Progress indicators | Show progress during generation | Progress output |
| Error messages | Actionable with troubleshooting | Error message templates |
| Checksum injection | Inject checksums from validation | Extend recipe generation |

## Considered Options

### Decision 1: Secrets Management Approach

**Option 1A: Environment Variables Only (Current)**

Continue requiring `ANTHROPIC_API_KEY` and `GOOGLE_API_KEY` in environment.

- **Pros**: Already implemented, no new code, standard practice
- **Cons**: No config file option, no permission enforcement, no guidance for users

**Option 1B: Config File with Permission Enforcement**

Add `internal/secrets/manager.go` that reads API keys from:
1. Environment variables (existing, takes priority if both set)
2. Config file `$TSUKU_HOME/config.toml` section `[secrets]`
3. Error with guidance if not found

Config file must have 0600 permissions or tighter. Use atomic file creation with `os.OpenFile(..., 0600)` to prevent race conditions.

- **Pros**: Flexible, enforces security best practice, better UX
- **Cons**: More complex, potential for users to store secrets in less-secure config, backup/git exposure risk

**Option 1C: External Secret Managers (keychain, pass)**

Integrate with system keychains or password managers.

- **Pros**: Best security practice
- **Cons**: Platform-specific, significant complexity, out of scope for MVP

### Decision 2: Rate Limiting Implementation

**Option 2A: File-Based State**

Track generation timestamps in `$TSUKU_HOME/state.json` (existing file). Enforce 10 generations per rolling 60-minute window (sliding window from current time, not calendar hour).

- **Pros**: Leverages existing state file, simple implementation
- **Cons**: Clock manipulation could bypass, per-machine limits (acceptable for CLI tool)

**State management details:**
- Timestamps older than 1 hour are pruned on each access
- Daily cost resets at UTC midnight
- Corrupted state file resets to empty (with warning)

**Option 2B: Server-Side Rate Limiting**

Require telemetry to track generations, enforce server-side.

- **Pros**: Harder to bypass
- **Cons**: Requires connectivity, breaks offline usage, privacy concerns

**Option 2C: No Rate Limiting (Honor System)**

Rely on cost display and confirmation to discourage overuse.

- **Pros**: Simpler implementation
- **Cons**: No protection against runaway scripts, accidental cost explosion

### Decision 3: Recipe Preview Format

**Option 3A: Minimal Summary**

Show only key information (downloads, binaries, verify command).

```
Generated recipe for gh:

  Downloads:
    - github.com/cli/cli/.../gh_2.42.0_linux_amd64.tar.gz

  Installs: gh
  Verifies: gh --version

Install? [y/N]
```

- **Pros**: Clean, easy to scan
- **Cons**: Hides details that might matter

**Option 3B: Full TOML Display**

Show the complete generated recipe.

```
Generated recipe:

[metadata]
name = "gh"
description = "GitHub CLI"
...

[install.steps]
...

Install? [y/N]
```

- **Pros**: Complete transparency
- **Cons**: Verbose, harder to understand for users unfamiliar with TOML

**Option 3C: Hybrid - Summary + Option for Full**

Show summary by default with option to see full recipe.

```
Generated recipe for gh:

  Downloads:
    - github.com/cli/cli/.../gh_2.42.0_linux_amd64.tar.gz

  Actions:
    1. Download release asset
    2. Extract tar.gz archive
    3. Install binary: gh

  Verification: gh --version

[v]iew full recipe, [i]nstall, [c]ancel?
```

- **Pros**: Balance of clarity and detail
- **Cons**: More complex UX, three options vs simple y/n

### Decision 4: Progress Indicator Style

**Option 4A: Spinner with Status**

```
Creating recipe for gh from github:cli/cli...
  ⠋ Fetching release metadata...
  ✓ Found release v2.42.0 with 24 assets
  ⠋ Analyzing assets with Claude...
  ✓ Asset matching complete
  ⠋ Validating recipe...
```

- **Pros**: Clear progress, professional look
- **Cons**: Requires terminal control, may not work in all environments

**Option 4B: Simple Line Progress**

```
Creating recipe for gh from github:cli/cli...
Fetching release metadata... done
Analyzing assets with Claude... done
Validating recipe... done
```

- **Pros**: Works everywhere, simple implementation
- **Cons**: Less polished

**Option 4C: Single Updating Line**

```
Creating recipe for gh... [Validating (attempt 1/3)]
```

- **Pros**: Compact
- **Cons**: Limited information, can confuse in terminal logs

### Decision 5: Failed Generation and Retry Behavior

**Option 5A: Failed Attempts Count Against Rate Limit**

All LLM API calls count against the hourly rate limit, including failed generations that trigger repair loops.

- **Pros**: Simple, predictable, prevents retry abuse
- **Cons**: Penalizes users with tools that are hard to validate

**Option 5B: Only Successful Generations Count**

Only completed (successful or user-cancelled) generations count. Failed attempts that retry don't increment the counter.

- **Pros**: Fair to users with difficult tools
- **Cons**: Could be exploited by deliberately failing and retrying

**Option 5C: Count Initial Attempts Only**

The initial generation attempt counts. Automatic repair loop retries (up to 3) don't count separately, but a new user-initiated `tsuku create` does.

- **Pros**: Balances fairness and abuse prevention
- **Cons**: More complex logic

## Decision Outcome

### Chosen Approach

**Secrets: 1B (Config File with Permission Enforcement)**

Rationale: Provides flexibility without adding significant complexity. Permission enforcement catches common security mistakes.

**Rate Limiting: 2A (File-Based State)**

Rationale: Simple, works offline, aligns with existing state file pattern. Clock manipulation is not a practical concern for a CLI tool.

**Recipe Preview: 3C (Hybrid Summary + Full Option)**

Rationale: Best balance of usability and transparency. Users can quickly scan the summary but drill down if needed.

**Progress: 4B (Simple Line Progress)**

Rationale: Reliable across all terminal environments. Can be enhanced later if needed.

**Retry Behavior: 5C (Count Initial Attempts Only)**

Rationale: Balances user fairness (repair loops are automatic, not user-initiated) with abuse prevention. A user-initiated `tsuku create` counts as one attempt regardless of how many repair loops it takes.

### Implementation Changes Summary

| Area | Change |
|------|--------|
| `internal/secrets/` | New package for API key resolution with 0600 enforcement |
| `internal/userconfig/` | Add rate limit, budget settings, config validation |
| `internal/state/` | Add LLM generation tracking with timestamp pruning |
| `internal/builders/` | Inject checksums from validation into recipes |
| `cmd/tsuku/create.go` | Add `--skip-validation`, `--yes`, preview flow, progress |
| Error templates | New actionable error messages with recovery guidance |

## Solution Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    cmd/tsuku/create.go                      │
│   ┌─────────────┬─────────────┬──────────────┬───────────┐ │
│   │ Flags       │ Preview     │ Progress     │ Errors    │ │
│   │ --skip-val  │ Recipe      │ Indicators   │ Actionable│ │
│   │ --force     │ Summary     │              │ Messages  │ │
│   └─────────────┴─────────────┴──────────────┴───────────┘ │
└────────────────────────────┬────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        ▼                    ▼                    ▼
┌──────────────┐   ┌──────────────────┐   ┌──────────────────┐
│ secrets/     │   │ userconfig/      │   │ state/           │
│ manager.go   │   │ (existing)       │   │ (existing)       │
│              │   │ + daily_budget   │   │ + llm_usage      │
│ - Env vars   │   │ + confirm_above  │   │ - timestamps     │
│ - Config     │   │                  │   │ - daily_cost     │
│ - 0600 check │   │                  │   │                  │
└──────────────┘   └──────────────────┘   └──────────────────┘
```

### 1. Secrets Manager (`internal/secrets/manager.go`)

```go
// Manager resolves API keys from multiple sources.
type Manager struct {
    configPath string
}

// Get retrieves an API key by name (e.g., "anthropic", "google").
// Resolution order:
// 1. Environment variable (ANTHROPIC_API_KEY, GOOGLE_API_KEY)
// 2. Config file $TSUKU_HOME/config.toml [secrets] section
// Returns error if not found or config has insecure permissions.
func (m *Manager) Get(name string) (string, error)

// RequireSecurePermissions checks config file is 0600 or tighter.
func RequireSecurePermissions(path string) error
```

Config file format (in `$TSUKU_HOME/config.toml`):
```toml
[secrets]
anthropic_api_key = "sk-ant-..."
google_api_key = "AIza..."
```

### 2. User Configuration Extensions

Add to `internal/userconfig/userconfig.go`:

```go
type LLMConfig struct {
    Enabled         *bool    `toml:"enabled,omitempty"`
    Providers       []string `toml:"providers,omitempty"`
    DailyBudget     *float64 `toml:"daily_budget,omitempty"`     // USD, default $5
    ConfirmAbove    *float64 `toml:"confirm_above,omitempty"`    // USD, default $0.50
    HourlyRateLimit *int     `toml:"hourly_rate_limit,omitempty"` // default 10
}

// Defaults
const (
    DefaultDailyBudget     = 5.0   // $5
    DefaultConfirmAbove    = 0.50  // $0.50
    DefaultHourlyRateLimit = 10
)
```

### 3. State Tracking Extensions

Add to `internal/state/state.go`:

```go
type LLMUsage struct {
    // Timestamps of recent generations (for rate limiting)
    GenerationTimestamps []time.Time `json:"generation_timestamps,omitempty"`

    // Daily cost tracking
    DailyCost     float64 `json:"daily_cost,omitempty"`
    DailyCostDate string  `json:"daily_cost_date,omitempty"` // YYYY-MM-DD
}

// RecordGeneration adds a generation timestamp and cost.
// Prunes timestamps older than 1 hour.
func (s *State) RecordGeneration(cost float64) error

// CanGenerate checks rate limit and daily budget.
// Returns (allowed, reason) where reason explains denial.
func (s *State) CanGenerate(config *userconfig.Config) (bool, string)

// DailySpent returns total cost for today.
func (s *State) DailySpent() float64
```

### 4. Create Command Enhancements

```go
// New flags
var (
    createFrom           string
    createForce          bool
    createSkipValidation bool  // NEW
    createAutoApprove    bool  // NEW: skip preview confirmation
)

func init() {
    createCmd.Flags().BoolVar(&createSkipValidation, "skip-validation", false,
        "Skip container validation (use when Docker is unavailable)")
    createCmd.Flags().BoolVar(&createAutoApprove, "yes", false,
        "Skip recipe preview confirmation")
}
```

### 5. Recipe Preview Flow

```go
func previewRecipe(recipe *recipe.Recipe, result *BuildResult) (approved bool, err error) {
    // Display summary
    fmt.Println("Generated recipe for", recipe.Metadata.Name+":")
    fmt.Println()

    // Show downloads with checksum status
    fmt.Println("  Downloads:")
    for _, dl := range extractDownloads(recipe) {
        checksumStatus := "verified"
        if dl.Checksum == "" {
            checksumStatus = "NO CHECKSUM"
        }
        fmt.Printf("    - %s [%s]\n", dl.URL, checksumStatus)
    }
    fmt.Println()

    // Show actions
    fmt.Println("  Actions:")
    for i, step := range recipe.Install.Steps {
        fmt.Printf("    %d. %s\n", i+1, describeStep(step))
    }
    fmt.Println()

    // Show verification
    if recipe.Verify != nil {
        fmt.Println("  Verification:", recipe.Verify.Command)
    }
    fmt.Println()

    // Show cost and warnings
    if result.Provider != "" {
        fmt.Printf("  LLM: %s (cost: $%.4f)\n", result.Provider, extractCost(result))
    }

    // Show validation metadata
    if result.ValidationSkipped {
        fmt.Println("  Warning: Validation was skipped (--skip-validation)")
    }
    if result.RepairAttempts > 0 {
        fmt.Printf("  Note: Recipe required %d repair attempt(s)\n", result.RepairAttempts)
    }

    for _, w := range result.Warnings {
        fmt.Println("  Warning:", w)
    }
    fmt.Println()

    // Prompt
    return promptForApproval()
}

func promptForApproval() (bool, error) {
    fmt.Print("[v]iew full recipe, [i]nstall, [c]ancel? ")
    // Read input and handle v/i/c
}
```

### 6. Progress Indicators

```go
func printProgress(stage string) {
    // Clear line and print status
    fmt.Printf("\r%s... ", stage)
}

func printProgressDone(stage string) {
    fmt.Printf("\r%s... done\n", stage)
}

// Usage in create command:
printProgress("Fetching release metadata")
releases, err := builder.FetchReleases(ctx, owner, repo)
printProgressDone("Fetching release metadata")

printProgress("Analyzing assets with LLM")
// ... LLM call
printProgressDone("Analyzing assets with LLM")
```

### 7. Actionable Error Messages

Create error templates:

```go
// internal/builders/errors.go

var ErrorNoReleases = ErrorTemplate{
    Summary: "No releases found for %q",
    Details: []string{
        "The repository has no GitHub releases",
        "Releases exist but contain no binary artifacts",
    },
    Suggestions: []string{
        "Check https://github.com/%s/releases",
        "Use --version flag to target a specific release",
    },
}

var ErrorValidationFailed = ErrorTemplate{
    Summary: "Validation failed after %d attempts",
    Details: []string{
        "Attempt %d: %s",
    },
    Suggestions: []string{
        "Review generated recipe at %s",
        "Edit the recipe manually and run 'tsuku install'",
        "Report issue at https://github.com/tsukumogami/tsuku/issues",
    },
}

var ErrorRateLimited = ErrorTemplate{
    Summary: "Rate limit exceeded",
    Details: []string{
        "%d generations in the last hour (limit: %d)",
    },
    Suggestions: []string{
        "Wait %s before the next generation",
        "Increase limit: tsuku config set llm.hourly_rate_limit 20",
    },
}

var ErrorBudgetExceeded = ErrorTemplate{
    Summary: "Daily budget exceeded",
    Details: []string{
        "Today's cost: $%.2f (budget: $%.2f)",
    },
    Suggestions: []string{
        "Wait until tomorrow for budget reset",
        "Increase budget: tsuku config set llm.daily_budget 10",
    },
}
```

### 8. Cost Confirmation Flow

```go
func confirmCostIfNeeded(estimatedCost float64, config *userconfig.Config) (bool, error) {
    threshold := config.ConfirmAbove()
    if estimatedCost <= threshold {
        return true, nil
    }

    fmt.Printf("Estimated cost: $%.4f (above $%.2f threshold)\n",
        estimatedCost, threshold)
    fmt.Print("Continue? [y/N] ")

    // Read y/n
}
```

## Implementation Plan

### Phase 1: Core Infrastructure

1. Create `internal/secrets/manager.go` with env + config resolution
2. Add permission checking for config file
3. Extend `internal/userconfig/` with LLM budget settings
4. Extend `internal/state/` with LLM usage tracking

### Phase 2: CLI Enhancements

1. Add `--skip-validation` flag to create command
2. Implement recipe preview flow
3. Add progress indicators
4. Implement cost display after generation

### Phase 3: Safety Controls

1. Implement rate limiting (check before generation)
2. Implement daily budget enforcement
3. Add cost confirmation prompt for high-cost operations

### Phase 4: Error UX

1. Create actionable error message templates
2. Update all error paths in create command
3. Add troubleshooting suggestions to common failures

### Testing Strategy

- **Unit tests**: Secrets manager, rate limiting logic, budget enforcement
- **Integration tests**: Full create flow with mock LLM responses
- **Manual testing**: End-to-end with real LLM providers

## Security Considerations

### Download Verification (CRITICAL)

**Risk**: LLM-generated recipes may lack checksum verification for downloaded binaries, enabling supply chain attacks.

**Mitigations**:
- Inject checksums captured during container validation into generated recipes
- Recipe preview prominently shows checksum status for each download
- Warn users if checksums are missing (only possible with `--skip-validation`)
- Document that checksums are required for production use

**Implementation**: The PreDownloader (Slice 2) already captures SHA256 checksums. These must be injected into the recipe before finalization.

### API Key Storage

**Risk**: API keys stored in config file could be exposed.

**Mitigations**:
- Use atomic file creation with `os.OpenFile(..., 0600)` to prevent race conditions
- Environment variables take priority and remain the recommended method
- Config file option is convenience, not recommendation
- Warn users to add config.toml to .gitignore
- Clear documentation warning about config file storage and backup exposure

### Cost Controls

**Risk**: Runaway scripts could accumulate significant costs.

**Mitigations**:
- Rate limiting (10/hour default, per user-initiated attempt)
- Daily budget ($5 default)
- Confirmation for operations >$0.50
- All limits configurable but with sensible defaults
- Actual cost displayed after generation (in addition to estimate before)

### Recipe Preview

**Risk**: Users might approve malicious recipes without reading.

**Mitigations**:
- Preview is mandatory (no bypass except --yes flag)
- Using `--yes` shows explicit warning about skipping review
- Summary highlights key actions (downloads, commands) and checksum status
- Full recipe viewable before approval
- Shows validation metadata (repair attempts, validation skipped)
- Documentation emphasizes importance of review

### Validation Escape Hatch

**Risk**: Users with `--skip-validation` install untested LLM output without checksums.

**Mitigations**:
- Require explicit consent when using `--skip-validation`:
  ```
  WARNING: Skipping validation. The recipe has NOT been tested.
  Risks: Binary path errors, missing extraction steps, failed verification, NO CHECKSUM VERIFICATION
  Continue without validation? (y/N)
  ```
- Add metadata to recipe: `llm_validation = "skipped"`
- Show warning during installation if recipe was not validated
- Document `--skip-validation` as debugging-only, not for production use

## Exit Criteria

### Functional Requirements
- [ ] `tsuku create <tool> --from github` works end-to-end
- [ ] Configuration hierarchy (flags → env → file → defaults) works correctly
- [ ] Cost is displayed after generation
- [ ] Confirmation required for operations >$0.50
- [ ] Rate limiting enforced (10/hour rolling window)
- [ ] Daily budget enforced ($5 default, resets at UTC midnight)
- [ ] Recipe preview shown before installation with checksum status
- [ ] `--skip-validation` flag works with consent flow
- [ ] Error messages are actionable with troubleshooting steps
- [ ] Progress indicators show during generation
- [ ] Checksums injected from validation into generated recipes

### Safety Requirements
- [ ] Rate limiting prevents >10 generations per rolling hour
- [ ] Config file permission check uses atomic creation (0600)
- [ ] `--skip-validation` requires explicit y/n consent
- [ ] `--yes` flag shows warning about skipping review
- [ ] State file corruption resets with warning (not silent)

### Testing Requirements
- [ ] Unit tests for secrets manager, rate limiting, budget enforcement
- [ ] Integration tests with mock LLM responses
- [ ] Concurrent access to state file doesn't corrupt data

## Consequences

### Positive
- Users have visibility into LLM costs before and after generation
- Rate limiting prevents accidental cost overruns
- Recipe preview enables informed decisions before installation
- Checksum injection provides supply chain protection
- Users without Docker can still use the feature (with appropriate warnings)

### Negative
- Additional confirmation steps slow down power users (mitigated by `--yes` flag)
- File-based rate limiting is per-machine, not per-user
- Cost estimation may differ from actual (repair loops add cost)
- `--skip-validation` reduces security guarantees

### Technical Debt
- External secret manager integration deferred (Option 1C)
- Server-side rate limiting not implemented (would require connectivity)
- No key rotation or expiration guidance
