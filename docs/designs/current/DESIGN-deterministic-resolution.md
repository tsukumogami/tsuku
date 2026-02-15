---
status: Current
problem: Tsuku recipes are dynamic and non-deterministic; the same tool version can produce different installation results across days or machines due to platform detection, asset selection, and external API responses, preventing reproducible team installations and making recipe testing difficult.
decision: Separate recipe evaluation (dynamic, produces deterministic installation plans) from plan execution (deterministic, downloads from exact URLs), making all installations plan-based to guarantee reproducibility by architecture.
rationale: This two-phase model provides reproducible installations by default, enables recipe testing without actual deployment, supports air-gapped deployments, and reuses existing security infrastructure. Installation plans become auditable artifacts that capture exactly what will be downloaded.
---

# Design: Deterministic Recipe Resolution and Installation Plans

## Status

Current
- **Issue**: #227
- **Author**: @dangazineu
- **Created**: 2025-12-09
- **Scope**: Strategic
- **Archived**: 2025-12-19
- **See Also**: docs/GUIDE-plan-based-installation.md (user guide)

## Context and Problem Statement

Tsuku installs developer tools by evaluating recipes at runtime. These recipes contain templates, version providers, and platform-specific logic that resolve dynamically during installation. While this flexibility enables recipes to work across platforms and versions, it creates a reproducibility problem: the same `tsuku install` command can produce different results depending on when and where it runs.

This non-determinism affects three key user scenarios:

1. **Team installations**: Different team members running `tsuku install ripgrep` may get different binaries if upstream assets change between installations.

2. **Recipe testing**: Recipe authors cannot verify their changes without performing actual installations. There's no way to see what a recipe would download without downloading it.

3. **Air-gapped environments**: Installations require network access at execution time. Organizations with strict security requirements cannot pre-approve specific downloads or install in isolated environments.

The root cause is that recipe evaluation and installation execution are interleaved. Platform detection, URL expansion, and asset downloads happen as a single operation with no intermediate representation. We need an architecture that separates "what to install" from "how to install it."

## Vision

**A recipe is a program that produces a deterministic installation plan.**

Tsuku should separate recipe evaluation from plan execution, making determinism the default rather than an opt-in feature. This enables:

1. **Reproducible installations**: Re-running the same install produces identical results
2. **Recipe testing without installation**: Verify builders produce correct plans
3. **Air-gapped deployments**: Generate plans online, execute offline

## Problem Statement

Tsuku recipes are dynamic - they contain templates and version provider configuration that resolve at runtime. Even with an exact version (`ripgrep@14.1.0`), the actual download URL and binary depend on:

- Platform detection (`{os}`, `{arch}`)
- Asset selection from releases
- Recipe template expansion
- External API responses

This creates non-determinism:
- Same command on different days may yield different binaries
- Teams cannot guarantee identical installations across machines
- No audit trail of exactly what was downloaded
- Recipe changes are hard to test without actual installation

## Core Insight

Split installation into two phases:

```
Phase 1: Evaluation (dynamic)
  Recipe + Version + Platform → Installation Plan
  - Queries external APIs
  - Expands templates
  - Downloads to compute checksums
  - Produces immutable plan

Phase 2: Execution (deterministic)
  Installation Plan → Installed Tool
  - Downloads from exact URLs
  - Verifies checksums
  - Fails on mismatch
  - No external API calls
```

The installation plan is the key artifact - a fully-resolved, deterministic specification that can be replayed.

## Strategic Deliverables

### Milestone 1: Installation Plans and `tsuku eval`

**Goal**: Introduce the plan concept and enable recipe testing without installation.

**Deliverables**:
- `tsuku eval <tool>[@version]` command that outputs an installation plan
- Installation plan format (JSON) capturing URLs, checksums, steps
- Plan storage in state.json for installed tools
- `tsuku plan show/export` for inspection

**Value delivered**:
- Recipe builders can test changes by comparing `tsuku eval` output
- Golden file testing: `diff expected.json <(tsuku eval tool@version)`
- Audit trail: know exactly what would be downloaded

**Key question**: Should `tsuku eval` actually download files to compute checksums, or just resolve URLs?
- Download approach: Real checksums, but requires network
- URL-only approach: Fast, but checksums computed at install time

### Milestone 2: Deterministic Execution

**Goal**: Refactor installation to use the two-phase (eval/exec) model, making all installations plan-based.

**Architectural change**: `tsuku install foo` becomes functionally equivalent to `tsuku eval foo | tsuku install --plan -`. Every installation generates a plan (or reuses a cached one), then executes that plan. This single execution path guarantees determinism by architecture rather than careful implementation.

