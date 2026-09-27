#!/usr/bin/env bash
set -uo pipefail

# Regression cases for escalate-sweep.sh, each constructing on purpose a state the original
# tests never produced.
#
# Three defects shipped in this sweeper and all three were invisible for the same reason:
# every test used a state in which the defect could not show. The tests only ever met
# workflows that were failing, so a recovered one never appeared. They only ever met
# workflows that did not parse, so a resolved name never appeared. And they were all run by
# hand, so the schedule never appeared.
#
# So these cases build the missing states rather than approximating them. `gh` is replaced
# on PATH by a stub that answers from fixtures, which is what makes "failed then recovered"
# and "previous sweep older than any fixed lookback" constructible at all.

SWEEP="$(cd "$(dirname "$0")/../.." && pwd)/scripts/escalate-sweep.sh"
pass=0; fail=0; void=0
report() {
  case "$1" in
    PASS) pass=$((pass+1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail+1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
    VOID) void=$((void+1)); echo "  [VOID] $2${3:+ -- $3}" >&2 ;;
  esac
}

# Build a sandbox: a stub gh, one registered workflow, a registry.
# $1 = latest conclusion, $2 = conclusion of the older in-window run,
# $3 = created_at of the last successful sweep (empty for none)
make_sandbox() {
  local latest="$1" older="$2" last_sweep="$3"
  local s; s=$(mktemp -d)
  mkdir -p "$s/bin" "$s/workflows"

  cat > "$s/workflows/demo.yml" <<YML
# escalation-policy: issue
# escalation-assignee: someone
# coverage: items
name: Demo Workflow
on:
  schedule:
    - cron: '0 * * * *'
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: gh issue create --title x --body y --label maintenance
YML
  printf 'registered:\n  - "Demo Workflow"\n' > "$s/registry.yml"

  cat > "$s/bin/gh" <<STUB
#!/usr/bin/env bash
# Stub gh. Answers only what escalate-sweep.sh asks, from the fixture values baked in.
args="\$*"
case "\$args" in
  *"actions/runs?"*)
      # The repository-wide run list. Its .name is the RECORDED name: a path for a run that
      # was rejected before parsing, even though the workflow parses now.
      if [ -n "\${STUB_STARTUP:-}" ] && [[ "\$args" == *"page=1"* ]]; then
        printf '%s' '{"workflow_runs":[{"id":999,"name":".github/workflows/demo.yml","html_url":"http://r/999","conclusion":"failure","created_at":"2099-01-01T00:00:00Z","head_branch":"main"}]}'
      else
        printf '%s' '{"workflow_runs":[]}'
      fi ;;
  *"actions/workflows/escalate-sweep.yml/runs"*)
      # Two different answers, so the row fails if the success filter is ever dropped.
      # A real repository would return the FAILED sweep as the most recent run; only
      # filtering on status=success reaches back to the one that did its work.
      case "\$args" in
        *status=success*) printf '%s' '${last_sweep}' ;;
        *)                printf '%s' '2026-09-25T18:00:00Z' ;;
      esac ;;
  *"--limit 30"*)
      # the current-state gate's question: what did the latest run conclude?
      printf '%s' '${latest}' ;;
  *"--limit 50"*)
      if [ -n "\${STUB_RUNLIST_FAILS:-}" ]; then exit 1; fi
      # runs inside the window: one older run with the fixture's conclusion
      if [ -n "\${STUB_STARTUP:-}" ]; then
        # Same run, resolved name — which is exactly what makes it invisible to a name test.
        printf '%s' "\$(printf '{"conclusion":"failure","event":"schedule","databaseId":999,"url":"http://r/999","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo Workflow","headBranch":"main"}' | base64 -w0)"
      else
        printf '%s' "\$(printf '{"conclusion":"${older}","event":"schedule","databaseId":111,"url":"http://x","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo Workflow","headBranch":"main"}' | base64 -w0)"
      fi ;;
  *"issue list"*) : ;;
  *"--limit 100"*) : ;;
  *) : ;;
esac
STUB
  chmod +x "$s/bin/gh"
  printf '%s' "$s"
}

