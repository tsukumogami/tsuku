# Design: Installation Plans and tsuku eval Command

- **Status**: Superseded by [DESIGN-deterministic-resolution.md](../current/DESIGN-deterministic-resolution.md)
- **Author**: @dangazineu
- **Created**: 2025-12-10
- **Scope**: Tactical
- **Archived**: 2025-12-19
- **See Also**: docs/GUIDE-plan-based-installation.md (user guide)

## Upstream Design Reference

This design implements Milestone 1 of [DESIGN-deterministic-resolution.md](DESIGN-deterministic-resolution.md).

**Relevant sections:**
- Vision: "A recipe is a program that produces a deterministic installation plan"
- Milestone 1: Installation Plans and `tsuku eval`
- Integration: PreDownloader reuse recommendation

## Context and Problem Statement

Tsuku recipes are dynamic specifications that resolve at runtime based on platform, API responses, and template expansion. When a user runs `tsuku install ripgrep@14.1.0`, the actual download URL and binary depend on:

- Platform detection (`{os}`, `{arch}` template variables)
- Asset selection from GitHub releases API
- Recipe template expansion
- External API responses

This dynamic resolution creates several problems:

1. **Recipe testing is difficult**: Builders cannot verify their recipe changes without performing actual installations
2. **No audit trail**: There's no record of exactly what was downloaded and installed
3. **Reproducibility is impossible**: Re-running the same install command may yield different results if upstream assets change

The current `--dry-run` flag shows what *would* be installed but doesn't capture the full resolution details needed for reproducibility.

### Scope

**In scope:**
- `tsuku eval <tool>[@version]` command that outputs an installation plan
- Installation plan JSON format capturing URLs, checksums, and steps
- Plan storage in state.json for installed tools
- `tsuku plan show <tool>` and `tsuku plan export <tool>` commands for inspection

**Out of scope:**
- Plan-based installation (`tsuku install --plan`) - Milestone 3
- Deterministic re-installation from cached plans - Milestone 2
- Lock files for team coordination - tracked separately
- Ecosystem installations (npm, pip, cargo, etc.) - deferred pending action evaluability analysis

## Decision Drivers

- **Testability**: Recipe changes must be verifiable via plan comparison without installation
- **Auditability**: Users should know exactly what would be downloaded before installation
- **Reuse**: Leverage existing infrastructure (PreDownloader, download cache, action system)
- **Performance**: Evaluation may require downloads for checksum computation; this is acceptable for most tools
- **Simplicity**: Plan format should be human-readable and machine-parseable
- **Future compatibility**: Design must support Milestone 3 (plan-based installation)

### Key Assumptions

- Plans are platform-specific by default, but `--os` and `--arch` flags enable cross-platform generation
- Plans are immutable once generated; regeneration creates a new plan
- Evaluation is idempotent and does not modify installation state
- "Reproducibility" means bitwise identical downloads given the same plan

## Implementation Context

### Existing Infrastructure

The codebase provides substantial infrastructure to build upon:

**Command patterns**: Commands in `cmd/tsuku/` follow a consistent structure using Cobra, with global context for cancellation, telemetry integration, and JSON/human-readable output formatting (see `info.go`, `install.go`).

**Executor flow**: The executor (`internal/executor/executor.go`) already separates version resolution from action execution. It maintains an `ExecutionContext` with resolved version, tool paths, and variable expansion. A `DryRun()` method exists but only prints what would happen.

**PreDownloader**: The `internal/validate/predownload.go` provides streaming download with SHA256 checksum computation in a single pass. It includes HTTPS enforcement, SSRF protection, and decompression bomb prevention.

**Download actions**: The `download` action (`internal/actions/download.go`) expands URL templates with platform variables and supports checksum verification. The download cache stores metadata including checksums.

**State management**: `internal/install/state.go` manages `state.json` with file locking for concurrent access. It already stores version information, binaries, and dependencies per tool.

### Integration Point

The upstream design recommends reusing `PreDownloader` for checksum computation during evaluation. This aligns with the LLM validation system's two-phase model: download assets to compute checksums, then execute in isolation.

## Considered Options

### Decision 1: Checksum Computation Strategy

The upstream design poses this as an open question: Should `tsuku eval` actually download files to compute checksums, or just resolve URLs?

#### Option 1A: Download for Checksum Computation

During evaluation, download assets via PreDownloader to compute real SHA256 checksums.

**Pros:**
- Real checksums enable plan verification and tamper detection
- Aligns with upstream design recommendation to reuse PreDownloader
- Supports golden file testing: checksums change when upstream assets change
- Downloaded files populate the cache, benefiting subsequent installation

