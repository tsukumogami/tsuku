#!/usr/bin/env bash
#
# implementation-issues.sh - Validate design document Implementation Issues section
#
# Implements implementation issues validation rules:
#   II00: Section must NOT exist for Proposed/Accepted status
#   II01: Section exists for Planned status (optional for Current)
#   II02: Table has correct columns (Issue, Dependencies, Tier) or legacy (Issue, Title, Dependencies, Tier)
#   II03: Issue/milestone links use valid format [#N](url) or [Name](milestone-url)
#   II04: Dependencies use link format, not plain text
#   II05: Tier values are valid (simple, testable, critical, milestone)
#   II06: Child design reference links must point to existing files
#   II07: Every issue row must be followed by a description row (grandfathered before cutoff)
#   II08: Strikethrough consistency between issue, child reference, and description rows
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
# Accept both formats:
#   New (3-col): | Issue | Dependencies | Tier |
#   Legacy (4-col): | Issue | Title | Dependencies | Tier |
REQUIRED_COLUMNS=("Issue" "Dependencies" "Tier")
for col in "${REQUIRED_COLUMNS[@]}"; do
    if ! echo "$TABLE_HEADER" | grep -qiE "\| *$col *\|"; then
        # Check if it's the last column (no trailing |)
        if ! echo "$TABLE_HEADER" | grep -qiE "\| *$col *$"; then
            emit_fail "II02: Issues table missing required column: $col. See: .github/scripts/docs/II02.md"
            FAILED=1
        fi
    fi
done