run_sweep() {  # run_sweep <sandbox> [extra args...]
  local s="$1"; shift
  PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r DEFAULT_BRANCH=main WORKFLOWS_DIR="$s/workflows" REGISTRY="$s/registry.yml" \
    bash "$SWEEP" --dry-run "$@" 2>&1
}

echo "Case 1: a workflow that failed and then RECOVERED inside the window"
echo "  (the state the original tests never produced: they only ever met failing workflows)"
s=$(make_sandbox "success" "failure" "2026-09-25T18:36:36Z")
out=$(run_sweep "$s")
if grep -qF '"Demo Workflow": latest run succeeded' <<< "$out" && ! grep -q '^gap:' <<< "$out"; then
  report PASS "a recovered workflow is not escalated, and is named"
else
  report FAIL "a recovered workflow is not escalated, and is named" "$(printf '%s' "$out" | grep -E '^gap:|Recovered' | head -2)"
fi
rm -rf "$s"

echo "Case 2: still failing — the gate must not suppress a real escalation"
s=$(make_sandbox "failure" "failure" "2026-09-25T18:36:36Z")
out=$(run_sweep "$s")
if grep -q '^gap:' <<< "$out"; then
  report PASS "a still-failing workflow is escalated"
else
  report FAIL "a still-failing workflow is escalated" "no gap reported"
fi
rm -rf "$s"

echo "Case 3: the previous sweep is older than any fixed lookback would have covered"
echo "  (the state a hand-run test never produces, because the schedule never appears)"
s=$(make_sandbox "failure" "failure" "2026-08-01T00:00:00Z")
out=$(run_sweep "$s")
if grep -qF 'since the last completed sweep at 2026-08-01T00:00:00Z' <<< "$out"; then
  report PASS "the window starts at the last completed sweep, however long ago"
else
  report FAIL "the window starts at the last completed sweep, however long ago" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 4: no previous sweep at all"
s=$(make_sandbox "failure" "failure" "")
out=$(run_sweep "$s")
if grep -qF 'no previous successful scheduled sweep found' <<< "$out"; then
  report PASS "the first sweep ever falls back to a default window and says so"
else
  report FAIL "the first sweep ever falls back to a default window and says so" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 5: an explicit --window-hours still overrides"
s=$(make_sandbox "failure" "failure" "2026-08-01T00:00:00Z")
out=$(run_sweep "$s" --window-hours 3)
if grep -qF 'Window: explicit, 3h' <<< "$out"; then
  report PASS "--window-hours overrides the derived window"
else
  report FAIL "--window-hours overrides the derived window" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 6: the previous sweep FAILED — the window must reach past it"
echo "  (a broken sweep must not advance the window, or its whole window is never examined)"
s=$(make_sandbox "failure" "failure" "2026-08-01T00:00:00Z")
out=$(run_sweep "$s")
if grep -qF 'since the last completed sweep at 2026-08-01T00:00:00Z' <<< "$out"; then
  report PASS "a failed previous sweep does not advance the window"
else
  report FAIL "a failed previous sweep does not advance the window" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 7: a workflow that cannot be queried must fail the sweep"
echo "  (an unreachable workflow is not an examined one, and the receipt must not say it was)"
s=$(make_sandbox "failure" "failure" "2026-09-25T18:36:36Z")
out=$(STUB_RUNLIST_FAILS=1 PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r DEFAULT_BRANCH=main         WORKFLOWS_DIR="$s/workflows" REGISTRY="$s/registry.yml"         bash "$SWEEP" --dry-run 2>&1); rc=$?
if [ $rc -ne 0 ] && grep -qF 'was NOT examined' <<< "$out"; then
  report PASS "an unreachable workflow fails the sweep rather than being counted"
else
  report FAIL "an unreachable workflow fails the sweep rather than being counted" "exit $rc"
fi
rm -rf "$s"

echo "Case 8: a run rejected before parsing, whose workflow NOW parses"
echo "  (the state the original detector was never tested in: it was proven only while the"
echo "   workflow was still broken, so there was no current name to resolve to)"
s=$(make_sandbox "failure" "failure" "2026-09-25T18:36:36Z")
out=$(STUB_STARTUP=1 PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r DEFAULT_BRANCH=main \
        WORKFLOWS_DIR="$s/workflows" REGISTRY="$s/registry.yml" \
        bash "$SWEEP" --dry-run 2>&1)
