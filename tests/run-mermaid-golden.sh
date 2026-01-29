#!/usr/bin/env bash
#
# run-mermaid-golden.sh - Run golden file tests for mermaid diagram validation
#
# Tests that mermaid.sh produces expected pass/fail results for known fixtures.
#
# Usage:
#   tests/run-mermaid-golden.sh
#
# Exit codes:
#   0 - All golden tests passed
#   1 - One or more golden tests failed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MERMAID_CHECK="$REPO_ROOT/.github/scripts/checks/mermaid.sh"
FIXTURE_DIR="$SCRIPT_DIR/fixtures/validation"

PASSED=0
FAILED=0
TOTAL=0

run_test() {
    local fixture="$1"
    local expected_exit="$2"
    local description="$3"
    TOTAL=$((TOTAL + 1))

    local actual_exit=0
    "$MERMAID_CHECK" --skip-status-check "$fixture" >/dev/null 2>&1 || actual_exit=$?

    if [[ "$actual_exit" -eq "$expected_exit" ]]; then
        echo "[PASS] $description (exit $actual_exit)"
        PASSED=$((PASSED + 1))
    else
        echo "[FAIL] $description: expected exit $expected_exit, got $actual_exit" >&2
        FAILED=$((FAILED + 1))
    fi
}

echo "Running mermaid golden file tests..."
echo ""

run_test "$FIXTURE_DIR/mermaid-issues-only.md" 0 "Issues-only diagram passes validation"
run_test "$FIXTURE_DIR/mermaid-milestones-only.md" 0 "Milestones-only diagram passes validation"
run_test "$FIXTURE_DIR/mermaid-mixed.md" 0 "Mixed issue+milestone diagram passes validation"
run_test "$FIXTURE_DIR/mermaid-invalid-node.md" 1 "Invalid node name fails MM10 validation"

echo ""
echo "Results: $PASSED passed, $FAILED failed, $TOTAL total"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
