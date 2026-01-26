#!/usr/bin/env bash
# shellcheck disable=SC2310,SC2312
#
# validate-design-doc.sh - Validate a design document
#
# This is the orchestrator for modular design document validation.
# It runs location/naming checks inline and delegates category-specific
# checks to scripts in the checks/ directory.
#
# Check categories:
#   - frontmatter.sh  : Frontmatter validation
#   - sections.sh     : Required sections (future)
#   - status-directory.sh : Status/directory alignment (future)
#   - implementation-issues.sh : Issues table format (future)
#   - mermaid.sh      : Diagram syntax (future)
#
# Usage:
#   validate-design-doc.sh <doc-path>
#
# Exit codes:
#   0 - Valid (all checks passed)
#   1 - Invalid (one or more checks failed)
#   2 - Operational error (missing argument, file not found)
#
# Example:
#   validate-design-doc.sh docs/designs/DESIGN-foo.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECKS_DIR="$SCRIPT_DIR/checks"

# Source common utilities
source "$CHECKS_DIR/common.sh"

usage() {
    cat >&2 <<'EOF'
Usage: validate-design-doc.sh <doc-path>

Validates a design document for:
- Location: must be under docs/designs/
- Naming: filename must start with DESIGN-
- Frontmatter: must have valid YAML frontmatter

Exit codes:
  0 - Valid
  1 - Invalid
  2 - Operational error
EOF
    exit $EXIT_ERROR
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
    exit $EXIT_ERROR
fi

echo "Validating $DOC_PATH..."

FAILED=0

# Inline check: Location - must be under docs/designs/
check_location() {
    local path="$1"
    if [[ "$path" == docs/designs/* ]] || [[ "$path" == ./docs/designs/* ]]; then
        emit_pass "Location: under docs/designs/"
        return 0
    else
        emit_fail "Location: not under docs/designs/ (got: $path)"
        return 1
    fi
}

# Inline check: Naming - filename must start with DESIGN-
check_naming() {
    local path="$1"
    local filename
    filename=$(basename "$path")
    if [[ "$filename" == DESIGN-* ]]; then
        emit_pass "Naming: starts with DESIGN-"
        return 0
    else
        emit_fail "Naming: filename must start with DESIGN- (got: $filename)"
        return 1
    fi
}

# Run inline checks
check_location "$DOC_PATH" || FAILED=1
check_naming "$DOC_PATH" || FAILED=1

# Run modular checks from checks/ directory
# Each check script is called as a subprocess
run_check() {
    local check_script="$1"
    local check_name
    check_name=$(basename "$check_script" .sh)

    if [[ -x "$check_script" ]]; then
        # Run check and capture output
        # Check scripts output [PASS]/[FAIL] messages
        if ! "$check_script" "$DOC_PATH"; then
            return 1
        fi
    fi
    return 0
}

# Run frontmatter check
if [[ -x "$CHECKS_DIR/frontmatter.sh" ]]; then
    run_check "$CHECKS_DIR/frontmatter.sh" || FAILED=1
fi

# Run sections check
if [[ -x "$CHECKS_DIR/sections.sh" ]]; then
    run_check "$CHECKS_DIR/sections.sh" || FAILED=1
fi

# Run status-directory check
if [[ -x "$CHECKS_DIR/status-directory.sh" ]]; then
    run_check "$CHECKS_DIR/status-directory.sh" || FAILED=1
fi

# Future: Run all check scripts dynamically
# for check_script in "$CHECKS_DIR"/*.sh; do
#     [[ "$check_script" == */common.sh ]] && continue
#     run_check "$check_script" || FAILED=1
# done

# Report final result
if [[ "$FAILED" -eq 0 ]]; then
    echo "Result: VALID"
    exit $EXIT_PASS
else
    echo "Result: INVALID"
    exit $EXIT_FAIL
fi