**Deliverables**:
- Refactor executor so all installations go through plan generation and execution
- Re-install uses stored plan when version constraint exactly matches resolved version
- `--fresh` flag to force fresh evaluation
- Checksum verification during plan execution
- Clear error on checksum mismatch (indicates upstream change)

**Value delivered**:
- Reproducible re-installs without opt-in
- Detection of upstream changes (re-tagged releases, modified assets)
- Security: checksum mismatch is a failure, not a warning

**Version constraint behavior**:
- Exact version (`ripgrep@14.1.0`): Reuse cached plan, verify checksums
- Dynamic constraint (`ripgrep`, `ripgrep@latest`, `go@1.x`): Always re-evaluate to resolve current version
- When dynamic resolution yields a new version, inform user of the change

**Behavior change**: For pinned versions, users must use `--fresh` to pick up upstream changes. This is intentional - determinism is the default for exact version pins.

### Milestone 3: Plan-Based Installation

**Goal**: Enable installation from pre-computed plans.

**Deliverables**:
- `tsuku install --plan <file>` to install from exported plan
- Support for piping: `tsuku eval tool | tsuku install --plan -`
- Offline installation when artifacts are pre-downloaded

**Value delivered**:
- Air-gapped environments can use pre-computed plans
- CI can generate plans and distribute to build nodes
- Aligns with LLM validation's pre-download model (see Integration section)

## Integration with Existing Infrastructure

### Download Infrastructure Reuse

The existing download system provides:
- `internal/actions/download.go`: URL expansion, HTTPS enforcement, SSRF protection
- `internal/actions/download_cache.go`: Caching with checksum validation
- Security: decompression bomb prevention, redirect validation

`tsuku eval` should reuse these security protections.

### LLM Validation Pre-Download Alignment

