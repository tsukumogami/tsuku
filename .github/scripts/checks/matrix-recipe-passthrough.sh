#!/usr/bin/env bash
set -euo pipefail

# Checks that every test in test-matrix.json declaring a `recipe` still carries
# that recipe after the matrix-generation step projects it.
#
# Why this exists: test-matrix.json lets a test name the recipe it should
# install. If the projection in scheduled-tests.yml drops that field, the
# install step resolves the tool from the registry instead. Where a same-named
# registry recipe exists the test passes while exercising a different code
# path, so the run is green and the coverage is imaginary. Only a test whose
# name is absent from the registry fails visibly, which is why this went
# unnoticed (tsukumogami/tsuku#2596).
#
# The check reads the jq programs out of the workflow rather than restating
# them. A copy here could drift from the workflow the same way the workflow
# drifted from the matrix, and then this check would pass while the defect
# returned.

MATRIX="${MATRIX_FILE:-test-matrix.json}"
WORKFLOW="${WORKFLOW_FILE:-.github/workflows/scheduled-tests.yml}"

for f in "$MATRIX" "$WORKFLOW"; do
  if [ ! -f "$f" ]; then
    echo "::error::required file not found: $f" >&2
    exit 2
  fi
done

# The tests known to declare a recipe, pinned. Without this the check has no
# subject of its own: deleting a `recipe` key would simply remove a test from
# the comparison and the check would pass having quietly stopped covering it.
# Pinning costs an edit here whenever a declaring test is added or removed, and
# that edit is the point — it makes the change deliberate and visible in review.
EXPECTED_DECLARING=(
  "cargo_cargo-audit_basic"
  "cpan_ack_with_dependency"
  "gem_bundler_multi_exec"
  "go_gofumpt_with_dependency"
  "npm_netlify-cli_basic"
  "pipx_ruff_basic"
  "tap_waypoint-tap_short_form"
)

ACTUAL_DECLARING=$(jq -r '.tests | to_entries[] | select(.value.recipe != null) | .key' "$MATRIX" | sort)
EXPECTED_SORTED=$(printf '%s\n' "${EXPECTED_DECLARING[@]}" | sort)

if [ -z "$ACTUAL_DECLARING" ]; then
  echo "::error::no test in ${MATRIX} declares a 'recipe'; this check examined nothing" >&2
  exit 1
fi
DECLARING=$(printf '%s\n' "$ACTUAL_DECLARING" | wc -l | tr -d ' ')

MISSING=$(comm -23 <(printf '%s\n' "$EXPECTED_SORTED") <(printf '%s\n' "$ACTUAL_DECLARING"))
ADDED=$(comm -13 <(printf '%s\n' "$EXPECTED_SORTED") <(printf '%s\n' "$ACTUAL_DECLARING"))

SUBJECT_DRIFT=0
if [ -n "$MISSING" ]; then
  while IFS= read -r id; do
    [ -n "$id" ] || continue
    echo "::error file=${MATRIX}::test '${id}' no longer declares a 'recipe'. If that is deliberate, remove it from EXPECTED_DECLARING in this script; otherwise the declaration was dropped." >&2
    SUBJECT_DRIFT=1
  done <<< "$MISSING"
fi
if [ -n "$ADDED" ]; then
  while IFS= read -r id; do
    [ -n "$id" ] || continue
    echo "::error file=${MATRIX}::test '${id}' newly declares a 'recipe' but is not in EXPECTED_DECLARING in this script. Add it so the set stays pinned." >&2
    SUBJECT_DRIFT=1
  done <<< "$ADDED"
fi
if [ "$SUBJECT_DRIFT" -ne 0 ]; then
  exit 1
fi

# Every jq program the workflow uses to build a matrix, taken from the file
# itself so the check cannot drift from what actually runs.
mapfile -t PROGRAMS < <(grep -oE "jq -c '[^']+'" "$WORKFLOW" | sed -E "s/^jq -c '//; s/'$//")
if [ "${#PROGRAMS[@]}" -eq 0 ]; then
  echo "::error::found no jq matrix projection in ${WORKFLOW}; this check examined nothing" >&2
  exit 1
fi

FAILED=0
EXAMINED=0

for prog in "${PROGRAMS[@]}"; do
  PROJECTED=$(jq -c "$prog" "$MATRIX") || {
    echo "::error::could not run a projection from ${WORKFLOW}" >&2
    exit 2
  }

  # For each projected entry whose test declares a recipe, the entry must carry
  # that same recipe. Identity, not count: the counts already match today while
  # the identities differ, which is exactly why counting would not catch this.
  while IFS=$'\t' read -r id declared got; do
    EXAMINED=$((EXAMINED + 1))
    if [ "$declared" != "$got" ]; then
      echo "::error file=${WORKFLOW}::test '${id}' declares recipe '${declared}' but the matrix projection emits '${got}'. The declared recipe is not reaching the install step." >&2
      FAILED=1
    fi
  done < <(
    jq -r --argjson projected "$PROJECTED" '
      ([$projected[] | {(.id): .}] | add // {}) as $byid
      | .tests
      | to_entries[]
      | select(.value.recipe != null)
      | select($byid[.key] != null)
      | [.key, .value.recipe, ($byid[.key].recipe // "<dropped>")]
      | @tsv
    ' "$MATRIX"
  )
done

if [ "$EXAMINED" -eq 0 ]; then
  echo "::error::no declaring test appeared in any matrix projection; this check examined nothing" >&2
  exit 1
fi

# A test that declares a recipe but appears nowhere under `.ci` is never run by
# any workflow, so the recipe it names reaches nothing. Membership counts
# whether a list holds ids as array entries or as object keys: `ci.blocked` is
# a map of id to reason, and an entry parked there is deliberately not run
# rather than forgotten.
ORPHANS=$(jq -r '
  [ .ci | to_entries[]
    | if (.value | type) == "array" then .value[]
      elif (.value | type) == "object" then (.value | keys[])
      else empty end
  ] as $listed
  | .tests
  | to_entries[]
  | select(.value.recipe != null)
  | select([.key] | inside($listed) | not)
  | .key
' "$MATRIX")

if [ -n "$ORPHANS" ]; then
  while IFS= read -r id; do
    [ -n "$id" ] || continue
    echo "::error file=${MATRIX}::test '${id}' declares a recipe but appears in no ci list, so no workflow runs it. Park it under ci.blocked or add it to a list." >&2
    FAILED=1
  done <<< "$ORPHANS"
fi

if [ "$FAILED" -eq 0 ]; then
  echo "Matrix recipe passthrough: ${EXAMINED} declared recipe(s) reach the install step across ${#PROGRAMS[@]} projection(s); ${DECLARING} test(s) declare one."
fi

exit $FAILED
