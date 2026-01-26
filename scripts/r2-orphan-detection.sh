#!/usr/bin/env bash
# R2 Orphan Detection Script
#
# Detects orphaned golden files in R2 - files that reference recipes that have
# been deleted from the repository.
#
# Usage:
#   ./scripts/r2-orphan-detection.sh [options]
#
# Options:
#   --json          Output JSON format instead of plain text
#   --recipes-dir   Path to recipes directory (default: recipes)
#   --dry-run       Report orphans without action (default behavior)
#   -h, --help      Show this help message
#
# Environment Variables:
#   R2_BUCKET_URL          - Required. R2 bucket endpoint URL
#   R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID       - Required. R2 access key ID (read-only is sufficient)
#   R2_SECRET_ACCESS_KEY   - Required. R2 secret access key
#
# Object Key Convention:
#   plans/{category}/{recipe}/v{version}/{platform}.json
#   - category: single letter (a-z) for registry recipes, "embedded" for embedded
#   - This script only detects orphans for registry recipes (single-letter categories)
#
# Output:
#   In plain text mode (default): One orphaned object key per line
#   In JSON mode: JSON object with orphan details
#
# Exit Codes:
#   0 - Success (orphans found or none)
#   1 - Error (missing credentials, API failure)
#   2 - Invalid arguments

set -euo pipefail

BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"
RECIPES_DIR="recipes"
JSON_OUTPUT=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --json)
            JSON_OUTPUT=true
            shift
            ;;
        --recipes-dir)
            RECIPES_DIR="$2"
            shift 2
            ;;
        --dry-run)
            # Default behavior, accepted for compatibility
            shift
            ;;
        -h|--help)
            cat <<'EOF'
R2 Orphan Detection Script

Detects orphaned golden files in R2 - files that reference recipes that have
been deleted from the repository.

Usage:
  ./scripts/r2-orphan-detection.sh [options]

Options:
  --json          Output JSON format instead of plain text
  --recipes-dir   Path to recipes directory (default: recipes)
  --dry-run       Report orphans without action (default behavior)
  -h, --help      Show this help message

Environment Variables:
  R2_BUCKET_URL          - Required. R2 bucket endpoint URL
  R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
  R2_ACCESS_KEY_ID       - Required. R2 access key ID (read-only)
  R2_SECRET_ACCESS_KEY   - Required. R2 secret access key

Output:
  In plain text mode: One orphaned object key per line, suitable for piping
  In JSON mode: JSON object with orphan counts and details

Examples:
  # List orphaned files
  ./scripts/r2-orphan-detection.sh

  # Output as JSON
  ./scripts/r2-orphan-detection.sh --json

  # Pipe to deletion (requires write credentials)
  ./scripts/r2-orphan-detection.sh | xargs -I {} aws s3 rm "s3://$BUCKET/{}"
EOF
            exit 0
            ;;
        *)
            echo "Error: Unknown option: $1" >&2
            echo "Use --help for usage information" >&2
            exit 2
            ;;
    esac
done

# Validate required environment variables
if [[ -z "${R2_BUCKET_URL:-}" ]]; then
    echo "Error: R2_BUCKET_URL environment variable is required" >&2
    exit 1
fi

if [[ -z "${R2_ACCESS_KEY_ID:-}" ]]; then
    echo "Error: R2_ACCESS_KEY_ID environment variable is required" >&2
    exit 1
fi

if [[ -z "${R2_SECRET_ACCESS_KEY:-}" ]]; then
    echo "Error: R2_SECRET_ACCESS_KEY environment variable is required" >&2
    exit 1
fi

# Validate recipes directory exists
if [[ ! -d "$RECIPES_DIR" ]]; then
    echo "Error: Recipes directory not found: $RECIPES_DIR" >&2
    exit 1
fi

# Export AWS credentials for subcommands
export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
export AWS_ENDPOINT_URL="$R2_BUCKET_URL"

# Temporary files for processing
OBJECTS_FILE=$(mktemp)
ORPHANS_FILE=$(mktemp)
trap "rm -f '$OBJECTS_FILE' '$ORPHANS_FILE'" EXIT

# List all objects in the bucket with prefix "plans/"
# Use pagination to handle large buckets
echo "Listing objects in R2 bucket..." >&2

CONTINUATION_TOKEN=""
TOTAL_OBJECTS=0

