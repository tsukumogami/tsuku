#!/usr/bin/env bash
#
# frontmatter.sh - Validate design document frontmatter
#
# Implements frontmatter validation rules from DESIGN-enhanced-validation-rules.md:
#   FM01: All 4 required fields present (status, problem, decision, rationale)
#   FM02: Status is valid value (Proposed, Accepted, Planned, Current, Superseded)
#   FM03: Frontmatter status matches body status section
#
# Usage:
#   frontmatter.sh <doc-path>
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Operational error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Constants
readonly REQUIRED_FIELDS=(status problem decision rationale)
readonly VALID_STATUSES=(Proposed Accepted Planned Current Superseded)

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

# Run checks
FAILED=0

# Check: Frontmatter exists (prerequisite for other checks)
if ! has_frontmatter "$DOC_PATH"; then
    emit_fail "Frontmatter: missing or malformed (must start with --- and have closing ---)"
    exit $EXIT_FAIL
fi

# Extract frontmatter content
FRONTMATTER=$(extract_frontmatter "$DOC_PATH")

# Helper: get field value from frontmatter
get_field() {
    local field="$1"
    echo "$FRONTMATTER" | awk -F': ' -v field="$field" '$1 == field { print substr($0, index($0, ": ") + 2) }'
}

# FM01: Check all required fields are present
for field in "${REQUIRED_FIELDS[@]}"; do
    value=$(get_field "$field")
    if [[ -z "$value" ]]; then
        emit_fail "Missing frontmatter field: $field. Required: status, problem, decision, rationale"
        FAILED=1
    fi
done

# FM02: Validate status value
FM_STATUS=$(get_field "status")
if [[ -n "$FM_STATUS" ]]; then
    valid=0
    for status in "${VALID_STATUSES[@]}"; do
        if [[ "$FM_STATUS" == "$status" ]]; then
            valid=1
            break
        fi
    done
    if [[ "$valid" -eq 0 ]]; then
        emit_fail "Invalid frontmatter status: $FM_STATUS. Must be: Proposed, Accepted, Planned, Current, Superseded"
        FAILED=1
    fi
fi

# FM03: Check frontmatter status matches body status
# Skip for Superseded docs (per design: "validation is relaxed to not block archival of legacy formats")
if [[ "$FM_STATUS" != "Superseded" ]]; then
    # Body status is in "## Status" section, formatted as **Status** (preferred) or just Status
    # Also handles inline **Status: Value** format
    extract_body_status() {
        local doc="$1"
        # Method 1: Look for ## Status section, then find status value
        local status
        status=$(awk '
            /^## Status/ { in_status = 1; next }
            in_status && /^\*\*[A-Za-z]+\*\*/ {
                # Bold format: **Proposed**
                gsub(/^\*\*/, "")
                gsub(/\*\*.*$/, "")
                print
                exit
            }
            in_status && /^[A-Za-z]+$/ {
                # Plain format: Proposed (on its own line)
                print
                exit
            }
            in_status && /^Superseded by/ {
                # Superseded with link format
                print "Superseded"
                exit
            }
            in_status && /^## / { exit }  # Hit next section
        ' "$doc")

        if [[ -n "$status" ]]; then
            echo "$status"
            return
        fi

        # Method 2: Look for inline status formats
        # Handles: **Status: Value** (all bold) or **Status:** Value (label bold only)
        awk '
            /^\*\*Status:\*\* [A-Za-z]+/ {
                # **Status:** Value format (label bold, value plain)
                gsub(/^\*\*Status:\*\* /, "")
                gsub(/ .*$/, "")
                print
                exit
            }
            /^\*\*Status: [A-Za-z]+\*\*/ {
                # **Status: Value** format (all bold)
                gsub(/^\*\*Status: /, "")
                gsub(/\*\*.*$/, "")
                print
                exit
            }
        ' "$doc"
    }

    BODY_STATUS=$(extract_body_status "$DOC_PATH")
    if [[ -z "$BODY_STATUS" ]]; then
        emit_fail "Could not extract body status from ## Status section (expected **Status** format)"
        FAILED=1
    elif [[ -n "$FM_STATUS" && "$FM_STATUS" != "$BODY_STATUS" ]]; then
        emit_fail "Frontmatter status '$FM_STATUS' doesn't match body status '$BODY_STATUS'"
        FAILED=1
    fi
fi

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
