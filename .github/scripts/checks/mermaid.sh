#!/usr/bin/env bash
#
# mermaid.sh - Validate Mermaid dependency diagrams in design documents
#
# Implements mermaid diagram validation rules:
#   MM01: Use 'graph', not 'flowchart'
#   MM02: Direction (LR or TD) required
#   MM03: No edges inside subgraph blocks
#   MM04: Class definitions must come after edges
#   MM05: Valid class names (done, ready, blocked, needsDesign)
#   MM06: Issue in table must appear in diagram
#   MM07: Diagram only allowed in "Planned" status
#   MM08: Only one mermaid diagram per document
#   MM09: Issue in diagram must appear in table (no orphans)
#   MM10: Node naming convention: I<number>
#   MM11: Every node must have a class assigned
#   MM12: If classDef present, colors must be standardized
#   MM13: If subgraph styling present, colors must be standardized
#   MM14: Legend line required after diagram
#   MM15: Class must match actual issue status (requires gh CLI)
#
# Usage:
#   mermaid.sh [-q|--quiet] [--skip-status-check] <doc-path>
#
# Options:
#   -q, --quiet          Suppress [PASS] messages, only show failures
#   --skip-status-check  Skip MM15 (GitHub API status validation)
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Operational error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

CI_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Valid class names and their standard colors
declare -A VALID_CLASS_COLORS=(
    ["done"]="#c8e6c9"
    ["ready"]="#bbdefb"
    ["blocked"]="#fff9c4"
    ["needsDesign"]="#e1bee7"
)

# Parse arguments
SKIP_STATUS_CHECK=0
DOC_PATH=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -q|--quiet)
            export QUIET_MODE=1
            shift
            ;;
        --skip-status-check)
            SKIP_STATUS_CHECK=1
            shift
            ;;
        -*)
            echo "Error: unknown option: $1" >&2
            exit $EXIT_ERROR
            ;;
        *)
            DOC_PATH="$1"
            shift
            ;;
    esac
done

if [[ -z "$DOC_PATH" ]]; then
    echo "Error: missing required argument <doc-path>" >&2
    exit $EXIT_ERROR
fi

# Check file exists
if [[ ! -f "$DOC_PATH" ]]; then
    echo "Error: file not found: $DOC_PATH" >&2
    exit $EXIT_ERROR
fi

# Get normalized status from frontmatter (uses shared function from common.sh)
FM_STATUS=$(get_frontmatter_status "$DOC_PATH")

# Count mermaid code blocks
# Note: grep -c returns 0 count with exit 1, so fallback must be outside subshell
MERMAID_COUNT=$(grep -c '```mermaid' "$DOC_PATH" 2>/dev/null) || MERMAID_COUNT=0

# If no mermaid diagram, check if Planned status requires one
if [[ "$MERMAID_COUNT" -eq 0 ]]; then
    if [[ "$FM_STATUS" == "Planned" ]]; then
        emit_fail "MM07: 'Planned' status requires an issue dependency diagram. See: .github/scripts/docs/MM07.md"
        exit $EXIT_FAIL
    fi
    exit $EXIT_PASS
fi

