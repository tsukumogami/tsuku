#!/usr/bin/env bash
set -uo pipefail

# Regression cases for `escalation-owned-by`, through the listener path (escalate.sh plus
# the listener's own receipt step) and through the sweeper.
#
# Every owned case asserts WHERE THE RUN WENT, not only that nothing new was filed. "No
# new issue" is also what an escalator that does nothing at all produces, so a test that
# stops there passes against the defect it exists to catch. Here a routed run must show a
# comment on the owner, an outcome naming the owner, and a receipt carrying both.
#
# `gh` is replaced on PATH by a stub that records every call and answers from fixtures,
# which is what makes an open owner, a closed owner and an unreadable owner constructible.

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
ESCALATE="$REPO_ROOT/.github/scripts/escalate.sh"
SWEEP="$REPO_ROOT/.github/scripts/escalate-sweep.sh"
LISTENER="$REPO_ROOT/.github/workflows/escalate.yml"
OWNER=4242

pass=0; fail=0
report() {
  case "$1" in
    PASS) pass=$((pass+1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail+1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
  esac
}

# make_sandbox <owner state: OPEN|CLOSED|UNREADABLE> <latest conclusion for the sweep gate>
make_sandbox() {
  local state="$1" latest="${2:-failure}"
  local s; s=$(mktemp -d)
  mkdir -p "$s/bin" "$s/workflows"
  : > "$s/calls.log"

  cat > "$s/workflows/demo.yml" <<YML
# escalation-policy: issue
# escalation-assignee: someone
# escalation-owned-by: ${OWNER}
# escalation-owned-by-reason: fixture
# coverage: items
name: Demo Workflow
on:
  schedule:
    - cron: '0 * * * *'
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: true
YML
  printf 'registered:\n  - "Demo Workflow"\n' > "$s/registry.yml"

  cat > "$s/bin/gh" <<STUB
#!/usr/bin/env bash
args="\$*"
printf '%s\n' "\$args" >> "$s/calls.log"
case "\$args" in
  "issue view ${OWNER} --json state"*)
      [ "${state}" = "UNREADABLE" ] && exit 1
      printf '%s\n' "${state}" ;;
  "issue view 9001 --json assignees"*) printf 'someone\n' ;;
  "issue create"*) printf 'https://github.com/o/r/issues/9001\n' ;;
  "issue comment"*|"issue close"*) : ;;
  "issue list"*) : ;;
  "api repos/o/r/assignees/"*) : ;;
  *"actions/workflows/escalate-sweep.yml/runs"*) printf '%s' '2026-09-25T00:00:00Z' ;;
  *"--limit 30"*) printf '%s' '${latest}' ;;
  *"--limit 50"*)
      printf '%s' "\$(printf '{"conclusion":"failure","event":"schedule","databaseId":111,"url":"http://x","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo Workflow"}' | base64 -w0)" ;;
  *) : ;;
esac
STUB
  chmod +x "$s/bin/gh"
  printf '%s' "$s"
}

