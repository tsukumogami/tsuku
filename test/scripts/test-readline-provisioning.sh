#!/bin/bash
# Test that tsuku can provision readline and build sqlite with readline support
# across different Linux distribution families.
#
# This validates:
# - sqlite recipe installs successfully with readline/ncurses dependencies
# - Complete dependency chain: sqlite → readline → ncurses
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

echo "=== Testing Readline Provisioning with Sandbox (Multi-Family) ==="
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

    echo "Generating plan for sqlite (family: $family)..."
    ./tsuku eval sqlite --os linux --linux-family "$family" --install-deps > "sqlite-$family.json"
    echo "✓ Plan generated"

    echo "Running sandbox test for sqlite (family: $family)..."
    # sqlite depends on readline, which depends on ncurses
    # Sandbox automatically extracts and provisions all system dependencies
    ./tsuku install --plan "sqlite-$family.json" --sandbox --force
    echo "✓ sqlite sandbox test passed for $family (with readline and ncurses dependencies)"

    # Clean up plan file
    rm -f "sqlite-$family.json"

    echo ""
done

echo "=== ALL TESTS PASSED ==="
if [ "$FAMILY" = "all" ]; then
    echo "✓ Tested across all Linux families: ${FAMILIES[*]}"
else
    echo "✓ Tested family: $FAMILY"
fi
echo "✓ sqlite recipe installs in isolated containers"
echo "✓ Complete dependency chain (sqlite → readline → ncurses) validated"
echo "✓ Sandbox automatically provisioned system dependencies for each family"
exit 0
