#!/usr/bin/env bash
set -euo pipefail

# Turns a listing of R2 golden-file keys into the declared set for each OS leg of
# Nightly Registry Validation (#2608).
#
# Usage: golden-declared-set.sh --out-dir <dir> [--exclude-prefix <prefix>]... < keys
#
# Reads one object key per line, of the form <anything>/<recipe>/v<version>/<platform>.json,
# and writes <out-dir>/declared-<os>.txt: one item per line, sorted and unique, where an
# item is `<recipe>/v<version>-<platform>`. That is the name the download step gives the
# file once it flattens the version directory, and the name validate-golden.sh writes into
# its receipt, so the two sides compare by content.
#
# The item is taken from the LAST THREE segments of the key only. Whatever sits between
# the bucket prefix and the recipe -- `registry` today, a single letter if #2448 moves the
# publisher -- does not change it, so the declared set follows that layout decision
# without being edited.
#
# The listing is the declared set because it is taken separately from the sync that
# fetches the files. A download that comes back partial shrinks what the validator sees,
# not what was declared, and the shortfall shows up in the assertion.

OUT_DIR=""
EXCLUDES=()
while [ $# -gt 0 ]; do
  case "$1" in
    --out-dir)        OUT_DIR="$2"; shift 2 ;;
    --exclude-prefix) EXCLUDES+=("$2"); shift 2 ;;
    *) echo "::error::unknown argument: $1" >&2; exit 2 ;;
  esac
done
if [ -z "$OUT_DIR" ]; then
  echo "::error::--out-dir is required" >&2
  exit 2
fi
mkdir -p "$OUT_DIR"

listed=0 excluded=0 skipped=0
declare -A per_os=()
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

while IFS= read -r key || [ -n "$key" ]; do
  [ -z "$key" ] && continue
  listed=$((listed + 1))
  for prefix in ${EXCLUDES[@]+"${EXCLUDES[@]}"}; do
    if [[ "$key" == "$prefix"* ]]; then
      excluded=$((excluded + 1))
      continue 2
    fi
  done
  # <...>/<recipe>/v<version>/<platform>.json, and nothing else
  if [[ ! "$key" =~ /([^/]+)/(v[^/]+)/([^/]+)\.json$ ]]; then
    echo "::warning::not a golden-file key, not declared: $key" >&2
    skipped=$((skipped + 1))
    continue
  fi
  recipe="${BASH_REMATCH[1]}" version="${BASH_REMATCH[2]}" platform="${BASH_REMATCH[3]}"
  os="${platform%%-*}"
  echo "$recipe/$version-$platform" >> "$tmp/$os"
  per_os[$os]=1
done

# Every OS the validator runs a leg for gets a file, even an empty one, so an empty
# listing produces "declared 0" rather than a missing declaration.
for os in linux darwin; do
  per_os[$os]=1
done
summary="Declared set: $listed key(s) listed, $excluded excluded by prefix, $skipped not golden files"
for os in $(printf '%s\n' "${!per_os[@]}" | sort); do
  if [ -f "$tmp/$os" ]; then
    sort -u "$tmp/$os" > "$OUT_DIR/declared-$os.txt"
  else
    : > "$OUT_DIR/declared-$os.txt"
  fi
  summary="$summary; $os $(wc -l < "$OUT_DIR/declared-$os.txt" | tr -d ' ')"
done
echo "$summary."
