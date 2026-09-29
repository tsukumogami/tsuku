#!/usr/bin/env bash
# Move a reviewed list of golden-plan keys out of the R2 layout, into quarantine.
#
# Usage:
#   ./scripts/r2-quarantine-keys.sh --list <file> --expect <n> [--execute] [--sample <n>]
#
# The list is one object key per line (plans/<letter>/<recipe>/v<version>/<platform>.json),
# committed and reviewed before anything runs. Every key must parse as a registry key in the
# current layout (scripts/lib/r2-layout.sh), and the list must hold exactly --expect keys;
# otherwise nothing is touched. Embedded goldens and anything outside plans/ are refused.
#
# Without --execute (the default) this is read-only. It lists the bucket once and reports how
# many of the keys are present, names any that are absent, and prints a sample of the present
# ones.
#
# With --execute, each present key is copied to quarantine/<date>/<key>. The copy's ETag is
# checked against the source's, and only then is the original deleted. A key whose copy doesn't
# check out is left in place and counted as failed. Quarantined objects are removed for good
# only by R2 Cleanup's hard delete (scripts/r2-cleanup.sh --hard-delete, 7 days minimum), so a
# mistake can be copied back until then.
#
# Environment: R2_BUCKET_URL, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, optional R2_BUCKET_NAME.
# Exit codes: 0 success, 1 a key failed to move, 2 usage error or the list was refused.

set -euo pipefail

# shellcheck source=lib/r2-layout.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/r2-layout.sh"

BUCKET="${R2_BUCKET_NAME:-tsuku-golden-registry}"
LIST="" EXPECT="" EXECUTE=false SAMPLE=10
while [[ $# -gt 0 ]]; do
    case "$1" in
        --list) LIST="$2"; shift 2 ;;
        --expect) EXPECT="$2"; shift 2 ;;
        --execute) EXECUTE=true; shift ;;
        --sample) SAMPLE="$2"; shift 2 ;;
        -h|--help) sed -n '2,24p' "$0"; exit 0 ;;
        *) echo "Unknown argument: $1" >&2; exit 2 ;;
    esac
done
if [[ -z "$LIST" || ! -f "$LIST" || ! "$EXPECT" =~ ^[0-9]+$ ]]; then
    echo "Usage: $0 --list <file> --expect <n> [--execute] [--sample <n>]" >&2
    exit 2
fi

# --- validate the list before touching anything ---------------------------------------------
mapfile -t KEYS < <(grep -v '^[[:space:]]*$' "$LIST")
if [[ ${#KEYS[@]} -ne $EXPECT ]]; then
    echo "Refused: $LIST holds ${#KEYS[@]} keys, expected $EXPECT" >&2
    exit 2
fi
bad=0
for key in "${KEYS[@]}"; do
    if ! r2_parse_plan_key "$key" || [[ "$R2_KEY_CATEGORY" != "registry" ]]; then
        echo "Refused key (not a registry key in the current layout): $key" >&2
        bad=$((bad + 1))
    fi
done
if [[ $(printf '%s\n' "${KEYS[@]}" | sort | uniq -d | wc -l) -gt 0 ]]; then
    echo "Refused: $LIST contains duplicate keys" >&2
    bad=$((bad + 1))
fi
[[ $bad -eq 0 ]] || exit 2

for var in R2_BUCKET_URL R2_ACCESS_KEY_ID R2_SECRET_ACCESS_KEY; do
    [[ -n "${!var:-}" ]] || { echo "Error: $var is required" >&2; exit 2; }
done
export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" AWS_ENDPOINT_URL="$R2_BUCKET_URL"

# --- one listing: what is actually there -----------------------------------------------------
listing=$(mktemp); trap 'rm -f "$listing"' EXIT
aws s3api list-objects-v2 --bucket "$BUCKET" --prefix "$R2_PLANS_ROOT/" --output json \
    | jq -r '.Contents // [] | .[] | [.Key, (.ETag | gsub("\""; ""))] | @tsv' > "$listing"
declare -A ETAG=()
while IFS=$'\t' read -r k e; do [[ -n "$k" ]] && ETAG["$k"]="$e"; done < "$listing"

present=() absent=()
for key in "${KEYS[@]}"; do
    if [[ -n "${ETAG[$key]+set}" ]]; then present+=("$key"); else absent+=("$key"); fi
done
echo "List $LIST: ${#KEYS[@]} keys, ${#present[@]} present, ${#absent[@]} absent (bucket objects listed under $R2_PLANS_ROOT/: $(grep -c . "$listing"))"
for key in "${absent[@]+"${absent[@]}"}"; do echo "  absent: $key"; done
echo "Sample of present keys:"
# Sliced rather than piped through head: with pipefail, head closing the pipe early would kill
# the script with a write error whenever there are more present keys than the sample size.
for key in "${present[@]:0:$SAMPLE}"; do echo "  $key"; done

if [[ "$EXECUTE" != true ]]; then
    echo "Dry run: nothing moved."
    exit 0
fi

# --- quarantine: copy, check, then delete the original --------------------------------------
day=$(date -u +%Y-%m-%d)
moved=0 failed=0
for key in "${present[@]+"${present[@]}"}"; do
    q="quarantine/$day/$key"
    if ! aws s3api copy-object --bucket "$BUCKET" --copy-source "$BUCKET/$key" --key "$q" > /dev/null; then
        echo "FAILED copy: $key" >&2; failed=$((failed + 1)); continue
    fi
    qe=$(aws s3api head-object --bucket "$BUCKET" --key "$q" --query ETag --output text | tr -d '"')
    if [[ "$qe" != "${ETAG[$key]}" ]]; then
        echo "FAILED check: quarantine copy of $key has ETag $qe, source ${ETAG[$key]}; original kept" >&2
        failed=$((failed + 1)); continue
    fi
    if ! aws s3api delete-object --bucket "$BUCKET" --key "$key" > /dev/null; then
        echo "FAILED delete: $key (quarantine copy exists)" >&2; failed=$((failed + 1)); continue
    fi
    moved=$((moved + 1))
done
echo "Quarantined $moved of ${#present[@]} present keys to quarantine/$day/, $failed failed"
[[ $failed -eq 0 ]]
