#!/usr/bin/env bash
set -uo pipefail

# Regression cases for the coverage receipt (#2608): the assertion, the declared set, the
# receipt lines validate-golden.sh writes, and the Nightly Registry Validation steps that
# produce them.
#
# Most rows assert on what a component SAYS about its work -- the counts on the
# assertion's summary line, the outcome on each receipt line -- against a truth the row
# constructed. A suite that only checks exit codes would pass a tool whose account of
# itself had drifted, which is the defect this contract exists to remove.
#
# The workflow rows run the step text extracted from the workflow file itself, under the
# shell Actions uses for an unspecified `shell:` (`bash -e`, no pipefail), so the row
# fails if the file stops doing what the row says rather than if a copy of it does.

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
ASSERT="$REPO_ROOT/.github/scripts/checks/assert-coverage.py"
DECLARE="$REPO_ROOT/.github/scripts/golden-declared-set.sh"
VALIDATE="$REPO_ROOT/scripts/validate-golden.sh"
NRV="$REPO_ROOT/.github/workflows/nightly-registry-validation.yml"
PASS_OUT="match,excluded,no-recipe,unsupported-platform,executed"
FAIL_OUT="mismatch,eval-failed,missing-platforms,failed,no-eligible-recipe"

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

T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

run_assert() {  # run_assert <leg spec>... ; sets OUT and RC
  local args=() spec
  for spec in "$@"; do args+=(--leg "$spec"); done
  OUT=$(python3 "$ASSERT" "${args[@]}" --pass-outcomes "$PASS_OUT" --fail-outcomes "$FAIL_OUT" 2>&1); RC=$?
}

echo "assert-coverage.py"

printf '%s\n' a b c > "$T/decl3"
printf '%s\n' '{"item":"a","outcome":"match"}' '{"item":"b","outcome":"excluded"}' '{"item":"c","outcome":"match"}' > "$T/full"
run_assert "leg=$T/decl3:$T/full"
expect "a covered leg passes and reports what it covered" 0 "$RC" "$OUT" \
  "leg: declared 3, attempted 3, not validated 0, undeclared 0" "excluded 1, match 2" "1 leg(s) asserted, all covered"

: > "$T/empty"
run_assert "leg=$T/decl3:$T/empty"
expect "an empty receipt fails as attempted nothing" 1 "$RC" "$OUT" \
  "leg: declared 3, attempted 0, not validated 3" "attempted nothing" "::error::leg did not cover"

run_assert "leg=$T/decl3:$T/nonexistent"
expect "a missing receipt fails, saying the leg did not run or was killed" 1 "$RC" "$OUT" \
  "declared 3, attempted 0 -- no receipt" "killed before uploading"

run_assert "leg=$T/nonexistent:$T/full"
expect "a missing declared set fails rather than passing over nothing" 1 "$RC" "$OUT" "no declared set"

head -2 "$T/full" > "$T/partial"
run_assert "leg=$T/decl3:$T/partial"
expect "a leg that stopped short fails, naming both numbers and the gap" 1 "$RC" "$OUT" \
  "declared 3, attempted 2, not validated 1" "1 declared item(s) have no receipt line: c"

cp "$T/full" "$T/extra"; echo '{"item":"z","outcome":"match"}' >> "$T/extra"
run_assert "leg=$T/decl3:$T/extra"
expect "a line naming an undeclared item fails" 1 "$RC" "$OUT" "undeclared 1" "name undeclared items: z"

printf '%s\n' '{"item":"a","outcome":"match"}' '{"item":"b","outcome":"mismatch"}' '{"item":"c","outcome":"eval-failed"}' > "$T/bad"
run_assert "leg=$T/decl3:$T/bad"
expect "failure outcomes fail the leg even when every item was attempted" 1 "$RC" "$OUT" \
  "declared 3, attempted 3" "2 item(s) failed: b, c"

