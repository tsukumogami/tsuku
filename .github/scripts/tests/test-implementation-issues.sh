#!/usr/bin/env bash
#
# test-implementation-issues.sh - Tests for implementation-issues.sh
#
# Tests the skip filter fix, II07, and II08 checks.
# Creates temporary design doc files and runs the check script against them.
#
# Usage:
#   ./test-implementation-issues.sh
#
# Exit codes:
#   0 - All tests passed
#   1 - One or more tests failed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK_SCRIPT="$SCRIPT_DIR/../checks/implementation-issues.sh"
EXTRACT_SCRIPT="$SCRIPT_DIR/../extract-design-issues.sh"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Temp directory for test files
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Create a test doc with given Implementation Issues table content
# The doc is created in a git-tracked temp location to support get-file-creation-commit.sh
# but since these are tmpfiles outside git, II07 grandfathering won't work via git.
# Instead we use a workaround: set II07_CUTOFF_OVERRIDE env var (see below).
create_test_doc() {
    local name="$1"
    local status="$2"
    local table_content="$3"
    local path="$TMPDIR/DESIGN-${name}.md"

    cat > "$path" <<EOF
---
status: $status
---

# DESIGN: Test $name

## Status

$status

## Implementation Issues

### Milestone: [test-milestone](https://github.com/org/repo/milestone/1)

$table_content

### Dependency Graph

\`\`\`mermaid
graph TD
    I1["#1"]
\`\`\`
EOF
    echo "$path"
}

# Run a test and check expected outcome
# Usage: run_test "test name" <expected_exit_code> <expected_stderr_pattern> <doc_path>
run_test() {
    local test_name="$1"
    local expected_exit="$2"
    local expected_pattern="$3"
    local doc_path="$4"
    local actual_exit=0
    local output=""

    TESTS_RUN=$((TESTS_RUN + 1))

    # Capture both stdout and stderr
    output=$("$CHECK_SCRIPT" "$doc_path" 2>&1) || actual_exit=$?

    if [[ "$actual_exit" -ne "$expected_exit" ]]; then
        echo "FAIL: $test_name"
        echo "  Expected exit code: $expected_exit"
        echo "  Actual exit code:   $actual_exit"
        echo "  Output: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    if [[ -n "$expected_pattern" ]]; then
        if ! echo "$output" | grep -qE "$expected_pattern"; then
            echo "FAIL: $test_name"
            echo "  Expected pattern: $expected_pattern"
            echo "  Output: $output"
            TESTS_FAILED=$((TESTS_FAILED + 1))
            return 1
        fi
    fi

    echo "PASS: $test_name"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
}

echo "=== Testing skip filter fix (scenario-1, scenario-2) ==="
echo ""

# scenario-1: Struck-through description rows do not trigger false II03 failures
DOC=$(create_test_doc "strikethrough-desc" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#1: feature one](https://github.com/org/repo/issues/1) | None | testable |
| _Implement feature one._ | | |
| ~~[#2: feature two](https://github.com/org/repo/issues/2)~~ | ~~None~~ | ~~testable~~ |
| ~~_Implement feature two._~~ | | |
TABLE
)")
run_test "scenario-1: Struck-through description rows don't trigger II03" 0 "" "$DOC"

# scenario-2: Struck-through child reference rows do not trigger false II03 failures
DOC=$(create_test_doc "strikethrough-child-ref" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#3: design something](https://github.com/org/repo/issues/3)~~ | ~~None~~ | ~~simple~~ |
| ~~^_Child: [DESIGN-something.md](docs/designs/DESIGN-something.md)_~~ | | | |
| ~~_Designs the something feature._~~ | | |
TABLE
)")
run_test "scenario-2: Struck-through child reference rows don't trigger II03" 0 "" "$DOC"

# Additional: non-struck child reference rows are also skipped
DOC=$(create_test_doc "child-ref-nonstruck" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#4: design other](https://github.com/org/repo/issues/4) | None | simple |
| ^_Child: [DESIGN-other.md](docs/designs/DESIGN-other.md)_ | | | |
| _Designs the other feature._ | | |
TABLE
)")
run_test "Non-struck child reference rows don't trigger II03" 0 "" "$DOC"

echo ""
echo "=== Testing II07: description row required (scenario-3, scenario-4) ==="
echo ""

# For II07 testing, we need to bypass the grandfathering since our temp files
# aren't in git. We'll create a wrapper that overrides the cutoff date to a
# far-future date (so nothing is grandfathered) or a far-past date (so everything is).
# Actually, the simplest approach: since get-file-creation-commit.sh will fail for
# files not in git, ii07_grandfathered() will return 1 (non-zero), meaning
# "not grandfathered" -> enforce II07. This works for our test purposes.

# scenario-3: II07 rejects issues missing description rows
DOC=$(create_test_doc "missing-desc" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#5: some feature](https://github.com/org/repo/issues/5) | None | testable |
| [#6: another feature](https://github.com/org/repo/issues/6) | None | testable |
TABLE
)")
run_test "scenario-3: II07 rejects issues missing description rows" 1 "II07" "$DOC"

# scenario-4: II07 passes when all issues have description rows
DOC=$(create_test_doc "all-descs" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#7: feature a](https://github.com/org/repo/issues/7) | None | testable |
| _Implement feature a._ | | |
| [#8: feature b](https://github.com/org/repo/issues/8) | [#7](https://github.com/org/repo/issues/7) | testable |
| _Implement feature b._ | | |
TABLE
)")
run_test "scenario-4: II07 passes when all issues have description rows" 0 "" "$DOC"

echo ""
echo "=== Testing II08: strikethrough consistency (scenario-5, scenario-6) ==="
echo ""

# scenario-5: II08 rejects struck-through issue with non-struck description row
DOC=$(create_test_doc "inconsistent-desc-strike" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#9: done feature](https://github.com/org/repo/issues/9)~~ | ~~None~~ | ~~testable~~ |
| _Description not struck through._ | | |
TABLE
)")
run_test "scenario-5: II08 rejects struck issue with non-struck description" 1 "II08" "$DOC"

# scenario-6: II08 rejects struck-through issue with non-struck child reference row
DOC=$(create_test_doc "inconsistent-child-strike" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#10: done design](https://github.com/org/repo/issues/10)~~ | ~~None~~ | ~~simple~~ |
| ^_Child: [DESIGN-thing.md](docs/designs/DESIGN-thing.md)_ | | | |
| ~~_Description is struck._~~ | | |
TABLE
)")
run_test "scenario-6: II08 rejects struck issue with non-struck child ref" 1 "II08" "$DOC"

# Additional: consistent strikethrough passes
DOC=$(create_test_doc "consistent-strike" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#11: done design](https://github.com/org/repo/issues/11)~~ | ~~None~~ | ~~simple~~ |
| ~~^_Child: [DESIGN-thing.md](docs/designs/DESIGN-thing.md)_~~ | | | |
| ~~_Description is struck._~~ | | |
TABLE
)")
run_test "Consistent strikethrough passes II08" 0 "" "$DOC"

# Additional: non-struck issue with non-struck description passes
DOC=$(create_test_doc "nonstruck-all" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#12: active feature](https://github.com/org/repo/issues/12) | None | testable |
| _Working on this feature._ | | |
TABLE
)")
run_test "Non-struck issue with non-struck description passes" 0 "" "$DOC"

echo ""
echo "=== Testing extract-design-issues.sh skip filter ==="
echo ""

# Test that extract-design-issues.sh correctly skips struck-through description and child ref rows
DOC=$(create_test_doc "extract-filter" "Planned" "$(cat <<'TABLE'
| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#13: active feature](https://github.com/org/repo/issues/13) | None | testable |
| _Active description._ | | |
| ~~[#14: done feature](https://github.com/org/repo/issues/14)~~ | ~~None~~ | ~~simple~~ |
| ~~^_Child: [DESIGN-done.md](path)_~~ | | | |
| ~~_Done description._~~ | | |
TABLE
)")

EXTRACT_OUTPUT=$("$EXTRACT_SCRIPT" "$DOC" 2>&1)
ENTRY_COUNT=$(echo "$EXTRACT_OUTPUT" | jq '.milestones[0].entries | length')
TESTS_RUN=$((TESTS_RUN + 1))
if [[ "$ENTRY_COUNT" -eq 2 ]]; then
    echo "PASS: extract-design-issues.sh returns exactly 2 entries (skips desc/child ref rows)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo "FAIL: extract-design-issues.sh expected 2 entries, got $ENTRY_COUNT"
    echo "  Output: $EXTRACT_OUTPUT"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

echo ""
echo "=== Testing existing checks still pass (no regressions) ==="
echo ""

# Run against the actual design doc that has description rows
REAL_DOC="/home/dgazineu/dev/workspace/tsuku/tsuku-5/private/tools/docs/designs/DESIGN-needs-design-lifecycle.md"
if [[ -f "$REAL_DOC" ]]; then
    run_test "Real design doc (DESIGN-needs-design-lifecycle.md) passes" 0 "" "$REAL_DOC"
fi

echo ""
echo "=== Testing scenario-7: doc files exist ==="
echo ""

TESTS_RUN=$((TESTS_RUN + 1))
if [[ -f "$SCRIPT_DIR/../docs/II07.md" ]] && [[ -f "$SCRIPT_DIR/../docs/II08.md" ]]; then
    echo "PASS: scenario-7: II07.md and II08.md doc files exist"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo "FAIL: scenario-7: II07.md and/or II08.md doc files missing"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

echo ""
echo "==============================="
echo "Tests: $TESTS_RUN | Passed: $TESTS_PASSED | Failed: $TESTS_FAILED"
echo "==============================="

if [[ "$TESTS_FAILED" -gt 0 ]]; then
    exit 1
fi
exit 0
