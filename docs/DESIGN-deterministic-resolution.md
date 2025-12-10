# Design: Deterministic Recipe Resolution and Installation Plans

- **Status**: Accepted
- **Issue**: #227
- **Author**: @dangazineu
- **Created**: 2025-12-09
- **Scope**: Strategic

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

**Goal**: Make plan replay the default for re-installations when version is pinned.

**Deliverables**:
- Re-install uses stored plan when version constraint exactly matches resolved version
- `--refresh` flag to force fresh evaluation
- Checksum verification during download
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

## Success Criteria

- [ ] `tsuku eval ripgrep@14.1.0` outputs deterministic plan
- [ ] Re-installing uses stored plan (no re-evaluation) for pinned versions
- [ ] Checksum mismatch fails installation
- [ ] `tsuku install --plan` works with pre-downloaded assets
- [ ] Recipe changes can be tested via plan comparison

## Future Work

Lock files for team version coordination are tracked separately in the vision repository. This design provides the infrastructure (installation plans) that lock files will build upon.

## References

- Issue #227: Deterministic Recipe Resolution
- Issue #303: Asset pre-download with checksum capture
- Design: Container Validation Slice 2 (pre-download pattern)
