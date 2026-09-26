# shellcheck shell=bash
# R2 golden-plan object layout.
#
# The one place that defines where golden plans live in the R2 bucket. Everything that
# writes, reads, lists or prunes plan objects sources this file instead of spelling the
# key itself. Two scripts that each spelled the prefix their own way are how publishing
# came to write plans/registry/... while every reader looked under plans/<letter>/... and
# found nothing (#2448).
#
# Layout:
#   plans/<segment>/<recipe>/v<version>/<platform>.json
#
#   segment = "embedded"               for embedded recipes (internal/recipe/recipes/)
#   segment = first letter of recipe   for registry recipes (recipes/<letter>/)
#
# Examples:
#   plans/embedded/go/v1.25.5/linux-amd64.json
#   plans/f/fzf/v0.60.0/darwin-arm64.json
#
# A registry segment is always exactly one character, so it can never collide with
# "embedded". Callers pass a CATEGORY ("embedded" or "registry"), never a segment: the
# mapping from category to segment happens here and nowhere else.
#
# Usage:
#   source "$(dirname "${BASH_SOURCE[0]}")/lib/r2-layout.sh"
#   key=$(r2_plan_key fzf registry 0.60.0 linux-amd64)

R2_PLANS_ROOT="plans"

# The prefix publishing wrote registry plans under before #2448. Nothing reads it; it
# exists so the one-off migration can name its source without spelling it by hand.
R2_LEGACY_REGISTRY_PREFIX="$R2_PLANS_ROOT/registry/"

# r2_plan_segment <recipe> <category>
# Prints the segment for a recipe. Fails on an unknown category rather than passing it
# through, so a caller cannot put an arbitrary word into the key.
r2_plan_segment() {
    local recipe="$1" category="$2"
    if [[ -z "$recipe" ]]; then
        echo "r2_plan_segment: empty recipe name" >&2
        return 2
    fi
    case "$category" in
        embedded) echo "embedded" ;;
        registry) echo "${recipe:0:1}" ;;
        *)
            echo "r2_plan_segment: unknown category '$category' (must be embedded or registry)" >&2
            return 2
            ;;
    esac
}

# r2_plan_prefix <recipe> <category>
# Prints the prefix holding every version and platform of a recipe, with a trailing slash.
r2_plan_prefix() {
    local segment
    segment=$(r2_plan_segment "$1" "$2") || return
    echo "$R2_PLANS_ROOT/$segment/$1/"
}

# r2_plan_key <recipe> <category> <version> <platform>
# Prints the object key for one plan. <version> is given without a leading "v".
r2_plan_key() {
    local prefix
    prefix=$(r2_plan_prefix "$1" "$2") || return
    echo "${prefix}v${3#v}/$4.json"
}

# r2_segment_category <segment>
# Prints the category a segment belongs to, or fails if the segment is not part of the
# layout (for example the legacy "registry" segment).
r2_segment_category() {
    local segment="$1"
    if [[ "$segment" == "embedded" ]]; then
        echo "embedded"
    elif [[ "$segment" =~ ^[a-z0-9]$ ]]; then
        echo "registry"
    else
        return 1
    fi
}

# r2_parse_plan_key <key>
# Splits a plan key into R2_KEY_SEGMENT, R2_KEY_CATEGORY, R2_KEY_RECIPE, R2_KEY_VERSION
# (without "v") and R2_KEY_PLATFORM. Returns 1, leaving them empty, for a key that is not
# a plan in this layout, including a legacy plans/registry/ key and a registry key whose
# segment does not match its recipe's first letter.
r2_parse_plan_key() {
    R2_KEY_SEGMENT="" R2_KEY_CATEGORY="" R2_KEY_RECIPE="" R2_KEY_VERSION="" R2_KEY_PLATFORM=""
    local key="$1"
    [[ "$key" =~ ^${R2_PLANS_ROOT}/([^/]+)/([^/]+)/v([^/]+)/([^/]+)\.json$ ]] || return 1
    local segment="${BASH_REMATCH[1]}" recipe="${BASH_REMATCH[2]}"
    local version="${BASH_REMATCH[3]}" platform="${BASH_REMATCH[4]}"
    local category
    category=$(r2_segment_category "$segment") || return 1
    if [[ "$category" == "registry" && "$segment" != "${recipe:0:1}" ]]; then
        return 1
    fi
    R2_KEY_SEGMENT="$segment"
    R2_KEY_CATEGORY="$category"
    R2_KEY_RECIPE="$recipe"
    R2_KEY_VERSION="$version"
    R2_KEY_PLATFORM="$platform"
}

# r2_legacy_registry_key_to_current <key>
# Maps a legacy plans/registry/<recipe>/v<version>/<platform>.json key to its key in the
# current layout. Fails for anything else.
r2_legacy_registry_key_to_current() {
    local key="$1"
    [[ "$key" =~ ^${R2_LEGACY_REGISTRY_PREFIX}([^/]+)/v([^/]+)/([^/]+)\.json$ ]] || return 1
    r2_plan_key "${BASH_REMATCH[1]}" registry "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
}
