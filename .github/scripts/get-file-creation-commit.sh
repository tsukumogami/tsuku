#!/usr/bin/env bash
#
# get-file-creation-commit.sh - Get the commit that first introduced a file
#
# Returns the commit hash and ISO date when a file was first added to git.
# Follows renames to find the true origin.
#
# Usage:
#   get-file-creation-commit.sh <file-path>
#
# Output (JSON):
#   {"commit": "abc123...", "date": "2025-01-15T10:30:00+00:00"}
#
# Exit codes:
#   0 - Success
#   1 - File not tracked by git
#   2 - Operational error (missing argument, file not found)
#
# Example usage in orchestrator:
#   CREATED=$(./get-file-creation-commit.sh "$file")
#   CREATED_DATE=$(echo "$CREATED" | jq -r '.date')
#   if [[ "$CREATED_DATE" < "2025-01-20" ]]; then
#     echo "Skipping check for grandfathered file"
#   fi

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Error: missing required argument <file-path>" >&2
    echo "Usage: get-file-creation-commit.sh <file-path>" >&2
    exit 2
fi

FILE_PATH="$1"

if [[ ! -e "$FILE_PATH" ]]; then
    echo "Error: file not found: $FILE_PATH" >&2
    exit 2
fi

# Get the first commit that introduced this file (follows renames)
# --diff-filter=A shows only commits where file was Added
# --follow traces through renames
# tail -1 gets the oldest (first) commit
FIRST_COMMIT=$(git log --follow --diff-filter=A --format=%H -- "$FILE_PATH" 2>/dev/null | tail -1)

if [[ -z "$FIRST_COMMIT" ]]; then
    echo "Error: file not tracked by git: $FILE_PATH" >&2
    exit 1
fi

# Get the commit date in ISO 8601 format
COMMIT_DATE=$(git show -s --format=%cI "$FIRST_COMMIT")

# Output as JSON for easy parsing
echo "{\"commit\": \"$FIRST_COMMIT\", \"date\": \"$COMMIT_DATE\"}"
