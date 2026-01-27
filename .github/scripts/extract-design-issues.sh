#!/usr/bin/env bash
#
# extract-design-issues.sh - Extract Implementation Issues from a design doc as JSON
#
# Parses the Implementation Issues table from a design document and outputs
# structured JSON with all issues and milestones, their status, dependencies, and tier.
#
# Usage:
#   extract-design-issues.sh <doc-path>
#
# Output (JSON):
#   {
#     "milestone": { "name": "...", "url": "..." },
#     "entries": [
#       { "type": "issue", "number": 123, "url": "...", "title": "...", ... }
#     ]
#   }
#
# Exit codes:
#   0 - Success
#   1 - No Implementation Issues section found
#   2 - Operational error (missing argument, file not found)
#
# Example:
#   ./extract-design-issues.sh docs/designs/DESIGN-feature.md

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Error: missing required argument <doc-path>" >&2
    echo "Usage: extract-design-issues.sh <doc-path>" >&2
    exit 2
fi

DOC_PATH="$1"

if [[ ! -f "$DOC_PATH" ]]; then
    echo "Error: file not found: $DOC_PATH" >&2
    exit 2
fi

# Check for Implementation Issues section
if ! grep -q "^## Implementation Issues" "$DOC_PATH"; then
    echo "Error: no Implementation Issues section found in $DOC_PATH" >&2
    exit 1
fi

