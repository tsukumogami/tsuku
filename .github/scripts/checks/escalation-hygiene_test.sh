#!/usr/bin/env bash
set -uo pipefail

# Proves every assertion in escalation-hygiene.py can fail.
#
# A hygiene check that cannot fail would be a fitting end to a milestone about checks that
# cannot fail, so each assertion gets a row and each row mutates a real workflow rather
# than a synthetic one. Every mutating row hashes its target and VOIDS itself if nothing
# changed.

CHECK="$(dirname "$0")/escalation-hygiene.py"
REPO_ROOT="$(git rev-parse --show-toplevel)"
WORKFLOWS="$REPO_ROOT/.github/workflows"
DELIVERER="discovery-freshness.yml"   # a real workflow that files an issue

pass=0; fail=0; void=0
report() {
  case "$1" in
    PASS) pass=$((pass+1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail+1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
    VOID) void=$((void+1)); echo "  [VOID] $2${3:+ -- $3}" >&2 ;;
  esac
}

# run_case <name> <want-exit> <want-text> <mutator|-->
run_case() {
  local name="$1" want_exit="$2" want_text="$3" mutator="$4"
  local sandbox before after out rc
  sandbox=$(mktemp -d); mkdir -p "$sandbox/wf"
  cp "$WORKFLOWS/$DELIVERER" "$sandbox/wf/"

  if [ "$mutator" != "--" ]; then
    before=$(sha256sum "$sandbox/wf/$DELIVERER" | cut -d' ' -f1)
    "$mutator" "$sandbox"
    after=$(sha256sum "$sandbox/wf/$DELIVERER" | cut -d' ' -f1)
    if [ "$before" = "$after" ]; then
      report VOID "$name" "mutation changed nothing"
      rm -rf "$sandbox"; return
    fi
  fi

  out=$(python3 "$CHECK" --workflows "$sandbox/wf" 2>&1); rc=$?
  if [ "$rc" -ne "$want_exit" ]; then
    report FAIL "$name" "exit $rc, wanted $want_exit"
  elif [ -n "$want_text" ] && ! printf '%s' "$out" | grep -qF -- "$want_text"; then
    report FAIL "$name" "output lacked: $want_text"
  else
    report PASS "$name"
  fi
  rm -rf "$sandbox"
}

echo "Control"
run_case "an unmutated deliverer passes" 0 "" --

echo "Assertion 1: a delivery step must not suppress its own failure"

# `|` cannot be the sed delimiter here: the replacement itself contains `||`, which ends
# the expression early. The first draft of this row silently mutated nothing and the void
# guard caught it, which is the guard doing its job on its own author.
suppress_or_true()  { sed -i 's@--label "maintenance,discovery-registry"$@--label "maintenance,discovery-registry" || true@' "$1/wf/$DELIVERER"; }
suppress_devnull()  { sed -i 's|--label "maintenance,discovery-registry"$|--label "maintenance,discovery-registry" 2>/dev/null|' "$1/wf/$DELIVERER"; }
step_coe()          { python3 -c "
import pathlib,sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
p.write_text(s.replace('      - name: Create failure issue\n','      - name: Create failure issue\n        continue-on-error: true\n',1))" "$1/wf/$DELIVERER"; }
job_coe()           { python3 -c "
import pathlib,sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
p.write_text(s.replace('    runs-on: ubuntu-latest\n','    runs-on: ubuntu-latest\n    continue-on-error: true\n',1))" "$1/wf/$DELIVERER"; }

run_case "|| true on the delivery command" 1 "applies \`|| true\` to the command" suppress_or_true
run_case "2>/dev/null on the delivery command" 1 "applies \`2>/dev/null\` to the command" suppress_devnull
run_case "continue-on-error on the delivery step" 1 "declares \`continue-on-error: true\`" step_coe
run_case "continue-on-error on the delivery job" 1 "contains an issue-filing step and declares" job_coe

echo "Assertion 1: and it must find the steps at all"
empty_dir=$(mktemp -d); mkdir -p "$empty_dir/wf"; cp "$WORKFLOWS/lint-workflows.yml" "$empty_dir/wf/"
out=$(python3 "$CHECK" --workflows "$empty_dir/wf" 2>&1); rc=$?
if [ $rc -eq 1 ] && printf '%s' "$out" | grep -qF "found no issue-filing steps at all"; then
  report PASS "a directory with no delivery steps fails the zero floor"
else
  report FAIL "a directory with no delivery steps fails the zero floor" "exit $rc"
fi
rm -rf "$empty_dir"

echo "Assertion 2: no workflow may contain an empty expression"

empty_expr_comment() { sed -i '3i # a comment that mentions ${{ }} and thereby breaks the file' "$1/wf/$DELIVERER"; }
empty_expr_run()     { python3 -c "
import pathlib,sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
p.write_text(s.replace('        run: |\n','        run: |\n          echo \"\${{ }}\"\n',1))" "$1/wf/$DELIVERER"; }
empty_expr_key()     { sed -i 's|^name: Discovery Registry Freshness$|name: ${{ }}|' "$1/wf/$DELIVERER"; }

# Three positions, one rule. The point of asserting on the class is that position does not
# matter to the evaluator, so it must not matter to the check.
run_case "empty expression in a comment" 1 "empty GitHub expression" empty_expr_comment
run_case "empty expression in a run: block" 1 "empty GitHub expression" empty_expr_run
run_case "empty expression in a key's value" 1 "empty GitHub expression" empty_expr_key

echo
echo "escalation-hygiene self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
