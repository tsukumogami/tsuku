#!/usr/bin/env bash
# One-time cleanup script for analyzing conflicting batch PRs
#
# This script analyzes the backlog of 16+ conflicting batch PRs that accumulated
# before PR coordination mechanisms were implemented. It classifies recipes as
# NEW (not in main), IDENTICAL (already merged), or MODIFIED (needs review).
#
# Usage: ./scripts/cleanup-conflicting-prs.sh [--output FILE]
#
# The script is READ-ONLY - it generates recommendations but does not close PRs.
# Human operator must review recommendations and manually close approved PRs.

set -euo pipefail

# Parse arguments
OUTPUT_FILE=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --output)
      OUTPUT_FILE="$2"
      shift 2
      ;;
    *)
      echo "Usage: $0 [--output FILE]"
      exit 1
      ;;
  esac
done

# Default output to timestamped file if not specified
if [ -z "$OUTPUT_FILE" ]; then
  OUTPUT_FILE="cleanup-analysis-$(date -u +%Y%m%d-%H%M%S).txt"
fi

echo "Batch PR Cleanup Analysis" | tee "$OUTPUT_FILE"
echo "Generated: $(date -u '+%Y-%m-%d %H:%M UTC')" | tee -a "$OUTPUT_FILE"
echo "" | tee -a "$OUTPUT_FILE"

# Query for open batch PRs
OPEN_PRS=$(gh pr list --state open --search "batch recipes in:title" --json number --jq '.[].number')

if [ -z "$OPEN_PRS" ]; then
  echo "No open batch PRs found matching pattern 'feat(recipes): add batch'" | tee -a "$OUTPUT_FILE"
  exit 0
fi

PR_COUNT=$(echo "$OPEN_PRS" | wc -l)
echo "Found $PR_COUNT open batch PR(s) to analyze" | tee -a "$OUTPUT_FILE"
echo "" | tee -a "$OUTPUT_FILE"

CLOSE_COUNT=0
RESCUE_COUNT=0
REVIEW_COUNT=0

