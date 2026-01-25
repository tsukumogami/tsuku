#!/usr/bin/env bash
# R2 Upload Script
#
# Uploads golden files to R2 with metadata and verification.
# Used by post-merge CI workflow to publish generated golden files.
#
# Usage:
#   ./scripts/r2-upload.sh <recipe> <version> <platform> <file>
#   ./scripts/r2-upload.sh --category <category> <recipe> <version> <platform> <file>
#
# Arguments:
#   recipe    - Recipe name (e.g., fzf, ripgrep)
#   version   - Version string (e.g., 0.60.0)
#   platform  - Platform identifier (e.g., linux-amd64, darwin-arm64)
#   file      - Path to the golden file to upload
#
# Options:
#   --category <category>  - Category for key path (default: auto-detected from first letter)
#                            Use 'embedded' for embedded recipes
#
# Environment Variables:
#   R2_BUCKET_URL          - Required. R2 bucket endpoint URL
#   R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID       - Required. R2 access key ID (must have write permission)
#   R2_SECRET_ACCESS_KEY   - Required. R2 secret access key
#
# Object Key Convention:
#   plans/{category}/{recipe}/v{version}/{platform}.json
#
# Object Metadata:
#   x-tsuku-recipe-hash     - SHA256 hash of the file content
#   x-tsuku-generated-at    - ISO 8601 timestamp of upload
#   x-tsuku-format-version  - Golden file format version
#   x-tsuku-generator-version - Version of tsuku that generated the file
#
# Exit Codes:
#   0 - Success: file uploaded and verified
#   1 - Failure: upload failed or verification mismatch
#   2 - Error: invalid arguments or missing dependencies

set -euo pipefail

BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"
FORMAT_VERSION="3"

# Parse arguments
CATEGORY=""
POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --category)
            CATEGORY="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--category <category>] <recipe> <version> <platform> <file>"
            exit 0
            ;;
        *)
            POSITIONAL_ARGS+=("$1")
            shift
            ;;
    esac
done

set -- "${POSITIONAL_ARGS[@]}"

if [[ $# -ne 4 ]]; then
    echo "Error: Expected 4 arguments: <recipe> <version> <platform> <file>" >&2
    echo "Usage: $0 [--category <category>] <recipe> <version> <platform> <file>" >&2
    exit 2
fi

RECIPE="$1"
VERSION="$2"
PLATFORM="$3"
FILE="$4"

# Validate file exists
if [[ ! -f "$FILE" ]]; then
    echo "Error: File not found: $FILE" >&2
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

# Auto-detect category from first letter if not specified
if [[ -z "$CATEGORY" ]]; then
    CATEGORY="${RECIPE:0:1}"
fi

# Build object key
OBJECT_KEY="plans/${CATEGORY}/${RECIPE}/v${VERSION}/${PLATFORM}.json"

# Calculate file hash
FILE_HASH="sha256:$(sha256sum "$FILE" | cut -d' ' -f1)"

# Get generator version (try to extract from tsuku binary if available)
GENERATOR_VERSION="${TSUKU_VERSION:-unknown}"
if [[ "$GENERATOR_VERSION" == "unknown" ]] && command -v ./tsuku &>/dev/null; then
    GENERATOR_VERSION=$(./tsuku version 2>/dev/null | head -1 || echo "unknown")
fi

# Generate timestamp
GENERATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "Uploading: $FILE -> s3://${BUCKET_NAME}/${OBJECT_KEY}"
echo "  Recipe: $RECIPE"
echo "  Version: $VERSION"
echo "  Platform: $PLATFORM"
echo "  Hash: $FILE_HASH"

# Upload with metadata
AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
AWS_ENDPOINT_URL="$R2_BUCKET_URL" \
aws s3 cp "$FILE" "s3://${BUCKET_NAME}/${OBJECT_KEY}" \
    --metadata "x-tsuku-recipe-hash=${FILE_HASH},x-tsuku-generated-at=${GENERATED_AT},x-tsuku-format-version=${FORMAT_VERSION},x-tsuku-generator-version=${GENERATOR_VERSION}" \
    --content-type "application/json"

# Verify upload by reading back and comparing hash
echo "Verifying upload..."
TEMP_FILE=$(mktemp)
trap "rm -f '$TEMP_FILE'" EXIT

AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
AWS_ENDPOINT_URL="$R2_BUCKET_URL" \
aws s3 cp "s3://${BUCKET_NAME}/${OBJECT_KEY}" "$TEMP_FILE"

VERIFY_HASH="sha256:$(sha256sum "$TEMP_FILE" | cut -d' ' -f1)"

if [[ "$FILE_HASH" != "$VERIFY_HASH" ]]; then
    echo "Error: Upload verification failed - hash mismatch" >&2
    echo "  Expected: $FILE_HASH" >&2
    echo "  Got: $VERIFY_HASH" >&2
    exit 1
fi

echo "Upload verified successfully"
echo "Object key: $OBJECT_KEY"
exit 0
