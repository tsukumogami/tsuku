#!/usr/bin/env bash
# Checksum drift monitoring script
# Checks recipes with checksum_url fields for upstream changes.
# Exit codes: 0 = no drift, 1 = drift detected, 2 = script error

set -euo pipefail

RECIPE_DIR="${1:-internal/recipe/recipes}"
DRIFT_DETECTED=0
RESULTS=()

log() { echo "[checksum-drift] $*"; }
warn() { echo "[checksum-drift] WARNING: $*" >&2; }

# Resolve latest version for a recipe based on its version source
resolve_version() {
    local recipe_file="$1"

    # Check for github_repo version source
    local github_repo
    github_repo=$(grep -oP 'github_repo\s*=\s*"\K[^"]+' "$recipe_file" 2>/dev/null || true)
    if [[ -n "$github_repo" ]]; then
        local tag
        tag=$(curl -sf "https://api.github.com/repos/${github_repo}/releases/latest" \
            -H "Accept: application/vnd.github+json" \
            ${GITHUB_TOKEN:+-H "Authorization: Bearer $GITHUB_TOKEN"} \
            | jq -r '.tag_name // empty' 2>/dev/null || true)
        if [[ -z "$tag" ]]; then
            return 1
        fi
        # Strip leading 'v' if present for {version}, keep original for {version_tag}
        echo "${tag#v}"
        return 0
    fi

    # Check for nodejs_dist version source
    local source
    source=$(grep -oP 'source\s*=\s*"\K[^"]+' "$recipe_file" 2>/dev/null || true)
    if [[ "$source" == "nodejs_dist" ]]; then
        local version
        version=$(curl -sf "https://nodejs.org/dist/index.json" \
            | jq -r '[.[] | select(.lts != false)] | first | .version // empty' 2>/dev/null || true)
        if [[ -z "$version" ]]; then
            return 1
        fi
        # Node versions come as "v22.0.0", strip 'v' for {version}
        echo "${version#v}"
        return 0
    fi

    return 1
}

# Resolve template variables in a URL
resolve_url() {
    local url="$1"
    local version="$2"
    local version_tag="v${version}"

    url="${url//\{version\}/$version}"
    url="${url//\{version_tag\}/$version_tag}"
    # Use linux/amd64 as the monitoring platform
    url="${url//\{os\}/linux}"
    url="${url//\{arch\}/amd64}"

    # Apply os_mapping and arch_mapping if present in the recipe
    # For now, handle known patterns
    echo "$url"
}

# Apply platform mappings from recipe to URL
apply_mappings() {
    local url="$1"
    local recipe_file="$2"

    # Extract os_mapping for linux
    local os_mapped
    os_mapped=$(grep -oP 'os_mapping\s*=\s*\{[^}]*linux\s*=\s*"\K[^"]+' "$recipe_file" 2>/dev/null || true)
    if [[ -n "$os_mapped" ]]; then
        url="${url//\{os\}/$os_mapped}"
    fi

    # Extract arch_mapping for amd64
    local arch_mapped
    arch_mapped=$(grep -oP 'arch_mapping\s*=\s*\{[^}]*amd64\s*=\s*"\K[^"]+' "$recipe_file" 2>/dev/null || true)
    if [[ -n "$arch_mapped" ]]; then
        url="${url//\{arch\}/$arch_mapped}"
    fi

    echo "$url"
}

# Download and return checksum content
fetch_checksum() {
    local url="$1"
    curl -sfL --max-time 30 "$url" 2>/dev/null || true
}

log "Scanning recipes in $RECIPE_DIR"

# Find recipes with checksum_url
mapfile -t recipes < <(grep -rl "checksum_url" "$RECIPE_DIR"/*.toml 2>/dev/null || true)

if [[ ${#recipes[@]} -eq 0 ]]; then
    log "No recipes with checksum_url found"
    exit 0
fi

log "Found ${#recipes[@]} recipe(s) with checksum_url"

for recipe in "${recipes[@]}"; do
    name=$(basename "$recipe" .toml)
    log "Checking $name..."

    # Extract checksum_url
    checksum_url=$(grep -oP 'checksum_url\s*=\s*"\K[^"]+' "$recipe" 2>/dev/null || true)
    if [[ -z "$checksum_url" ]]; then
        warn "$name: Could not extract checksum_url"
        continue
    fi

    # Resolve version
    version=$(resolve_version "$recipe" || true)
    if [[ -z "$version" ]]; then
        warn "$name: Could not resolve version, skipping"
        continue
    fi
    log "  Version: $version"

    # Resolve checksum URL with template variables
    resolved_url=$(resolve_url "$checksum_url" "$version")
    resolved_url=$(apply_mappings "$resolved_url" "$recipe")
    log "  Checksum URL: $resolved_url"

    # Download current checksum
    current_checksum=$(fetch_checksum "$resolved_url")
    if [[ -z "$current_checksum" ]]; then
        warn "$name: Failed to fetch checksum from $resolved_url"
        continue
    fi

    # Compare against stored baseline (if exists)
    baseline_file="data/checksums/${name}-${version}.sha256"
    if [[ -f "$baseline_file" ]]; then
        stored_checksum=$(cat "$baseline_file")
        if [[ "$current_checksum" != "$stored_checksum" ]]; then
            log "  DRIFT DETECTED for $name v$version"
            DRIFT_DETECTED=1
            RESULTS+=("$(jq -n \
                --arg name "$name" \
                --arg version "$version" \
                --arg recipe_path "$recipe" \
                --arg checksum_url "$resolved_url" \
                --arg expected "$(echo "$stored_checksum" | head -c 128)" \
                --arg actual "$(echo "$current_checksum" | head -c 128)" \
                '{name: $name, version: $version, recipe: $recipe_path, url: $checksum_url, expected: $expected, actual: $actual}'
            )")
        else
            log "  OK - checksum unchanged"
        fi
    else
        log "  No baseline found, storing initial checksum"
        mkdir -p "data/checksums"
        echo "$current_checksum" > "$baseline_file"
    fi

    # Rate limiting between requests
    sleep 2
done

# Output results
if [[ $DRIFT_DETECTED -eq 1 ]]; then
    log "Drift detected in ${#RESULTS[@]} recipe(s)"
    # Output as JSON array for the workflow to consume
    printf '%s\n' "${RESULTS[@]}" | jq -s '.' > /tmp/checksum-drift-results.json
    cat /tmp/checksum-drift-results.json
    exit 1
else
    log "No drift detected"
    exit 0
fi
