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

WINDOW_HOURS=""          # empty means derive from the last completed sweep
DEFAULT_FIRST_WINDOW_HOURS=24
# A runaway guard, not a horizon. The real bound is the window: paging stops as soon as a
# page predates SINCE, so how far back it reaches follows from what the sweep claims to
# cover rather than from a number chosen in advance. A fixed page count would be the same
# kind of nominal figure as the two-hour lookback this sweeper just stopped using -- fine
# at today's rate, silently short on a busier week.
#
# Safe because the API returns runs newest-first, verified across a page boundary: once a
# page's oldest run predates the window, no later page can contain a newer one.
STARTUP_PAGE_GUARD=200
startup_query_failed=0
DRY_RUN=""
WORKFLOWS_DIR="${WORKFLOWS_DIR:-.github/workflows}"
ESCALATION_LABEL="${ESCALATION_LABEL:-maintenance}"
REGISTRY="${REGISTRY:-.github/escalation-registry.yml}"

while [ $# -gt 0 ]; do
  case "$1" in
    --window-hours) WINDOW_HOURS="$2"; shift 2 ;;
    --dry-run)      DRY_RUN="--dry-run"; shift ;;
    *) echo "::error::unknown argument: $1" >&2; exit 2 ;;
  esac
done

[ -f "$REGISTRY" ] || { echo "::error::registry not found: $REGISTRY" >&2; exit 2; }

# Only runs on the default branch are escalated (#2686), and every run query below is
# filtered to it -- the window start, the recovery gate, the gap scan, the startup pass and
# the deferred count. A branch run left in any one of them either files a failure main does
# not have or, worse, reads as a recovery main has not made. Asked of the API rather than
# taken from the event, which for a schedule does not carry the repository; DEFAULT_BRANCH
# overrides it for the self-tests. Could-not-look stops the sweep rather than sweeping
# every branch.
if [ -z "${DEFAULT_BRANCH:-}" ]; then
  if ! DEFAULT_BRANCH=$(gh api "repos/${GITHUB_REPOSITORY}" --jq '.default_branch') || [ -z "$DEFAULT_BRANCH" ]; then
    echo "::error::could not read the repository's default branch; not sweeping every branch instead" >&2
    exit 2
  fi
fi
export DEFAULT_BRANCH
echo "Branch: only runs on '$DEFAULT_BRANCH' are examined"

mapfile -t REGISTERED < <(awk '/^  - "/{gsub(/^  - "|"$/,""); print}' "$REGISTRY")
if [ "${#REGISTERED[@]}" -eq 0 ]; then
  echo "::error file=${REGISTRY}::registry lists no workflows; this sweep would examine nothing" >&2
  exit 2
fi

# The window starts where the last COMPLETED sweep started, not a fixed number of hours
# ago. Sizing it to the nominal cron interval assumes the schedule fires on time, and it
# does not: this repository's hourly crons fire roughly every five hours (#2652), so a
# two-hour window examined about 40% of the clock and the rest was never looked at by
# anything. R2 Health Monitor failed at 2026-09-25T16:12:45Z and fell in one of those holes
# permanently.
#
# Deriving it from the previous sweep makes lateness free: however long the gap, the next
# run covers all of it. An explicit --window-hours still overrides, for a deliberate wide
# sweep.
if [ -n "$WINDOW_HOURS" ]; then
  SINCE=$(date -u -d "${WINDOW_HOURS} hours ago" +%Y-%m-%dT%H:%M:%SZ)
  echo "Window: explicit, ${WINDOW_HOURS}h"
else
  # `status=success` already excludes the run doing the asking, which is in progress. The
  # branch filter keeps a green sweep dispatched from a branch from moving the window past
  # failures no sweep on the default branch has examined.
  SINCE=$(gh api "repos/${GITHUB_REPOSITORY}/actions/workflows/escalate-sweep.yml/runs?status=success&branch=${DEFAULT_BRANCH}&per_page=1" \
            --jq '.workflow_runs[0].created_at // empty' 2>/dev/null || true)
  if [ -z "$SINCE" ]; then
    SINCE=$(date -u -d "${DEFAULT_FIRST_WINDOW_HOURS} hours ago" +%Y-%m-%dT%H:%M:%SZ)
    echo "Window: no previous successful sweep found; falling back to ${DEFAULT_FIRST_WINDOW_HOURS}h"
  else
    echo "Window: since the last completed sweep at $SINCE"
  fi
fi
echo "Sweeping ${#REGISTERED[@]} registered workflow(s) for non-success runs since $SINCE"

