#!/usr/bin/env bash
# validate-queue.sh - Validate priority queue data against its JSON schema.
#
# Usage: ./scripts/validate-queue.sh
#
# Validates data/priority-queue.json against data/schemas/priority-queue.schema.json.
# Exits 0 on success, 1 on validation failure, 2 on missing schema.
# If the queue file doesn't exist yet, prints a warning and exits 0.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SCHEMA="$REPO_ROOT/data/schemas/priority-queue.schema.json"
QUEUE="$REPO_ROOT/data/priority-queue.json"

if [[ ! -f "$SCHEMA" ]]; then
  echo "ERROR: Schema file not found: $SCHEMA" >&2
  exit 2
fi

if [[ ! -f "$QUEUE" ]]; then
  echo "WARN: Queue file not found: $QUEUE (skipping validation)" >&2
  exit 0
fi

echo "Validating $QUEUE against schema..."
if pipx run check-jsonschema --schemafile "$SCHEMA" "$QUEUE"; then
  echo "PASS: $QUEUE is valid"
else
  echo "FAIL: $QUEUE does not conform to schema" >&2
  exit 1
fi
