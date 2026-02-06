#!/usr/bin/env bash
# Migrate monolithic priority-queue.json to per-ecosystem files.
#
# Usage: ./scripts/migrate-priority-queue.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OLD_QUEUE="$REPO_ROOT/data/priority-queue.json"
QUEUES_DIR="$REPO_ROOT/data/queues"

if [ ! -f "$OLD_QUEUE" ]; then
  echo "error: $OLD_QUEUE not found"
  exit 1
fi

# Create queues directory
mkdir -p "$QUEUES_DIR"

# Count packages before migration
BEFORE_COUNT=$(jq '.packages | length' "$OLD_QUEUE")
echo "Migrating $BEFORE_COUNT packages from $OLD_QUEUE"

# Extract unique ecosystem values
ECOSYSTEMS=$(jq -r '.packages[].source' "$OLD_QUEUE" | sort -u)

# Split packages by ecosystem
for ecosystem in $ECOSYSTEMS; do
  OUTPUT_FILE="$QUEUES_DIR/priority-queue-${ecosystem}.json"

  # Filter packages for this ecosystem
  jq --arg eco "$ecosystem" '{
    schema_version: .schema_version,
    updated_at: .updated_at,
    tiers: .tiers,
    packages: [.packages[] | select(.source == $eco)]
  }' "$OLD_QUEUE" > "$OUTPUT_FILE"

  COUNT=$(jq '.packages | length' "$OUTPUT_FILE")
  echo "  $OUTPUT_FILE: $COUNT packages"
done

# Verify total count matches
AFTER_COUNT=0
for queue_file in "$QUEUES_DIR"/priority-queue-*.json; do
  COUNT=$(jq '.packages | length' "$queue_file")
  AFTER_COUNT=$((AFTER_COUNT + COUNT))
done

echo ""
echo "Verification:"
echo "  Before: $BEFORE_COUNT packages"
echo "  After:  $AFTER_COUNT packages"

if [ "$BEFORE_COUNT" -eq "$AFTER_COUNT" ]; then
  echo "  ✓ Package count matches"
else
  echo "  ✗ Package count mismatch!"
  exit 1
fi

echo ""
echo "Migration complete. Per-ecosystem queues written to $QUEUES_DIR"
echo "Next steps:"
echo "  1. Review the split queue files"
echo "  2. Update cmd/batch-generate to use ecosystem-specific paths"
echo "  3. Update cmd/seed-queue to use ecosystem-specific paths"
echo "  4. Test ecosystem isolation"
echo "  5. Remove old data/priority-queue.json after confirming everything works"
