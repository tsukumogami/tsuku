#!/usr/bin/env bash
# gap-analysis.sh - Query failure data to find packages blocked by specific dependencies.
#
# Usage: ./scripts/gap-analysis.sh --blocked-by <dep> [--ecosystem <name>] [--environment <name>] [--data-dir <path>]
#
# Reads failure record JSON files and identifies packages whose failures
# list the given dependency in their blocked_by field. Results are sorted
# by package_id.
#
# Exit codes:
#   0 - Matches found
#   1 - No matches found
#   2 - Error (missing data directory, invalid arguments, missing dependencies)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="$REPO_ROOT/data/failures"
BLOCKED_BY=""
ECOSYSTEM=""
ENVIRONMENT=""

usage() {
  cat >&2 <<'USAGE'
Usage: gap-analysis.sh --blocked-by <dep> [OPTIONS]

Find packages blocked by a specific missing dependency.

Required:
  --blocked-by <dep>       Dependency name to search for

Options:
  --ecosystem <name>       Filter by ecosystem (e.g., homebrew)
  --environment <name>     Filter by environment (e.g., linux-glibc)
  --data-dir <path>        Path to failures directory (default: data/failures)
  --help                   Show this help message

Exit codes:
  0  Matches found
  1  No matches
  2  Error
USAGE
  exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --blocked-by)   BLOCKED_BY="$2"; shift 2 ;;
    --ecosystem)    ECOSYSTEM="$2"; shift 2 ;;
    --environment)  ENVIRONMENT="$2"; shift 2 ;;
    --data-dir)     DATA_DIR="$2"; shift 2 ;;
    --help|-h)      usage ;;
    *) echo "ERROR: Unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$BLOCKED_BY" ]]; then
  echo "ERROR: --blocked-by is required" >&2
  exit 2
fi

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required but not found" >&2
  exit 2
fi

if [[ ! -d "$DATA_DIR" ]]; then
  echo "ERROR: Data directory not found: $DATA_DIR" >&2
  exit 2
fi

shopt -s nullglob
FILES=("$DATA_DIR"/*.json)
shopt -u nullglob

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "No failure files found in $DATA_DIR" >&2
  exit 1
fi

# Build jq filter for ecosystem and environment
JQ_FILE_FILTER="true"
if [[ -n "$ECOSYSTEM" ]]; then
  JQ_FILE_FILTER="$JQ_FILE_FILTER and .ecosystem == \"$ECOSYSTEM\""
fi
if [[ -n "$ENVIRONMENT" ]]; then
  JQ_FILE_FILTER="$JQ_FILE_FILTER and .environment == \"$ENVIRONMENT\""
fi

MATCHES=""

for file in "${FILES[@]}"; do
  # Check if file-level filters match
  if ! jq -e "$JQ_FILE_FILTER" "$file" > /dev/null 2>&1; then
    continue
  fi

  # Find failures blocked by the specified dependency
  RESULT=$(jq -r \
    --arg dep "$BLOCKED_BY" \
    '.failures[]
     | select(.blocked_by != null)
     | select(.blocked_by | index($dep))
     | .package_id' \
    "$file" 2>/dev/null) || continue

  if [[ -n "$RESULT" ]]; then
    MATCHES="${MATCHES}${RESULT}"$'\n'
  fi
done

# Remove trailing newline and check for results
MATCHES=$(echo -n "$MATCHES" | sed '/^$/d' | sort)

if [[ -z "$MATCHES" ]]; then
  echo "No packages found blocked by '$BLOCKED_BY'" >&2
  exit 1
fi

echo "Packages blocked by '$BLOCKED_BY':"
echo "$MATCHES"
