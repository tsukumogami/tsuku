#!/usr/bin/env bash
set -euo pipefail

# Derives and writes platform constraints to a recipe TOML based on validation results.
#
# Usage: write-platform-constraints.sh <recipe-file> <recipe-name> <results-json>
#
# The script analyzes which platforms passed/failed and writes the narrowest
# constraint that describes the failure pattern into the [metadata] section.

RECIPE_FILE="$1"
RECIPE_NAME="$2"
RESULTS_JSON="$3"

# Extract passed/failed platform lists
PASSED=$(jq -r --arg r "$RECIPE_NAME" '[.[] | select(.recipe == $r and .status == "pass")] | .[].platform' "$RESULTS_JSON")
FAILED=$(jq -r --arg r "$RECIPE_NAME" '[.[] | select(.recipe == $r and .status == "fail")] | .[].platform' "$RESULTS_JSON")

# Check if all darwin platforms failed
ALL_DARWIN_FAILED=true
for p in $PASSED; do
  case "$p" in darwin-*) ALL_DARWIN_FAILED=false ;; esac
done

# Check if all linux platforms failed
ALL_LINUX_FAILED=true
for p in $PASSED; do
  case "$p" in linux-*) ALL_LINUX_FAILED=false ;; esac
done

# Check if only musl (alpine) platforms failed
ONLY_MUSL_FAILED=true
for p in $FAILED; do
  case "$p" in
    *-musl-*) ;; # musl failure, expected
    *) ONLY_MUSL_FAILED=false ;;
  esac
done
# Verify at least one musl actually failed
HAS_MUSL_FAILURE=false
for p in $FAILED; do
  case "$p" in *-musl-*) HAS_MUSL_FAILURE=true ;; esac
done
if [ "$HAS_MUSL_FAILURE" = "false" ]; then
  ONLY_MUSL_FAILED=false
fi

# Determine constraint to write
CONSTRAINT_LINE=""

if [ "$ALL_DARWIN_FAILED" = "true" ]; then
  # All macOS failed, all Linux passed -> supported_os = ["linux"]
  CONSTRAINT_LINE='supported_os = ["linux"]'
  echo "Constraint: $CONSTRAINT_LINE"
elif [ "$ALL_LINUX_FAILED" = "true" ]; then
  # All Linux failed, all macOS passed -> supported_os = ["darwin"]
  CONSTRAINT_LINE='supported_os = ["darwin"]'
  echo "Constraint: $CONSTRAINT_LINE"
elif [ "$ONLY_MUSL_FAILED" = "true" ]; then
  # Only musl (alpine) failed -> supported_libc = ["glibc"]
  CONSTRAINT_LINE='supported_libc = ["glibc"]'
  echo "Constraint: $CONSTRAINT_LINE"
else
  # Complex failure pattern: collect specific unsupported platforms
  UNSUPPORTED=""

  # Check for family-level failures (both arches of same family failed)
  for family in debian rhel arch suse alpine; do
    x86_fail=false
    arm_fail=false
    for p in $FAILED; do
      case "$p" in
        linux-${family}-*-x86_64) x86_fail=true ;;
        linux-${family}-*-arm64) arm_fail=true ;;
      esac
    done
    if [ "$x86_fail" = "true" ] && [ "$arm_fail" = "true" ]; then
      UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/$family\""
    fi
  done

  # Check for arch-level failures (all families of one arch failed)
  ALL_ARM64_LINUX_FAILED=true
  for p in $PASSED; do
    case "$p" in linux-*-arm64) ALL_ARM64_LINUX_FAILED=false ;; esac
  done
  if [ "$ALL_ARM64_LINUX_FAILED" = "true" ]; then
    # Check if any arm64 linux was tested
    HAS_ARM64=false
    for p in $FAILED; do
      case "$p" in linux-*-arm64) HAS_ARM64=true ;; esac
    done
    if [ "$HAS_ARM64" = "true" ]; then
      UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/arm64\""
    fi
  fi

  # Check darwin individually
  for p in $FAILED; do
    case "$p" in
      darwin-arm64) UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"darwin/arm64\"" ;;
      darwin-x86_64) UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"darwin/amd64\"" ;;
    esac
  done

  if [ -n "$UNSUPPORTED" ]; then
    CONSTRAINT_LINE="unsupported_platforms = [$UNSUPPORTED]"
    echo "Constraint: $CONSTRAINT_LINE"
  else
    echo "WARNING: Could not derive clean constraint for $RECIPE_NAME"
    echo "Failed platforms: $FAILED"
    # Fall back to listing all failed as unsupported
    for p in $FAILED; do
      os=$(echo "$p" | cut -d'-' -f1)
      arch=$(echo "$p" | rev | cut -d'-' -f1 | rev)
      UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"${os}/${arch}\""
    done
    CONSTRAINT_LINE="unsupported_platforms = [$UNSUPPORTED]"
  fi
fi

# Write constraint into [metadata] section (after the last metadata field, before [[steps]])
if [ -n "$CONSTRAINT_LINE" ]; then
  # Find the last non-empty line in [metadata] (before the next section)
  NEXT_SECTION=$(grep -n '^\[\[steps\]\]\|^\[version\]\|^\[verify\]' "$RECIPE_FILE" | head -1 | cut -d: -f1)

  if [ -n "$NEXT_SECTION" ]; then
    # Find the last non-blank line before the next section
    INSERT_AT=$((NEXT_SECTION - 1))
    # Skip blank lines backwards
    while [ "$INSERT_AT" -gt 0 ] && sed -n "${INSERT_AT}p" "$RECIPE_FILE" | grep -q '^$'; do
      INSERT_AT=$((INSERT_AT - 1))
    done
    # Insert after the last metadata field
    sed -i "${INSERT_AT}a\\${CONSTRAINT_LINE}" "$RECIPE_FILE"
    echo "Written to $RECIPE_FILE after line $INSERT_AT"
  else
    echo "$CONSTRAINT_LINE" >> "$RECIPE_FILE"
    echo "Appended to $RECIPE_FILE"
  fi
fi
