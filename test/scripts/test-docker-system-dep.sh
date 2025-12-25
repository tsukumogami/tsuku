#!/bin/bash
# Test docker system dependency recipe in isolated Docker environments.
#
# This validates:
# - docker recipe fails gracefully when Docker is not installed
# - docker recipe succeeds with checkmark when Docker is installed
# - No state entries or directories created for system dependencies
# - Platform-specific installation guides are shown
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed

set -e

# Ensure we use the correct Docker API version
export DOCKER_API_VERSION=1.52

echo "=== Testing Docker System Dependency Recipe ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Test 1: Docker NOT installed - should fail with installation guide
echo ""
echo "=== Test 1: Docker not installed - expect failure with guide ==="
DOCKERFILE_NO_DOCKER=$(mktemp)
cat > "$DOCKERFILE_NO_DOCKER" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Install minimal dependencies - explicitly NO docker
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        && \
    rm -rf /var/lib/apt/lists/*

# Verify docker is NOT installed
RUN ! command -v docker && echo "✓ docker not in system (expected)"

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku

ENV PATH="/home/testuser/.tsuku/bin:${PATH}"
EOF

IMAGE_TAG_NO_DOCKER="tsuku-docker-test-no-docker:$$"
echo "Building Docker image (Ubuntu 22.04 without docker)..."
docker build -t "$IMAGE_TAG_NO_DOCKER" -f "$DOCKERFILE_NO_DOCKER" . > /dev/null
rm "$DOCKERFILE_NO_DOCKER"

echo "Running tsuku install docker (should fail)..."
# Disable errexit temporarily to capture failure
set +e
docker run --rm "$IMAGE_TAG_NO_DOCKER" ./tsuku install docker 2>&1 | tee /tmp/docker-test-1.log
EXIT_CODE=${PIPESTATUS[0]}
set -e

if [ $EXIT_CODE -eq 0 ]; then
    echo "ERROR: Expected tsuku install docker to fail when docker is not installed"
    docker rmi "$IMAGE_TAG_NO_DOCKER" > /dev/null 2>&1 || true
    exit 1
fi

# Verify error message contains installation guide
if ! grep -q "Installation guide" /tmp/docker-test-1.log; then
    echo "ERROR: Output should contain 'Installation guide'"
    cat /tmp/docker-test-1.log
    docker rmi "$IMAGE_TAG_NO_DOCKER" > /dev/null 2>&1 || true
    exit 1
fi

echo "✓ Correctly failed with installation guide when docker not installed"
docker rmi "$IMAGE_TAG_NO_DOCKER" > /dev/null 2>&1 || true

# Test 2: Docker IS installed - should succeed with checkmark
echo ""
echo "=== Test 2: Docker installed - expect success with checkmark ==="
DOCKERFILE_WITH_DOCKER=$(mktemp)
cat > "$DOCKERFILE_WITH_DOCKER" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Install docker CLI and minimal dependencies
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        docker.io \
        && \
    rm -rf /var/lib/apt/lists/*

# Verify docker IS installed
RUN command -v docker && echo "✓ docker in system (expected)"

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku

ENV PATH="/home/testuser/.tsuku/bin:${PATH}"
ENV HOME="/home/testuser"
EOF

IMAGE_TAG_WITH_DOCKER="tsuku-docker-test-with-docker:$$"
echo "Building Docker image (with docker CLI)..."
docker build -t "$IMAGE_TAG_WITH_DOCKER" -f "$DOCKERFILE_WITH_DOCKER" . > /dev/null
rm "$DOCKERFILE_WITH_DOCKER"

echo "Running tsuku install docker (should succeed)..."
docker run --rm "$IMAGE_TAG_WITH_DOCKER" ./tsuku install docker 2>&1 | tee /tmp/docker-test-2.log

# Verify success message contains checkmark
if ! grep -q "✓.*is available on your system" /tmp/docker-test-2.log; then
    echo "ERROR: Output should contain '✓ ... is available on your system'"
    cat /tmp/docker-test-2.log
    docker rmi "$IMAGE_TAG_WITH_DOCKER" > /dev/null 2>&1 || true
    exit 1
fi

# Verify note about tsuku not managing the dependency
if ! grep -q "tsuku doesn't manage this dependency" /tmp/docker-test-2.log; then
    echo "ERROR: Output should contain note about tsuku not managing the dependency"
    cat /tmp/docker-test-2.log
    docker rmi "$IMAGE_TAG_WITH_DOCKER" > /dev/null 2>&1 || true
    exit 1
fi

echo "✓ Success message with checkmark shown when docker is installed"

# Test 3: Verify no state entries or directories created
echo ""
echo "=== Test 3: Verify no state entries or directories created ==="
docker run --rm "$IMAGE_TAG_WITH_DOCKER" bash -c '
set -e

# Install docker system dependency
./tsuku install docker > /dev/null 2>&1

# Check that no docker-* directory was created
if [ -d "$HOME/.tsuku/tools/docker-"* ] 2>/dev/null; then
    echo "ERROR: docker-* directory should not be created in ~/.tsuku/tools/"
    ls -la "$HOME/.tsuku/tools/"
    exit 1
fi

# Check state.json for docker entry
if [ -f "$HOME/.tsuku/state.json" ]; then
    if grep -q "\"docker\"" "$HOME/.tsuku/state.json"; then
        echo "ERROR: docker should not be in state.json"
        cat "$HOME/.tsuku/state.json"
        exit 1
    fi
fi

echo "✓ No state entries or directories created for system dependency"
'

docker rmi "$IMAGE_TAG_WITH_DOCKER" > /dev/null 2>&1 || true

echo ""
echo "=== ALL TESTS PASSED ==="
echo "✓ docker recipe fails gracefully when docker not installed"
echo "✓ docker recipe shows installation guide on failure"
echo "✓ docker recipe succeeds with checkmark when docker installed"
echo "✓ docker recipe shows note about tsuku not managing it"
echo "✓ No state entries or directories created"
exit 0
