#!/bin/bash
# Test CUDA system dependency recipe in isolated Docker environments.
#
# This validates:
# - cuda recipe fails gracefully when CUDA is not installed
# - cuda recipe succeeds with checkmark when CUDA is installed (mock nvcc)
# - cuda recipe shows platform-specific messages (macOS unsupported)
# - cuda recipe validates minimum version constraint (11.0)
# - No state entries or directories created for system dependencies
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed

set -e

# Ensure we use the correct Docker API version
export DOCKER_API_VERSION=1.52

echo "=== Testing CUDA System Dependency Recipe ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Test 1: CUDA NOT installed - should fail with installation guide
echo ""
echo "=== Test 1: CUDA not installed - expect failure with guide ==="
DOCKERFILE_NO_CUDA=$(mktemp)
cat > "$DOCKERFILE_NO_CUDA" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Install minimal dependencies - explicitly NO CUDA/nvcc
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        && \
    rm -rf /var/lib/apt/lists/*

# Verify nvcc is NOT installed
RUN ! command -v nvcc && echo "✓ nvcc not in system (expected)"

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku

ENV PATH="/home/testuser/.tsuku/bin:${PATH}"
EOF

IMAGE_TAG_NO_CUDA="tsuku-cuda-test-no-cuda:$$"
echo "Building Docker image (Ubuntu 22.04 without CUDA)..."
docker build -t "$IMAGE_TAG_NO_CUDA" -f "$DOCKERFILE_NO_CUDA" . > /dev/null
rm "$DOCKERFILE_NO_CUDA"

echo "Running tsuku install cuda (should fail)..."
# Disable errexit temporarily to capture failure
set +e
docker run --rm "$IMAGE_TAG_NO_CUDA" ./tsuku install cuda 2>&1 | tee /tmp/cuda-test-1.log
EXIT_CODE=${PIPESTATUS[0]}
set -e

if [ $EXIT_CODE -eq 0 ]; then
    echo "ERROR: Expected tsuku install cuda to fail when CUDA is not installed"
    docker rmi "$IMAGE_TAG_NO_CUDA" > /dev/null 2>&1 || true
    exit 1
fi

# Verify error message contains installation guide
if ! grep -q "Installation guide" /tmp/cuda-test-1.log; then
    echo "ERROR: Output should contain 'Installation guide'"
    cat /tmp/cuda-test-1.log
    docker rmi "$IMAGE_TAG_NO_CUDA" > /dev/null 2>&1 || true
    exit 1
fi

# Verify installation guide mentions NVIDIA
if ! grep -qi "nvidia" /tmp/cuda-test-1.log; then
    echo "ERROR: Installation guide should mention NVIDIA"
    cat /tmp/cuda-test-1.log
    docker rmi "$IMAGE_TAG_NO_CUDA" > /dev/null 2>&1 || true
    exit 1
fi

echo "✓ Correctly failed with installation guide when CUDA not installed"
docker rmi "$IMAGE_TAG_NO_CUDA" > /dev/null 2>&1 || true

# Test 2: CUDA IS installed - should succeed with checkmark
echo ""
echo "=== Test 2: CUDA installed - expect success with checkmark ==="
DOCKERFILE_WITH_CUDA=$(mktemp)
cat > "$DOCKERFILE_WITH_CUDA" << 'EOF'
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Install minimal dependencies
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

# Create a mock nvcc that outputs a valid CUDA version (12.2)
RUN mkdir -p /home/testuser/bin && \
    echo '#!/bin/bash' > /home/testuser/bin/nvcc && \
    echo 'if [ "$1" = "--version" ]; then' >> /home/testuser/bin/nvcc && \
    echo '  echo "nvcc: NVIDIA (R) Cuda compiler driver"' >> /home/testuser/bin/nvcc && \
    echo '  echo "Copyright (c) 2005-2023 NVIDIA Corporation"' >> /home/testuser/bin/nvcc && \
    echo '  echo "Built on Tue_Aug_15_22:02:13_PDT_2023"' >> /home/testuser/bin/nvcc && \
    echo '  echo "Cuda compilation tools, release 12.2, V12.2.140"' >> /home/testuser/bin/nvcc && \
    echo '  echo "Build cuda_12.2.r12.2/compiler.33191640_0"' >> /home/testuser/bin/nvcc && \
    echo 'fi' >> /home/testuser/bin/nvcc && \
    chmod +x /home/testuser/bin/nvcc

# Verify nvcc IS available and outputs expected format
RUN /home/testuser/bin/nvcc --version && echo "✓ nvcc mock available (expected)"

COPY --chown=testuser:testuser tsuku /home/testuser/tsuku

ENV PATH="/home/testuser/bin:/home/testuser/.tsuku/bin:${PATH}"
ENV HOME="/home/testuser"
EOF

IMAGE_TAG_WITH_CUDA="tsuku-cuda-test-with-cuda:$$"
echo "Building Docker image (with mock nvcc)..."
docker build -t "$IMAGE_TAG_WITH_CUDA" -f "$DOCKERFILE_WITH_CUDA" . > /dev/null
rm "$DOCKERFILE_WITH_CUDA"

echo "Running tsuku install cuda (should succeed)..."
docker run --rm "$IMAGE_TAG_WITH_CUDA" ./tsuku install cuda 2>&1 | tee /tmp/cuda-test-2.log

# Verify success message contains checkmark
if ! grep -q "✓.*is available on your system" /tmp/cuda-test-2.log; then
    echo "ERROR: Output should contain '✓ ... is available on your system'"
    cat /tmp/cuda-test-2.log
    docker rmi "$IMAGE_TAG_WITH_CUDA" > /dev/null 2>&1 || true
    exit 1
fi

# Verify note about tsuku not managing the dependency
if ! grep -q "tsuku doesn't manage this dependency" /tmp/cuda-test-2.log; then
    echo "ERROR: Output should contain note about tsuku not managing the dependency"
    cat /tmp/cuda-test-2.log
    docker rmi "$IMAGE_TAG_WITH_CUDA" > /dev/null 2>&1 || true
    exit 1
fi

echo "✓ Success message with checkmark shown when CUDA is installed"

# Test 3: Verify no state entries or directories created
echo ""
echo "=== Test 3: Verify no state entries or directories created ==="
docker run --rm "$IMAGE_TAG_WITH_CUDA" bash -c '
set -e

# Install cuda system dependency
./tsuku install cuda > /dev/null 2>&1

# Check that no cuda-* directory was created
if [ -d "$HOME/.tsuku/tools/cuda-"* ] 2>/dev/null; then
    echo "ERROR: cuda-* directory should not be created in ~/.tsuku/tools/"
    ls -la "$HOME/.tsuku/tools/"
    exit 1
fi

# Check state.json for cuda entry
if [ -f "$HOME/.tsuku/state.json" ]; then
    if grep -q "\"cuda\"" "$HOME/.tsuku/state.json"; then
        echo "ERROR: cuda should not be in state.json"
        cat "$HOME/.tsuku/state.json"
        exit 1
    fi
fi

echo "✓ No state entries or directories created for system dependency"
'

docker rmi "$IMAGE_TAG_WITH_CUDA" > /dev/null 2>&1 || true

# Test 4: Verify minimum version constraint check
echo ""
echo "=== Test 4: Verify minimum version constraint ==="

echo "Validating CUDA recipe has min_version constraint..."
if ! grep -q 'min_version = "11.0"' internal/recipe/recipes/c/cuda.toml; then
    echo "ERROR: cuda.toml should have min_version = \"11.0\""
    exit 1
fi
echo "✓ CUDA recipe has minimum version constraint (11.0)"

# Test 5: Recipe validation
echo ""
echo "=== Test 5: Recipe validation ==="
./tsuku validate --strict internal/recipe/recipes/c/cuda.toml > /tmp/cuda-validate.log 2>&1

if ! grep -q "Valid recipe: cuda" /tmp/cuda-validate.log; then
    echo "ERROR: CUDA recipe validation failed"
    cat /tmp/cuda-validate.log
    exit 1
fi

echo "✓ CUDA recipe validates successfully"

# Test 6: Verify platform-specific messaging
echo ""
echo "=== Test 6: Verify platform-specific installation guides ==="

# Check that cuda.toml has macOS-specific message
if ! grep -q "CUDA is not supported on macOS" internal/recipe/recipes/c/cuda.toml; then
    echo "ERROR: cuda.toml should have macOS unsupported message"
    exit 1
fi

# Check that cuda.toml has Linux installation link
if ! grep -q "developer.nvidia.com/cuda-downloads" internal/recipe/recipes/c/cuda.toml; then
    echo "ERROR: cuda.toml should have NVIDIA CUDA downloads link"
    exit 1
fi

echo "✓ Platform-specific installation guides present"

echo ""
echo "=== ALL TESTS PASSED ==="
echo "✓ CUDA recipe fails gracefully when CUDA not installed"
echo "✓ CUDA recipe succeeds with checkmark when CUDA installed (mock nvcc)"
echo "✓ CUDA recipe shows note about tsuku not managing it"
echo "✓ No state entries or directories created"
echo "✓ CUDA recipe has minimum version constraint (11.0)"
echo "✓ CUDA recipe validates successfully"
echo "✓ Platform-specific installation guides present (macOS unsupported, Linux links)"
exit 0
