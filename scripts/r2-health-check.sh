#!/usr/bin/env bash
# R2 Health Check Script
#
# Checks if the R2 bucket is accessible and responding within acceptable latency.
# Used by CI workflows to gate validation runs.
#
# Usage:
#   ./scripts/r2-health-check.sh
#
# Environment Variables:
#   R2_BUCKET_URL     - Required. R2 bucket endpoint URL (e.g., https://<account>.r2.cloudflarestorage.com)
#   R2_BUCKET_NAME    - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID  - Required. R2 access key ID
#   R2_SECRET_ACCESS_KEY - Required. R2 secret access key
#
# Exit Codes:
#   0 - Success: R2 is healthy (HTTP 200, latency < 2000ms)
#   1 - Failure: R2 is unavailable (timeout, error, or non-200 response)
#   2 - Degraded: R2 is slow (HTTP 200, latency >= 2000ms)
#
# Health Check Contract:
#   - Endpoint: HEAD request to health/ping.json
#   - Timeout: 5 seconds
#   - Success: HTTP 200 with latency < 2000ms
#   - Degraded: HTTP 200 with latency >= 2000ms
#   - Failure: Any other response or timeout

set -euo pipefail

# Configuration
TIMEOUT_SECONDS=5
LATENCY_THRESHOLD_MS=2000
HEALTH_OBJECT="health/ping.json"
BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"

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

# Build the health check URL
HEALTH_URL="${R2_BUCKET_URL}/${BUCKET_NAME}/${HEALTH_OBJECT}"

# Perform health check with timing
START_TIME=$(date +%s%3N 2>/dev/null || python3 -c "import time; print(int(time.time() * 1000))")

# Use AWS CLI for S3-compatible HEAD request
HTTP_STATUS=$(AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
    AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
    AWS_ENDPOINT_URL="$R2_BUCKET_URL" \
    aws s3api head-object \
        --bucket "$BUCKET_NAME" \
        --key "$HEALTH_OBJECT" \
        --output text \
        --query "ContentLength" \
        2>&1) && STATUS_CODE=200 || STATUS_CODE=1

END_TIME=$(date +%s%3N 2>/dev/null || python3 -c "import time; print(int(time.time() * 1000))")

# Calculate latency
LATENCY_MS=$((END_TIME - START_TIME))

# Evaluate result
if [[ "$STATUS_CODE" -ne 200 ]]; then
    echo "R2 health check failed: unable to reach ${HEALTH_OBJECT}"
    echo "Status: failure"
    echo "Latency: ${LATENCY_MS}ms"
    exit 1
fi

if [[ "$LATENCY_MS" -ge "$LATENCY_THRESHOLD_MS" ]]; then
    echo "R2 health check degraded: latency ${LATENCY_MS}ms >= ${LATENCY_THRESHOLD_MS}ms threshold"
    echo "Status: degraded"
    echo "Latency: ${LATENCY_MS}ms"
    exit 2
fi

echo "R2 health check passed"
echo "Status: healthy"
echo "Latency: ${LATENCY_MS}ms"
exit 0
