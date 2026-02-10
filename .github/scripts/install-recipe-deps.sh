#!/usr/bin/env bash
#
# install-recipe-deps.sh - Install system packages declared by a tsuku recipe
#
# Uses tsuku info to extract system package requirements for a target Linux
# family, then installs them using the appropriate package manager.
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

# Extract system packages from recipe
DEPS=$("$TSUKU" info --deps-only --system --family "$FAMILY" "$RECIPE")

# Exit cleanly if no packages needed
if [[ -z "$DEPS" ]]; then
    echo "No system packages required for $RECIPE on $FAMILY"
    exit 0
fi

echo "Installing packages for $RECIPE on $FAMILY: $DEPS"

# Install packages using family-appropriate package manager
# shellcheck disable=SC2086
case "$FAMILY" in
    alpine)
        apk add --no-cache $DEPS
        ;;
    debian)
        apt-get install -y --no-install-recommends $DEPS
        ;;
    rhel)
        dnf install -y --setopt=install_weak_deps=False $DEPS
        ;;
    arch)
        pacman -S --noconfirm $DEPS
        ;;
    suse)
        zypper -n install $DEPS
        ;;
esac
