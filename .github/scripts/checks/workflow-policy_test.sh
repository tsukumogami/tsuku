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

# --- the `none` kinds, and the expiry that makes a deferral a deferral ---

drop_none_kind() { sed -i '/^# escalation-none-kind:/d' "$1/workflows/r2-health-monitor.yml"; }
bogus_none_kind() { sed -i 's/^# escalation-none-kind: deferred$/# escalation-none-kind: maybe/' "$1/workflows/r2-health-monitor.yml"; }
deferred_without_issue() { sed -i '/^# escalation-tracking-issue:/d' "$1/workflows/r2-health-monitor.yml"; }
# #2633 is closed. A deferred `none` pointing at it is an exemption whose condition has
# resolved, which is the state the expiry rule exists to catch.
deferred_on_closed_issue() { sed -i 's/^# escalation-tracking-issue: 2593$/# escalation-tracking-issue: 2633/' "$1/workflows/r2-health-monitor.yml"; }
# A permanent `none` needs no issue: it is not waiting for anything.
make_permanent() {
  sed -i 's/^# escalation-none-kind: deferred$/# escalation-none-kind: permanent/' "$1/workflows/r2-health-monitor.yml"
  sed -i '/^# escalation-tracking-issue:/d' "$1/workflows/r2-health-monitor.yml"
}

run_mutation "a none that does not say which kind it is is reported" \
  1 "escalation-none-kind" drop_none_kind

run_mutation "an unrecognised none kind is reported" \
  1 "is not one of" bogus_none_kind

run_mutation "a deferred none with no tracking issue is reported" \
  1 "naming the issue number" deferred_without_issue

run_mutation "a deferred none whose tracking issue is closed is reported" \
  1 "condition that justified the deferred \`none\` has resolved" deferred_on_closed_issue

run_mutation "a permanent none needs no tracking issue" \
  0 "" make_permanent

run_mutation "a registry without the registered key is an operational error" \
  2 "declares no workflows" registry_without_key

# --- ownership: a failure already tracked by an open issue ---

# No real workflow declares an owner yet, so each case adds one to a sandbox copy. Seed
# Queue is a plain `issue` workflow with no self-filing. #2593 is open (R2 Health
# Monitor's deferral already depends on it); #2633 is closed.
declare_owner() {  # declare_owner <file> <owner value> [reason]
  local line="# escalation-owned-by: $2"
  [ -n "${3:-}" ] && line="$line\n# escalation-owned-by-reason: $3"
  sed -i "s/^# escalation-assignee: \(.*\)$/# escalation-assignee: \1\n$line/" "$1"
}
owner_open() { declare_owner "$1/workflows/seed-queue.yml" 2593 fixture; }
# Ownership held by a closed issue has expired, like a deferral.
owner_closed() { declare_owner "$1/workflows/seed-queue.yml" 2633 fixture; }
owner_malformed() { declare_owner "$1/workflows/seed-queue.yml" soon fixture; }
owner_without_reason() { declare_owner "$1/workflows/seed-queue.yml" 2593; }
# Discovery Registry Freshness declares self-filing. Ownership would redirect only the
# escalator while its own filing step kept running, so the pair is rejected outright.
owner_and_self_files() { declare_owner "$1/workflows/discovery-freshness.yml" 2593 fixture; }
owner_on_none() {
  sed -i 's/^# escalation-policy: none$/# escalation-policy: none\n# escalation-owned-by: 2593\n# escalation-owned-by-reason: fixture/' \
    "$1/workflows/r2-health-monitor.yml"
}

run_mutation "an open owner with a reason passes" \
  0 "" owner_open

run_mutation "an owner that is closed is reported as expired" \
  1 "condition that justified the ownership declaration has resolved" owner_closed

run_mutation "an owner that is not an issue number is reported" \
  1 "must name the number of the open issue" owner_malformed

run_mutation "an owner with no reason is reported" \
  1 "escalation-owned-by-reason" owner_without_reason

run_mutation "an owner alongside self-filing is rejected, not resolved by precedence" \
  1 "declares both \`escalation-owned-by:\` and \`escalation-self-files:\`" owner_and_self_files

run_mutation "an owner on a workflow that escalates nothing is reported" \
  1 "nothing to route to an owner" owner_on_none

