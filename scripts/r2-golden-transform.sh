#!/usr/bin/env bash
# Turn a local mirror of the bucket's plans/ prefix into the golden-file tree that
# validate-golden.sh and validate-all-golden.sh read.
#
# Usage: ./scripts/r2-golden-transform.sh <mirror-dir> <out-dir>
#
#   <mirror-dir>  Directory holding the synced contents of s3://<bucket>/plans/, so a file
#                 at <mirror-dir>/f/fzf/v0.60.0/linux-amd64.json came from the key
#                 plans/f/fzf/v0.60.0/linux-amd64.json.
#   <out-dir>     Receives <out-dir>/<letter>/<recipe>/v<version>-<platform>.json.
#
# Only registry plans are transformed; embedded goldens are validated from the repo.
#
# Keys are parsed by scripts/lib/r2-layout.sh, never by position. Keys under the legacy
# plans/registry/ prefix are read only when the current layout has no object for the
# same recipe, version and platform, and every one read that way is counted, so the
# fallback is visible for as long as it is doing anything. It stops mattering once the
# legacy objects have been copied to the current layout (r2-migrate-registry-layout.yml)
# and removed.
#
# The last line of output is a summary in this fixed form:
#   Transformed <N> registry plans (<C> current layout, <L> legacy fallback); ignored <I> other objects
#
# Exit codes:
#   0  Transformed at least one plan
#   1  Transformed nothing (an empty mirror is not a tree anyone can validate)
#   2  Usage error

set -euo pipefail

# shellcheck source=lib/r2-layout.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/r2-layout.sh"

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <mirror-dir> <out-dir>" >&2
    exit 2
fi

MIRROR="${1%/}"
OUT="${2%/}"

if [[ ! -d "$MIRROR" ]]; then
    echo "Error: mirror directory not found: $MIRROR" >&2
    exit 2
fi

mkdir -p "$OUT"

CURRENT=0
LEGACY=0
IGNORED=0
LEGACY_CANDIDATES=()

place() {  # place <file> <recipe> <version> <platform>
    local target_dir
    target_dir="$OUT/${2:0:1}/$2"
    mkdir -p "$target_dir"
    cp "$1" "$target_dir/v${3}-${4}.json"
}

while IFS= read -r -d '' file; do
    key="$R2_PLANS_ROOT/${file#"$MIRROR"/}"
    if r2_parse_plan_key "$key"; then
        if [[ "$R2_KEY_CATEGORY" == "registry" ]]; then
            place "$file" "$R2_KEY_RECIPE" "$R2_KEY_VERSION" "$R2_KEY_PLATFORM"
            CURRENT=$((CURRENT + 1))
        else
            IGNORED=$((IGNORED + 1))
        fi
    elif r2_legacy_registry_key_to_current "$key" > /dev/null; then
        # Deferred until every current-layout object is in place, so a current object
        # always wins over its legacy copy regardless of traversal order.
        LEGACY_CANDIDATES+=("$file")
    else
        IGNORED=$((IGNORED + 1))
    fi
done < <(find "$MIRROR" -type f -name '*.json' -print0)

for file in "${LEGACY_CANDIDATES[@]+"${LEGACY_CANDIDATES[@]}"}"; do
    key="$R2_PLANS_ROOT/${file#"$MIRROR"/}"
    current_key=$(r2_legacy_registry_key_to_current "$key")
    r2_parse_plan_key "$current_key"
    target="$OUT/${R2_KEY_RECIPE:0:1}/$R2_KEY_RECIPE/v${R2_KEY_VERSION}-${R2_KEY_PLATFORM}.json"
    if [[ -e "$target" ]]; then
        continue
    fi
    place "$file" "$R2_KEY_RECIPE" "$R2_KEY_VERSION" "$R2_KEY_PLATFORM"
    LEGACY=$((LEGACY + 1))
done

TOTAL=$((CURRENT + LEGACY))
if [[ $LEGACY -gt 0 ]]; then
    echo "::warning::$LEGACY registry plans were read from the legacy ${R2_LEGACY_REGISTRY_PREFIX} prefix because the current layout has no copy of them"
fi
echo "Transformed $TOTAL registry plans ($CURRENT current layout, $LEGACY legacy fallback); ignored $IGNORED other objects"

if [[ $TOTAL -eq 0 ]]; then
    exit 1
fi
