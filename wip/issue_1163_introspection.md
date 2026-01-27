# Issue 1163 Introspection

## Context Reviewed
- Design doc: docs/designs/DESIGN-registry-cache-policy.md (Stage 7: Cache Info Enhancement)
- Sibling issues reviewed: All 6 completed (#1156-1162)
- Prior patterns identified: CacheManager methods, cache.go output format

## Gap Analysis

### Minor Gaps

1. **CacheStats missing names**: Issue asks for "Oldest: fzf (cached 5 days ago)" but CacheStats only has OldestAccess timestamp, not recipe name. Need to extend or add new method.

2. **Stale count not in CacheStats**: Issue asks for "Stale: 3 entries" but Info() doesn't calculate this. Need TTL-aware counting.

3. **formatAgeDuration pattern exists**: The cache_cleanup.go uses `int(detail.Age.Hours() / 24)` for days. Can use similar pattern.

### Moderate Gaps

None - the implementation is straightforward extension of existing patterns.

### Major Gaps

None - issue spec aligns with design doc Stage 7.

## Recommendation

**Proceed** - Minor gaps can be resolved by extending CacheStats or adding an InfoExtended() method.

## Implementation Approach

Option 1: Extend CacheStats to include name fields and stale count
Option 2: Add new RegistryInfo struct with all needed fields
Option 3: Have cmd calculate stale count from CacheStats + TTL

Recommend Option 1 - extend CacheStats with optional fields:
- OldestName, NewestName strings
- StaleCount int (calculated with TTL parameter)

Or add a separate InfoWithDetails() method that returns extended stats.
