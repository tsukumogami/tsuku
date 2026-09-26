#!/usr/bin/env bash
# Copy registry golden plans from the legacy plans/registry/ prefix to the current layout.
#
# Usage:
#   ./scripts/r2-migrate-registry-layout.sh --plan <out-dir>
#   ./scripts/r2-migrate-registry-layout.sh --execute <plan-dir>
#   ./scripts/r2-migrate-registry-layout.sh --verify <plan-dir> [--sample <n>]
#
# Until #2448, publishing wrote registry plans to plans/registry/<recipe>/v<ver>/<p>.json
# while every reader looked under plans/<first-letter>/<recipe>/... This copies each legacy
# object to its key in the current layout (scripts/lib/r2-layout.sh) with a server-side
# copy. It never deletes and never overwrites: removing the legacy prefix is a separate,
# deliberate step.
#
# --plan <out-dir>      Read-only. Lists both prefixes and writes:
#                         copy.tsv          source-key  destination-key  source-etag
#                         present.tsv       keys already copied (destination has the same ETag)
#                         collisions.tsv    destination exists with a DIFFERENT ETag (skipped)
#                         unmapped.txt      legacy keys that do not map to the layout (skipped)
#                         summary.txt       the counts, one per line
# --execute <plan-dir>  Copies every row of copy.tsv. Refuses to overwrite a destination
#                       that appeared since the plan was made. Checks each copy's ETag
#                       against its source.
# --verify <plan-dir>   Read-only. Downloads a sample of copied pairs and compares SHA-256,
#                       and counts every object under the destination prefixes.
#
# Environment: R2_BUCKET_URL, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, optional R2_BUCKET_NAME.
#
# Exit codes: 0 success, 1 a copy or check failed, 2 usage or environment error.

set -euo pipefail

# shellcheck source=lib/r2-layout.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/r2-layout.sh"

BUCKET="${R2_BUCKET_NAME:-tsuku-golden-registry}"
MODE=""
DIR=""
SAMPLE=25

while [[ $# -gt 0 ]]; do
    case "$1" in
        --plan|--execute|--verify) MODE="${1#--}"; DIR="${2:-}"; shift 2 ;;
        --sample) SAMPLE="$2"; shift 2 ;;
        -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
        *) echo "Unknown argument: $1" >&2; exit 2 ;;
    esac
done

if [[ -z "$MODE" || -z "$DIR" ]]; then
    echo "Usage: $0 --plan <out-dir> | --execute <plan-dir> | --verify <plan-dir> [--sample <n>]" >&2
    exit 2
fi

for var in R2_BUCKET_URL R2_ACCESS_KEY_ID R2_SECRET_ACCESS_KEY; do
    if [[ -z "${!var:-}" ]]; then
        echo "Error: $var is required" >&2
        exit 2
    fi
done
export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
export AWS_ENDPOINT_URL="$R2_BUCKET_URL"

# list_keys <prefix>: prints "<key>\t<etag>" for every object under the prefix. Fails,
# rather than printing nothing, when the listing itself fails.
list_keys() {
    aws s3api list-objects-v2 --bucket "$BUCKET" --prefix "$1" --output json \
        | jq -r '.Contents // [] | .[] | [.Key, (.ETag | gsub("\""; ""))] | @tsv'
}

etag_of() {
    aws s3api head-object --bucket "$BUCKET" --key "$1" --query ETag --output text | tr -d '"'
}

