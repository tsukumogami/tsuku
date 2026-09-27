#!/usr/bin/env bash
# Self-test for the R2 golden-plan layout and the tools that read it.
#
# Hermetic: no network, no R2, no tsuku binary. Covers
#   - scripts/lib/r2-layout.sh: key building, parsing, legacy mapping
#   - scripts/r2-golden-transform.sh: current wins over legacy, counts, empty mirror fails
#   - scripts/validate-all-golden.sh: per-recipe results are counted as matched, failed or
#     not compared; the summary line states those counts; shards partition the recipes;
#     a run that compared nothing does not exit 0
#
# Every case asserts on what the tool REPORTS as well as what it does, because a tool's
# account of its own work is the part nothing else checks.
#
# Usage: scripts/lib/r2-layout_test.sh

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS="$(cd "$HERE/.." && pwd)"
# shellcheck source=r2-layout.sh
source "$HERE/r2-layout.sh"

PASS=0
FAIL=0
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

ok()   { PASS=$((PASS + 1)); echo "ok   $1"; }
bad()  { FAIL=$((FAIL + 1)); echo "FAIL $1"; }
eq()   { if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1: expected [$3], got [$2]"; fi; }
fails() { local name="$1"; shift; if "$@" > /dev/null 2>&1; then bad "$name: expected failure"; else ok "$name"; fi; }

# --- key building ------------------------------------------------------------------------

eq "registry key uses first letter" "$(r2_plan_key fzf registry 0.60.0 linux-amd64)" "plans/f/fzf/v0.60.0/linux-amd64.json"
eq "embedded key uses embedded" "$(r2_plan_key go embedded 1.25.5 darwin-arm64)" "plans/embedded/go/v1.25.5/darwin-arm64.json"
eq "leading v is not doubled" "$(r2_plan_key fzf registry v0.60.0 linux-amd64)" "plans/f/fzf/v0.60.0/linux-amd64.json"
eq "registry prefix" "$(r2_plan_prefix openssl@3 registry)" "plans/o/openssl@3/"
fails "a segment is never accepted as a category" r2_plan_key fzf f 0.60.0 linux-amd64
fails "the legacy word is not a category" r2_plan_prefix fzf legacy
fails "empty recipe" r2_plan_prefix "" registry

# --- segments ----------------------------------------------------------------------------

eq "segment: letter is registry" "$(r2_segment_category f)" "registry"
eq "segment: embedded" "$(r2_segment_category embedded)" "embedded"
fails "segment: legacy word is not a segment" r2_segment_category registry
fails "segment: two letters is not a segment" r2_segment_category fz
fails "segment: uppercase is not a segment" r2_segment_category F

# --- parsing -----------------------------------------------------------------------------

if r2_parse_plan_key plans/f/fzf/v0.60.0/linux-debian-amd64.json; then
    eq "parse: recipe" "$R2_KEY_RECIPE" "fzf"
    eq "parse: category" "$R2_KEY_CATEGORY" "registry"
    eq "parse: version has no v" "$R2_KEY_VERSION" "0.60.0"
    eq "parse: platform" "$R2_KEY_PLATFORM" "linux-debian-amd64"
else
    bad "parse: current registry key"
fi
if r2_parse_plan_key plans/embedded/go/v1.25.5/linux-amd64.json; then
    eq "parse: embedded category" "$R2_KEY_CATEGORY" "embedded"
else
    bad "parse: embedded key"
fi
fails "parse: legacy registry key is outside the layout" r2_parse_plan_key plans/registry/fzf/v0.60.0/linux-amd64.json
fails "parse: segment must match recipe" r2_parse_plan_key plans/g/fzf/v0.60.0/linux-amd64.json
fails "parse: not a plan" r2_parse_plan_key health/ping.json
fails "parse: missing version dir" r2_parse_plan_key plans/f/fzf/linux-amd64.json
r2_parse_plan_key plans/registry/fzf/v1/linux-amd64.json || true
eq "parse: failure clears fields" "$R2_KEY_RECIPE" ""

# Round trip: every key built from the layout parses back to what built it.
for spec in "fzf registry 0.60.0 linux-amd64" "go embedded 1.25.5 darwin-arm64" "node registry 22.1.0-rc.1 linux-alpine-amd64"; do
    read -r r c v p <<< "$spec"
    key=$(r2_plan_key "$r" "$c" "$v" "$p")
    if r2_parse_plan_key "$key"; then
        eq "round trip $r" "$R2_KEY_RECIPE/$R2_KEY_CATEGORY/$R2_KEY_VERSION/$R2_KEY_PLATFORM" "$r/$c/$v/$p"
    else
        bad "round trip $r: $key did not parse"
    fi
done

# --- legacy mapping ----------------------------------------------------------------------

eq "legacy maps to letter" "$(r2_legacy_registry_key_to_current plans/registry/vale/v3.13.0/darwin-arm64.json)" "plans/v/vale/v3.13.0/darwin-arm64.json"
fails "legacy: current key is not legacy" r2_legacy_registry_key_to_current plans/v/vale/v3.13.0/darwin-arm64.json
fails "legacy: embedded is not legacy" r2_legacy_registry_key_to_current plans/embedded/go/v1/linux-amd64.json

# --- transform ---------------------------------------------------------------------------

M="$TMP/mirror"
mkdir -p "$M/f/fzf/v1.0.0" "$M/registry/fzf/v1.0.0" "$M/registry/vale/v3.13.0" "$M/embedded/go/v1.25.5" "$M/registry/weird"
echo '{"from":"current"}' > "$M/f/fzf/v1.0.0/linux-amd64.json"
echo '{"from":"legacy"}' > "$M/registry/fzf/v1.0.0/linux-amd64.json"
echo '{"from":"legacy-only"}' > "$M/registry/fzf/v1.0.0/darwin-arm64.json"
echo '{"from":"legacy-only"}' > "$M/registry/vale/v3.13.0/linux-amd64.json"
echo '{}' > "$M/embedded/go/v1.25.5/linux-amd64.json"
echo '{}' > "$M/registry/weird/not-a-plan.json"

out=$("$SCRIPTS/r2-golden-transform.sh" "$M" "$TMP/out" 2>&1) && rc=0 || rc=$?
eq "transform: exit" "$rc" "0"
eq "transform: summary" "$(tail -1 <<< "$out")" "Transformed 3 registry plans (1 current layout, 2 legacy fallback); ignored 2 other objects"
eq "transform: current wins over legacy" "$(cat "$TMP/out/f/fzf/v1.0.0-linux-amd64.json")" '{"from":"current"}'
eq "transform: legacy fills a gap" "$(cat "$TMP/out/f/fzf/v1.0.0-darwin-arm64.json")" '{"from":"legacy-only"}'
eq "transform: legacy recipe lands under its letter" "$(cat "$TMP/out/v/vale/v3.13.0-linux-amd64.json")" '{"from":"legacy-only"}'
eq "transform: no registry directory in output" "$([[ -e "$TMP/out/registry" ]] && echo present || echo absent)" "absent"
eq "transform: output file count" "$(find "$TMP/out" -type f | wc -l | tr -d ' ')" "3"

mkdir -p "$TMP/empty"
"$SCRIPTS/r2-golden-transform.sh" "$TMP/empty" "$TMP/out-empty" > "$TMP/empty.log" 2>&1 && rc=0 || rc=$?
eq "transform: empty mirror fails" "$rc" "1"
eq "transform: empty mirror says so" "$(tail -1 "$TMP/empty.log")" "Transformed 0 registry plans (0 current layout, 0 legacy fallback); ignored 0 other objects"

# --- validate-all-golden.sh counting and sharding ----------------------------------------
#
# validate-all-golden.sh runs validate-golden.sh from its own directory, so a copy of it
# next to a stub exercises its bookkeeping without tsuku. The stub's exit code is chosen
# by recipe name: ok-* match, bad-* mismatch, none-* have no platform to compare.

S="$TMP/scripts"
mkdir -p "$S"
cp "$SCRIPTS/validate-all-golden.sh" "$S/"
cat > "$S/validate-golden.sh" <<'STUB'
#!/usr/bin/env bash
echo "$1" >> "${STUB_LOG:?}"
case "$1" in ok-*) exit 0 ;; bad-*) exit 1 ;; none-*) exit 3 ;; *) exit 2 ;; esac
STUB
chmod +x "$S/validate-golden.sh"

