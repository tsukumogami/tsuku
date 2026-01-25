#!/usr/bin/env bash
#
# sections.sh - Validate design document required sections
#
# Implements section validation rules from DESIGN-enhanced-validation-rules.md:
#   SC01: All 9 required sections exist
#   SC02: Sections appear in correct order
#   SC03: Security Considerations section has meaningful content
#
# Usage:
#   sections.sh <doc-path>
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Operational error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Constants - required sections in order
readonly REQUIRED_SECTIONS=(
    "Status"
    "Context and Problem Statement"
    "Decision Drivers"
    "Considered Options"
    "Decision Outcome"
    "Solution Architecture"
    "Implementation Approach"
    "Security Considerations"
    "Consequences"
)

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

# Extract frontmatter status to determine if we should run
# Skip sections check for Superseded status (legacy format allowed)
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

FM_STATUS=$(get_frontmatter_status "$DOC_PATH")

# Skip validation for Superseded documents (legacy format allowed per design)
if [[ "$FM_STATUS" == "Superseded" ]]; then
    exit $EXIT_PASS
fi

# Extract all ## level headings from the document (after frontmatter)
# Returns: heading name and line number, tab-separated
# Skips headings inside fenced code blocks (``` ... ```)
extract_headings() {
    local doc="$1"
    awk '
        # Skip frontmatter
        NR == 1 && /^---$/ { in_frontmatter = 1; next }
        in_frontmatter && /^---$/ { in_frontmatter = 0; next }
        in_frontmatter { next }

        # Track fenced code blocks (``` or ~~~)
        /^```/ || /^~~~/ { in_code_block = !in_code_block; next }
        in_code_block { next }

        # Match ## headings (not ### or deeper)
        /^## / {
            # Remove the ## prefix and any trailing whitespace
            heading = $0
            sub(/^## /, "", heading)
            sub(/[[:space:]]*$/, "", heading)
            print NR "\t" heading
        }
    ' "$doc"
}

# Run checks
FAILED=0

# Extract headings with line numbers
HEADINGS=$(extract_headings "$DOC_PATH")

# SC01: Check all 9 required sections exist
# Build a set of found sections for O(1) lookup
declare -A FOUND_SECTIONS
declare -A SECTION_LINES
while IFS=$'\t' read -r line_num heading; do
    FOUND_SECTIONS["$heading"]=1
    SECTION_LINES["$heading"]=$line_num
done <<< "$HEADINGS"

MISSING_COUNT=0
MISSING_SECTIONS=()
for section in "${REQUIRED_SECTIONS[@]}"; do
    if [[ -z "${FOUND_SECTIONS[$section]:-}" ]]; then
        MISSING_SECTIONS+=("$section")
        MISSING_COUNT=$((MISSING_COUNT + 1))
    fi
done

if [[ "$MISSING_COUNT" -gt 0 ]]; then
    emit_fail "SC01: Missing $MISSING_COUNT of 9 required sections:"
    for section in "${MISSING_SECTIONS[@]}"; do
        echo "       - ## $section" >&2
    done
    FAILED=1
fi

# SC02: Validate sections appear in correct order
# Only check if all sections are present (otherwise SC01 already failed)
if [[ "$MISSING_COUNT" -eq 0 ]]; then
    ORDER_FAILED=0
    PREV_LINE=0
    PREV_SECTION=""

    for section in "${REQUIRED_SECTIONS[@]}"; do
        CURRENT_LINE=${SECTION_LINES[$section]}
        if [[ "$PREV_LINE" -gt 0 ]] && [[ "$CURRENT_LINE" -lt "$PREV_LINE" ]]; then
            emit_fail "SC02: Wrong section order - '## $section' (line $CURRENT_LINE) appears before '## $PREV_SECTION' (line $PREV_LINE)"
            FAILED=1
            ORDER_FAILED=1
        fi
        PREV_LINE=$CURRENT_LINE
        PREV_SECTION=$section
    done

fi

# SC03: Security Considerations section has meaningful content
# Extract content between "Security Considerations" and next ## heading
if [[ -n "${FOUND_SECTIONS["Security Considerations"]:-}" ]]; then
    SECURITY_CONTENT=$(awk '
        # Skip frontmatter
        NR == 1 && /^---$/ { in_frontmatter = 1; next }
        in_frontmatter && /^---$/ { in_frontmatter = 0; next }
        in_frontmatter { next }

        # Find Security Considerations section
        /^## Security Considerations/ { in_security = 1; next }

        # Stop at next ## heading
        in_security && /^## / { exit }

        # Collect content
        in_security { print }
    ' "$DOC_PATH")

    # Strip whitespace and check for meaningful content
    STRIPPED_CONTENT=$(echo "$SECURITY_CONTENT" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | tr -d '\n')

    # Check if empty or only N/A (case insensitive)
    if [[ -z "$STRIPPED_CONTENT" ]]; then
        emit_fail "SC03: Security Considerations section is empty (must address security dimensions)"
        FAILED=1
    elif [[ "${STRIPPED_CONTENT,,}" =~ ^n/?a\.?$ ]]; then
        emit_fail "SC03: Security Considerations section contains only 'N/A' (must provide analysis or justification)"
        FAILED=1
    fi
fi

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    # Print concise help with full template inline
    cat >&2 <<'TEMPLATE'

Fix: Add/reorder sections to match this structure:

    ## Status
    ## Context and Problem Statement
    ## Decision Drivers
    ## Considered Options
    ## Decision Outcome
    ## Solution Architecture
    ## Implementation Approach
    ## Security Considerations
    ## Consequences

Each section must be an H2 heading (## ) in the document body.
TEMPLATE
    exit $EXIT_FAIL
fi
