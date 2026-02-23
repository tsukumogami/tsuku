#!/usr/bin/env bash
set -euo pipefail

# Checks workflow files for CI anti-patterns that the consolidation patterns
# are designed to prevent. See docs/ci-patterns.md for the standard patterns.
#
# What this checks:
#   1. Any for/while loop that runs tsuku install should have ::group:: markers
#   2. Such loops should collect failures (FAILED+=, file-based, or equivalent)
#
# What this intentionally ignores:
#   - Loops that don't run tsuku install (validation, file processing, etc.)
#   - Loops inside docker run commands (container manages its own output)
#   - strategy.matrix usage (legitimate for genuinely different runner types)
#   - Loops in non-workflow scripts (only scans .github/workflows/*.yml)
#
# The heuristic is deliberately narrow: it only fires on loops that directly
# contain a tsuku install command. Other loops have their own error handling
# conventions and aren't test serialization.

WORKFLOW_DIR=".github/workflows"
FAILED=0

if [ ! -d "$WORKFLOW_DIR" ]; then
  echo "::error::Workflow directory not found: $WORKFLOW_DIR"
  exit 2
fi

# Extract multi-line run: blocks from workflow files and check them.
check_loops_have_groups() {
  local file="$1"
  local in_run_block=0
  local run_indent=0
  local run_block=""
  local run_start_line=0
  local line_num=0

  while IFS= read -r line; do
    line_num=$((line_num + 1))

    # Detect start of a run: | block
    if echo "$line" | grep -qE '^\s+run:\s*\|'; then
      in_run_block=1
      run_indent=$(echo "$line" | sed 's/[^ ].*//' | wc -c)
      run_block=""
      run_start_line=$line_num
      continue
    fi

    if [ "$in_run_block" -eq 1 ]; then
      local stripped_len
      stripped_len=$(echo "$line" | sed 's/[^ ].*//' | wc -c)

      if [ -z "$(echo "$line" | tr -d '[:space:]')" ] || [ "$stripped_len" -gt "$run_indent" ]; then
        run_block="${run_block}${line}"$'\n'
      else
        if [ -n "$run_block" ]; then
          check_run_block "$file" "$run_start_line" "$run_block"
        fi
        in_run_block=0
        run_block=""
      fi
    fi
  done < "$file"

  # Check final block if file ends inside a run: block
  if [ "$in_run_block" -eq 1 ] && [ -n "$run_block" ]; then
    check_run_block "$file" "$run_start_line" "$run_block"
  fi
}

check_run_block() {
  local file="$1"
  local start_line="$2"
  local block="$3"

  # Does this block contain a for or while loop?
  if ! echo "$block" | grep -qE '^\s*(for |while )'; then
    return
  fi

  # Only flag loops that directly run tsuku install -- the specific pattern
  # that should use the consolidation approach.
  if ! echo "$block" | grep -qE '\./tsuku install|tsuku install'; then
    return
  fi

  # Skip blocks where tsuku install runs inside docker run -- the container
  # manages its own output and failure reporting.
  if echo "$block" | grep -qE 'docker run'; then
    return
  fi

  # Check 1: Loop should have ::group:: markers
  if ! echo "$block" | grep -q '::group::'; then
    echo "::error file=${file},line=${start_line}::Loop with 'tsuku install' lacks ::group::/::endgroup:: markers. See docs/ci-patterns.md"
    FAILED=1
  fi

  # Check 2: Loop should have failure collection.
  # Accept multiple patterns:
  #   - Bash array:  FAILED+=
  #   - File-based:  >> "$FAIL_FILE"  or  >> /tmp/fail
  #   - String:      FAILED="$FAILED
  local has_failure_collection=0
  if echo "$block" | grep -qEi '(FAILED|FAILURES|FAIL)\+='; then
    has_failure_collection=1
  elif echo "$block" | grep -qE '>>\s*"\$FAIL'; then
    has_failure_collection=1
  elif echo "$block" | grep -qE '>>\s*/tmp/fail'; then
    has_failure_collection=1
  elif echo "$block" | grep -qE '>>\s*"\$\{'; then
    # >> "${runner.temp}/failed-..." pattern
    has_failure_collection=1
  fi

  if [ "$has_failure_collection" -eq 0 ]; then
    echo "::error file=${file},line=${start_line}::Loop with 'tsuku install' lacks failure collection (FAILED+= or file-based). See docs/ci-patterns.md"
    FAILED=1
  fi
}

for workflow in "$WORKFLOW_DIR"/*.yml; do
  check_loops_have_groups "$workflow"
done

if [ "$FAILED" -eq 0 ]; then
  echo "All workflow files follow CI consolidation patterns."
fi

exit $FAILED
