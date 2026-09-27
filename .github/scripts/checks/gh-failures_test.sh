#!/usr/bin/env bash
set -uo pipefail

# Regression cases for #2642: gh calls that swallowed their own failure.
#
# Each site used to throw gh's stderr away and substitute a default, so a failed read
# arrived as data. The batch guard's substitute was `0`, which is the one answer that
# disables it, and on 2026-08-17 it opened a second batch pull request while another was
# open. These rows make gh fail on purpose and assert two things: the step stops (or, for
# the golden exclusions, counts the lookup as invalid), and gh's own error text reaches the
# log. The second half matters as much as the first -- the incident could not be diagnosed
# because the error was gone.
#
# Workflow rows run the step text extracted from the workflow file itself, under the shell
# Actions uses for an unspecified `shell:` (`bash -e`, no pipefail), so a row fails if the
# file stops doing what the row says rather than if a copy of it does. `gh` is replaced on
# PATH by a stub; nothing here reaches GitHub.

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WF="$REPO_ROOT/.github/workflows"
STUB_ERR="HTTP 502: stub gh failure for #2642 regression"

pass=0; fail=0
report() {
  case "$1" in
    PASS) pass=$((pass+1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail+1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
  esac
}
# expect <description> <expected exit> <actual exit> <output> [<substring the output must contain>]...
expect() {
  local desc="$1" want="$2" got="$3" out="$4"; shift 4
  if [ "$got" != "$want" ]; then report FAIL "$desc" "exit $got, wanted $want; output: $out"; return; fi
  local needle
  for needle in "$@"; do
    if ! grep -Fq -- "$needle" <<< "$out"; then report FAIL "$desc" "output lacks '$needle': $out"; return; fi
  done
  report PASS "$desc"
}
# lacks <description> <text> <substring that must NOT appear>
lacks() {
  if grep -Fq -- "$3" <<< "$2"; then report FAIL "$1" "found '$3' in: $2"; else report PASS "$1"; fi
}

T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

# The stub records each invocation in $T/calls. The read under test (`pr list`,
# `release view`, `api`) is answered from STUB_MODE; any other call (the writes that follow
# a read) succeeds silently, so a row can see whether the step went on to make it.
#   fail       exit 1 with STUB_ERR on stderr
#   notfound   exit 1 with gh's own "release not found"
#   out:<text> exit 0 printing <text>
mkdir -p "$T/bin"
cat > "$T/bin/gh" <<'STUB'
#!/usr/bin/env bash
echo "gh $*" >> "$STUB_CALLS"
case "$1 ${2:-}" in
  "pr list"|"release view"|api\ *) ;;
  *) exit 0 ;;
esac
case "$STUB_MODE" in
  fail)     echo "$STUB_ERR" >&2; exit 1 ;;
  notfound) echo "release not found" >&2; exit 1 ;;
  out:*)    printf '%s\n' "${STUB_MODE#out:}"; exit 0 ;;
esac
echo "stub gh: unknown STUB_MODE '$STUB_MODE'" >&2; exit 99
STUB
# git is stubbed too: release.yml's create path fetches a tag, which must not touch a
# network or a real repository here.
cat > "$T/bin/git" <<'STUB'
#!/usr/bin/env bash
echo "git $*" >> "$STUB_CALLS"
STUB
chmod +x "$T/bin/gh" "$T/bin/git"

step_run() {  # step_run <workflow> <job> <step name> : prints the step's run text
  python3 - "$1" "$2" "$3" <<'PY'
import sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
for step in wf["jobs"][sys.argv[2]]["steps"]:
    if step.get("name") == sys.argv[3]:
        print(step["run"])
        break
else:
    sys.exit(f"no step {sys.argv[3]!r} in job {sys.argv[2]!r}")
PY
}

# run_step <script> <stub mode> : sets OUT, RC, CALLS, GHOUT
run_step() {
  : > "$T/calls"; : > "$T/github_output"
  OUT=$(cd "$T" && PATH="$T/bin:$PATH" STUB_MODE="$2" STUB_ERR="$STUB_ERR" STUB_CALLS="$T/calls" \
        GITHUB_OUTPUT="$T/github_output" GITHUB_REF_NAME="v9.9.9" bash -e "$1" 2>&1); RC=$?
  CALLS=$(cat "$T/calls"); GHOUT=$(cat "$T/github_output")
}

echo "batch-generate.yml: Check for open batch PRs"

