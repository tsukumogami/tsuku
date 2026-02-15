# Issue 846 Implementation Plan

## Summary

Incorporate unique implementation details from four superseded designs into the parent design (DESIGN-deterministic-resolution.md), then fix the one broken cross-reference found in the codebase.

## Approach

The parent design covers the strategic vision and three milestones at a high level. The four superseded designs contain substantial tactical detail -- interface definitions, data structures, algorithms, ecosystem research, implementation tracks -- that isn't captured in the parent. Rather than copying everything verbatim (which would bloat the parent), the approach is to add targeted sections that preserve key architectural decisions and reference-quality implementation details, while keeping the parent coherent as a single authoritative document.

Content will be organized into new subsections under the existing structure, following the parent's three-milestone progression. The superseded designs map cleanly to milestones: installation-plans-eval -> Milestone 1, deterministic-execution + decomposable-actions -> Milestone 2, plan-based-installation -> Milestone 3.

## Files to Modify

- `docs/designs/current/DESIGN-deterministic-resolution.md` - Add implementation detail sections from all four superseded designs
- `docs/GUIDE-actions-and-primitives.md` - Fix broken cross-reference on line 615 (points to `../DESIGN-decomposable-actions.md` which moved to archive)
- `cmd/tsuku/eval.go` - Update comment on line 18 referencing old design name (low priority, code comment only)

## Files to Create

None.

## Implementation Steps

- [x] **Analyze and organize unique content from each superseded design into categories**
  - Data structures / interfaces (InstallationPlan, ResolvedStep, Decomposable, PlanCacheKey, ChecksumMismatchError, ecosystem primitive params)
  - Algorithms (recursive decomposition, two-phase evaluation, plan cache lookup)
  - Classification tables (primitive tiers, action evaluability, ecosystem investigation results)
  - Implementation patterns (ExecutePlan flow, getOrGeneratePlan orchestration, plan loading, external plan validation)
  - CLI details (platform override flags, --fresh flag, --plan flag with stdin support, tool name handling)

- [x] **Add "Detailed Data Structures" section to parent design** (after Solution Architecture)
  - InstallationPlan and ResolvedStep structs (from installation-plans-eval)
  - PlanCacheKey struct and cache key generation (from deterministic-execution)
  - Decomposable interface, Step, and EvalContext structs (from decomposable-actions)
  - ChecksumMismatchError type with recovery message (from deterministic-execution)

- [x] **Expand Milestone 1 section with implementation details**
  - PlanGenerator component and how it integrates with executor
  - Action evaluability classification table (fully evaluable vs non-evaluable)
  - Conditional step handling (when clause filtering)
  - Download cache interaction (eval populates cache, install reuses)
  - Platform override flags (--os, --arch) with validation requirements
  - Recipe hash computation

- [x] **Expand Milestone 2 section with implementation details**
  - Two-phase evaluation model: version resolution (always runs) + artifact verification (cached)
  - Cache lookup by resolution output (not user input), with rationale
  - Plan validation: multi-factor (recipe hash, format version, platform)
  - ExecutePlan() flow with per-step checksum verification
  - getOrGeneratePlan() orchestration pattern
  - --fresh flag semantics
  - Checksum mismatch: hard failure with recovery path via --fresh

- [x] **Add "Decomposable Actions" section under Milestone 2**
  - Primitive classification: Tier 1 (file operation primitives) and Tier 2 (ecosystem primitives) tables
  - Ecosystem composite action mapping table (go_install -> go_build, etc.)
  - Recursive decomposition algorithm description with cycle detection
  - github_archive decomposition example (key example from the design)
  - Plan structure showing primitives-only constraint and deterministic flag

- [x] **Add "Ecosystem Primitives" appendix or section**
  - Summary table of all ecosystems (Nix, Go, Cargo, npm, pip, gem, CPAN) with lock mechanisms and determinism levels
  - Recommended primitive struct definitions per ecosystem (GoBuildParams, CargoBuildParams, etc.)
  - Investigation template for future ecosystem additions
  - Note about planned but uncreated detailed research files

- [x] **Expand Milestone 3 section with implementation details**
  - Plan loading: file path and stdin support (- convention)
  - External plan validation (structural + platform + tool name checks)
  - Tool name handling: optional on CLI, defaults from plan
  - Offline installation workflow
  - Plan trust model (treat plans as code, verify source)

- [x] **Add consolidated implementation issues table**
  - Merge implementation issue tables from all four superseded designs into parent
  - Organize by milestone / track for clarity

- [x] **Fix broken cross-reference in docs/GUIDE-actions-and-primitives.md**
  - Line 615: change `../DESIGN-decomposable-actions.md` to `designs/current/DESIGN-deterministic-resolution.md`
  - Update the link text to reference the consolidated design

- [x] **Update code comment in cmd/tsuku/eval.go**
  - Line 18: change `DESIGN-installation-plans-eval.md` to `DESIGN-deterministic-resolution.md`

- [x] **Review parent design for coherence after consolidation**
  - Ensure new sections flow naturally within existing structure
  - Remove any redundancy between original and new content
  - Verify all internal cross-references within the document work
  - Confirm document table of contents / section hierarchy is logical

## Success Criteria

- [ ] Parent design contains all key implementation details from superseded designs (data structures, algorithms, classification tables, CLI details)
- [ ] No unique technical content of architectural significance remains only in superseded designs
- [ ] Parent design remains well-organized and coherent (not a copy-paste dump)
- [ ] Broken cross-reference in GUIDE-actions-and-primitives.md is fixed
- [ ] Code comment in eval.go references correct design document
- [ ] No other documents in the repo reference superseded designs at their old paths

## Open Questions

None. The scope is clear from the issue and introspection analysis. The parent design's existing structure provides natural insertion points for the consolidated content.
