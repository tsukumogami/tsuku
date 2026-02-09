#!/bin/bash
# Reconcile queue data with actual recipe state.
#
# This script:
# 1. Marks homebrew-sourced recipes as "success" in the queue
# 2. Updates missing_dep failures from "failed" to "blocked" status
#
# Usage:
#   ./scripts/reconcile-queue.sh [--dry-run]
#
# Options:
#   --dry-run   Show what would change without modifying files

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

QUEUE_FILE="$REPO_ROOT/data/queues/priority-queue-homebrew.json"
FAILURES_FILE="$REPO_ROOT/data/failures/homebrew.jsonl"
RECIPES_DIR="$REPO_ROOT/recipes"

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    echo "=== DRY RUN MODE ==="
    echo ""
fi

echo "Finding homebrew-sourced recipes..."

# Find all recipes that use homebrew action
mapfile -t HOMEBREW_RECIPES < <(grep -l 'action = "homebrew"' "$RECIPES_DIR"/*/*.toml 2>/dev/null | xargs -I{} basename {} .toml)

echo "Found ${#HOMEBREW_RECIPES[@]} homebrew-sourced recipes"
echo ""

# Build lookup of current queue status (name -> status)
echo "Loading queue data..."
QUEUE_STATUS_FILE=$(mktemp)
jq -r '.packages[] | "\(.name)\t\(.status)"' "$QUEUE_FILE" > "$QUEUE_STATUS_FILE"
trap "rm -f $QUEUE_STATUS_FILE" EXIT

# Build associative array for fast lookup
declare -A QUEUE_STATUS
while IFS=$'\t' read -r name status; do
    QUEUE_STATUS["$name"]="$status"
done < "$QUEUE_STATUS_FILE"

echo "Queue has ${#QUEUE_STATUS[@]} packages"
echo ""

# Categorize recipes
ALREADY_SUCCESS=()
TO_UPDATE=()
TO_ADD=()

for recipe in "${HOMEBREW_RECIPES[@]}"; do
    status="${QUEUE_STATUS[$recipe]:-}"

    if [[ -z "$status" ]]; then
        TO_ADD+=("$recipe")
    elif [[ "$status" == "success" ]]; then
        ALREADY_SUCCESS+=("$recipe")
    else
        TO_UPDATE+=("$recipe:$status")
    fi
done

echo "=== Recipe Status Changes ==="
echo "Already success: ${#ALREADY_SUCCESS[@]}"
echo "Will update to success: ${#TO_UPDATE[@]}"
echo "Will add to queue: ${#TO_ADD[@]}"
echo ""

if [[ ${#TO_UPDATE[@]} -gt 0 ]]; then
    echo "Recipes to update to success:"
    for item in "${TO_UPDATE[@]}"; do
        recipe="${item%%:*}"
        old_status="${item##*:}"
        echo "  - $recipe (was: $old_status)"
    done
    echo ""
fi

if [[ ${#TO_ADD[@]} -gt 0 ]]; then
    echo "Recipes to add (first 20):"
    count=0
    for recipe in "${TO_ADD[@]}"; do
        if [[ $count -lt 20 ]]; then
            echo "  - $recipe"
        fi
        count=$((count + 1))
    done
    if [[ ${#TO_ADD[@]} -gt 20 ]]; then
        echo "  ... and $((${#TO_ADD[@]} - 20)) more"
    fi
    echo ""
fi

# Find missing_dep failures that should be "blocked"
echo "=== Missing Dep Status Changes ==="

MISSING_DEP_TO_UPDATE=()
if [[ -f "$FAILURES_FILE" ]]; then
    while IFS= read -r pkg_name; do
        status="${QUEUE_STATUS[$pkg_name]:-}"
        if [[ "$status" == "failed" ]]; then
            MISSING_DEP_TO_UPDATE+=("$pkg_name")
            echo "  - $pkg_name: failed -> blocked"
        fi
    done < <(jq -r '.failures[]? | select(.category == "missing_dep") | .package_id | sub("^homebrew:"; "")' "$FAILURES_FILE" 2>/dev/null || true)
fi

echo ""
echo "Will update to blocked: ${#MISSING_DEP_TO_UPDATE[@]}"
echo ""

if [[ "$DRY_RUN" == "true" ]]; then
    echo "=== DRY RUN - No changes made ==="
    exit 0
fi

echo "Applying changes..."

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build jq filter for all updates at once
# This is much faster than running jq multiple times

# Create arrays for jq
UPDATE_NAMES=$(printf '%s\n' "${TO_UPDATE[@]:-}" | cut -d: -f1 | jq -R -s -c 'split("\n") | map(select(length > 0))')
ADD_NAMES=$(printf '%s\n' "${TO_ADD[@]:-}" | jq -R -s -c 'split("\n") | map(select(length > 0))')
BLOCKED_NAMES=$(printf '%s\n' "${MISSING_DEP_TO_UPDATE[@]:-}" | jq -R -s -c 'split("\n") | map(select(length > 0))')

# Apply all changes in one jq call
jq --argjson update_names "$UPDATE_NAMES" \
   --argjson add_names "$ADD_NAMES" \
   --argjson blocked_names "$BLOCKED_NAMES" \
   --arg ts "$TIMESTAMP" '
   # Update existing entries to success
   .packages |= map(
       if (.name | IN($update_names[])) then
           .status = "success"
       else
           .
       end
   ) |
   # Update missing_dep to blocked
   .packages |= map(
       if (.name | IN($blocked_names[])) and .status == "failed" then
           .status = "blocked"
       else
           .
       end
   ) |
   # Add new entries
   .packages += ($add_names | map({
       id: ("homebrew:" + .),
       source: "homebrew",
       name: .,
       tier: 2,
       status: "success",
       added_at: $ts
   })) |
   # Update timestamp
   .updated_at = $ts
' "$QUEUE_FILE" > "${QUEUE_FILE}.tmp"

# Validate and replace
if jq empty "${QUEUE_FILE}.tmp" 2>/dev/null; then
    mv "${QUEUE_FILE}.tmp" "$QUEUE_FILE"
else
    echo "ERROR: Generated invalid JSON"
    rm -f "${QUEUE_FILE}.tmp"
    exit 1
fi

echo ""
echo "=== Summary ==="
echo "Recipes found:        ${#HOMEBREW_RECIPES[@]}"
echo "Already success:      ${#ALREADY_SUCCESS[@]}"
echo "Updated to success:   ${#TO_UPDATE[@]}"
echo "Added to queue:       ${#TO_ADD[@]}"
echo "Updated to blocked:   ${#MISSING_DEP_TO_UPDATE[@]}"
echo ""
echo "Queue updated: $QUEUE_FILE"
