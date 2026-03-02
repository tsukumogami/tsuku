#!/bin/bash
# Reconcile priority queue status with actual recipe state on disk.
#
# Finds all recipes in recipes/*/*.toml, matches them to entries in
# data/queues/priority-queue.json by name, and marks non-success entries
# as "success". This fixes drift caused by rapid merges overwhelming
# the update-queue-status workflow's concurrency model.
#
# Usage:
#   ./scripts/reconcile-queue-status.sh [--dry-run]
#
# Options:
#   --dry-run   Show what would change without modifying files

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

QUEUE_FILE="$REPO_ROOT/data/queues/priority-queue.json"
RECIPES_DIR="$REPO_ROOT/recipes"

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    echo "=== DRY RUN MODE ==="
    echo ""
fi

if [[ ! -f "$QUEUE_FILE" ]]; then
    echo "ERROR: Queue file not found at $QUEUE_FILE"
    exit 1
fi

echo "Scanning recipes on disk..."

# Collect all recipe names from disk
mapfile -t ALL_RECIPES < <(find "$RECIPES_DIR" -name '*.toml' -type f | xargs -I{} basename {} .toml | sort)

echo "Found ${#ALL_RECIPES[@]} recipes on disk"
echo ""

# Build lookup of current queue status (name -> status)
echo "Loading queue data..."
QUEUE_STATUS_FILE=$(mktemp)
trap "rm -f $QUEUE_STATUS_FILE" EXIT

jq -r '.entries[] | "\(.name)\t\(.status)"' "$QUEUE_FILE" > "$QUEUE_STATUS_FILE"

declare -A QUEUE_STATUS
while IFS=$'\t' read -r name status; do
    QUEUE_STATUS["$name"]="$status"
done < "$QUEUE_STATUS_FILE"

echo "Queue has ${#QUEUE_STATUS[@]} entries"
echo ""

# Categorize recipes
ALREADY_SUCCESS=()
TO_UPDATE=()
NOT_IN_QUEUE=()

for recipe in "${ALL_RECIPES[@]}"; do
    status="${QUEUE_STATUS[$recipe]:-}"

    if [[ -z "$status" ]]; then
        NOT_IN_QUEUE+=("$recipe")
    elif [[ "$status" == "success" ]]; then
        ALREADY_SUCCESS+=("$recipe")
    else
        TO_UPDATE+=("$recipe:$status")
    fi
done

echo "=== Recipe Status ==="
echo "Already success:   ${#ALREADY_SUCCESS[@]}"
echo "Will mark success: ${#TO_UPDATE[@]}"
echo "Not in queue:      ${#NOT_IN_QUEUE[@]}"
echo ""

if [[ ${#TO_UPDATE[@]} -gt 0 ]]; then
    echo "Entries to update:"
    for item in "${TO_UPDATE[@]}"; do
        recipe="${item%%:*}"
        old_status="${item##*:}"
        echo "  $recipe ($old_status -> success)"
    done
    echo ""
fi

if [[ ${#NOT_IN_QUEUE[@]} -gt 0 ]]; then
    echo "Recipes not in queue (no action taken):"
    for recipe in "${NOT_IN_QUEUE[@]}"; do
        echo "  $recipe"
    done
    echo ""
fi

if [[ ${#TO_UPDATE[@]} -eq 0 ]]; then
    echo "Nothing to update. Queue is in sync."
    exit 0
fi

if [[ "$DRY_RUN" == "true" ]]; then
    echo "=== DRY RUN - No changes made ==="
    exit 0
fi

echo "Applying changes..."

# Build the list of names to update
UPDATE_NAMES=$(printf '%s\n' "${TO_UPDATE[@]}" | cut -d: -f1 | jq -R -s -c 'split("\n") | map(select(length > 0))')

# Single jq call to update all matching entries
jq --argjson update_names "$UPDATE_NAMES" '
    .entries |= map(
        if (.name | IN($update_names[])) and .status != "success" then
            .status = "success"
        else
            .
        end
    )
' "$QUEUE_FILE" > "${QUEUE_FILE}.tmp"

# Validate before replacing
if jq empty "${QUEUE_FILE}.tmp" 2>/dev/null; then
    mv "${QUEUE_FILE}.tmp" "$QUEUE_FILE"
else
    echo "ERROR: Generated invalid JSON"
    rm -f "${QUEUE_FILE}.tmp"
    exit 1
fi

echo ""
echo "=== Summary ==="
echo "Recipes on disk:    ${#ALL_RECIPES[@]}"
echo "Already success:    ${#ALREADY_SUCCESS[@]}"
echo "Updated to success: ${#TO_UPDATE[@]}"
echo "Not in queue:       ${#NOT_IN_QUEUE[@]}"
echo ""
echo "Queue updated: $QUEUE_FILE"
echo ""
echo "Next step: build and run queue-maintain to requeue unblocked packages:"
echo "  go build -o queue-maintain ./cmd/queue-maintain && ./queue-maintain"
