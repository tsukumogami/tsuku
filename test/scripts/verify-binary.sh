#!/bin/bash
# Verify binary quality of a tsuku-installed tool.
# This performs two types of checks in a single binary iteration:
#
# 1. Relocation checks: Ensure no hardcoded system paths in RPATH/install_name
# 2. Dependency checks: Ensure runtime dependencies resolve to tsuku or system libc
#
# Usage: ./scripts/verify-binary.sh <tool-name>
#
# Exit codes:
#   0 - All binaries pass quality checks
#   1 - Found issues (hardcoded paths or unexpected dependencies)

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

echo "=== Verifying binary quality for: $TOOL_NAME ==="
echo "Tool directory: $TOOL_DIR"
echo ""

# Detect platform
OS="$(uname -s)"

# ============================================================================
# Relocation checks: Patterns that indicate hardcoded non-relocatable paths
# ============================================================================
FORBIDDEN_PATTERNS=(
    "/usr/local"
    "/opt/homebrew"
    "/home/linuxbrew"
    "/nix/store"
    "@@HOMEBREW"
    "HOMEBREW_PREFIX"
)

# ============================================================================
# Dependency checks: Allowed system library patterns
# ============================================================================
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

# ============================================================================
# Linux binary checks
# ============================================================================
check_binary_linux() {
    local binary="$1"
    local issues=0

    # Check if it's an ELF binary
    if ! file "$binary" | grep -q "ELF"; then
        return 0
    fi

    echo "Checking: $binary"

    # --- Relocation checks ---
    # Check RPATH/RUNPATH for forbidden patterns
    local rpath_output
    rpath_output=$(readelf -d "$binary" 2>/dev/null | grep -E "(RPATH|RUNPATH)" || true)

    if [ -n "$rpath_output" ]; then
        echo "  RPATH/RUNPATH: $rpath_output"

        for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
            if echo "$rpath_output" | grep -q "$pattern"; then
                echo "  ERROR (relocation): Found forbidden pattern '$pattern' in RPATH"
                issues=1
            fi
        done
    fi

    # Check for hardcoded paths in the binary strings
    for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
        if strings "$binary" 2>/dev/null | grep -q "$pattern"; then
            if strings "$binary" 2>/dev/null | grep "$pattern" | grep -qE "^/|lib/|bin/"; then
                echo "  WARNING (relocation): Found '$pattern' in binary strings (may be benign)"
            fi
        fi
    done

    # --- Dependency checks ---
    local ldd_output
    ldd_output=$(ldd "$binary" 2>/dev/null || true)

    if [ -z "$ldd_output" ]; then
        echo "  (no dynamic dependencies)"
    else
        while IFS= read -r line; do
            [ -z "$line" ] && continue

            # Check if it's "not found" (which is a problem)
            if echo "$line" | grep -q "not found"; then
                echo "  ERROR (dependency): Missing dependency: $line"
                issues=1
                continue
            fi

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

            if [ $allowed -eq 0 ]; then
                local lib_path
                lib_path=$(echo "$line" | sed 's/.*=> //' | sed 's/ (.*//')
                if [ -n "$lib_path" ] && [ "$lib_path" != "$line" ]; then
                    echo "  WARNING (dependency): System dependency: $lib_path"
                fi
            fi
        done <<< "$ldd_output"
    fi

    return $issues
}

# ============================================================================
# macOS binary checks
# ============================================================================
check_binary_macos() {
    local binary="$1"
    local issues=0

    # Check if it's a Mach-O binary
    if ! file "$binary" | grep -qE "Mach-O|universal binary"; then
        return 0
    fi

    echo "Checking: $binary"

    # --- Get otool output (used for both checks) ---
    local otool_output
    otool_output=$(otool -L "$binary" 2>/dev/null || true)

    if [ -z "$otool_output" ]; then
        echo "  (no dynamic dependencies)"
        return 0
    fi

    # --- Relocation checks ---
    for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
        if echo "$otool_output" | grep -q "$pattern"; then
            echo "  ERROR (relocation): Found forbidden pattern '$pattern' in library paths:"
            echo "$otool_output" | grep "$pattern" | sed 's/^/    /'
            issues=1
        fi
    done

    # Check for absolute paths that should use @rpath
    local abs_paths
    abs_paths=$(echo "$otool_output" | grep -E "^\s+/" | grep -vE "^\s+/usr/lib|^\s+/System" || true)
    if [ -n "$abs_paths" ]; then
        echo "  WARNING (relocation): Found absolute paths (should use @rpath):"
        echo "$abs_paths" | sed 's/^/    /'
    fi

    # --- Dependency checks ---
    local deps_only
    deps_only=$(echo "$otool_output" | tail -n +2)

    while IFS= read -r line; do
        [ -z "$line" ] && continue

        local lib_path
        lib_path=$(echo "$line" | sed 's/^[[:space:]]*//' | sed 's/ (.*$//')

        local allowed=0
        for pattern in "${ALLOWED_MACOS[@]}"; do
            if echo "$lib_path" | grep -q "$pattern"; then
                allowed=1
                break
            fi
        done

        if echo "$lib_path" | grep -q "$TSUKU_HOME"; then
            allowed=1
        fi

        if [ $allowed -eq 0 ]; then
            echo "  WARNING (dependency): Non-standard dependency: $lib_path"
        fi
    done <<< "$deps_only"

    return $issues
}

# ============================================================================
# Main: Find and check all binaries
# ============================================================================
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
    echo "=== PASS: Binary quality checks passed ==="
    exit 0
else
    echo "=== FAIL: Found binary quality issues ==="
    exit 1
fi
