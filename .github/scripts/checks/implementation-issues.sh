#!/usr/bin/env bash
#
# implementation-issues.sh - Validate design document Implementation Issues section
#
# Implements implementation issues validation rules:
#   II00: Section must NOT exist for Proposed/Accepted status
#   II01: Section exists for Planned status (optional for Current)
#   II02: Table has correct columns (Issue, Title, Dependencies, Tier)
#   II03: Issue/milestone links use valid format [#N](url) or [Name](milestone-url)
#   II04: Dependencies use link format, not plain text
#   II05: Tier values are valid (simple, testable, critical, milestone)
#
# Usage:
#   implementation-issues.sh <doc-path>
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

# Get normalized status from frontmatter (uses shared function from common.sh)
FM_STATUS=$(get_frontmatter_status "$DOC_PATH")

# II00: Implementation Issues section must NOT exist for Proposed/Accepted status
# These statuses indicate issues haven't been created yet via /plan
if [[ "$FM_STATUS" == "Proposed" || "$FM_STATUS" == "Accepted" ]]; then
    if grep -q "^## Implementation Issues" "$DOC_PATH"; then
        emit_fail "II00: Implementation Issues section not allowed in '$FM_STATUS' status. See: .github/scripts/docs/II00.md"
        exit $EXIT_FAIL
    fi
    exit $EXIT_PASS
fi

# Only validate table format for Planned and Current status
if [[ "$FM_STATUS" != "Planned" && "$FM_STATUS" != "Current" ]]; then
    exit $EXIT_PASS
fi

FAILED=0

# II01: Check for Implementation Issues section
# - Planned status: MUST have this section (created by /plan)
# - Current status: MAY have this section (Accepted→Current direct path doesn't create it)
if ! grep -q "^## Implementation Issues" "$DOC_PATH"; then
    if [[ "$FM_STATUS" == "Planned" ]]; then
        emit_fail "II01: Planned status requires '## Implementation Issues' section. See: .github/scripts/docs/II01.md"
        FAILED=1
        exit $EXIT_FAIL
    else
        # Current status without section - valid (direct Accepted→Current path)
        exit $EXIT_PASS
    fi
fi