**Cons:**
- Requires network access during evaluation
- Slower evaluation for large assets (mitigated by progress display)
- Downloads may be wasted if user doesn't proceed to install

#### Option 1B: URL-Only Resolution

Resolve URLs and capture them in the plan, but defer checksum computation to installation time.

**Pros:**
- Fast evaluation (no downloads)
- Works offline if version info is cached
- Lower bandwidth usage

**Cons:**
- Plans are not fully deterministic (checksum unknown until install)
- Cannot detect upstream changes via plan comparison
- Defeats primary use case of golden file testing

### Decision 2: Plan Storage Location

#### Option 2A: Inline in state.json

Store plans as part of the existing ToolState structure in state.json.

**Pros:**
- Single source of truth for tool state
- Uses existing file locking infrastructure
- Simple implementation

**Cons:**
- state.json grows larger with plan data
- Plans are tied to installation state (cannot store plans for uninstalled tools)

#### Option 2B: Separate Plan Files

Store plans in dedicated files (e.g., `$TSUKU_HOME/plans/<tool>-<version>.json`).

**Pros:**
- Plans can exist independently of installations
- Easier to manage plan lifecycle
- Clearer separation of concerns

**Cons:**
- Additional file management complexity
- Must coordinate with state.json for active version tracking
- More implementation work

#### Option 2C: Inline with Export Capability

Store plans inline in state.json, but provide export functionality for standalone use.

**Pros:**
- Simple storage (inline)
- Export enables plan sharing and archival
- Matches deliverables: `tsuku plan export <tool>`

**Cons:**
- state.json growth (same as 2A)
- Exported plans need import path for future milestones

### Decision 3: Plan Content Scope

#### Option 3A: All Resolved Steps

Plans capture all recipe steps with resolved parameters (downloads, extractions, binary installations).

**Pros:**
- Complete record of what installation would do
- Enables full plan replay in Milestone 3 (required for that milestone)
- Better debugging: see exactly what each step resolves to
- Captures extraction parameters (strip_dirs, format) needed for reproducibility

**Cons:**
- Larger plan format
- Some steps (chmod, symlinks) are simpler to capture
- More complex implementation initially

#### Option 3B: Download Steps Only

Plans capture only download-related information: URLs, checksums, sizes.

**Pros:**
- Simple, focused plan format
- Smaller plan size

**Cons:**
- Doesn't capture full installation intent
- Cannot replay extraction or binary installation steps
- **Incompatible with Milestone 3** (plan-based installation requires all steps)
- Would require format migration when Milestone 3 is implemented

## Evaluation Against Decision Drivers

| Option | Testability | Auditability | Reuse | Performance | Future Compat |
|--------|-------------|--------------|-------|-------------|---------------|
| 1A: Download | Good | Good | Good | Fair | Good |
| 1B: URL-only | Poor | Fair | Fair | Good | Good |
| 2A: Inline | Good | Good | Good | Good | Good |
| 2B: Separate files | Good | Good | Fair | Good | Good |
| 2C: Inline+Export | Good | Good | Good | Good | Good |
| 3A: All steps | Good | Good | Good | Fair | Good |
| 3B: Downloads only | Fair | Fair | Good | Good | **Poor** |

## Uncertainties

- **Cache interaction**: How should evaluation interact with the existing download cache? Resolved: yes, evaluated files are cached for reuse during installation.
- **Multi-platform plans**: Resolved: support `--os` and `--arch` flags to generate plans for other platforms.

## Decision Outcome

**Chosen: 1A + 2C + 3A**

### Summary

Download assets during evaluation to compute real checksums (1A), store plans inline in state.json with export capability (2C), and capture all resolved steps for full installation reproducibility (3A). This combination provides the testability and auditability required while maintaining compatibility with Milestone 3's plan-based installation.

### Rationale

**Download for checksum computation (1A)** is essential because:
- Testability requires detecting upstream changes via checksum comparison
- The upstream design explicitly recommends reusing PreDownloader
- Downloaded files benefit subsequent installation via cache population
- URL-only resolution (1B) defeats the golden file testing use case

**Inline with export capability (2C)** is chosen because:
- Matches the deliverables: `tsuku plan show` and `tsuku plan export`
- Simpler than separate files (2B) while providing the same capabilities
- Uses existing state.json locking infrastructure
- Exported plans enable sharing without complex file management

**All resolved steps (3A)** is required because:
- Milestone 3 (plan-based installation) needs all steps for replay
- Option 3B (downloads only) would require format migration later
- Complete plans provide better debugging and auditability