for pr in $OPEN_PRS; do
  echo "Analyzing PR #$pr..." | tee -a "$OUTPUT_FILE"

  # Extract recipe paths from PR diff
  RECIPE_PATHS=$(gh pr diff "$pr" --name-only 2>/dev/null | grep '^recipes/' || true)

  if [ -z "$RECIPE_PATHS" ]; then
    echo "  WARNING: No recipe files found in diff" | tee -a "$OUTPUT_FILE"
    echo "  RECOMMENDATION: MANUAL REVIEW (unexpected PR structure)" | tee -a "$OUTPUT_FILE"
    echo "" | tee -a "$OUTPUT_FILE"
    REVIEW_COUNT=$((REVIEW_COUNT + 1))
    continue
  fi

  # Extract recipe names from paths (recipes/X/name.toml -> name)
  RECIPES=$(echo "$RECIPE_PATHS" | sed -n 's|^recipes/[^/]*/\([^/]*\)\.toml$|\1|p')

  NEW_RECIPES=()
  MODIFIED_RECIPES=()
  IDENTICAL_RECIPES=()

  for recipe in $RECIPES; do
    # Validate recipe name (alphanumeric, dash, underscore only)
    if ! echo "$recipe" | grep -qE '^[a-zA-Z0-9_-]+$'; then
      echo "  WARNING: Skipping invalid recipe name: $recipe" | tee -a "$OUTPUT_FILE"
      continue
    fi

    # Check if recipe exists in main
    RECIPE_FILE=$(find recipes -name "${recipe}.toml" 2>/dev/null | head -1 || true)

    if [ -z "$RECIPE_FILE" ]; then
      # New recipe not yet in main
      NEW_RECIPES+=("$recipe")
    else
      # Recipe exists - check if content differs
      # Get additions + deletions count for this specific recipe file
      PR_DIFF_STATS=$(gh pr view "$pr" --json files --jq ".files[] | select(.path | contains(\"${recipe}.toml\")) | .additions + .deletions" 2>/dev/null || echo "")

      if [ -z "$PR_DIFF_STATS" ]; then
        # Couldn't get diff stats, treat as modified for safety
        MODIFIED_RECIPES+=("$recipe")
      elif [ "$PR_DIFF_STATS" = "0" ]; then
        # Zero changes means identical content
        IDENTICAL_RECIPES+=("$recipe")
      else
        # Has changes
        MODIFIED_RECIPES+=("$recipe")
      fi
    fi
  done

  # Generate recommendation
  TOTAL_NEW=${#NEW_RECIPES[@]}
  TOTAL_MODIFIED=${#MODIFIED_RECIPES[@]}
  TOTAL_IDENTICAL=${#IDENTICAL_RECIPES[@]}
  TOTAL_UNIQUE=$((TOTAL_NEW + TOTAL_MODIFIED))

  echo "  New recipes: $TOTAL_NEW" | tee -a "$OUTPUT_FILE"
  if [ "$TOTAL_NEW" -gt 0 ]; then
    echo "    ${NEW_RECIPES[*]}" | tee -a "$OUTPUT_FILE"
  fi

  echo "  Identical recipes: $TOTAL_IDENTICAL" | tee -a "$OUTPUT_FILE"
  if [ "$TOTAL_IDENTICAL" -gt 0 ]; then
    echo "    ${IDENTICAL_RECIPES[*]}" | tee -a "$OUTPUT_FILE"
  fi

  echo "  Modified recipes: $TOTAL_MODIFIED" | tee -a "$OUTPUT_FILE"
  if [ "$TOTAL_MODIFIED" -gt 0 ]; then
    echo "    ${MODIFIED_RECIPES[*]}" | tee -a "$OUTPUT_FILE"
  fi

  # Determine recommendation
  if [ "$TOTAL_UNIQUE" -eq 0 ]; then
    echo "  RECOMMENDATION: CLOSE (all recipes identical to main)" | tee -a "$OUTPUT_FILE"
    CLOSE_COUNT=$((CLOSE_COUNT + 1))
  elif [ "$TOTAL_MODIFIED" -gt 0 ]; then
    echo "  RECOMMENDATION: MANUAL REVIEW (contains modified recipes - could be updates or conflicts)" | tee -a "$OUTPUT_FILE"
    REVIEW_COUNT=$((REVIEW_COUNT + 1))
  else
    echo "  RECOMMENDATION: RESCUE ($TOTAL_NEW unique new recipe(s) not yet in main)" | tee -a "$OUTPUT_FILE"
    RESCUE_COUNT=$((RESCUE_COUNT + 1))
  fi

  echo "" | tee -a "$OUTPUT_FILE"
done

echo "========================================" | tee -a "$OUTPUT_FILE"
echo "Summary" | tee -a "$OUTPUT_FILE"
echo "========================================" | tee -a "$OUTPUT_FILE"
echo "Total PRs analyzed: $PR_COUNT" | tee -a "$OUTPUT_FILE"
echo "  - Close recommended: $CLOSE_COUNT" | tee -a "$OUTPUT_FILE"
echo "  - Rescue recommended: $RESCUE_COUNT" | tee -a "$OUTPUT_FILE"
echo "  - Manual review needed: $REVIEW_COUNT" | tee -a "$OUTPUT_FILE"
echo "" | tee -a "$OUTPUT_FILE"
echo "Analysis saved to: $OUTPUT_FILE" | tee -a "$OUTPUT_FILE"
echo "" | tee -a "$OUTPUT_FILE"
echo "Next steps:" | tee -a "$OUTPUT_FILE"
echo "1. Review the recommendations above" | tee -a "$OUTPUT_FILE"
echo "2. For CLOSE: manually close PRs with comment explaining all recipes are already merged" | tee -a "$OUTPUT_FILE"
echo "3. For RESCUE: extract unique recipes and create consolidated PR" | tee -a "$OUTPUT_FILE"
echo "4. For MANUAL REVIEW: inspect modified recipes to determine if updates or conflicts" | tee -a "$OUTPUT_FILE"
