#!/usr/bin/env bash
#
# validate-design-doc.sh - Validate a design document
#
# Checks:
# 1. File is under docs/designs/ directory
# 2. Filename starts with DESIGN-
# 3. File has YAML frontmatter (starts with ---, has closing ---)
#
# Usage:
#   validate-design-doc.sh <doc-path>
#
# Exit codes:
#   0 - Valid
#   1 - Invalid (failed one or more checks)
#   2 - Operational error (missing argument, file not found)
#
# Example:
#   validate-design-doc.sh docs/designs/DESIGN-foo.md

set -euo pipefail

usage() {
    cat >&2 <<'EOF'
Usage: validate-design-doc.sh <doc-path>

Validates a design document for:
- Location: must be under docs/designs/
- Naming: filename must start with DESIGN-
- Frontmatter: must have YAML frontmatter (--- delimiters)

Exit codes:
  0 - Valid
  1 - Invalid
  2 - Operational error
EOF
    exit 2
}

# Check arguments
if [[ $# -lt 1 ]]; then
    echo "Error: missing required argument <doc-path>" >&2
    usage
fi

DOC_PATH="$1"

# Check file exists
if [[ ! -f "$DOC_PATH" ]]; then
    echo "Error: file not found: $DOC_PATH" >&2
    exit 2
fi

echo "Validating $DOC_PATH..."

FAILED=0

# Check 1: Location - must be under docs/designs/
check_location() {
    local path="$1"
    if [[ "$path" == docs/designs/* ]] || [[ "$path" == ./docs/designs/* ]]; then
        echo "  [PASS] Location: under docs/designs/"
        return 0
    else
        echo "  [FAIL] Location: not under docs/designs/ (got: $path)" >&2
        return 1
    fi
}

# Check 2: Naming - filename must start with DESIGN-
check_naming() {
    local path="$1"
    local filename
    filename=$(basename "$path")
    if [[ "$filename" == DESIGN-* ]]; then
        echo "  [PASS] Naming: starts with DESIGN-"
        return 0
    else
        echo "  [FAIL] Naming: filename must start with DESIGN- (got: $filename)" >&2
        return 1
    fi
}

# Check 3: Frontmatter - must start with --- and have closing ---
check_frontmatter() {
    local path="$1"

    # Read first line
    local first_line
    first_line=$(head -1 "$path")

    if [[ "$first_line" != "---" ]]; then
        echo "  [FAIL] Frontmatter: file must start with ---" >&2
        return 1
    fi

    # Look for closing --- (must be after line 1)
    # Use awk to find if there's a --- after the first line
    local has_closing
    has_closing=$(awk 'NR > 1 && /^---$/ { found=1; exit } END { print found+0 }' "$path")

    if [[ "$has_closing" -eq 1 ]]; then
        echo "  [PASS] Frontmatter: present"
        return 0
    else
        echo "  [FAIL] Frontmatter: missing closing ---" >&2
        return 1
    fi
}

# Run checks
check_location "$DOC_PATH" || FAILED=1
check_naming "$DOC_PATH" || FAILED=1
check_frontmatter "$DOC_PATH" || FAILED=1

if [[ "$FAILED" -eq 0 ]]; then
    echo "Result: VALID"
    exit 0
else
    echo "Result: INVALID"
    exit 1
fi
