#!/usr/bin/env bash
# R2 Retention Check Script
#
# Identifies golden file versions that exceed the retention policy (2 versions
# per recipe per platform). Used by the cleanup workflow to prune old versions.
#
# Usage:
#   ./scripts/r2-retention-check.sh [options]
#
# Options:
#   --json          Output JSON format instead of plain text
#   --recipe NAME   Check only the specified recipe
#   --dry-run       Report without action (default behavior)
#   -h, --help      Show this help message
#
# Environment Variables:
#   R2_BUCKET_URL          - Required. R2 bucket endpoint URL
#   R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID       - Required. R2 access key ID
#   R2_SECRET_ACCESS_KEY   - Required. R2 secret access key
#
# Retention Policy:
#   Keep latest 2 versions per recipe per platform.
#   Version ordering uses semantic versioning comparison.
#   Pre-release versions count toward the limit.
#
# Output:
#   In plain text mode (default): One object key per line for versions to prune
#   In JSON mode: JSON object with excess version details
#
# Exit Codes:
#   0 - Success (excess versions found or none)
#   1 - Error (missing credentials, API failure)
#   2 - Invalid arguments

set -euo pipefail

BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"
JSON_OUTPUT=false
RECIPE_FILTER=""
RETENTION_COUNT=2

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --json)
            JSON_OUTPUT=true
            shift
            ;;
        --recipe)
            RECIPE_FILTER="$2"
            shift 2
            ;;
        --dry-run)
            # Default behavior, accepted for compatibility
            shift
            ;;
        -h|--help)
            cat <<'EOF'
R2 Retention Check Script

Identifies golden file versions that exceed the retention policy (2 versions
per recipe per platform).

Usage:
  ./scripts/r2-retention-check.sh [options]

Options:
  --json          Output JSON format instead of plain text
  --recipe NAME   Check only the specified recipe
  --dry-run       Report without action (default behavior)
  -h, --help      Show this help message

Environment Variables:
  R2_BUCKET_URL          - Required. R2 bucket endpoint URL
  R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
  R2_ACCESS_KEY_ID       - Required. R2 access key ID
  R2_SECRET_ACCESS_KEY   - Required. R2 secret access key

Retention Policy:
  Keep latest 2 versions per recipe per platform.
  Older versions are reported as excess.

Examples:
  # Check all recipes
  ./scripts/r2-retention-check.sh

  # Check specific recipe
  ./scripts/r2-retention-check.sh --recipe fzf

  # Output as JSON
  ./scripts/r2-retention-check.sh --json
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

# Export AWS credentials for subcommands
export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
export AWS_ENDPOINT_URL="$R2_BUCKET_URL"

# Temporary files for processing
OBJECTS_FILE=$(mktemp)
EXCESS_FILE=$(mktemp)
trap "rm -f '$OBJECTS_FILE' '$EXCESS_FILE'" EXIT

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

# Build a map of recipe/platform -> versions
# Structure: versions[category/recipe/platform] = [v1, v2, v3, ...]
declare -A VERSION_MAP

while IFS= read -r object_key; do
    # Skip empty lines
    [[ -z "$object_key" ]] && continue

    # Parse object key: plans/{category}/{recipe}/v{version}/{platform}.json
    # Example: plans/f/fzf/v0.60.0/linux-amd64.json
    if [[ ! "$object_key" =~ ^plans/([^/]+)/([^/]+)/v([^/]+)/(.+)\.json$ ]]; then
        continue
    fi

    category="${BASH_REMATCH[1]}"
    recipe="${BASH_REMATCH[2]}"
    version="${BASH_REMATCH[3]}"
    platform="${BASH_REMATCH[4]}"

    # Skip embedded recipes (they're in git, not subject to R2 retention)
    if [[ "$category" == "embedded" ]]; then
        continue
    fi

    # Skip if not a single letter category (registry recipes use a-z)
    if [[ ! "$category" =~ ^[a-z]$ ]]; then
        continue
    fi

    # Apply recipe filter if specified
    if [[ -n "$RECIPE_FILTER" && "$recipe" != "$RECIPE_FILTER" ]]; then
        continue
    fi

    # Build key for version map
    map_key="${category}/${recipe}/${platform}"

    # Append version to the map (comma-separated, we'll split later)
    if [[ -n "${VERSION_MAP[$map_key]:-}" ]]; then
        VERSION_MAP[$map_key]="${VERSION_MAP[$map_key]},$version"
    else
        VERSION_MAP[$map_key]="$version"
    fi
done < "$OBJECTS_FILE"

echo "Processing ${#VERSION_MAP[@]} recipe/platform combinations..." >&2

# Sort versions using semantic versioning and identify excess
EXCESS_COUNT=0

# Function to compare semantic versions
# Returns 0 if v1 > v2, 1 otherwise
version_gt() {
    local v1="$1"
    local v2="$2"

    # Handle pre-release versions: 1.0.0-beta < 1.0.0
    # Strip 'v' prefix if present
    v1="${v1#v}"
    v2="${v2#v}"

    # Use sort -V for version comparison
    local highest
    highest=$(printf '%s\n%s\n' "$v1" "$v2" | sort -V | tail -n1)
    [[ "$v1" == "$highest" && "$v1" != "$v2" ]]
}

for map_key in "${!VERSION_MAP[@]}"; do
    # Split versions and sort
    IFS=',' read -ra versions <<< "${VERSION_MAP[$map_key]}"

    # Sort versions descending (newest first)
    IFS=$'\n' sorted_versions=($(printf '%s\n' "${versions[@]}" | sort -V -r))

    # Extract category, recipe, platform from key
    IFS='/' read -r category recipe platform <<< "$map_key"

    # Keep first RETENTION_COUNT versions, mark rest as excess
    for ((i = RETENTION_COUNT; i < ${#sorted_versions[@]}; i++)); do
        excess_version="${sorted_versions[$i]}"
        object_key="plans/${category}/${recipe}/v${excess_version}/${platform}.json"
        echo "$object_key" >> "$EXCESS_FILE"
        EXCESS_COUNT=$((EXCESS_COUNT + 1))
    done
done

echo "Identified $EXCESS_COUNT excess versions" >&2

# Output results
if [[ "$JSON_OUTPUT" == true ]]; then
    # Build JSON output
    EXCESS_KEYS=$(jq -R -s 'split("\n") | map(select(length > 0))' < "$EXCESS_FILE")

    # Group by recipe for summary
    RECIPES_SUMMARY=$(cat "$EXCESS_FILE" | \
        sed -n 's|^plans/\([^/]*/[^/]*\)/.*|\1|p' | \
        sort -u | \
        jq -R -s 'split("\n") | map(select(length > 0))')

    jq -n \
        --argjson excess_count "$EXCESS_COUNT" \
        --argjson total_objects "$TOTAL_OBJECTS" \
        --argjson retention_count "$RETENTION_COUNT" \
        --argjson excess_keys "$EXCESS_KEYS" \
        --argjson recipes_affected "$RECIPES_SUMMARY" \
        '{
            summary: {
                total_objects: $total_objects,
                excess_count: $excess_count,
                retention_policy: $retention_count,
                recipes_affected_count: ($recipes_affected | length)
            },
            recipes_affected: $recipes_affected,
            excess_keys: $excess_keys
        }'
else
    # Plain text output - one key per line
    cat "$EXCESS_FILE"
fi

exit 0