if step_run "$WF/batch-generate.yml" generate "Check for open batch PRs" > "$T/guard.sh"; then
  run_step "$T/guard.sh" fail
  expect "a failed gh pr list stops the run and shows gh's error" 1 "$RC" "$OUT" \
    "$STUB_ERR" "refusing to generate"
  lacks "a failed gh pr list never reports 'proceeding'" "$OUT" "proceeding with batch generation"
  lacks "a failed gh pr list never sets should_skip=false" "$GHOUT" "should_skip=false"

  run_step "$T/guard.sh" "out:"
  expect "an empty answer is not read as zero" 1 "$RC" "$OUT" "rather than a count"
  lacks "an empty answer never sets should_skip=false" "$GHOUT" "should_skip=false"

  # Controls: the guard still answers both ways when gh answers.
  run_step "$T/guard.sh" "out:2"
  expect "two open batch PRs skip the run" 0 "$RC" "$OUT" "2 open batch PR(s) exist"
  expect "two open batch PRs set should_skip=true" 0 "$RC" "$GHOUT" "should_skip=true"

  run_step "$T/guard.sh" "out:0"
  expect "zero open batch PRs proceed" 0 "$RC" "$OUT" "proceeding with batch generation"
  expect "zero open batch PRs set should_skip=false" 0 "$RC" "$GHOUT" "should_skip=false"
else
  report FAIL "extract the batch guard step"
fi

echo "release.yml: Upload assets to draft release or create one"

if step_run "$WF/release.yml" release "Upload assets to draft release or create one" > "$T/release.sh"; then
  run_step "$T/release.sh" fail
  expect "a failed gh release view stops the step and shows gh's error" 1 "$RC" "$OUT" \
    "$STUB_ERR" "could not determine whether a release exists"
  lacks "a failed gh release view never goes on to create a release" "$CALLS" "gh release create"

  run_step "$T/release.sh" notfound
  expect "'release not found' takes the create path" 0 "$RC" "$OUT" "No draft release found"
  expect "'release not found' creates a draft release" 0 "$RC" "$CALLS" "gh release create v9.9.9 --draft"

  run_step "$T/release.sh" "out:true"
  expect "an existing draft gets the assets uploaded" 0 "$RC" "$CALLS" "gh release upload v9.9.9"
  lacks "an existing draft is not created again" "$CALLS" "gh release create"
else
  report FAIL "extract the release upload step"
fi

echo "r2-cost-monitoring.yml, r2-credential-rotation-reminder.yml: labels"

# The runtime `gh label create ... || true` calls are gone. What stands in for them is the
# label reference check, so these rows prove it fails, naming the label, when either label
# is missing from the manifest.
for f in r2-cost-monitoring.yml r2-credential-rotation-reminder.yml; do
  if grep -En '^[^#]*gh label create' "$WF/$f" > /dev/null; then
    report FAIL "$f creates labels at run time" "$(grep -En '^[^#]*gh label create' "$WF/$f")"
  else
    report PASS "$f does not create labels at run time"
  fi
done
for label in r2-cost-alert maintenance; do
  grep -vFx -- "- name: \"$label\"" "$REPO_ROOT/.github/labels.yml" > "$T/labels-without-$label.yml"
  if cmp -s "$REPO_ROOT/.github/labels.yml" "$T/labels-without-$label.yml"; then
    report FAIL "remove $label from a copy of the manifest" "it was not declared there to begin with"
    continue
  fi
  OUT=$(python3 "$REPO_ROOT/.github/scripts/checks/label-references.py" \
        --manifest "$T/labels-without-$label.yml" --workflows "$WF" 2>&1); RC=$?
  expect "a manifest without $label fails the label check, naming it" 1 "$RC" "$OUT" \
    "label \"$label\" is referenced but not declared"
done

echo "scripts/validate-golden-exclusions.sh --check-issues"

cat > "$T/exclusions.json" <<'JSON'
{"exclusions": [{"recipe": "demo", "issue": "https://github.com/tsukumogami/tsuku/issues/1", "reason": "fixture"}]}
JSON
run_excl() {  # run_excl <stub mode> : sets OUT and RC
  OUT=$(PATH="$T/bin:$PATH" STUB_MODE="$1" STUB_ERR="$STUB_ERR" STUB_CALLS="$T/calls" GITHUB_TOKEN=stub \
        "$REPO_ROOT/scripts/validate-golden-exclusions.sh" --file "$T/exclusions.json" --check-issues 2>&1); RC=$?
}
run_excl fail
expect "a failed issue lookup counts as invalid and shows gh's error" 2 "$RC" "$OUT" \
  "Could not fetch issue status: $STUB_ERR" "1 invalid exclusion(s) found"
run_excl "out:open"
expect "an open issue passes" 0 "$RC" "$OUT" "OK: Issue is open"
run_excl "out:closed"
expect "a closed issue is reported stale" 1 "$RC" "$OUT" "STALE: Issue is closed"

echo ""
echo "gh-failures: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