plan() {
    mkdir -p "$DIR"
    : > "$DIR/copy.tsv"; : > "$DIR/present.tsv"; : > "$DIR/collisions.tsv"; : > "$DIR/unmapped.txt"

    local legacy current
    legacy=$(mktemp); current=$(mktemp)
    list_keys "$R2_LEGACY_REGISTRY_PREFIX" > "$legacy"
    list_keys "$R2_PLANS_ROOT/" > "$current"

    declare -A existing=()
    local key etag dest
    while IFS=$'\t' read -r key etag; do
        [[ -n "$key" ]] && existing["$key"]="$etag"
    done < "$current"

    while IFS=$'\t' read -r key etag; do
        [[ -n "$key" ]] || continue
        if ! dest=$(r2_legacy_registry_key_to_current "$key"); then
            echo "$key" >> "$DIR/unmapped.txt"
        elif [[ -z "${existing[$dest]+set}" ]]; then
            printf '%s\t%s\t%s\n' "$key" "$dest" "$etag" >> "$DIR/copy.tsv"
        elif [[ "${existing[$dest]}" == "$etag" ]]; then
            printf '%s\t%s\n' "$key" "$dest" >> "$DIR/present.tsv"
        else
            printf '%s\t%s\t%s\t%s\n' "$key" "$dest" "$etag" "${existing[$dest]}" >> "$DIR/collisions.tsv"
        fi
    done < "$legacy"

    {
        echo "source_prefix=$R2_LEGACY_REGISTRY_PREFIX"
        echo "destination=$R2_PLANS_ROOT/<first-letter>/<recipe>/ (scripts/lib/r2-layout.sh)"
        echo "legacy_objects=$(grep -c . "$legacy" || true)"
        echo "to_copy=$(grep -c . "$DIR/copy.tsv" || true)"
        echo "already_present=$(grep -c . "$DIR/present.tsv" || true)"
        echo "collisions=$(grep -c . "$DIR/collisions.tsv" || true)"
        echo "unmapped=$(grep -c . "$DIR/unmapped.txt" || true)"
        echo "recipes=$(cut -f2 "$DIR/copy.tsv" | cut -d/ -f3 | sort -u | grep -c . || true)"
    } > "$DIR/summary.txt"
    rm -f "$legacy" "$current"
    cat "$DIR/summary.txt"
}

execute() {
    [[ -f "$DIR/copy.tsv" ]] || { echo "Error: no plan at $DIR/copy.tsv" >&2; exit 2; }
    local total copied=0 failed=0 src dest etag
    total=$(grep -c . "$DIR/copy.tsv" || true)
    while IFS=$'\t' read -r src dest etag; do
        [[ -n "$src" ]] || continue
        # Refuse to overwrite anything that appeared after the plan was made.
        if aws s3api head-object --bucket "$BUCKET" --key "$dest" > /dev/null 2>&1; then
            echo "SKIP (destination now exists): $dest" >&2
            failed=$((failed + 1))
            continue
        fi
        if ! aws s3api copy-object --bucket "$BUCKET" --copy-source "$BUCKET/$src" --key "$dest" \
                --metadata-directive COPY > /dev/null; then
            echo "FAILED copy: $src -> $dest" >&2
            failed=$((failed + 1))
            continue
        fi
        if [[ "$(etag_of "$dest")" != "$etag" ]]; then
            echo "FAILED check: $dest ETag differs from $src" >&2
            failed=$((failed + 1))
            continue
        fi
        copied=$((copied + 1))
    done < "$DIR/copy.tsv"
    echo "Copied $copied of $total planned objects, $failed failed"
    [[ $failed -eq 0 && $copied -eq $total ]]
}

verify() {
    [[ -f "$DIR/copy.tsv" ]] || { echo "Error: no plan at $DIR/copy.tsv" >&2; exit 2; }
    local tmp checked=0 bad=0 src dest etag a b
    tmp=$(mktemp -d)
    while IFS=$'\t' read -r src dest etag; do
        [[ -n "$src" ]] || continue
        aws s3 cp "s3://$BUCKET/$src" "$tmp/a" --quiet
        aws s3 cp "s3://$BUCKET/$dest" "$tmp/b" --quiet
        a=$(sha256sum "$tmp/a" | cut -d' ' -f1)
        b=$(sha256sum "$tmp/b" | cut -d' ' -f1)
        if [[ "$a" == "$b" ]]; then
            echo "MATCH $a $dest"
        else
            echo "DIFFER $dest (source $a, destination $b)"
            bad=$((bad + 1))
        fi
        checked=$((checked + 1))
    done < <(shuf -n "$SAMPLE" "$DIR/copy.tsv")
    rm -rf "$tmp"

    # Count what the readers will now see: every registry key in the current layout. The
    # listing goes to a file first so a failed listing stops the script instead of
    # reading as zero objects.
    local listing registry_keys=0 key
    listing=$(mktemp)
    list_keys "$R2_PLANS_ROOT/" > "$listing"
    while IFS=$'\t' read -r key _; do
        if r2_parse_plan_key "$key" && [[ "$R2_KEY_CATEGORY" == "registry" ]]; then
            registry_keys=$((registry_keys + 1))
        fi
    done < "$listing"
    rm -f "$listing"
    echo "Sampled $checked copies: $((checked - bad)) identical, $bad differ"
    echo "Registry objects in the current layout: $registry_keys"
    [[ $bad -eq 0 && $checked -gt 0 ]]
}

"$MODE"
