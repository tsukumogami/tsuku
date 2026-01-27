#!/usr/bin/env bash
#
# extract-closing-issues.sh - Extract issue numbers that a PR will close on merge
#
# Parses PR body (and optionally commit messages) for GitHub closing keywords
# (Fixes, Closes, Resolves and variants) and extracts the referenced issue numbers.
#
# Usage:
#   extract-closing-issues.sh --pr <number>    # Fetch PR and extract closing issues
#   extract-closing-issues.sh --stdin          # Read text from stdin
#
# Output (JSON array):
#   [123, 456]
#
# Exit codes:
#   0 - Success (array may be empty)
#   1 - PR not found (--pr mode only)
#   2 - Operational error (missing argument, invalid usage)
#
# Supported closing keywords (case-insensitive):
#   close, closes, closed
#   fix, fixes, fixed
#   resolve, resolves, resolved
#
# Supported reference formats:
#   #N                                    - Issue number
#   https://github.com/owner/repo/issues/N - Full URL
#
# Example:
#   echo "Fixes #123 and closes #456" | ./extract-closing-issues.sh --stdin
#   # Output: [123, 456]
#
#   ./extract-closing-issues.sh --pr 42
#   # Output: [123, 456] (from PR body and commit messages)

set -euo pipefail

# Show usage
usage() {
    echo "Usage: extract-closing-issues.sh --pr <number> | --stdin" >&2
    echo "" >&2
    echo "Options:" >&2
    echo "  --pr <number>   Fetch PR body and commits via gh" >&2
    echo "  --stdin         Read text from stdin" >&2
    exit 2
}

# Extract closing issue numbers from text
# Outputs one issue number per line (for later deduplication)
extract_issues() {
    local text="$1"

    # Pattern for closing keywords (case-insensitive)
    # Matches: close, closes, closed, fix, fixes, fixed, resolve, resolves, resolved
    # Followed by whitespace and either #N or a GitHub URL

    # Extract #N format
    echo "$text" | grep -oiE '(close[sd]?|fix(es|ed)?|resolve[sd]?)\s+#[0-9]+' | grep -oE '[0-9]+' || true

    # Extract URL format: github.com/owner/repo/issues/N
    echo "$text" | grep -oiE '(close[sd]?|fix(es|ed)?|resolve[sd]?)\s+https?://github\.com/[^/]+/[^/]+/issues/[0-9]+' | grep -oE 'issues/[0-9]+' | grep -oE '[0-9]+' || true
}

# Main
if [[ $# -lt 1 ]]; then
    usage
fi

MODE=""
PR_NUMBER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --pr)
            MODE="pr"
            if [[ $# -lt 2 ]]; then
                echo "Error: --pr requires a PR number" >&2
                usage
            fi
            PR_NUMBER="$2"
            shift 2
            ;;
        --stdin)
            MODE="stdin"
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Error: unknown option: $1" >&2
            usage
            ;;
    esac
done

if [[ -z "$MODE" ]]; then
    usage
fi

TEXT=""

if [[ "$MODE" == "pr" ]]; then
    # Fetch PR body and commit messages via gh
    if ! command -v gh &> /dev/null; then
        echo "Error: gh CLI not found" >&2
        exit 2
    fi

    # Get PR body and commit messages as combined text
    PR_DATA=$(gh pr view "$PR_NUMBER" --json body,commits 2>/dev/null) || {
        echo "Error: PR #$PR_NUMBER not found" >&2
        exit 1
    }

    # Extract body and all commit message headlines/bodies, join with newlines
    TEXT=$(echo "$PR_DATA" | jq -r '([.body // ""] + [.commits[]? | .messageHeadline, .messageBody] | map(select(. != null))) | join("\n")')

elif [[ "$MODE" == "stdin" ]]; then
    TEXT=$(cat)
fi

# Extract issue numbers, deduplicate, sort, and output as JSON array
ISSUES=$(extract_issues "$TEXT" | sort -n | uniq)

if [[ -z "$ISSUES" ]]; then
    echo "[]"
else
    # Convert newline-separated numbers to JSON array
    echo "$ISSUES" | jq -R -s 'split("\n") | map(select(length > 0) | tonumber)'
fi
