#!/usr/bin/env bash
set -euo pipefail

# Checks workflow files for retired GitHub Actions runner images.
# Add entries to RETIRED_RUNNERS as GitHub retires them.
#
# An image name is matched as a whole token anywhere in a workflow, not only on a
# `runs-on:` line. Matrices name their runners in values (`runner: macos-14`,
# `os: macos-14`, JSON strings) and pass them to `runs-on: ${{ matrix.runner }}`, so a
# `runs-on:`-only match can't see them. Comment lines are skipped. The token boundary
# keeps `macos-15` from matching `macos-15-intel`.

WORKFLOWS=.github/workflows

RETIRED_RUNNERS=(
  # GitHub ends support for the macos-14 image on 2026-11-02, and Homebrew has dropped it
  # to its lowest support level (#2613).
  "macos-14"
  "macos-13"
  "macos-12"
  "macos-11"
  "ubuntu-20.04"
  "ubuntu-18.04"
  "windows-2019"
)

# A scan that found no workflow files has checked nothing, and must not report clean.
shopt -s nullglob
files=("$WORKFLOWS"/*.yml "$WORKFLOWS"/*.yaml)
if [ "${#files[@]}" -eq 0 ]; then
  echo "::error::no workflow files found under ${WORKFLOWS}; nothing was checked"
  exit 2
fi

FAILED=0

for runner in "${RETIRED_RUNNERS[@]}"; do
  pattern="(^|[^[:alnum:]_.-])${runner//./\\.}([^[:alnum:]_.-]|$)"
  # grep exits 1 for no match, which is fine, and 2 for an error, which is not: a check
  # that could not read its input has not found the input clean.
  rc=0
  matches=$(grep -HnE "$pattern" "${files[@]}") || rc=$?
  if [ "$rc" -gt 1 ]; then
    echo "::error::grep failed (exit ${rc}) while scanning for '${runner}'"
    exit 2
  fi
  while IFS=: read -r file line content; do
    [ -n "$file" ] || continue
    case "${content#"${content%%[![:space:]]*}"}" in
      '#'*) continue ;;
    esac
    echo "::error file=${file},line=${line}::Retired runner '${runner}' found. See https://github.com/actions/runner-images for current images."
    FAILED=1
  done <<< "$matches"
done

if [ "$FAILED" -eq 0 ]; then
  echo "No retired runners found in ${#files[@]} workflow files."
fi

exit $FAILED
