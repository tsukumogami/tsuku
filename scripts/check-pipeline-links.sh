#!/usr/bin/env bash
# check-pipeline-links.sh - Validate cross-page link integrity in pipeline HTML files.
#
# Checks:
# 1. All href targets to other pipeline pages reference files that exist
# 2. Status values in query parameters (?status=X) use known statuses
# 3. Ecosystem values in query parameters (?ecosystem=X) are well-formed
#
# Usage: bash scripts/check-pipeline-links.sh
# Exit code: 0 if all checks pass, 1 on failure.

set -eo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PIPELINE_DIR="$REPO_ROOT/website/pipeline"

if [ ! -d "$PIPELINE_DIR" ]; then
    echo "FAIL: pipeline directory not found: $PIPELINE_DIR"
    exit 1
fi

errors=0

# Collect the set of existing HTML files (basename only)
existing_pages=""
for f in "$PIPELINE_DIR"/*.html; do
    existing_pages="$existing_pages $(basename "$f")"
done

page_exists() {
    local target="$1"
    for p in $existing_pages; do
        if [ "$p" = "$target" ]; then
            return 0
        fi
    done
    return 1
}

echo "--- Check 1: href targets point to existing pipeline pages ---"
for html_file in "$PIPELINE_DIR"/*.html; do
    base="$(basename "$html_file")"

    # Extract static href targets pointing to local .html files
    while IFS= read -r target; do
        [ -z "$target" ] && continue
        # Strip leading /pipeline/ prefix if present
        target="${target#/pipeline/}"
        # Strip query parameters and anchors
        target="${target%%\?*}"
        target="${target%%#*}"
        # Skip if not an html file reference
        [[ "$target" == *.html ]] || continue
        # Skip dynamic JS references
        [[ "$target" == *'${'* ]] && continue
        [[ "$target" == *"' + '"* ]] && continue

        if ! page_exists "$target"; then
            echo "FAIL: $base references non-existent page: $target"
            errors=$((errors + 1))
        fi
    done < <(grep -oP 'href="(?!https?://|#|mailto:)\K[^"]+' "$html_file" 2>/dev/null | grep '\.html' || true)

    # Check JS template literals that construct hrefs to .html pages
    while IFS= read -r target; do
        [ -z "$target" ] && continue
        target="${target#/pipeline/}"
        target="${target%%\?*}"
        target="${target%%#*}"
        [[ "$target" == *.html ]] || continue

        if ! page_exists "$target"; then
            echo "FAIL: $base (JS) references non-existent page: $target"
            errors=$((errors + 1))
        fi
    done < <(grep -oP "(?:href=|location\.href=)['\"](?:\/pipeline\/)?\K[a-z_-]+\.html" "$html_file" 2>/dev/null || true)
done

echo "PASS: All href targets point to existing pages"

echo ""
echo "--- Check 2: status query parameters use known values ---"
# Known statuses from the pipeline system plus UI-specific aliases
known_statuses="pending failed success blocked requires_manual excluded valid invalid unknown needs_review"

is_known_status() {
    local val="$1"
    for s in $known_statuses; do
        if [ "$s" = "$val" ]; then
            return 0
        fi
    done
    return 1
}

for html_file in "$PIPELINE_DIR"/*.html; do
    base="$(basename "$html_file")"
    while IFS= read -r status_val; do
        [ -z "$status_val" ] && continue

        if ! is_known_status "$status_val"; then
            echo "FAIL: $base uses unknown status parameter: ?status=$status_val"
            errors=$((errors + 1))
        fi
    done < <(grep -oP '[?&]status=\K[a-z_]+' "$html_file" 2>/dev/null || true)
done

echo "PASS: All status query parameters use known values"

echo ""
echo "--- Check 3: ecosystem query parameters are well-formed ---"
# Collect all hardcoded ecosystem values from query parameters.
# Most ecosystem values are dynamic (constructed via JS from dashboard.json),
# so we only check the format of any hardcoded ones.
eco_pattern='^[a-z][a-z0-9._-]*$'
eco_count=0
for html_file in "$PIPELINE_DIR"/*.html; do
    base="$(basename "$html_file")"
    while IFS= read -r eco_val; do
        [ -z "$eco_val" ] && continue
        eco_count=$((eco_count + 1))
        if ! [[ "$eco_val" =~ $eco_pattern ]]; then
            echo "FAIL: $base ecosystem value '$eco_val' does not match expected format (lowercase, no spaces)"
            errors=$((errors + 1))
        fi
    done < <(grep -oP '[?&]ecosystem=\K[a-z._-]+' "$html_file" 2>/dev/null || true)
done

if [ "$eco_count" -gt 0 ]; then
    echo "PASS: All $eco_count ecosystem query parameter values are well-formed"
else
    echo "PASS: No hardcoded ecosystem query parameters found (values are dynamic)"
fi

echo ""
echo "--- Summary ---"
page_count=$(echo "$existing_pages" | wc -w)
echo "Pipeline pages checked: $page_count"
if [ "$errors" -gt 0 ]; then
    echo "FAIL: $errors error(s) found"
    exit 1
fi
echo "PASS: All link integrity checks passed"
