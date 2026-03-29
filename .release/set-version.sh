#!/usr/bin/env bash
# Set-version hook called by the reusable release workflow.
# Receives the version without v prefix (e.g., 0.6.2 or 0.6.3-dev).
#
# Stamps version into both Cargo.toml files. Go binary version is injected
# via goreleaser ldflags from the git tag, so no stamping needed there.

set -euo pipefail

VERSION="${1:?Usage: set-version.sh <version>}"

sed -i "s/^version = \".*\"/version = \"${VERSION}\"/" cmd/tsuku-dltest/Cargo.toml
echo "Stamped cmd/tsuku-dltest/Cargo.toml to ${VERSION}"

sed -i "s/^version = \".*\"/version = \"${VERSION}\"/" tsuku-llm/Cargo.toml
echo "Stamped tsuku-llm/Cargo.toml to ${VERSION}"