# Extract all mermaid block content to check for issue dependency nodes
ALL_MERMAID_CONTENT=$(awk '
    /^```mermaid/ { in_mermaid = 1; next }
    /^```/ && in_mermaid { in_mermaid = 0; next }
    in_mermaid { print }
' "$DOC_PATH")

# Check if any diagram contains I<number> nodes (issue dependency diagram)
# Pattern: I followed by one or more digits, as a word boundary
ISSUE_NODES=$(echo "$ALL_MERMAID_CONTENT" | grep -oE '\bI[0-9]+\b' | sort -u || true)
HAS_ISSUE_DIAGRAM=0
if [[ -n "$ISSUE_NODES" ]]; then
    HAS_ISSUE_DIAGRAM=1
fi

# MM07: Issue dependency diagram rules
# - Issue dependency diagrams (with I<number> nodes) ONLY allowed in Planned status
# - Non-issue diagrams (documentation, examples) allowed in any status
# - Planned status MUST have exactly one issue dependency diagram
if [[ "$HAS_ISSUE_DIAGRAM" -eq 1 ]]; then
    if [[ "$FM_STATUS" != "Planned" ]]; then
        emit_fail "MM07: Issue dependency diagram (with I<number> nodes) only allowed in 'Planned' status, found in '$FM_STATUS'. See: .github/scripts/docs/MM07.md"
        exit $EXIT_FAIL
    fi
else
    # No issue dependency diagram - check if Planned status requires one
    if [[ "$FM_STATUS" == "Planned" ]]; then
        emit_fail "MM07: 'Planned' status requires an issue dependency diagram (with I<number> nodes). See: .github/scripts/docs/MM07.md"
        exit $EXIT_FAIL
    fi
    # Non-issue diagrams in non-Planned status - skip all further validation
    exit $EXIT_PASS
fi

# At this point: we have an issue dependency diagram in Planned status
# Continue with detailed validation

# MM08: Only one diagram per doc (for issue dependency diagrams)
if [[ "$MERMAID_COUNT" -gt 1 ]]; then
    emit_fail "MM08: Only one mermaid diagram allowed per document, found $MERMAID_COUNT. See: .github/scripts/docs/MM08.md"
    exit $EXIT_FAIL
fi

# Use the already extracted mermaid content
MERMAID_CONTENT="$ALL_MERMAID_CONTENT"

FAILED=0

# MM01: Check for flowchart keyword
if echo "$MERMAID_CONTENT" | grep -qE '^flowchart\b'; then
    emit_fail "MM01: Mermaid diagram should use 'graph', not 'flowchart'. See: .github/scripts/docs/MM01.md"
    FAILED=1
fi

# MM02: Check for direction (LR or TD)
GRAPH_LINE=$(echo "$MERMAID_CONTENT" | grep -E '^graph\b' | head -1 || true)
if [[ -n "$GRAPH_LINE" ]]; then
    if ! echo "$GRAPH_LINE" | grep -qE '^graph\s+(LR|TD)\b'; then
        emit_fail "MM02: Mermaid diagram missing direction (LR or TD). See: .github/scripts/docs/MM02.md"
        FAILED=1
    fi
fi

# MM03: Check for edges inside subgraph blocks
# Track subgraph depth and detect edges (-->, ---)
IN_SUBGRAPH=0
LINE_NUM=0
while IFS= read -r line; do
    LINE_NUM=$((LINE_NUM + 1))
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*%% ]] && continue

    # Track subgraph entry/exit
    if echo "$line" | grep -qE '^\s*subgraph\b'; then
        IN_SUBGRAPH=$((IN_SUBGRAPH + 1))
    elif echo "$line" | grep -qE '^\s*end\b'; then
        IN_SUBGRAPH=$((IN_SUBGRAPH - 1))
        [[ $IN_SUBGRAPH -lt 0 ]] && IN_SUBGRAPH=0
    fi

    # Check for edge while inside subgraph
    if [[ $IN_SUBGRAPH -gt 0 ]]; then
        if echo "$line" | grep -qE -- '-->|---'; then
            # Extract the edge for error message
            EDGE=$(echo "$line" | grep -oE -- '[A-Za-z0-9_]+\s*(-->|---)\s*[A-Za-z0-9_]+' | head -1)
            emit_fail "MM03: Mermaid edge inside subgraph will not render: '$EDGE'. See: .github/scripts/docs/MM03.md"
            FAILED=1
        fi
    fi
done <<< "$MERMAID_CONTENT"

# MM04: Check class definitions come after all edges
# Find line numbers of last edge and first class/classDef
LAST_EDGE_LINE=0
FIRST_CLASS_LINE=0
LINE_NUM=0
while IFS= read -r line; do
    LINE_NUM=$((LINE_NUM + 1))
    if echo "$line" | grep -qE -- '-->|---'; then
        LAST_EDGE_LINE=$LINE_NUM
    fi
    if echo "$line" | grep -qE '^\s*(class|classDef)\b'; then
        if [[ $FIRST_CLASS_LINE -eq 0 ]]; then
            FIRST_CLASS_LINE=$LINE_NUM
        fi
    fi
done <<< "$MERMAID_CONTENT"

if [[ $FIRST_CLASS_LINE -gt 0 && $LAST_EDGE_LINE -gt $FIRST_CLASS_LINE ]]; then
    emit_fail "MM04: Class definition before diagram edges. See: .github/scripts/docs/MM04.md"
    FAILED=1
