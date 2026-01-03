#!/bin/bash
# Test that tsuku can provision cmake and build cmake-based projects in a
# clean environment without system cmake.
#
# This validates:
# - cmake recipe installs successfully in isolated container
# - ninja recipe builds successfully using cmake_build action
#
# Uses tsuku's --sandbox flag to automatically build containers with
# system dependencies extracted from installation plans.
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed

set -e

echo "=== Testing CMake Provisioning with Sandbox ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

echo ""
echo "=== Test 1: Install cmake via sandbox ==="
./tsuku install cmake --sandbox --force
echo "✓ cmake sandbox test passed"

echo ""
echo "=== Test 2: Build ninja using cmake_build action ==="
# This is the ultimate test - building ninja requires:
# - cmake (to run the build)
# - make (invoked by cmake)
# - zig (as the C++ compiler)
# Sandbox automatically extracts and provisions all system dependencies
./tsuku install ninja --sandbox --force
echo "✓ ninja sandbox test passed"

echo ""
echo "=== ALL TESTS PASSED ==="
echo "✓ cmake recipe installs in isolated container"
echo "✓ ninja builds successfully using cmake_build action"
echo "✓ Sandbox automatically provisioned all system dependencies"
exit 0
