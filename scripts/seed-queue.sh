#!/usr/bin/env bash
# seed-queue.sh - Populate the priority queue with Homebrew formulas.
#
# Usage: ./scripts/seed-queue.sh --source homebrew [--limit N]
#
# Fetches formula names and download analytics from the Homebrew API,
# assigns tiers based on download counts and a curated list, then
# writes data/priority-queue.json conforming to the priority queue schema.
#
# Tiers:
#   1 - Curated high-impact developer tools (hardcoded list)
#   2 - Popular formulas (>10K installs per week, ~40K per 30 days)
#   3 - Everything else
#
# Exit codes:
#   0 - Success
#   1 - Invalid arguments or missing dependencies
#   2 - API fetch failure after retries

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT="$REPO_ROOT/data/priority-queue.json"

# Tier 1: Curated high-impact developer tools.
# These are well-known CLI tools that tsuku should support first.
TIER1_LIST=(
  ripgrep fd bat eza hyperfine tokei delta
  jq yq fzf
  gh git-lfs
  shellcheck shfmt
  cmake ninja meson
  go node python3 rust
  kubectl helm terraform
  htop btop tmux tree wget curl
  neovim vim
  sqlite
)

LIMIT=100
SOURCE=""
MAX_RETRIES=3

usage() {
  echo "Usage: $0 --source homebrew [--limit N]" >&2
  echo "" >&2
  echo "Options:" >&2
  echo "  --source SOURCE  Package source (only 'homebrew' supported)" >&2
  echo "  --limit N        Max formulas to include (default: 100)" >&2
  exit 1
}

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --source) SOURCE="$2"; shift 2 ;;
    --limit)  LIMIT="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "ERROR: Unknown argument: $1" >&2; usage ;;
  esac
done

if [[ -z "$SOURCE" ]]; then
  echo "ERROR: --source is required" >&2
  usage
fi

if [[ "$SOURCE" != "homebrew" ]]; then
  echo "ERROR: Only 'homebrew' source is supported" >&2
  exit 1
fi

# Check dependencies
for cmd in curl jq; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "ERROR: Required command not found: $cmd" >&2
    exit 1
  fi
done

# Fetch URL with retry and exponential backoff.
# Arguments: URL
# Outputs: response body on stdout
fetch_with_retry() {
  local url="$1"
  local attempt=0
  local delay=1

  while (( attempt < MAX_RETRIES )); do
    attempt=$((attempt + 1))
    local http_code
    local tmpfile
    tmpfile=$(mktemp)

    http_code=$(curl -s -o "$tmpfile" -w '%{http_code}' "$url") || true

    if [[ "$http_code" == "200" ]]; then
      cat "$tmpfile"
      rm -f "$tmpfile"
      return 0
    fi

    rm -f "$tmpfile"

    if [[ "$http_code" == "429" ]] || [[ "$http_code" =~ ^5 ]]; then
      echo "  Retry $attempt/$MAX_RETRIES (HTTP $http_code), waiting ${delay}s..." >&2
      sleep "$delay"
      delay=$((delay * 2))
    else
      echo "ERROR: HTTP $http_code fetching $url" >&2
      return 2
    fi
  done

  echo "ERROR: Failed after $MAX_RETRIES retries fetching $url" >&2
  return 2
}

echo "Fetching Homebrew analytics (install-on-request, 30d)..." >&2
ANALYTICS_JSON=$(fetch_with_retry "https://formulae.brew.sh/api/analytics/install-on-request/30d.json")

echo "Processing analytics data..." >&2

# Build the tier 1 list as a jq-friendly JSON array.
TIER1_JSON=$(printf '%s\n' "${TIER1_LIST[@]}" | jq -R . | jq -s .)

# The analytics API returns 30-day totals. The issue says tier 2 is >10K
# weekly downloads, so the 30-day threshold is ~40K (conservative: 4 weeks).
TIER2_THRESHOLD=40000

NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Process analytics items into priority queue packages.
# 1. Parse analytics items (formula name + count)
# 2. Remove commas from count strings, convert to integers
# 3. Sort by count descending
# 4. Apply limit
# 5. Assign tiers based on curation list and download count
# 6. Build conformant package objects
echo "Assigning tiers and building queue (limit: $LIMIT)..." >&2

echo "$ANALYTICS_JSON" | jq \
  --argjson tier1 "$TIER1_JSON" \
  --argjson threshold "$TIER2_THRESHOLD" \
  --argjson limit "$LIMIT" \
  --arg now "$NOW" \
  --arg source "homebrew" \
'
{
  schema_version: 1,
  updated_at: $now,
  tiers: {
    "1": "Critical - manually curated high-impact tools",
    "2": "Popular - >10K weekly downloads (>40K/30d)",
    "3": "Standard - all other packages"
  },
  packages: [
    .items[:$limit][] |
    {
      id: ($source + ":" + .formula),
      source: $source,
      name: .formula,
      tier: (
        if (.formula | IN($tier1[])) then 1
        elif ((.count | gsub(","; "") | tonumber) >= $threshold) then 2
        else 3
        end
      ),
      status: "pending",
      added_at: $now
    }
  ]
}
' > "$OUTPUT"

COUNT=$(jq '.packages | length' "$OUTPUT")
TIER1_COUNT=$(jq '[.packages[] | select(.tier == 1)] | length' "$OUTPUT")
TIER2_COUNT=$(jq '[.packages[] | select(.tier == 2)] | length' "$OUTPUT")
TIER3_COUNT=$(jq '[.packages[] | select(.tier == 3)] | length' "$OUTPUT")

echo "Wrote $COUNT packages to $OUTPUT" >&2
echo "  Tier 1 (curated): $TIER1_COUNT" >&2
echo "  Tier 2 (popular): $TIER2_COUNT" >&2
echo "  Tier 3 (standard): $TIER3_COUNT" >&2
echo "Done." >&2