fi

# MM05: Validate class names are valid
# Extract class assignments: class NodeA,NodeB classname
while IFS= read -r line; do
    if echo "$line" | grep -qE '^\s*class\s+'; then
        # Extract class name (last word on the line)
        CLASS_NAME=$(echo "$line" | awk '{print $NF}')
        if [[ ! "${VALID_CLASS_COLORS[$CLASS_NAME]+isset}" ]]; then
            emit_fail "MM05: Invalid class '$CLASS_NAME'. Valid: done, ready, blocked, needsDesign. See: .github/scripts/docs/MM05.md"
            FAILED=1
        fi
    fi
done <<< "$MERMAID_CONTENT"

# MM12: If classDef present, validate colors are standardized
while IFS= read -r line; do
    if echo "$line" | grep -qE '^\s*classDef\s+'; then
        # Extract class name and color
        CLASS_NAME=$(echo "$line" | awk '{print $2}')
        COLOR=$(echo "$line" | grep -oE 'fill:#[a-fA-F0-9]{6}' | sed 's/fill://' || true)

        if [[ -n "$COLOR" && "${VALID_CLASS_COLORS[$CLASS_NAME]+isset}" ]]; then
            EXPECTED_COLOR="${VALID_CLASS_COLORS[$CLASS_NAME]}"
            if [[ "${COLOR,,}" != "${EXPECTED_COLOR,,}" ]]; then
                emit_fail "MM12: classDef '$CLASS_NAME' has wrong color '$COLOR', expected '$EXPECTED_COLOR'. See: .github/scripts/docs/MM12.md"
                FAILED=1
            fi
        fi
    fi
done <<< "$MERMAID_CONTENT"

# Extract all node definitions (I<number>["..."])
# Pattern: I followed by digits, optionally with label in brackets
# Use the already extracted issue nodes
DIAGRAM_NODES="$ISSUE_NODES"

# MM10: Validate node naming convention
# Find any node definitions that don't follow I<number> pattern
# First, extract subgraph names to exclude them (subgraphs are not issue nodes)
SUBGRAPH_NAMES=$(echo "$MERMAID_CONTENT" | grep -E '^\s*subgraph\s+' | \
    sed 's/^\s*subgraph\s*//' | sed 's/\[.*$//' | tr -d ' ' || true)

# Look for node definitions like: NodeName["label"]
OTHER_NODES=$(echo "$MERMAID_CONTENT" | grep -oE '\b[A-Za-z][A-Za-z0-9_]*\[' | sed 's/\[$//' | grep -vE '^I[0-9]+$' || true)
if [[ -n "$OTHER_NODES" ]]; then
    while IFS= read -r node; do
        [[ -z "$node" ]] && continue
        # Skip known keywords
        [[ "$node" == "subgraph" || "$node" == "graph" || "$node" == "end" ]] && continue
        # Skip subgraph names (they're allowed to be non-I<number>)
        if [[ -n "$SUBGRAPH_NAMES" ]] && echo "$SUBGRAPH_NAMES" | grep -qE "^${node}$"; then
            continue
        fi
        emit_fail "MM10: Node '$node' doesn't follow naming convention I<number>. See: .github/scripts/docs/MM10.md"
        FAILED=1
    done <<< "$OTHER_NODES"
fi

# Extract issue numbers from Implementation Issues table
TABLE_ISSUES=""
if grep -q "^## Implementation Issues" "$DOC_PATH"; then
    TABLE_ISSUES=$("$CI_DIR/extract-design-issues.sh" "$DOC_PATH" 2>/dev/null | \
        jq -r '.milestones[].entries[] | select(.type == "issue") | .number' 2>/dev/null | sort -n || true)
fi

# MM06: Issue in table must appear in diagram
if [[ -n "$TABLE_ISSUES" ]]; then
    while IFS= read -r issue_num; do
        [[ -z "$issue_num" ]] && continue
        NODE_ID="I${issue_num}"
        if ! echo "$DIAGRAM_NODES" | grep -qE "^${NODE_ID}$"; then
            emit_fail "MM06: Issue #$issue_num in table but not in diagram. See: .github/scripts/docs/MM06.md"
            FAILED=1
        fi
    done <<< "$TABLE_ISSUES"
