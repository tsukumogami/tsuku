#!/bin/bash
# scripts/rollback-batch.sh
# Usage: ./rollback-batch.sh 2026-01-28-001

set -euo pipefail

BATCH_ID="$1"

if [ -z "$BATCH_ID" ]; then
  echo "Usage: $0 <batch_id>"
  echo "Example: $0 2026-01-28-001"
  exit 1
fi

# Find all recipes from this batch
echo "Finding recipes from batch $BATCH_ID..."
files=$(git log --all --name-only --grep="batch_id: $BATCH_ID" --format="" |
        grep '^recipes/' | sort -u)

if [ -z "$files" ]; then
  echo "No recipes found for batch $BATCH_ID"
  exit 1
fi

echo "Found $(echo "$files" | wc -l) recipes to rollback:"
echo "$files"

# Create rollback branch
branch="rollback-batch-$BATCH_ID"
git checkout -b "$branch"

# Remove the files
echo "$files" | xargs git rm

# Commit
git commit -m "chore: rollback batch $BATCH_ID

batch_id: $BATCH_ID
rollback_reason: manual rollback request
"

echo ""
echo "Rollback branch created: $branch"
echo "Review changes with: git diff main...$branch"
echo "Create PR with: gh pr create --title 'Rollback batch $BATCH_ID'"
