#!/usr/bin/env bash
#
# issue-status.sh - Validate strikethrough formatting matches GitHub issue status
#
# Validates that issues/milestones marked with strikethrough in the Implementation
# Issues table are actually closed in GitHub, and vice versa.
#
# Usage:
#   issue-status.sh <doc-path> [--pr <number>]
#
# Options:
#   --pr <number>   Consider issues that will be closed by this PR as "closed"
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Operational error
#
# Environment:
#   GH_TOKEN - GitHub token for API access (optional but recommended)
#
# Notes:
#   - Requires gh CLI and jq
#   - Graceful degradation: warns but doesn't fail if GitHub API unavailable

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Validate arguments
if [[ $# -lt 1 ]]; then
    echo "Error: missing required argument <doc-path>" >&2
    exit $EXIT_ERROR
fi

DOC_PATH="$1"
shift

PR_NUMBER=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --pr)
            if [[ $# -lt 2 ]]; then
                echo "Error: --pr requires a PR number" >&2
                exit $EXIT_ERROR
            fi
            PR_NUMBER="$2"
            shift 2
            ;;
        *)
            echo "Error: unknown option: $1" >&2
            exit $EXIT_ERROR
            ;;
    esac
done

# Check file exists
if [[ ! -f "$DOC_PATH" ]]; then
    echo "Error: file not found: $DOC_PATH" >&2
    exit $EXIT_ERROR
fi

# Check for required tools
if ! command -v gh &> /dev/null; then
    echo "Warning: gh CLI not found, skipping issue status validation" >&2
    exit $EXIT_PASS
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq required but not found" >&2
    exit $EXIT_ERROR
fi

# Get sibling scripts directory
CI_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Extract issues from design doc
DESIGN_ISSUES=$("$CI_DIR/extract-design-issues.sh" "$DOC_PATH" 2>/dev/null) || {
    # No Implementation Issues section - nothing to validate
    exit $EXIT_PASS
}

# Get list of issues that will be closed by the PR
CLOSING_ISSUES="[]"
if [[ -n "$PR_NUMBER" ]]; then
    CLOSING_ISSUES=$("$CI_DIR/extract-closing-issues.sh" --pr "$PR_NUMBER" 2>/dev/null) || CLOSING_ISSUES="[]"
fi

# Get GitHub status for all issues/milestones (batch query)
GITHUB_STATUS=$("$CI_DIR/get-github-status.sh" --from-doc "$DOC_PATH") || {
    echo "Error: could not query GitHub API" >&2
    exit $EXIT_FAIL
}

FAILED=0

# Process each entry in the design doc (flatten all milestones)
ENTRIES=$(echo "$DESIGN_ISSUES" | jq -c '.milestones[].entries[]')

while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue

    TYPE=$(echo "$entry" | jq -r '.type')
    NUMBER=$(echo "$entry" | jq -r '.number')
    TITLE=$(echo "$entry" | jq -r '.title')
    COMPLETED=$(echo "$entry" | jq -r '.completed')

    # Skip entries without a number (named milestones without ID)
    if [[ "$NUMBER" == "null" || -z "$NUMBER" ]]; then
        continue
    fi

    # Get actual status from cached GitHub data
    if [[ "$TYPE" == "issue" ]]; then
        ACTUAL_STATUS=$(echo "$GITHUB_STATUS" | jq -r ".issues[\"$NUMBER\"] // \"unknown\"")
    else
        ACTUAL_STATUS=$(echo "$GITHUB_STATUS" | jq -r ".milestones[\"$NUMBER\"] // \"unknown\"")
    fi

    # Skip if we couldn't get status
    if [[ "$ACTUAL_STATUS" == "unknown" ]]; then
        continue
    fi

    # Check if issue will be closed by this PR
    WILL_BE_CLOSED="false"
    if [[ "$TYPE" == "issue" && "$CLOSING_ISSUES" != "[]" ]]; then
        if echo "$CLOSING_ISSUES" | jq -e "index($NUMBER)" > /dev/null 2>&1; then
            WILL_BE_CLOSED="true"
        fi
    fi

    # Determine effective status (considering PR closures)
    EFFECTIVE_STATUS="$ACTUAL_STATUS"
    if [[ "$WILL_BE_CLOSED" == "true" ]]; then
        EFFECTIVE_STATUS="closed"
    fi

    # Validate: strikethrough should match closed status
    if [[ "$COMPLETED" == "true" ]]; then
        # Marked complete - should be closed
        if [[ "$EFFECTIVE_STATUS" != "closed" ]]; then
            emit_fail "IS01: #$NUMBER marked complete but is $ACTUAL_STATUS in GitHub. See: .github/scripts/docs/IS01.md"
            FAILED=1
        fi
    else
        # Not marked complete - should be open
        if [[ "$EFFECTIVE_STATUS" == "closed" ]]; then
            if [[ "$WILL_BE_CLOSED" == "true" ]]; then
                emit_fail "IS02: #$NUMBER will be closed by this PR but is not marked complete. See: .github/scripts/docs/IS02.md"
            else
                emit_fail "IS02: #$NUMBER is closed in GitHub but not marked complete. See: .github/scripts/docs/IS02.md"
            fi
            FAILED=1
        fi
    fi
done <<< "$ENTRIES"

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
