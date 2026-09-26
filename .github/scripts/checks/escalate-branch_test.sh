#!/usr/bin/env bash
set -uo pipefail

# Regression cases for #2686: only runs on the default branch are escalated.
#
# The defect runs in two directions and both are constructed here. A failing branch run
# must not escalate as though main had failed. A green branch run must not unsay a real
# failure: not by closing the open item, not by reading as a recovery, not by moving the
# sweep's window past failures nobody examined.
#
# `gh` is replaced on PATH by a stub that answers DIFFERENTLY with and without the branch
# filter: the unfiltered answer is the one a branch run would corrupt. So a row fails if the
# filter is dropped at the site it covers, rather than passing because the fixture happened
# to hold only main runs.

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
ESCALATE="$REPO_ROOT/.github/scripts/escalate.sh"
SWEEP="$REPO_ROOT/.github/scripts/escalate-sweep.sh"
LISTENER="$REPO_ROOT/.github/workflows/escalate.yml"

pass=0; fail=0
report() {
  case "$1" in
    PASS) pass=$((pass+1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail+1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
  esac
}

# make_sandbox: one registered workflow with an escalator item #77 open, one deferred
# `none` workflow, and a gh stub whose answers depend on whether the branch filter is there.
make_sandbox() {
  local s; s=$(mktemp -d)
  mkdir -p "$s/bin" "$s/workflows"
  : > "$s/calls.log"
  cat > "$s/workflows/demo.yml" <<'YML'
# escalation-policy: issue
# escalation-assignee: someone
# coverage: items
name: Demo Workflow
on:
  schedule:
    - cron: '0 * * * *'
  workflow_dispatch:
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: true
YML
  cat > "$s/workflows/quiet.yml" <<'YML'
# escalation-policy: none
# escalation-reason: fixture
# escalation-none-kind: deferred
# escalation-tracking-issue: 5
# coverage: items
name: Quiet Workflow
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
filtered=0; [[ "\$args" == *"--branch main"* || "\$args" == *"branch=main"* ]] && filtered=1
b64() { printf '%s' "\$1" | base64 -w0; printf ' '; }
case "\$args" in
  *"actions/runs?"*)
      # Repository-wide list, filtered client-side: one rejected run on main, one on a branch.
      printf '%s' '{"workflow_runs":[
        {"id":999,"name":".github/workflows/demo.yml","html_url":"http://r/999","conclusion":"failure","created_at":"2099-01-01T00:00:00Z","head_branch":"main"},
        {"id":998,"name":".github/workflows/quiet.yml","html_url":"http://r/998","conclusion":"failure","created_at":"2099-01-01T00:00:00Z","head_branch":"feature"}]}' ;;
  *"actions/workflows/escalate-sweep.yml/runs"*)
      # The newest successful sweep anywhere is a branch dispatch; the newest on main is older.
      if [ \$filtered = 1 ]; then printf '2026-08-01T00:00:00Z'; else printf '2026-09-26T00:00:00Z'; fi ;;
  "api repos/o/r --jq .default_branch")
      [ -n "\${STUB_NO_DEFAULT:-}" ] && exit 1
      printf 'main\n' ;;
  *"--limit 30"*)
      # Recovery gate. Main's latest run failed; a newer green run exists on a branch.
      if [ \$filtered = 1 ]; then printf 'failure'; else printf 'success'; fi ;;
  *"quiet.yml"*"--limit 50"*)
      # Deferred-none count: three failing runs, all on a branch.
      if [ \$filtered = 1 ]; then printf '0'; else printf '3'; fi ;;
  *"--limit 50"*)
      # Gap scan: one failing main run; unfiltered, a failing branch run as well.
      b64 '{"conclusion":"failure","event":"schedule","databaseId":111,"url":"http://x/111","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo Workflow","headBranch":"main"}'
      if [ \$filtered = 0 ]; then
        b64 '{"conclusion":"failure","event":"workflow_dispatch","databaseId":222,"url":"http://x/222","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo Workflow","headBranch":"feature"}'
      fi ;;
  "issue list"*)
      # The escalator's item for Demo Workflow is open -- unless a sweep row needs it absent.
      if [ -z "\${STUB_NO_ITEM:-}" ]; then printf '77\n'; fi ;;
  "issue view 77 --json assignees"*) printf 'someone\n' ;;
  *) : ;;
