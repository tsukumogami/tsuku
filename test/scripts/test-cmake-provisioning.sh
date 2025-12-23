#!/bin/bash
# Test that tsuku can provision cmake and build cmake-based projects in a
# clean environment without system cmake.
#
# This validates:
# - cmake recipe installs successfully
# - cmake --version works from relocated path
# - cmake can configure and build a simple project
# - ninja recipe builds successfully using cmake_build action
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed

set -e

echo "=== Testing CMake Provisioning ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Create Dockerfile for clean environment testing
DOCKERFILE=$(mktemp)
cat > "$DOCKERFILE" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Install ONLY minimal dependencies - explicitly NO cmake, NO build tools
# Just enough to run tsuku: wget for downloads, ca-certificates for HTTPS
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        patchelf \
        && \
    rm -rf /var/lib/apt/lists/*

# Verify cmake is NOT installed
RUN ! command -v cmake && echo "✓ cmake not in system (expected)"

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku

ENV PATH="/home/testuser/.tsuku/tools/current:${PATH}"
EOF

# Build Docker image
echo "Building Docker image (Ubuntu 22.04 without cmake)..."
IMAGE_TAG="tsuku-cmake-test:$$"
docker build -t "$IMAGE_TAG" -f "$DOCKERFILE" . > /dev/null

# Clean up Dockerfile
rm "$DOCKERFILE"

# Run all tests in a single container to persist installations
echo ""
echo "Running all tests in container..."
docker run --rm "$IMAGE_TAG" bash -c '
set -e
export PATH="$HOME/.tsuku/tools/current:$PATH"
export LD_LIBRARY_PATH="$HOME/.tsuku/libs/openssl-3.6.0/lib:$HOME/.tsuku/libs/zlib-1.3.1/lib:$LD_LIBRARY_PATH"

echo ""
echo "=== Test 1: Install cmake via tsuku ==="
./tsuku install cmake
echo "✓ cmake installed successfully"

echo ""
echo "=== Test 2: Verify cmake works ==="
cmake --version
echo "✓ cmake --version works"

echo ""
echo "=== Test 3: Build ninja using cmake_build action ==="
# This is the ultimate test - building ninja requires:
# - cmake (to run the build)
# - make (invoked by cmake)
# - zig (as the C++ compiler)
# All of these should be provided by tsuku, not the system
./tsuku install ninja --force
echo "✓ ninja built successfully using cmake_build"

echo ""
echo "=== Test 4: Verify ninja works ==="
ninja --version
echo "✓ ninja --version works"
'

# Clean up Docker image
docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true

echo ""
echo "=== ALL TESTS PASSED ==="
echo "✓ cmake recipe installs without system cmake"
echo "✓ cmake --version works from relocated path"
echo "✓ ninja builds successfully using cmake_build action"
echo "✓ ninja --version works"
echo "✓ Complete build toolchain (cmake + make + zig) validated"
exit 0