# Extract the Implementation Issues section content
# Get content from "## Implementation Issues" until next "## " heading or end of file
ISSUES_SECTION=$(awk '
    /^## Implementation Issues/ { in_section = 1; next }
    in_section && /^## / { exit }
    in_section { print }
' "$DOC_PATH")

# Find the issues table (first table after section heading)
# Table starts with | Issue | and has separator row |---|
TABLE_HEADER=$(echo "$ISSUES_SECTION" | grep -E "^\| *Issue" | head -1 || true)

if [[ -z "$TABLE_HEADER" ]]; then
    emit_fail "II02: Implementation Issues section missing issues table. See: .github/scripts/docs/II02.md"
    FAILED=1
    exit $EXIT_FAIL
fi

# II02: Validate required columns exist
REQUIRED_COLUMNS=("Issue" "Title" "Dependencies" "Tier")
for col in "${REQUIRED_COLUMNS[@]}"; do
    if ! echo "$TABLE_HEADER" | grep -qiE "\| *$col *\|"; then
        # Check if it's the last column (no trailing |)
        if ! echo "$TABLE_HEADER" | grep -qiE "\| *$col *$"; then
            emit_fail "II02: Issues table missing required column: $col. See: .github/scripts/docs/II02.md"
            FAILED=1
        fi
    fi
done

# Extract table data rows (skip header, separator, and description rows)
# Description rows have italic text in first cell with empty remaining cells: | _text_ | | | |
TABLE_ROWS=$(echo "$ISSUES_SECTION" | awk '
    /^\|/ && !/^\| *-/ && !/^\| *Issue/ && !/^\| *_/ { print }
')

# Parse column positions from header
get_column_position() {
    local header="$1"
    local col_name="$2"
    echo "$header" | awk -v col="$col_name" '
    BEGIN { FS = "|"; pos = 0 }
    {
        for (i = 1; i <= NF; i++) {
            gsub(/^[ \t]+|[ \t]+$/, "", $i)
            if (tolower($i) == tolower(col)) {
                print i
                exit
            }
        }
    }
    '
}

ISSUE_COL=$(get_column_position "$TABLE_HEADER" "Issue")
DEP_COL=$(get_column_position "$TABLE_HEADER" "Dependencies")
TIER_COL=$(get_column_position "$TABLE_HEADER" "Tier")

# Helper: Strip strikethrough formatting ~~text~~ -> text
# Completed issues/milestones are shown with strikethrough
strip_strikethrough() {
    echo "$1" | sed 's/^~~//;s/~~$//'
}

# Helper: Check if a value is a valid markdown link [text](url)
is_markdown_link() {
    local val
    val=$(strip_strikethrough "$1")
    [[ "$val" =~ ^\[.+\]\(.+\)$ ]]
}

# Helper: Check if a value is a milestone link (contains /milestone/ in URL)
is_milestone_link() {
    local val
    val=$(strip_strikethrough "$1")
    [[ "$val" =~ ^\[.+\]\(.*milestone.*\)$ ]]
}

# Helper: Check if a value is an issue link [#N](url)
is_issue_link() {
    local val
    val=$(strip_strikethrough "$1")
    [[ "$val" =~ ^\[#[0-9]+\]\(.+\)$ ]]
}

# Process each row
while IFS= read -r row; do
    [[ -z "$row" ]] && continue

    # Extract column values
    ISSUE_VAL=$(echo "$row" | awk -F'|' -v col="$ISSUE_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
    DEP_VAL=$(echo "$row" | awk -F'|' -v col="$DEP_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
    TIER_VAL=$(echo "$row" | awk -F'|' -v col="$TIER_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')

    # Strip strikethrough for comparison (completed items use ~~text~~)
    TIER_VAL_CLEAN=$(strip_strikethrough "$TIER_VAL")

    # Determine if this is a milestone row or issue row
    IS_MILESTONE_ROW=false
    if [[ "$TIER_VAL_CLEAN" == "milestone" ]] || is_milestone_link "$ISSUE_VAL"; then
        IS_MILESTONE_ROW=true
    fi

    # II03: Validate first column format
    if [[ -n "$ISSUE_VAL" ]]; then
        if [[ "$IS_MILESTONE_ROW" == true ]]; then
            # Milestone row: must be a valid markdown link
            if ! is_markdown_link "$ISSUE_VAL"; then
                emit_fail "II03: Invalid milestone link format: '$ISSUE_VAL'. See: .github/scripts/docs/II03.md"
                FAILED=1
            fi
        else
            # Issue row: must be [#N](url) format
            if ! is_issue_link "$ISSUE_VAL"; then
                emit_fail "II03: Invalid issue link format: '$ISSUE_VAL'. See: .github/scripts/docs/II03.md"
                FAILED=1
            fi
        fi
    fi

    # II04: Validate dependencies format
    if [[ -n "$DEP_VAL" ]]; then
        # Strip strikethrough and check for "None"
        DEP_VAL_CLEAN=$(strip_strikethrough "$DEP_VAL")
        if [[ "$DEP_VAL_CLEAN" != "None" ]]; then
            # Split on comma and validate each
            IFS=',' read -ra DEPS <<< "$DEP_VAL"
            for dep in "${DEPS[@]}"; do
                # Trim whitespace
                dep=$(echo "$dep" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
                # Strip strikethrough for validation
                dep_clean=$(strip_strikethrough "$dep")
                # Check for plain text issue reference (e.g., #123)
                if [[ "$dep_clean" =~ ^#[0-9]+$ ]]; then
                    emit_fail "II04: Dependencies must use link format, not plain text: '$dep'. See: .github/scripts/docs/II04.md"
                    FAILED=1
                # Check for plain text milestone reference (e.g., M38)
                elif [[ "$dep_clean" =~ ^M[0-9]+$ ]]; then
                    emit_fail "II04: Dependencies must use link format, not plain text: '$dep'. See: .github/scripts/docs/II04.md"
                    FAILED=1
                # Must be a valid markdown link (issue or milestone)
                elif ! is_markdown_link "$dep"; then
                    emit_fail "II04: Invalid dependency format: '$dep'. See: .github/scripts/docs/II04.md"
                    FAILED=1
                fi
            done
        fi
    fi

    # II05: Validate tier value (use cleaned value, strikethrough is valid)
    if [[ -n "$TIER_VAL" ]]; then
        case "$TIER_VAL_CLEAN" in
            simple|testable|critical|milestone)
                # Valid
                ;;
            *)
                emit_fail "II05: Invalid tier value: '$TIER_VAL'. See: .github/scripts/docs/II05.md"
                FAILED=1
                ;;
        esac
    fi
done <<< "$TABLE_ROWS"

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