esac
STUB
  chmod +x "$s/bin/gh"
  printf '%s' "$s"
}

called() { grep -qF -- "$2" "$1/calls.log"; }

run_escalate() {  # run_escalate <sandbox> <conclusion> <branch>
  local s="$1"
  (cd "$s" && PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r WORKFLOWS_DIR="$s/workflows" DEFAULT_BRANCH=main \
    bash "$ESCALATE" --workflow "Demo Workflow" --conclusion "$2" --run-id 111 \
      --run-url http://x --event workflow_dispatch --branch "$3" --outcome-file "$s/outcome.tsv" 2>&1)
}

run_sweep() {  # run_sweep <sandbox> [env assignments...]
  local s="$1"; shift
  (cd "$REPO_ROOT" && env "$@" PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r WORKFLOWS_DIR="$s/workflows" \
    REGISTRY="$s/registry.yml" bash "$SWEEP" --dry-run 2>&1)
}

echo "Listener path (escalate.sh)"

s=$(make_sandbox)
out=$(run_escalate "$s" failure feature); rc=$?
if [ $rc -eq 0 ] && [ "$(cat "$s/outcome.tsv" 2>/dev/null)" = "$(printf 'not-eligible\t')" ] \
   && ! called "$s" "issue" && grep -qF "was on branch 'feature'" <<< "$out"; then
  report PASS "a failing branch run is refused as not-eligible, and nothing is filed or commented"
else
  report FAIL "a failing branch run is refused as not-eligible" "exit $rc, outcome $(cat "$s/outcome.tsv" 2>/dev/null), calls: $(tr '\n' ';' < "$s/calls.log")"
fi
rm -rf "$s"

s=$(make_sandbox)
out=$(run_escalate "$s" success main); rc=$?
if [ $rc -eq 0 ] && called "$s" "issue close 77"; then
  report PASS "control: a green run on main does reach the close path in this sandbox"
else
  report FAIL "control: a green run on main does reach the close path in this sandbox" "exit $rc, calls: $(tr '\n' ';' < "$s/calls.log")"
fi
rm -rf "$s"

s=$(make_sandbox)
out=$(run_escalate "$s" success feature); rc=$?
if [ $rc -eq 0 ] && ! called "$s" "issue close" && [ "$(cat "$s/outcome.tsv" 2>/dev/null)" = "$(printf 'not-eligible\t')" ]; then
  report PASS "a green branch run does not close the open item for a failure main still has"
else
  report FAIL "a green branch run does not close the open item" "exit $rc, calls: $(tr '\n' ';' < "$s/calls.log")"
fi
rm -rf "$s"

s=$(make_sandbox)
out=$(cd "$s" && PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r WORKFLOWS_DIR="$s/workflows" \
  bash "$ESCALATE" --workflow "Demo Workflow" --conclusion failure --run-id 111 \
    --run-url http://x --event schedule --branch main 2>&1); rc=$?
if [ $rc -eq 2 ] && ! called "$s" "issue"; then
  report PASS "without DEFAULT_BRANCH the script stops rather than judging the run"
else
  report FAIL "without DEFAULT_BRANCH the script stops rather than judging the run" "exit $rc"
fi
rm -rf "$s"

OUT=$(python3 - "$LISTENER" <<'PY' 2>&1
import sys, yaml
doc = yaml.safe_load(open(sys.argv[1]))
problems = []
trigger = (doc.get(True) or doc.get("on"))["workflow_run"]
if "branches" in trigger or "branches-ignore" in trigger:
    problems.append("the trigger filters branches, so a refused run leaves no record")
step = next(s for s in doc["jobs"]["escalate"]["steps"] if s.get("name") == "Escalate the run")
env = step.get("env", {})
if env.get("WF_BRANCH") != "${{ github.event.workflow_run.head_branch }}":
    problems.append(f"WF_BRANCH is {env.get('WF_BRANCH')!r}, not the run's head branch")
if env.get("DEFAULT_BRANCH") != "${{ github.event.repository.default_branch }}":
    problems.append(f"DEFAULT_BRANCH is {env.get('DEFAULT_BRANCH')!r}, not the repository's default branch")
if '--branch "$WF_BRANCH"' not in step["run"]:
    problems.append("the step does not pass --branch \"$WF_BRANCH\"")
print("; ".join(problems) or "wired")
sys.exit(1 if problems else 0)
PY
); rc=$?
if [ $rc -eq 0 ]; then
  report PASS "the listener passes the run's branch and the default branch, and filters nothing at the trigger"
else
  report FAIL "the listener passes the run's branch and the default branch" "$OUT"
fi

echo "Sweeper"

s=$(make_sandbox)
out=$(STUB_NO_ITEM=1 run_sweep "$s" DEFAULT_BRANCH=main); rc=$?
if grep -qF 'gap: "Demo Workflow" run 111' <<< "$out" && ! grep -qF '"Demo Workflow": latest run succeeded' <<< "$out"; then
  report PASS "a newer green branch run does not read as recovery while main is failing"
else
  report FAIL "a newer green branch run does not read as recovery" "$(printf '%s' "$out" | grep -E 'gap:|Recovered' | head -3)"
fi
if ! grep -qF 'run 222' <<< "$out"; then
  report PASS "a failing branch run is not a gap"
else
  report FAIL "a failing branch run is not a gap" "$(grep -F 'run 222' <<< "$out")"
fi
if grep -qF 'since the last completed sweep at 2026-08-01T00:00:00Z' <<< "$out"; then
  report PASS "a green sweep dispatched on a branch does not move the window"
else
  report FAIL "a green sweep dispatched on a branch does not move the window" "$(printf '%s' "$out" | grep -i 'window' | head -1)"
fi
if grep -qF '.github/workflows/demo.yml (run 999)' <<< "$out" && ! grep -qF 'run 998' <<< "$out"; then
  report PASS "a run rejected on a branch is not reported; one rejected on main still is"
else
  report FAIL "a run rejected on a branch is not reported; one rejected on main still is" "$(printf '%s' "$out" | grep -E 'run 99[89]' | head -3)"
fi
if grep -qF 'Deferred escalation-policy: none with failing runs: none.' <<< "$out"; then
  report PASS "failing branch runs of a deferred-none workflow are not counted as its cost"
else
  report FAIL "failing branch runs of a deferred-none workflow are not counted" "$(printf '%s' "$out" | grep -A1 'Deferred' | head -2)"
fi
if grep -qF "Branch: only runs on 'main' are examined" <<< "$out"; then
  report PASS "the sweep says which branch it examined"
else
  report FAIL "the sweep says which branch it examined" "$(printf '%s' "$out" | head -3)"
fi
rm -rf "$s"

s=$(make_sandbox)
out=$(run_sweep "$s"); rc=$?
if [ $rc -eq 0 ] && called "$s" "api repos/o/r --jq .default_branch" && grep -qF "only runs on 'main'" <<< "$out"; then
  report PASS "without DEFAULT_BRANCH the sweep asks the API for it"
else
  report FAIL "without DEFAULT_BRANCH the sweep asks the API for it" "exit $rc: $(printf '%s' "$out" | head -3)"
fi
rm -rf "$s"

s=$(make_sandbox)
out=$(run_sweep "$s" STUB_NO_DEFAULT=1); rc=$?
if [ $rc -eq 2 ] && grep -qF "could not read the repository's default branch" <<< "$out" && ! called "$s" "--limit"; then
  report PASS "an unreadable default branch stops the sweep instead of sweeping every branch"
else
  report FAIL "an unreadable default branch stops the sweep" "exit $rc: $(printf '%s' "$out" | head -3)"
fi
rm -rf "$s"

echo
echo "escalate-branch self-test: $pass passed, $fail failed."
[ "$fail" -eq 0 ] && [ "$pass" -gt 0 ]
