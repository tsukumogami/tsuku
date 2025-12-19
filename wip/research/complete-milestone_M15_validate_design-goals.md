# Milestone 15: Deterministic Recipe Execution - Design Goal Validation

**Milestone**: M15 - Deterministic Recipe Execution
**Design Document**: docs/DESIGN-deterministic-resolution.md
**Validation Date**: 2025-12-18
**Status**: PASS with minor findings

## Executive Summary

Milestone 15 successfully delivered all three strategic milestones outlined in the design document:
1. **Installation Plans and `tsuku eval`** - Fully implemented
2. **Deterministic Execution** - Fully implemented with two-phase architecture
3. **Plan-Based Installation** - Fully implemented

The implementation went beyond the design goals by implementing action decomposition (composite → primitives), ecosystem-specific lockfile capture for evaluability, and nested dependency plans for self-contained installations. The core vision of "a recipe is a program that produces a deterministic installation plan" has been achieved.

**Findings**: 2 minor architectural enhancements beyond design scope (not gaps)

## Design Document Analysis

### Stated Vision and Goals

From DESIGN-deterministic-resolution.md:

> **A recipe is a program that produces a deterministic installation plan.**
>
> Tsuku should separate recipe evaluation from plan execution, making determinism the default rather than an opt-in feature.

**Core Deliverables**:
1. Two-phase installation model: evaluation (dynamic) → execution (deterministic)
2. `tsuku eval` command for plan generation
3. Plan storage in state.json
4. `tsuku plan show/export` commands
5. Plan-based installation via `--plan` flag
6. Checksum verification during execution
7. Cached plan reuse for exact versions

### Success Criteria (from design doc)

- [x] `tsuku eval ripgrep@14.1.0` outputs deterministic plan
- [x] Re-installing uses stored plan (no re-evaluation) for pinned versions
- [x] Checksum mismatch fails installation
- [x] `tsuku install --plan` works with pre-downloaded assets
- [x] Recipe changes can be tested via plan comparison

**Result**: All success criteria met.

## Implementation Scope

### Closed Issues in M15

**Total**: 46 closed issues