echo "The listener's trigger list"

# The rule is: the listener's workflow_run list is the registry MINUS its own name. Not
# equality, and the asymmetry is structural -- naming itself would wake it on its own
# completion, and the design does not depend on knowing whether the platform prevents that.
listener_drops_one() { sed -i '0,/^      - "/{s/^      - "[^"]*"$//}' "$1/workflows/escalate.yml"; }
listener_names_itself() { sed -i 's/^    types: \[completed\]$/      - "Escalate"\n    types: [completed]/' "$1/workflows/escalate.yml"; }
listener_has_a_stranger() { sed -i 's/^    types: \[completed\]$/      - "A Workflow Nobody Registered"\n    types: [completed]/' "$1/workflows/escalate.yml"; }

run_mutation "a registered workflow missing from the listener list is reported" \
  1 "absent from the listener" listener_drops_one

run_mutation "the listener naming itself is reported" \
  1 "names itself" listener_names_itself

run_mutation "a listener entry that is not registered is reported" \
  1 "not in the registry" listener_has_a_stranger

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
out=$(python3 "$CHECK" --registry "$REGISTRY" --workflows "$WORKFLOWS" --expect-scheduled 23 2>&1); rc=$?
if [ $rc -eq 1 ] && printf '%s' "$out" | grep -qF "expected 23"; then
  report PASS "a count differing from the pin is reported"
else
  report FAIL "a count differing from the pin is reported" "exit $rc"
fi

echo "Ref isolation"

# --ref must read the ref, not the working tree. Mutate a tracked workflow, then assert a
# --ref run is unaffected while a working-tree run is. This is what stops a pull request
# registering itself by editing a comment in its own diff.
# Both ref rows compare HEAD against a deliberately mutated working tree, so their premise
# is that the tree otherwise MATCHES HEAD. Any unrelated uncommitted change to a
# declaration makes `--ref HEAD` read a different set than the tree for reasons that have
# nothing to do with the property under test. That must VOID the row rather than fail it:
# a row whose premise does not hold has not tested anything, and reporting failure would
# be as wrong as reporting success.
target="$WORKFLOWS/seed-queue.yml"
if ! git -C "$REPO_ROOT" diff --quiet -- "$WORKFLOWS"; then
  report VOID "--ref reads the ref, not the working tree" "workflows dir has uncommitted changes; premise does not hold"
  report VOID "--ref reads the registry from the ref, not from disk" "workflows dir has uncommitted changes; premise does not hold"
elif git -C "$REPO_ROOT" diff --quiet -- "$target" && git -C "$REPO_ROOT" ls-files --error-unmatch "$target" >/dev/null 2>&1; then
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

# The registry must come from the ref too. Reading declarations from the ref while taking
# the registry off disk would let a pull request register itself by editing the half that
# was still read locally, which is the substitution --ref exists to prevent.
reg="$REPO_ROOT/.github/escalation-registry.yml"
if ! git -C "$REPO_ROOT" diff --quiet -- "$WORKFLOWS"; then
  : # already voided above with the workflow row; do not report twice
elif git -C "$REPO_ROOT" diff --quiet -- "$reg"; then
  before=$(sha256sum "$reg" | cut -d' ' -f1)
  printf '  - "A Workflow Registered Only In The Working Tree"\n' >> "$reg"
  after=$(sha256sum "$reg" | cut -d' ' -f1)
  if [ "$before" = "$after" ]; then
    report VOID "--ref reads the registry from the ref, not from disk" "mutation changed nothing"
  else
    out=$(python3 "$CHECK" --registry "$reg" --workflows "$WORKFLOWS" --ref HEAD 2>&1); rc=$?
    if [ $rc -eq 0 ] && ! printf '%s' "$out" | grep -q 'Working Tree'; then
      report PASS "--ref reads the registry from the ref, not from disk"
    else
      report FAIL "--ref reads the registry from the ref, not from disk" "exit $rc; working-tree entry leaked into a ref run"
    fi
  fi
  git -C "$REPO_ROOT" checkout -- "$reg"
else
  report VOID "--ref reads the registry from the ref, not from disk" "registry not clean; cannot mutate safely"
fi

echo
echo "workflow-policy self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
