#!/bin/bash
set -euo pipefail

# tsuku installer
# Downloads and installs the latest tsuku release

REPO="tsuku-dev/tsuku"
INSTALL_DIR="${TSUKU_INSTALL_DIR:-$HOME/.tsuku}"
BIN_DIR="$INSTALL_DIR/bin"

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
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
    echo "Failed to determine latest version" >&2
    exit 1
fi

echo "Latest version: $LATEST"

# Download binary
BINARY_NAME="tsuku-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST}/${BINARY_NAME}"
CHECKSUM_URL="${DOWNLOAD_URL}.sha256"

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

echo "Downloading ${BINARY_NAME}..."
curl -fsSL -o "$TEMP_DIR/tsuku" "$DOWNLOAD_URL"
curl -fsSL -o "$TEMP_DIR/tsuku.sha256" "$CHECKSUM_URL"

# Verify checksum
echo "Verifying checksum..."
cd "$TEMP_DIR"
if command -v sha256sum &>/dev/null; then
    echo "$(cat tsuku.sha256 | awk '{print $1}')  tsuku" | sha256sum -c - >/dev/null
elif command -v shasum &>/dev/null; then
    echo "$(cat tsuku.sha256 | awk '{print $1}')  tsuku" | shasum -a 256 -c - >/dev/null
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

# Check if bin directory is in PATH
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo "Add tsuku to your PATH by adding this to your shell config:"
    echo ""
    echo "  export PATH=\"\$HOME/.tsuku/bin:\$PATH\""
    echo ""
fi
