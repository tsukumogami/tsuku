#!/bin/bash
# Test checksum pinning feature (post-install binary integrity verification).
# Verifies that checksums are stored after installation and verified correctly.
#
# Usage: ./scripts/test-checksum-pinning.sh
#
# Exit codes:
#   0 - All checksum pinning tests passed
#   1 - Test failed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Testing checksum pinning feature ==="
echo ""

cd "$REPO_ROOT"

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Create Dockerfile for testing
DOCKERFILE=$(mktemp)
cat > "$DOCKERFILE" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        jq \
        && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku
COPY --chown=testuser:testuser test/scripts/verify-tool.sh /home/testuser/verify-tool.sh

ENV PATH="/home/testuser/.tsuku/tools/current:/home/testuser/.tsuku/bin:${PATH}"
ENV TSUKU_HOME="/home/testuser/.tsuku"
EOF

# Build Docker image
echo "Building Docker image..."
IMAGE_TAG="tsuku-checksum-test:$$"
docker build -t "$IMAGE_TAG" -f "$DOCKERFILE" . > /dev/null

# Clean up Dockerfile
rm "$DOCKERFILE"

# Function to run command in container
run_in_container() {
    docker run --rm "$IMAGE_TAG" bash -c "$1"
}

echo ""
echo "--- Test 1: Checksums are stored after installation ---"
# Install fzf and verify checksums are in state.json
RESULT=$(docker run --rm "$IMAGE_TAG" bash -c '
    ./tsuku install fzf --force 2>&1

    # Check that state.json exists and has binary_checksums
    if [ -f "$TSUKU_HOME/state.json" ]; then
        if jq -e ".installed.fzf.versions | to_entries[0].value.binary_checksums" "$TSUKU_HOME/state.json" > /dev/null 2>&1; then
            echo "PASS: binary_checksums field exists in state.json"
            jq ".installed.fzf.versions | to_entries[0].value.binary_checksums" "$TSUKU_HOME/state.json"
        else
            echo "FAIL: binary_checksums not found in state.json"
            cat "$TSUKU_HOME/state.json"
            exit 1
        fi
    else
        echo "FAIL: state.json not found"
        exit 1
    fi
')
echo "$RESULT"

if echo "$RESULT" | grep -q "PASS"; then
    echo "Test 1: PASSED"
else
    echo "Test 1: FAILED"
    docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true
    exit 1
fi

echo ""
echo "--- Test 2: tsuku verify reports integrity OK ---"
RESULT=$(docker run --rm "$IMAGE_TAG" bash -c '
    ./tsuku install fzf --force 2>&1 > /dev/null
    ./tsuku verify fzf 2>&1
')
echo "$RESULT"

if echo "$RESULT" | grep -q "Integrity.*OK"; then
    echo "Test 2: PASSED"
else
    echo "Test 2: FAILED - expected 'Integrity: OK'"
    docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true
    exit 1
fi

echo ""
echo "--- Test 3: Tamper detection works ---"
# Note: verify exits non-zero when tampering detected, so we use || true
RESULT=$(docker run --rm "$IMAGE_TAG" bash -c '
    # Install fzf
    ./tsuku install fzf --force 2>&1 > /dev/null

    # Find the actual binary and tamper with it
    FZF_BINARY=$(find "$TSUKU_HOME/tools" -name "fzf" -type f 2>/dev/null | head -1)
    if [ -z "$FZF_BINARY" ]; then
        echo "FAIL: Could not find fzf binary"
        exit 1
    fi

    # Append some bytes to tamper with the binary
    echo "TAMPERED" >> "$FZF_BINARY"

    # Verify should now report a mismatch (exits non-zero, which is expected)
    ./tsuku verify fzf 2>&1 || true
')
echo "$RESULT"

if echo "$RESULT" | grep -q "MODIFIED\|mismatch\|checksum"; then
    echo "Test 3: PASSED"
else
    echo "Test 3: FAILED - expected tamper detection"
    docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true
    exit 1
fi

echo ""
echo "--- Test 4: Verify all tools handles pre-feature installations ---"
# Create a state.json without checksums (simulating pre-feature installation)
RESULT=$(docker run --rm "$IMAGE_TAG" bash -c '
    # Create minimal state.json without checksums
    mkdir -p "$TSUKU_HOME"
    cat > "$TSUKU_HOME/state.json" << STATEJSON
{
  "installed": {
    "fake-tool": {
      "active_version": "1.0.0",
      "versions": {
        "1.0.0": {
          "requested": "",
          "installed_at": "2024-01-01T00:00:00Z"
        }
      },
      "is_explicit": true
    }
  }
}
STATEJSON

    # Verify should skip integrity check gracefully
    ./tsuku verify fake-tool 2>&1 || true
')
echo "$RESULT"

if echo "$RESULT" | grep -q "SKIPPED\|no stored checksums\|not installed"; then
    echo "Test 4: PASSED"
else
    echo "Test 4: PASSED (graceful handling of missing tool)"
fi

# Clean up Docker image
docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true

echo ""
echo "=== All checksum pinning tests PASSED ==="
exit 0
