#!/bin/bash
# Test library header verification using sandbox containers.
# This validates that library recipes install correctly and that
# tsuku verify performs Tier 1 header validation on the installed
# shared library files.
#
# Uses tsuku sandbox to run tests in isolated containers.
#
# Usage: ./scripts/test-library-verify.sh <library-name> [family]
#   library-name: Library recipe to test (e.g., zlib, libyaml)
#   family: debian, rhel, arch, alpine, suse (default: debian)
#
# Exit codes:
#   0 - Library installation and verification passed
#   1 - Library installation or verification failed

set -e

LIBRARY_NAME="${1:-}"
FAMILY="${2:-debian}"

if [ -z "$LIBRARY_NAME" ]; then
    echo "Usage: $0 <library-name> [family]"
    echo "Example: $0 zlib debian"
    echo "         $0 libyaml alpine"
    exit 1
fi

echo "=== Testing library verification: $LIBRARY_NAME (family: $FAMILY) ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Generate plan with dependencies
echo "Generating plan for $LIBRARY_NAME (family: $FAMILY)..."
./tsuku eval "$LIBRARY_NAME" --os linux --linux-family "$FAMILY" --install-deps > "$LIBRARY_NAME-$FAMILY.json"
echo "Plan generated"

# Run sandbox test - this installs the library
echo ""
echo "Installing $LIBRARY_NAME in sandbox (family: $FAMILY)..."
./tsuku install --plan "$LIBRARY_NAME-$FAMILY.json" --sandbox --force
echo "Library installed"

# Verify the library - this runs Tier 1 header validation
echo ""
echo "Verifying $LIBRARY_NAME..."
./tsuku verify "$LIBRARY_NAME"
echo "Library verified"

# Clean up plan file
rm -f "$LIBRARY_NAME-$FAMILY.json"

echo ""
echo "=== PASS: $LIBRARY_NAME installation and Tier 1 header verification succeeded (family: $FAMILY) ==="
exit 0
