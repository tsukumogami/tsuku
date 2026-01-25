#!/usr/bin/env bash
# R2 Download Script
#
# Downloads golden files from R2 with checksum validation.
# Used by nightly validation workflow to fetch golden files for comparison.
#
# Usage:
#   ./scripts/r2-download.sh <recipe> <version> <platform> [output-file]
#   ./scripts/r2-download.sh --category <category> <recipe> <version> <platform> [output-file]
#
# Arguments:
#   recipe      - Recipe name (e.g., fzf, ripgrep)
#   version     - Version string (e.g., 0.60.0)
#   platform    - Platform identifier (e.g., linux-amd64, darwin-arm64)
#   output-file - Optional. Output path (default: stdout)
#
# Options:
#   --category <category>  - Category for key path (default: auto-detected from first letter)
#   --skip-verify          - Skip checksum verification (not recommended)
#   --metadata-only        - Print object metadata without downloading content
#
# Environment Variables:
#   R2_BUCKET_URL          - Required. R2 bucket endpoint URL
#   R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID       - Required. R2 access key ID (read-only is sufficient)
#   R2_SECRET_ACCESS_KEY   - Required. R2 secret access key
#
# Object Key Convention:
#   plans/{category}/{recipe}/v{version}/{platform}.json
#
# Checksum Validation:
#   Downloads the file and compares SHA256 hash against x-tsuku-recipe-hash metadata.
#   Returns non-zero if checksum mismatch is detected.
#
# Exit Codes:
#   0 - Success: file downloaded and verified
#   1 - Failure: download failed or checksum mismatch
#   2 - Error: invalid arguments or missing dependencies
#   3 - Not found: object does not exist

set -euo pipefail

BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"

# Parse arguments
CATEGORY=""
SKIP_VERIFY=false
METADATA_ONLY=false
POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --category)
            CATEGORY="$2"
            shift 2
            ;;
        --skip-verify)
            SKIP_VERIFY=true
            shift
            ;;
        --metadata-only)
            METADATA_ONLY=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [--category <category>] [--skip-verify] [--metadata-only] <recipe> <version> <platform> [output-file]"
            exit 0
            ;;
        *)
            POSITIONAL_ARGS+=("$1")
            shift
            ;;
    esac
done

set -- "${POSITIONAL_ARGS[@]}"

if [[ $# -lt 3 ]] || [[ $# -gt 4 ]]; then
    echo "Error: Expected 3-4 arguments: <recipe> <version> <platform> [output-file]" >&2
    echo "Usage: $0 [--category <category>] <recipe> <version> <platform> [output-file]" >&2
    exit 2
fi

RECIPE="$1"
VERSION="$2"
PLATFORM="$3"
OUTPUT_FILE="${4:-}"

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

# Auto-detect category from first letter if not specified
if [[ -z "$CATEGORY" ]]; then
    CATEGORY="${RECIPE:0:1}"
fi

# Build object key
OBJECT_KEY="plans/${CATEGORY}/${RECIPE}/v${VERSION}/${PLATFORM}.json"

# Export AWS credentials for subcommands
export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
export AWS_ENDPOINT_URL="$R2_BUCKET_URL"

# Check if object exists and get metadata
METADATA=$(aws s3api head-object \
    --bucket "$BUCKET_NAME" \
    --key "$OBJECT_KEY" \
    --output json 2>&1) || {
    if echo "$METADATA" | grep -q "Not Found\|404\|NoSuchKey"; then
        echo "Error: Object not found: s3://${BUCKET_NAME}/${OBJECT_KEY}" >&2
        exit 3
    else
        echo "Error: Failed to access object: $METADATA" >&2
        exit 1
    fi
}

# Extract stored hash from metadata
STORED_HASH=$(echo "$METADATA" | jq -r '.Metadata["x-tsuku-recipe-hash"] // empty')

if [[ "$METADATA_ONLY" == true ]]; then
    echo "$METADATA" | jq -r '.Metadata'
    exit 0
fi

# Download to temp file for verification
TEMP_FILE=$(mktemp)
trap "rm -f '$TEMP_FILE'" EXIT

aws s3 cp "s3://${BUCKET_NAME}/${OBJECT_KEY}" "$TEMP_FILE" >/dev/null

# Verify checksum if not skipped
if [[ "$SKIP_VERIFY" != true ]] && [[ -n "$STORED_HASH" ]]; then
    COMPUTED_HASH="sha256:$(sha256sum "$TEMP_FILE" | cut -d' ' -f1)"

    if [[ "$STORED_HASH" != "$COMPUTED_HASH" ]]; then
        echo "Error: Checksum mismatch for $OBJECT_KEY" >&2
        echo "  Expected (x-tsuku-recipe-hash): $STORED_HASH" >&2
        echo "  Computed: $COMPUTED_HASH" >&2
        exit 1
    fi
fi

# Output result
if [[ -n "$OUTPUT_FILE" ]]; then
    cp "$TEMP_FILE" "$OUTPUT_FILE"
    echo "Downloaded: s3://${BUCKET_NAME}/${OBJECT_KEY} -> $OUTPUT_FILE" >&2
else
    cat "$TEMP_FILE"
fi

exit 0