printf '%s\n' '{"item":"a","outcome":"match"}' '{"item":"b","outcome":"match"}' '{"item":"c","outcome":"looked-at-it"}' > "$T/unknown"
run_assert "leg=$T/decl3:$T/unknown"
expect "an outcome in neither list fails" 1 "$RC" "$OUT" "unknown outcome(s): looked-at-it"

printf '%s\n' '{"item":"a","outcome":"match"}' 'not json' '{"item":"b"}' '{"item":"c","outcome":"match"}' > "$T/malformed"
run_assert "leg=$T/decl3:$T/malformed"
expect "malformed lines fail, and are not counted" 1 "$RC" "$OUT" "attempted 2" "2 malformed receipt line(s), first at line 2"

cat "$T/full" "$T/full" > "$T/dup"
run_assert "leg=$T/decl3:$T/dup"
expect "an item reported twice fails" 1 "$RC" "$OUT" "3 item(s) appear more than once"

: > "$T/decl0"
run_assert "leg=$T/decl0:$T/empty"
expect "declared nothing and attempted nothing still fails" 1 "$RC" "$OUT" "declared 0, attempted 0" "attempted nothing"

run_assert "good=$T/decl3:$T/full" "gone=$T/decl3:$T/nonexistent"
expect "one missing leg fails the run, and the covered leg is still reported" 1 "$RC" "$OUT" \
  "good: declared 3, attempted 3" "gone: declared 3, attempted 0 -- no receipt" "2 leg(s) asserted, NOT all covered"

OUT=$(python3 "$ASSERT" --leg "x=$T/decl3:$T/full" --pass-outcomes match --fail-outcomes match 2>&1); RC=$?
expect "an outcome in both lists is a usage error" 2 "$RC" "$OUT" "in both lists"

echo "golden-declared-set.sh"

cat > "$T/keys-registry" <<'EOF'
plans/registry/fzf/v0.60.0/linux-amd64.json
plans/registry/fzf/v0.60.0/darwin-arm64.json
plans/registry/jq/v1.7.1/linux-debian-amd64.json
plans/embedded/go/v1.22.0/linux-amd64.json
plans/registry/fzf/README.txt
EOF
sed -e 's#^plans/registry/fzf/#plans/f/fzf/#' -e 's#^plans/registry/jq/#plans/j/jq/#' "$T/keys-registry" > "$T/keys-letter"
OUT=$(bash "$DECLARE" --out-dir "$T/dreg" --exclude-prefix plans/embedded/ < "$T/keys-registry" 2>&1); RC=$?
expect "a listing declares per OS and says what it did with every key" 0 "$RC" "$OUT" \
  "5 key(s) listed, 1 excluded by prefix, 1 not golden files; darwin 1; linux 2."
if [ "$(cat "$T/dreg/declared-linux.txt" 2>/dev/null)" = $'fzf/v0.60.0-linux-amd64\njq/v1.7.1-linux-debian-amd64' ] &&
   [ "$(cat "$T/dreg/declared-darwin.txt" 2>/dev/null)" = "fzf/v0.60.0-darwin-arm64" ]; then
  report PASS "items are named the way validate-golden.sh names them"
else
  report FAIL "items are named the way validate-golden.sh names them" "$(cat "$T"/dreg/* 2>&1)"
fi
bash "$DECLARE" --out-dir "$T/dlet" --exclude-prefix plans/embedded/ < "$T/keys-letter" > /dev/null 2>&1
if diff -r "$T/dreg" "$T/dlet" > /dev/null && [ -s "$T/dlet/declared-linux.txt" ]; then
  report PASS "the category segment (registry or a letter) does not change the declared set"
else
  report FAIL "the category segment (registry or a letter) does not change the declared set" "$(diff -r "$T/dreg" "$T/dlet" 2>&1)"
fi
OUT=$(bash "$DECLARE" --out-dir "$T/dnone" < /dev/null 2>&1); RC=$?
if [ "$RC" = 0 ] && [ -f "$T/dnone/declared-linux.txt" ] && [ ! -s "$T/dnone/declared-linux.txt" ] && [ -f "$T/dnone/declared-darwin.txt" ]; then
  report PASS "an empty listing declares zero for each leg rather than nothing"