### Trade-offs Accepted

By choosing to download during evaluation, we accept:
- Evaluation requires network access
- Large downloads add latency (e.g., nix-portable at ~60MB)

These are acceptable because:
- Progress display mitigates user frustration for large downloads
- Downloaded files populate the cache, making subsequent installation faster
- The testability benefits outweigh the performance cost

## Solution Architecture

### Overview

The solution introduces an installation plan concept that captures the fully-resolved output of recipe evaluation. Plans are generated via `tsuku eval`, stored in state.json after installation, and can be inspected or exported via `tsuku plan` subcommands.

### Plan Data Structure

```go
type InstallationPlan struct {
    // Format version for future compatibility
    FormatVersion int `json:"format_version"` // Currently 1

    // Metadata
    Tool        string    `json:"tool"`
    Version     string    `json:"version"`
    Platform    Platform  `json:"platform"`
    GeneratedAt time.Time `json:"generated_at"`

    // Recipe provenance
    RecipeHash   string `json:"recipe_hash"`   // SHA256 of recipe file content
    RecipeSource string `json:"recipe_source"` // "registry" or file path

    // Resolved steps
    Steps []ResolvedStep `json:"steps"`
}

type Platform struct {
    OS   string `json:"os"`   // e.g., "linux"
    Arch string `json:"arch"` // e.g., "amd64"
}

type ResolvedStep struct {
    Action    string                 `json:"action"`
    Params    map[string]interface{} `json:"params"`
    Evaluable bool                   `json:"evaluable"` // false for run_command, ecosystem installs

    // For download steps only
    URL      string `json:"url,omitempty"`
    Checksum string `json:"checksum,omitempty"`
    Size     int64  `json:"size,omitempty"`
}
```

**Format versioning**: The `format_version` field enables future evolution of the plan format. Readers should reject plans with unsupported versions. Version 1 is the initial format defined here.

### Components

```
                 +-----------------+
                 |  tsuku eval     |
                 +--------+--------+
                          |
                          v
    +---------------------+---------------------+
    |              PlanGenerator                |
    | - Resolves version via existing Executor  |
    | - Processes recipe steps                  |
    | - Downloads for checksum computation      |
    +---------------------+---------------------+
                          |
                          v
    +---------------------+---------------------+
    |              PreDownloader                |
    | - Downloads via existing DownloadCache    |
    | - Computes SHA256 checksum                |
    | - Returns checksum and file size          |
    +---------------------+---------------------+
                          |
                          v
                 +--------+--------+
                 | InstallationPlan|
                 +-----------------+
                          |
           +--------------+--------------+
           |              |              |
           v              v              v
    +-----------+  +------------+  +------------+
    | JSON      |  | state.json |  | tsuku plan |
    | stdout    |  | (after     |  | show/export|
    |           |  |  install)  |  |            |
    +-----------+  +------------+  +------------+
```

**Note**: PreDownloader reuses the existing `internal/actions/download_cache.go` infrastructure. Downloaded files are cached for subsequent installation.

### Deterministic Download Cache

Downloads during evaluation are stored in the existing download cache (`$TSUKU_HOME/cache/downloads/`). The cache key is derived from the URL, making it deterministic:

1. **During evaluation**: Files are downloaded and cached with their computed checksums
2. **During installation**: If a cached file exists and checksum matches, skip re-download
3. **Cache invalidation**: Checksum mismatch triggers re-download (detects upstream changes)

This ensures that `tsuku install` immediately after `tsuku eval` reuses the already-downloaded files, avoiding redundant network requests.

**Implementation requirements**:
- Cache directory must be created with mode 0700 (user-only) to prevent exposure on multi-user systems
- Cache writes must check for symlinks via `os.Lstat()` before writing to prevent symlink attacks
- Cache eviction policy is deferred to future work; for now, cache grows unbounded (acceptable for Milestone 1)

### Platform Override Flags

The `tsuku eval` command supports platform override flags for cross-platform plan generation:

```
tsuku eval [--os <os>] [--arch <arch>] <tool>[@version]
```

- `--os`: Override the operating system (e.g., `linux`, `darwin`, `windows`)
- `--arch`: Override the architecture (e.g., `amd64`, `arm64`)

If omitted, flags default to the current system's values (`runtime.GOOS`, `runtime.GOARCH`).

**Implementation requirements**:
- Flag values must be validated against a whitelist of known OS/arch values to prevent path traversal injection through template variables
- Valid OS values: `linux`, `darwin`, `windows`, `freebsd`
- Valid arch values: `amd64`, `arm64`, `386`, `arm`

