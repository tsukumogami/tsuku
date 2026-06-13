#!/usr/bin/env bash
#
# capture-absorption-baseline.sh - Capture an external check's real fired-rule
# set for shirabe's absorption parity harness.
#
# The shirabe parity harness (crates/shirabe/tests/absorption_parity.rs) proves
# that an absorbed engine check fires the same rules as the external bash check
# it replaces, over a frozen corpus. The external side of that comparison is a
# committed baseline at `expected/<case>/external_rules`. This script PRODUCES
# that baseline from the real bash script, replacing the hand-authored stand-ins
# the harness shipped with.
#
# It runs ONE check script over ONE document and writes, to stdout, the SORTED
# UNIQUE set of rule codes the check fired (one per line) -- the exact payload
# the harness reads. A check that fires nothing emits an empty set (the clean
# baseline). This is the capture-ahead model: a developer runs this once in a
# controlled environment and commits the output; `cargo test` and CI never run
# the bash.
#
# Usage (single capture):
#   capture-absorption-baseline.sh <check-script> <doc-path>
#
# Usage (regenerate a whole corpus from a capture manifest):
#   capture-absorption-baseline.sh --manifest <manifest.tsv> <golden-dir>
#
#   The manifest is tab-separated `case_id <TAB> check-script <TAB> doc_relpath`
#   (comments with '#' and blank lines ignored). For each row the script runs
#   <check-script> over <golden-dir>/corpus/<case_id>/<doc_relpath> and writes
#   the captured set to <golden-dir>/expected/<case_id>/external_rules, with a
#   provenance header naming the check and the capture command.
#
# The normalization contract: each `[FAIL] ...` line an external check writes to
# stderr leads with its rule code (FM01-03 from frontmatter.sh, SC01-03 from
# sections.sh, SD01-02 from status-directory.sh). This script extracts those
# leading codes verbatim -- they ARE the mapping.tsv keys the harness translates
# into engine codes. The check's own exit 1 (findings present) is expected and
# not an error; only an operational failure (exit 2) is surfaced.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Resolve a manifest's check reference: an absolute/relative path is used as-is;
# a bare name resolves under this script's checks/ directory.
resolve_check() {
    local ref="$1"
    if [[ "$ref" == */* ]]; then
        printf '%s\n' "$ref"
    else
        printf '%s\n' "$SCRIPT_DIR/checks/$ref"
    fi
}

# Extract the set of rule codes from a check's stderr. Each fired rule is the
# leading two-letter/two-digit token of a `[FAIL] ...` line; the same code also
# appears in that line's `See: .github/scripts/docs/<CODE>.md` reference, so
# scanning [FAIL] lines and de-duplicating yields exactly the fired set.
extract_codes() {
    grep -E '^\[FAIL\]' | grep -oE '[A-Z]{2}[0-9]{2}' | sort -u
}

# Run one check over one doc; echo the captured code set (possibly empty).
# Returns 2 (and prints to stderr) on an operational error from the check.
capture_one() {
    local check="$1" doc="$2"
    if [[ ! -x "$check" ]]; then
        echo "error: check script not executable: $check" >&2
        return 2
    fi
    if [[ ! -f "$doc" ]]; then
        echo "error: document not found: $doc" >&2
        return 2
    fi
    local stderr rc
    # Capture stderr, discard stdout (the [PASS] chatter); the check's exit 1
    # on findings is expected, so do not let it abort the script.
    stderr="$("$check" "$doc" 2>&1 1>/dev/null)"
    rc=$?
    if [[ $rc -eq 2 ]]; then
        echo "error: operational failure (exit 2) from $check over $doc:" >&2
        printf '%s\n' "$stderr" >&2
        return 2
    fi
    # An empty fired-set (clean document) is a valid baseline, not an error, so
    # the grep pipeline's empty-match exit 1 must not propagate.
    local codes
    codes="$(printf '%s\n' "$stderr" | extract_codes)"
    [[ -n "$codes" ]] && printf '%s\n' "$codes"
    return 0
}

main() {
    if [[ "${1:-}" == "--manifest" ]]; then
        local manifest="${2:-}" golden="${3:-}"
        if [[ -z "$manifest" || -z "$golden" ]]; then
            echo "usage: $0 --manifest <manifest.tsv> <golden-dir>" >&2
            exit 2
        fi
        [[ -f "$manifest" ]] || { echo "no such manifest: $manifest" >&2; exit 2; }
        [[ -d "$golden" ]] || { echo "no such golden dir: $golden" >&2; exit 2; }
        local case_id check_ref check doc_relpath doc out codes
        while IFS=$'\t' read -r case_id check_ref doc_relpath; do
            [[ -z "$case_id" || "$case_id" == \#* ]] && continue
            check="$(resolve_check "$check_ref")"
            doc="$golden/corpus/$case_id/$doc_relpath"
            out="$golden/expected/$case_id/external_rules"
            codes="$(capture_one "$check" "$doc")" || exit 2
            mkdir -p "$(dirname "$out")"
            {
                echo "# external fired-rule set for $case_id (captured ahead of time)."
                echo "# source: $(basename "$check") over corpus/$case_id/$doc_relpath"
                echo "# regenerate: capture-absorption-baseline.sh --manifest <manifest> <golden-dir>"
                [[ -n "$codes" ]] && printf '%s\n' "$codes"
            } > "$out"
            echo "captured $case_id -> ${codes:-<empty>}" >&2
        done < "$manifest"
        return 0
    fi

    local check="${1:-}" doc="${2:-}"
    if [[ -z "$check" || -z "$doc" ]]; then
        echo "usage: $0 <check-script> <doc-path>" >&2
        echo "       $0 --manifest <manifest.tsv> <golden-dir>" >&2
        exit 2
    fi
    capture_one "$check" "$doc" || exit 2
}

main "$@"