else
  report FAIL "an empty listing declares zero for each leg rather than nothing" "rc=$RC $OUT"
fi

echo "validate-golden.sh receipt lines"

# A sandbox repository: the real script, a stub tsuku, and recipes whose golden files
# construct each outcome. The stub reads a recipe's platforms from a sidecar file and
# echoes the golden file back as the generated plan, unless the recipe is set up to drift
# or to fail generation.
S="$T/repo"
mkdir -p "$S/scripts" "$S/testdata/golden" "$S/golden"
cp "$VALIDATE" "$S/scripts/validate-golden.sh"
mkdir -p "$S/scripts/lib"
cp "$(dirname "$VALIDATE")/lib/r2-layout.sh" "$S/scripts/lib/"
cat > "$S/tsuku" <<'EOF'
#!/usr/bin/env bash
cmd="$1"; shift
recipe="" pin=""
while [ $# -gt 0 ]; do
  case "$1" in
    --recipe) recipe="$2"; shift 2 ;;
    --pin-from) pin="$2"; shift 2 ;;
    *) shift ;;
  esac
done
case "$cmd" in
  info) printf '{"supported_platforms":%s}\n' "$(cat "$recipe.platforms")" ;;
  eval)
    case "$recipe" in
      *evalfail*) exit 1 ;;
      *drift*) jq '.changed = true' "$pin" ;;
      *) cat "$pin" ;;
    esac ;;
esac
EOF
chmod +x "$S/tsuku"
echo '{"exclusions":[{"recipe":"skipme","reason":"toolchain drift","issue":"#1"}]}' > "$S/testdata/golden/code-validation-exclusions.json"
echo '{"exclusions":[]}' > "$S/testdata/golden/exclusions.json"
LINUX='[{"os":"linux","arch":"amd64"}]'
mkrecipe() {  # mkrecipe <name> <platforms json|-> <golden file>...
  local name="$1" platforms="$2" l="${1:0:1}"; shift 2
  if [ "$platforms" != "-" ]; then
    mkdir -p "$S/recipes/$l"
    echo "[metadata]" > "$S/recipes/$l/$name.toml"
    echo "$platforms" > "$S/recipes/$l/$name.toml.platforms"
  fi
  mkdir -p "$S/golden/$l/$name"
  local g
  for g in "$@"; do echo "{\"recipe\":\"$name\",\"file\":\"$g\"}" > "$S/golden/$l/$name/$g.json"; done
}
mkrecipe fine '[{"os":"linux","arch":"amd64"},{"os":"darwin","arch":"arm64"}]' \
  v1.0.0-linux-amd64 v1.0.0-darwin-arm64 v1.0.0-linux-alpine-amd64
mkrecipe drift "$LINUX" v2.0.0-linux-amd64
mkrecipe evalfail "$LINUX" v3.0.0-linux-amd64
mkrecipe skipme "$LINUX" v4.0.0-linux-amd64
mkrecipe ghost - v5.0.0-linux-amd64
mkrecipe gap '[{"os":"darwin","arch":"amd64"},{"os":"darwin","arch":"arm64"}]' v6.0.0-darwin-arm64

vg() {  # vg <recipe> <os> ; appends to $S/receipt, sets RC
  COVERAGE_RECEIPT="$S/receipt" GITHUB_TOKEN=stub \
    bash "$S/scripts/validate-golden.sh" "$1" --os "$2" --category registry --golden-dir "$S/golden" > "$S/vg.log" 2>&1
  RC=$?
}
lines_for() { grep -F "\"item\":\"$1/" "$S/receipt" | sort | tr '\n' ' '; }
check_lines() {  # check_lines <desc> <recipe> <want exit> <want lines, sorted, space-joined>
  local got; got=$(lines_for "$2")
  if [ "$RC" = "$3" ] && [ "$got" = "$4" ]; then report PASS "$1"
  else report FAIL "$1" "exit $RC (wanted $3); lines: $got; log: $(tail -3 "$S/vg.log")"; fi
}
: > "$S/receipt"
vg fine linux
check_lines "a compared file reads match; a file for a platform the recipe dropped is named, not skipped" fine 0 \
  '{"item":"fine/v1.0.0-linux-alpine-amd64","outcome":"unsupported-platform"} {"item":"fine/v1.0.0-linux-amd64","outcome":"match"} '
