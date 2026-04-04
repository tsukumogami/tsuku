#!/usr/bin/env bash
# Set-version hook called by the reusable release workflow.
# Receives the version without v prefix (e.g., 0.6.2 or 0.6.3-dev).
#
# Stamps version into Cargo.toml files and Claude Code plugin JSON files.
# Go binary version is injected via goreleaser ldflags from the git tag,
# so no stamping needed there.

set -euo pipefail

VERSION="${1:?Usage: set-version.sh <version>}"

# Rust binaries
sed -i "s/^version = \".*\"/version = \"${VERSION}\"/" cmd/tsuku-dltest/Cargo.toml
echo "Stamped cmd/tsuku-dltest/Cargo.toml to ${VERSION}"

sed -i "s/^version = \".*\"/version = \"${VERSION}\"/" tsuku-llm/Cargo.toml
echo "Stamped tsuku-llm/Cargo.toml to ${VERSION}"

# Claude Code plugin files
MARKETPLACE_JSON=".claude-plugin/marketplace.json"
if [ -f "$MARKETPLACE_JSON" ]; then
  jq --arg v "$VERSION" '(.plugins[].version) = $v' "$MARKETPLACE_JSON" > "$MARKETPLACE_JSON.tmp" \
    && mv "$MARKETPLACE_JSON.tmp" "$MARKETPLACE_JSON"
  echo "Stamped $MARKETPLACE_JSON to ${VERSION}"
fi

for plugin_json in plugins/*/.claude-plugin/plugin.json; do
  [ -f "$plugin_json" ] || continue
  jq --arg v "$VERSION" '.version = $v' "$plugin_json" > "$plugin_json.tmp" \
    && mv "$plugin_json.tmp" "$plugin_json"
  echo "Stamped $plugin_json to ${VERSION}"
done
