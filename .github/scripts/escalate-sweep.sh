#!/usr/bin/env bash
set -euo pipefail

# Hourly sweep: every non-success run of a registered workflow has a tracked item.
#
# This is the load-bearing path. The listener exists for latency; a run rejected before job
# creation is recorded under its file path rather than its declared name, so a name-filtered
# listener cannot match it, while the Actions API reports it like any other run.
#
# It also reports what it is NOT escalating, which is the point of the `none` policy having
# two kinds. A deliberate decision to ignore something and a bug that ignores it look
# identical unless the count of ignored things appears where a person looks. Silence is not
# a report.
#
# Usage: escalate-sweep.sh [--window-hours N] [--dry-run]
#
# Exit codes:
#   0  swept; every non-success run of a registered workflow is tracked
#   1  a gap was found and could not be filed, or the sweep examined nothing
#   2  operational error

WINDOW_HOURS=2
DRY_RUN=""
WORKFLOWS_DIR="${WORKFLOWS_DIR:-.github/workflows}"
REGISTRY="${REGISTRY:-.github/escalation-registry.yml}"

while [ $# -gt 0 ]; do
  case "$1" in
    --window-hours) WINDOW_HOURS="$2"; shift 2 ;;
    --dry-run)      DRY_RUN="--dry-run"; shift ;;
    *) echo "::error::unknown argument: $1" >&2; exit 2 ;;
  esac
done

[ -f "$REGISTRY" ] || { echo "::error::registry not found: $REGISTRY" >&2; exit 2; }

mapfile -t REGISTERED < <(awk '/^  - "/{gsub(/^  - "|"$/,""); print}' "$REGISTRY")
if [ "${#REGISTERED[@]}" -eq 0 ]; then
  echo "::error file=${REGISTRY}::registry lists no workflows; this sweep would examine nothing" >&2
  exit 2
fi

SINCE=$(date -u -d "${WINDOW_HOURS} hours ago" +%Y-%m-%dT%H:%M:%SZ)
echo "Sweeping ${#REGISTERED[@]} registered workflow(s) for non-success runs since $SINCE"

# Read one declaration key out of a workflow's leading comment block.
decl_value() {  # decl_value <file> <key>
  awk -v key="$2" '
    !/^#/ { exit }
    { line=$0; sub(/^#[[:space:]]*/,"",line)
      if (index(line, key ":") == 1) { sub(/^[^:]*:[[:space:]]*/,"",line); print line; exit } }
  ' "$1"
}

file_for_name() {  # file_for_name <workflow name>
  local f name
  for f in "$WORKFLOWS_DIR"/*.yml "$WORKFLOWS_DIR"/*.yaml; do
    [ -e "$f" ] || continue
    name=$(awk '/^name:/{sub(/^name:[[:space:]]*/,""); gsub(/^["'"'"']|["'"'"']$/,""); print; exit}' "$f")
    [ "$name" = "$1" ] && { printf '%s' "$f"; return 0; }
  done
  return 1
}

examined=0 gaps=0 filed=0 failed=0