**Key categories**:
1. **Core infrastructure** (Issues #367, #368, #370, #401-407, #470-479)
   - Plan data types (InstallationPlan, ResolvedStep, Platform)
   - Plan generator with decomposition
   - Plan cache infrastructure
   - Plan validation
   - ExecutePlan method with checksum verification
   - getOrGeneratePlan orchestration
   - CLI commands (eval, plan show/export, install --plan)

2. **Action decomposition** (Issues #436-444)
   - Decomposable interface and primitive registry
   - Recursive decomposition algorithm
   - Composite action decomposition (download_archive, github_archive, github_file, hashicorp_release)
   - Primitive flag on plan schema

3. **Ecosystem primitives** (Issues #443-449, #608-613)
   - go_build, cargo_build, npm_exec, pip_install, gem_exec, nix_realize, cpan_install
   - Lockfile capture for evaluability: npm (package-lock.json), pipx (requirements.txt), gem (Gemfile.lock), cargo (Cargo.lock), go (go.sum), nix (flake.lock)

4. **Plan enhancements** (Issues #598, #621)
   - Homebrew bottle evaluability with checksum verification
   - Nested dependency plans for self-contained installations

5. **Bug fixes** (Issues #600, #614)
   - Downloader integration in install flow
   - strip_dirs handling in plan execution

### Key Implementation Files

**CLI Layer** (`cmd/tsuku/`):
- `eval.go` - `tsuku eval` command with platform flags (--os, --arch)
- `plan.go` - `tsuku plan show/export` commands
- `plan_install.go` - Plan-based installation (`--plan` flag)
- `install_deps.go` - getOrGeneratePlan orchestration with cache lookup
- `install.go` - Enhanced with `--fresh` and `--plan` flags

**Executor Layer** (`internal/executor/`):
- `plan.go` - InstallationPlan, DependencyPlan, ResolvedStep, validation
- `plan_generator.go` - GeneratePlan with decomposition and dependency resolution
- `plan_cache.go` - Cache key computation and validation
- `plan_conversion.go` - Conversion between executor and storage plans
- `executor.go` - ExecutePlan with checksum verification and dependency installation

**Actions Layer** (`internal/actions/`):
- `decomposable.go` - Decomposable interface, primitive registry, IsDeterministic
- `composites.go` - Decompose implementations for download_archive, github_archive, etc.
- `eval_deps.go` - Eval-time dependency resolution
- Ecosystem actions with Decompose methods (npm_install.go, cargo_install.go, etc.)

**State Layer** (`internal/install/`):
- `state.go` - Plan storage in VersionState

## Capability Comparison: Design vs. Implementation

### Milestone 1: Installation Plans and `tsuku eval`

| Design Capability | Implementation Status | Evidence |
|------------------|----------------------|----------|
| `tsuku eval <tool>[@version]` command | ✅ Implemented | cmd/tsuku/eval.go (lines 35-183) |
| Installation plan format (JSON) | ✅ Implemented | internal/executor/plan.go (lines 20-116) |
| Plan captures URLs, checksums, steps | ✅ Implemented | ResolvedStep includes URL, Checksum, Size fields |
| Plan storage in state.json | ✅ Implemented | internal/install/state.go (line 20: Plan field in VersionState) |
| `tsuku plan show` for inspection | ✅ Implemented | cmd/tsuku/plan.go (lines 20-146) |
| `tsuku plan export` for sharing | ✅ Implemented | cmd/tsuku/plan.go (lines 37-321) |
| Cross-platform plan generation | ✅ Implemented | eval.go supports --os and --arch flags |
| Recipe testing via plan comparison | ✅ Implemented | JSON output enables diff-based testing |

**Finding 1**: Implementation added action decomposition (composite → primitive) which was not in the original design. This enhances determinism by ensuring plans only contain primitive actions.

### Milestone 2: Deterministic Execution

| Design Capability | Implementation Status | Evidence |
|------------------|----------------------|----------|
| Two-phase architecture (eval/exec) | ✅ Implemented | install_deps.go getOrGeneratePlan + ExecutePlan |
| All installs go through plan generation | ✅ Implemented | installWithDependencies calls getOrGeneratePlan (line 400) |
| Cached plan reuse for pinned versions | ✅ Implemented | getOrGeneratePlanWith checks cache unless --fresh (lines 91-104) |
| `--fresh` flag to force re-evaluation | ✅ Implemented | install.go registers freshFlag |
| Checksum verification during execution | ✅ Implemented | ExecutePlan validates checksums (executor.go lines 277-436) |
| Checksum mismatch = failure | ✅ Implemented | ChecksumMismatchError type with detailed error message |
| Version constraint behavior | ✅ Implemented | Phase 1 resolution (line 78-85) then cache lookup |

**Architectural Note**: The design stated:
> `tsuku install foo` becomes functionally equivalent to `tsuku eval foo | tsuku install --plan -`

Implementation achieves this through `getOrGeneratePlan` which generates a plan internally, then `ExecutePlan` executes it. The equivalence is functional but not literal piping.

### Milestone 3: Plan-Based Installation

| Design Capability | Implementation Status | Evidence |
|------------------|----------------------|----------|
| `tsuku install --plan <file>` | ✅ Implemented | plan_install.go runPlanBasedInstall |
| Piping support (`--plan -`) | ✅ Implemented | plan_utils.go loadPlanFromSource handles stdin |
| Offline installation support | ✅ Implemented | ExecutePlan uses DownloadCache, plan has checksums |
| Air-gapped deployment workflow | ✅ Implemented | eval generates plan with checksums, install --plan executes |

### Integration with Existing Infrastructure

| Integration Point | Design Expectation | Implementation Status |
|------------------|-------------------|----------------------|
| Download infrastructure reuse | Use download.go with SSRF protection | ✅ Implemented - PreDownloader used |
| LLM validation alignment | Reuse PreDownloader from validate/ | ✅ Implemented - PreDownloaderAdapter |
| Executor refactoring | Separate plan generation from execution | ✅ Implemented - GeneratePlan + ExecutePlan |
| Cache integration | Plans cached separately from artifacts | ✅ Implemented - DownloadCache used |

## Gaps and Deviations

### No Significant Gaps Found

All design deliverables were implemented. The implementation delivered on all three milestones.

### Enhancements Beyond Design Scope

**Finding 2**: Nested dependency plans (#621) - The design mentioned dependencies but did not specify nested plans. The implementation went further by creating self-contained plans that include all dependency installation steps in a tree structure.

Evidence:
- `DependencyPlan` type in plan.go (lines 64-79)
- Recursive dependency installation in executor.go (lines 495-609)
- `generateDependencyPlans` in plan_generator.go

**Rationale**: This makes plans truly self-contained for air-gapped deployments, exceeding the design goal.

### Architectural Decisions Made During Implementation

1. **Plan format version 3**: Started at v2 (decomposed primitives), evolved to v3 (nested dependencies)
2. **Deterministic flag computation**: Plan-level deterministic flag computed from all steps (including nested deps)
3. **Tier-based primitives**: Actions classified as core (deterministic) vs. ecosystem (residual non-determinism)
4. **Eval-time dependencies**: npm/pipx/cargo/go/gem/nix actions can trigger dependency installation during plan generation

These decisions align with the design's goal of "determinism by default" and "reuse existing infrastructure."

## Verification of Key Features

### Feature: Checksum Verification

**Code path**: executor.go ExecutePlan → executeDownloadWithVerification (lines 396-436)

**Verification**:
```go
// Line 424-432: Strict checksum comparison
if actualChecksum != expectedChecksum {
    return &ChecksumMismatchError{
        Tool:             plan.Tool,
        Version:          plan.Version,
        URL:              step.URL,
        ExpectedChecksum: expectedChecksum,
        ActualChecksum:   actualChecksum,
    }
}
```

**Result**: ✅ Checksum mismatch causes installation failure as designed.

### Feature: Plan Caching

**Code path**: install_deps.go getOrGeneratePlanWith (lines 61-127)

**Verification**:
```go
// Lines 91-104: Cache lookup with validation
if !cfg.Fresh {
    cachedPlan, err := cacheReader.GetCachedPlan(cfg.Tool, resolvedVersion)
    if err == nil && cachedPlan != nil {
        execPlan := executor.FromStoragePlan(cachedPlan)
        if execPlan != nil {
            if err := executor.ValidateCachedPlan(execPlan, cacheKey); err == nil {
                printInfof("Using cached plan for %s@%s\n", cfg.Tool, resolvedVersion)
                return execPlan, nil
            }
        }
    }
}
```

**Result**: ✅ Cached plans reused for exact versions unless `--fresh` specified.

### Feature: Cross-Platform Plan Generation

**Code path**: eval.go (lines 31-66, 107-115)

**Verification**:
```go
// Lines 31-33: Platform flags
var evalOS string
var evalArch string

// Lines 63-64: Flag registration
evalCmd.Flags().StringVar(&evalOS, "os", "", "Target operating system...")
evalCmd.Flags().StringVar(&evalArch, "arch", "", "Target architecture...")

// Lines 107-115: Validation
if err := ValidateOS(evalOS); err != nil { ... }
if err := ValidateArch(evalArch); err != nil { ... }
```

**Result**: ✅ Plans can be generated for other platforms via --os and --arch flags.

### Feature: Plan Validation

**Code path**: plan.go ValidatePlan (lines 224-376)

**Verification**:
- Format version check (line 228-234)
- Platform compatibility (line 237-244)
- Composite action rejection (line 249-266)
- Unknown action rejection (line 269-276)
- Checksum requirement for download_file (line 279-296)
- Recursive dependency validation (line 299-376)

**Result**: ✅ Comprehensive validation ensures plan integrity.

## Security Considerations

The design document outlined several security considerations. Here's how they were addressed:

| Security Concern | Design Mitigation | Implementation Status |
|-----------------|------------------|----------------------|
| Download verification | Checksums mandatory in plans | ✅ ValidatePlan rejects download_file without checksum |
| TOCTOU mitigation | Verify before extraction, atomic moves | ✅ Checksum verified in ExecutePlan before extraction |
| Plan files as trusted input | Plans should be generated via tsuku eval | ✅ Documentation emphasizes this (plan.go line 42-44) |
| Supply chain integrity | Plans provide audit trail | ✅ Plans include RecipeHash, GeneratedAt, full step list |
| Upstream change detection | Checksum mismatch indicates changes | ✅ ChecksumMismatchError with clear messaging |

**Result**: All security considerations addressed in implementation.

## Test Coverage

Based on file analysis, comprehensive test coverage exists:

**Unit tests**:
- `plan_test.go` - Plan validation
- `plan_generator_test.go` - Plan generation with decomposition
- `plan_cache_test.go` - Cache key computation
- `plan_conversion_test.go` - Storage conversion
- `eval_test.go` - CLI eval command
- `plan_test.go` (cmd) - CLI plan commands
- `install_deps_test.go` - getOrGeneratePlan orchestration

**Integration tests**:
- `eval_plan_integration_test.go` - Full eval/install workflow

## Open Questions (from Design)

The design document listed 4 open questions. Here's their resolution:

1. **Checksum source**: "Should tsuku eval download files to compute checksums?"
   - **Resolution**: Yes - PreDownloader downloads files and computes SHA256 checksums
   - **Evidence**: eval.go line 149 creates PreDownloader

2. **Plan storage**: "Inline in state.json vs separate plan files?"
   - **Resolution**: Inline with export capability
   - **Evidence**: state.go Plan field in VersionState, plan export command

3. **Multi-platform plans**: "Should tsuku eval generate plans for other platforms?"
   - **Resolution**: Yes, via --os and --arch flags
   - **Evidence**: eval.go lines 31-33, 63-64

4. **Cache integration**: "Should plans be cached separately from downloaded artifacts?"
   - **Resolution**: Plans cached in state.json, artifacts in DownloadCache
   - **Evidence**: state.go Plan field, DownloadCache usage in plan_generator.go

**Result**: All open questions resolved with implementation decisions.

## Conclusion

### Deliverables Assessment

| Milestone | Design Scope | Implementation | Status |
|-----------|-------------|----------------|---------|
| M1: Plans & eval | 8 capabilities | All 8 + decomposition | ✅ Exceeded |
| M2: Deterministic exec | 7 capabilities | All 7 implemented | ✅ Complete |
| M3: Plan-based install | 4 capabilities | All 4 + nested deps | ✅ Exceeded |

### Final Verdict

**Status**: PASS

The implementation fully delivers on the design document's vision and all strategic milestones. The two-phase installation model (eval/exec) is in place, determinism is the default, and plan-based installation enables air-gapped deployments.

**Enhancements beyond design scope**:
1. Action decomposition to primitives (improves plan determinism)
2. Nested dependency plans (enables fully self-contained plans)
3. Ecosystem-specific lockfile capture (extends evaluability)
4. Tier-based determinism classification (documents residual non-determinism)

These enhancements align with the design's core principles and improve the overall solution without introducing technical debt or deviating from the vision.

### Recommended Next Steps

Per the design document's "Future Work" section:
> Lock files for team version coordination are tracked separately in the vision repository. This design provides the infrastructure (installation plans) that lock files will build upon.

The milestone has successfully delivered the foundation for future lock file functionality.
