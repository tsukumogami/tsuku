#!/usr/bin/env bash
set -euo pipefail

# Opens, updates and closes the tracked item for one workflow's run.
#
# Shared by the listener (.github/workflows/escalate.yml) and the hourly sweeper
# (.github/workflows/escalate-sweep.yml) so the two paths cannot disagree about what
# escalation means. They differ only in how they learn a run happened.
#
# Usage:
#   escalate.sh --workflow <name> --conclusion <c> --run-id <id> --run-url <url> \
#               --event <event> [--dry-run]
#
# Exit codes:
#   0  handled: filed, commented, closed, or deliberately not escalated
#   1  a step that was supposed to deliver did not
#   2  operational error (bad arguments, missing declaration, unreadable registry)
#
# NOTHING HERE SUPPRESSES ITS OWN FAILURE.
#
# No `2>/dev/null`, no `|| true` on a delivery step, no `continue-on-error`. A failure to
# escalate is the thing this repository spent a milestone learning it could not see, so it
# is reported as a failure rather than absorbed. The one `|| true` below is on a read whose
# emptiness is a legitimate answer, and it is commented as such.

DRY_RUN=0
WORKFLOW="" CONCLUSION="" RUN_ID="" RUN_URL="" EVENT=""
WORKFLOWS_DIR="${WORKFLOWS_DIR:-.github/workflows}"

while [ $# -gt 0 ]; do
  case "$1" in
    --workflow)   WORKFLOW="$2"; shift 2 ;;
    --conclusion) CONCLUSION="$2"; shift 2 ;;
    --run-id)     RUN_ID="$2"; shift 2 ;;
    --run-url)    RUN_URL="$2"; shift 2 ;;
    --event)      EVENT="$2"; shift 2 ;;
    --dry-run)    DRY_RUN=1; shift ;;
    *) echo "::error::unknown argument: $1" >&2; exit 2 ;;
  esac
done

for required in WORKFLOW CONCLUSION RUN_ID RUN_URL EVENT; do
  if [ -z "${!required}" ]; then
    echo "::error::--${required,,} is required" >&2
    exit 2
  fi
done

# --- the declaration -------------------------------------------------------------------
#
# Read from the checkout, which for both callers is the default branch. The listener and
# the sweeper both run their own workflow file from the default branch, and both check out
# that branch explicitly, so a pull request cannot change a policy or an assignee by
# editing a comment in its own diff.

declaration_file=""
for f in "$WORKFLOWS_DIR"/*.yml "$WORKFLOWS_DIR"/*.yaml; do
  [ -e "$f" ] || continue
  name=$(awk '/^name:/{sub(/^name:[[:space:]]*/,""); gsub(/^["'"'"']|["'"'"']$/,""); print; exit}' "$f")
  if [ "$name" = "$WORKFLOW" ]; then declaration_file="$f"; break; fi
done

if [ -z "$declaration_file" ]; then
  echo "::error::no workflow file declares name \"$WORKFLOW\"; cannot read its policy" >&2
  exit 2
fi

