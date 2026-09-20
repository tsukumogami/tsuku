#!/usr/bin/env bash
set -euo pipefail

# Reconciles `.github/labels.yml` with the repository's labels, or reports the
# difference without changing anything.
#
#   label-manifest.sh --check   report drift, exit 1 if any (no writes)
#   label-manifest.sh --apply   create missing labels and update changed ones
#
# There is deliberately NO delete path. A label present on the repository but
# absent from the manifest is reported and left alone: deleting it would strip
# it from every issue already carrying it, and an absent code path cannot be
# switched on by mistake the way a disabled flag can.

MANIFEST="${MANIFEST_FILE:-.github/labels.yml}"
REPO="${GH_REPO:-tsukumogami/tsuku}"
MODE="${1:---check}"

case "$MODE" in
  --check|--apply) ;;
  *) echo "::error::usage: $0 [--check|--apply]" >&2; exit 2 ;;
esac

if [ ! -f "$MANIFEST" ]; then
  echo "::error::manifest not found: $MANIFEST" >&2
  exit 2
fi

# Parse the manifest. Deliberately a narrow parser over the exact shape this
# file is generated in, so a malformed entry fails here rather than silently
# contributing nothing.
# `|| true` on the assignment only: awk's own exit status must not kill the
# script under `set -e` before the PARSE_ERROR branch below can report what
# went wrong. Without this a malformed manifest exits 1 with no message at all.
DECLARED=$(awk '
  /^- name: "/ { name=$0; sub(/^- name: "/,"",name); sub(/"$/,"",name); next }
  /^  color: "/ { color=$0; sub(/^  color: "/,"",color); sub(/"$/,"",color); next }
  /^  description: "/ {
    desc=$0; sub(/^  description: "/,"",desc); sub(/"$/,"",desc)
    if (name == "" || color == "") { print "PARSE_ERROR"; bad=1 }
    printf "%s\t%s\t%s\n", name, color, desc
    name=""; color=""
  }
' "$MANIFEST" || true)

if printf '%s\n' "$DECLARED" | grep -q PARSE_ERROR; then
  echo "::error file=${MANIFEST}::malformed entry; every label needs name, color and description" >&2
  exit 2
fi

DECLARED_COUNT=$(printf '%s\n' "$DECLARED" | grep -c . || true)
if [ "${DECLARED_COUNT:-0}" -eq 0 ]; then
  echo "::error file=${MANIFEST}::declares no labels; this check examined nothing" >&2
  exit 1
fi

LIVE=$(gh label list --repo "$REPO" --limit 300 --json name,color,description)
LIVE_COUNT=$(printf '%s' "$LIVE" | jq 'length')
if [ "$LIVE_COUNT" -eq 0 ]; then
  echo "::error::the repository reports zero labels; refusing to treat that as drift" >&2
  exit 2
fi

MISSING=0 CHANGED=0 UNDECLARED=0

while IFS=$'\t' read -r name color desc; do
  [ -n "$name" ] || continue
  entry=$(printf '%s' "$LIVE" | jq -r --arg n "$name" '.[] | select(.name == $n) | "\(.color)\t\(.description // "")"')
  if [ -z "$entry" ]; then
    MISSING=$((MISSING + 1))
    if [ "$MODE" = "--apply" ]; then
      gh label create "$name" --repo "$REPO" --color "$color" --description "$desc"
      echo "created: $name"
    else
      echo "::error file=${MANIFEST}::label '${name}' is declared but does not exist on the repository" >&2
    fi
    continue
  fi
  live_color=$(printf '%s' "$entry" | cut -f1)
  live_desc=$(printf '%s' "$entry" | cut -f2)
  if [ "$live_color" != "$color" ] || [ "$live_desc" != "$desc" ]; then
    CHANGED=$((CHANGED + 1))
    if [ "$MODE" = "--apply" ]; then
      gh label edit "$name" --repo "$REPO" --color "$color" --description "$desc"
      echo "updated: $name"
    else
      echo "::error file=${MANIFEST}::label '${name}' differs from the repository (colour or description)" >&2
    fi
  fi
done <<< "$DECLARED"

# Reported, never removed.
while IFS= read -r name; do
  [ -n "$name" ] || continue
  if ! printf '%s\n' "$DECLARED" | cut -f1 | grep -qx "$name"; then
    UNDECLARED=$((UNDECLARED + 1))
    echo "::notice::label '${name}' exists on the repository but is not declared in ${MANIFEST}. It is left alone; add it there if it is wanted." >&2
  fi
done < <(printf '%s' "$LIVE" | jq -r '.[].name')

echo "Label manifest: ${DECLARED_COUNT} declared, ${LIVE_COUNT} on the repository, ${MISSING} missing, ${CHANGED} differing, ${UNDECLARED} undeclared."

if [ "$MODE" = "--check" ] && { [ "$MISSING" -gt 0 ] || [ "$CHANGED" -gt 0 ]; }; then
  exit 1
fi
exit 0
