#!/usr/bin/env bash
# Self-test for scripts/r2-quarantine-keys.sh. Hermetic: `aws` is a stub on PATH that serves a
# fixed bucket listing and records every call, so each case asserts what the script DID to the
# bucket, not only its exit code.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$HERE/r2-quarantine-keys.sh"
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
pass=0 fail=0
ok()  { pass=$((pass + 1)); echo "ok   $1"; }
bad() { fail=$((fail + 1)); echo "FAIL $1"; }

mkdir -p "$T/bin"
cat > "$T/bin/aws" <<'STUB'
#!/usr/bin/env bash
echo "$*" >> "$STUB_CALLS"
case "$2" in
  list-objects-v2) printf '{"Contents":[{"Key":"plans/f/fzf/v1.0/linux-amd64.json","ETag":"\\"e1\\""},{"Key":"plans/f/fzf/v0.9/linux-amd64.json","ETag":"\\"e2\\""}]}\n' ;;
  copy-object) echo '{}' ;;
  head-object) if [ -n "${STUB_BAD_ETAG:-}" ]; then echo '"zzz"'; else case "$*" in *v1.0*) echo '"e1"' ;; *) echo '"e2"' ;; esac; fi ;;
  delete-object) echo '{}' ;;
esac
STUB
chmod +x "$T/bin/aws"
export PATH="$T/bin:$PATH" R2_BUCKET_URL=x R2_ACCESS_KEY_ID=x R2_SECRET_ACCESS_KEY=x STUB_CALLS="$T/calls"

run() {  # run <list-content> <expect> [args...]: sets RC, OUT; resets the call log
    printf '%s' "$1" > "$T/list"; : > "$T/calls"
    OUT=$("$SCRIPT" --list "$T/list" --expect "$2" "${@:3}" 2>&1); RC=$?
}
two=$'plans/f/fzf/v1.0/linux-amd64.json\nplans/f/fzf/v0.8/linux-amd64.json\n'

run "$two" 3
[ "$RC" = 2 ] && [ ! -s "$T/calls" ] && ok "wrong count is refused before any call to R2" || bad "count mismatch: rc=$RC calls=$(cat "$T/calls")"

run $'plans/registry/fzf/v1.0/linux-amd64.json\n' 1
[ "$RC" = 2 ] && [ ! -s "$T/calls" ] && ok "a legacy-prefix key is refused" || bad "legacy key: rc=$RC"
run $'plans/embedded/go/v1/linux-amd64.json\n' 1
[ "$RC" = 2 ] && [ ! -s "$T/calls" ] && ok "an embedded key is refused" || bad "embedded key: rc=$RC"
run $'plans/f/fzf/v1.0/linux-amd64.json\nplans/f/fzf/v1.0/linux-amd64.json\n' 2
[ "$RC" = 2 ] && ok "duplicate keys are refused" || bad "duplicates: rc=$RC"

run "$two" 2
[ "$RC" = 0 ] && grep -q "2 keys, 1 present, 1 absent" <<< "$OUT" && grep -q "absent: plans/f/fzf/v0.8" <<< "$OUT" \
  && ! grep -q "copy-object\|delete-object" "$T/calls" && ok "dry run reports present/absent and changes nothing" \
  || bad "dry run: rc=$RC out=$OUT calls=$(cat "$T/calls")"

run "$two" 2 --execute
[ "$RC" = 0 ] && grep -q "Quarantined 1 of 1 present keys" <<< "$OUT" \
  && [ "$(grep -n 'copy-object' "$T/calls" | cut -d: -f1)" -lt "$(grep -n 'delete-object' "$T/calls" | cut -d: -f1)" ] \
  && grep -q "delete-object .*--key plans/f/fzf/v1.0/linux-amd64.json" "$T/calls" \
  && grep -q "copy-object .*--key quarantine/[0-9-]*/plans/f/fzf/v1.0/linux-amd64.json" "$T/calls" \
  && ! grep -q "v0.8" "$T/calls" && ok "execute copies to quarantine, then deletes the original, and skips absent keys" \
  || bad "execute: rc=$RC out=$OUT calls=$(cat "$T/calls")"

STUB_BAD_ETAG=1 run "$two" 2 --execute
[ "$RC" = 1 ] && ! grep -q "delete-object" "$T/calls" && grep -q "original kept" <<< "$OUT" \
  && ok "a quarantine copy whose ETag differs keeps the original" || bad "bad etag: rc=$RC calls=$(cat "$T/calls")"

echo; echo "r2-quarantine-keys self-test: $pass passed, $fail failed"
[ "$fail" = 0 ] && [ "$pass" -gt 0 ]
