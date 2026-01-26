# Issue 1101 Implementation Plan

## Overview

Create `scripts/r2-orphan-detection.sh` to detect orphaned golden files in R2 - files that reference deleted recipes or unsupported platforms.

## Orphan Types

1. **Deleted recipe**: Recipe file no longer exists in `recipes/{letter}/`
2. **Dropped platform**: Recipe exists but no longer supports that OS/arch combination

## Implementation Approach

### Simplified Platform Detection

For V1, focus on deleted recipe detection (type 1) rather than platform analysis (type 2):
- Check if recipe file exists in `recipes/{letter}/{recipe}.toml`
- If recipe doesn't exist, all its golden files are orphans
- Platform analysis is complex (requires TOML parsing) and can be added later

This approach:
- Catches the most common case (recipe deletion)
- Avoids complex TOML parsing in bash
- Still provides value for downstream #1102

### Script Structure

```bash
#!/usr/bin/env bash
# r2-orphan-detection.sh
# Usage: ./scripts/r2-orphan-detection.sh [--json] [--delete]
#
# Options:
#   --json    Output JSON format instead of plain text
#   --delete  Actually delete orphans (default: dry-run)
#   --help    Show usage
```

### Algorithm

1. List all objects in R2 bucket with prefix `plans/`
2. Parse each object key: `plans/{category}/{recipe}/v{version}/{platform}.json`
3. For each unique recipe (category + name):
   - Skip if category is "embedded" (not registry)
   - Check if `recipes/{category}/{recipe}.toml` exists
   - If not, mark all golden files for this recipe as orphans
4. Output list of orphaned object keys

### R2 Integration

- Use AWS CLI with pagination (--max-items and --starting-token)
- Requires: R2_BUCKET_URL, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY
- Uses read-only credentials (detection only)

## Files to Create

1. `scripts/r2-orphan-detection.sh` - main detection script

## Testing

- Script handles missing R2 credentials gracefully
- Script provides help output
- Verify with mock data (create temp recipes dir, simulate R2 output)
