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
#   0 - Success: R2 is healthy (HTTP 200, latency < threshold)
#   1 - Failure: R2 is unavailable (timeout, error, or non-200 response)
#   2 - Degraded: R2 is slow (HTTP 200, latency >= threshold)
#
# Health Check Contract:
#   - Endpoint: SigV4-signed HEAD request to health/ping.json
#   - Timeout: 5 seconds for the whole request
#   - Success: HTTP 200 with latency < LATENCY_THRESHOLD_MS
#   - Degraded: HTTP 200 with latency >= LATENCY_THRESHOLD_MS
#   - Failure: Any other response, a connection error, or timeout
#
# What the latency measures:
#   The request, as curl times it: name lookup, connection, TLS handshake and the
#   response. It does not include starting the program that makes the request. An
#   earlier version timed an entire `aws s3api head-object` process, interpreter start-up
#   included, and that start-up alone put healthy runs on both sides of the threshold
#   (#2593). The connection, TLS and first-byte times are printed separately so a slow
#   handshake can be told from a slow storage response.
#
# Requires curl 7.75 or later (for --aws-sigv4).

set -euo pipefail

# Configuration
TIMEOUT_SECONDS=5
# Provisional. This was set against the old process-level measurement; it is re-derived
# from recorded request latencies once enough scheduled runs have used this measurement.
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

# Build the health check URL (path-style: <endpoint>/<bucket>/<key>)
HEALTH_URL="${R2_BUCKET_URL%/}/${BUCKET_NAME}/${HEALTH_OBJECT}"

# curl reports its own timings for the request. Seconds, with microsecond precision.
WRITE_OUT='%{http_code} %{time_connect} %{time_appconnect} %{time_starttransfer} %{time_total}'

# Credentials go to curl on stdin as a config file, not on the command line, so they
# never appear in the process list.
escape() { local s="${1//\\/\\\\}"; printf '%s' "${s//\"/\\\"}"; }
CURL_ERR=$(mktemp)
trap 'rm -f "$CURL_ERR"' EXIT

set +e
TIMINGS=$(printf 'user = "%s:%s"\n' "$(escape "$R2_ACCESS_KEY_ID")" "$(escape "$R2_SECRET_ACCESS_KEY")" |
    curl --config - \
        --silent --show-error \
        --head --output /dev/null \
        --max-time "$TIMEOUT_SECONDS" \
        --aws-sigv4 "aws:amz:auto:s3" \
        --write-out "$WRITE_OUT" \
        "$HEALTH_URL" 2>"$CURL_ERR")
CURL_EXIT=$?
set -e

read -r HTTP_CODE T_CONNECT T_TLS T_FIRST_BYTE T_TOTAL <<<"$TIMINGS"

to_ms() { awk -v s="${1:-0}" 'BEGIN { printf "%d", s * 1000 + 0.5 }'; }
LATENCY_MS=$(to_ms "$T_TOTAL")

report_timings() {
    echo "Latency: ${LATENCY_MS}ms"
    echo "Connect: $(to_ms "$T_CONNECT")ms"
    # time_appconnect is 0 when there was no TLS handshake (plain http).
    echo "TLS: $(to_ms "$T_TLS")ms"
    echo "First byte: $(to_ms "$T_FIRST_BYTE")ms"
}

# Evaluate result
if [[ "$CURL_EXIT" -ne 0 ]]; then
    echo "R2 health check failed: request for ${HEALTH_OBJECT} did not complete (curl exit ${CURL_EXIT}): $(head -c 300 "$CURL_ERR")"
    echo "Status: failure"
    report_timings
    exit 1
fi

if [[ "$HTTP_CODE" != "200" ]]; then
    echo "R2 health check failed: HTTP ${HTTP_CODE} for ${HEALTH_OBJECT}"
    echo "Status: failure"
    report_timings
    exit 1
fi

if [[ "$LATENCY_MS" -ge "$LATENCY_THRESHOLD_MS" ]]; then
    echo "R2 health check degraded: latency ${LATENCY_MS}ms >= ${LATENCY_THRESHOLD_MS}ms threshold"
    echo "Status: degraded"
    report_timings
    exit 2
fi

echo "R2 health check passed"
echo "Status: healthy"
report_timings
exit 0
