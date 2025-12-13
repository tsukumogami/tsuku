#!/bin/sh
# Installs the git hooks from the scripts/ directory.

HOOKS_DIR=$(git rev-parse --git-dir)/hooks
SCRIPTS_DIR=$(git rev-parse --show-toplevel)/scripts

ln -sfv "$SCRIPTS_DIR/pre-commit" "$HOOKS_DIR/pre-commit"

echo "Git hooks installed successfully."
