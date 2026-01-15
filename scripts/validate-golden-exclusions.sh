#!/usr/bin/env bash
# Validate golden file exclusions
# Usage: ./scripts/validate-golden-exclusions.sh [--check-issues]
#
# Validates that:
# 1. Exclusions file is valid JSON
# 2. Each exclusion links to a valid GitHub issue
# 3. With --check-issues: Each linked issue is still OPEN (stale exclusions fail)
#
# Exit codes:
#   0: All exclusions valid
#   1: Stale exclusion found (linked issue is closed)
#   2: Invalid exclusion file or missing issue

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXCLUSIONS_FILE="$REPO_ROOT/testdata/golden/exclusions.json"

CHECK_ISSUES=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --check-issues) CHECK_ISSUES=true; shift ;;
        -h|--help)
            echo "Usage: $0 [--check-issues]"
            echo ""
            echo "Validate golden file exclusions."
            echo ""
            echo "Options:"
            echo "  --check-issues  Verify linked issues are still open (requires GITHUB_TOKEN)"
            exit 0
            ;;
        *) echo "Unknown flag: $1" >&2; exit 2 ;;
    esac
done

# Check exclusions file exists
if [[ ! -f "$EXCLUSIONS_FILE" ]]; then
    echo "Exclusions file not found: $EXCLUSIONS_FILE" >&2
    exit 2
fi

# Validate JSON
if ! jq empty "$EXCLUSIONS_FILE" 2>/dev/null; then
    echo "Invalid JSON in exclusions file" >&2
    exit 2
fi

# Get exclusion count
EXCLUSION_COUNT=$(jq '.exclusions | length' "$EXCLUSIONS_FILE")
echo "Found $EXCLUSION_COUNT exclusion(s)"

if [[ "$EXCLUSION_COUNT" -eq 0 ]]; then
    echo "No exclusions to validate"
    exit 0
fi

# Validate each exclusion
STALE_COUNT=0
INVALID_COUNT=0

while IFS= read -r exclusion; do
    recipe=$(echo "$exclusion" | jq -r '.recipe')
    os=$(echo "$exclusion" | jq -r '.platform.os')
    arch=$(echo "$exclusion" | jq -r '.platform.arch')
    family=$(echo "$exclusion" | jq -r '.platform.family // empty')
    issue_url=$(echo "$exclusion" | jq -r '.issue')
    reason=$(echo "$exclusion" | jq -r '.reason')

    # Build platform string for display
    if [[ -n "$family" ]]; then
        platform_str="$os-$family-$arch"
    else
        platform_str="$os-$arch"
    fi

    echo ""
    echo "Checking: $recipe ($platform_str)"
    echo "  Issue: $issue_url"
    echo "  Reason: $reason"

    # Validate issue URL format
    if ! [[ "$issue_url" =~ ^https://github.com/([^/]+)/([^/]+)/issues/([0-9]+)$ ]]; then
        echo "  ERROR: Invalid issue URL format" >&2
        INVALID_COUNT=$((INVALID_COUNT + 1))
        continue
    fi

    owner="${BASH_REMATCH[1]}"
    repo="${BASH_REMATCH[2]}"
    issue_number="${BASH_REMATCH[3]}"

    # Check issue status if requested
    if [[ "$CHECK_ISSUES" == "true" ]]; then
        if [[ -z "${GITHUB_TOKEN:-}" ]]; then
            echo "  WARNING: GITHUB_TOKEN not set, skipping issue status check"
            continue
        fi

        # Query issue state via GitHub API
        issue_state=$(gh api "repos/$owner/$repo/issues/$issue_number" --jq '.state' 2>/dev/null || echo "error")

        if [[ "$issue_state" == "error" ]]; then
            echo "  ERROR: Could not fetch issue status" >&2
            INVALID_COUNT=$((INVALID_COUNT + 1))
        elif [[ "$issue_state" == "open" ]]; then
            echo "  OK: Issue is open"
        else
            echo "  STALE: Issue is $issue_state - exclusion should be removed" >&2
            STALE_COUNT=$((STALE_COUNT + 1))
        fi
    fi
done < <(jq -c '.exclusions[]' "$EXCLUSIONS_FILE")

echo ""
echo "========================================"

if [[ "$INVALID_COUNT" -gt 0 ]]; then
    echo "ERROR: $INVALID_COUNT invalid exclusion(s) found" >&2
    exit 2
fi

if [[ "$STALE_COUNT" -gt 0 ]]; then
    echo "ERROR: $STALE_COUNT stale exclusion(s) found" >&2
    echo ""
    echo "Stale exclusions link to closed issues. Either:"
    echo "  1. Generate golden files for the recipe/platform"
    echo "  2. Update the exclusion with a new blocking issue"
    exit 1
fi

echo "All exclusions valid"
exit 0
