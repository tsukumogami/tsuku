#!/bin/bash
set -euo pipefail

# tsuku installer
# Downloads and installs the latest tsuku release

# Parse arguments
MODIFY_PATH=true
NO_TELEMETRY=false
for arg in "$@"; do
    case "$arg" in
        --no-modify-path)
            MODIFY_PATH=false
            ;;
        --no-telemetry)
            NO_TELEMETRY=true
            ;;
    esac
done

# Respect existing TSUKU_NO_TELEMETRY environment variable
if [ -n "${TSUKU_NO_TELEMETRY:-}" ]; then
    NO_TELEMETRY=true
fi

REPO="tsuku-dev/tsuku"
INSTALL_DIR="${TSUKU_INSTALL_DIR:-$HOME/.tsuku}"
BIN_DIR="$INSTALL_DIR/bin"
ENV_FILE="$INSTALL_DIR/env"
TELEMETRY_NOTICE_FILE="$INSTALL_DIR/telemetry_notice_shown"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux|darwin) ;;
    *)
        echo "Unsupported OS: $OS" >&2
        exit 1
        ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

echo "Detected platform: ${OS}-${ARCH}"

# Get latest release version
echo "Fetching latest release..."
# Use GITHUB_TOKEN if available to avoid rate limiting
if [ -n "${GITHUB_TOKEN:-}" ]; then
    LATEST=$(curl -fsSL -H "Authorization: token $GITHUB_TOKEN" "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
else
    LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
fi

if [ -z "$LATEST" ]; then
    echo "Failed to determine latest version" >&2
    exit 1
fi

echo "Latest version: $LATEST"

# Strip 'v' prefix from version for binary name (v0.1.0 -> 0.1.0)
VERSION="${LATEST#v}"

# Download binary
# Release assets follow goreleaser naming: tsuku-{os}-{arch}_{version}_{os}_{arch}
BINARY_NAME="tsuku-${OS}-${ARCH}_${VERSION}_${OS}_${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST}/${BINARY_NAME}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${LATEST}/checksums.txt"

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

echo "Downloading ${BINARY_NAME}..."
curl -fsSL -o "$TEMP_DIR/tsuku" "$DOWNLOAD_URL"
curl -fsSL -o "$TEMP_DIR/checksums.txt" "$CHECKSUM_URL"

# Verify checksum
echo "Verifying checksum..."
cd "$TEMP_DIR"
EXPECTED_CHECKSUM=$(grep "${BINARY_NAME}$" checksums.txt | awk '{print $1}')
if [ -z "$EXPECTED_CHECKSUM" ]; then
    echo "Error: Could not find checksum for ${BINARY_NAME}" >&2
    exit 1
fi

if command -v sha256sum &>/dev/null; then
    echo "${EXPECTED_CHECKSUM}  tsuku" | sha256sum -c - >/dev/null
elif command -v shasum &>/dev/null; then
    echo "${EXPECTED_CHECKSUM}  tsuku" | shasum -a 256 -c - >/dev/null
else
    echo "Warning: Could not verify checksum (sha256sum/shasum not found)" >&2
fi

# Install
echo "Installing to ${BIN_DIR}..."
mkdir -p "$BIN_DIR"
chmod +x "$TEMP_DIR/tsuku"
mv "$TEMP_DIR/tsuku" "$BIN_DIR/tsuku"

echo ""
echo "tsuku ${LATEST} installed successfully!"
echo ""

# Create env file with PATH exports
cat > "$ENV_FILE" << 'ENVEOF'
# tsuku shell configuration
# Add tsuku directories to PATH

# Set TSUKU_HOME if not already set
export TSUKU_HOME="${TSUKU_HOME:-$HOME/.tsuku}"

# Add tsuku bin and tools/current to PATH
export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
ENVEOF

# Add telemetry opt-out to env file if requested
if [ "$NO_TELEMETRY" = true ]; then
    cat >> "$ENV_FILE" << 'ENVEOF'

# Telemetry opt-out (set during installation)
export TSUKU_NO_TELEMETRY=1
ENVEOF
fi

# Configure shell if requested
if [ "$MODIFY_PATH" = true ]; then
    # Determine shell config file based on $SHELL
    SHELL_NAME=$(basename "$SHELL")
    SHELL_CONFIG=""

    case "$SHELL_NAME" in
        bash)
            # Prefer .bash_profile for login shells, fall back to .profile
            if [ -f "$HOME/.bash_profile" ]; then
                SHELL_CONFIG="$HOME/.bash_profile"
            elif [ -f "$HOME/.profile" ]; then
                SHELL_CONFIG="$HOME/.profile"
            else
                # Create .bash_profile if neither exists
                SHELL_CONFIG="$HOME/.bash_profile"
            fi
            ;;
        zsh)
            SHELL_CONFIG="$HOME/.zshenv"
            ;;
        *)
            echo "Unknown shell: $SHELL_NAME"
            echo "Add this to your shell config to use tsuku:"
            echo ""
            echo "  . \"$ENV_FILE\""
            echo ""
            ;;
    esac

    if [ -n "$SHELL_CONFIG" ]; then
        # Check if source line already exists (idempotent)
        SOURCE_LINE=". \"$ENV_FILE\""
        if [ -f "$SHELL_CONFIG" ] && grep -qF "$ENV_FILE" "$SHELL_CONFIG" 2>/dev/null; then
            echo "Shell already configured: $SHELL_CONFIG"
        else
            # Append source line
            {
                echo ""
                echo "# tsuku"
                echo "$SOURCE_LINE"
            } >> "$SHELL_CONFIG"
            echo "Configured shell: $SHELL_CONFIG"
        fi
        echo ""
        echo "Restart your shell or run:"
        echo "  source \"$ENV_FILE\""
    fi
else
    echo "Skipped shell configuration (--no-modify-path)"
    echo ""
    echo "To use tsuku, add this to your shell config:"
    echo "  . \"$ENV_FILE\""
    echo ""
fi

# Show telemetry notice if telemetry is enabled
if [ "$NO_TELEMETRY" = false ]; then
    # Print disclaimer to stderr
    cat >&2 << 'NOTICE'
tsuku collects anonymous usage statistics to improve the tool.
No personal information is collected. See: https://tsuku.dev/telemetry

To opt out: export TSUKU_NO_TELEMETRY=1
NOTICE
    # Create marker file so CLI doesn't show notice again
    touch "$TELEMETRY_NOTICE_FILE"
fi
