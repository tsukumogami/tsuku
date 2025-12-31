# Research Document Relevance Review

## Context

The `linux_family` targeting model has been adopted. This review categorizes all research documents by their continued relevance now that the core decision has been made and captured in the design documents.

## Summary

| Status | Count | Description |
|--------|-------|-------------|
| Keep | 7 | Still provides valuable context or decisions to preserve |
| Archive | 35 | Historical value only, decisions captured in design docs |
| Delete | 9 | Superseded, duplicative, or no longer relevant |

## Detailed Review

### Core Decision Documents

| File | Status | Reason |
|------|--------|--------|
| `DISCUSSION-targeting-model.md` | **Keep** | Authoritative record of the targeting model decision process. Documents the evolution from Model A to Model C (linux_family). Essential design rationale. |
| `findings_targeting-model-recommendation.md` | Archive | Original recommendation for Model B (package_manager). Superseded by DISCUSSION which documents the evolution to Model C (linux_family). Historical value only. |
| `RESEARCH-HANDOFF.md` | Archive | Phase coordination document. All phases completed. Research outcomes captured in DISCUSSION and design docs. |

### Research Specifications (SPEC-*.md)

| File | Status | Reason |
|------|--------|--------|
| `SPEC-prior-art.md` | Archive | Research scope definition. Work completed, findings captured in `findings_prior-art-*.md`. |
| `SPEC-binary-survey.md` | Archive | Research scope definition. Work completed, findings captured in `findings_binary-*.md`. |
| `SPEC-package-managers.md` | Archive | Research scope definition. Work completed, findings captured in `findings_pm-*.md`. |
| `SPEC-ecosystem-analysis.md` | Archive | Research scope definition. Work completed, findings captured in `findings_*.md`. |
| `SPEC-package-manager-baseline.md` | Archive | Phase 2 research spec. Empirical testing completed, findings captured in relevant documents. |

### Investigation Paths (investigation_*.md)

| File | Status | Reason |
|------|--------|--------|
| `investigation_linux-hierarchy.md` | Archive | Question catalog. Decisions made and captured in DISCUSSION. Historical reference for why family model was chosen. |
| `investigation_binary-compatibility.md` | Archive | Question catalog. Binary strategy questions answered in `findings_binary-recommendations.md`. |
| `investigation_fragmentation.md` | Archive | Question catalog. Fragmentation concerns addressed in 80/20 analysis and tier recommendations. |
| `investigation_package-ecosystems.md` | Archive | Question catalog. Package manager questions answered in design docs. |

### Findings: Prior Art

| File | Status | Reason |
|------|--------|--------|
| `findings_prior-art-matrix.md` | **Keep** | Valuable reference comparing Ansible, Puppet, Chef, Nix, Homebrew, asdf, rustup approaches. Useful for future architecture decisions. |
| `findings_prior-art-patterns.md` | Archive | Pattern catalog. Key patterns adopted in design docs. |
| `findings_prior-art-antipatterns.md` | Archive | Anti-pattern catalog. Lessons incorporated into design decisions. |
| `findings_prior-art-recommendations.md` | **Keep** | Synthesized recommendations with adopt/avoid table. Useful reference for implementation. |

### Findings: Binary Compatibility

| File | Status | Reason |
|------|--------|--------|
| `findings_binary-survey-data.md` | Archive | Raw survey data. Key statistics extracted to recommendations doc. |
| `findings_binary-patterns.md` | Archive | Pattern analysis. Summary in recommendations doc. |
| `findings_binary-compatibility.md` | Archive | Compatibility test results. Summary in recommendations doc. |
| `findings_binary-recommendations.md` | **Keep** | Actionable binary selection strategy. Guides asset selection logic implementation. |

### Findings: Package Manager Baselines

| File | Status | Reason |
|------|--------|--------|
| `findings_universal-baseline.md` | **Keep** | Critical empirical data on what exists on any Linux. Required for bootstrap logic and minimum requirements. |
| `findings_pm-detection.md` | Archive | Detection algorithm. Incorporated into design and will be in implementation. |
| `findings_pm-baselines.md` | Archive | Per-PM baseline data. Key facts in universal-baseline and design docs. |
| `findings_package-name-mapping.md` | **Keep** | Essential reference for package name translation across PMs. Required for implementation. |
| `findings_bootstrap-requirements.md` | Archive | Bootstrap requirements. Captured in findings_universal-baseline and design docs. |
| `findings_no-pm-strategy.md` | Archive | Strategy for NixOS/Gentoo. Decisions captured in design docs. |

### Findings: Ecosystem Analysis

| File | Status | Reason |
|------|--------|--------|
| `findings_ci-providers.md` | Archive | CI provider matrix. Data incorporated into 80/20 analysis. |
| `findings_container-ecosystem.md` | Archive | Container ecosystem data. Summary in 80/20 analysis. |
| `findings_cloud-defaults.md` | Archive | Cloud provider defaults. Summary in 80/20 analysis. |
| `findings_80-20-analysis.md` | **Keep** | Essential tier recommendations (Tier 1/2/3 distros). Guides testing and support scope. |
| `findings_ecosystem-recommendations.md` | Delete | Duplicative of 80/20 analysis. Superseded. |

