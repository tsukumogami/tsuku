#!/usr/bin/env bash
#
# strikethrough-all-docs.sh - Validate closing issues are marked complete in all design docs
#
# When a PR closes an issue, this check ensures that issue is marked as
# strikethrough (completed) in ALL design documents that reference it.
#
# Usage:
#   strikethrough-all-docs.sh --pr <number>
#
# Exit codes:
#   0 - All checks passed (or no issues to validate)
#   1 - One or more issues not marked complete in design docs
#   2 - Operational error
#
# Example:
#   # PR #408 has "Fixes #404" in body
#   ./strikethrough-all-docs.sh --pr 408
#   # Checks that #404 is strikethrough in all DESIGN-*.md files that reference it

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

CI_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Validate arguments
if [[ $# -lt 2 ]] || [[ "$1" != "--pr" ]]; then
    echo "Usage: strikethrough-all-docs.sh --pr <number>" >&2
    exit $EXIT_ERROR
fi

PR_NUMBER="$2"

# Check for required tools
if ! command -v gh &> /dev/null; then
    echo "Warning: gh CLI not found, skipping cross-doc strikethrough validation" >&2
    exit $EXIT_PASS
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq required but not found" >&2
    exit $EXIT_ERROR
fi

# Get issues being closed by this PR
CLOSING_ISSUES=$("$CI_DIR/extract-closing-issues.sh" --pr "$PR_NUMBER" 2>/dev/null) || {
    echo "Warning: could not extract closing issues from PR #$PR_NUMBER" >&2
    exit $EXIT_PASS
}

# If no issues being closed, nothing to validate
if [[ "$CLOSING_ISSUES" == "[]" ]] || [[ -z "$CLOSING_ISSUES" ]]; then
    exit $EXIT_PASS
fi

# Find all design docs (exclude archive - superseded docs don't need updates)
DESIGN_DOCS=$(find docs/designs -name 'DESIGN-*.md' -type f ! -path '*/archive/*' 2>/dev/null || true)

if [[ -z "$DESIGN_DOCS" ]]; then
    # No design docs found
    exit $EXIT_PASS
fi

FAILED=0
FAILURES=()

# For each issue being closed
while IFS= read -r issue_num; do
    [[ -z "$issue_num" ]] && continue

    # Check each design doc
    while IFS= read -r doc_path; do
        [[ -z "$doc_path" ]] && continue

        # Extract issues from this doc
        DOC_ISSUES=$("$CI_DIR/extract-design-issues.sh" "$doc_path" 2>/dev/null) || continue

        # Check if this issue is referenced in the doc
        # Look for entry with matching number that is NOT completed
        INCOMPLETE_MATCH=$(echo "$DOC_ISSUES" | jq -r --argjson num "$issue_num" '
            .milestones[].entries[]
            | select(.type == "issue" and .number == $num and .completed == false)
            | .number
        ' 2>/dev/null || true)

        if [[ -n "$INCOMPLETE_MATCH" ]]; then
            FAILURES+=("Issue #$issue_num will be closed but not marked complete in $doc_path")
            FAILED=1
        fi
    done <<< "$DESIGN_DOCS"
done <<< "$(echo "$CLOSING_ISSUES" | jq -r '.[]')"

# Report results
if [[ "$FAILED" -eq 1 ]]; then
    emit_fail "Closing issues not marked complete in all design docs:"
    for failure in "${FAILURES[@]}"; do
        echo "       - $failure" >&2
    done
    echo "" >&2
    echo "Fix: Add strikethrough to mark issues complete: ~~[#N](url)~~" >&2
    exit $EXIT_FAIL
fi

exit $EXIT_PASS
