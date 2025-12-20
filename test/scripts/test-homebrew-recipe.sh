#!/bin/bash
# Test a homebrew-based recipe in a Docker environment with patchelf.
# This is needed because homebrew bottles on Linux require patchelf to fix RPATH.
#
# Usage: ./scripts/test-homebrew-recipe.sh <tool-name>
#
# Exit codes:
#   0 - Tool installation and verification passed
#   1 - Tool installation or verification failed

set -e

TOOL_NAME="${1:-}"

if [ -z "$TOOL_NAME" ]; then
    echo "Usage: $0 <tool-name>"
    echo "Example: $0 pkg-config"
    exit 1
fi

echo "=== Testing homebrew recipe: $TOOL_NAME ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Create Dockerfile for testing
DOCKERFILE=$(mktemp)
cat > "$DOCKERFILE" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Install patchelf (required for homebrew bottle RPATH fixup on Linux)
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        patchelf \
        && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku

ENV PATH="/home/testuser/.tsuku/tools/current:/home/testuser/.tsuku/bin:${PATH}"
EOF

# Build Docker image
echo "Building Docker image with patchelf..."
IMAGE_TAG="tsuku-homebrew-test:$$"
docker build -t "$IMAGE_TAG" -f "$DOCKERFILE" . > /dev/null

# Clean up Dockerfile
rm "$DOCKERFILE"

# Run test in container
echo ""
echo "Installing $TOOL_NAME in Docker container..."
docker run --rm "$IMAGE_TAG" install "$TOOL_NAME"

echo ""
echo "Verifying $TOOL_NAME..."
# Run the verify command from the recipe
docker run --rm "$IMAGE_TAG" verify "$TOOL_NAME"

# Clean up Docker image
docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true

echo ""
echo "=== PASS: $TOOL_NAME installation and verification succeeded ==="
exit 0
