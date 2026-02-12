#!/usr/bin/env bash
#
# install-recipe-deps.sh - Install system packages declared by a tsuku recipe
#
# Uses tsuku info to extract system requirements (packages and repositories)
# for a target Linux family, then configures repositories and installs packages
# using the appropriate package manager.
#
# Usage:
#   install-recipe-deps.sh <family> <recipe> [tsuku-binary]
#
# Arguments:
#   family        Linux family: alpine, debian, rhel, arch, suse
#   recipe        Recipe name to extract dependencies for
#   tsuku-binary  Path to tsuku binary (default: ./tsuku)
#
# Exit codes:
#   0 - Success (packages installed or none needed)
#   1 - Error (invalid arguments, tsuku failed, install failed)
#
# Examples:
#   install-recipe-deps.sh alpine zlib
#   install-recipe-deps.sh debian curl ./build/tsuku

set -euo pipefail

# Validate arguments
if [[ $# -lt 2 ]]; then
    echo "Usage: install-recipe-deps.sh <family> <recipe> [tsuku-binary]" >&2
    exit 1
fi

FAMILY="$1"
RECIPE="$2"
TSUKU="${3:-./tsuku}"

# Validate family
case "$FAMILY" in
    alpine|debian|rhel|arch|suse)
        ;;
    *)
        echo "Error: invalid family '$FAMILY'" >&2
        echo "Valid families: alpine, debian, rhel, arch, suse" >&2
        exit 1
        ;;
esac

# Check tsuku binary exists
if [[ ! -x "$TSUKU" ]]; then
    echo "Error: tsuku binary not found or not executable: $TSUKU" >&2
    exit 1
fi

# Check jq is available
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed" >&2
    exit 1
fi

# Extract system requirements from recipe as JSON
DEPS_JSON=$("$TSUKU" info --deps-only --system --family "$FAMILY" --json "$RECIPE")

# Extract packages array (as space-separated string)
PACKAGES=$(echo "$DEPS_JSON" | jq -r '.packages // [] | join(" ")')

# Exit cleanly if no packages and no repositories
REPO_COUNT=$(echo "$DEPS_JSON" | jq '.repositories // [] | length')
if [[ -z "$PACKAGES" && "$REPO_COUNT" -eq 0 ]]; then
    echo "No system packages required for $RECIPE on $FAMILY"
    exit 0
fi

# verify_https checks that a URL uses HTTPS
verify_https() {
    local url="$1"
    local context="$2"
    if [[ ! "$url" =~ ^https:// ]]; then
        echo "Error: $context must use HTTPS: $url" >&2
        return 1
    fi
}

# verify_key_sha256 downloads a key and verifies its SHA256 hash
verify_key_sha256() {
    local key_url="$1"
    local expected_sha256="$2"
    local temp_key

    # Create secure temporary file
    temp_key=$(mktemp)
    trap "rm -f '$temp_key'" RETURN

    # Download key
    if ! curl -fsSL "$key_url" -o "$temp_key"; then
        echo "Error: failed to download GPG key from $key_url" >&2
        return 1
    fi

    # Verify SHA256
    local actual_sha256
    actual_sha256=$(sha256sum "$temp_key" | cut -d' ' -f1)
    if [[ "$actual_sha256" != "$expected_sha256" ]]; then
        echo "Error: GPG key SHA256 mismatch" >&2
        echo "  Expected: $expected_sha256" >&2
        echo "  Got:      $actual_sha256" >&2
        return 1
    fi

    # Output verified key path
    echo "$temp_key"
}

# Configure repositories first (if any)
if [[ "$REPO_COUNT" -gt 0 ]]; then
    echo "Configuring $REPO_COUNT repository/repositories for $RECIPE on $FAMILY"

    case "$FAMILY" in
        debian)
            # Process apt repositories and PPAs
            echo "$DEPS_JSON" | jq -c '.repositories // [] | .[]' | while read -r repo; do
                TYPE=$(echo "$repo" | jq -r '.type')
                case "$TYPE" in
                    ppa)
                        PPA=$(echo "$repo" | jq -r '.ppa')
                        echo "Adding PPA: $PPA"
                        add-apt-repository -y "ppa:$PPA"
                        ;;
                    repo)
                        URL=$(echo "$repo" | jq -r '.url')
                        KEY_URL=$(echo "$repo" | jq -r '.key_url // empty')
                        KEY_SHA256=$(echo "$repo" | jq -r '.key_sha256 // empty')

                        if [[ -n "$KEY_URL" ]]; then
                            # Verify HTTPS for key URL
                            verify_https "$KEY_URL" "GPG key URL"

                            if [[ -z "$KEY_SHA256" ]]; then
                                echo "Error: key_sha256 is required when key_url is provided" >&2
                                exit 1
                            fi

                            echo "Adding APT key from: $KEY_URL"
                            KEY_FILE=$(verify_key_sha256 "$KEY_URL" "$KEY_SHA256")

                            # Import verified key
                            apt-key add "$KEY_FILE"
                            rm -f "$KEY_FILE"
                        fi

                        echo "Adding APT repository: $URL"
                        echo "$URL" >> /etc/apt/sources.list.d/tsuku-deps.list
                        ;;
                esac
            done
            apt-get update
            ;;
        rhel)
            # Process dnf repositories
            echo "$DEPS_JSON" | jq -c '.repositories // [] | .[]' | while read -r repo; do
                URL=$(echo "$repo" | jq -r '.url')
                KEY_URL=$(echo "$repo" | jq -r '.key_url // empty')
                KEY_SHA256=$(echo "$repo" | jq -r '.key_sha256 // empty')

                if [[ -n "$KEY_URL" ]]; then
                    # Verify HTTPS for key URL
                    verify_https "$KEY_URL" "GPG key URL"

                    if [[ -z "$KEY_SHA256" ]]; then
                        echo "Error: key_sha256 is required when key_url is provided" >&2
                        exit 1
                    fi

                    echo "Importing GPG key from: $KEY_URL"
                    KEY_FILE=$(verify_key_sha256 "$KEY_URL" "$KEY_SHA256")

                    # Import verified key
                    rpm --import "$KEY_FILE"
                    rm -f "$KEY_FILE"
                fi

                echo "Adding DNF repository: $URL"
                dnf config-manager --add-repo "$URL"
            done
            ;;
        *)
            echo "Warning: repository configuration not implemented for $FAMILY" >&2
            ;;
    esac
fi

# Install packages
if [[ -n "$PACKAGES" ]]; then
    echo "Installing packages for $RECIPE on $FAMILY: $PACKAGES"

    # shellcheck disable=SC2086
    case "$FAMILY" in
        alpine)
            apk add --no-cache $PACKAGES
            ;;
        debian)
            apt-get install -y --no-install-recommends $PACKAGES
            ;;
        rhel)
            dnf install -y --setopt=install_weak_deps=False $PACKAGES
            ;;
        arch)
            pacman -S --noconfirm $PACKAGES
            ;;
        suse)
            zypper -n install $PACKAGES
            ;;
    esac
fi