vg drift linux
check_lines "a differing plan reads mismatch" drift 1 '{"item":"drift/v2.0.0-linux-amd64","outcome":"mismatch"} '
vg evalfail linux
check_lines "a plan that fails to generate reads eval-failed, and the script exits 1" evalfail 1 \
  '{"item":"evalfail/v3.0.0-linux-amd64","outcome":"eval-failed"} '
vg skipme linux
check_lines "a code-validation exclusion reads excluded" skipme 0 '{"item":"skipme/v4.0.0-linux-amd64","outcome":"excluded"} '
vg ghost linux
check_lines "golden files with no recipe read no-recipe" ghost 2 '{"item":"ghost/v5.0.0-linux-amd64","outcome":"no-recipe"} '
vg gap darwin
check_lines "files held back by a missing platform read missing-platforms" gap 1 \
  '{"item":"gap/v6.0.0-darwin-arm64","outcome":"missing-platforms"} '
: > "$S/receipt"
vg fine darwin
check_lines "a leg writes lines only for its own OS" fine 0 '{"item":"fine/v1.0.0-darwin-arm64","outcome":"match"} '
: > "$S/receipt"
vg drift darwin
check_lines "a recipe with no files for the leg's OS writes nothing, so it cannot inflate the count, and exits 3 (not compared)" drift 3 ''
OUT=$(COVERAGE_RECEIPT="$S/receipt" GITHUB_TOKEN=stub bash "$S/scripts/validate-golden.sh" fine --golden-dir "$S/golden" 2>&1); RC=$?
expect "a receipt without --os is refused" 2 "$RC" "$OUT" "COVERAGE_RECEIPT needs --os"

echo "nightly-registry-validation.yml steps"

