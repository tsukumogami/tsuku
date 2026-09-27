#!/usr/bin/env bash
# validate-queue-recipes.sh - Check that pending queue entries don't duplicate existing recipes.
#
# Usage: ./scripts/validate-queue-recipes.sh [queue-file]
#
# Cross-references pending entries in data/queues/priority-queue.json (or the
# given file) against the recipes/ directory. Fails if any pending entry's
# name matches an existing recipe file. Name matching only: an entry whose
# name differs from the recipe that already covers it is not detected here.
#
# Exit codes:
#   0 - No conflicts found
#   1 - One or more conflicts found
#   2 - Missing dependencies (jq, queue file) or unreadable queue

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

QUEUE="${1:-$REPO_ROOT/data/queues/priority-queue.json}"
RECIPES_DIR="$REPO_ROOT/recipes"

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required but not found" >&2
  exit 2
fi

# A missing queue is an error, not a pass: a validator that cannot find its
# input has not validated anything.
if [[ ! -f "$QUEUE" ]]; then
  echo "ERROR: Queue file not found: $QUEUE" >&2
  exit 2
fi

if ! jq -e '.entries | type == "array"' "$QUEUE" >/dev/null 2>&1; then
  echo "ERROR: $QUEUE is not valid JSON with an .entries array" >&2
  exit 2
fi

# Read the pending names up front (not in a process substitution) so a jq
# failure stops the script under set -e rather than reading as zero entries.
PENDING=$(jq -r '.entries[] | select(.status == "pending") | .name' "$QUEUE")

CONFLICTS=0
CHECKED=0

while IFS= read -r name; do
  [[ -z "$name" ]] && continue
  ((CHECKED++)) || true
  # Construct recipe path: recipes/{first_letter}/{name}.toml
  first_letter="${name:0:1}"
  first_letter="${first_letter,,}" # lowercase
  recipe_path="$RECIPES_DIR/$first_letter/$name.toml"

  if [[ -f "$recipe_path" ]]; then
    echo "CONFLICT: $name -> $recipe_path"
    ((CONFLICTS++)) || true
  fi
done <<<"$PENDING"

if [[ $CONFLICTS -gt 0 ]]; then
  echo "FAIL: $CONFLICTS of $CHECKED pending entries conflict with existing recipes" >&2
  echo "Mark the entry success or excluded, or remove it, if the recipe already covers it" >&2
  exit 1
fi

echo "PASS: $CHECKED pending entries checked, none conflict with existing recipes"
