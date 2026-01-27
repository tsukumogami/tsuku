#!/usr/bin/env bash
#
# check-issue-status.sh - Verify issue has expected status in design doc Mermaid diagram
#
# Usage:
#   check-issue-status.sh <doc-path> <issue-num> <expected-status>
#
# Arguments:
#   doc-path        Path to design document
#   issue-num       Issue number (e.g., 384)
#   expected-status Expected class: done, ready, blocked, needsDesign
#
# Exit codes:
#   0 - Status matches expected
#   1 - Status mismatch
#   2 - Operational error (file not found, no diagram, invalid arguments)
#
# Example:
#   check-issue-status.sh docs/designs/DESIGN-foo.md 350 done

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Valid status values
VALID_STATUSES=("done" "ready" "blocked" "needsDesign")

# Validate arguments
if [[ $# -lt 3 ]]; then
    echo "Error: missing required arguments" >&2
    echo "Usage: check-issue-status.sh <doc-path> <issue-num> <expected-status>" >&2
    exit 2
fi

DOC_PATH="$1"
ISSUE_NUM="$2"
EXPECTED_STATUS="$3"

# Check file exists
if [[ ! -f "$DOC_PATH" ]]; then
    echo "Error: file not found: $DOC_PATH" >&2
    exit 2
fi

# Validate issue number is numeric
if ! [[ "$ISSUE_NUM" =~ ^[0-9]+$ ]]; then
    echo "Error: invalid issue number: $ISSUE_NUM" >&2
    exit 2
fi

# Validate expected status
VALID=0
for status in "${VALID_STATUSES[@]}"; do
    if [[ "$EXPECTED_STATUS" == "$status" ]]; then
        VALID=1
        break
    fi
done
if [[ $VALID -eq 0 ]]; then
    echo "Error: invalid status '$EXPECTED_STATUS'. Valid: ${VALID_STATUSES[*]}" >&2
    exit 2
fi

# Check for mermaid block
if ! grep -q '```mermaid' "$DOC_PATH"; then
    echo "Error: no Mermaid diagram found in $DOC_PATH" >&2
    exit 2
fi

# Extract the mermaid block content
MERMAID_CONTENT=$(awk '
    /^```mermaid/ { in_mermaid = 1; next }
    /^```/ && in_mermaid { in_mermaid = 0; next }
    in_mermaid { print }
' "$DOC_PATH")

# Build a map of node -> class from class assignments
# Handles both single and grouped: "class I100 done" and "class I100,I101,I102 blocked"
declare -A NODE_CLASSES

while IFS= read -r line; do
    if echo "$line" | grep -qE '^\s*class\s+'; then
        # Extract class name (last word on the line)
        CLASS_NAME=$(echo "$line" | awk '{print $NF}')
        # Extract nodes (everything between "class " and the class name)
        NODES_STR=$(echo "$line" | sed 's/^\s*class\s*//' | sed "s/\s*${CLASS_NAME}$//" | tr -d ' ')
        # Split by comma and assign
        IFS=',' read -ra NODE_LIST <<< "$NODES_STR"
        for node in "${NODE_LIST[@]}"; do
            [[ -n "$node" ]] && NODE_CLASSES[$node]="$CLASS_NAME"
        done
    fi
done <<< "$MERMAID_CONTENT"

# Look up the status for our issue
NODE_ID="I${ISSUE_NUM}"
ACTUAL_STATUS="${NODE_CLASSES[$NODE_ID]:-}"

if [[ -z "$ACTUAL_STATUS" ]]; then
    echo "Error: issue #$ISSUE_NUM (node $NODE_ID) not found in diagram" >&2
    exit 2
fi

# Compare
if [[ "$ACTUAL_STATUS" == "$EXPECTED_STATUS" ]]; then
    echo "Issue #$ISSUE_NUM: status '$ACTUAL_STATUS' matches expected '$EXPECTED_STATUS'"
    exit 0
else
    echo "Issue #$ISSUE_NUM: expected '$EXPECTED_STATUS', found '$ACTUAL_STATUS'" >&2
    exit 1
fi