**Use cases**:
- CI can generate plans for all target platforms from a single runner
- Recipe builders can test cross-platform URL resolution without multiple machines
- Golden file testing can verify all platform variants

**Limitations**:
- Cross-platform plans cannot be installed on the current system
- Some version providers may not support cross-platform queries (e.g., GitHub asset selection may differ)
- Binaries for other platforms cannot be executed locally for verification

**Note**: SHA256 checksums are platform-agnostic and can be computed for any downloaded file regardless of target platform. The limitation is execution/extraction verification, not checksum computation.

### Key Interfaces

**PlanGenerator** (new in `internal/executor/plan.go`):
```go
// GeneratePlan evaluates a recipe and produces an installation plan
func (e *Executor) GeneratePlan(ctx context.Context) (*InstallationPlan, error)

// processStep resolves a single recipe step for plan inclusion
func (e *Executor) processStep(ctx context.Context, step recipe.Step) (*ResolvedStep, error)
```

**Plan storage** (extends `internal/install/state.go`):
```go
type VersionState struct {
    Requested   string            `json:"requested"`
    Binaries    []string          `json:"binaries"`
    InstalledAt time.Time         `json:"installed_at"`
    Plan        *InstallationPlan `json:"plan,omitempty"`  // NEW
}
```

### Conditional Step Handling

Recipe steps may have `when` clauses that filter by platform. During plan generation:
- Steps are evaluated against the current platform
- Steps whose `when` clause evaluates to false are **excluded** from the plan
- Plans are therefore platform-specific by design

### Recipe Hash Computation

The `recipe_hash` field captures a SHA256 hash of the raw TOML recipe file content. This enables detection of recipe changes: if the hash differs, the plan may be stale.

### Action Evaluability

Not all actions can be evaluated deterministically. Actions are classified by their evaluability:

**Fully evaluable actions** (can be captured in plans):
- `download`, `download_archive`, `github_archive`, `github_file`, `hashicorp_release` - URL and checksum captured
- `extract` - format, strip_dirs, files captured
- `install_binaries`, `chmod` - parameters captured verbatim
- `create_symlink`, `set_env`, `write_file` - parameters captured verbatim
- `validate_checksum` - checksum value captured
- `set_rpath`, `link_dependencies`, `install_libraries` - parameters captured verbatim

**Non-evaluable actions** (cannot guarantee reproducibility):
- `run_command` - arbitrary shell execution, outcome unpredictable
- `npm_install`, `pipx_install`, `gem_install`, `cargo_install`, `go_install` - delegate to external package managers
- `cpan_install`, `nix_install` - external ecosystem resolution

A recipe is considered **evaluable** if all its actions (after `when` clause filtering) are fully evaluable.

**Behavior for non-evaluable recipes**:
- `tsuku eval` on a non-evaluable recipe outputs a partial plan with warnings
- Non-evaluable steps are marked in the plan with `"evaluable": false`
- Users are informed which steps cannot be guaranteed reproducible

This classification enables future work:
- Recipe metadata could explicitly declare evaluability
- CI could enforce evaluability for recipes in the registry
- `tsuku eval --strict` could fail on non-evaluable recipes

### Data Flow

1. **Evaluation flow** (`tsuku eval`):
   ```
   User runs: tsuku eval ripgrep@14.1.0
   → Load recipe
   → Compute recipe hash (SHA256 of TOML content)
   → Resolve version (14.1.0 → v14.1.0)
   → For each step:
     - Evaluate `when` clause (skip if false)
     - Expand templates ({version}, {os}, {arch})
     - If download step: download via PreDownloader, capture checksum
     - Create ResolvedStep
   → Output JSON plan to stdout
   ```

2. **Installation flow** (modified):
   ```
   User runs: tsuku install ripgrep@14.1.0
   → Generate plan (same as eval)
   → Execute plan steps
   → Store plan in state.json
   ```

3. **Plan inspection flow**:
   ```
   User runs: tsuku plan show ripgrep
   → Load state.json
   → Find tool's plan
   → Display formatted plan

   User runs: tsuku plan export ripgrep
   → Load state.json
   → Output plan as JSON
   ```

## Implementation Approach

### Phase 1: Plan Generation Core

- Add `InstallationPlan` and `ResolvedStep` types
- Implement `PlanGenerator` in executor package
- Integrate with PreDownloader for checksum computation
- Unit tests for plan generation

### Phase 2: eval Command

