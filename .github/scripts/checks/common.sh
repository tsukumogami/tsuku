#!/usr/bin/env bash
#
# common.sh - Shared utilities for design document validation checks
#
# This file provides the interface contract that all check scripts must follow.
#
# Interface Contract:
#   Input:  Single argument - path to design document
#   Output: [PASS] message to stdout for passing checks
#           [FAIL] message to stderr for failing checks
#   Exit:   0 = all checks passed
#           1 = one or more checks failed
#           2 = operational error (file not found, invalid args)
#
# Required Exports:
#   - extract_frontmatter <doc-path>  : Outputs frontmatter content (between --- delimiters)
#   - emit_pass <message>             : Outputs [PASS] message to stdout
#   - emit_fail <message>             : Outputs [FAIL] message to stderr
#   - EXIT_PASS, EXIT_FAIL, EXIT_ERROR: Exit code constants
#
# Usage in check scripts:
#   source "$(dirname "$0")/common.sh"
#   # ... validation logic ...
#   emit_pass "Frontmatter valid"
#   exit $EXIT_PASS

set -euo pipefail

# Exit code constants
readonly EXIT_PASS=0
readonly EXIT_FAIL=1
readonly EXIT_ERROR=2

# emit_pass - Output a passing check message
# Usage: emit_pass "Check passed"
emit_pass() {
    echo "[PASS] $1"
}

# emit_fail - Output a failing check message
# Usage: emit_fail "Check failed: reason"
emit_fail() {
    echo "[FAIL] $1" >&2
}

# extract_frontmatter - Extract YAML frontmatter from a document
# Usage: content=$(extract_frontmatter "/path/to/doc.md")
# Returns: Frontmatter content (lines between --- delimiters, excluding delimiters)
# Exit: 0 if frontmatter found, 1 if not found
extract_frontmatter() {
    local doc_path="$1"

    # Check file exists
    if [[ ! -f "$doc_path" ]]; then
        return 1
    fi

    # Check first line is ---
    local first_line
    first_line=$(head -1 "$doc_path")
    if [[ "$first_line" != "---" ]]; then
        return 1
    fi

    # Extract content between first --- and second ---
    # Use awk to get lines between first and second ---
    awk '
        NR == 1 && /^---$/ { in_frontmatter = 1; next }
        in_frontmatter && /^---$/ { exit }
        in_frontmatter { print }
    ' "$doc_path"
}

# has_frontmatter - Check if document has valid frontmatter structure
# Usage: if has_frontmatter "/path/to/doc.md"; then ...
# Returns: 0 if frontmatter exists, 1 if not
has_frontmatter() {
    local doc_path="$1"

    # Check file exists
    if [[ ! -f "$doc_path" ]]; then
        return 1
    fi

    # Check first line is ---
    local first_line
    first_line=$(head -1 "$doc_path")
    if [[ "$first_line" != "---" ]]; then
        return 1
    fi

    # Check for closing ---
    local has_closing
    has_closing=$(awk 'NR > 1 && /^---$/ { found=1; exit } END { print found+0 }' "$doc_path")

    [[ "$has_closing" -eq 1 ]]
}