# Extract table data rows (skip header, separator, description rows, and child reference rows)
# Description rows: | _text_ | | | | or struck-through: | ~~_text_~~ | | | |
# Child reference rows: | ^_Child: ..._ | | | | or struck-through: | ~~^_Child: ..._~~ | | | |
TABLE_ROWS=$(echo "$ISSUES_SECTION" | awk '
    /^\|/ && !/^\| *-/ && !/^\| *Issue/ && !/^\| *_/ && !/^\| *~~_/ && !/^\| *\^_/ && !/^\| *~~\^_/ { print }
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

# Helper: Check if a value is an issue link [#N](url) or [#N: title](url)
is_issue_link() {
    local val
    val=$(strip_strikethrough "$1")
    [[ "$val" =~ ^\[#[0-9]+(\]|:\ .*\])\(.+\)$ ]]
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

# II07 and II08: Row sequence validation
# These checks operate on the full sequence of table rows (including description and child reference rows)
# to validate that issue rows are followed by description rows and that strikethrough is consistent.

# II07 grandfathering: only enforce for docs created on or after this date
II07_CUTOFF="2026-02-16"

# Helper: check if the doc predates the II07 cutoff
ii07_grandfathered() {
    local file="$1"
    local creation_info
    creation_info=$("$SCRIPT_DIR/../get-file-creation-commit.sh" "$file" 2>/dev/null) || return 1
    local file_date
    file_date=$(echo "$creation_info" | sed 's/.*"date": "\([0-9-]*\)T.*/\1/')
    [[ "$file_date" < "$II07_CUTOFF" ]]
}

# Helper: check if a row has strikethrough on its first cell content
row_has_strikethrough() {
    local row="$1"
    local first_cell
    first_cell=$(echo "$row" | awk -F'|' '{ gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2 }')
    [[ "$first_cell" =~ ^~~ ]]
}

# Helper: check if a row is a description row (| _text_ | | | or | ~~_text_~~ | | |)
is_description_row() {
    local row="$1"
    [[ "$row" =~ ^\|\ *_ ]] || [[ "$row" =~ ^\|\ *~~_ ]]
}

# Helper: check if a row is a child reference row (| ^_text_ | | | or | ~~^_text_~~ | | |)
is_child_ref_row() {
    local row="$1"
    [[ "$row" =~ ^\|\ *\^_ ]] || [[ "$row" =~ ^\|\ *~~\^_ ]]
}

# Helper: check if a row is an issue/milestone data row (not header, separator, description, or child ref)
is_data_row() {
    local row="$1"
    [[ "$row" =~ ^\| ]] && ! [[ "$row" =~ ^\|\ *- ]] && ! [[ "$row" =~ ^\|\ *Issue ]] && \
        ! is_description_row "$row" && ! is_child_ref_row "$row"
}

# Determine if II07 applies to this doc
ENFORCE_II07=true
if ii07_grandfathered "$DOC_PATH"; then
    ENFORCE_II07=false
fi

# Extract ALL table rows (including description and child reference rows, but not header/separator)
# We need the full sequence to check row ordering
ALL_TABLE_ROWS=$(echo "$ISSUES_SECTION" | awk '
    /^\|/ && !/^\| *-/ && !/^\| *Issue/ { print }
')

# Build an array of all rows for sequential analysis
declare -a ROW_ARRAY
ROW_INDEX=0
while IFS= read -r row; do
    [[ -z "$row" ]] && continue
    ROW_ARRAY[$ROW_INDEX]="$row"
    ROW_INDEX=$((ROW_INDEX + 1))
done <<< "$ALL_TABLE_ROWS"

TOTAL_ROWS=${#ROW_ARRAY[@]}

# Walk through rows and validate II07/II08
i=0
while [[ $i -lt $TOTAL_ROWS ]]; do
    row="${ROW_ARRAY[$i]}"

    if is_data_row "$row"; then
        # This is an issue or milestone row
        ISSUE_STRIKETHROUGH=false
        if row_has_strikethrough "$row"; then
            ISSUE_STRIKETHROUGH=true
        fi

        # Extract the issue identifier for error messages
        ISSUE_ID=$(echo "$row" | awk -F'|' '{ gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2 }')

        # Look at the next row(s)
        NEXT_IDX=$((i + 1))
        HAS_CHILD_REF=false
        HAS_DESCRIPTION=false

        # Check for optional child reference row
        if [[ $NEXT_IDX -lt $TOTAL_ROWS ]] && is_child_ref_row "${ROW_ARRAY[$NEXT_IDX]}"; then
            HAS_CHILD_REF=true
            CHILD_REF_ROW="${ROW_ARRAY[$NEXT_IDX]}"

            # II08: child reference row strikethrough must match issue row
            if [[ "$ISSUE_STRIKETHROUGH" == true ]] && ! row_has_strikethrough "$CHILD_REF_ROW"; then
                emit_fail "II08: Struck-through issue row has non-struck child reference row: '$ISSUE_ID'. See: .github/scripts/docs/II08.md"
                FAILED=1
            fi

            # II06: Validate child design reference link points to existing file
            CHILD_CELL=$(echo "$CHILD_REF_ROW" | awk -F'|' '{ gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2 }')
            # Strip strikethrough if present
            CHILD_CELL_CLEAN=$(echo "$CHILD_CELL" | sed 's/^~~//;s/~~$//')
            # Extract path from ^_Child: [DESIGN-name.md](path)_
            II06_REF_PATH=$(echo "$CHILD_CELL_CLEAN" | grep -oE '\]\([^)]+\)' | sed 's/^\](\(.*\))$/\1/' || true)

            if [[ -n "$II06_REF_PATH" ]]; then
                # Check for cross-repo references (absolute URLs)
                if [[ "$II06_REF_PATH" =~ ^https?:// ]]; then
                    emit_pass "II06: Child reference in '$ISSUE_ID' is a cross-repo URL, skipping validation"
                else
                    # Resolve relative to the design doc's directory
                    II06_DOC_DIR=$(dirname "$DOC_PATH")
                    II06_CHILD_PATH="$II06_DOC_DIR/$II06_REF_PATH"

                    # Check if path resolves outside repo root
                    II06_REPO_ROOT=$(cd "$II06_DOC_DIR" && git rev-parse --show-toplevel 2>/dev/null || echo "")
                    II06_IS_CROSS_REPO=false
                    if [[ -n "$II06_REPO_ROOT" ]]; then
                        II06_REAL_PATH=$(cd "$II06_DOC_DIR" && realpath -m "$II06_REF_PATH" 2>/dev/null || echo "$II06_CHILD_PATH")
                        if [[ ! "$II06_REAL_PATH" =~ ^"$II06_REPO_ROOT" ]]; then
                            II06_IS_CROSS_REPO=true
                        fi
                    fi

                    if [[ "$II06_IS_CROSS_REPO" == true ]]; then
                        emit_pass "II06: Child reference in '$ISSUE_ID' points outside repo, skipping validation"
                    elif [[ ! -f "$II06_CHILD_PATH" ]]; then
                        emit_fail "II06: Child reference in '$ISSUE_ID' points to nonexistent file: '$II06_REF_PATH'. See: .github/scripts/docs/II06.md"
                        FAILED=1
                    fi
                fi
            fi

            NEXT_IDX=$((NEXT_IDX + 1))
        fi

        # Check for description row
        if [[ $NEXT_IDX -lt $TOTAL_ROWS ]] && is_description_row "${ROW_ARRAY[$NEXT_IDX]}"; then
            HAS_DESCRIPTION=true
            DESC_ROW="${ROW_ARRAY[$NEXT_IDX]}"

            # II08: description row strikethrough must match issue row
            if [[ "$ISSUE_STRIKETHROUGH" == true ]] && ! row_has_strikethrough "$DESC_ROW"; then
                emit_fail "II08: Struck-through issue row has non-struck description row: '$ISSUE_ID'. See: .github/scripts/docs/II08.md"
                FAILED=1
            fi
        fi

        # II07: issue row must be followed by a description row
        if [[ "$ENFORCE_II07" == true ]] && [[ "$HAS_DESCRIPTION" == false ]]; then
            emit_fail "II07: Issue row missing required description row: '$ISSUE_ID'. See: .github/scripts/docs/II07.md"
            FAILED=1
        fi
    fi

    i=$((i + 1))
done

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
