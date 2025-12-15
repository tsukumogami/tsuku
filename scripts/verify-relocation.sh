#!/bin/bash
# Verify that a tsuku-installed tool has no hardcoded system paths in its binaries.
# This checks RPATH (Linux) or install_name (macOS) for relocatability.
#
# Usage: ./scripts/verify-relocation.sh <tool-name>
#
# Exit codes:
#   0 - All binaries are properly relocatable
#   1 - Found hardcoded system paths or verification failed

set -e

TOOL_NAME="${1:-}"
TSUKU_HOME="${TSUKU_HOME:-$HOME/.tsuku}"

if [ -z "$TOOL_NAME" ]; then
    echo "Usage: $0 <tool-name>"
    echo "Example: $0 make"
    exit 1
fi

# Find the tool's installation directory
TOOL_DIR=$(find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "${TOOL_NAME}-*" | head -1)
if [ -z "$TOOL_DIR" ] || [ ! -d "$TOOL_DIR" ]; then
    echo "Error: Tool '$TOOL_NAME' not found in $TSUKU_HOME/tools"
    exit 1
fi

echo "=== Verifying relocation for: $TOOL_NAME ==="
echo "Tool directory: $TOOL_DIR"
echo ""

# Detect platform
OS="$(uname -s)"

# Patterns that indicate hardcoded non-relocatable paths
# These should NOT appear in properly relocated binaries
FORBIDDEN_PATTERNS=(
    "/usr/local"
    "/opt/homebrew"
    "/home/linuxbrew"
    "/nix/store"
    "@@HOMEBREW"
    "HOMEBREW_PREFIX"
)

FOUND_ISSUES=0

check_binary_linux() {
    local binary="$1"
    local issues=0

    # Check if it's an ELF binary
    if ! file "$binary" | grep -q "ELF"; then
        return 0
    fi

    echo "Checking: $binary"

    # Check RPATH/RUNPATH
    local rpath_output
    rpath_output=$(readelf -d "$binary" 2>/dev/null | grep -E "(RPATH|RUNPATH)" || true)

    if [ -n "$rpath_output" ]; then
        echo "  RPATH/RUNPATH: $rpath_output"

        for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
            if echo "$rpath_output" | grep -q "$pattern"; then
                echo "  ERROR: Found forbidden pattern '$pattern' in RPATH"
                issues=1
            fi
        done
    fi

    # Check for hardcoded paths in the binary strings (limited check)
    for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
        if strings "$binary" 2>/dev/null | grep -q "$pattern"; then
            # Only report if it looks like a path, not just a string constant
            if strings "$binary" 2>/dev/null | grep "$pattern" | grep -qE "^/|lib/|bin/"; then
                echo "  WARNING: Found '$pattern' in binary strings (may be benign)"
            fi
        fi
    done

    return $issues
}

check_binary_macos() {
    local binary="$1"
    local issues=0

    # Check if it's a Mach-O binary
    if ! file "$binary" | grep -qE "Mach-O|universal binary"; then
        return 0
    fi

    echo "Checking: $binary"

    # Check install_name and linked libraries
    local otool_output
    otool_output=$(otool -L "$binary" 2>/dev/null || true)

    if [ -n "$otool_output" ]; then
        for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
            if echo "$otool_output" | grep -q "$pattern"; then
                echo "  ERROR: Found forbidden pattern '$pattern' in library paths:"
                echo "$otool_output" | grep "$pattern" | sed 's/^/    /'
                issues=1
            fi
        done

        # Check that paths use @rpath, @loader_path, or @executable_path
        local abs_paths
        abs_paths=$(echo "$otool_output" | grep -E "^\s+/" | grep -vE "^\\s+/usr/lib|^\\s+/System" || true)
        if [ -n "$abs_paths" ]; then
            echo "  WARNING: Found absolute paths (should use @rpath):"
            echo "$abs_paths" | sed 's/^/    /'
        fi
    fi

    return $issues
}

# Find and check all binaries
echo "Scanning for binaries..."
echo ""

while IFS= read -r -d '' binary; do
    if [ "$OS" = "Linux" ]; then
        if ! check_binary_linux "$binary"; then
            FOUND_ISSUES=1
        fi
    elif [ "$OS" = "Darwin" ]; then
        if ! check_binary_macos "$binary"; then
            FOUND_ISSUES=1
        fi
    else
        echo "Warning: Unknown OS '$OS', skipping binary checks"
        break
    fi
done < <(find "$TOOL_DIR" -type f \( -perm -u+x -o -name "*.so" -o -name "*.dylib" -o -name "*.a" \) -print0 2>/dev/null)

echo ""
if [ $FOUND_ISSUES -eq 0 ]; then
    echo "=== PASS: No relocation issues found ==="
    exit 0
else
    echo "=== FAIL: Found relocation issues ==="
    exit 1
fi
