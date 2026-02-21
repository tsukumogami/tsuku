#!/bin/bash
# Test that tsuku-installed tools can discover tsuku-installed CA certificates.
#
# Installs ca-certificates (embedded) and curl (from source) separately,
# then checks whether the tsuku-built curl can make HTTPS requests.
#
# This answers the question: will tools installed with tsuku be able to
# find CA certs installed by tsuku, without explicit per-recipe wiring?
#
# Usage: ./test/scripts/test-tls-cacerts.sh [tsuku-binary]
#   tsuku-binary: Path to tsuku binary (default: ./tsuku)
#
# Environment:
#   TSUKU_HOME: Override tsuku home directory (default: temp dir)
#   GITHUB_TOKEN: Required for version resolution
#
# Exit codes:
#   0 - TLS verification works (with or without SSL_CERT_FILE)
#   1 - TLS verification failed entirely

set -e

TSUKU="${1:-./tsuku}"

if [ ! -x "$TSUKU" ]; then
    echo "ERROR: tsuku binary not found or not executable: $TSUKU"
    exit 1
fi

# Use a temporary TSUKU_HOME unless already set
if [ -z "$TSUKU_HOME" ]; then
    export TSUKU_HOME="$(mktemp -d)"
    CLEANUP_HOME=true
    trap "rm -rf $TSUKU_HOME" EXIT
fi

echo "=== TLS CA Certificates Integration Test ==="
echo "TSUKU_HOME=$TSUKU_HOME"
echo "tsuku binary: $TSUKU"
echo ""

# Step 1: Install ca-certificates (embedded recipe)
echo "=== Step 1: Installing ca-certificates ==="
"$TSUKU" install ca-certificates --force
echo ""

# Locate the CA bundle
CA_BUNDLE=$(find "$TSUKU_HOME" -path "*/share/ca-certificates/cacert.pem" -type f | head -1)
if [ -z "$CA_BUNDLE" ]; then
    echo "ERROR: CA bundle not found after installing ca-certificates"
    echo "Contents of TSUKU_HOME:"
    find "$TSUKU_HOME" -type f | head -20
    exit 1
fi
echo "CA bundle located: $CA_BUNDLE"
echo ""

# Step 2: Install curl from source (testdata recipe)
echo "=== Step 2: Installing curl from source ==="
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CURL_RECIPE="$REPO_ROOT/testdata/recipes/curl-source.toml"

if [ ! -f "$CURL_RECIPE" ]; then
    echo "ERROR: curl-source.toml not found at $CURL_RECIPE"
    exit 1
fi

"$TSUKU" install --recipe "$CURL_RECIPE" --force
echo ""

# Locate the tsuku-installed curl binary (symlinked to tools/current/)
TSUKU_CURL="$TSUKU_HOME/tools/current/curl"
if [ ! -x "$TSUKU_CURL" ]; then
    echo "ERROR: curl binary not found at $TSUKU_CURL"
    echo "Contents of TSUKU_HOME/tools/current:"
    ls -la "$TSUKU_HOME/tools/current/" 2>/dev/null || echo "(empty)"
    echo "Contents of TSUKU_HOME/tools:"
    ls "$TSUKU_HOME/tools/" 2>/dev/null || echo "(empty)"
    exit 1
fi
echo "curl binary: $TSUKU_CURL"
"$TSUKU_CURL" --version | head -1
echo ""

# Step 3: Test TLS without any environment help
echo "=== Test A: HTTPS request without SSL_CERT_FILE ==="
# Unset any existing SSL env vars to test the default behavior
unset SSL_CERT_FILE 2>/dev/null || true
unset SSL_CERT_DIR 2>/dev/null || true
unset CURL_CA_BUNDLE 2>/dev/null || true

if "$TSUKU_CURL" -sS -o /dev/null -w "%{http_code}" https://example.com 2>/dev/null | grep -q "200"; then
    echo "PASS: TLS works natively (no SSL_CERT_FILE needed)"
    NATIVE_TLS=true
else
    echo "FAIL: TLS does not work without SSL_CERT_FILE"
    echo "Error output:"
    "$TSUKU_CURL" -sS https://example.com 2>&1 || true
    NATIVE_TLS=false
fi
echo ""

# Step 4: Test TLS with explicit SSL_CERT_FILE
echo "=== Test B: HTTPS request with SSL_CERT_FILE ==="
export SSL_CERT_FILE="$CA_BUNDLE"

if "$TSUKU_CURL" -sS -o /dev/null -w "%{http_code}" https://example.com 2>/dev/null | grep -q "200"; then
    echo "PASS: TLS works with SSL_CERT_FILE=$CA_BUNDLE"
    EXPLICIT_TLS=true
else
    echo "FAIL: TLS does not work even with SSL_CERT_FILE"
    echo "Error output:"
    "$TSUKU_CURL" -sS https://example.com 2>&1 || true
    EXPLICIT_TLS=false
fi
echo ""

# Summary
echo "=== Results ==="
echo "Native TLS (no env vars): $([ "$NATIVE_TLS" = true ] && echo "PASS" || echo "FAIL")"
echo "Explicit SSL_CERT_FILE:   $([ "$EXPLICIT_TLS" = true ] && echo "PASS" || echo "FAIL")"
echo ""

if [ "$EXPLICIT_TLS" = true ]; then
    if [ "$NATIVE_TLS" = true ]; then
        echo "TLS works out of the box. CA certs are discoverable without configuration."
    else
        echo "TLS requires SSL_CERT_FILE to be set. This reveals a gap in tsuku's"
        echo "CA certificate discovery: tools installed by tsuku cannot automatically"
        echo "find CA certs also installed by tsuku."
    fi
    exit 0
else
    echo "ERROR: TLS failed entirely. The CA bundle may be invalid or curl/openssl"
    echo "build has a linking problem."
    exit 1
fi