if grep -qF 'rejected before job creation: .github/workflows/demo.yml' <<< "$out" \
   && ! grep -qF 'gap: "Demo Workflow" run 999' <<< "$out"; then
  report PASS "a rejected run is reported as rejected, not escalated as a failure"
else
  report FAIL "a rejected run is reported as rejected, not escalated as a failure" \
    "$(printf '%s' "$out" | grep -E 'rejected|gap:' | head -2)"
fi
rm -rf "$s"

echo "Case 9: the receipt must count a gated workflow as examined"
echo "  (none of the cases above asserts on \`attempted\` — they check what got escalated,"
echo "   never what got counted, which is how the field understated coverage by half)"
s9=$(mktemp -d); mkdir -p "$s9/bin" "$s9/workflows"
for n in one two; do
  cat > "$s9/workflows/demo-$n.yml" <<YML
# escalation-policy: issue
# escalation-assignee: someone
# coverage: items
name: Demo $n
on:
  schedule:
    - cron: '0 * * * *'
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: gh issue create --title x --body y --label maintenance
YML
done
printf 'registered:\n  - "Demo one"\n  - "Demo two"\n' > "$s9/registry.yml"
cat > "$s9/bin/gh" <<'STUB'
#!/usr/bin/env bash
args="$*"
case "$args" in
  *"actions/runs?"*) printf '%s' '{"workflow_runs":[]}' ;;
  *"actions/workflows/escalate-sweep.yml/runs"*) printf '%s' '2026-09-25T18:36:36Z' ;;
  *"--limit 30"*)
      # demo-one is failing; demo-two has recovered. Both are examined.
      case "$args" in *demo-one*) printf 'failure' ;; *) printf 'success' ;; esac ;;
  *"--limit 50"*)
      printf '%s' "$(printf '{"conclusion":"failure","event":"schedule","databaseId":222,"url":"http://x","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo one","headBranch":"main"}' | base64 -w0)" ;;
  *) : ;;
esac
STUB
chmod +x "$s9/bin/gh"
out=$(PATH="$s9/bin:$PATH" GITHUB_REPOSITORY=o/r DEFAULT_BRANCH=main WORKFLOWS_DIR="$s9/workflows" \
        REGISTRY="$s9/registry.yml" bash "$SWEEP" --dry-run 2>&1)
if grep -qF 'Receipt: declared 2, attempted 2, unreachable 0' <<< "$out" \
   && grep -qF '"Demo two": latest run succeeded' <<< "$out"; then
  report PASS "a workflow gated as recovered still counts as examined"
else
  report FAIL "a workflow gated as recovered still counts as examined" \
    "$(printf '%s' "$out" | grep -E '^Receipt:' | head -1)"
fi
rm -rf "$s9"

# --- the window as production computes it (#2689) -----------------------------------------
#
# Every case above calls the script directly, where the arguments are whatever the test
# chooses, so none of them could see that the workflow always passed an explicit two hours.
# These run the Sweep step AS WRITTEN in escalate-sweep.yml, with its env evaluated the way
# a schedule (no inputs) or a dispatch (the declared defaults, or given values) evaluates
# it. The stub answers the anchor query the way the API does, filtering a fixture of sweep
# runs by the query's own status, event and branch parameters, and applies the caller's
# --jq with real jq.

SWEEP_WF="$(cd "$(dirname "$0")/../.." && pwd)/workflows/escalate-sweep.yml"
REPO_ROOT_W="$(cd "$(dirname "$0")/../../.." && pwd)"

