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

# Guard: if no results exist for this recipe, skip
if [ -z "$PASSED" ] && [ -z "$FAILED" ]; then
  echo "WARNING: No validation results for $RECIPE_NAME, skipping constraint derivation"
  exit 0
fi

# Check if all darwin platforms failed (no darwin in passed list)
ALL_DARWIN_FAILED=true
HAS_DARWIN_FAILURE=false
for p in $PASSED; do
  case "$p" in darwin-*) ALL_DARWIN_FAILED=false ;; esac
done
for p in $FAILED; do
  case "$p" in darwin-*) HAS_DARWIN_FAILURE=true ;; esac
done
# Only claim "all darwin failed" if darwin was actually tested and failed
if [ "$HAS_DARWIN_FAILURE" = "false" ]; then
  ALL_DARWIN_FAILED=false
fi

# Check if all linux platforms failed (no linux in passed list)
ALL_LINUX_FAILED=true
HAS_LINUX_FAILURE=false
for p in $PASSED; do
  case "$p" in linux-*) ALL_LINUX_FAILED=false ;; esac
done
for p in $FAILED; do
  case "$p" in linux-*) HAS_LINUX_FAILURE=true ;; esac
done
if [ "$HAS_LINUX_FAILURE" = "false" ]; then
  ALL_LINUX_FAILED=false
fi

# Check if only musl (alpine) platforms failed
ONLY_MUSL_FAILED=true
HAS_MUSL_FAILURE=false
HAS_GLIBC_PASS=false
for p in $FAILED; do
  case "$p" in
    *-musl-*) HAS_MUSL_FAILURE=true ;;
    *) ONLY_MUSL_FAILED=false ;;
  esac
done
for p in $PASSED; do
  case "$p" in *-glibc-*) HAS_GLIBC_PASS=true ;; esac
done
# Only set ONLY_MUSL_FAILED if musl actually failed AND at least one glibc passed
if [ "$HAS_MUSL_FAILURE" = "false" ] || [ "$HAS_GLIBC_PASS" = "false" ]; then
  ONLY_MUSL_FAILED=false
fi

# Determine constraint to write
CONSTRAINT_LINE=""

if [ "$ALL_DARWIN_FAILED" = "true" ]; then
  CONSTRAINT_LINE='supported_os = ["linux"]'
  echo "Constraint: $CONSTRAINT_LINE"
elif [ "$ALL_LINUX_FAILED" = "true" ]; then
  CONSTRAINT_LINE='supported_os = ["darwin"]'
  echo "Constraint: $CONSTRAINT_LINE"
elif [ "$ONLY_MUSL_FAILED" = "true" ]; then
  CONSTRAINT_LINE='supported_libc = ["glibc"]'
  echo "Constraint: $CONSTRAINT_LINE"
else
  # Complex failure pattern: collect specific unsupported platforms
  UNSUPPORTED=""

  # Check for arch-level failures first (broader constraint)
  ALL_ARM64_LINUX_FAILED=true
  HAS_ARM64_TESTED=false
  for p in $PASSED; do
    case "$p" in linux-*-arm64) ALL_ARM64_LINUX_FAILED=false ;; esac
  done
  for p in $FAILED; do
    case "$p" in linux-*-arm64) HAS_ARM64_TESTED=true ;; esac
  done
  if [ "$ALL_ARM64_LINUX_FAILED" = "true" ] && [ "$HAS_ARM64_TESTED" = "true" ]; then
    UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/arm64\""
  fi

  # Check for family-level failures (both arches of same family failed)
  # Skip families already covered by arch-level constraint
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
      # If arm64 is already fully excluded, only add if x86 also fails
      if [ "$ALL_ARM64_LINUX_FAILED" = "true" ] && [ "$HAS_ARM64_TESTED" = "true" ]; then
        # arm64 already covered; only flag family if x86_64 also fails
        UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/$family\""
      else
        UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/$family\""
      fi
    elif [ "$x86_fail" = "true" ]; then
      # Only x86_64 failed for this family
      UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/${family}/amd64\""
    elif [ "$arm_fail" = "true" ] && [ "$ALL_ARM64_LINUX_FAILED" = "false" ]; then
      # Only arm64 failed for this family (and arm64 not already fully excluded)
      UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"linux/${family}/arm64\""
    fi
  done

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
      # Normalize x86_64 to amd64
      if [ "$arch" = "x86_64" ]; then arch="amd64"; fi
      UNSUPPORTED="${UNSUPPORTED:+$UNSUPPORTED, }\"${os}/${arch}\""
    done
    CONSTRAINT_LINE="unsupported_platforms = [$UNSUPPORTED]"
  fi
fi

# Write constraint into [metadata] section (after the last metadata field, before [[steps]])
if [ -n "$CONSTRAINT_LINE" ]; then
  NEXT_SECTION=$(grep -n '^\[\[steps\]\]\|^\[version\]\|^\[verify\]' "$RECIPE_FILE" | head -1 | cut -d: -f1)

  if [ -n "$NEXT_SECTION" ]; then
    INSERT_AT=$((NEXT_SECTION - 1))
    while [ "$INSERT_AT" -gt 0 ] && sed -n "${INSERT_AT}p" "$RECIPE_FILE" | grep -q '^$'; do
      INSERT_AT=$((INSERT_AT - 1))
    done
    sed -i "${INSERT_AT}a\\${CONSTRAINT_LINE}" "$RECIPE_FILE"
    echo "Written to $RECIPE_FILE after line $INSERT_AT"
  else
    echo "$CONSTRAINT_LINE" >> "$RECIPE_FILE"
    echo "Appended to $RECIPE_FILE"
  fi
fi
