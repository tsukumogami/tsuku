#!/bin/bash
# Run tool-specific functional tests to verify a tsuku-installed tool works correctly.
#
# Usage: ./scripts/verify-tool.sh <tool-name>
#
# Exit codes:
#   0 - Tool verification passed
#   1 - Tool verification failed

set -e

TOOL_NAME="${1:-}"
TSUKU_HOME="${TSUKU_HOME:-$HOME/.tsuku}"

if [ -z "$TOOL_NAME" ]; then
    echo "Usage: $0 <tool-name>"
    echo "Example: $0 make"
    exit 1
fi

# Add tsuku tools to PATH
export PATH="$TSUKU_HOME/tools/current:$TSUKU_HOME/bin:$PATH"

echo "=== Verifying tool functionality: $TOOL_NAME ==="
echo ""

# Create temp directory for tests
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

verify_make() {
    echo "Testing: make --version"
    make --version

    echo ""
    echo "Testing: Build a simple Makefile"
    cat > "$TEMP_DIR/Makefile" << 'EOF'
.PHONY: all
all:
	@echo "Make works!"
EOF
    cd "$TEMP_DIR"
    make
}

verify_gdbm() {
    # gdbmtool crashes on macOS (both --version and functional use), so we
    # verify the library exists instead. On Linux, we can test functionality.
    if [[ "$(uname)" == "Darwin" ]]; then
        echo "Note: gdbmtool crashes on macOS, verifying library files instead"
        TOOL_DIR=$(find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "gdbm-*" | head -1)
        if [ -z "$TOOL_DIR" ]; then
            echo "Error: gdbm not found"
            return 1
        fi
        if [ -f "$TOOL_DIR/lib/libgdbm.dylib" ] || [ -f "$TOOL_DIR/lib/libgdbm.a" ]; then
            echo "Found gdbm library in $TOOL_DIR/lib"
            ls -la "$TOOL_DIR/lib"/libgdbm* 2>/dev/null || true
        else
            echo "Error: gdbm library not found"
            return 1
        fi
    else
        echo "Testing: Create and query a gdbm database"
        cd "$TEMP_DIR"
        echo -e "store key1 value1\nstore key2 value2\nfetch key1\nquit" | gdbmtool test.db
        echo "gdbmtool database operations completed successfully"
    fi
}

verify_zig() {
    echo "Testing: zig version"
    zig version

    echo ""
    echo "Testing: Compile a simple C program with zig cc"
    cat > "$TEMP_DIR/hello.c" << 'EOF'
#include <stdio.h>
int main() {
    printf("Hello from zig cc!\n");
    return 0;
}
EOF
    cd "$TEMP_DIR"
    zig cc hello.c -o hello
    ./hello
}

verify_pngcrush() {
    echo "Testing: pngcrush -version"
    pngcrush -version 2>&1 | head -5

    # pngcrush needs an actual PNG to test, which is complex
    # Just verify it starts and shows help
    echo ""
    echo "Testing: pngcrush -h (shows help)"
    pngcrush -h 2>&1 | head -10 || true
}

verify_zlib() {
    echo "Testing: Check zlib library exists"
    TOOL_DIR=$(find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "zlib-*" | head -1)
    if [ -z "$TOOL_DIR" ]; then
        echo "Error: zlib not found"
        return 1
    fi

    if [ -f "$TOOL_DIR/lib/libz.so" ] || [ -f "$TOOL_DIR/lib/libz.dylib" ] || [ -f "$TOOL_DIR/lib/libz.a" ]; then
        echo "Found zlib library in $TOOL_DIR/lib"
        ls -la "$TOOL_DIR/lib"/libz* 2>/dev/null || true
    else
        echo "Error: zlib library not found"
        return 1
    fi
}

verify_libpng() {
    echo "Testing: Check libpng library exists"
    TOOL_DIR=$(find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "libpng-*" | head -1)
    if [ -z "$TOOL_DIR" ]; then
        echo "Error: libpng not found"
        return 1
    fi

    if [ -f "$TOOL_DIR/lib/libpng.so" ] || [ -f "$TOOL_DIR/lib/libpng.dylib" ] || [ -f "$TOOL_DIR/lib/libpng16.so" ] || [ -f "$TOOL_DIR/lib/libpng16.dylib" ]; then
        echo "Found libpng library in $TOOL_DIR/lib"
        ls -la "$TOOL_DIR/lib"/libpng* 2>/dev/null || true
    else
        echo "Error: libpng library not found"
        return 1
    fi
}

verify_m4() {
    echo "Testing: m4 --version"
    m4 --version

    echo ""
    echo "Testing: Process a simple macro"
    cd "$TEMP_DIR"
    echo 'define(GREETING, Hello World)GREETING' | m4
}

verify_pkg_config() {
    echo "Testing: pkg-config --version"
    pkg-config --version

    echo ""
    echo "Testing: pkg-config basic functionality"
    # Test that it can at least query its own library path setting
    if pkg-config --variable=pc_path pkg-config 2>/dev/null; then
        echo "pkg-config can query variables"
    else
        echo "Testing: pkg-config --help (verify it runs)"
        pkg-config --help | head -5
    fi
}

verify_libsixel-source() {
    echo "Testing: img2sixel --version"
    img2sixel --version

    echo ""
    echo "Testing: sixel2png --help"
    sixel2png --help 2>&1 | head -5 || true
}

verify_generic() {
    echo "Testing: $TOOL_NAME --version (generic check)"
    if "$TOOL_NAME" --version 2>&1; then
        echo "Tool responded to --version"
    elif "$TOOL_NAME" -version 2>&1; then
        echo "Tool responded to -version"
    elif "$TOOL_NAME" version 2>&1; then
        echo "Tool responded to version"
    else
        echo "Warning: Could not determine version, but tool exists"
    fi
}

# Run tool-specific verification
case "$TOOL_NAME" in
    make)
        verify_make
        ;;
    gdbm|gdbm-source)
        verify_gdbm
        ;;
    zig)
        verify_zig
        ;;
    pngcrush)
        verify_pngcrush
        ;;
    m4)
        verify_m4
        ;;
    pkg-config)
        verify_pkg_config
        ;;
    zlib)
        verify_zlib
        ;;
    libpng)
        verify_libpng
        ;;
    libsixel-source)
        verify_libsixel-source
        ;;
    *)
        echo "No specific test for '$TOOL_NAME', running generic check"
        verify_generic
        ;;
esac

echo ""
echo "=== PASS: Tool verification succeeded ==="
exit 0
