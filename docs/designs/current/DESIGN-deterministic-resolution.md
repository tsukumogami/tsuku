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
- `--refresh` flag to force fresh evaluation
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

**Behavior change**: For pinned versions, users must use `--refresh` to pick up upstream changes. This is intentional - determinism is the default for exact version pins.

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

### Plan Storage

Plans are stored inline in `$TSUKU_HOME/state.json` alongside version information. The `tsuku plan export` command extracts plans for sharing or air-gapped use.

## Implementation Approach

Implementation proceeds in three milestones, each delivering incremental value.

### Milestone 1: Installation Plans and `tsuku eval`

Introduce the plan concept and the `tsuku eval` command. This milestone focuses on plan generation without changing installation behavior.

1. Define the installation plan JSON schema in `internal/plan/`
2. Implement plan generation by extracting evaluation logic from the executor
3. Add `tsuku eval` command that outputs plans to stdout
4. Reuse `PreDownloader` from `internal/validate/predownload.go` for checksum computation

After this milestone, recipe authors can test changes by comparing `tsuku eval` output against expected plans.

### Milestone 2: Deterministic Execution

Refactor the executor to use plans internally. All installations become plan-based, making determinism architectural.

1. Split executor into `PlanGenerator` and `PlanExecutor` components
2. Modify `tsuku install` to generate a plan, then execute it
3. Store plans in state.json for installed tools
4. Add `--refresh` flag to force re-evaluation for pinned versions
5. Implement checksum verification with failure on mismatch

After this milestone, re-installing a tool with a pinned version reuses the stored plan.

### Milestone 3: Plan-Based Installation

Enable installation from externally-provided plans for air-gapped environments.

1. Add `--plan` flag to `tsuku install`
2. Support reading plans from file or stdin
3. Implement offline installation when cached assets are available
4. Add `tsuku plan show` and `tsuku plan export` commands

After this milestone, teams can generate plans on connected machines and execute them in isolated environments.

## Open Questions

1. **Checksum source**: Should `tsuku eval` download files to compute checksums, or rely on upstream-provided checksums where available?

2. **Plan storage**: Inline in state.json vs separate plan files? Current recommendation: inline with export capability.

3. **Multi-platform plans**: Should `tsuku eval` be able to generate plans for other platforms (via API queries), or only the current platform?

4. **Cache integration**: Should plans be cached separately from downloaded artifacts?

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

- **Behavior change for pinned versions**: Users expecting `tsuku install ripgrep@14.1.0` to pick up upstream fixes must now use `--refresh`. This trades convenience for security.

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

- Milestone: [Deterministic Recipe Execution](https://github.com/tsukumogami/tsuku/milestone/15)
- #367: Installation plans and tsuku eval command
- #368: Deterministic execution for pinned versions
- #370: Plan-based installation

## Future Work

Lock files for team version coordination are tracked separately in the vision repository. This design provides the infrastructure (installation plans) that lock files will build upon.

## References

- Issue #227: Deterministic Recipe Resolution
- Issue #303: Asset pre-download with checksum capture
- Design: Container Validation Slice 2 (pre-download pattern)
