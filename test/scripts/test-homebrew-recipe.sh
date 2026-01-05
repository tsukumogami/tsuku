#!/bin/bash
# Test a homebrew-based recipe using sandbox containers.
# This validates that homebrew recipes install correctly with patchelf (Linux)
# or install_name_tool (macOS) handling RPATH fixup.
#
# Uses tsuku sandbox to run tests in isolated containers without hardcoding
# apt-get calls. System dependencies (including patchelf) are automatically
# provisioned based on the Linux distribution family.
#
# Usage: ./scripts/test-homebrew-recipe.sh <tool-name> [family]
#   tool-name: Recipe to test (e.g., pkg-config, tree)
#   family: debian, rhel, arch, alpine, suse (default: debian)
#
# Exit codes:
#   0 - Tool installation and verification passed
#   1 - Tool installation or verification failed

set -e

TOOL_NAME="${1:-}"
FAMILY="${2:-debian}"

if [ -z "$TOOL_NAME" ]; then
    echo "Usage: $0 <tool-name> [family]"
    echo "Example: $0 pkg-config debian"
    echo "         $0 tree alpine"
    exit 1
fi

echo "=== Testing homebrew recipe: $TOOL_NAME (family: $FAMILY) ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Generate plan with dependencies (includes patchelf via homebrew_relocate action)
echo "Generating plan for $TOOL_NAME (family: $FAMILY)..."
./tsuku eval "$TOOL_NAME" --os linux --linux-family "$FAMILY" --install-deps > "$TOOL_NAME-homebrew-$FAMILY.json"
echo "Plan generated"

# Run sandbox test - this installs the tool and runs verification
echo ""
echo "Running sandbox test for $TOOL_NAME (family: $FAMILY)..."
./tsuku install --plan "$TOOL_NAME-homebrew-$FAMILY.json" --sandbox --force
echo "Sandbox test passed"

# Clean up plan file
rm -f "$TOOL_NAME-homebrew-$FAMILY.json"

echo ""
echo "=== PASS: $TOOL_NAME installation and verification succeeded (family: $FAMILY) ==="
exit 0
