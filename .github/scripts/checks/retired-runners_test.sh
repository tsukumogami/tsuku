#!/usr/bin/env bash
set -uo pipefail

# Proves retired-runners.sh can fail, and fails for the reason claimed.
#
# Each case builds a one-file workflow tree in a scratch directory, runs the check from
# there, and asserts both the exit code and the text. The cases cover the shapes a runner
# image takes outside a `runs-on:` line -- matrix values under `runner:` and `os:`, and
# JSON strings -- because the check once matched only `runs-on:` and was blind to two of
# the six places macos-14 was pinned. A case whose scratch workflow could not be written
# VOIDS rather than running against nothing.

CHECK="$(cd "$(dirname "$0")" && pwd)/retired-runners.sh"
SCRATCH=$(mktemp -d)
trap 'rm -rf "$SCRATCH"' EXIT

pass=0; fail=0; void=0

report() {  # report <outcome> <name> [detail]
  case "$1" in
    PASS) pass=$((pass + 1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail + 1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
    VOID) void=$((void + 1)); echo "  [VOID] $2${3:+ -- $3}" >&2 ;;
  esac
}

# assert_case <name> <expected-exit> <expected-substring> <workflow content | EMPTY>
assert_case() {
  local name="$1" want_exit="$2" want_text="$3" content="$4"
  local dir="$SCRATCH/$name" out rc
  mkdir -p "$dir/.github/workflows"
  if [ "$content" != EMPTY ]; then
    printf '%s\n' "$content" > "$dir/.github/workflows/w.yml"
    if [ ! -s "$dir/.github/workflows/w.yml" ]; then
      report VOID "$name" "scratch workflow was not written; premise does not hold"
      return
    fi
  fi
  out=$(cd "$dir" && bash "$CHECK" 2>&1); rc=$?
  if [ "$rc" -ne "$want_exit" ]; then
    report FAIL "$name" "exit $rc, wanted $want_exit"
    return
  fi
  if [ -n "$want_text" ] && ! printf '%s' "$out" | grep -qF -- "$want_text"; then
    report FAIL "$name" "output did not contain: $want_text"
    return
  fi
  report PASS "$name"
}

echo "retired-runners self-test:"

assert_case "no workflow files is an error, not a clean result" 2 "nothing was checked" EMPTY
assert_case "runs-on names a retired image" 1 "line=3::Retired runner 'macos-14'" \
  $'jobs:\n  a:\n    runs-on: macos-14'
assert_case "matrix runner: value names a retired image" 1 "Retired runner 'macos-14'" \
  $'        include:\n          - { runner: macos-14, os: darwin }'
assert_case "matrix os: value names a retired image" 1 "Retired runner 'macos-14'" \
  $'          - os: macos-14'
assert_case "JSON string names a retired image" 1 "Retired runner 'macos-14'" \
  $'  M="{\\"os\\":\\"macos-14\\"}"'
assert_case "dotted image name matches" 1 "Retired runner 'ubuntu-20.04'" \
  $'    runs-on: ubuntu-20.04'
assert_case "a comment mentioning a retired image is not a use" 0 "No retired runners" \
  $'    # moved off macos-14\n    runs-on: macos-15'
assert_case "a current image is clean" 0 "No retired runners" \
  $'    runs-on: macos-15'
assert_case "a longer name with a retired prefix is not a match" 0 "No retired runners" \
  $'    runs-on: macos-140'
assert_case "the dot in a retired name is literal" 0 "No retired runners" \
  $'    runs-on: ubuntu-20x04'

echo "retired-runners self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
