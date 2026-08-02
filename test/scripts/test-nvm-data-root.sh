#!/bin/bash
# Test that nvm's data root survives an nvm upgrade.
#
# nvm keeps everything it manages on the user's behalf under $NVM_DIR: installed Node
# versions, their global npm packages, and the alias/ tree that decides which version a
# new shell gets. tsuku points NVM_DIR at a stable per-tool data directory rather than at
# the versioned tool directory it reclaims, so that upgrading nvm does not take the
# user's Node installs with it.
#
# This installs a real nvm, installs a real Node version through it, sets a default
# alias, upgrades nvm to a newer release, and then checks in a fresh shell that all of it
# survived.
#
# `nvm exec` is checked explicitly. It is the one subcommand that breaks when the program
# files are missing from the data root, and it breaks quietly -- install, ls, use, which
# and alias all keep working -- so a test that stops at `nvm ls` would pass against a
# broken install.
#
# Usage: ./test/scripts/test-nvm-data-root.sh
#
# Exit codes:
#   0 - nvm data survived the upgrade
#   1 - Test failed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

NODE_VERSION="${NODE_VERSION:-22}"

cd "$REPO_ROOT"

echo "=== Testing that nvm's data root survives an nvm upgrade ==="
echo ""

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

export TSUKU_HOME="$WORK_DIR/tsuku"
export TSUKU_TELEMETRY=0

echo "Building tsuku..."
CGO_ENABLED=0 go build -o "$WORK_DIR/tsuku" ./cmd/tsuku
TSUKU="$WORK_DIR/tsuku"

# Two real releases, so the upgrade swaps genuinely different program files.
OLD_NVM="0.40.1"
NEW_NVM="0.40.3"

echo "Installing nvm $OLD_NVM..."
"$TSUKU" install "nvm@$OLD_NVM"

# Everything below runs through a login-shell-shaped subshell: source the env file the
# way a user's shell does, then drive nvm. Each block is its own shell, so nothing leaks
# between steps and every assertion is about what a *new* shell sees.
run_in_shell() {
  bash --norc --noprofile -c "set -euo pipefail; . \"\$TSUKU_HOME/env\"; $1"
}

echo "Installing Node $NODE_VERSION through nvm and setting it as the default..."
run_in_shell "nvm install $NODE_VERSION && nvm alias default $NODE_VERSION"

NODE_INSTALLED="$(run_in_shell "nvm ls --no-colors | tr -d ' *' | grep -o 'v[0-9.]*' | head -1")"
if [ -z "$NODE_INSTALLED" ]; then
  echo "ERROR: no Node version was installed before the upgrade"
  exit 1
fi
echo "Installed Node: $NODE_INSTALLED"

DATA_ROOT="$(run_in_shell 'printf %s "$NVM_DIR"')"
echo "NVM_DIR before upgrade: $DATA_ROOT"
case "$DATA_ROOT" in
  "$TSUKU_HOME"/tools/*)
    echo "ERROR: NVM_DIR points inside the versioned tool directory, which tsuku reclaims"
    exit 1
    ;;
esac

echo ""
echo "Upgrading nvm to $NEW_NVM..."
"$TSUKU" install "nvm@$NEW_NVM"

# Removing the superseded version is what actually deletes the old tool directory, so
# the assertions below are only meaningful once it has happened.
echo "Removing the superseded nvm $OLD_NVM..."
"$TSUKU" remove "nvm@$OLD_NVM" --force 2>/dev/null || "$TSUKU" remove "nvm@$OLD_NVM"

echo ""
echo "=== Checking a fresh shell after the upgrade ==="

DATA_ROOT_AFTER="$(run_in_shell 'printf %s "$NVM_DIR"')"
echo "NVM_DIR after upgrade: $DATA_ROOT_AFTER"
if [ "$DATA_ROOT_AFTER" != "$DATA_ROOT" ]; then
  echo "ERROR: NVM_DIR moved across the upgrade: $DATA_ROOT -> $DATA_ROOT_AFTER"
  exit 1
fi

echo "Checking nvm ls still lists $NODE_INSTALLED..."
if ! run_in_shell "nvm ls --no-colors" | grep -q "$NODE_INSTALLED"; then
  echo "ERROR: nvm ls no longer lists $NODE_INSTALLED after the upgrade"
  run_in_shell "nvm ls --no-colors" || true
  exit 1
fi

echo "Checking the default alias survived..."
if ! run_in_shell "nvm alias --no-colors" | grep -q "default"; then
  echo "ERROR: the default alias did not survive the upgrade"
  run_in_shell "nvm alias --no-colors" || true
  exit 1
fi

echo "Checking node still runs..."
NODE_REPORTED="$(run_in_shell "nvm use default >/dev/null 2>&1; node --version")"
if [ "$NODE_REPORTED" != "$NODE_INSTALLED" ]; then
  echo "ERROR: node --version reported $NODE_REPORTED, want $NODE_INSTALLED"
  exit 1
fi

# The quiet one. Needs both nvm.sh and nvm-exec present in the data root.
echo "Checking nvm exec still works..."
EXEC_REPORTED="$(run_in_shell "nvm exec --silent $NODE_INSTALLED node --version")"
if [ "$EXEC_REPORTED" != "$NODE_INSTALLED" ]; then
  echo "ERROR: nvm exec reported '$EXEC_REPORTED', want $NODE_INSTALLED"
  exit 1
fi

echo "Checking nvm reports the new program version..."
NVM_REPORTED="$(run_in_shell "nvm --version")"
if [ "$NVM_REPORTED" != "$NEW_NVM" ]; then
  echo "ERROR: nvm --version reported $NVM_REPORTED, want $NEW_NVM"
  exit 1
fi

echo ""
echo "=== All nvm data root tests passed ==="
echo "  Node $NODE_INSTALLED survived the nvm $OLD_NVM -> $NEW_NVM upgrade"
echo "  default alias survived"
echo "  nvm exec and node both work"