step_run() {  # step_run <job> <step name> : prints the step's run text
  python3 - "$NRV" "$1" "$2" <<'PY'
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

W="$T/ws"
mkdir -p "$W/scripts"
cat > "$W/scripts/validate-all-golden.sh" <<'EOF'
#!/usr/bin/env bash
printf 'Failed recipes (1 of 1):\n  - broken-recipe\n\nChecked 1 recipes: 0 matched, 1 failed, 0 not compared\n'
exit 1
EOF
chmod +x "$W/scripts/validate-all-golden.sh"
step_run validate-plans-linux "Validate registry golden files" > "$W/validate.sh" || report FAIL "extract the Linux validate step"
rm -rf "$W/results"
OUT=$(cd "$W" && GITHUB_OUTPUT="$W/out" GITHUB_STEP_SUMMARY="$W/summary" COVERAGE_RECEIPT="$W/r.ndjson" GOLDEN_DIR=x SHARD=0 SHARD_COUNT=8 bash -e validate.sh 2>&1); RC=$?
if [ "$RC" != 0 ] && [ "$(cat "$W/results/failed.txt" 2>/dev/null)" = broken-recipe ]; then
  report PASS "a failing validator fails the Linux validate step (it would pass through tee without pipefail)"
else
  report FAIL "a failing validator fails the Linux validate step" "exit $RC; failed list: $(cat "$W/results/failed.txt" 2>&1); $OUT"
fi
step_run validate-plans-macos "Validate registry golden files" > "$W/validate-mac.sh" || report FAIL "extract the macOS validate step"
rm -f "$W/out"
rm -rf "$W/results"
OUT=$(cd "$W" && GITHUB_OUTPUT="$W/out" GITHUB_STEP_SUMMARY="$W/summary" COVERAGE_RECEIPT="$W/r.ndjson" GOLDEN_DIR=x SHARD=0 SHARD_COUNT=8 bash -e validate-mac.sh 2>&1); RC=$?
if [ "$RC" != 0 ] && [ "$(cat "$W/results/failed.txt" 2>/dev/null)" = broken-recipe ]; then
  report PASS "a failing validator fails the macOS validate step"
else
  report FAIL "a failing validator fails the macOS validate step" "exit $RC; failed list: $(cat "$W/results/failed.txt" 2>&1); $OUT"
fi

# Execute Sample over a flattened download holding eligible recipes for a and b only.
E="$T/sample"
mkdir -p "$E/r2-golden-files/plans/a/alpha" "$E/r2-golden-files/plans/b/beta" "$E/r2-golden-files/plans/c/gamma" "$E/tmp"
echo '{}' > "$E/r2-golden-files/plans/a/alpha/v1.0.0-linux-amd64.json"
echo '{}' > "$E/r2-golden-files/plans/b/beta/v2.0.0-linux-debian-amd64.json"
echo '{}' > "$E/r2-golden-files/plans/c/gamma/v3.0.0-darwin-arm64.json"
printf '#!/usr/bin/env bash\nexit 0\n' > "$E/tsuku"; chmod +x "$E/tsuku"
step_run execute-sample-linux "Execute sample registry recipes" | sed "s#\${{ runner.temp }}#$E/tmp#g" > "$E/sample.sh" ||
  report FAIL "extract the Execute Sample step"
if grep -q '\${{' "$E/sample.sh"; then report FAIL "Execute Sample step has an expression this row does not substitute"; fi
OUT=$(cd "$E" && GITHUB_OUTPUT="$E/out" GITHUB_PATH="$E/path" GITHUB_STEP_SUMMARY="$E/summary" bash -e sample.sh 2>&1); RC=$?
got=$(tr '\n' ' ' < "$E/coverage/receipt-execute-sample.ndjson" 2>/dev/null)
want='{"item":"a","outcome":"executed"} {"item":"b","outcome":"executed"} {"item":"c","outcome":"no-eligible-recipe"}'
if [ "$RC" = 0 ] && [ "$(wc -l < "$E/coverage/receipt-execute-sample.ndjson" 2>/dev/null)" = 26 ] && [[ "$got" == "$want"* ]]; then
  report PASS "Execute Sample finds flattened golden files and writes a line for each of the 26 letters"
else
  report FAIL "Execute Sample finds flattened golden files and writes a line for each of the 26 letters" "exit $RC; receipt: $got; $OUT"
fi
printf '%s\n' {a..z} > "$E/declared"
run_assert "execute-sample=$E/declared:$E/coverage/receipt-execute-sample.ndjson"
expect "and the assertion fails the letters that had nothing to run" 1 "$RC" "$OUT" \
  "execute-sample: declared 26, attempted 26" "24 item(s) failed"

# --- states the first scheduled run met (run 36304210984, 2026-09-27) ----------------------
#
# The download job died before it uploaded anything, so Assert Coverage started in a
# workspace with no coverage/ directory at all. Its first write failed ("No such file or
# directory") and the run ended red without ever naming a leg or a count. These rows run
# the assert step as written in that state, and check the upload that would have given it
# a declared set to report against.

A="$T/assert-empty"
mkdir -p "$A"
cp -r "$REPO_ROOT/.github" "$A/.github"
step_run assert-coverage "Assert every leg covered its declared set" > "$A/assert.sh" ||
  report FAIL "extract the Assert Coverage step"
OUT=$(cd "$A" && R2_AVAILABLE=true HEALTH_STATUS=healthy GITHUB_STEP_SUMMARY="$A/summary" bash -e assert.sh 2>&1); RC=$?
if [ "$RC" != 0 ] && ! grep -qF 'No such file or directory' <<< "$OUT" \
   && grep -qF 'validate-linux: no declared set' <<< "$OUT" \
   && grep -qF 'validate-darwin: no declared set' <<< "$OUT" \
   && grep -qF 'execute-sample: declared 26, attempted 0 -- no receipt' <<< "$OUT" \
   && grep -qF 'NOT all covered' <<< "$OUT"; then
  report PASS "with nothing downloaded, Assert Coverage still names every leg and fails on the counts"
else
  report FAIL "with nothing downloaded, Assert Coverage still names every leg and fails on the counts" "exit $RC: $(tail -5 <<< "$OUT")"
fi

OUT=$(python3 - "$NRV" <<'PY' 2>&1
import sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
step = next(s for s in wf["jobs"]["download-golden-files"]["steps"] if s.get("name") == "Upload declared set")
ok = step.get("if") == "always()" and (step.get("with") or {}).get("if-no-files-found") == "error"
print("ok" if ok else f"if={step.get('if')!r} if-no-files-found={(step.get('with') or {}).get('if-no-files-found')!r}")
sys.exit(0 if ok else 1)
PY
); RC=$?
expect "the declared set is uploaded with always(), so a later failure in its job doesn't take it down too" 0 "$RC" "$OUT" "ok"

# The assertion's legs have to be the receipts the workflow uploads. A leg renamed on one
# side only would be reported missing every night, or never checked at all.
OUT=$(python3 - "$NRV" <<'PY' 2>&1
import re, sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
uploads = {}
sharded = {}
for job_id, job in wf["jobs"].items():
    for step in job.get("steps", []):
        w = step.get("with") or {}
        if "upload-artifact" in step.get("uses", "") and str(w.get("name", "")).startswith("coverage-receipt-"):
            ok = step.get("if") == "always()" and w.get("if-no-files-found") == "error"
            name = str(w["name"])
            if name.endswith("-${{ matrix.shard }}"):
                name = name[: -len("-${{ matrix.shard }}")]
                shards = [str(x) for x in (job.get("strategy") or {}).get("matrix", {}).get("shard", [])]
                sharded[name] = (job_id, shards)
            uploads[name] = (w["path"], ok, job_id)
run = next(s["run"] for s in wf["jobs"]["assert-coverage"]["steps"] if s.get("name") == "Assert every leg covered its declared set")
legs = dict(re.findall(r"--leg ([\w-]+)=\S+?:coverage/(coverage-receipt-[\w-]+)/", run))
problems = []
for name, (path, ok, job_id) in uploads.items():
    if not ok:
        problems.append(f"{job_id}: {name} is not uploaded with always() and if-no-files-found: error")
    if name not in legs.values():
        problems.append(f"{name} is uploaded but never asserted")
for leg, art in legs.items():
    if art not in uploads:
        problems.append(f"leg {leg} asserts {art}, which nothing uploads")
joined = re.search(r"for shard in ([\d ]+); do", run)
joined = joined.group(1).split() if joined else []
for name, (job_id, shards) in sharded.items():
    if not shards:
        problems.append(f"{job_id}: {name} is uploaded per shard but the job has no shard matrix")
    elif shards != joined:
        problems.append(f"{job_id} runs shards {shards} but assert-coverage joins {joined}")
    for step in wf["jobs"][job_id]["steps"]:
        count = (step.get("env") or {}).get("SHARD_COUNT")
        if count is not None and str(count) != str(len(shards)):
            problems.append(f"{job_id}: SHARD_COUNT {count} but the matrix runs {len(shards)} shards")
needs = set(wf["jobs"]["assert-coverage"]["needs"])
for job_id in {j for (_, _, j) in uploads.values()}:
    if job_id not in needs:
        problems.append(f"assert-coverage does not wait for {job_id}")
if wf["jobs"]["assert-coverage"].get("if") != "always()":
    problems.append("assert-coverage does not run with always()")
print("\n".join(problems) or f"{len(uploads)} receipt(s) uploaded, {len(legs)} leg(s) asserted")
sys.exit(1 if problems else 0)
PY
); RC=$?
expect "every uploaded receipt is asserted, every asserted leg is uploaded, always()" 0 "$RC" "$OUT" \
  "3 receipt(s) uploaded, 3 leg(s) asserted"

echo
echo "coverage-receipt self-test: $pass passed, $fail failed."
[ "$fail" -eq 0 ]
