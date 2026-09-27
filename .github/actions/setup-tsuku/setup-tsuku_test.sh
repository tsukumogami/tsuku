#!/usr/bin/env bash
# Hermetic self-test for setup-tsuku.sh. `curl` is replaced on PATH by a stub that
# serves files from a fixture release, so every verification branch -- including a
# binary that doesn't match a genuine checksums.txt, which no real release can
# produce -- runs without the network.
#
# Each failure case asserts three things: a non-zero exit, no tsuku binary under
# TSUKU_HOME, and nothing written to GITHUB_PATH or GITHUB_ENV. The pass case asserts
# the opposite, so a script that installs nothing can't satisfy the suite.

set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/setup-tsuku.sh"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; esac
asset="tsuku-${os}-${arch}"

root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT

# The stub serves $FIXTURE/<basename of URL> to the path after -o.
mkdir -p "$root/stub"
cat > "$root/stub/curl" <<'STUB'
#!/usr/bin/env bash
out="" url=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
src="$FIXTURE/$(basename "$url")"
[ -f "$src" ] || { echo "stub curl: 404 $url" >&2; exit 22; }
cp "$src" "$out"
STUB
chmod +x "$root/stub/curl"

# make_fixture DIR [binary-body] [extra checksums lines]
make_fixture() {
  local dir="$1" body="${2:-genuine}" extra="${3:-}"
  mkdir -p "$dir"
  printf '#!/bin/sh\necho "tsuku version 0.0.0-fixture (%s)"\n' "$body" > "$dir/$asset"
  {
    # Near-miss names: a loose match on the asset name would pick these up too.
    echo "0000000000000000000000000000000000000000000000000000000000000000  tsuku-dltest-${os}-${arch}"
    echo "2222222222222222222222222222222222222222222222222222222222222222  ${asset}-musl"
    echo "$(sha256_of "$dir/$asset")  $asset"
    [ -n "$extra" ] && printf '%s\n' "$extra"
  } > "$dir/checksums.txt"
}

pass=0
fail=0

# run_case NAME EXPECT(ok|fail) FIXTURE VERSION CHECKSUMS_SHA [REASON]
# REASON, for a fail case, is a fixed string the error must contain, so a case can't
# pass by failing somewhere other than the check it exists to exercise.
run_case() {
  local name="$1" expect="$2" fixture="$3" version="$4" list_sha="$5" reason="${6:-}"
  local case_dir="$root/case-$name"
  mkdir -p "$case_dir"
  : > "$case_dir/path"
  : > "$case_dir/env"

  local rc=0
  FIXTURE="$fixture" PATH="$root/stub:$PATH" \
    TSUKU_HOME="$case_dir/home" RUNNER_TEMP="$case_dir" \
    GITHUB_PATH="$case_dir/path" GITHUB_ENV="$case_dir/env" \
    TSUKU_VERSION="$version" TSUKU_CHECKSUMS_SHA256="$list_sha" \
    "$script" > "$case_dir/out" 2>&1 || rc=$?

  local installed=no
  [ -e "$case_dir/home/bin/tsuku" ] && installed=yes
  local wrote=no
  if [ -s "$case_dir/path" ] || [ -s "$case_dir/env" ]; then wrote=yes; fi

  local ok=1
  if [ "$expect" = ok ]; then
    [ "$rc" -eq 0 ] || ok=0
    [ "$installed" = yes ] || ok=0
    grep -qx "$case_dir/home/bin" "$case_dir/path" || ok=0
    grep -qx "$case_dir/home/tools/current" "$case_dir/path" || ok=0
    grep -qx 'TSUKU_NO_TELEMETRY=1' "$case_dir/env" || ok=0
    grep -q '^Verified checksums.txt for tsuku v' "$case_dir/out" || ok=0
    grep -q "^Verified $asset against checksums.txt" "$case_dir/out" || ok=0
  else
    [ "$rc" -ne 0 ] || ok=0
    [ "$installed" = no ] || ok=0
    [ "$wrote" = no ] || ok=0
    grep -qF -- "$reason" "$case_dir/out" || ok=0
  fi

  if [ "$ok" -eq 1 ]; then
    echo "PASS  $name (exit $rc, installed=$installed)"
    pass=$((pass + 1))
  else
    echo "FAIL  $name (expected $expect; exit $rc, installed=$installed, wrote PATH/ENV=$wrote)"
    sed 's/^/      /' "$case_dir/out"
    fail=$((fail + 1))
  fi
}

good="$root/fx-good"
make_fixture "$good"
good_sha="$(sha256_of "$good/checksums.txt")"

run_case genuine ok "$good" 1.2.3 "$good_sha"
run_case v-prefixed-version ok "$good" v1.2.3 "$good_sha"
run_case wrong-checksums-sha fail "$good" 1.2.3 "$(printf '0%.0s' $(seq 64))" "checksums.txt for v1.2.3 has sha256"

# checksums.txt is genuine and matches the committed hash, but the binary served
# alongside it is not the one it describes.
swapped="$root/fx-swapped"
make_fixture "$swapped"
printf '#!/bin/sh\necho "tampered"\n' > "$swapped/$asset"
run_case binary-mismatch fail "$swapped" 1.2.3 "$(sha256_of "$swapped/checksums.txt")" "checksums.txt says"

missing="$root/fx-missing"
make_fixture "$missing"
grep -v " $asset\$" "$missing/checksums.txt" > "$missing/c" && mv "$missing/c" "$missing/checksums.txt"
run_case asset-not-listed fail "$missing" 1.2.3 "$(sha256_of "$missing/checksums.txt")" "expected exactly one $asset line"

dup="$root/fx-dup"
make_fixture "$dup" genuine "1111111111111111111111111111111111111111111111111111111111111111  $asset"
run_case asset-listed-twice fail "$dup" 1.2.3 "$(sha256_of "$dup/checksums.txt")" "expected exactly one $asset line"

run_case malformed-version fail "$good" latest "$good_sha" "version must look like"
run_case malformed-checksums-sha fail "$good" 1.2.3 "$(printf %s "$good_sha" | tr a-f A-F)" "checksums-sha256 must be"

declared=8
echo "$pass passed, $fail failed, $declared declared"
if [ "$fail" -ne 0 ] || [ "$((pass + fail))" -ne "$declared" ]; then
  exit 1
fi