# --- runs rejected before job creation, identified once ------------------------------------
#
# Such a run has no readable `name:`, so GitHub records it under the workflow's FILE PATH.
# That is the only signal, and reading it needs care: `gh run list --json workflowName`
# resolves to the workflow's CURRENT name, so once a broken workflow is fixed the evidence
# that it was ever rejected disappears from that field. Only the REST API keeps the name as
# recorded (#2651).
#
# One query serves both readings: the report below, and the skip in the loop that would
# otherwise escalate these as ordinary failures under a name they never had.
# Paginated deliberately. One page is a horizon, not a window: this repository produced 508
# runs in five days, so `per_page=100` reaches back about a day and silently omits anything
# older -- which is how the first version of this query returned nothing while both runs it
# was looking for sat two pages back. Walk until the page is older than SINCE.
STARTUP_IDS=""
startup_rest=""
page=1
while [ "$page" -le "$STARTUP_PAGE_GUARD" ]; do
  if ! body=$(gh api "repos/${GITHUB_REPOSITORY}/actions/runs?per_page=100&page=${page}&created=>=${SINCE%%T*}" 2>/dev/null); then
    echo "::error::could not page the run list at page ${page}; startup failures may be" \
         "unreported for this sweep" >&2
    startup_query_failed=1
    break
  fi
  count=$(printf '%s' "$body" | jq '.workflow_runs | length')
  [ "${count:-0}" -eq 0 ] && break
  startup_rest="${startup_rest}$(printf '%s' "$body" | jq -r --arg b "$DEFAULT_BRANCH" '.workflow_runs[] | select(.head_branch == $b) | select(.conclusion == "failure") | select(.name | startswith(".github/")) | [(.id|tostring), .name, .html_url] | @tsv')
"
  oldest=$(printf '%s' "$body" | jq -r '[.workflow_runs[].created_at] | min')
  [[ "$oldest" < "$SINCE" ]] && break
  page=$((page + 1))
done
if [ "$page" -gt "$STARTUP_PAGE_GUARD" ]; then
  echo "::error::could not reach the start of my own window ($SINCE) within" \
       "${STARTUP_PAGE_GUARD} pages, so this sweep did not examine the period it claims" \
       "to cover" >&2
  startup_query_failed=1
fi
while IFS=$'\t' read -r sid sname surl; do
  [ -n "$sid" ] || continue
  STARTUP_IDS="$STARTUP_IDS $sid"
done <<< "$startup_rest"

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

examined=0 gaps=0 filed=0 failed=0 self_filed=0 recovered=0 attempted=0 unreachable=0
recovered_report=""
self_report=""
owned_report=""

