---
summary:
  constraints:
    - Replace hardcoded packages with helper script calls
    - Keep bootstrap packages (curl, bash) hardcoded
    - Must work for both integration and integration-arm64-musl jobs
  integration_points:
    - .github/workflows/platform-integration.yml
    - .github/scripts/install-recipe-deps.sh (from #1574)
  risks:
    - Need to identify correct job sections in workflow
  approach_notes: |
    Find and replace hardcoded apk add commands with:
    1. Keep bootstrap packages (curl, bash)
    2. Add helper script call for recipe deps
---

# Implementation Context: Issue #1576

Migrate platform-integration.yml from hardcoded Alpine packages to recipe-driven deps.

## Changes needed

**Before:**
```yaml
apk add --no-cache zlib-dev yaml-dev libgcc
```

**After:**
```yaml
apk add --no-cache curl bash  # bootstrap only
./.github/scripts/install-recipe-deps.sh alpine ${{ matrix.library }}
```
