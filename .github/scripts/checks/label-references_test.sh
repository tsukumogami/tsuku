#!/usr/bin/env bash
set -uo pipefail

# Proves label-references.py can fail, and fails for the reason claimed.
#
# A check nobody has seen fail is a check nobody knows works. Each case below mutates a
# copy of the inputs, runs the check, and asserts both the exit code and the text. Every
# mutating case hashes its target before and after and VOIDS ITSELF if the file did not
# change -- a mutation that silently applied to nothing would otherwise report the exit
# code it was hoping for and pass.

CHECK="$(dirname "$0")/label-references.py"
FIXTURE="$(dirname "$0")/testdata/label-references/2026-09-20"
REPO_ROOT="$(git rev-parse --show-toplevel)"

pass=0; fail=0; void=0

report() {  # report <outcome> <name> [detail]
  case "$1" in
    PASS) pass=$((pass + 1)); echo "  [PASS] $2" ;;
    FAIL) fail=$((fail + 1)); echo "  [FAIL] $2${3:+ -- $3}" >&2 ;;
    VOID) void=$((void + 1)); echo "  [VOID] $2${3:+ -- $3}" >&2 ;;
  esac
}

# assert_run <name> <expected-exit> <expected-substring> -- <check args...>
assert_run() {
  local name="$1" want_exit="$2" want_text="$3"; shift 4
  local out rc
  out=$(python3 "$CHECK" "$@" 2>&1); rc=$?
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

echo "Control cases"

# The repository as it stands must pass. If this fails the check is wrong about today,
# and every mutation below would be measuring the wrong baseline.
assert_run "working tree passes" 0 "" -- \
  --manifest "$REPO_ROOT/.github/labels.yml" --workflows "$REPO_ROOT/.github/workflows"

# The frozen pre-repair snapshot must report the exact finding this check was built for.
assert_run "fixture reports nine names" 1 "9 undeclared name(s) at 15 site(s)" -- \
  --manifest "$FIXTURE/labels.yml" --workflows "$FIXTURE/workflows"

# One run, no per-site configuration: the variable that resolves to an existing label is
# quiet and the one that resolves to a missing label is reported. Both are `$LABEL`.
assert_run "variable resolving to a live label is not reported" 1 "" -- \
  --manifest "$FIXTURE/labels.yml" --workflows "$FIXTURE/workflows"
# Capture first, then match. Piping the check straight into grep would let `pipefail`
# surface the check's own exit 1 as the pipeline's status, which is the expected exit
# here and would make the assertion read backwards.
fixture_out=$(python3 "$CHECK" --manifest "$FIXTURE/labels.yml" --workflows "$FIXTURE/workflows" 2>&1)
if printf '%s' "$fixture_out" | grep -q 'r2-degradation'; then
  report FAIL "variable resolving to a live label is not reported" "r2-degradation was reported"
fi
if printf '%s' "$fixture_out" | grep -q 'r2-cost-alert.*via \$LABEL'; then
  report PASS "variable resolving to a missing label is reported, via \$LABEL"
else
  report FAIL "variable resolving to a missing label is reported" "no \$LABEL attribution"
fi

# The container-build `labels:` value is wholly a ${{ }} expression -- OCI image labels
# fed to a build, not issue labels. It must be excluded by the SHAPE of the value, so the
# assertion is that the file is never mentioned, from a fixture that contains it. If the
# exclusion regressed it would surface as an unresolvable reference, which the
# working-tree row above would also catch by failing to exit 0.
if printf '%s' "$fixture_out" | grep -q 'container-build'; then
  report FAIL "a wholly-\${{ }} labels value is excluded" "container-build.yml was reported"
else
  report PASS "a wholly-\${{ }} labels value is excluded"
fi

echo "Mutation cases"

# run_mutation <name> <expected-exit> <expected-text> <target-relative> <mutator>
# The mutator receives the sandbox root as $1 and edits files in place.
run_mutation() {
  local name="$1" want_exit="$2" want_text="$3" target="$4" mutator="$5"
  local sandbox before after out rc
  sandbox=$(mktemp -d)
  cp -r "$FIXTURE/workflows" "$sandbox/workflows"
  cp "$FIXTURE/labels.yml" "$sandbox/labels.yml"

  before=$(find "$sandbox/$target" -type f -exec sha256sum {} \; 2>/dev/null | sort | sha256sum)
  "$mutator" "$sandbox"
  after=$(find "$sandbox/$target" -type f -exec sha256sum {} \; 2>/dev/null | sort | sha256sum)

  if [ "$before" = "$after" ]; then
    report VOID "$name" "mutation changed nothing; the row proves nothing"
    rm -rf "$sandbox"
    return
  fi

  out=$(python3 "$CHECK" --manifest "$sandbox/labels.yml" --workflows "$sandbox/workflows" 2>&1)
  rc=$?
  if [ "$rc" -ne "$want_exit" ]; then
    report FAIL "$name" "exit $rc, wanted $want_exit"
  elif [ -n "$want_text" ] && ! printf '%s' "$out" | grep -qF -- "$want_text"; then
    report FAIL "$name" "output did not contain: $want_text"
  else
    report PASS "$name"
  fi
  rm -rf "$sandbox"
}

strip_all_labels() {
  # Remove every label argument everywhere. The check must fail on the zero-reference
  # floor rather than congratulate itself on finding no undeclared names.
  find "$1/workflows" -type f -print0 | xargs -0 sed -i -E \
    -e 's/--(add-)?label[= ]+("[^"]*"|'"'"'[^'"'"']*'"'"'|[^ ]+)//g' \
    -e 's/(^|[^-])labels:.*/\1/'
}

add_unresolvable() {
  printf '\n      - run: gh issue create --label "$NEVER_ASSIGNED"\n' \
    >> "$1/workflows/curated-nightly.yml"
}

delete_live_label() {
  # r2-degradation is referenced only through $LABEL, so its name appears nowhere on the
  # lines that will be reported. Removing it proves the resolver ran: a check that only
  # matched literals could not name those lines at all.
  python3 - "$1/labels.yml" <<'PY'
import sys, pathlib
p = pathlib.Path(sys.argv[1]); out = []; skip = False
for line in p.read_text().splitlines():
    if line.startswith('- name: "'):
        skip = line == '- name: "r2-degradation"'
    if not skip:
        out.append(line)
p.write_text("\n".join(out) + "\n")
PY
}

empty_manifest() { : > "$1/labels.yml"; }

run_mutation "every label argument stripped -> zero-reference floor" \
  1 "found no label reference at all" workflows strip_all_labels

run_mutation "unresolvable reference is reported with file and line" \
  1 "cannot be reduced to a literal" workflows add_unresolvable

run_mutation "deleting an in-use label names its referencing lines" \
  1 "r2-health-monitor.yml:123" labels.yml delete_live_label

run_mutation "empty manifest is an operational error, not a pass" \
  2 "declares no labels" labels.yml empty_manifest

# `--add-label` appears nowhere in this repository today, so the fixture cannot exercise
# it. Without this row that half of the extractor would ship unproven.
add_label_flag_form() {
  printf '\n      - run: gh issue edit 1 --add-label "not-a-real-label"\n' \
    >> "$1/workflows/curated-nightly.yml"
}

# A comma-joined value names several labels, because that is how `gh` reads it. Both
# halves must be reported, not just the first.
comma_joined_value() {
  printf '\n      - run: gh issue create --label "absent-alpha,absent-beta"\n' \
    >> "$1/workflows/curated-nightly.yml"
}

run_mutation "--add-label is extracted like --label" \
  1 "not-a-real-label" workflows add_label_flag_form

# Asserted against the quoted name, not a bare substring: an extractor that did NOT
# split would report the whole `absent-alpha,absent-beta` as one undeclared name, and a
# bare `absent-beta` match would be satisfied by that string too.
run_mutation "a comma-joined value is split into separate names" \
  1 'label "absent-beta" is referenced' workflows comma_joined_value

echo
echo "label-references self-test: $pass passed, $fail failed, $void void."
[ "$fail" -eq 0 ] && [ "$void" -eq 0 ]