# Extract the Implementation Issues section content
ISSUES_SECTION=$(awk '
    /^## Implementation Issues/ { in_section = 1; next }
    in_section && /^## / { exit }
    in_section { print }
' "$DOC_PATH")

# Extract milestone info from heading: ### Milestone: [Name](url)
MILESTONE_LINE=$(echo "$ISSUES_SECTION" | grep -E "^### Milestone:" | head -1 || true)
MILESTONE_JSON="null"

if [[ -n "$MILESTONE_LINE" ]]; then
    # Extract [Name](url) pattern
    MILESTONE_NAME=$(echo "$MILESTONE_LINE" | sed -n 's/.*\[\([^]]*\)\].*/\1/p')
    MILESTONE_URL=$(echo "$MILESTONE_LINE" | sed -n 's/.*(\([^)]*\)).*/\1/p')
    if [[ -n "$MILESTONE_NAME" && -n "$MILESTONE_URL" ]]; then
        MILESTONE_JSON=$(jq -n --arg name "$MILESTONE_NAME" --arg url "$MILESTONE_URL" \
            '{ "name": $name, "url": $url }')
    fi
fi

# Find the issues table (first table after section heading)
TABLE_HEADER=$(echo "$ISSUES_SECTION" | grep -E "^\| *Issue" | head -1 || true)

if [[ -z "$TABLE_HEADER" ]]; then
    # No table found, output with empty entries
    jq -n --argjson milestone "$MILESTONE_JSON" \
        '{ "milestone": $milestone, "entries": [] }'
    exit 0
fi

# Get column positions
get_column_position() {
    local header="$1"
    local col_name="$2"
    echo "$header" | awk -v col="$col_name" '
    BEGIN { FS = "|" }
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
TITLE_COL=$(get_column_position "$TABLE_HEADER" "Title")
DEP_COL=$(get_column_position "$TABLE_HEADER" "Dependencies")
TIER_COL=$(get_column_position "$TABLE_HEADER" "Tier")

# Extract table data rows (skip header and separator)
TABLE_ROWS=$(echo "$ISSUES_SECTION" | awk '
    /^\|/ && !/^\| *-/ && !/^\| *Issue/ { print }
')

# Helper to strip strikethrough: ~~text~~ -> text
strip_strikethrough() {
    echo "$1" | sed 's/^~~//;s/~~$//'
}

# Helper to check if value has strikethrough
has_strikethrough() {
    [[ "$1" =~ ^~~.*~~$ ]]
}

# Helper to extract link text: [text](url) -> text
extract_link_text() {
    local val
    val=$(strip_strikethrough "$1")
    echo "$val" | sed -n 's/^\[\([^]]*\)\].*/\1/p'
}

# Helper to extract link URL: [text](url) -> url
extract_link_url() {
    local val
    val=$(strip_strikethrough "$1")
    echo "$val" | sed -n 's/.*(\([^)]*\))$/\1/p'
}

# Helper to determine entry type (issue or milestone)
get_entry_type() {
    local issue_val="$1"
    local tier_val="$2"
    local clean_issue
    clean_issue=$(strip_strikethrough "$issue_val")
    local clean_tier
    clean_tier=$(strip_strikethrough "$tier_val")

    # Milestone if tier is "milestone" or URL contains /milestone/
    if [[ "$clean_tier" == "milestone" ]] || [[ "$clean_issue" =~ milestone/ ]]; then
        echo "milestone"
    else
        echo "issue"
    fi
}

# Helper to extract issue/milestone number from link
extract_number() {
    local val="$1"
    local entry_type="$2"
    local url
    url=$(extract_link_url "$val")

    if [[ "$entry_type" == "issue" ]]; then
        # Extract from [#N](url) format
        local text
        text=$(extract_link_text "$val")
        echo "$text" | grep -oE '[0-9]+' || true
    else
        # Extract from milestone URL: .../milestone/N
        echo "$url" | grep -oE 'milestone/[0-9]+' | grep -oE '[0-9]+' || true
    fi
}

# Parse dependencies: "None" or "[#X](url), [#Y](url)"
parse_dependencies() {
    local dep_val="$1"
    local dep_clean
    dep_clean=$(strip_strikethrough "$dep_val")

    if [[ "$dep_clean" == "None" || -z "$dep_clean" ]]; then
        echo "[]"
        return
    fi

    # Split on comma and extract each reference
    local deps=()
    IFS=',' read -ra DEP_PARTS <<< "$dep_val"
    for part in "${DEP_PARTS[@]}"; do
        part=$(echo "$part" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        local text
        text=$(extract_link_text "$part")
        if [[ -n "$text" ]]; then
            deps+=("$text")
        fi
    done

    # Output as JSON array
    printf '%s\n' "${deps[@]}" | jq -R -s 'split("\n") | map(select(length > 0))'
}

# Build entries array
ENTRIES="[]"

while IFS= read -r row; do
    [[ -z "$row" ]] && continue

    # Extract column values
    ISSUE_VAL=$(echo "$row" | awk -F'|' -v col="$ISSUE_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
    TITLE_VAL=$(echo "$row" | awk -F'|' -v col="$TITLE_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
    DEP_VAL=$(echo "$row" | awk -F'|' -v col="$DEP_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
    TIER_VAL=$(echo "$row" | awk -F'|' -v col="$TIER_COL" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')

    [[ -z "$ISSUE_VAL" ]] && continue

    # Determine type, number, URL, completion status
    ENTRY_TYPE=$(get_entry_type "$ISSUE_VAL" "$TIER_VAL")
    ENTRY_NUMBER=$(extract_number "$ISSUE_VAL" "$ENTRY_TYPE")
    ENTRY_URL=$(extract_link_url "$ISSUE_VAL")
    ENTRY_TITLE=$(strip_strikethrough "$TITLE_VAL")
    ENTRY_TIER=$(strip_strikethrough "$TIER_VAL")
    ENTRY_DEPS=$(parse_dependencies "$DEP_VAL")

    # Check if completed (strikethrough on issue column)
    if has_strikethrough "$ISSUE_VAL"; then
        COMPLETED="true"
    else
        COMPLETED="false"
    fi

    # Handle null number for named milestones without numeric ID
    if [[ -z "$ENTRY_NUMBER" ]]; then
        NUMBER_JSON="null"
    else
        NUMBER_JSON="$ENTRY_NUMBER"
    fi

    # Build entry JSON
    ENTRY=$(jq -n \
        --arg type "$ENTRY_TYPE" \
        --argjson number "$NUMBER_JSON" \
        --arg url "$ENTRY_URL" \
        --arg title "$ENTRY_TITLE" \
        --argjson deps "$ENTRY_DEPS" \
        --arg tier "$ENTRY_TIER" \
        --argjson completed "$COMPLETED" \
        '{
            "type": $type,
            "number": $number,
            "url": $url,
            "title": $title,
            "dependencies": $deps,
            "tier": $tier,
            "completed": $completed
        }')

    ENTRIES=$(echo "$ENTRIES" | jq --argjson entry "$ENTRY" '. + [$entry]')
done <<< "$TABLE_ROWS"

# Output final JSON
jq -n --argjson milestone "$MILESTONE_JSON" --argjson entries "$ENTRIES" \
    '{ "milestone": $milestone, "entries": $entries }'