run_escalate() {  # run_escalate <sandbox> <conclusion>
  local s="$1"
  (cd "$s" && PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r WORKFLOWS_DIR="$s/workflows" \
    bash "$ESCALATE" --workflow "Demo Workflow" --conclusion "$2" --run-id 111 \
      --run-url http://x --event schedule --outcome-file "$s/outcome.tsv" 2>&1)
}

# The listener's receipt step, run as written in escalate.yml rather than restated here,
# so the receipt under test is the receipt that ships.
run_receipt() {  # run_receipt <sandbox>
  local s="$1" script
  script=$(python3 - "$LISTENER" <<'PY'
import sys, yaml
doc = yaml.safe_load(open(sys.argv[1]))
for step in doc["jobs"]["escalate"]["steps"]:
    if step.get("name") == "Emit receipt":
        print(step["run"])
PY
)
  [ -n "$script" ] || { echo "no Emit receipt step found"; return 3; }
  mkdir -p "$s/receipt-dir"
  [ -f "$s/outcome.tsv" ] && cp "$s/outcome.tsv" "$s/receipt-dir/outcome.tsv"
  (cd "$s/receipt-dir" && WF_NAME="Demo Workflow" bash -e -c "$script" 2>&1)
}

run_sweep() {  # run_sweep <sandbox>
  local s="$1"
  (cd "$REPO_ROOT" && PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r WORKFLOWS_DIR="$s/workflows" \
    REGISTRY="$s/registry.yml" bash "$SWEEP" --dry-run 2>&1)
}

called() { grep -qF -- "$2" "$1/calls.log"; }

echo "Listener path"

# 1. Owner open, run failed: delivered to the owner, and the receipt says so.
s=$(make_sandbox OPEN)
out=$(run_escalate "$s" failure); rc=$?
receipt=$(run_receipt "$s"); rrc=$?
if [ $rc -ne 0 ]; then
  report FAIL "an owned failure is routed to its open owner" "exit $rc: $out"
elif ! called "$s" "issue comment ${OWNER}"; then
  report FAIL "an owned failure is routed to its open owner" "no comment on #${OWNER}: $(cat "$s/calls.log")"
elif called "$s" "issue create"; then
  report FAIL "an owned failure is routed to its open owner" "filed an item of its own as well"
elif [ "$(cat "$s/outcome.tsv" 2>/dev/null)" != "$(printf 'owned\t%s' "$OWNER")" ]; then
  report FAIL "an owned failure is routed to its open owner" "outcome was: $(cat "$s/outcome.tsv" 2>/dev/null)"
elif [ $rrc -ne 0 ] || ! jq -e --arg d "$OWNER" '.outcome == "owned" and .destination == $d and .subject == "Demo Workflow"' \
       "$s/receipt-dir/receipt.ndjson" >/dev/null 2>&1; then
  report FAIL "an owned failure is routed to its open owner" "receipt did not name the owner: $receipt"
else
  report PASS "an owned failure is routed to its open owner, and the receipt names #${OWNER}"
fi
rm -rf "$s"

# 2. Owner open, run succeeded: the owner is a decision, not a status; it is not closed.
s=$(make_sandbox OPEN)
out=$(run_escalate "$s" success); rc=$?
if [ $rc -ne 0 ] || called "$s" "issue close" || called "$s" "issue comment"; then
  report FAIL "a green run leaves the owner alone" "exit $rc, calls: $(cat "$s/calls.log")"
elif [ "$(cat "$s/outcome.tsv" 2>/dev/null)" != "$(printf 'owned-healthy\t%s' "$OWNER")" ]; then
  report FAIL "a green run leaves the owner alone" "outcome was: $(cat "$s/outcome.tsv" 2>/dev/null)"
else
  report PASS "a green run leaves the owner alone"
fi
rm -rf "$s"

# 3. Owner closed: the declaration has expired, so the run gets an item of its own
#    rather than reaching an issue nobody reads.
s=$(make_sandbox CLOSED)
out=$(run_escalate "$s" failure); rc=$?
if [ $rc -ne 0 ]; then
  report FAIL "a closed owner falls through to filing" "exit $rc: $out"
elif called "$s" "issue comment ${OWNER}" || ! called "$s" "issue create"; then
  report FAIL "a closed owner falls through to filing" "calls: $(cat "$s/calls.log")"
elif ! printf '%s' "$out" | grep -qF "declaration has expired"; then
  report FAIL "a closed owner falls through to filing" "no expiry warning: $out"
elif [ "$(cat "$s/outcome.tsv" 2>/dev/null)" != "$(printf 'filed\t9001')" ]; then
  report FAIL "a closed owner falls through to filing" "outcome was: $(cat "$s/outcome.tsv" 2>/dev/null)"
else
  report PASS "a closed owner falls through to filing an item of its own"
fi
rm -rf "$s"

# 4. Owner state unreadable: delivered nowhere, and it says so by failing.
s=$(make_sandbox UNREADABLE)
out=$(run_escalate "$s" failure); rc=$?
if [ $rc -eq 0 ] || called "$s" "issue comment" || called "$s" "issue create"; then
  report FAIL "an unreadable owner fails rather than guessing" "exit $rc, calls: $(cat "$s/calls.log")"
else
  report PASS "an unreadable owner fails rather than guessing"
fi
rm -rf "$s"

# 5. The receipt refuses to report a run the escalator recorded nothing about.
s=$(make_sandbox OPEN)
receipt=$(run_receipt "$s"); rrc=$?
if [ $rrc -eq 0 ]; then
  report FAIL "a receipt with no recorded outcome fails" "exit 0: $receipt"
else
  report PASS "a receipt with no recorded outcome fails"
fi
rm -rf "$s"

echo "Sweeper"

# 6. Owner open: the run is named as routed to its owner, not counted as a gap.
s=$(make_sandbox OPEN)
out=$(run_sweep "$s"); rc=$?
if [ $rc -ne 0 ]; then
  report FAIL "the sweep names an owned run as routed" "exit $rc: $out"
elif printf '%s' "$out" | grep -qF 'gap: "Demo Workflow"'; then
  report FAIL "the sweep names an owned run as routed" "treated as a gap: $out"
elif ! printf '%s' "$out" | grep -qF "run 111 routed to its owner #${OWNER}"; then
  report FAIL "the sweep names an owned run as routed" "no routed line: $out"
else
  report PASS "the sweep names an owned run as routed to #${OWNER}, not a gap"
fi
rm -rf "$s"

# 7. Owner closed: expired, so the run is a gap like any other.
s=$(make_sandbox CLOSED)
out=$(run_sweep "$s"); rc=$?
if printf '%s' "$out" | grep -qF "routed to its owner" || ! printf '%s' "$out" | grep -qF 'gap: "Demo Workflow" run 111'; then
  report FAIL "the sweep treats an expired owner as no owner" "exit $rc: $out"
else
  report PASS "the sweep treats an expired owner as no owner"
fi
rm -rf "$s"

# 8. Owner unreadable: the sweep fails rather than reporting the run as handled.
s=$(make_sandbox UNREADABLE)
out=$(run_sweep "$s"); rc=$?
if [ $rc -eq 0 ] || printf '%s' "$out" | grep -qF "routed to its owner"; then
  report FAIL "the sweep fails on an unreadable owner" "exit $rc: $out"
else
  report PASS "the sweep fails on an unreadable owner"
fi
rm -rf "$s"

echo
echo "escalate-owned self-test: $pass passed, $fail failed."
[ "$fail" -eq 0 ] && [ "$pass" -gt 0 ]
