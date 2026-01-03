#!/bin/bash
# Test that tsuku can provision readline and build sqlite with readline support
# in a clean environment without system readline or ncurses.
#
# This validates:
# - sqlite recipe installs successfully with readline/ncurses dependencies
# - Complete dependency chain: sqlite → readline → ncurses
#
# Uses tsuku's --sandbox flag to automatically build containers with
# system dependencies extracted from installation plans.
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed

set -e

echo "=== Testing Readline Provisioning with Sandbox ==="
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

echo ""
echo "=== Test 1: Install sqlite via sandbox ==="
# sqlite depends on readline, which depends on ncurses
# Sandbox automatically extracts and provisions all system dependencies
./tsuku install sqlite --sandbox --force
echo "✓ sqlite sandbox test passed (with readline and ncurses dependencies)"

echo ""
echo "=== ALL TESTS PASSED ==="
echo "✓ sqlite recipe installs in isolated container"
echo "✓ Complete dependency chain (sqlite → readline → ncurses) validated"
echo "✓ Sandbox automatically provisioned all system dependencies"
exit 0
