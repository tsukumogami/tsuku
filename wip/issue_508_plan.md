# Issue 508 Implementation Plan

## Summary

Create user-facing documentation for plan-based installation, covering air-gapped deployments and CI distribution workflows.

## Approach

Create a new guide document `docs/GUIDE-plan-based-installation.md` with comprehensive documentation, and add a brief reference in the README's "Reproducible Installations" section.

## Files to Create/Modify

- `docs/GUIDE-plan-based-installation.md` - New comprehensive guide
- `README.md` - Add brief mention and link

## Implementation Steps

- [x] Create docs/GUIDE-plan-based-installation.md with:
  - Overview of two-phase installation (eval/exec)
  - Basic usage examples (file and stdin)
  - Air-gapped deployment workflow
  - CI distribution workflow
  - Plan format reference
- [x] Update README.md to reference the new guide
- [x] Update design doc dependency graph (I508 done)

## Content Outline for GUIDE

1. **Overview**
   - Why plan-based installation (reproducibility, air-gapped)
   - Two-phase architecture (eval generates, exec installs)

2. **Basic Usage**
   - File-based: `tsuku install --plan plan.json`
   - Piping: `tsuku eval rg | tsuku install --plan -`

3. **Air-Gapped Deployment**
   - Generate plan on connected machine
   - Transfer plan + assets to air-gapped machine
   - Execute plan

4. **CI Distribution**
   - Generate plans during release
   - Store plans as release artifacts
   - Reproducible CI installations

5. **Plan Format Reference**
   - Key fields (tool, version, platform, steps)
   - Checksum verification

## Success Criteria

- [ ] Guide covers all acceptance criteria from issue
- [ ] Examples are clear and copy-pasteable
- [ ] README links to guide appropriately
