---
summary:
  constraints:
    - Must implement Refresh() and RefreshAll() methods on CachedRegistry
    - Must add --dry-run, --recipe, and --all flags to update-registry command
    - GetRecipe() signature changed in #1159 to return ([]byte, *CacheInfo, error)
    - Must use existing CacheMetadata.CachedAt for age calculation
    - Output format matches design spec with recipe-by-recipe status
  integration_points:
    - internal/registry/cached_registry.go - Add Refresh() and RefreshAll() methods
    - cmd/tsuku/update_registry.go - Add new flags and implement selective refresh logic
    - internal/registry/registry.go - May need ListCached() for iterating all cached recipes
  risks:
    - GetRecipe() signature changed in #1159 - ensure Refresh() uses correct signature
    - Need to handle case where recipe is not cached (for --recipe flag)
    - RefreshAll() must handle partial failures gracefully
  approach_notes: |
    This is Stage 5 of the design (Update-Registry Command Enhancement).
    Stages 1-4 (#1156, #1157, #1158, #1159) are complete.

    Main deliverables:
    1. Refresh(ctx, name) - Forces fetch regardless of TTL, updates cache
    2. RefreshAll(ctx) - Iterates all cached recipes, returns RefreshStats
    3. --dry-run flag - Shows what would be refreshed without network activity
    4. --recipe flag - Refreshes only specified recipe
    5. --all flag - Refreshes all cached recipes (default)

    Output format from design:
    ```
    Refreshing recipe cache...
      fzf: refreshed (was 2 days old)
      ripgrep: already fresh
      bat: refreshed (was 5 days old)

    Refreshed 2 of 3 cached recipes.
    ```
---

# Implementation Context: Issue #1160

**Source**: docs/designs/DESIGN-registry-cache-policy.md (Stage 5: Update-Registry Command Enhancement)

## Key Design Points

### Command Signature
```
tsuku update-registry [flags]

Flags:
  --dry-run          Show what would be refreshed without fetching
  --recipe <name>    Refresh a specific recipe only
  --all              Refresh all cached recipes (default if no --recipe)
```

### New Methods on CachedRegistry

```go
func (c *CachedRegistry) Refresh(ctx context.Context, name string) ([]byte, error)
func (c *CachedRegistry) RefreshAll(ctx context.Context) (*RefreshStats, error)
```

### RefreshStats Struct

```go
type RefreshStats struct {
    Total     int
    Refreshed int
    Fresh     int
    Errors    int
    Details   []RefreshDetail
}

type RefreshDetail struct {
    Name    string
    Status  string // "refreshed", "already fresh", "error"
    Age     time.Duration
    Error   error
}
```

### Dependencies Completed

- #1156: CacheMetadata infrastructure (CachedAt, ExpiresAt, LastAccess)
- #1157: CachedRegistry with GetRecipe() and TTL checking
- #1158: CacheManager with Size(), EnforceLimit()
- #1159: Stale fallback, GetRecipe() returns ([]byte, *CacheInfo, error)