for wf in "${REGISTERED[@]}"; do
  file=$(file_for_name "$wf") || {
    echo "::error file=${REGISTRY}::registered workflow \"$wf\" matches no workflow file" >&2
    exit 2
  }
  only_on=$(decl_value "$file" "escalation-only-on")
  self_files=$(decl_value "$file" "escalation-self-files")
  owned_by=$(decl_value "$file" "escalation-owned-by"); owned_by="${owned_by#\#}"
  owner_state=""

  # --- the current-state gate -------------------------------------------------------------
  #
  # Escalate only what is failing NOW. Without this the sweep files on any non-success run
  # inside the window regardless of what happened since, so a workflow that failed and then
  # recovered gets an issue announcing a condition that has already cleared. Measured on
  # 2026-09-25: seven of the fourteen workflows a wide sweep would have escalated were green
  # at the time, Scheduled Tests among them — the escalator would have announced it broken on
  # the day #2596's repair made it pass.
  #
  # Recovered workflows are COUNTED AND NAMED rather than silently skipped, because a
  # deliberate skip and a missed workflow look identical in an absence.
  # The exit status is kept here too. A failed query leaves `latest` empty, which is not
  # "not successful" -- it is "unknown", and treating it as the former would escalate a
  # workflow whose state was never read.
  if [ -n "$only_on" ]; then
    latest=$(gh run list --workflow "$(basename "$file")" --branch "$DEFAULT_BRANCH" --limit 30 \
               --json conclusion,event,createdAt \
               --jq "[.[] | select(.event == \"$only_on\") | select(.conclusion != \"\" and .conclusion != null)] | .[0].conclusion // empty" 2>/dev/null)
  else
    latest=$(gh run list --workflow "$(basename "$file")" --branch "$DEFAULT_BRANCH" --limit 30 \
               --json conclusion,createdAt \
               --jq '[.[] | select(.conclusion != "" and .conclusion != null)] | .[0].conclusion // empty' 2>/dev/null)
  fi
  if [ $? -ne 0 ]; then
    unreachable=$((unreachable + 1))
    echo "::error::could not read the latest conclusion for \"$wf\"; NOT examined" >&2
    continue
  fi

  # Examined: its state was read and a decision made. That includes deciding not to
  # escalate it. An earlier version counted only workflows that reached the second query,
  # so the receipt reported 8 of 22 on a sweep that had assessed all 22 -- understating its
  # own coverage, in the field that exists to state it.
  attempted=$((attempted + 1))

  if [ "$latest" = "success" ]; then
    recovered=$((recovered + 1))
    recovered_report="${recovered_report}    \"$wf\": latest run succeeded; nothing escalated for earlier failures in the window\n"
    continue
  fi

  # The exit status is kept, not discarded. An empty result and a failed query are
  # different facts: the first means this workflow had no non-success runs in the window,
  # the second means we do not know. Collapsing them is how a receipt comes to say it
  # examined a population it never reached.
  if runs=$(gh run list --workflow "$(basename "$file")" --branch "$DEFAULT_BRANCH" --limit 50 \
              --json conclusion,event,databaseId,url,createdAt,workflowName,headBranch \
              --jq "[.[] | select(.createdAt > \"$SINCE\") | select(.conclusion != \"success\" and .conclusion != \"\" and .conclusion != null)] | .[] | @base64" 2>/dev/null); then
    :
  else
    attempted=$((attempted - 1))
    unreachable=$((unreachable + 1))
    echo "::error::could not query runs for \"$wf\"; this workflow was NOT examined" >&2
    continue
  fi

  for encoded in $runs; do
    row=$(printf '%s' "$encoded" | base64 -d)
    conclusion=$(printf '%s' "$row" | jq -r .conclusion)
    event=$(printf '%s' "$row" | jq -r .event)
    run_id=$(printf '%s' "$row" | jq -r .databaseId)
    run_url=$(printf '%s' "$row" | jq -r .url)
    branch=$(printf '%s' "$row" | jq -r .headBranch)

    if [ -n "$only_on" ] && [ "$only_on" != "$event" ]; then
      continue
    fi

    # A run rejected before job creation is handled by the startup pass below. Reporting it
    # here too would produce two tracked items for one run, and the less accurate of the
    # two: this run did not fail, it was never allowed to start.
    #
    # Membership of the id set, not a name test. `gh run list`'s workflowName is resolved,
    # so it says "Escalate" for a run recorded as ".github/workflows/escalate.yml" and the
    # name test silently stopped matching the moment the workflow was fixed (#2651).
    case " $STARTUP_IDS " in
      *" $run_id "*) continue ;;
    esac
    examined=$((examined + 1))

    # A workflow that still files its own issue is skipped because it SAID SO. Counted
    # and named below, never silently: from here, "escalated by the workflow itself" and
    # "escalated by nobody" are the same signal.
    if [ -n "$self_files" ]; then
      self_filed=$((self_filed + 1))
      self_report="${self_report}    \"$wf\": non-success run $run_id, not escalated because it files its own issue, migration tracked in #$self_files\n"
      continue
    fi

    # A failure already tracked by an open issue the workflow names is delivered there by
    # the escalator, not filed as a second item. The sweep does not comment again for runs
    # the listener already delivered; it names them, so "routed to its owner" is on the
    # record rather than inferred from nothing having been filed. A closed owner means the
    # declaration expired, and the run is treated like any other.
    if [ -n "$owned_by" ]; then
      if [ -z "$owner_state" ]; then
        owner_state=$(gh issue view "$owned_by" --json state --jq .state) || owner_state=""
      fi
      if [ -z "$owner_state" ]; then
        failed=$((failed + 1))
        echo "::error::cannot read the state of #$owned_by, which \"$wf\" names as its owner;" \
             "run $run_id was delivered nowhere this sweep can confirm" >&2
        continue
      fi
      if [ "$owner_state" = "OPEN" ]; then
        owned_report="${owned_report}    \"$wf\": non-success run $run_id routed to its owner #$owned_by, no item of its own\n"
        continue
      fi
    fi

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
         --run-id "$run_id" --run-url "$run_url" --event "$event" --branch "$branch" $DRY_RUN; then
      filed=$((filed + 1))
    else
      failed=$((failed + 1))
    fi
  done
done

