#!/bin/bash
# Test that zig cc works as a C compiler substitute when no system gcc is available.
# This test runs in a Docker container without gcc installed.
#
# Usage: ./scripts/test-zig-cc.sh
#
# The test:
# 1. Creates a minimal container with Go but NO gcc
# 2. Builds tsuku from source
# 3. Installs zig and make via tsuku
# 4. Builds gdbm from source (which requires a C compiler)
# 5. Verifies gdbm-source was built successfully using zig cc

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Testing zig cc as C compiler substitute ==="
echo "Repository: $REPO_ROOT"
echo ""

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is required but not installed."
    exit 1
fi

# Create a temporary Dockerfile
DOCKERFILE=$(mktemp)
cat > "$DOCKERFILE" << 'DOCKERFILE_CONTENT'
# Use minimal glibc-based image (Homebrew bottles require glibc, Alpine uses musl)
FROM debian:bookworm-slim

# Install minimal dependencies (NO gcc, NO build-essential)
# We're testing that zig can be used as the C compiler instead of system gcc
# patchelf is needed to fix RPATH on Homebrew bottles
# binutils provides ld (linker) which autotools requires
# automake/autoconf needed because tarball timestamps can trigger regeneration
RUN apt-get update && apt-get install -y --no-install-recommends \
    autoconf \
    automake \
    binutils \
    ca-certificates \
    curl \
    flex \
    git \
    patchelf \
    texinfo \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Install Go manually (without gcc/cgo support)
ENV GO_VERSION=1.23.12
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -xz -C /usr/local
ENV PATH="/usr/local/go/bin:$PATH"
ENV CGO_ENABLED=0

# Verify no C compiler is available
RUN ! command -v gcc && ! command -v cc && echo "Confirmed: No system C compiler"

WORKDIR /tsuku
COPY . .

# Build tsuku (enable toolchain auto-download for newer Go version)
ENV GOTOOLCHAIN=auto
RUN go build -o tsuku ./cmd/tsuku

# Install zig (provides zig cc)
RUN ./tsuku install --force zig

# Install make (needed for configure_make)
RUN ./tsuku install --force make

# Add tsuku tools to PATH for build dependencies
# ~/.tsuku/tools/current contains symlinks to installed binaries
ENV PATH="/root/.tsuku/tools/current:$PATH"

# The actual test: build gdbm from source
# This MUST use zig cc since no system compiler exists
RUN ./tsuku install --recipe testdata/recipes/gdbm-source.toml --sandbox

# Verify the installation works
RUN ~/.tsuku/tools/current/gdbmtool --version

# Final verification message
RUN echo "SUCCESS: gdbm-source built using zig cc (no system gcc available)"
DOCKERFILE_CONTENT

echo "Building and running test container..."
echo ""

# Build and run the container
docker build -f "$DOCKERFILE" -t tsuku-zig-cc-test "$REPO_ROOT"
RESULT=$?

# Cleanup
rm -f "$DOCKERFILE"

if [ $RESULT -eq 0 ]; then
    echo ""
    echo "=== TEST PASSED ==="
    echo "Successfully built gdbm from source using zig cc as the C compiler."
    echo "This validates that tsuku can build autotools projects without system gcc."
else
    echo ""
    echo "=== TEST FAILED ==="
    exit 1
fi
