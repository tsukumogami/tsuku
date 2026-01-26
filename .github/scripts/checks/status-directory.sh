#!/usr/bin/env bash
#
# status-directory.sh - Validate design document status-directory alignment
#
# Implements status-directory validation rules from DESIGN-enhanced-validation-rules.md:
#   SD01: File is in correct directory for its status
#   SD02: Superseded documents have "Superseded by" link
#
# Usage:
#   status-directory.sh <doc-path>
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Operational error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Validate arguments
if [[ $# -lt 1 ]]; then
    echo "Error: missing required argument <doc-path>" >&2
    exit $EXIT_ERROR
fi

DOC_PATH="$1"

# Check file exists
if [[ ! -f "$DOC_PATH" ]]; then
    echo "Error: file not found: $DOC_PATH" >&2
    exit $EXIT_ERROR
fi

# Extract status from frontmatter
get_frontmatter_status() {
    local doc="$1"
    if ! has_frontmatter "$doc"; then
        echo ""
        return
    fi
    local frontmatter
    frontmatter=$(extract_frontmatter "$doc")
    echo "$frontmatter" | awk -F': ' '$1 == "status" { print $2 }'
}

# Get expected directory for a given status
# Returns the directory suffix after docs/designs
get_expected_directory() {
    local status="$1"
    case "$status" in
        Proposed|Accepted|Planned)
            echo "docs/designs"
            ;;
        Current)
            echo "docs/designs/current"
            ;;
        Superseded)
            echo "docs/designs/archive"
            ;;
        *)
            echo ""
            ;;
    esac
}

# Extract actual directory from file path
# Normalizes to match expected directory format
get_actual_directory() {
    local path="$1"
    local dir
    dir=$(dirname "$path")

    # Remove leading ./ if present
    dir="${dir#./}"

    # Return the normalized directory
    echo "$dir"
}

FM_STATUS=$(get_frontmatter_status "$DOC_PATH")

# If no status found, can't validate - other checks will catch this
if [[ -z "$FM_STATUS" ]]; then
    exit $EXIT_PASS
fi

FAILED=0

# SD01: Validate file is in correct directory for its status
EXPECTED_DIR=$(get_expected_directory "$FM_STATUS")
ACTUAL_DIR=$(get_actual_directory "$DOC_PATH")

if [[ -n "$EXPECTED_DIR" ]]; then
    # Check if actual directory ends with expected pattern
    # This handles both relative (docs/designs/) and absolute (/path/to/docs/designs/) paths
    if [[ "$ACTUAL_DIR" != "$EXPECTED_DIR" && "$ACTUAL_DIR" != */"$EXPECTED_DIR" ]]; then
        emit_fail "SD01: Status '$FM_STATUS' requires directory '$EXPECTED_DIR', found in '$ACTUAL_DIR'"
        FAILED=1
    fi
fi

# SD02: Superseded documents must have "Superseded by" link
if [[ "$FM_STATUS" == "Superseded" ]]; then
    # Search for "Superseded by" followed by markdown link pattern
    # Pattern: Superseded by [anything](anything)
    if ! grep -qE "Superseded by \[.+\]\(.+\)" "$DOC_PATH"; then
        emit_fail "SD02: Superseded status requires 'Superseded by [DESIGN-...](path)' link in body"
        FAILED=1
    fi
fi

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