# --- runs rejected before job creation --------------------------------------------------
#
# A workflow rejected at startup creates no jobs, and GitHub records the run under the
# file's PATH rather than its declared name. Every loop above matches on name, so this
# class is structurally invisible to them -- and to the listener, whose trigger is a name
# filter. It is the one failure mode this repository has established it cannot see.
#
# It is detectable, though, and cheaply: a run whose workflow name begins with `.github/`
# is one whose name could not be read. Both escalation workflows failed exactly this way
# on their first push and nothing reported it.

startup_failures=0
startup_report=""
while IFS=$'\t' read -r sf_name sf_id sf_url; do
  [ -n "$sf_name" ] || continue
  case "$sf_name" in
    .github/*) ;;
    *) continue ;;
  esac
  startup_failures=$((startup_failures + 1))
  startup_report="${startup_report}    $sf_name (run $sf_id)\n"
  title="Workflow rejected before job creation: $sf_name"
  existing=$(gh issue list --search "\"$title\" in:title" --state open \
               --json number --jq '.[0].number // empty' || true)
  [ -n "$existing" ] && continue
  if [ -n "$DRY_RUN" ]; then
    echo "[dry-run] would file \"$title\""
  else
    body="[Run]($sf_url) was rejected before any job was created, so it is recorded under its file path rather than its declared name. Nothing that matches on workflow name can see it -- including this repository's escalation listener. Common causes: invalid YAML, an unknown key, or an empty \`\${{ }}\` expression anywhere in the file."
    gh issue create --title "$title" --body "$body" --label "$ESCALATION_LABEL" >/dev/null
    echo "filed a startup-failure report for $sf_name"
  fi
done <<< "$(printf '%s' "$startup_rest" | awk -F'\t' 'NF{print $2"\t"$1"\t"$3}')"

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
  recent=$(gh run list --workflow "$(basename "$f")" --branch "$DEFAULT_BRANCH" --limit 50 \
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
if [ -n "$recovered_report" ]; then
  echo "Recovered — failed in the window, green now, deliberately not escalated:"
  printf "$recovered_report"
else
  echo "Recovered workflows skipped: none."
fi
echo "Permanent escalation-policy: none (not a gap, the policy working): $permanent_count."
if [ -n "$self_report" ]; then
  echo "Not escalated because the workflow files its own issue:"
  printf "$self_report"
else
  echo "Workflows filing their own issues, with failing runs: none."
fi
if [ -n "$owned_report" ]; then
  echo "Routed to the open issue that owns the failure (escalation-owned-by):"
  printf "$owned_report"
else
  echo "Owned workflows with failing runs: none."
fi
if [ "$startup_failures" -gt 0 ]; then
  echo "Runs rejected before job creation (invisible to every name-matching path):"
  printf "$startup_report"
elif [ "$startup_query_failed" -eq 1 ]; then
  echo "Runs rejected before job creation: UNKNOWN -- the query did not complete."
else
  echo "Runs rejected before job creation: none."
fi

# Zero-floor. The sweeper's items are the registered workflows it CHECKED, not the problems
# it found. Finding zero problems is an ordinary green with a non-zero attempted count; a
# sweep that checked nothing -- registry unreadable, API returning empty -- attempted zero
# and must fail.
# `attempted` is COUNTED, not restated. An earlier version wrote the registry's size into
# both fields, so the receipt could not express "attempted 0 of 22" -- the one thing it
# exists to be able to say. The floor below then guarded a number that could not be zero
# whenever the registry loaded, which is a floor that cannot fire.
if [ "$attempted" -eq 0 ]; then
  echo "::error::examined none of the ${#REGISTERED[@]} registered workflow(s); this sweep" \
       "establishes nothing and must not be treated as a completed one" >&2
  exit 1
fi

printf '{"declared":%d,"attempted":%d,"unreachable":%d,"gaps":%d,"filed":%d}\n' \
  "${#REGISTERED[@]}" "$attempted" "$unreachable" "$gaps" "$filed" > receipt.ndjson
echo "Receipt: declared ${#REGISTERED[@]}, attempted $attempted, unreachable $unreachable."

# A sweep that could not reach part of its population did not do its job, and must not
# advance the window for the next one. Failing here is what keeps that true, because the
# window lookup filters on a successful conclusion.
if [ "$unreachable" -gt 0 ]; then
  echo "::error::$unreachable of ${#REGISTERED[@]} registered workflow(s) could not be" \
       "examined; failing so the next sweep's window reaches back past this one" >&2
  exit 1
fi

[ "$failed" -eq 0 ]
