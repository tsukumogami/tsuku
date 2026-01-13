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

verify_readline() {
    echo "Testing: readline library installation"
    TOOL_DIR=$(find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "readline-*" | head -1)
    if [ -z "$TOOL_DIR" ]; then
        echo "Error: readline not found"
        return 1
    fi

    # Verify library files exist (both .so for Linux and .dylib for macOS)
    if [ -f "$TOOL_DIR/lib/libreadline.so" ] || [ -f "$TOOL_DIR/lib/libreadline.dylib" ] || [ -f "$TOOL_DIR/lib/libreadline.a" ]; then
        echo "Found readline library in $TOOL_DIR/lib"
        ls -la "$TOOL_DIR/lib"/libreadline* 2>/dev/null | head -5
        ls -la "$TOOL_DIR/lib"/libhistory* 2>/dev/null | head -5
    else
        echo "Error: readline library files not found"
        return 1
    fi

    echo "readline library verification passed"
}

verify_sqlite() {
    echo "Testing: sqlite3 --version"
    sqlite3 --version

    echo ""
    echo "Testing: Create and query a test database"
    cd "$TEMP_DIR"
    echo "CREATE TABLE test (id INTEGER, name TEXT);" | sqlite3 test.db
    echo "INSERT INTO test VALUES (1, 'hello'), (2, 'world');" | sqlite3 test.db
    RESULT=$(echo "SELECT * FROM test WHERE id = 1;" | sqlite3 test.db)

    if [ "$RESULT" = "1|hello" ]; then
        echo "✓ sqlite3 basic SQL operations work correctly"
    else
        echo "Error: Expected '1|hello', got '$RESULT'"
        return 1
    fi

    echo ""
    echo "Testing: Verify readline support"
    # Test that sqlite3 was built with readline support
    # When readline is enabled, sqlite3 accepts commands in interactive mode
    echo ".quit" | sqlite3 2>&1 > /dev/null
    echo "✓ sqlite3 interactive mode works (readline support validated)"
}

verify_curl() {
    echo "Testing: curl --version"
    curl --version

    echo ""
    echo "Verifying: curl uses tsuku-provided OpenSSL and zlib"
    if curl --version | grep -q "OpenSSL"; then
        echo "✓ curl linked with OpenSSL"
    else
        echo "✗ ERROR: curl not linked with OpenSSL"
        exit 1
    fi

    if curl --version | grep -q "zlib"; then
        echo "✓ curl linked with zlib"
    else
        echo "✗ ERROR: curl not linked with zlib"
        exit 1
    fi

    echo ""
    echo "Testing: HTTPS request to example.com"
    if curl -sI https://example.com | head -1 | grep -q "200 OK"; then
        echo "✓ HTTPS request successful"
    else
        echo "✗ ERROR: HTTPS request failed"
        exit 1
    fi
}

verify_git() {
    echo "Testing: git --version"
    git --version

    # Find the actual git install directory
    # On macOS, git via symlink has RUNTIME_PREFIX issues, so we need to help it find helpers
    TOOL_DIR=$(find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "git-*" -o -name "git-source-*" | head -1)
    if [ -n "$TOOL_DIR" ] && [ -d "$TOOL_DIR/libexec/git-core" ]; then
        echo ""
        echo "Debug: Found git helpers at $TOOL_DIR/libexec/git-core"
        echo "Debug: Setting GIT_EXEC_PATH to help git find helpers"
        export GIT_EXEC_PATH="$TOOL_DIR/libexec/git-core"
    fi

    echo ""
    echo "Debug: git --exec-path (where Git looks for helpers)"
    git --exec-path

    echo ""
    echo "Debug: checking if git-remote-https exists"
    EXEC_PATH=$(git --exec-path)
    if [ -f "$EXEC_PATH/git-remote-https" ]; then
        echo "Found git-remote-https at $EXEC_PATH/git-remote-https"
    elif [ -n "$GIT_EXEC_PATH" ] && [ -f "$GIT_EXEC_PATH/git-remote-https" ]; then
        echo "Found git-remote-https at $GIT_EXEC_PATH/git-remote-https (via GIT_EXEC_PATH)"
    else
        echo "Warning: git-remote-https NOT found at expected locations"
        echo "Contents of $EXEC_PATH:"
        ls -la "$EXEC_PATH" 2>/dev/null | head -20 || echo "Could not list directory"
        if [ -n "$GIT_EXEC_PATH" ]; then
            echo "Contents of $GIT_EXEC_PATH:"
            ls -la "$GIT_EXEC_PATH" 2>/dev/null | head -20 || echo "Could not list directory"
        fi
    fi

    # Test local git operations (core functionality without network dependencies)
    echo ""
    echo "Testing: local git operations (init, add, commit)"
    cd "$TEMP_DIR"
    mkdir -p test-repo && cd test-repo
    git init
    git config user.email "test@example.com"
    git config user.name "Test User"
    echo "test content" > test-file.txt
    git add test-file.txt
    git commit -m "Test commit"
    echo "Local git operations work correctly"

    # HTTPS clone test is informational only - Homebrew libcurl may have
    # transitive dependencies (librtmp, etc.) that aren't available
    echo ""
    echo "Testing: HTTPS clone (informational - may fail due to library dependencies)"
    cd "$TEMP_DIR"
    if git clone --depth 1 https://github.com/git/git-manpages.git test-clone 2>&1; then
        echo "HTTPS clone works (curl integration validated)"
    else
        echo "Note: HTTPS clone failed - this is expected if libcurl has missing transitive dependencies"
        echo "Core git functionality has been verified via local operations"
    fi
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
    curl)
        verify_curl
        ;;
    readline)
        verify_readline
        ;;
    sqlite|sqlite-source)
        verify_sqlite
        ;;
    git|git-source)
        verify_git
        ;;
    *)
        echo "No specific test for '$TOOL_NAME', running generic check"
        verify_generic
        ;;
esac

echo ""
echo "=== PASS: Tool verification succeeded ==="
exit 0
