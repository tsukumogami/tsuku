#!/usr/bin/env bash
# validate-queue-recipes.sh - Check that pending queue entries don't duplicate existing recipes.
#
# Usage: ./scripts/validate-queue-recipes.sh
#
# Cross-references pending entries in data/priority-queue.json against the
# recipes/ directory. Fails if any pending entry matches an existing recipe
# unless that entry has force_override set to true.
#
# Exit codes:
#   0 - No conflicts found
#   1 - One or more conflicts found
#   2 - Missing dependencies (jq, queue file)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

QUEUE="$REPO_ROOT/data/priority-queue.json"
RECIPES_DIR="$REPO_ROOT/recipes"

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required but not found" >&2
  exit 2
fi

if [[ ! -f "$QUEUE" ]]; then
  echo "WARN: Queue file not found: $QUEUE (skipping validation)" >&2
  exit 0
fi

CONFLICTS=0
OVERRIDES=0

# Extract pending entries: name and force_override for each
while IFS=$'\t' read -r name force_override; do
  # Construct recipe path: recipes/{first_letter}/{name}.toml
  first_letter="${name:0:1}"
  first_letter="${first_letter,,}" # lowercase
  recipe_path="$RECIPES_DIR/$first_letter/$name.toml"

  if [[ -f "$recipe_path" ]]; then
    if [[ "$force_override" == "true" ]]; then
      echo "OVERRIDE: $name"
      ((OVERRIDES++)) || true
    else
      echo "CONFLICT: $name -> $recipe_path"
      ((CONFLICTS++)) || true
    fi
  fi
done < <(jq -r '.packages[] | select(.status == "pending") | [.name, (.force_override // false | tostring)] | @tsv' "$QUEUE")

if [[ $OVERRIDES -gt 0 ]]; then
  echo "INFO: $OVERRIDES entries have force_override set"
fi

if [[ $CONFLICTS -gt 0 ]]; then
  echo "FAIL: $CONFLICTS pending entries conflict with existing recipes" >&2
  echo "Set force_override: true on these entries if overwrite is intentional" >&2
  exit 1
fi

echo "PASS: No conflicts between pending queue entries and existing recipes"