# sweep_step_env <inputs json>: prints NAME=value lines for the Sweep step's env, each
# value evaluated as GitHub would for these inputs. Handles the forms the file uses:
# a context path, a quoted literal, `a || b` and `a && b`.
sweep_step_env() {
  python3 - "$SWEEP_WF" "$1" <<'PY'
import json, re, sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
ctx = {"inputs": json.loads(sys.argv[2]), "secrets": {"GITHUB_TOKEN": "stub"},
       "github": {"repository": "o/r"}}
def tokens(expr):
    return re.findall(r"'[^']*'|\|\||&&|[A-Za-z_][\w.]*", expr)
def operand(t):
    if t.startswith("'"):
        return t[1:-1]
    if t in ("true", "false"):
        return t == "true"
    v = ctx
    for part in t.split("."):
        v = v.get(part) if isinstance(v, dict) else None
    return v
def evaluate(expr):
    # || of && chains, left to right, JavaScript-style: returns an operand, not a boolean.
    result = None
    for alt in re.split(r"\s*\|\|\s*", expr.strip()):
        val = None
        for i, t in enumerate(re.split(r"\s*&&\s*", alt)):
            val = operand(t.strip())
            if not val:
                break
        result = val
        if val:
            return val
    return result
def render(v):
    if v is None or v is False:
        return ""
    return "true" if v is True else str(v)
step = next(s for s in wf["jobs"]["sweep"]["steps"] if s.get("name") == "Sweep")
for k, v in step["env"].items():
    m = re.fullmatch(r"\$\{\{(.*)\}\}", str(v).strip())
    print(f"{k}={render(evaluate(m.group(1))) if m else v}")
PY
}

# The dispatch inputs' declared defaults, as a dispatch with nothing filled in would get them.
dispatch_defaults() {
  python3 - "$SWEEP_WF" <<'PY'
import json, sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
on = wf.get(True) or wf.get("on")
print(json.dumps({k: v.get("default") for k, v in on["workflow_dispatch"]["inputs"].items()}))
PY
}