G="$TMP/golden"
for r in ok-a ok-b bad-c none-d ok-e; do mkdir -p "$G/${r:0:1}/$r"; done

run_all() {  # run_all <log> <args...>: sets RC and OUT
    local log="$1"; shift
    : > "$log"
    OUT=$(STUB_LOG="$log" "$S/validate-all-golden.sh" --category registry --golden-dir "$G" "$@" 2>&1) && RC=0 || RC=$?
}

run_all "$TMP/all.log"
eq "all: exit on a mismatch" "$RC" "1"
eq "all: summary counts" "$(tail -1 <<< "$OUT")" "Checked 5 recipes: 3 matched, 1 failed, 1 not compared"
eq "all: failed list names the mismatch only" "$(awk '/^Failed recipes \(/ {f=1; next} f && /^  - / {print $2; next} f {exit}' <<< "$OUT")" "bad-c"
eq "all: declared failed count" "$(sed -n 's/^Failed recipes (\([0-9]*\) of [0-9]*):$/\1/p' <<< "$OUT")" "1"

rm -rf "$G/b"
run_all "$TMP/pass.log"
eq "pass: exit" "$RC" "0"
eq "pass: summary" "$(tail -1 <<< "$OUT")" "Checked 4 recipes: 3 matched, 0 failed, 1 not compared"

# Shards: together they run every recipe exactly once, and no shard runs another's.
: > "$TMP/union.log"
for i in 0 1 2; do
    run_all "$TMP/shard$i.log" --shard "$i/3"
    cat "$TMP/shard$i.log" >> "$TMP/union.log"
done
eq "shards: union covers every recipe once" "$(sort "$TMP/union.log" | tr '\n' ' ')" "none-d ok-a ok-b ok-e "
eq "shards: no duplicates" "$(sort "$TMP/union.log" | uniq -d | wc -l | tr -d ' ')" "0"
fails "shards: index must be below count" "$S/validate-all-golden.sh" --shard 3/3
fails "shards: malformed" "$S/validate-all-golden.sh" --shard 1

# Nothing compared is not a pass.
rm -rf "$G/o"
run_all "$TMP/none.log"
eq "nothing compared: exit" "$RC" "3"
eq "nothing compared: summary" "$(tail -1 <<< "$OUT")" "Checked 1 recipes: 0 matched, 0 failed, 1 not compared"

rm -rf "$G"/*
run_all "$TMP/empty.log"
eq "no recipes: exit" "$RC" "3"
eq "no recipes: summary" "$(tail -1 <<< "$OUT")" "Checked 0 recipes: 0 matched, 0 failed, 0 not compared"

echo
echo "$PASS passed, $FAIL failed"
[[ $FAIL -eq 0 && $PASS -gt 0 ]]
