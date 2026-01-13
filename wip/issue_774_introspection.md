# Issue 774 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-structured-install-guide.md` (status: Planned)
- Design doc: `docs/designs/current/DESIGN-golden-plan-testing.md` (status: Current)
- Sibling issues reviewed: #772, #758
- Prior patterns identified: Family-aware golden file naming, regeneration script with --family flag

## Gap Analysis

### Minor Gaps

1. **Issue title mentions "system dependency recipes"** but the acceptance criteria is broader - it's about generating golden files for recipes using typed actions that previously couldn't have golden files (the ones migrated in #772).

2. **Regeneration script already supports --family flag**: The script at `scripts/regenerate-golden.sh` already handles family-aware recipes via `--linux-family` parameter and auto-detects family from `tsuku info --metadata-only`. The acceptance criteria item "Update `./scripts/regenerate-golden.sh` to accept `--family` flag" appears to already be complete.

3. **Naming convention established**: CONTRIBUTING.md documents the family-aware naming: `{version}-{os}-{family}-{arch}.json` (e.g., `v1.0.0-linux-debian-amd64.json`).

4. **Recipes were migrated with full Linux family coverage**: PR #855 converted docker.toml and test-tuples.toml to use all Linux PM actions (apt, dnf, pacman, apk, zypper), not just apt. This means golden files should be generated for all 5 Linux families per recipe.

### Moderate Gaps

None identified.

### Major Gaps

None identified.

## Recommendation

**Proceed** - No user input needed. The issue scope is clear: generate golden files for the recipes that were migrated in #772 (docker, test-tuples, cuda). The regeneration script infrastructure is already in place.

## Clarifications for Implementation

1. **cuda.toml was NOT migrated** in #772 - it still uses `require_system` because CUDA cannot be auto-installed. Per the design, recipes using `require_system` cannot have golden files generated (they require system packages). Issue #774's goal of "No recipes excluded due to system dependencies" may need interpretation:
   - Either: cuda should be excluded (it's a true system dependency)
   - Or: cuda needs different handling

2. **What "no recipes excluded" means**: Given the design context, this likely means the recipes that WERE migrated (docker, test-tuples) should now be able to generate golden files. cuda remains a special case as a true system dependency that cannot be sandbox-tested.

3. **Golden file paths for docker**: With 6 Linux families (debian, rhel, arch, alpine, suse) × 1 arch (amd64, since arm64 is skipped) + 2 macOS = ~7 golden files expected per version.
