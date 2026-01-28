# Documentation Gap Analysis: M42 Cache Management and Documentation

## Milestone Context

Milestone M42 "Cache Management and Documentation" contains two closed issues:
- **#1037**: Cache policy implementation (TTL, size limits, stale fallback, environment variables)
- **#1038**: Documentation updates for recipe separation in CONTRIBUTING.md

## Documentation Inventory

### Primary Documentation Files Examined:
1. `/docs/ENVIRONMENT.md` - Environment variable reference
2. `/README.md` - User-facing documentation
3. `/CONTRIBUTING.md` - Contributor guide
4. `/docs/EMBEDDED_RECIPES.md` - Embedded recipe documentation
5. `/docs/designs/current/DESIGN-registry-cache-policy.md` - Cache policy design doc

### Command Help Files Examined:
1. `/cmd/tsuku/cache.go` - Cache command implementation
2. `/cmd/tsuku/cache_cleanup.go` - Cache cleanup subcommand
3. `/cmd/tsuku/update_registry.go` - Update registry command

## Features Delivered in M42

### From #1037 (Cache Policy Implementation):
1. TTL-based cache expiration (TSUKU_RECIPE_CACHE_TTL)
2. Stale-if-error fallback (TSUKU_RECIPE_CACHE_STALE_FALLBACK, TSUKU_RECIPE_CACHE_MAX_STALE)
3. LRU cache size management (TSUKU_RECIPE_CACHE_SIZE_LIMIT)
4. Enhanced `tsuku update-registry` command (--dry-run, --recipe, --all flags)
5. New `tsuku cache cleanup` subcommand (--dry-run, --max-age, --force-limit flags)
6. Enhanced `tsuku cache info` with registry section

### From #1038 (Documentation Updates):
1. Recipe category guidance (decision flowchart)
2. Three directory explanation table
3. Troubleshooting: "Recipe Works Locally But Fails in CI"
4. Troubleshooting: "Recipe Not Found (Network Issues)"
5. Nightly validation documentation
6. Security incident response playbook

## Documentation Coverage Analysis

### docs/ENVIRONMENT.md - PASS

All environment variables are properly documented:
- TSUKU_RECIPE_CACHE_TTL (lines 77-87)
- TSUKU_RECIPE_CACHE_SIZE_LIMIT (lines 89-98)
- TSUKU_RECIPE_CACHE_MAX_STALE (lines 99-108)
- TSUKU_RECIPE_CACHE_STALE_FALLBACK (lines 110-118)
- Summary table includes all variables (lines 153-167)

Each variable has:
- Default value
- Valid range
- Format specification
- Usage example

### CONTRIBUTING.md - PASS

All required documentation from #1038 is present:
- Recipe category decision flowchart (lines 244-253)
- Three directory table (lines 233-240)
- Troubleshooting: "Recipe Works Locally But Fails in CI" (lines 772-786)
- Troubleshooting: "Recipe Not Found (Network Issues)" (lines 788-797)
- Nightly Registry Validation section (lines 813-830)
- Security Incident Response section (lines 834-871)

### README.md - FINDING

The README.md lacks user-facing documentation for the new cache management features:

1. **Missing command reference for cache commands**:
   - `tsuku cache` is not listed in the Commands table (lines documenting available commands)
   - `tsuku cache cleanup` is not mentioned
   - `tsuku cache info` is not mentioned

2. **Missing cache management usage examples**:
   - No examples showing how to view cache statistics
   - No examples showing how to clean up the cache
   - No examples showing update-registry with new flags

3. **Minor: update-registry documentation is minimal**
   - The README mentions `tsuku update-registry` briefly in CLAUDE.local.md but not in the main README
   - The enhanced flags (--dry-run, --recipe, --all) are undocumented for users

### docs/EMBEDDED_RECIPES.md - PASS

Document is complete and accurate. No changes needed for M42 as cache policy does not affect embedded recipes.

### Design Document - PASS

The design document at `/docs/designs/current/DESIGN-registry-cache-policy.md` is complete with all sections including implementation issues table (showing all issues as struck through/completed).

## Gap Summary

| Gap ID | Severity | Location | Description |
|--------|----------|----------|-------------|
| G1 | Medium | README.md | Missing `tsuku cache` command in command reference table |
| G2 | Low | README.md | Missing usage examples for cache cleanup and cache info |
| G3 | Low | README.md | Enhanced update-registry flags (--dry-run, --recipe, --all) not documented |

## Recommendations

### G1: Add cache command to README Commands table

Add to the Commands table in README.md:
```markdown
| `tsuku cache info` | Show cache statistics |
| `tsuku cache cleanup` | Remove old cache entries |
```

### G2: Add cache management section to README

Add a brief "Cache Management" section near the end of README with examples:
```bash
# View cache statistics
tsuku cache info

# Remove cache entries older than 7 days
tsuku cache cleanup --max-age 7d

# Preview what would be removed
tsuku cache cleanup --dry-run
```

### G3: Document update-registry enhancements

In README.md, update the brief mention of `tsuku update-registry` to include:
```bash
# Refresh recipe cache
tsuku update-registry

# Preview what would be refreshed
tsuku update-registry --dry-run

# Refresh a specific recipe
tsuku update-registry --recipe fzf
```

## Conclusion

The core documentation for M42 is complete. The environment variables are fully documented in ENVIRONMENT.md, and all contributor-facing documentation is present in CONTRIBUTING.md. The only gaps are in user-facing README.md documentation for the new cache management commands.
