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
      printf '%s' "\$(printf '{"conclusion":"${older}","event":"schedule","databaseId":111,"url":"http://x","createdAt":"2099-01-01T00:00:00Z","workflowName":"Demo Workflow"}' | base64 -w0)" ;;
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
  PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r WORKFLOWS_DIR="$s/workflows" REGISTRY="$s/registry.yml" \
    bash "$SWEEP" --dry-run "$@" 2>&1
}

echo "Case 1: a workflow that failed and then RECOVERED inside the window"
echo "  (the state the original tests never produced: they only ever met failing workflows)"
s=$(make_sandbox "success" "failure" "2026-09-25T18:36:36Z")
out=$(run_sweep "$s")
if printf '%s' "$out" | grep -q 'Recovered' && ! printf '%s' "$out" | grep -q '^gap:'; then
  report PASS "a recovered workflow is not escalated, and is named"
else
  report FAIL "a recovered workflow is not escalated, and is named" "$(printf '%s' "$out" | grep -E '^gap:|Recovered' | head -2)"
fi
rm -rf "$s"

echo "Case 2: still failing — the gate must not suppress a real escalation"
s=$(make_sandbox "failure" "failure" "2026-09-25T18:36:36Z")
out=$(run_sweep "$s")
if printf '%s' "$out" | grep -q '^gap:'; then
  report PASS "a still-failing workflow is escalated"
else
  report FAIL "a still-failing workflow is escalated" "no gap reported"
fi
rm -rf "$s"

echo "Case 3: the previous sweep is older than any fixed lookback would have covered"
echo "  (the state a hand-run test never produces, because the schedule never appears)"
s=$(make_sandbox "failure" "failure" "2026-08-01T00:00:00Z")
out=$(run_sweep "$s")
if printf '%s' "$out" | grep -qF 'since the last completed sweep at 2026-08-01T00:00:00Z'; then
  report PASS "the window starts at the last completed sweep, however long ago"
else
  report FAIL "the window starts at the last completed sweep, however long ago" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 4: no previous sweep at all"
s=$(make_sandbox "failure" "failure" "")
out=$(run_sweep "$s")
if printf '%s' "$out" | grep -qF 'no previous successful sweep found'; then
  report PASS "the first sweep ever falls back to a default window and says so"
else
  report FAIL "the first sweep ever falls back to a default window and says so" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 5: an explicit --window-hours still overrides"
s=$(make_sandbox "failure" "failure" "2026-08-01T00:00:00Z")
out=$(run_sweep "$s" --window-hours 3)
if printf '%s' "$out" | grep -qF 'Window: explicit, 3h'; then
  report PASS "--window-hours overrides the derived window"
else
  report FAIL "--window-hours overrides the derived window" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 6: the previous sweep FAILED — the window must reach past it"
echo "  (a broken sweep must not advance the window, or its whole window is never examined)"
s=$(make_sandbox "failure" "failure" "2026-08-01T00:00:00Z")
out=$(run_sweep "$s")
if printf '%s' "$out" | grep -qF 'since the last completed sweep at 2026-08-01T00:00:00Z'; then
  report PASS "a failed previous sweep does not advance the window"
else
  report FAIL "a failed previous sweep does not advance the window" "$(printf '%s' "$out" | grep -i window | head -1)"
fi
rm -rf "$s"

echo "Case 7: a workflow that cannot be queried must fail the sweep"
echo "  (an unreachable workflow is not an examined one, and the receipt must not say it was)"
s=$(make_sandbox "failure" "failure" "2026-09-25T18:36:36Z")
out=$(STUB_RUNLIST_FAILS=1 PATH="$s/bin:$PATH" GITHUB_REPOSITORY=o/r         WORKFLOWS_DIR="$s/workflows" REGISTRY="$s/registry.yml"         bash "$SWEEP" --dry-run 2>&1); rc=$?
if [ $rc -ne 0 ] && printf '%s' "$out" | grep -qF 'was NOT examined'; then
  report PASS "an unreachable workflow fails the sweep rather than being counted"
else
  report FAIL "an unreachable workflow fails the sweep rather than being counted" "exit $rc"
fi
rm -rf "$s"

echo
echo "escalate-sweep self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
