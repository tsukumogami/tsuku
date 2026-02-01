#!/usr/bin/env bash
set -euo pipefail

# Checks workflow files for retired GitHub Actions runner images.
# Add entries to RETIRED_RUNNERS as GitHub retires them.

RETIRED_RUNNERS=(
  "macos-13"
  "macos-12"
  "macos-11"
  "ubuntu-20.04"
  "ubuntu-18.04"
  "windows-2019"
)

FAILED=0

for runner in "${RETIRED_RUNNERS[@]}"; do
  while IFS=: read -r file line content; do
    echo "::error file=${file},line=${line}::Retired runner '${runner}' found. See https://github.com/actions/runner-images for current images."
    FAILED=1
  done < <(grep -rn "runs-on:.*${runner}" .github/workflows/ 2>/dev/null || true)
done

if [ "$FAILED" -eq 0 ]; then
  echo "No retired runners found in workflow files."
fi

exit $FAILED
