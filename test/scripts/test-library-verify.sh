#!/bin/bash
# Test library header verification.
# This validates that library recipes install correctly and that
# tsuku verify performs Tier 1 header validation on the installed
# shared library files.
#
# Installs libraries directly (not in sandbox) to persist state for verification.
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

# Use a temporary TSUKU_HOME to avoid polluting the system
export TSUKU_HOME="$(mktemp -d)"
trap "rm -rf $TSUKU_HOME" EXIT
echo "Using TSUKU_HOME=$TSUKU_HOME"
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Install the library directly (not in sandbox) to populate state
echo ""
echo "Installing $LIBRARY_NAME..."
./tsuku install "$LIBRARY_NAME" --force
echo "Library installed"

# Verify the library - this runs Tier 1 header validation
echo ""
echo "Verifying $LIBRARY_NAME..."
./tsuku verify "$LIBRARY_NAME"
echo "Library verified"

echo ""
echo "=== PASS: $LIBRARY_NAME installation and Tier 1 header verification succeeded (family: $FAMILY) ==="
exit 0
