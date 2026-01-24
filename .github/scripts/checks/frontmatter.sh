#!/usr/bin/env bash
#
# frontmatter.sh - Validate design document frontmatter
#
# This is a stub implementation for the modular check framework skeleton.
# It validates basic frontmatter presence. Full FM01-FM03 rules are
# implemented in issue #380.
#
# Current checks:
#   - Frontmatter exists (--- delimiters present)
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

# Check: Frontmatter exists
if has_frontmatter "$DOC_PATH"; then
    emit_pass "Frontmatter: present"
else
    emit_fail "Frontmatter: missing or malformed (must start with --- and have closing ---)"
    FAILED=1
fi

# Return appropriate exit code
if [[ "$FAILED" -eq 0 ]]; then
    exit $EXIT_PASS
else
    exit $EXIT_FAIL
fi
