# Issue 802 Introspection

## Context Reviewed
- Design docs:
  - `docs/DESIGN-sandbox-dependencies.md` (for #703)
  - `docs/DESIGN-sandbox-implicit-dependencies.md` (for #805)
- Sibling issues reviewed: #770, #771, #757, #767, #768, #769
- Prior work: PR #804, PR #808
- Related issues: #805 (closed), #703 (closed), #806 (closed)

## Prior Patterns Identified

1. **Test script pattern established in PR #804**: Two test scripts (`test-cmake-provisioning.sh`, `test-readline-provisioning.sh`) were already migrated to the new sandbox pattern using:
   - `tsuku eval --linux-family <family> --install-deps > plan.json`
   - `tsuku install --plan plan.json --sandbox --force`

2. **Multi-family validation structure**: Test scripts iterate over `FAMILIES=("debian" "rhel" "arch" "alpine" "suse")` and generate family-specific plans.

3. **No test recipes created**: The migrated scripts use existing recipes (`cmake`, `ninja`, `sqlite`) directly rather than creating dedicated test recipes like `build-essentials.toml`.

## Gap Analysis

### Minor Gaps
None - the approach is clear from prior implementations.

### Moderate Gaps

1. **Issue comment blockers are obsolete**: The comment on issue #802 from 2026-01-04 states the issue is blocked by #805 and #703. However:
   - #805 was fixed by PR #808 (merged 2026-01-05T00:13:42Z)
   - #703 was also addressed by the same PR (DESIGN-sandbox-dependencies.md confirms this)
   - Both issues are now CLOSED

   **The blockers are resolved** but the issue still has the outdated blocking comment.

2. **Partial completion not documented**: PR #804 already completed part of this issue:
   - `test-cmake-provisioning.sh` - DONE (migrated to sandbox pattern)
   - `test-readline-provisioning.sh` - DONE (migrated to sandbox pattern)

   The acceptance criteria checkboxes don't reflect this progress.

3. **Issue #806 was closed**: Issue #806 (multi-family CI expansion) is now closed, which may have completed some or all of the intended validation work.

### Major Gaps

**None identified** - The issue's core intent (migrate remaining test scripts to sandbox containers) remains valid. The blockers have been resolved.

## Remaining Work Assessment

Based on the current state of test scripts:

| Script | Status | Notes |
|--------|--------|-------|
| `test-cmake-provisioning.sh` | Already migrated | Uses eval + sandbox pattern |
| `test-readline-provisioning.sh` | Already migrated | Uses eval + sandbox pattern |
| `test-checksum-pinning.sh` | Needs migration | Has apt-get in Dockerfile |
| `test-docker-system-dep.sh` | May not apply | Tests system deps, not build deps |
| `test-cuda-system-dep.sh` | May not apply | Tests system deps, not build deps |
| `test-homebrew-recipe.sh` | Needs migration | Has apt-get in Dockerfile |

**Key observation**: `test-docker-system-dep.sh` and `test-cuda-system-dep.sh` intentionally test system dependency behavior (detecting if docker/cuda is installed vs not installed). These scripts may not be candidates for sandbox migration because their purpose is to test tsuku's handling of pre-existing system dependencies, not building tools.

The real scope of remaining work is:
1. `test-checksum-pinning.sh` - migrate apt-get calls to recipe dependencies
2. `test-homebrew-recipe.sh` - migrate apt-get calls to recipe dependencies (needs patchelf)

## Recommendation

**Proceed** - with scope clarification

The blockers (#805, #703) have been resolved. The infrastructure is complete. Two of the six scripts listed have already been migrated.

However, the issue should be amended to:
1. Remove the blocking comment (or update it to reflect resolved state)
2. Mark the partially completed scripts as done
3. Clarify which scripts actually apply (docker/cuda system dep tests may be intentionally different)
4. Update scope to remaining 2-4 scripts

## Proposed Amendments

If user approves, add this comment to the issue:

```
## Status Update (Post-Introspection)

### Blockers Resolved
- #805 fixed by PR #808 (merged 2026-01-05)
- #703 addressed by same PR
- Both issues now closed

### Completed Work
The following were migrated in PR #804:
- [x] test-cmake-provisioning.sh
- [x] test-readline-provisioning.sh

### Remaining Work
Scripts that need migration:
- [ ] test-checksum-pinning.sh
- [ ] test-homebrew-recipe.sh

### Scope Clarification
These scripts intentionally test system dependency detection and should NOT be migrated:
- test-docker-system-dep.sh (tests docker system dep recipe behavior)
- test-cuda-system-dep.sh (tests cuda system dep recipe behavior)
```
