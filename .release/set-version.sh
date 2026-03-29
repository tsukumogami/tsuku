#!/usr/bin/env bash
# Hook script for shirabe's reusable release workflow.
# Receives a version argument without the 'v' prefix (e.g., "0.6.2" or "0.6.3-dev").
# Stamps version into Cargo.toml files that need it.

set -euo pipefail

VERSION="${1:?Usage: set-version.sh <version>}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

FILES=(
  "cmd/tsuku-dltest/Cargo.toml"
  "tsuku-llm/Cargo.toml"
)

for file in "${FILES[@]}"; do
  target="${REPO_ROOT}/${file}"
  if [ ! -f "$target" ]; then
    echo "Warning: ${file} not found, skipping" >&2
    continue
  fi
  sed -i "s/^version = .*/version = \"${VERSION}\"/" "$target"
  echo "Stamped ${file} -> ${VERSION}"
done
