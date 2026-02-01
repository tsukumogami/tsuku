#!/usr/bin/env bash
#
# extract-design-issues.sh - Extract Implementation Issues from a design doc as JSON
#
# Parses the Implementation Issues tables from a design document and outputs
# structured JSON with all milestones and their issues, status, dependencies, and tier.
#
# Usage:
#   extract-design-issues.sh <doc-path>
#
# Output (JSON):
#   {
#     "milestones": [
#       {
#         "name": "...",
#         "url": "...",
#         "entries": [
#           { "type": "issue", "number": 123, "url": "...", "title": "...", ... }
#         ]
#       }
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

# Get column positions from header
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

# Parse a table and return its entries as JSON array
parse_table() {
    local table_content="$1"
    local header
    header=$(echo "$table_content" | grep -E "^\| *Issue" | head -1 || true)

    if [[ -z "$header" ]]; then
        echo "[]"
        return
    fi

    local issue_col title_col dep_col tier_col
    issue_col=$(get_column_position "$header" "Issue")
    title_col=$(get_column_position "$header" "Title")
    dep_col=$(get_column_position "$header" "Dependencies")
    tier_col=$(get_column_position "$header" "Tier")

    # Extract table data rows (skip header, separator, and description rows)
    # Description rows have italic text in first cell: | _text_ | | | |
    local rows
    rows=$(echo "$table_content" | awk '
        /^\|/ && !/^\| *-/ && !/^\| *Issue/ && !/^\| *_/ { print }
    ')

    local entries="[]"

    while IFS= read -r row; do
        [[ -z "$row" ]] && continue

        # Extract column values
        local issue_val title_val dep_val tier_val
        issue_val=$(echo "$row" | awk -F'|' -v col="$issue_col" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
        title_val=$(echo "$row" | awk -F'|' -v col="$title_col" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
        dep_val=$(echo "$row" | awk -F'|' -v col="$dep_col" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')
        tier_val=$(echo "$row" | awk -F'|' -v col="$tier_col" '{ gsub(/^[ \t]+|[ \t]+$/, "", $col); print $col }')

        [[ -z "$issue_val" ]] && continue

        # Determine type, number, URL, completion status
        local entry_type entry_number entry_url entry_title entry_tier entry_deps completed number_json
        entry_type=$(get_entry_type "$issue_val" "$tier_val")
        entry_number=$(extract_number "$issue_val" "$entry_type")
        entry_url=$(extract_link_url "$issue_val")
        entry_title=$(strip_strikethrough "$title_val")
        entry_tier=$(strip_strikethrough "$tier_val")
        entry_deps=$(parse_dependencies "$dep_val")

        # Check if completed (strikethrough on issue column)
        if has_strikethrough "$issue_val"; then
            completed="true"
        else
            completed="false"
        fi

        # Handle null number for named milestones without numeric ID
        if [[ -z "$entry_number" ]]; then
            number_json="null"
        else
            number_json="$entry_number"
        fi

        # Build entry JSON
        local entry
        entry=$(jq -n \
            --arg type "$entry_type" \
            --argjson number "$number_json" \
            --arg url "$entry_url" \
            --arg title "$entry_title" \
            --argjson deps "$entry_deps" \
            --arg tier "$entry_tier" \
            --argjson completed "$completed" \
            '{
                "type": $type,
                "number": $number,
                "url": $url,
                "title": $title,
                "dependencies": $deps,
                "tier": $tier,
                "completed": $completed
            }')

        entries=$(echo "$entries" | jq --argjson entry "$entry" '. + [$entry]')
    done <<< "$rows"

    echo "$entries"
}

# Split content by milestone headings and parse each section
MILESTONES="[]"

# Find all milestone headings with their line numbers
MILESTONE_LINES=$(echo "$ISSUES_SECTION" | grep -n "^### Milestone:" || true)

if [[ -z "$MILESTONE_LINES" ]]; then
    # No milestone headings - treat entire section as unnamed milestone
    TABLE_HEADER=$(echo "$ISSUES_SECTION" | grep -E "^\| *Issue" | head -1 || true)
    if [[ -n "$TABLE_HEADER" ]]; then
        entries=$(parse_table "$ISSUES_SECTION")
        milestone_obj=$(jq -n --arg name "" --arg url "" --argjson entries "$entries" \
            '{ "name": $name, "url": $url, "entries": $entries }')
        MILESTONES=$(echo "$MILESTONES" | jq --argjson m "$milestone_obj" '. + [$m]')
    fi
else
    # Multiple milestones - split by heading
    TOTAL_LINES=$(echo "$ISSUES_SECTION" | wc -l)

    # Parse each milestone section
    while IFS= read -r line_info; do
        [[ -z "$line_info" ]] && continue

        LINE_NUM=$(echo "$line_info" | cut -d: -f1)
        MILESTONE_LINE=$(echo "$line_info" | cut -d: -f2-)

        # Extract milestone name and URL
        MILESTONE_NAME=$(echo "$MILESTONE_LINE" | sed -n 's/.*\[\([^]]*\)\].*/\1/p')
        MILESTONE_URL=$(echo "$MILESTONE_LINE" | sed -n 's/.*(\([^)]*\)).*/\1/p')

        # Find the end of this milestone section (next milestone or end of section)
        NEXT_LINE=$(echo "$MILESTONE_LINES" | awk -F: -v current="$LINE_NUM" '$1 > current { print $1; exit }')
        if [[ -z "$NEXT_LINE" ]]; then
            NEXT_LINE=$((TOTAL_LINES + 1))
        fi

        # Extract content for this milestone (from heading to next heading)
        SECTION_CONTENT=$(echo "$ISSUES_SECTION" | sed -n "${LINE_NUM},$((NEXT_LINE - 1))p")

        # Parse the table in this section
        entries=$(parse_table "$SECTION_CONTENT")

        # Build milestone object
        milestone_obj=$(jq -n \
            --arg name "$MILESTONE_NAME" \
            --arg url "$MILESTONE_URL" \
            --argjson entries "$entries" \
            '{ "name": $name, "url": $url, "entries": $entries }')

        MILESTONES=$(echo "$MILESTONES" | jq --argjson m "$milestone_obj" '. + [$m]')
    done <<< "$MILESTONE_LINES"
fi

# Output final JSON
jq -n --argjson milestones "$MILESTONES" '{ "milestones": $milestones }'