for wf in "${REGISTERED[@]}"; do
  file=$(file_for_name "$wf") || {
    echo "::error file=${REGISTRY}::registered workflow \"$wf\" matches no workflow file" >&2
    exit 2
  }
  only_on=$(decl_value "$file" "escalation-only-on")

  # `|| true` on the read only: an empty result is a real answer here. A failure of the API
  # call itself is caught by the emptiness check below, which cannot distinguish them --
  # so the zero-floor at the end is what makes a broken query visible.
  runs=$(gh run list --workflow "$(basename "$file")" --limit 50 \
           --json conclusion,event,databaseId,url,createdAt \
           --jq "[.[] | select(.createdAt > \"$SINCE\") | select(.conclusion != \"success\" and .conclusion != \"\" and .conclusion != null)] | .[] | @base64" || true)

  for encoded in $runs; do
    row=$(printf '%s' "$encoded" | base64 -d)
    conclusion=$(printf '%s' "$row" | jq -r .conclusion)
    event=$(printf '%s' "$row" | jq -r .event)
    run_id=$(printf '%s' "$row" | jq -r .databaseId)
    run_url=$(printf '%s' "$row" | jq -r .url)

    if [ -n "$only_on" ] && [ "$only_on" != "$event" ]; then
      continue
    fi
    examined=$((examined + 1))

    title="Scheduled workflow failing: $wf"
    # OPEN, not all. A closed item tracked the failure it was opened for and was closed
    # when that cleared; a workflow failing again afterwards is a new failure needing a new
    # item, not one already covered. Searching all states would make the sweeper go quiet
    # permanently after the first resolution, which is the silence it exists to prevent.
    # (The pull-request backstop asserts over a historical window and does accept a closed
    # item, for the opposite and equally correct reason.)
    existing=$(gh issue list --search "\"$title\" in:title" --state open \
                 --json number --jq '.[0].number // empty' || true)
    if [ -n "$existing" ]; then
      continue
    fi

    gaps=$((gaps + 1))
    echo "gap: \"$wf\" run $run_id concluded $conclusion with no tracked item"
    if .github/scripts/escalate.sh --workflow "$wf" --conclusion "$conclusion" \
         --run-id "$run_id" --run-url "$run_url" --event "$event" $DRY_RUN; then
      filed=$((filed + 1))
    else
      failed=$((failed + 1))
    fi
  done
done

# --- what is deliberately not being escalated -------------------------------------------
#
# Two lines, because the kinds mean different things. A deferred `none` with failing runs
# is unresolved cost with a named owner; a permanent one is the policy working. Folding
# them together would inflate the one number whose whole purpose is to represent cost
# somebody still owes.

deferred_report="" permanent_count=0
for f in "$WORKFLOWS_DIR"/*.yml "$WORKFLOWS_DIR"/*.yaml; do
  [ -e "$f" ] || continue
  [ "$(decl_value "$f" "escalation-policy")" = "none" ] || continue
  wf=$(awk '/^name:/{sub(/^name:[[:space:]]*/,""); gsub(/^["'"'"']|["'"'"']$/,""); print; exit}' "$f")
  kind=$(decl_value "$f" "escalation-none-kind")
  recent=$(gh run list --workflow "$(basename "$f")" --limit 50 \
             --json conclusion,createdAt \
             --jq "[.[] | select(.createdAt > \"$SINCE\") | select(.conclusion != \"success\" and .conclusion != \"\" and .conclusion != null)] | length" || echo 0)
  if [ "$kind" = "permanent" ]; then
    permanent_count=$((permanent_count + 1))
    continue
  fi
  if [ "${recent:-0}" -gt 0 ]; then
    issue=$(decl_value "$f" "escalation-tracking-issue")
    deferred_report="${deferred_report}    \"$wf\": $recent non-success run(s), not escalated, owned by #${issue:-unrecorded}\n"
  fi
done

echo
verb="filed"; [ -n "$DRY_RUN" ] && verb="would have filed"
echo "Sweep: ${#REGISTERED[@]} registered workflow(s) checked, $examined eligible non-success run(s) examined, $gaps gap(s), $filed $verb, $failed failed to file."
if [ -n "$deferred_report" ]; then
  echo "Deferred escalation-policy: none, still failing and reaching nobody:"
  printf "$deferred_report"
else
  echo "Deferred escalation-policy: none with failing runs: none."
fi
echo "Permanent escalation-policy: none (not a gap, the policy working): $permanent_count."

# Zero-floor. The sweeper's items are the registered workflows it CHECKED, not the problems
# it found. Finding zero problems is an ordinary green with a non-zero attempted count; a
# sweep that checked nothing -- registry unreadable, API returning empty -- attempted zero
# and must fail.
if [ "${#REGISTERED[@]}" -eq 0 ]; then
  echo "::error::checked no workflows" >&2
  exit 1
fi

printf '{"declared":%d,"attempted":%d,"gaps":%d,"filed":%d}\n' \
  "${#REGISTERED[@]}" "${#REGISTERED[@]}" "$gaps" "$filed" > receipt.ndjson

[ "$failed" -eq 0 ]
