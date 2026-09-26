#!/usr/bin/env bash
# Check if golden files exist in R2 for a recipe
#
# Usage: ./scripts/check-golden-exists.sh <recipe> [--category <embedded|registry>]
#
# This script checks if golden files have been generated for a recipe in R2.
# Used to distinguish between new recipes (no golden files yet) and modified
# recipes (golden files exist but may not match).
#
# Environment Variables:
#   R2_BUCKET_URL        - Required. R2 bucket endpoint URL
#   R2_BUCKET_NAME       - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID     - Required. R2 access key ID
#   R2_SECRET_ACCESS_KEY - Required. R2 secret access key
#
# Exit Codes:
#   0 - Golden files exist in R2
#   1 - Golden files do not exist in R2 (new recipe)
#   2 - Error (missing environment variables, invalid arguments, or R2 could not be
#       listed). Could-not-look is not the same answer as not-found: treating a failed
#       listing as "new recipe" would skip the comparison and report success.

set -euo pipefail

# Configuration
BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"

# Script location for relative paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# shellcheck source=lib/r2-layout.sh
source "$SCRIPT_DIR/lib/r2-layout.sh"

# Parse arguments
RECIPE=""
CATEGORY=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --category)
            CATEGORY="$2"
            shift 2
            ;;
        -*)
            echo "Error: Unknown option: $1" >&2
            exit 2
            ;;
        *)
            if [[ -z "$RECIPE" ]]; then
                RECIPE="$1"
            else
                echo "Error: Unexpected argument: $1" >&2
                exit 2
            fi
            shift
            ;;
    esac
done

if [[ -z "$RECIPE" ]]; then
    echo "Usage: $0 <recipe> [--category <embedded|registry>]" >&2
    exit 2
fi

# Validate required environment variables
if [[ -z "${R2_BUCKET_URL:-}" ]]; then
    echo "Error: R2_BUCKET_URL environment variable is required" >&2
    exit 2
fi

if [[ -z "${R2_ACCESS_KEY_ID:-}" ]]; then
    echo "Error: R2_ACCESS_KEY_ID environment variable is required" >&2
    exit 2
fi

if [[ -z "${R2_SECRET_ACCESS_KEY:-}" ]]; then
    echo "Error: R2_SECRET_ACCESS_KEY environment variable is required" >&2
    exit 2
fi

# Auto-detect category if not specified
if [[ -z "$CATEGORY" ]]; then
    EMBEDDED_PATH="$REPO_ROOT/internal/recipe/recipes/$RECIPE.toml"
    if [[ -f "$EMBEDDED_PATH" ]]; then
        CATEGORY="embedded"
    else
        CATEGORY="registry"
    fi
fi

GOLDEN_PREFIX=$(r2_plan_prefix "$RECIPE" "$CATEGORY") || exit 2

# Check if any objects exist with this prefix using AWS CLI
# We use list-objects-v2 with max-keys=1 to minimize data transfer
if ! FIRST_KEY=$(AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
   AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
   AWS_ENDPOINT_URL="$R2_BUCKET_URL" \
   aws s3api list-objects-v2 \
       --bucket "$BUCKET_NAME" \
       --prefix "$GOLDEN_PREFIX" \
       --max-keys 1 \
       --query "Contents[0].Key" \
       --output text); then
    echo "Error: could not list R2 objects under $GOLDEN_PREFIX" >&2
    exit 2
fi

if [[ -n "$FIRST_KEY" && "$FIRST_KEY" != "None" ]]; then
    echo "Golden files exist for $RECIPE at $GOLDEN_PREFIX"
    exit 0
else
    echo "No golden files found for $RECIPE at $GOLDEN_PREFIX"
    exit 1
fi
