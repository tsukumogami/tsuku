#!/usr/bin/env bash
# requeue-unblocked.sh - Re-queue packages whose blockers have been resolved.
#
# Usage: ./scripts/requeue-unblocked.sh [--dry-run]
#
# Scans failure records for missing_dep entries, checks whether each
# blocked_by recipe now exists in the registry, and flips resolved
# queue entries from "blocked" to "pending".
#
# Exit codes:
#   0 - Success (or nothing to do)
#   1 - Missing dependencies (jq)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ECOSYSTEM="${ECOSYSTEM:-homebrew}"
QUEUE="$REPO_ROOT/data/queues/priority-queue-$ECOSYSTEM.json"
FAILURES_DIR="$REPO_ROOT/data/failures"
RECIPES_DIR="$REPO_ROOT/recipes"
EMBEDDED_DIR="$REPO_ROOT/internal/recipe/recipes"
DRY_RUN=false

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

if ! command -v jq &>/dev/null; then
  echo "error: jq is required" >&2
  exit 1
fi

if [[ ! -d "$FAILURES_DIR" ]]; then
  echo "No failures directory found, nothing to do."
  exit 0
fi

if [[ ! -f "$QUEUE" ]]; then
  echo "error: queue file not found: $QUEUE" >&2
  exit 1
fi

# recipe_exists checks if a recipe TOML file exists in the registry or
# embedded recipes directory.
recipe_exists() {
  local name="$1"
  local first="${name:0:1}"
  first="$(echo "$first" | tr '[:upper:]' '[:lower:]')"
  [[ -f "$RECIPES_DIR/$first/$name.toml" ]] || [[ -f "$EMBEDDED_DIR/$name.toml" ]]
}

# Collect all blocked_by entries from missing_dep failures across all JSONL files.
# Output: one line per (package_id, blocked_by_name) pair.
blocked_pairs() {
  for f in "$FAILURES_DIR"/*.jsonl; do
    [[ -f "$f" ]] || continue
    # Filter out null/empty lines, then process each JSON object
    # Handle both old format (.failures[]) and new format (one object per line)
    grep -v '^\s*null\s*$' "$f" | grep -v '^\s*$' | jq -r '
      # Handle old format with .failures array
      if has("failures") then
        .failures[]
        | select(.category == "missing_dep")
        | .package_id as $pid
        | .blocked_by[]
        | [$pid, .] | @tsv
      # Handle new format (one failure object per line)
      elif .category == "missing_dep" then
        .package_id as $pid
        | .blocked_by[]
        | [$pid, .] | @tsv
      else
        empty
      end
    ' 2>/dev/null || true
  done | sort -u
}

# Build a map: package_id -> list of unresolved blockers.
declare -A unresolved

while IFS=$'\t' read -r pkg_id blocker; do
  if ! recipe_exists "$blocker"; then
    unresolved[$pkg_id]="${unresolved[$pkg_id]:-}${unresolved[$pkg_id]:+ }$blocker"
  fi
done < <(blocked_pairs)

# Find blocked queue entries that have all blockers resolved (not in unresolved map).
requeued=0
blocked_ids=$(jq -r '.packages[] | select(.status == "blocked") | .id' "$QUEUE")

for pkg_id in $blocked_ids; do
  if [[ -z "${unresolved[$pkg_id]:-}" ]]; then
    if $DRY_RUN; then
      echo "[dry-run] Would requeue: $pkg_id"
    else
      # Use jq to flip status from blocked to pending.
      tmp=$(mktemp)
      jq --arg id "$pkg_id" '
        .packages |= map(if .id == $id then .status = "pending" else . end)
      ' "$QUEUE" > "$tmp" && mv "$tmp" "$QUEUE"
    fi
    requeued=$((requeued + 1))
  else
    echo "Still blocked: $pkg_id (waiting on: ${unresolved[$pkg_id]})"
  fi
done

if [[ $requeued -eq 0 ]]; then
  echo "No packages ready for requeue."
else
  if $DRY_RUN; then
    echo "$requeued package(s) would be requeued."
  else
    echo "$requeued package(s) requeued to pending."
  fi
fi
