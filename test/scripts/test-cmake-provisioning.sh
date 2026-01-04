#!/bin/bash
# Test that tsuku can provision cmake and build cmake-based projects across
# different Linux distribution families.
#
# This validates:
# - cmake recipe installs successfully in isolated containers
# - ninja recipe builds successfully using cmake_build action
# - System dependencies are correctly extracted for each family
# - Container specs are generated for debian, rhel, arch, alpine, suse
#
# Uses tsuku eval --linux-family to generate plans for different families,
# then tsuku install --plan --sandbox to test in isolated containers.
#
# Takes an optional family argument to test a specific family only.
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed

set -e

FAMILY="${1:-all}"

echo "=== Testing CMake Provisioning with Sandbox (Multi-Family) ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku
echo ""

# Define families to test
if [ "$FAMILY" = "all" ]; then
    FAMILIES=("debian" "rhel" "arch" "alpine" "suse")
else
    FAMILIES=("$FAMILY")
fi

for family in "${FAMILIES[@]}"; do
    echo "=== Testing family: $family ==="
    echo ""

    echo "Generating plan for cmake (family: $family)..."
    ./tsuku eval cmake --os linux --linux-family "$family" --install-deps > "cmake-$family.json"
    echo "✓ Plan generated"

    echo "Running sandbox test for cmake (family: $family)..."
    ./tsuku install --plan "cmake-$family.json" --sandbox --force
    echo "✓ cmake sandbox test passed for $family"

    echo "Generating plan for ninja (family: $family)..."
    ./tsuku eval ninja --os linux --linux-family "$family" --install-deps > "ninja-$family.json"
    echo "✓ Plan generated"

    echo "Running sandbox test for ninja (family: $family)..."
    ./tsuku install --plan "ninja-$family.json" --sandbox --force
    echo "✓ ninja sandbox test passed for $family"

    # Clean up plan files
    rm -f "cmake-$family.json" "ninja-$family.json"

    echo ""
done

echo "=== ALL TESTS PASSED ==="
if [ "$FAMILY" = "all" ]; then
    echo "✓ Tested across all Linux families: ${FAMILIES[*]}"
else
    echo "✓ Tested family: $FAMILY"
fi
echo "✓ cmake recipe installs in isolated containers"
echo "✓ ninja builds successfully using cmake_build action"
echo "✓ Sandbox automatically provisioned system dependencies for each family"
exit 0
