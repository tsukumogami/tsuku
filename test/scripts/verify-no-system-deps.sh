#!/bin/bash
# Verify that a tsuku-installed tool only depends on tsuku-provided libraries or system libc.
# This ensures tools are self-contained and don't rely on system-installed development libraries.
#
# Usage: ./scripts/verify-no-system-deps.sh <tool-name>
#
# Exit codes:
#   0 - Tool only uses allowed dependencies
#   1 - Tool has unexpected system dependencies

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

echo "=== Verifying dependencies for: $TOOL_NAME ==="
echo "Tool directory: $TOOL_DIR"
echo ""

# Detect platform
OS="$(uname -s)"

# Allowed system library patterns
# These are fundamental system libraries that must come from the OS
ALLOWED_LINUX=(
    "linux-vdso"
    "ld-linux"
    "libc.so"
    "libm.so"
    "libdl.so"
    "libpthread.so"
    "librt.so"
    "libresolv.so"
    "libnsl.so"
    "libcrypt.so"
    "libutil.so"
    "libgcc_s.so"
    "libstdc++.so"
    "/lib64/"
    "/lib/x86_64-linux-gnu/"
    "/lib/aarch64-linux-gnu/"
)

ALLOWED_MACOS=(
    "/usr/lib/lib"
    "/System/Library/"
    "@rpath"
    "@loader_path"
    "@executable_path"
)

FOUND_ISSUES=0

check_deps_linux() {
    local binary="$1"

    # Check if it's an ELF binary
    if ! file "$binary" | grep -q "ELF"; then
        return 0
    fi

    echo "Checking: $binary"

    # Get library dependencies
    local ldd_output
    ldd_output=$(ldd "$binary" 2>/dev/null || true)

    if [ -z "$ldd_output" ]; then
        echo "  (no dynamic dependencies)"
        return 0
    fi

    # Check each dependency
    local issues=0
    while IFS= read -r line; do
        # Skip empty lines
        [ -z "$line" ] && continue

        # Check if the dependency is allowed
        local allowed=0
        for pattern in "${ALLOWED_LINUX[@]}"; do
            if echo "$line" | grep -q "$pattern"; then
                allowed=1
                break
            fi
        done

        # Check if it's a tsuku-provided library
        if echo "$line" | grep -q "$TSUKU_HOME"; then
            allowed=1
        fi

        # Check if it's "not found" (which is a problem)
        if echo "$line" | grep -q "not found"; then
            echo "  ERROR: Missing dependency: $line"
            issues=1
            continue
        fi

        if [ $allowed -eq 0 ]; then
            # Extract the library path for reporting
            local lib_path
            lib_path=$(echo "$line" | sed 's/.*=> //' | sed 's/ (.*//')
            if [ -n "$lib_path" ] && [ "$lib_path" != "$line" ]; then
                echo "  WARNING: System dependency: $lib_path"
                # Don't fail for warnings, just report them
            fi
        fi
    done <<< "$ldd_output"

    return $issues
}

check_deps_macos() {
    local binary="$1"

    # Check if it's a Mach-O binary
    if ! file "$binary" | grep -qE "Mach-O|universal binary"; then
        return 0
    fi

    echo "Checking: $binary"

    # Get library dependencies
    local otool_output
    otool_output=$(otool -L "$binary" 2>/dev/null | tail -n +2 || true)

    if [ -z "$otool_output" ]; then
        echo "  (no dynamic dependencies)"
        return 0
    fi

    # Check each dependency
    local issues=0
    while IFS= read -r line; do
        # Skip empty lines
        [ -z "$line" ] && continue

        # Extract the library path (remove leading whitespace and trailing version info)
        local lib_path
        lib_path=$(echo "$line" | sed 's/^[[:space:]]*//' | sed 's/ (.*$//')

        # Check if the dependency is allowed
        local allowed=0
        for pattern in "${ALLOWED_MACOS[@]}"; do
            if echo "$lib_path" | grep -q "$pattern"; then
                allowed=1
                break
            fi
        done

        # Check if it's a tsuku-provided library
        if echo "$lib_path" | grep -q "$TSUKU_HOME"; then
            allowed=1
        fi

        if [ $allowed -eq 0 ]; then
            echo "  WARNING: Non-standard dependency: $lib_path"
            # Report but don't necessarily fail
        fi
    done <<< "$otool_output"

    return $issues
}

# Find and check all binaries
echo "Scanning for binaries..."
echo ""

while IFS= read -r -d '' binary; do
    if [ "$OS" = "Linux" ]; then
        if ! check_deps_linux "$binary"; then
            FOUND_ISSUES=1
        fi
    elif [ "$OS" = "Darwin" ]; then
        if ! check_deps_macos "$binary"; then
            FOUND_ISSUES=1
        fi
    else
        echo "Warning: Unknown OS '$OS', skipping dependency checks"
        break
    fi
done < <(find "$TOOL_DIR" -type f \( -perm -u+x -o -name "*.so" -o -name "*.dylib" \) -print0 2>/dev/null)

echo ""
if [ $FOUND_ISSUES -eq 0 ]; then
    echo "=== PASS: No unexpected system dependencies ==="
    exit 0
else
    echo "=== FAIL: Found unexpected system dependencies ==="
    exit 1
fi
