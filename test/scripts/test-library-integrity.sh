#!/bin/bash
# Test library integrity verification.
# This validates that library checksums are computed at install time
# and that tsuku verify --integrity correctly detects modifications.
#
# Test scenarios:
#   1. Fresh install: integrity verification should pass
#   2. Modified file: integrity verification should fail
#
# Usage: ./scripts/test-library-integrity.sh <library-name> [family]
#   library-name: Library recipe to test (e.g., zlib, libyaml)
#   family: debian, rhel, arch, alpine, suse (default: debian)
#
# Exit codes:
#   0 - All integrity tests passed
#   1 - Test failed

set -e

LIBRARY_NAME="${1:-}"
FAMILY="${2:-debian}"

if [ -z "$LIBRARY_NAME" ]; then
    echo "Usage: $0 <library-name> [family]"
    echo "Example: $0 zlib debian"
    echo "         $0 libyaml alpine"
    exit 1
fi

echo "=== Testing library integrity verification: $LIBRARY_NAME (family: $FAMILY) ==="
echo ""

# Use a temporary TSUKU_HOME to avoid polluting the system
export TSUKU_HOME="$(mktemp -d)"
trap "rm -rf $TSUKU_HOME" EXIT
echo "Using TSUKU_HOME=$TSUKU_HOME"
echo ""

# Build tsuku binary
echo "Building tsuku..."
go build -o tsuku ./cmd/tsuku

# Install the library
echo ""
echo "Installing $LIBRARY_NAME..."
./tsuku install "$LIBRARY_NAME" --force
echo "Library installed"

# Check that checksums were stored in state
echo ""
echo "Checking state.json for checksums..."
if ! grep -q '"checksums"' "$TSUKU_HOME/state.json"; then
    echo "ERROR: No checksums found in state.json"
    echo "State contents:"
    cat "$TSUKU_HOME/state.json"
    exit 1
fi
echo "Checksums present in state.json"

# Test 1: Verify integrity passes on fresh install
echo ""
echo "=== Test 1: Fresh install integrity check ==="
echo "Running: ./tsuku verify $LIBRARY_NAME --integrity"
if ! ./tsuku verify "$LIBRARY_NAME" --integrity; then
    echo "ERROR: Integrity verification failed on fresh install"
    exit 1
fi
echo "PASS: Fresh install integrity check passed"

# Test 2: Modify a file and verify integrity fails
echo ""
echo "=== Test 2: Modified file integrity check ==="

# Find the library directory
LIB_DIR=$(find "$TSUKU_HOME/libs" -maxdepth 1 -type d -name "${LIBRARY_NAME}-*" | head -1)
if [ -z "$LIB_DIR" ]; then
    echo "ERROR: Could not find library directory for $LIBRARY_NAME"
    exit 1
fi
echo "Library directory: $LIB_DIR"

# Find a regular file to modify (prefer .so or .a files, but any file will do)
TARGET_FILE=$(find "$LIB_DIR" -type f \( -name "*.so*" -o -name "*.a" -o -name "*.h" \) | head -1)
if [ -z "$TARGET_FILE" ]; then
    # Fallback: any regular file
    TARGET_FILE=$(find "$LIB_DIR" -type f | head -1)
fi

if [ -z "$TARGET_FILE" ]; then
    echo "ERROR: Could not find any file to modify in $LIB_DIR"
    exit 1
fi
echo "Modifying file: $TARGET_FILE"

# Make file writable (library files are typically read-only)
chmod +w "$TARGET_FILE"

# Append data to the file to change its checksum
echo "# modified by integrity test" >> "$TARGET_FILE"
echo "File modified"

# Verify integrity now fails
echo "Running: ./tsuku verify $LIBRARY_NAME --integrity (expecting failure)"
if ./tsuku verify "$LIBRARY_NAME" --integrity 2>&1; then
    echo "ERROR: Integrity verification should have failed after modification"
    exit 1
fi
echo "PASS: Modified file correctly detected by integrity check"

echo ""
echo "=== PASS: All integrity tests succeeded for $LIBRARY_NAME (family: $FAMILY) ==="
exit 0