while true; do
    if [[ -z "$CONTINUATION_TOKEN" ]]; then
        RESPONSE=$(aws s3api list-objects-v2 \
            --bucket "$BUCKET_NAME" \
            --prefix "plans/" \
            --output json 2>&1) || {
            echo "Error: Failed to list objects: $RESPONSE" >&2
            exit 1
        }
    else
        RESPONSE=$(aws s3api list-objects-v2 \
            --bucket "$BUCKET_NAME" \
            --prefix "plans/" \
            --continuation-token "$CONTINUATION_TOKEN" \
            --output json 2>&1) || {
            echo "Error: Failed to list objects: $RESPONSE" >&2
            exit 1
        }
    fi

    # Extract object keys and append to file
    KEYS=$(echo "$RESPONSE" | jq -r '.Contents[]?.Key // empty')
    if [[ -n "$KEYS" ]]; then
        echo "$KEYS" >> "$OBJECTS_FILE"
        COUNT=$(echo "$KEYS" | wc -l)
        TOTAL_OBJECTS=$((TOTAL_OBJECTS + COUNT))
    fi

    # Check for more pages
    IS_TRUNCATED=$(echo "$RESPONSE" | jq -r '.IsTruncated // false')
    if [[ "$IS_TRUNCATED" == "true" ]]; then
        CONTINUATION_TOKEN=$(echo "$RESPONSE" | jq -r '.NextContinuationToken')
    else
        break
    fi
done

echo "Found $TOTAL_OBJECTS objects in R2" >&2

# Process each object key to detect orphans
ORPHAN_COUNT=0
RECIPES_CHECKED=()

while IFS= read -r object_key; do
    # Skip empty lines
    [[ -z "$object_key" ]] && continue

    # Parse object key: plans/{category}/{recipe}/v{version}/{platform}.json
    # Example: plans/f/fzf/v0.60.0/linux-amd64.json
    if [[ ! "$object_key" =~ ^plans/([^/]+)/([^/]+)/v([^/]+)/(.+)\.json$ ]]; then
        echo "Warning: Unexpected key format: $object_key" >&2
        continue
    fi

    category="${BASH_REMATCH[1]}"
    recipe="${BASH_REMATCH[2]}"
    version="${BASH_REMATCH[3]}"
    platform="${BASH_REMATCH[4]}"

    # Skip embedded recipes (handled separately, not orphan candidates)
    if [[ "$category" == "embedded" ]]; then
        continue
    fi

    # Skip if not a single letter category (registry recipes use a-z)
    if [[ ! "$category" =~ ^[a-z]$ ]]; then
        echo "Warning: Unexpected category format: $category (key: $object_key)" >&2
        continue
    fi

    # Check if we've already determined this recipe's status
    recipe_id="${category}/${recipe}"
    recipe_file="$RECIPES_DIR/$category/$recipe.toml"

    # Check if recipe file exists
    if [[ ! -f "$recipe_file" ]]; then
        # Recipe doesn't exist - this is an orphan
        echo "$object_key" >> "$ORPHANS_FILE"
        ORPHAN_COUNT=$((ORPHAN_COUNT + 1))
    fi
done < "$OBJECTS_FILE"

echo "Detected $ORPHAN_COUNT orphaned objects" >&2

# Output results
if [[ "$JSON_OUTPUT" == true ]]; then
    # Build JSON output
    ORPHAN_KEYS=$(jq -R -s 'split("\n") | map(select(length > 0))' < "$ORPHANS_FILE")

    # Group by recipe for summary
    RECIPES_SUMMARY=$(cat "$ORPHANS_FILE" | \
        sed -n 's|^plans/\([^/]*/[^/]*\)/.*|\1|p' | \
        sort -u | \
        jq -R -s 'split("\n") | map(select(length > 0))')

    jq -n \
        --argjson orphan_count "$ORPHAN_COUNT" \
        --argjson total_objects "$TOTAL_OBJECTS" \
        --argjson orphan_keys "$ORPHAN_KEYS" \
        --argjson deleted_recipes "$RECIPES_SUMMARY" \
        '{
            summary: {
                total_objects: $total_objects,
                orphan_count: $orphan_count,
                deleted_recipes_count: ($deleted_recipes | length)
            },
            deleted_recipes: $deleted_recipes,
            orphan_keys: $orphan_keys
        }'
else
    # Plain text output - one key per line
    cat "$ORPHANS_FILE"
fi

exit 0
