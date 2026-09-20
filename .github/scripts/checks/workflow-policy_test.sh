#!/usr/bin/env bash
set -uo pipefail

# Proves workflow-policy.py can fail, and fails for the reason claimed.
#
# Every mutating case hashes its target before and after and VOIDS itself if nothing
# changed, so a row cannot pass by not running.

CHECK="$(dirname "$0")/workflow-policy.py"
REPO_ROOT="$(git rev-parse --show-toplevel)"
WORKFLOWS="$REPO_ROOT/.github/workflows"
REGISTRY="$REPO_ROOT/.github/escalation-registry.yml"

pass=0; fail=0; void=0
report() {
  case "$1" in
    PASS) pass=$((pass+1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail+1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
    VOID) void=$((void+1)); echo "  [VOID] $2${3:+ -- $3}" >&2 ;;
  esac
}

hash_of() { find "$1" -type f -exec sha256sum {} \; 2>/dev/null | sort | sha256sum; }

echo "Control"
out=$(python3 "$CHECK" --registry "$REGISTRY" --workflows "$WORKFLOWS" 2>&1); rc=$?
if [ $rc -eq 0 ]; then report PASS "working tree passes"; else report FAIL "working tree passes" "exit $rc: $out"; fi

echo "Mutation cases"

# run_mutation <name> <want-exit> <want-text> <mutator>
# The mutator edits the sandbox copy; $1 is the sandbox root.
run_mutation() {
  local name="$1" want_exit="$2" want_text="$3" mutator="$4"
  local sandbox before after out rc
  sandbox=$(mktemp -d)
  cp -r "$WORKFLOWS" "$sandbox/workflows"
  cp "$REGISTRY" "$sandbox/registry.yml"

  before=$(hash_of "$sandbox")
  "$mutator" "$sandbox"
  after=$(hash_of "$sandbox")
  if [ "$before" = "$after" ]; then
    report VOID "$name" "mutation changed nothing"
    rm -rf "$sandbox"; return
  fi

  out=$(python3 "$CHECK" --registry "$sandbox/registry.yml" --workflows "$sandbox/workflows" 2>&1); rc=$?
  if [ "$rc" -ne "$want_exit" ]; then
    report FAIL "$name" "exit $rc, wanted $want_exit"
  elif [ -n "$want_text" ] && ! printf '%s' "$out" | grep -qF -- "$want_text"; then
    report FAIL "$name" "output lacked: $want_text"
  else
    report PASS "$name"
  fi
  rm -rf "$sandbox"
}

drop_declaration() { sed -i '/^# escalation-policy:/d' "$1/workflows/seed-queue.yml"; }
policy_none_no_reason() { sed -i 's/^# escalation-policy: issue/# escalation-policy: none/' "$1/workflows/seed-queue.yml"; }
drop_assignee() { sed -i '/^# escalation-assignee:/d' "$1/workflows/seed-queue.yml"; }
drop_coverage() { sed -i '/^# coverage: items/d' "$1/workflows/seed-queue.yml"; }
drop_only_on() { sed -i '/^# escalation-only-on: schedule/d' "$1/workflows/test.yml"; }
unregister_one() { sed -i '/^  - "Seed Queue"$/d' "$1/registry.yml"; }
register_a_ghost() { printf '  - "A Workflow That Does Not Exist"\n' >> "$1/registry.yml"; }
empty_registry() { : > "$1/registry.yml"; }
registry_without_key() { printf 'something_else: []\n' > "$1/registry.yml"; }

run_mutation "a workflow with no escalation-policy is reported" \
  1 "no \`escalation-policy:\` declared" drop_declaration

run_mutation "policy none without a reason is reported" \
  1 "has to be said out loud" policy_none_no_reason

run_mutation "policy issue without an assignee is reported" \
  1 "no \`escalation-assignee:\`" drop_assignee

run_mutation "a missing coverage declaration is reported" \
  1 "no \`coverage:\` declared" drop_coverage

run_mutation "a dual-trigger workflow missing escalation-only-on is reported" \
  1 "escalation-only-on: schedule" drop_only_on

# Both directions of the registry comparison. Each catches a different mistake and
# neither alone is sufficient, so both are pinned.
run_mutation "declared but not registered is reported" \
  1 "is not in" unregister_one

run_mutation "registered but not declared is reported" \
  1 "A Workflow That Does Not Exist" register_a_ghost

run_mutation "an empty registry is an operational error, not a pass" \
  2 "declares no workflows" empty_registry

run_mutation "a registry without the registered key is an operational error" \
  2 "declares no workflows" registry_without_key

echo "Floor and pin"

# Zero-floor: point it at a directory with no workflows at all.
empty_dir=$(mktemp -d); mkdir -p "$empty_dir/workflows"
out=$(python3 "$CHECK" --registry "$REGISTRY" --workflows "$empty_dir/workflows" 2>&1); rc=$?
if [ $rc -eq 2 ] && printf '%s' "$out" | grep -qF "examined nothing"; then
  report PASS "an empty workflow directory fails the zero-floor"
else
  report FAIL "an empty workflow directory fails the zero-floor" "exit $rc"
fi
rm -rf "$empty_dir"

# The pinned count is a real assertion, not decoration.
out=$(python3 "$CHECK" --registry "$REGISTRY" --workflows "$WORKFLOWS" --expect-scheduled 22 2>&1); rc=$?
if [ $rc -eq 1 ] && printf '%s' "$out" | grep -qF "expected 22"; then
  report PASS "a count differing from the pin is reported"
else
  report FAIL "a count differing from the pin is reported" "exit $rc"
fi

echo "Ref isolation"

# --ref must read the ref, not the working tree. Mutate a tracked workflow, then assert a
# --ref run is unaffected while a working-tree run is. This is what stops a pull request
# registering itself by editing a comment in its own diff.
target="$WORKFLOWS/seed-queue.yml"
if git -C "$REPO_ROOT" diff --quiet -- "$target" && git -C "$REPO_ROOT" ls-files --error-unmatch "$target" >/dev/null 2>&1; then
  before=$(sha256sum "$target" | cut -d' ' -f1)
  sed -i '/^# escalation-policy:/d' "$target"
  after=$(sha256sum "$target" | cut -d' ' -f1)
  if [ "$before" = "$after" ]; then
    report VOID "--ref reads the ref, not the working tree" "mutation changed nothing"
  else
    python3 "$CHECK" --registry "$REGISTRY" --workflows "$WORKFLOWS" >/dev/null 2>&1
    tree_rc=$?
    python3 "$CHECK" --registry "$REGISTRY" --workflows ".github/workflows" --ref HEAD >/dev/null 2>&1
    ref_rc=$?
    if [ $tree_rc -ne 0 ] && [ $ref_rc -eq 0 ]; then
      report PASS "--ref reads the ref, not the working tree"
    else
      report FAIL "--ref reads the ref, not the working tree" "tree=$tree_rc ref=$ref_rc (wanted non-zero, 0)"
    fi
  fi
  git -C "$REPO_ROOT" checkout -- "$target"
else
  report VOID "--ref reads the ref, not the working tree" "target not clean/tracked; cannot mutate safely"
fi

echo
echo "workflow-policy self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