The LLM builder validation system (#268, #303) implements:
- `PreDownloader`: Downloads assets with SHA256 checksum capture
- Container validation with `--network=none` (air-gapped execution)
- Two-phase model: download assets, then validate in isolation

**Key insight**: LLM validation's air-gapped container execution is conceptually the same as `tsuku install --plan` with pre-downloaded assets:

| LLM Validation | tsuku eval/exec |
|----------------|-----------------|
| PreDownloader downloads assets | `tsuku eval` downloads to compute checksums |
| Container runs with --network=none | `tsuku install --plan` can work offline |
| Assets mounted read-only | Plan specifies exact URLs and checksums |

**Recommendation**: Reuse `PreDownloader` from `internal/validate/predownload.go` for `tsuku eval`. This provides:
- SHA256 checksum computation during download
- HTTPS enforcement and SSRF protection
- Proven, tested implementation

### Executor Refactoring

Current executor flow:
```
Recipe → Version Resolution → Action Execution (download, extract, install)
```

New flow:
```
Recipe → Version Resolution → Plan Generation → Plan Execution
                                    ↑                  ↑
                              tsuku eval          tsuku install
```

The executor should be refactored to:
1. Accept a plan as input (not just a recipe)
2. Separate "generate plan" from "execute plan" code paths
3. Store plans alongside version info in state.json

## Decision Drivers

1. **Determinism by default**: Don't require flags for reproducibility
2. **Testability**: Enable recipe testing without installation
3. **Reuse**: Leverage existing download and validation infrastructure
4. **Security**: Checksum verification is mandatory, mismatches are failures

## Considered Options

### Option A: Checksum-only verification (rejected)

Add checksum verification to the existing installation flow. Recipes would include optional checksums, and installation would fail on mismatch.

**Pros**: Minimal changes to existing architecture.

**Cons**: Doesn't solve testability. Doesn't enable air-gapped installations. Checksums must be maintained manually in recipes. Non-determinism remains the default.

### Option B: Lock file approach (deferred)

Introduce a `tsuku.lock` file that captures resolved versions and checksums for a project's tools. Similar to `package-lock.json` or `Cargo.lock`.

**Pros**: Familiar pattern. Good for project-level reproducibility.

**Cons**: Requires project context. Doesn't help with single-tool installations. Doesn't address recipe testing. Lock file generation still needs the underlying infrastructure this design provides.

### Option C: Two-phase eval/exec model (selected)

Separate recipe evaluation (produces deterministic plan) from plan execution (deterministic by construction). All installations go through plan generation, making determinism architectural.

**Pros**: Determinism by default. Enables recipe testing via plan comparison. Supports air-gapped installation. Plans are auditable artifacts. Lock files become a thin layer on top.

**Cons**: Requires executor refactoring. Introduces new concept (plans) users may need to understand.

## Decision Outcome

We chose the two-phase eval/exec model (Option C). This approach makes determinism the default behavior rather than an opt-in feature. By refactoring the executor so that every installation generates a plan before execution, we guarantee reproducibility by architecture.

The key insight is that an installation plan is a fully-resolved, immutable specification. Once generated, a plan contains exact URLs, checksums, and installation steps. Re-executing the same plan produces identical results. This model also enables lock files (Option B) as a future enhancement that stores plans for project-level coordination.

## Solution Architecture

The solution introduces an installation plan as the intermediate representation between recipe evaluation and execution.

### Installation Plan Format

Plans are JSON documents containing:

- **Tool metadata**: Name, resolved version, platform
- **Download specifications**: Exact URLs with SHA256 checksums
- **Installation steps**: Ordered list of actions (extract, chmod, install_binaries)
- **Provenance**: Recipe version, evaluation timestamp

### Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Recipe Evaluation                         │
│  Recipe + Version + Platform → Installation Plan             │
│  - Queries version providers (GitHub, PyPI, etc.)           │
│  - Expands URL templates                                    │
│  - Downloads assets to compute checksums                    │
│  - Resolves platform-specific paths                         │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Installation Plan                          │
│  {                                                           │
│    "tool": "ripgrep",                                       │
│    "version": "14.1.0",                                     │
│    "platform": {"os": "linux", "arch": "amd64"},           │
│    "downloads": [{                                          │
│      "url": "https://github.com/.../rg-14.1.0.tar.gz",     │
│      "sha256": "abc123..."                                  │
│    }],                                                      │
│    "steps": [...]                                           │
│  }                                                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Plan Execution                            │
│  Installation Plan → Installed Tool                          │
│  - Downloads from exact URLs                                │
│  - Verifies SHA256 checksums (fails on mismatch)           │
│  - Executes installation steps                              │
│  - No version resolution or template expansion              │
└─────────────────────────────────────────────────────────────┘
```

### Command Interface

- `tsuku eval <tool>[@version]`: Generate plan without installing
- `tsuku install <tool>`: Generate plan, then execute (current behavior, refactored internally)
- `tsuku install --plan <file>`: Execute pre-computed plan
- `tsuku plan show <tool>`: Display stored plan for installed tool
- `tsuku plan export <tool>`: Export stored plan to a file for sharing or air-gapped use

### Plan Storage

Plans are stored inline in `$TSUKU_HOME/state.json` alongside version information. The `tsuku plan export` command extracts plans for sharing or air-gapped use.

### Detailed Data Structures

The core data types that implement the plan concept:

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

type Platform struct {
    OS   string `json:"os"`
    Arch string `json:"arch"`
}

type ResolvedStep struct {
    Action    string                 `json:"action"`
    Params    map[string]interface{} `json:"params"`
    Evaluable bool                   `json:"evaluable"`
    URL       string                 `json:"url,omitempty"`
    Checksum  string                 `json:"checksum,omitempty"`
    Size      int64                  `json:"size,omitempty"`
}
```

**Plan cache key** is based on the OUTPUT of version resolution, not user input. This means `tsuku eval ripgrep` and `tsuku eval ripgrep@14.1.0` that resolve to the same version share the same cache:

```go
type PlanCacheKey struct {
    Tool       string `json:"tool"`
    Version    string `json:"version"`     // RESOLVED version
    Platform   string `json:"platform"`    // e.g., "linux-amd64"
    RecipeHash string `json:"recipe_hash"` // SHA256 of recipe TOML
}
```

**Checksum mismatch** is a hard failure with a clear recovery path:

```go
type ChecksumMismatchError struct {
    URL              string
    ExpectedChecksum string
    ActualChecksum   string
}
```

The error message explicitly mentions both legitimate updates and supply chain attacks as possibilities, guiding users to `tsuku install <tool> --fresh` to re-verify artifacts.

### Action Classification and Decomposition

Composite actions in recipes are authoring conveniences that must decompose into primitive operations during plan generation. This ensures plans contain only primitives that execute deterministically.

#### Primitive Tiers

**Tier 1: File Operation Primitives** - Fully atomic operations with deterministic, reproducible behavior:

| Primitive | Purpose | Key Parameters |
|-----------|---------|----------------|
| `download_file` | Fetch URL to file | `url`, `dest`, `checksum` |
| `extract` | Decompress archive | `archive`, `format`, `strip_dirs` |
| `chmod` | Set file permissions | `files`, `mode` |
| `install_binaries` | Copy to install dir, create symlinks | `binaries`, `install_mode` |
| `set_env` | Set environment variables | `vars` |
| `set_rpath` | Modify binary rpath | `binary`, `rpath` |
| `link_dependencies` | Create dependency symlinks | `dependencies` |
| `install_libraries` | Install shared libraries | `libraries` |

**Tier 2: Ecosystem Primitives** - The decomposition barrier for ecosystem-specific operations. Atomic from tsuku's perspective but internally invoke external tooling. The plan captures maximum constraint to minimize non-determinism:

| Primitive | Ecosystem | Locked at Eval | Residual Non-determinism |
|-----------|-----------|----------------|--------------------------|
| `go_build` | Go | go.sum, module versions | Compiler version, CGO |
| `cargo_build` | Rust | Cargo.lock | Compiler version |
| `npm_exec` | Node.js | package-lock.json | Native addon builds |
| `pip_exec` | Python | requirements.txt with hashes | Native extensions |
| `gem_exec` | Ruby | Gemfile.lock | Native extensions |
| `nix_realize` | Nix | Derivation hash | None (fully deterministic) |
| `cpan_install` | Perl | cpanfile.snapshot | XS modules, Makefile.PL |

#### Ecosystem Composite Actions

Recipe-authoring composites decompose to ecosystem primitives during evaluation:

| Composite | Decomposes To | Purpose |
|-----------|---------------|---------|
| `go_install` | `go_build` | Install Go module from source |
| `cargo_install` | `cargo_build` | Install Rust crate from source |
| `npm_install` | `npm_exec` | Install npm package |
| `gem_install` | `gem_exec` | Install Ruby gem |
| `pipx_install` | `pip_exec` | Install Python package with pipx |
| `nix_install` | `nix_realize` | Install package via Nix |
| `cpan_install` | `cpan_install` | Install Perl module |

These composites are **evaluable** - they resolve versions, capture lock files, and compute checksums during evaluation, then emit the corresponding primitive in the plan. The primitive appears in the plan, not the composite.

#### Decomposable Interface

Composite actions implement the `Decomposable` interface:

```go
type Decomposable interface {
    Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error)
}

type Step struct {
    Action   string
    Params   map[string]interface{}
    Checksum string
    Size     int64
}

type EvalContext struct {
    Version    string
    VersionTag string
    OS         string
    Arch       string
    Recipe     *recipe.Recipe
    Resolver   *version.Resolver
    Downloader *validate.PreDownloader
}
```

The plan generator recursively decomposes composite actions until only primitives remain. Cycle detection uses a visited set of `(action, params_hash)` tuples.

**Example**: `github_archive` decomposes to `download_file` + `extract` + `chmod` + `install_binaries`. Ecosystem composites like `go_install` decompose to a single `go_build` primitive with captured lock information (`go.sum`, resolved module versions).

Plans that contain ecosystem primitives carry a `deterministic: false` flag on those steps to explicitly mark residual non-determinism.

### Two-Phase Evaluation Model

Plan generation splits into two distinct phases:

1. **Version Resolution** (always runs): Maps user input to a resolved version. Fast, requires only API calls.
2. **Artifact Verification** (cached by resolution output): Downloads artifacts and computes checksums. Slow, requires network.

The cache key is based on what Phase 1 produces, not what the user typed. This means different version constraints that resolve to the same version share the same cache. The `--fresh` flag bypasses artifact cache to force re-verification.

Cache lookup validates three factors before reusing a plan:
- Recipe hash matches (recipe hasn't changed)
- Format version matches (plan structure is compatible)
- Platform matches (OS and architecture)

The orchestration logic (`getOrGeneratePlan`) resolves the version first, generates a cache key from the resolution output, checks for a cached plan, and only runs Phase 2 if no valid cache exists. Download cache reuse ensures that files downloaded during plan generation are available to `ExecutePlan()` without re-downloading.

### Plan Execution and Verification

`ExecutePlan()` iterates over plan steps and executes each primitive. For download steps with checksums, it computes the SHA256 of the downloaded file and compares against the plan:

- **Match**: Proceed to next step
- **Mismatch**: Hard failure via `ChecksumMismatchError` with recovery guidance

The executor validates that plans contain only primitive actions before execution begins. This is the single execution path for all installations - there is no legacy `Execute()` method.

### External Plan Loading and Validation

For `tsuku install --plan`, plans can be loaded from file paths or stdin (using `-`). Before execution, external plans undergo validation:

- **Structural**: Format version, primitive-only actions, checksum requirements
- **Platform**: Plan's OS/arch must match the current system
- **Tool name**: If specified on the command line, must match the plan's tool field

Tool name is optional on the command line and defaults to the plan's tool name. This supports both explicit use (`tsuku install ripgrep --plan plan.json`) and scripted workflows (`tsuku install --plan plan.json`).

Offline installation works when artifacts are pre-cached: the download action checks `$TSUKU_HOME/cache/downloads/` first and skips network requests when a cached file's checksum matches the plan.

## Implementation Approach

Implementation proceeds in three milestones, each delivering incremental value.

### Milestone 1: Installation Plans and `tsuku eval`

Introduce the plan concept and the `tsuku eval` command. This milestone focuses on plan generation without changing installation behavior.

1. Define the installation plan JSON schema in `internal/plan/`
2. Implement plan generation by extracting evaluation logic from the executor
3. Add `tsuku eval` command that outputs plans to stdout
4. Reuse `PreDownloader` from `internal/validate/predownload.go` for checksum computation

After this milestone, recipe authors can test changes by comparing `tsuku eval` output against expected plans.

**Key decisions for this milestone:**

- **Download for checksum computation**: `tsuku eval` downloads assets to compute real SHA256 checksums rather than just resolving URLs. This is essential for golden file testing and tamper detection. Downloaded files populate `$TSUKU_HOME/cache/downloads/` for reuse during subsequent installation.
- **All resolved steps captured**: Plans include every recipe step (not just downloads) to support full replay in Milestone 3.
- **Inline storage with export**: Plans stored in `state.json` with `tsuku plan export` for standalone use.
- **Conditional step handling**: Recipe steps with `when` clauses are evaluated against the target platform; steps that don't match are excluded from the plan. Plans are platform-specific by design.
- **Platform override flags**: `tsuku eval` accepts `--os` and `--arch` flags for cross-platform plan generation. Values are validated against a whitelist of known platforms to prevent injection through template variables.
- **Recipe hash**: SHA256 of the raw TOML recipe file content, enabling detection of recipe changes that might invalidate a plan.
- **Action evaluability**: Actions are classified as fully evaluable (URL/checksum captured) or non-evaluable (arbitrary shell, ecosystem installers). Non-evaluable steps are marked with `evaluable: false` in the plan.

### Milestone 2: Deterministic Execution

Refactor the executor to use plans internally. All installations become plan-based, making determinism architectural.

1. Split executor into `PlanGenerator` and `PlanExecutor` components
2. Modify `tsuku install` to generate a plan, then execute it
3. Store plans in state.json for installed tools
4. Add `--fresh` flag to force re-evaluation for pinned versions
5. Implement checksum verification with failure on mismatch
6. Define `Decomposable` interface and primitive registry
7. Implement recursive decomposition for composite actions
8. Migrate composite actions (`github_archive`, `download_archive`, etc.) to `Decompose()`
9. Implement ecosystem primitives (`go_build`, `cargo_build`, etc.)
10. Validate plans contain only primitives; add `deterministic` flag to plan schema

After this milestone, re-installing a tool with a pinned version reuses the stored plan.

**Key decisions for this milestone:**

- **Cache by resolution output** (not user input): Version resolution always runs. Artifact verification is cached based on what resolution produces. This avoids complex version constraint classification and aligns with Nix's evaluation/realization model.
- **Replace Execute with ExecutePlan**: The old `Execute()` method is removed, not kept alongside. Since tsuku is pre-1.0, there's no backward compatibility burden. All execution goes through plans by construction.
- **Hard failure with recovery on checksum mismatch**: Mismatches are installation failures, not warnings. The error message guides users to `--fresh` for re-verification. This prevents silent installation of modified binaries.
- **Composite decomposition at eval time** (not interpreter pattern): Composite actions implement `Decompose()` rather than having the plan generator understand their internals. This localizes complexity and makes decomposition independently testable.
- **Two primitive tiers**: File operation primitives (Tier 1) are fully deterministic. Ecosystem primitives (Tier 2) represent the decomposition barrier where external tooling is invoked with maximum constraint but residual non-determinism exists.
- **Implementation organized in parallel tracks**: Plan cache types (A), state manager (B), executor methods (C), and CLI changes (D) touch different files and can be developed in parallel, with integration in track D.

### Milestone 3: Plan-Based Installation

Enable installation from externally-provided plans for air-gapped environments.

1. Add `--plan` flag to `tsuku install`
2. Support reading plans from file or stdin (using `-` for stdin per Unix convention)
3. Implement comprehensive pre-execution validation for external plans
4. Implement offline installation when cached assets are available
5. Add `tsuku plan show` and `tsuku plan export` commands

After this milestone, teams can generate plans on connected machines and execute them in isolated environments.

**Key decisions for this milestone:**

- **File path with stdin support**: `--plan plan.json` reads from file, `--plan -` reads from stdin. This enables the canonical `tsuku eval tool | tsuku install --plan -` workflow.
- **Comprehensive pre-execution validation**: External plans are validated for format version, platform compatibility, primitive-only steps, and tool name match before execution begins. No partial installation on validation failure.
- **Tool name optional, defaults from plan**: `tsuku install --plan plan.json` infers the tool name from the plan. `tsuku install ripgrep --plan plan.json` validates that the plan matches. This supports both scripted and interactive workflows.
- **Plan trust model**: External plans are treated as code - review before execution, verify source. Plan signing is deferred to future work.
- **Minimal new code**: Reuses existing `ExecutePlan()` entirely, adding only plan loading and validation layers.

## Resolved Questions

These questions were posed in the original strategic design and resolved during tactical design:

1. **Checksum source**: Resolved: download files to compute real checksums. URL-only resolution defeats golden file testing. Downloaded files populate the cache for subsequent installation.

2. **Plan storage**: Resolved: inline in state.json with export capability via `tsuku plan export`. Simpler than separate files while providing the same capabilities.

3. **Multi-platform plans**: Resolved: `tsuku eval` accepts `--os` and `--arch` flags for cross-platform generation, defaulting to the current system. Flag values validated against a whitelist.

4. **Cache integration**: Resolved: plans are stored inline in state.json. Downloaded artifacts go to `$TSUKU_HOME/cache/downloads/`. Plan cache lookup is based on resolution output (tool, resolved version, platform, recipe hash), not user input.

## Security Considerations

### Download Verification
- Checksums mandatory in plans
- Mismatch = installation failure (security feature)
- TOCTOU mitigation: verify before extraction, atomic moves

### Plan Files as Trusted Input
- `--plan` flag trusts the plan file
- Malicious plan could point to malicious URLs
- Mitigation: plans should be generated via `tsuku eval`, not hand-crafted

### Supply Chain
- Plans provide audit trail of intended downloads
- Upstream changes detected via checksum mismatch
- Residual risk: initial plan creation inherits existing compromise

## Consequences

### Positive

- **Reproducibility by default**: Teams get identical installations without extra configuration. The architecture guarantees determinism rather than relying on careful implementation.

- **Testable recipes**: Recipe changes can be validated by comparing `tsuku eval` output. Golden file testing becomes possible without performing actual installations.

- **Air-gapped support**: Organizations with strict security requirements can pre-approve plans and execute them in isolated environments.

- **Audit trail**: Plans document exactly what will be downloaded. Security teams can review plans before approving installations.

- **Detection of upstream changes**: Checksum mismatches surface when upstream maintainers re-tag releases or modify assets. This converts a silent supply chain risk into a visible failure.

- **Foundation for lock files**: Lock files become a thin layer that stores plans for project-level coordination. The complex work (plan generation) is already done.

### Negative

- **Increased complexity**: The executor refactoring adds a new abstraction layer. Contributors must understand the plan concept when modifying installation logic.

- **Behavior change for pinned versions**: Users expecting `tsuku install ripgrep@14.1.0` to pick up upstream fixes must now use `--fresh`. This trades convenience for security.

- **Storage overhead**: Plans stored in state.json increase its size. For users with many installed tools, this may become noticeable.

- **Initial download during eval**: Computing checksums requires downloading assets during evaluation. This means `tsuku eval` is slower than a simple dry-run and requires network access.

### Neutral

- **Plan format is internal**: The JSON plan format is an implementation detail. We may change it between versions without breaking user workflows, since plans are typically consumed immediately after generation.

## Success Criteria

- [ ] `tsuku eval ripgrep@14.1.0` outputs deterministic plan
- [ ] Re-installing uses stored plan (no re-evaluation) for pinned versions
- [ ] Checksum mismatch fails installation
- [ ] `tsuku install --plan` works with pre-downloaded assets
- [ ] Recipe changes can be tested via plan comparison

## Implementation Issues

Milestone: [Deterministic Recipe Execution](https://github.com/tsukumogami/tsuku/milestone/15)

### Strategic Issues

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#367](https://github.com/tsukumogami/tsuku/issues/367) | Installation plans and tsuku eval command | None |
| [#368](https://github.com/tsukumogami/tsuku/issues/368) | Deterministic execution for pinned versions | #367 |
| [#370](https://github.com/tsukumogami/tsuku/issues/370) | Plan-based installation | #368 |

### Milestone 2: Decomposable Actions

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#436](https://github.com/tsukumogami/tsuku/issues/436) | Define Decomposable interface and primitive registry | None |
| [#437](https://github.com/tsukumogami/tsuku/issues/437) | Implement recursive decomposition algorithm | None |
| [#438](https://github.com/tsukumogami/tsuku/issues/438) | Implement Decompose() for github_archive | None |
| [#439](https://github.com/tsukumogami/tsuku/issues/439) | Implement Decompose() for download_archive, github_file, hashicorp_release | None |
| [#440](https://github.com/tsukumogami/tsuku/issues/440) | Update plan generator to decompose composite actions | None |
| [#441](https://github.com/tsukumogami/tsuku/issues/441) | Validate plans contain only primitives | None |
| [#442](https://github.com/tsukumogami/tsuku/issues/442) | Add deterministic flag to plan schema | None |

### Milestone 2: Ecosystem Primitives

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#443](https://github.com/tsukumogami/tsuku/issues/443) | Implement go_build ecosystem primitive | None |
| [#444](https://github.com/tsukumogami/tsuku/issues/444) | Implement cargo_build ecosystem primitive | None |
| [#445](https://github.com/tsukumogami/tsuku/issues/445) | Implement npm_exec ecosystem primitive | None |
| [#446](https://github.com/tsukumogami/tsuku/issues/446) | Implement pip_install ecosystem primitive | None |
| [#447](https://github.com/tsukumogami/tsuku/issues/447) | Implement gem_exec ecosystem primitive | None |
| [#448](https://github.com/tsukumogami/tsuku/issues/448) | Implement nix_realize ecosystem primitive | None |
| [#449](https://github.com/tsukumogami/tsuku/issues/449) | Implement cpan_install ecosystem primitive | None |

### Milestone 2: Deterministic Execution Infrastructure

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#470](https://github.com/tsukumogami/tsuku/issues/470) | Add plan cache infrastructure | None |
| [#471](https://github.com/tsukumogami/tsuku/issues/471) | Add GetCachedPlan to StateManager | None |
| [#472](https://github.com/tsukumogami/tsuku/issues/472) | Expose ResolveVersion public method | None |
| [#473](https://github.com/tsukumogami/tsuku/issues/473) | Add ExecutePlan with checksum verification | #470 |
| [#474](https://github.com/tsukumogami/tsuku/issues/474) | Add --fresh flag to install command | None |
| [#475](https://github.com/tsukumogami/tsuku/issues/475) | Add plan conversion helpers | #470 |
| [#477](https://github.com/tsukumogami/tsuku/issues/477) | Implement getOrGeneratePlan orchestration | #470, #471, #472, #474, #475 |
| [#478](https://github.com/tsukumogami/tsuku/issues/478) | Wire up plan-based installation flow | #473, #477 |
| [#479](https://github.com/tsukumogami/tsuku/issues/479) | Remove legacy Execute method | #478 |

### Milestone 3: Plan-Based Installation

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#506](https://github.com/tsukumogami/tsuku/issues/506) | Add plan loading utilities for external plans | None |
| [#507](https://github.com/tsukumogami/tsuku/issues/507) | Add --plan flag to install command | #506 |
| [#508](https://github.com/tsukumogami/tsuku/issues/508) | Document plan-based installation workflow | #507 |

## Future Work

Lock files for team version coordination are tracked separately in the vision repository. This design provides the infrastructure (installation plans) that lock files will build upon.

## Appendix A: Ecosystem Primitive Details

Each ecosystem primitive requires dedicated investigation to determine what can be locked at eval time, what reproducibility guarantees the ecosystem provides, and what residual non-determinism must be accepted.

### Summary

| Ecosystem | Lock File | Deterministic | Key Limitation |
|-----------|-----------|---------------|----------------|
| **Nix** | flake.lock + derivation hash | **Yes** | Binary cache trust |
| **Go** | go.sum (MVS checksums) | Yes (pure Go) | CGO, compiler version |
| **Cargo** | Cargo.lock (SHA-256) | No | Compiler, build scripts |
| **npm** | package-lock.json v3 | Partial | Native addons, scripts |
| **pip** | requirements.txt + hashes | No | Platform wheels, C extensions |
| **gem** | Gemfile.lock + checksums | No | Native extensions, hooks |
| **CPAN** | cpanfile.snapshot (Carton) | No | XS modules, Makefile.PL |

### Recommended Primitive Interfaces

```go
type GoBuildParams struct {
    Module      string   `json:"module"`
    Version     string   `json:"version"`
    Executables []string `json:"executables"`
    GoSum       string   `json:"go_sum"`
    GoVersion   string   `json:"go_version"`
    CGOEnabled  bool     `json:"cgo_enabled"`
    BuildFlags  []string `json:"build_flags"`
}

type CargoBuildParams struct {
    Crate        string   `json:"crate"`
    Version      string   `json:"version"`
    Executables  []string `json:"executables"`
    CargoLock    string   `json:"cargo_lock"`
    RustVersion  string   `json:"rust_version"`
    TargetTriple string   `json:"target_triple"`
}

type NpmExecParams struct {
    Package       string   `json:"package"`
    Version       string   `json:"version"`
    Executables   []string `json:"executables"`
    PackageLock   string   `json:"package_lock"`
    NodeVersion   string   `json:"node_version"`
    IgnoreScripts bool     `json:"ignore_scripts"`
}

type PipInstallParams struct {
    Package       string   `json:"package"`
    Version       string   `json:"version"`
    Executables   []string `json:"executables"`
    Requirements  string   `json:"requirements"`
    PythonVersion string   `json:"python_version"`
    OnlyBinary    bool     `json:"only_binary"`
}

type GemExecParams struct {
    Gem         string   `json:"gem"`
    Version     string   `json:"version"`
    Executables []string `json:"executables"`
    LockData    string   `json:"lock_data"`
    RubyVersion string   `json:"ruby_version"`
    Platforms   []string `json:"platforms"`
}

type NixRealizeParams struct {
    FlakeRef       string          `json:"flake_ref"`
    Executables    []string        `json:"executables"`
    DerivationPath string          `json:"derivation_path"`
    OutputPath     string          `json:"output_path"`
    FlakeLock      json.RawMessage `json:"flake_lock"`
    LockedRef      string          `json:"locked_ref"`
}

type CpanInstallParams struct {
    Distribution string `json:"distribution"`
    Version      string `json:"version"`
    Executables  []string `json:"executables"`
    Snapshot     string `json:"snapshot"`
    PerlVersion  string `json:"perl_version"`
    MirrorOnly   bool   `json:"mirror_only"`
    CachedBundle bool   `json:"cached_bundle"`
}
```

### Ecosystem-Specific Notes

**Go**: Minimum Version Selection (MVS) provides reproducible dependency resolution. Eval captures `go.sum` and module versions. Locked execution uses `CGO_ENABLED=0 GOPROXY=off go build -trimpath -buildvcs=false`. Nix provides the strongest reproducibility guarantees of all ecosystems.

**Cargo**: `Cargo.lock` with SHA-256 checksums. Locked execution via `cargo build --release --locked --offline`. Build scripts (`build.rs`) and proc macros are sources of non-determinism.

**npm**: `package-lock.json` v2/v3 with SHA-512 integrity hashes. `--ignore-scripts` is the default for security. Native addons (`node-gyp`) are the primary source of non-determinism.

**pip**: `requirements.txt` with hashes via pip-tools. `--only-binary :all:` prevents source distribution builds. Platform wheels and C extensions remain non-deterministic.

**gem**: `Gemfile.lock` with checksums (Bundler 2.6+). Native extensions and Ruby version ABI compatibility are the main concerns. Install hooks execute arbitrary code with no disable mechanism.

**Nix**: Fully deterministic via content-addressed derivations. `flake.lock` pins all inputs. Binary cache trust model is the only consideration.

**CPAN**: `cpanfile.snapshot` via Carton. XS module compilation and `Makefile.PL` decisions introduce non-determinism. Bundling and mirror-only mode provide partial mitigation.

## Appendix B: Ecosystem Investigation Template

For future ecosystem additions:

1. **Lock mechanism**: What file/format captures the dependency graph?
2. **Eval-time capture**: What commands extract lock information without installing?
3. **Locked execution**: What flags/env ensure the lock is respected?
4. **Reproducibility guarantees**: What does the ecosystem guarantee about builds?
5. **Residual non-determinism**: What can still vary between runs?
6. **Recommended primitive interface**: Struct definition with locked fields
7. **Security considerations**: Ecosystem-specific risks

## References

- Issue #227: Deterministic Recipe Resolution
- Issue #303: Asset pre-download with checksum capture
- Design: Container Validation Slice 2 (pre-download pattern)

### Superseded Designs

The following designs were consolidated into this document. They remain in `docs/designs/archive/` for historical reference:

- `DESIGN-installation-plans-eval.md` - Milestone 1 tactical design: plan data structures, action evaluability, platform flags, download cache
- `DESIGN-deterministic-execution.md` - Milestone 2 tactical design: two-phase evaluation, cache-by-resolution-output, ExecutePlan, checksum mismatch handling
- `DESIGN-decomposable-actions.md` - Milestone 2 tactical design: primitive tiers, Decomposable interface, recursive decomposition, ecosystem primitives
- `DESIGN-plan-based-installation.md` - Milestone 3 tactical design: plan loading, external validation, tool name handling, offline workflow
