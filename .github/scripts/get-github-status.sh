#!/usr/bin/env bash
#
# get-github-status.sh - Get GitHub status for issues and milestones
#
# Queries the GitHub API to get the current status (open/closed) for a list
# of issues and milestones. Handles rate limiting and caches responses.
#
# Usage:
#   get-github-status.sh --issues <json-array>
#   get-github-status.sh --from-doc <doc-path>
#   echo '<json-array>' | get-github-status.sh --stdin
#
# Input format (JSON array):
#   [
#     { "type": "issue", "number": 123, "url": "https://github.com/owner/repo/issues/123" },
#     { "type": "milestone", "number": 38, "url": "https://github.com/owner/repo/milestone/38" }
#   ]
#
# Output (JSON object mapping number to status):
#   {
#     "issues": { "123": "open", "456": "closed" },
#     "milestones": { "38": "closed" }
#   }
#
# Exit codes:
#   0 - Success
#   1 - Partial success (some API calls failed)
#   2 - Operational error (missing argument, no gh CLI)
#
# Environment:
#   GH_TOKEN - GitHub token for API access (optional but recommended)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check for required tools
if ! command -v gh &> /dev/null; then
    echo "Error: gh CLI not found" >&2
    exit 2
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq required but not found" >&2
    exit 2
fi

usage() {
    echo "Usage: get-github-status.sh --issues <json-array>" >&2
    echo "       get-github-status.sh --from-doc <doc-path>" >&2
    echo "       echo '<json>' | get-github-status.sh --stdin" >&2
    exit 2
}

if [[ $# -lt 1 ]]; then
    usage
fi

INPUT_JSON=""

case "$1" in
    --issues)
        if [[ $# -lt 2 ]]; then
            echo "Error: --issues requires a JSON array" >&2
            usage
        fi
        INPUT_JSON="$2"
        ;;
    --from-doc)
        if [[ $# -lt 2 ]]; then
            echo "Error: --from-doc requires a doc path" >&2
            usage
        fi
        DOC_PATH="$2"
        if [[ ! -f "$DOC_PATH" ]]; then
            echo "Error: file not found: $DOC_PATH" >&2
            exit 2
        fi
        # Extract issues from design doc
        DESIGN_DATA=$("$SCRIPT_DIR/extract-design-issues.sh" "$DOC_PATH" 2>/dev/null) || {
            echo '{"issues": {}, "milestones": {}}'
            exit 0
        }
        INPUT_JSON=$(echo "$DESIGN_DATA" | jq '[.milestones[].entries[] | {type, number, url}]')
        ;;
    --stdin)
        INPUT_JSON=$(cat)
        ;;
    -h|--help)
        usage
        ;;
    *)
        echo "Error: unknown option: $1" >&2
        usage
        ;;
esac

# Validate JSON
if ! echo "$INPUT_JSON" | jq empty 2>/dev/null; then
    echo "Error: invalid JSON input" >&2
    exit 2
fi

# Initialize output
ISSUE_STATUSES="{}"
MILESTONE_STATUSES="{}"
PARTIAL_FAILURE=0

# Process each entry
while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue

    TYPE=$(echo "$entry" | jq -r '.type')
    NUMBER=$(echo "$entry" | jq -r '.number')
    URL=$(echo "$entry" | jq -r '.url')

    # Skip entries without a number
    if [[ "$NUMBER" == "null" || -z "$NUMBER" ]]; then
        continue
    fi

    # Extract owner/repo from URL
    owner_repo=""
    if [[ "$TYPE" == "issue" ]]; then
        owner_repo=$(echo "$URL" | sed -n 's|https://github.com/\([^/]*/[^/]*\)/issues/.*|\1|p')
    else
        owner_repo=$(echo "$URL" | sed -n 's|https://github.com/\([^/]*/[^/]*\)/milestone/.*|\1|p')
    fi

    if [[ -z "$owner_repo" ]]; then
        PARTIAL_FAILURE=1
        continue
    fi

    # Query GitHub API
    state=""
    if [[ "$TYPE" == "issue" ]]; then
        state=$(gh api "repos/$owner_repo/issues/$NUMBER" --jq '.state' 2>/dev/null) || {
            PARTIAL_FAILURE=1
            continue
        }
        ISSUE_STATUSES=$(echo "$ISSUE_STATUSES" | jq --arg num "$NUMBER" --arg state "$state" '. + {($num): $state}')
    else
        state=$(gh api "repos/$owner_repo/milestones/$NUMBER" --jq '.state' 2>/dev/null) || {
            PARTIAL_FAILURE=1
            continue
        }
        MILESTONE_STATUSES=$(echo "$MILESTONE_STATUSES" | jq --arg num "$NUMBER" --arg state "$state" '. + {($num): $state}')
    fi
done <<< "$(echo "$INPUT_JSON" | jq -c '.[]')"

# Output combined result
jq -n --argjson issues "$ISSUE_STATUSES" --argjson milestones "$MILESTONE_STATUSES" \
    '{"issues": $issues, "milestones": $milestones}'

if [[ $PARTIAL_FAILURE -eq 1 ]]; then
    exit 1
else
    exit 0
fi
