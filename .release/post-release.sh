#!/usr/bin/env bash
# Post-release hook called by shirabe's finalize-release workflow.
# Generates a unified checksums.txt covering all release binaries
# (Go, Rust, LLM) and uploads it to the release, replacing goreleaser's
# partial checksums that only cover Go binaries.
#
# Receives a version argument without the 'v' prefix (e.g., "0.6.2").

set -euo pipefail

VERSION="${1:?Usage: post-release.sh <version>}"
TAG="v${VERSION}"
REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"

mkdir -p ./artifacts
cd ./artifacts

# Download all binary artifacts (skip goreleaser's partial checksums.txt)
gh release download "$TAG" \
  --repo "$REPO" \
  --pattern "tsuku-*" \
  --skip-existing

# Generate unified checksums for all binaries
sha256sum tsuku-* > checksums.txt
echo "Generated checksums.txt:"
cat checksums.txt

# Upload unified checksums, replacing goreleaser's partial file
gh release upload "$TAG" --repo "$REPO" checksums.txt --clobber
echo "Uploaded unified checksums.txt to $TAG"
