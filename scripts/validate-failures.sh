#!/usr/bin/env bash
# validate-failures.sh - Validate failure record files against their JSON schema.
#
# Usage: ./scripts/validate-failures.sh
#
# Validates all data/failures/*.json files against data/schemas/failure-record.schema.json.
# Exits 0 on success, 1 on validation failure, 2 on missing schema.
# If no failure files exist yet, prints a warning and exits 0.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SCHEMA="$REPO_ROOT/data/schemas/failure-record.schema.json"
FAILURES_DIR="$REPO_ROOT/data/failures"

if [[ ! -f "$SCHEMA" ]]; then
  echo "ERROR: Schema file not found: $SCHEMA" >&2
  exit 2
fi

if [[ ! -d "$FAILURES_DIR" ]]; then
  echo "WARN: Failures directory not found: $FAILURES_DIR (skipping validation)" >&2
  exit 0
fi

shopt -s nullglob
FILES=("$FAILURES_DIR"/*.json)
shopt -u nullglob

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "WARN: No failure files found in $FAILURES_DIR (skipping validation)" >&2
  exit 0
fi

FAILED=0

for file in "${FILES[@]}"; do
  echo "Validating $file against schema..."
  if pipx run check-jsonschema --schemafile "$SCHEMA" "$file"; then
    echo "PASS: $file is valid"
  else
    echo "FAIL: $file does not conform to schema" >&2
    FAILED=1
  fi
done

if [[ $FAILED -eq 1 ]]; then
  echo "FAIL: One or more failure files are invalid" >&2
  exit 1
fi

echo "PASS: All ${#FILES[@]} failure file(s) are valid"