fi

# MM09: Issue in diagram must appear in table (no orphans)
if [[ -n "$DIAGRAM_NODES" ]]; then
    while IFS= read -r node; do
        [[ -z "$node" ]] && continue
        # Extract issue number from node ID (I123 -> 123)
        ISSUE_NUM=$(echo "$node" | sed 's/^I//')
        if [[ -n "$TABLE_ISSUES" ]]; then
            if ! echo "$TABLE_ISSUES" | grep -qE "^${ISSUE_NUM}$"; then
                emit_fail "MM09: Node $node in diagram but issue #$ISSUE_NUM not in table. See: .github/scripts/docs/MM09.md"
                FAILED=1
            fi
        fi
    done <<< "$DIAGRAM_NODES"
fi

# MM11: Every node must have a class assigned
# Extract all nodes that have class assignments
CLASSED_NODES=$(echo "$MERMAID_CONTENT" | grep -E '^\s*class\s+' | \
    sed 's/^\s*class\s*//' | sed 's/\s*[a-zA-Z]*$//' | tr ',' '\n' | tr -d ' ' | sort -u)

if [[ -n "$DIAGRAM_NODES" ]]; then
    while IFS= read -r node; do
        [[ -z "$node" ]] && continue
        if ! echo "$CLASSED_NODES" | grep -qE "^${node}$"; then
            emit_fail "MM11: Node '$node' has no class assigned. See: .github/scripts/docs/MM11.md"
            FAILED=1
        fi
    done <<< "$DIAGRAM_NODES"
fi

# MM13: If subgraph styling present, validate colors are standardized
# Standard: fill:#f5f5f5,stroke:#e0e0e0
while IFS= read -r line; do
    if echo "$line" | grep -qE '^\s*style\s+[A-Za-z]'; then
        # Extract fill color if present
        FILL_COLOR=$(echo "$line" | grep -oE 'fill:#[a-fA-F0-9]{6}' | sed 's/fill://' || true)
        STROKE_COLOR=$(echo "$line" | grep -oE 'stroke:#[a-fA-F0-9]{6}' | sed 's/stroke://' || true)

        if [[ -n "$FILL_COLOR" && "${FILL_COLOR,,}" != "#f5f5f5" ]]; then
            emit_fail "MM13: Subgraph fill color '$FILL_COLOR' should be '#f5f5f5'. See: .github/scripts/docs/MM13.md"
            FAILED=1
        fi
        if [[ -n "$STROKE_COLOR" && "${STROKE_COLOR,,}" != "#e0e0e0" ]]; then
            emit_fail "MM13: Subgraph stroke color '$STROKE_COLOR' should be '#e0e0e0'. See: .github/scripts/docs/MM13.md"
            FAILED=1
        fi
    fi
done <<< "$MERMAID_CONTENT"

