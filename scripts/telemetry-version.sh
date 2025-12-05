#!/usr/bin/env bash
# Fetch the deployed telemetry worker version
# Requires: gh CLI authenticated, TELEMETRY_VERSION_TOKEN set as repo variable

set -euo pipefail

REPO="tsukumogami/tsuku"
ENDPOINT="https://telemetry.tsuku.dev/version"

TOKEN=$(gh variable get TELEMETRY_VERSION_TOKEN --repo "$REPO" 2>/dev/null) || {
  echo "Error: TELEMETRY_VERSION_TOKEN not set as repository variable" >&2
  echo "Set it with: gh variable set TELEMETRY_VERSION_TOKEN --repo $REPO" >&2
  exit 1
}

curl -sf -H "Authorization: Bearer $TOKEN" "$ENDPOINT" | jq .
