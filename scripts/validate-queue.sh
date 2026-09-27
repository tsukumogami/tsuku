#!/usr/bin/env bash
# validate-queue.sh - Validate priority queue data against its JSON schema.
#
# Usage: ./scripts/validate-queue.sh [queue-file]
#
# Validates data/queues/priority-queue.json (or the given file) against
# data/schemas/priority-queue.schema.json.
# Exits 0 on success, 1 on validation failure, 2 on a missing schema,
# queue file or validator. A missing queue is an error, not a pass: a validator that
# cannot find its input has not validated anything.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SCHEMA="$REPO_ROOT/data/schemas/priority-queue.schema.json"
QUEUE="${1:-$REPO_ROOT/data/queues/priority-queue.json}"

if [[ ! -f "$SCHEMA" ]]; then
  echo "ERROR: Schema file not found: $SCHEMA" >&2
  exit 2
fi

if [[ ! -f "$QUEUE" ]]; then
  echo "ERROR: Queue file not found: $QUEUE" >&2
  exit 2
fi

# Distinguish "could not run the validator" from "the queue is invalid".
if ! pipx --version >/dev/null 2>&1; then
  echo "ERROR: pipx is required to run check-jsonschema but is not runnable" >&2
  exit 2
fi

echo "Validating $QUEUE against schema..."
if pipx run check-jsonschema --schemafile "$SCHEMA" "$QUEUE"; then
  echo "PASS: $QUEUE is valid"
else
  echo "FAIL: $QUEUE does not conform to schema" >&2
  exit 1
fi