# MM14: Legend line required after diagram
# Look for "**Legend**:" pattern after the mermaid block
LEGEND_LINE=$(awk '
    /^```mermaid/ { in_mermaid = 1; next }
    /^```/ && in_mermaid { in_mermaid = 0; after_mermaid = 1; next }
    after_mermaid && /^\*\*Legend\*\*:/ { found = 1; exit }
    after_mermaid && /^##/ { exit }  # Hit next section
    END { print found ? "found" : "" }
' "$DOC_PATH")

if [[ -z "$LEGEND_LINE" ]]; then
    emit_fail "MM14: Legend line required after diagram. Add: **Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design. See: .github/scripts/docs/MM14.md"
    FAILED=1
fi

# MM15: Validate class matches actual issue status
# Only run if gh CLI is available, we have diagram nodes, and --skip-status-check not set
if [[ "$SKIP_STATUS_CHECK" -eq 0 ]] && command -v gh &>/dev/null && [[ -n "$DIAGRAM_NODES" ]]; then
    # Build dependency map: for each node, list its blockers (nodes it depends on)
    # Edge A --> B means A blocks B (B depends on A)
    declare -A BLOCKERS
    while IFS= read -r line; do
        # Match edges: I123 --> I456 or I123 --- I456
        if echo "$line" | grep -qE -- 'I[0-9]+\s*(-->|---)\s*I[0-9]+'; then
            # Extract source and target
            SOURCE=$(echo "$line" | grep -oE 'I[0-9]+' | head -1)
            TARGET=$(echo "$line" | grep -oE 'I[0-9]+' | tail -1)
            if [[ -n "$SOURCE" && -n "$TARGET" && "$SOURCE" != "$TARGET" ]]; then
                # Add source as a blocker of target
                BLOCKERS[$TARGET]="${BLOCKERS[$TARGET]:-} $SOURCE"
            fi
        fi
    done <<< "$MERMAID_CONTENT"

    # Get actual class assignments from diagram
    declare -A ACTUAL_CLASS
    while IFS= read -r line; do
        if echo "$line" | grep -qE '^\s*class\s+'; then
            # Format: class I123,I456 done
            CLASS_NAME=$(echo "$line" | awk '{print $NF}')
            NODES_STR=$(echo "$line" | sed 's/^\s*class\s*//' | sed "s/\s*${CLASS_NAME}$//" | tr -d ' ')
            IFS=',' read -ra NODE_LIST <<< "$NODES_STR"
            for node in "${NODE_LIST[@]}"; do
                [[ -n "$node" ]] && ACTUAL_CLASS[$node]="$CLASS_NAME"
            done
        fi
    done <<< "$MERMAID_CONTENT"

    # Query GitHub for each issue and compute expected class
    while IFS= read -r node; do
        [[ -z "$node" ]] && continue
        ISSUE_NUM=$(echo "$node" | sed 's/^I//')

        # Get issue state and labels from GitHub
        ISSUE_DATA=$(gh issue view "$ISSUE_NUM" --json state,labels 2>/dev/null || true)
        if [[ -z "$ISSUE_DATA" ]]; then
            # Skip if we can't fetch issue data (might not exist or be in different repo)
            continue
        fi

        ISSUE_STATE=$(echo "$ISSUE_DATA" | jq -r '.state')
        HAS_NEEDS_DESIGN=$(echo "$ISSUE_DATA" | jq -r '.labels[]?.name' 2>/dev/null | grep -qE '^needs-design$' && echo "true" || echo "false")

        # Determine expected class
        EXPECTED_CLASS=""
        if [[ "$ISSUE_STATE" == "CLOSED" ]]; then
            EXPECTED_CLASS="done"
        else
            # Check if any blocker is not done
            HAS_OPEN_BLOCKER="false"
            BLOCKER_LIST="${BLOCKERS[$node]:-}"
            for blocker in $BLOCKER_LIST; do
                BLOCKER_CLASS="${ACTUAL_CLASS[$blocker]:-}"
                if [[ "$BLOCKER_CLASS" != "done" ]]; then
                    HAS_OPEN_BLOCKER="true"
                    break
                fi
            done

            if [[ "$HAS_OPEN_BLOCKER" == "true" ]]; then
                EXPECTED_CLASS="blocked"
            elif [[ "$HAS_NEEDS_DESIGN" == "true" ]]; then
                EXPECTED_CLASS="needsDesign"
            else
                EXPECTED_CLASS="ready"
            fi
        fi

        # Build reason string for error context
        REASON=""
        if [[ "$ISSUE_STATE" == "CLOSED" ]]; then
            REASON="(issue is closed)"
        elif [[ "$HAS_OPEN_BLOCKER" == "true" ]]; then
            # Format blocker list as "#123, #456"
            REASON="(blocked by $(echo "$BLOCKER_LIST" | sed 's/I\([0-9]*\)/#\1/g; s/ /, /g'))"
        elif [[ "$HAS_NEEDS_DESIGN" == "true" ]]; then
            REASON="(is not blocked and has 'needs-design' label)"
        else
            REASON="(issue is open, no blocking dependencies)"
        fi

        # Compare with actual class
        ACTUAL="${ACTUAL_CLASS[$node]:-}"
        if [[ -n "$EXPECTED_CLASS" && -n "$ACTUAL" && "$ACTUAL" != "$EXPECTED_CLASS" ]]; then
            emit_fail "MM15: Node $node has class '$ACTUAL' but issue #$ISSUE_NUM requires '$EXPECTED_CLASS' $REASON. See: .github/scripts/docs/MM15.md"
            FAILED=1
        fi
    done <<< "$DIAGRAM_NODES"
fi

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