# make_window_sandbox: sweep-run fixture = one successful scheduled sweep on main
# (2026-08-01) and a NEWER successful manual dry-run sweep on main (2026-09-26).
make_window_sandbox() {
  local s; s=$(mktemp -d)
  mkdir -p "$s/bin" "$s/workflows"
  cat > "$s/workflows/demo.yml" <<'YML'
# escalation-policy: issue
# escalation-assignee: someone
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
  cat > "$s/sweeps.json" <<'JSON'
{"workflow_runs":[
 {"created_at":"2026-09-26T00:00:00Z","status":"completed","conclusion":"success","event":"workflow_dispatch","head_branch":"main"},
 {"created_at":"2026-08-01T00:00:00Z","status":"completed","conclusion":"success","event":"schedule","head_branch":"main"}]}
JSON
  cat > "$s/bin/gh" <<STUB
#!/usr/bin/env bash
args="\$*"
case "\$args" in
  *"actions/workflows/escalate-sweep.yml/runs"*)
      [ -n "\${STUB_ANCHOR_FAILS:-}" ] && exit 1
      url="\$2"; q="\${url#*\?}"; jqexpr=""
      shift 2; while [ \$# -gt 0 ]; do [ "\$1" = "--jq" ] && jqexpr="\$2"; shift; done
      param() { tr '&' '\n' <<< "\$q" | sed -n "s/^\$1=//p"; }
      st=\$(param status); ev=\$(param event); br=\$(param branch)
      jq -r --arg st "\$st" --arg ev "\$ev" --arg br "\$br" "
        .workflow_runs |= map(select((\\\$st == \"\" or .conclusion == \\\$st)
                                   and (\\\$ev == \"\" or .event == \\\$ev)
                                   and (\\\$br == \"\" or .head_branch == \\\$br)))
        | \$jqexpr" "$s/sweeps.json" ;;
  *"actions/runs?"*)
      [ -n "\${STUB_RUNLIST_PAGE_FAILS:-}" ] && exit 1
      printf '%s' '{"workflow_runs":[]}' ;;
  *"--limit 30"*) printf 'success' ;;
  *) : ;;
esac
STUB
  chmod +x "$s/bin/gh"
  printf '%s' "$s"
}

run_sweep_step() {  # run_sweep_step <sandbox> <inputs json> [extra env...]
  local s="$1" inputs="$2"; shift 2
  local envs run
  mapfile -t envs < <(sweep_step_env "$inputs")
  run=$(python3 -c 'import sys,yaml; wf=yaml.safe_load(open(sys.argv[1])); print(next(s["run"] for s in wf["jobs"]["sweep"]["steps"] if s.get("name")=="Sweep"))' "$SWEEP_WF")
  (cd "$REPO_ROOT_W" && env "${envs[@]}" "$@" DEFAULT_BRANCH=main PATH="$s/bin:$PATH" \
     WORKFLOWS_DIR="$s/workflows" REGISTRY="$s/registry.yml" bash -e -c "$run" 2>&1)
}

echo "Case 10: a SCHEDULED sweep, run as the workflow runs it, with no inputs"
s=$(make_window_sandbox)
out=$(run_sweep_step "$s" '{}'); rc=$?
if [ $rc -eq 0 ] && grep -qF 'Window: since the last completed sweep at 2026-08-01T00:00:00Z' <<< "$out"; then
  report PASS "a scheduled sweep derives its window from the last scheduled sweep"
else
  report FAIL "a scheduled sweep derives its window from the last scheduled sweep" "exit $rc: $(grep -F 'Window:' <<< "$out")"
fi
rm -rf "$s"

echo "Case 11: a dispatch with nothing filled in gets the input defaults"
s=$(make_window_sandbox)
defaults=$(dispatch_defaults)
out=$(run_sweep_step "$s" "$defaults"); rc=$?
if [ $rc -eq 0 ] && grep -qF 'Window: since the last completed sweep at 2026-08-01T00:00:00Z' <<< "$out"; then
  report PASS "a default dispatch derives its window too"
else
  report FAIL "a default dispatch derives its window too" "defaults $defaults; exit $rc: $(grep -F 'Window:' <<< "$out")"
fi
rm -rf "$s"

echo "Case 12: a dry-run sweep followed by a real one"
echo "  (the dry run is the newest successful sweep; it filed nothing, so it must not anchor)"
s=$(make_window_sandbox)
out=$(run_sweep_step "$s" '{}'); rc=$?
if grep -qF '2026-08-01T00:00:00Z' <<< "$out" && ! grep -qF 'since the last completed sweep at 2026-09-26' <<< "$out"; then
  report PASS "the real sweep's window reaches back past the dry run"
else
  report FAIL "the real sweep's window reaches back past the dry run" "$(grep -F 'Window:' <<< "$out")"
fi
rm -rf "$s"

echo "Case 13: an explicit window on a dispatch is still honoured"
s=$(make_window_sandbox)
out=$(run_sweep_step "$s" '{"window_hours":"3","dry_run":true}'); rc=$?
if [ $rc -eq 0 ] && grep -qF 'Window: explicit, 3h' <<< "$out"; then
  report PASS "a dispatch that names a window gets that window"
else
  report FAIL "a dispatch that names a window gets that window" "exit $rc: $(grep -F 'Window:' <<< "$out")"
fi
rm -rf "$s"

echo "Case 14: the anchor cannot be read"
s=$(make_window_sandbox)
out=$(run_sweep_step "$s" '{}' STUB_ANCHOR_FAILS=1); rc=$?
if [ $rc -ne 0 ] && grep -qF 'Window: UNKNOWN anchor' <<< "$out" && grep -qF 'could not establish its window' <<< "$out"; then
  report PASS "an unreadable anchor is not 'no previous sweep', and fails the sweep"
else
  report FAIL "an unreadable anchor is not 'no previous sweep', and fails the sweep" "exit $rc: $(grep -E 'Window:|establish' <<< "$out")"
fi
rm -rf "$s"

echo "Case 15: the run list for rejected runs cannot be read"
s=$(make_window_sandbox)
out=$(run_sweep_step "$s" '{}' STUB_RUNLIST_PAGE_FAILS=1); rc=$?
if [ $rc -ne 0 ] && grep -qF 'could not establish its window or read every run in it' <<< "$out"; then
  report PASS "a sweep that could not read every run in its window fails, so it cannot anchor the next"
else
  report FAIL "a sweep that could not read every run in its window fails" "exit $rc: $(tail -3 <<< "$out")"
fi
rm -rf "$s"

echo
echo "escalate-sweep self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