### Findings: Package Manager Inventory

| File | Status | Reason |
|------|--------|--------|
| `findings_package-manager-inventory.md` | Archive | PM inventory. Key data in design docs and package-name-mapping. |
| `findings_distro-pm-mapping.md` | Archive | Distro-to-PM mapping. Core of linux_family model in design docs. |
| `findings_pm-compatibility.md` | Archive | Compatibility matrix. Tier decisions in 80/20 and design docs. |
| `findings_pm-action-coverage.md` | Delete | Action coverage analysis. Superseded by DESIGN-system-dependency-actions.md. |
| `findings_pm-recommendations.md` | Delete | PM recommendations. Superseded by design docs. |

### System Dependencies Research (system-deps_*.md)

| File | Status | Reason |
|------|--------|--------|
| `system-deps_api-design.md` | Delete | API design assessment. Decisions captured in DESIGN-system-dependency-actions.md. |
| `system-deps_platform-detection.md` | Delete | Platform detection research. Superseded by design docs and DISCUSSION. |
| `system-deps_security.md` | Delete | Security considerations. Incorporated into design docs. |
| `system-deps_authoring-ux.md` | Delete | Authoring UX research. Incorporated into design docs. |
| `system-deps_implementation.md` | Delete | Implementation research. Superseded by design docs. |

### Design Fit Assessment (design-fit_*.md)

| File | Status | Reason |
|------|--------|--------|
| `design-fit_usecase-alignment.md` | Delete | Use case alignment review. Purpose served, in design docs. |
| `design-fit_current-behavior.md` | Archive | Current behavior analysis. Historical reference for migration. |
| `design-fit_sandbox-executor.md` | Archive | Sandbox executor analysis. Context for DESIGN-structured-install-guide.md. |

### Planning Documents (plan_platform-distro_*.md)

| File | Status | Reason |
|------|--------|--------|
| `plan_platform-distro_golden-files.md` | Archive | Golden file planning. Implementation details in design docs. |
| `plan_platform-distro_ux-implications.md` | Archive | UX implications. Incorporated into design docs. |
| `plan_platform-distro_container-strategy.md` | Archive | Container strategy. In DESIGN-structured-install-guide.md. |
| `plan_platform-distro_detection-challenges.md` | Archive | Detection challenges. Addressed in design and implementation. |

### Review Documents (structured-guide_*.md, final-review_*.md)

| File | Status | Reason |
|------|--------|--------|
| `structured-guide_phase-review.md` | Archive | Phase review document. Review process completed. |
| `structured-guide_alignment-review.md` | Archive | Alignment review. Design finalized. |
| `structured-guide_sandbox-accuracy.md` | Archive | Sandbox accuracy review. Incorporated into final design. |
| `final-review_clarity.md` | Archive | Clarity review. Design finalized. |
| `final-review_consistency.md` | Archive | Consistency review. Design finalized. |
| `final-review_completeness.md` | Archive | Completeness review. Design finalized. |
| `final-review_technical.md` | Archive | Technical review. Design finalized. |

## Recommended Actions

### 1. Keep (7 files)

These files should be preserved as they contain valuable reference material:

1. `DISCUSSION-targeting-model.md` - Authoritative decision record
2. `findings_prior-art-matrix.md` - Tool comparison reference
3. `findings_prior-art-recommendations.md` - Adopt/avoid guidance
4. `findings_binary-recommendations.md` - Binary selection strategy
5. `findings_universal-baseline.md` - Critical baseline data
6. `findings_package-name-mapping.md` - PM translation table
7. `findings_80-20-analysis.md` - Tier recommendations

### 2. Archive (35 files)

These files have historical value and document the research process. They should be:
- Moved to `wip/research/archive/` or
- Retained in current location with clear "historical" header

### 3. Delete (9 files)

These files are superseded by design docs and provide no additional value:
- `findings_ecosystem-recommendations.md`
- `findings_pm-action-coverage.md`
- `findings_pm-recommendations.md`
- `system-deps_api-design.md`
- `system-deps_platform-detection.md`
- `system-deps_security.md`
- `system-deps_authoring-ux.md`
- `system-deps_implementation.md`
- `design-fit_usecase-alignment.md`

## Notes

1. **Design docs are authoritative**: All key decisions are now captured in:
   - `docs/DESIGN-system-dependency-actions.md`
   - `docs/DESIGN-structured-install-guide.md`

2. **Keep files support implementation**: The 7 "Keep" files provide reference data needed during implementation that isn't fully captured in design docs.

3. **Archive vs Delete**: Archive is appropriate when the file documents the reasoning process or contains raw data that might be useful for future reference. Delete is appropriate when the content is fully superseded with no unique value.