- Add `cmd/tsuku/eval.go` command
- JSON output formatting
- Progress display for downloads
- Integration tests

### Phase 3: Plan Storage

- Extend `VersionState` with Plan field
- Modify installation flow to generate and store plan
- Migration: existing installations have no plan (acceptable)

### Phase 4: plan Subcommands

- Add `cmd/tsuku/plan.go` with show/export subcommands
- Human-readable formatting for `show`
- JSON output for `export`

## Consequences

### Positive

- Recipe changes can be tested via `diff <(tsuku eval tool) expected.json`
- Users get full visibility into what would be downloaded
- Plans enable future deterministic re-installation (Milestone 2)
- Downloaded files populate cache, speeding up subsequent install

### Negative

- Evaluation requires network access and may be slow for large downloads
- state.json grows with plan data (typically 1-5KB per tool)
- Cross-platform eval has limitations (some providers may not support it)

### Mitigations

- Progress display during evaluation reduces user frustration
- Plan data is relatively small; state.json growth is acceptable
- `--os` and `--arch` flags enable cross-platform testing from a single machine

## Security Considerations

### Download Verification

During evaluation, files are downloaded to compute checksums. The design reuses `PreDownloader` from the LLM validation system, which provides:

- **HTTPS enforcement**: Only HTTPS URLs are accepted; HTTP is rejected
- **SHA256 checksums**: Computed during streaming download, stored in plans
- **Verification failure**: If checksum computation fails, the step fails and evaluation stops

Plans capture checksums that can be verified during installation (Milestone 2). This creates an audit trail: if an upstream asset changes, the checksum mismatch will be detected.

### Execution Isolation

The `tsuku eval` command does not execute downloaded artifacts. It only:

- **File system access**: Downloads to a temporary directory (cleaned up after evaluation), writes to download cache (`$TSUKU_HOME/cache/downloads/`)
- **Network access**: Required for version resolution and asset downloads
- **No privilege escalation**: Runs entirely in user space, no sudo required

The `tsuku plan show/export` commands only read from state.json; they have no network access and cannot modify system state.

### Supply Chain Risks

**Source trust model**: Plans capture URLs from recipe definitions. Recipes are:
- Maintained in the tsuku registry (trusted)
- Or loaded from local files (`--recipes-dir`)

**Upstream compromise risks**:
- If an upstream release is re-tagged with different assets, checksums in existing plans will detect the change
- Initial plan creation inherits any existing compromise (no mitigation)
- Plans provide audit trail for forensic analysis

**Plan file trust**: In Milestone 3, `tsuku install --plan` will trust the plan file. Malicious plans could specify malicious URLs. Mitigation: plans should be generated via `tsuku eval`, not hand-crafted.

### User Data Exposure

**Local data accessed**:
- Recipes (registry or local files)
- state.json (for plan storage and inspection)
- Download cache metadata

**Data sent externally**:
- Version provider API calls (GitHub, PyPI, etc.) to resolve versions
- HTTP requests to download assets
- No user-specific data is transmitted

**Privacy implications**: Download URLs and version queries could reveal what tools a user is evaluating. This is the same exposure as `tsuku install` today.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Man-in-the-middle attacks | HTTPS enforcement, SSRF protection in PreDownloader | Compromised CAs |
| Malicious redirects | Redirect validation rejects HTTPS→HTTP downgrade | None |
| Decompression bombs | PreDownloader disables compression | None |
| Upstream asset tampering | Checksum comparison detects changes post-evaluation | Initial compromise undetected |
| Plan file manipulation | Plans should be generated via tsuku eval | User trust decision for external plans |
| Information disclosure via network | Standard exposure, same as install | None beyond existing |
| Resource exhaustion (large downloads) | PreDownloader uses context cancellation; users can interrupt | Large files may fill disk before cancelled |
| Cache symlink attacks | Check via `os.Lstat()` before writing cache files | None if implemented |
| Cache directory exposure | Create cache directory with mode 0700 | None if implemented |
| Platform flag injection | Validate against whitelist of known OS/arch values | None if implemented |

### Future Security Enhancements (Milestone 2+)

The following security measures are deferred to future milestones:

- **Plan signing**: Cryptographic signatures on plans to detect tampering. Required before `tsuku install --plan` trusts external plan files.
- **Checksum mismatch policy**: Clear behavior when stored plan checksum doesn't match downloaded file during installation.
- **Download size limits**: Optional maximum download size to prevent resource exhaustion.

These are not required for Milestone 1 (evaluation only) but will be addressed before plan-based installation.