decl_value() {  # decl_value <key>
  awk -v key="$1" '
    !/^#/ { exit }
    { line=$0; sub(/^#[[:space:]]*/,"",line)
      if (index(line, key ":") == 1) { sub(/^[^:]*:[[:space:]]*/,"",line); print line; exit } }
  ' "$declaration_file"
}

POLICY=$(decl_value "escalation-policy")
ASSIGNEE=$(decl_value "escalation-assignee")
ONLY_ON=$(decl_value "escalation-only-on")

if [ -z "$POLICY" ]; then
  echo "::error file=${declaration_file}::no escalation-policy declared" >&2
  exit 2
fi

# --- eligibility -----------------------------------------------------------------------

if [ -n "$ONLY_ON" ] && [ "$ONLY_ON" != "$EVENT" ]; then
  echo "not eligible: \"$WORKFLOW\" escalates only on '$ONLY_ON' and this run was '$EVENT'"
  exit 0
fi

if [ "$POLICY" = "none" ]; then
  # Reported rather than skipped in silence. The sweeper counts these; see its summary.
  echo "policy-none: \"$WORKFLOW\" declares escalation-policy: none, not filing"
  exit 0
fi

TITLE="Scheduled workflow failing: $WORKFLOW"

# `maintenance` rather than a new label. It already exists, is already declared in
# .github/labels.yml, and is already what the sibling housekeeping workflow files under --
# so no bootstrap is needed and the label-reference check passes on the first run. A
# dedicated `escalation` label would query more precisely, but it would have to be created
# before this could file, which is the same ordering trap that made the label repair a
# staged job. Dedup here is title-keyed and does not depend on the label.
ESCALATION_LABEL="maintenance"

# --- find the tracked item ---------------------------------------------------------------
#
# Title-keyed, so three runs against the same failure converge on one item. `|| true` is
# legitimate here and only here: no matching issue is a real answer, not an error.

EXISTING=$(gh issue list --search "\"$TITLE\" in:title" --state open \
             --json number --jq '.[0].number // empty' || true)

if [ "$CONCLUSION" = "success" ]; then
  if [ -z "$EXISTING" ]; then
    echo "healthy, and nothing open for \"$WORKFLOW\""
    exit 0
  fi
  if [ "$DRY_RUN" = "1" ]; then echo "[dry-run] would close #$EXISTING"; exit 0; fi
  BODY="Recovered. [Run]($RUN_URL) concluded \`success\`."
  gh issue comment "$EXISTING" --body "$BODY"
  gh issue close "$EXISTING" --reason completed
  echo "closed #$EXISTING for \"$WORKFLOW\""
  exit 0
fi

BODY="[Run]($RUN_URL) concluded \`$CONCLUSION\` (run id $RUN_ID, event \`$EVENT\`)."

if [ -n "$EXISTING" ]; then
  if [ "$DRY_RUN" = "1" ]; then echo "[dry-run] would comment on #$EXISTING"; exit 0; fi
  gh issue comment "$EXISTING" --body "$BODY"
  echo "commented on #$EXISTING for \"$WORKFLOW\""
  exit 0
fi

# --- assignee pre-flight -----------------------------------------------------------------
#
# The REST API ACCEPTS an assignee who lacks repository access and silently discards them,
# so sending one proves nothing. Ask first.

if [ -z "$ASSIGNEE" ]; then
  echo "::error file=${declaration_file}::escalation-policy: issue with no escalation-assignee" >&2
  exit 2
fi

if ! gh api "repos/${GITHUB_REPOSITORY}/assignees/${ASSIGNEE}" --silent; then
  echo "::error file=${declaration_file}::\"$ASSIGNEE\" is not assignable on ${GITHUB_REPOSITORY}." \
       "Filing an issue that names them would silently drop the assignment." >&2
  exit 1
fi

if [ "$DRY_RUN" = "1" ]; then echo "[dry-run] would file \"$TITLE\" assigned to $ASSIGNEE"; exit 0; fi

NUMBER=$(gh issue create --title "$TITLE" --body "$BODY" \
           --assignee "$ASSIGNEE" --label "$ESCALATION_LABEL" \
         | grep -oE '[0-9]+$')

if [ -z "$NUMBER" ]; then
  echo "::error::issue creation returned no number" >&2
  exit 1
fi

# --- assignee read-back --------------------------------------------------------------------
#
# Assert identity against what the API reports, rather than trusting that the request was
# accepted. An assignment that was accepted and discarded looks exactly like one that
# worked, from the sending side.

ACTUAL=$(gh issue view "$NUMBER" --json assignees --jq '[.assignees[].login] | join(",")')
case ",${ACTUAL}," in
  *",${ASSIGNEE},"*) : ;;
  *)
    echo "::error::filed #$NUMBER but its assignees are [${ACTUAL}], not including ${ASSIGNEE}." \
         "The assignment was accepted and discarded." >&2
    exit 1 ;;
esac

echo "filed #$NUMBER for \"$WORKFLOW\", assigned to $ASSIGNEE and read back"
