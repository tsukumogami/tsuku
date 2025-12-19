# Design: Plan-Based Installation

- **Status**: Implemented (Milestone M15)
- **Milestone**: Deterministic Recipe Execution
- **Author**: @dangazineu
- **Created**: 2025-12-13
- **Scope**: Tactical
- **Archived**: 2025-12-19
- **See Also**: docs/GUIDE-plan-based-installation.md (user guide)

## Implementation Issues

### Milestone: [Deterministic Recipe Execution](https://github.com/tsukumogami/tsuku/milestone/15)

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#506](https://github.com/tsukumogami/tsuku/issues/506) | feat(cli): add plan loading utilities for external plans | None |
| [#507](https://github.com/tsukumogami/tsuku/issues/507) | feat(cli): add --plan flag to install command | [#506](https://github.com/tsukumogami/tsuku/issues/506) |
| [#508](https://github.com/tsukumogami/tsuku/issues/508) | docs(cli): document plan-based installation workflow | [#507](https://github.com/tsukumogami/tsuku/issues/507) |

### Dependency Graph

```mermaid
graph LR
    I506["#506: plan loading utilities"]
    I507["#507: --plan flag"]
    I508["#508: documentation"]

    I506 --> I507
    I507 --> I508

    classDef done fill:#c8e6c9
    classDef ready fill:#bbdefb
    classDef blocked fill:#fff9c4
    classDef needsDesign fill:#e1bee7

    class I506 done
    class I507 done
    class I508 done
```

**Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design

## Upstream Design Reference

This design implements Milestone 3 of [DESIGN-deterministic-resolution.md](DESIGN-deterministic-resolution.md).

**Relevant sections:**
- Vision: "A recipe is a program that produces a deterministic installation plan"
- Milestone 3: Plan-Based Installation
- Integration: Air-gapped container execution alignment

**Prerequisite work completed:**
- Milestone 1 (#367): Installation plans and `tsuku eval` command
- Milestone 2 (#368): Deterministic execution with plan caching
- Decomposable Actions (#436-#449): Plans contain only primitive operations

## Context and Problem Statement

Tsuku now has a two-phase installation architecture where all installations go through plan generation and execution. The `tsuku eval` command generates plans, `tsuku plan export` exports stored plans, and `ExecutePlan()` executes plans with checksum verification.

However, there is currently no way to install from an externally-provided plan. Users can generate and export plans, but cannot use them for installation. This creates several limitations:

1. **Air-gapped environments**: Organizations with isolated networks cannot leverage pre-computed plans. They must either allow network access during installation or manually replicate the entire recipe resolution process.

2. **CI distribution**: Build pipelines cannot generate plans centrally and distribute them to build nodes. Each node must independently resolve versions and compute checksums, wasting time and creating potential inconsistencies.

3. **Team standardization**: Teams cannot share exact installation specifications. Even with version pinning, minor recipe changes or network timing can produce different installations.

The upstream design explicitly calls for `tsuku install --plan <file>` as the final deliverable of the deterministic execution milestone, completing the vision that "a recipe is a program that produces a deterministic installation plan."

### Scope

**In scope:**
- `tsuku install --plan <file>` to install from a plan file
- Support for stdin: `tsuku eval tool | tsuku install --plan -`
- Checksum verification for plan-based installation
- Clear error handling for invalid or incompatible plans

**Out of scope:**
- Plan signing or cryptographic verification (future security enhancement)
- Multi-tool plans (plans are single-tool by design)
- Plan format migration from older versions (format version 2 is stable)
- Lock files for team coordination (tracked separately)
- Dependency resolution from plans (plan-based installation assumes dependencies are already installed via normal flow)

## Decision Drivers

1. **Architectural equivalence**: `tsuku install foo` and `tsuku eval foo | tsuku install --plan -` must produce identical installed results
2. **Offline capability**: Plan-based installation should work offline when artifacts are pre-cached
3. **Safety**: Invalid or incompatible plans must fail clearly, not silently produce wrong results
4. **Simplicity**: Minimal changes to existing infrastructure; reuse `ExecutePlan()`
5. **Unix philosophy**: Support piping for composability with other tools

## Considered Options

### Decision 1: Plan Input Method

#### Option 1A: File Path Only

Accept only file paths for plan input.

```bash
tsuku install --plan plan.json
```

**Pros:**
- Simple implementation (just read file)
- Clear semantics

**Cons:**
- Requires intermediate file for piping (`tsuku eval tool > plan.json && tsuku install --plan plan.json`)
- Doesn't support streaming workflows
- Extra disk I/O

#### Option 1B: File Path with Stdin Support

Accept file path OR `-` for stdin.

```bash
tsuku install --plan plan.json    # from file
tsuku eval tool | tsuku install --plan -  # from pipe
```

**Pros:**
- Supports both batch and streaming workflows
- Follows Unix convention (`-` for stdin)
- No intermediate files needed
- Enables `curl ... | tsuku install --plan -` patterns

**Cons:**
- Slightly more complex (must detect stdin mode)
- Stdin can only be read once (no retry on parse failure)

### Decision 2: Plan Validation Strategy

#### Option 2A: Minimal Validation (Format Only)

Validate that the JSON parses and has required fields. Trust the plan content.

**Pros:**
- Simple implementation
- Fast validation
- Supports hand-edited plans

**Cons:**
- Platform mismatches discovered during execution (late failure)
- Checksum failures provide cryptic errors
- May partially install before failing

#### Option 2B: Comprehensive Pre-Execution Validation

Validate format, platform compatibility, format version, and primitive-only steps before execution begins.

**Pros:**
- Fails fast with clear error messages
- No partial installation on validation failure
- Catches stale/incompatible plans immediately
- Better user experience

**Cons:**
- More validation code
- Slightly slower startup (negligible for file I/O)

### Decision 3: Tool Name Handling

When a plan is provided, the tool name could come from:
- Command line: `tsuku install ripgrep --plan plan.json`
- Plan file: plan contains `"tool": "ripgrep"`
- Both (must match or error)

#### Option 3A: Tool Name from Plan Only

Tool name comes exclusively from the plan. Command line tool name is ignored or disallowed.

```bash
tsuku install --plan plan.json  # tool name in plan
```

**Pros:**
- Plan is self-contained
- No mismatch possible
- Simpler command syntax

**Cons:**
- Different syntax than normal install (`tsuku install <tool>` vs `tsuku install --plan`)
- User must inspect plan to know what will be installed
- Doesn't match existing install command structure

#### Option 3B: Tool Name Required, Must Match Plan

Tool name required on command line, must match plan's tool field.

```bash
tsuku install ripgrep --plan plan.json  # must match plan
```

**Pros:**
- Explicit about what's being installed
- Catches accidental wrong-plan usage
- Consistent with normal install command structure

**Cons:**
- Redundant information (tool name in two places)
- Error on mismatch requires user to fix command
- Breaks simple piping workflows: `cat plan.json | tsuku install --plan -` requires extracting tool name first
- Scripting becomes complex: `for plan in *.json; do tsuku install $(jq -r .tool $plan) --plan $plan; done`

#### Option 3C: Tool Name Optional, Defaults from Plan

Tool name optional on command line. If provided, must match. If omitted, use plan's tool name.

```bash
tsuku install --plan plan.json          # tool from plan
tsuku install ripgrep --plan plan.json  # explicit, must match
```

**Pros:**
- Flexible for different use cases
- Supports both explicit and implicit workflows
- Good balance of safety and convenience

**Cons:**
- Most complex option
- Two valid syntaxes to document

## Decision Outcome

**Chosen: 1B + 2B + 3C**

### Summary

Accept plan input from file path or stdin using `-` convention (1B). Perform comprehensive validation before execution including platform and format version checks (2B). Allow tool name to be optional on command line, defaulting to the plan's tool name if omitted (3C).

### Rationale

**File path with stdin support (1B)** enables the canonical Unix workflow: `tsuku eval tool | tsuku install --plan -`. This composability is essential for scripting and aligns with the upstream design's explicit mention of piping support. The `-` convention is well-understood by Unix users.

**Comprehensive pre-execution validation (2B)** prevents partial installations and provides clear error messages. Since plans are the authoritative specification in this flow, catching incompatibilities before execution begins is critical. A plan generated on Linux shouldn't silently fail halfway through on macOS.

**Optional tool name defaulting from plan (3C)** provides the best user experience. In scripted workflows, omitting the tool name reduces redundancy (`tsuku install --plan plan.json`). In interactive use, explicit tool names catch mistakes (`tsuku install ripgrep --plan plan.json` fails if plan is actually for `rg`). This flexibility matches how users actually work.

### Trade-offs Accepted

By choosing comprehensive validation (2B), we accept:
- Slightly more complex validation code
- Plans with unknown primitive actions will fail validation (not execution)

By choosing optional tool names (3C), we accept:
- Two valid command syntaxes to document
- Slightly more complex argument parsing

These trade-offs favor user experience and safety over implementation simplicity.

## Solution Architecture

### Overview

The solution adds a `--plan` flag to `tsuku install` that provides an external plan, bypassing the normal evaluation phase. When `--plan` is provided, the installation flow becomes:

```
User provides plan → Validate plan → ExecutePlan() → Store in state
```

This reuses the existing `ExecutePlan()` infrastructure entirely, adding only the plan loading and validation layer.

### Component Architecture

```
                        ┌──────────────────────┐
                        │   tsuku install      │
                        │   --plan <path>      │
                        └──────────┬───────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    ▼                             ▼
             path == "-"                    path is file
                    │                             │
                    ▼                             ▼
            ┌───────────────┐            ┌───────────────┐
            │  Read stdin   │            │  Read file    │
            └───────┬───────┘            └───────┬───────┘
                    │                             │
                    └──────────────┬──────────────┘
                                   │
                                   ▼
                        ┌──────────────────────┐
                        │   Parse JSON         │
                        │   → InstallationPlan │
                        └──────────┬───────────┘
                                   │
                                   ▼
                        ┌──────────────────────┐
                        │   Validate Plan      │
                        │   - Format version   │
                        │   - Platform match   │
                        │   - Primitives only  │
                        │   - Tool name match  │
                        └──────────┬───────────┘
                                   │
                                   ▼
                        ┌──────────────────────┐
                        │   ExecutePlan()      │
                        │   (existing impl)    │
                        └──────────┬───────────┘
                                   │
                                   ▼
                        ┌──────────────────────┐
                        │   Store in state     │
                        └──────────────────────┘
```

### Key Data Structures

The existing `InstallationPlan` structure is used unchanged:

```go
type InstallationPlan struct {
    FormatVersion int       `json:"format_version"`
    Tool          string    `json:"tool"`
    Version       string    `json:"version"`
    Platform      Platform  `json:"platform"`
    GeneratedAt   time.Time `json:"generated_at"`
    RecipeHash    string    `json:"recipe_hash"`
    RecipeSource  string    `json:"recipe_source"`
    Steps         []ResolvedStep `json:"steps"`
}
```

### Plan Loading

```go
// loadPlanFromSource reads a plan from file path or stdin.
// If path is "-", reads from stdin.
func loadPlanFromSource(path string) (*executor.InstallationPlan, error) {
    var reader io.Reader
    if path == "-" {
        reader = os.Stdin
    } else {
        f, err := os.Open(path)
        if err != nil {
            return nil, fmt.Errorf("failed to open plan file: %w", err)
        }
        defer f.Close()
        reader = f
    }

    var plan executor.InstallationPlan
    decoder := json.NewDecoder(reader)
    if err := decoder.Decode(&plan); err != nil {
        if path == "-" {
            return nil, fmt.Errorf("failed to parse plan from stdin: %w\nHint: Save plan to a file first for debugging", err)
        }
        return nil, fmt.Errorf("failed to parse plan from %s: %w", path, err)
    }

    return &plan, nil
}
```

### Plan Validation

Plan validation is split into two layers:

1. **Structural validation**: Reuses existing `executor.ValidatePlan()` for format version, primitive-only actions, and checksum requirements
2. **External plan validation**: Adds platform compatibility and tool name checks specific to external plans

```go
// validateExternalPlan performs comprehensive validation of an external plan.
// Reuses existing ValidatePlan for structural checks, adds external-plan-specific checks.
func validateExternalPlan(plan *executor.InstallationPlan, toolName string) error {
    // First, run existing structural validation (format version, primitives, checksums)
    if err := executor.ValidatePlan(plan); err != nil {
        return fmt.Errorf("plan validation failed: %w", err)
    }

    // Check platform compatibility (external-plan-specific)
    if plan.Platform.OS != runtime.GOOS || plan.Platform.Arch != runtime.GOARCH {
        return fmt.Errorf("plan is for %s-%s, but this system is %s-%s",
            plan.Platform.OS, plan.Platform.Arch, runtime.GOOS, runtime.GOARCH)
    }

    // Check tool name if provided on command line (external-plan-specific)
    if toolName != "" && toolName != plan.Tool {
        return fmt.Errorf("plan is for tool '%s', but '%s' was specified",
            plan.Tool, toolName)
    }

    return nil
}
```

### CLI Changes

```go
var installPlanPath string

func init() {
    installCmd.Flags().StringVar(&installPlanPath, "plan", "",
        "Install from a pre-computed plan file (use '-' for stdin)")
}

// In Run function:
if installPlanPath != "" {
    // Plan-based installation: tool name is optional (inferred from plan)
    // but multiple tools are not allowed with --plan
    if len(args) > 1 {
        printError(fmt.Errorf("cannot specify multiple tools with --plan flag"))
        exitWithCode(ExitInvalidArgs)
    }

    var toolName string
    if len(args) == 1 {
        toolName = args[0]
    }

    if err := runPlanBasedInstall(installPlanPath, toolName); err != nil {
        printError(err)
        exitWithCode(ExitInstallFailed)
    }
} else {
    // Normal installation (existing code)
    // ...
}
```

### Executor Creation for Plan-Based Install

When using `--plan`, a minimal executor is created without a full recipe. The plan provides all necessary information:

```go
func runPlanBasedInstall(planPath, toolName string) error {
    // Load plan
    plan, err := loadPlanFromSource(planPath)
    if err != nil {
        return err
    }

    // Validate plan (includes tool name check if provided)
    if err := validateExternalPlan(plan, toolName); err != nil {
        return err
    }

    // Create minimal recipe for executor context
    minimalRecipe := &recipe.Recipe{
        Metadata: recipe.MetadataSection{
            Name: plan.Tool,
        },
    }

    // Create executor with minimal recipe
    exec, err := executor.NewWithVersion(minimalRecipe, plan.Version)
    if err != nil {
        return fmt.Errorf("failed to create executor: %w", err)
    }
    defer exec.Cleanup()

    // Execute the plan
    if err := exec.ExecutePlan(globalCtx, plan); err != nil {
        return err
    }

    // Store in state using existing InstallWithOptions
    // ...
}
```

### Integration with Existing Flow

The plan-based installation reuses:
- `ExecutePlan()` - unchanged
- State storage via `InstallWithOptions()` - unchanged
- Checksum verification - unchanged (part of ExecutePlan)
- Error handling for `ChecksumMismatchError` - unchanged

The only new code is:
- Plan loading from file/stdin
- Pre-execution validation
- CLI flag handling

### Offline Installation

When artifacts are pre-cached (from a previous `tsuku eval` run or manual download), plan-based installation works offline:

1. Plan specifies exact URLs and checksums
2. Download action checks cache first
3. If cached file exists with matching checksum, skip network request
4. Installation proceeds without network access

This enables the workflow:
```bash
# Online machine
tsuku eval ripgrep > plan.json
# Transfer plan.json and $TSUKU_HOME/cache/downloads/* to offline machine
# Offline machine
tsuku install --plan plan.json  # works without network
```

## Implementation Approach

The implementation is straightforward given the existing infrastructure.

### Phase 1: Plan Loading

- Add `loadPlanFromSource()` function to handle file and stdin
- Add `validateExternalPlan()` function for comprehensive validation
- Unit tests for both functions

### Phase 2: CLI Integration

- Add `--plan` flag to install command
- Add `runPlanBasedInstall()` function that:
  1. Loads plan from source
  2. Validates plan
  3. Creates executor (minimal, for ExecutePlan context)
  4. Calls ExecutePlan()
  5. Stores result in state
- Integration tests for `--plan` flag

### Phase 3: Documentation

- Update `tsuku install --help` with `--plan` usage
- Add examples to README or documentation

## Security Considerations

### Download Verification

**Analysis**: All downloads during plan execution verify checksums against the plan. This is handled by the existing `ExecutePlan()` implementation. Checksums in external plans are trusted as authoritative.

**Mitigation**: ChecksumMismatchError fails the installation with a clear message explaining the mismatch. Users must explicitly investigate before proceeding.

**TOCTOU consideration**: Checksum verification happens after download completes, creating a theoretical time-of-check-time-of-use window. However, work directories are created with mode 0700 within `$TSUKU_HOME`, requiring user-level or root access to exploit. An attacker with such access could bypass verification entirely. This mirrors the threat model of other package managers (npm, pip, cargo) and is an accepted risk for user-space tooling.

### Execution Isolation

**Analysis**: Plan-based installation runs with the same privileges as normal installation (user space, no sudo). The plan specifies actions but cannot escape the executor's sandboxed work directory.

**Mitigations**:
- Work directories created with mode 0700
- Only primitive actions are allowed (validated before execution)
- Actions are constrained to `$TSUKU_HOME` directory structure

### Supply Chain Risks

**Analysis**: External plans are trusted input. A malicious plan could specify URLs pointing to malicious binaries. The checksums in the plan would match (since they were computed for the malicious content).

**Mitigations**:
- Plans should be generated via `tsuku eval`, not hand-crafted
- HTTPS enforcement on all URLs (existing download action protection)
- Users should verify plan source before use
- Future enhancement: plan signing for organizational trust

**Residual risk**: Users who accept plans from untrusted sources may install malicious software. This is a user trust decision, similar to running scripts from the internet.

### User Data Exposure

**Analysis**: Plan files contain URLs and checksums for tool downloads. No user credentials or sensitive data is included in plans.

**Mitigations**:
- Plans don't include authentication tokens
- URLs are public download links (GitHub releases, etc.)
- No additional data exposure beyond normal installation

### Plan File Trust Model

External plans should be treated as code: review before execution, verify source authenticity, don't accept from untrusted sources. The design intentionally does not include automatic trust mechanisms (like plan signing) to keep the scope focused. Plan signing is noted as a future security enhancement.

### Key Assumptions

The following assumptions are made explicit for security analysis:

1. **Plan lifetime**: Plans are intended for short-term use (hours to days for CI workflows), not long-term archival. Recipe and format evolution may invalidate older plans.

2. **Platform compatibility**: Plans are strictly platform-specific. A plan generated for linux-amd64 cannot be used on linux-arm64, even if the binaries might be compatible.

3. **Cached artifact trust**: Cached artifacts in `$TSUKU_HOME/cache/downloads/` are trusted based on checksum verification alone. In offline mode, no external verification is possible.

4. **Upstream trust inheritance**: Plans inherit the trust model of the generation environment. A plan generated from a compromised recipe inherits that compromise.

## Consequences

### Positive

- **Air-gapped deployments enabled**: Organizations can generate plans online and install offline
- **CI optimization**: Central plan generation, distributed execution
- **Reproducibility guarantee**: Same plan → same installation (within platform)
- **Milestone completion**: Delivers the final piece of deterministic recipe execution
- **Minimal code changes**: Reuses existing ExecutePlan() infrastructure

### Negative

- **Trust burden on users**: External plans are trusted input; users must verify sources
- **No built-in trust mechanism**: Plan signing deferred to future work
- **Documentation needed**: New workflow requires user education

### Neutral

- **Two installation modes**: Normal and plan-based, but both ultimately use ExecutePlan()
- **Cache dependency for offline**: Offline installation requires pre-cached artifacts
