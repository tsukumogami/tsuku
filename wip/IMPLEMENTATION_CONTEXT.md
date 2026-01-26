---
summary:
  constraints:
    - R2 object key convention: plans/{category}/{recipe}/v{version}/{platform}.json
    - Script should work with read-only R2 credentials (detection only, not deletion)
    - Must integrate with existing R2 helper scripts pattern
    - Dry-run by default - reports orphans without deleting
  integration_points:
    - scripts/r2-*.sh - follow existing script conventions
    - recipes/ - source of truth for valid registry recipes
    - internal/recipe/recipes/ - embedded recipes (not relevant for orphan detection)
    - AWS CLI - for listing R2 objects
  risks:
    - False positives if recipe exists but under different name
    - Platform detection accuracy - need to parse recipe for supported platforms
    - Large number of R2 objects may require pagination
  approach_notes: |
    1. List all objects in R2 bucket under plans/
    2. Parse object keys to extract recipe name, version, platform
    3. For each object, check if recipe exists in recipes/ directory
    4. For recipes that exist, verify platform is still supported
    5. Output list of orphaned object keys for cleanup
---

# Implementation Context: Issue #1101

**Source**: docs/designs/DESIGN-r2-golden-storage.md

## Key Points

- This is Phase 5 (Cleanup Automation) of the R2 migration
- Orphan detection identifies golden files that no longer have corresponding recipes
- Two types of orphans:
  1. Recipe deleted - entire recipe directory gone from recipes/
  2. Platform dropped - recipe exists but no longer supports that platform
- Script outputs object keys suitable for piping to deletion commands
- Downstream issue #1102 will use this for version retention and cleanup workflow
