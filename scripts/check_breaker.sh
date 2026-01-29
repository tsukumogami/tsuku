#!/bin/bash
# check_breaker.sh - Check circuit breaker state for an ecosystem
# Usage: ./scripts/check_breaker.sh <ecosystem>
#
# Reads the circuit breaker state from batch-control.json and determines
# whether processing should proceed for the given ecosystem.
#
# Outputs (to stdout or GITHUB_OUTPUT if available):
#   skip=true|false
#   state=closed|open|half-open
#
# Exit code is always 0 (the job should not fail due to breaker state).

set -euo pipefail

ECOSYSTEM="${1:?Usage: $0 <ecosystem>}"
CONTROL_FILE="${CONTROL_FILE:-batch-control.json}"

# Read current state (default to closed if not present)
state=$(jq -r --arg eco "$ECOSYSTEM" '.circuit_breaker[$eco].state // "closed"' "$CONTROL_FILE")

output_var() {
  local key="$1" value="$2"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    echo "${key}=${value}" >> "$GITHUB_OUTPUT"
  fi
  echo "${key}=${value}"
}

case "$state" in
  closed)
    output_var "skip" "false"
    output_var "state" "closed"
    ;;
  open)
    opens_at=$(jq -r --arg eco "$ECOSYSTEM" '.circuit_breaker[$eco].opens_at // ""' "$CONTROL_FILE")
    if [ -z "$opens_at" ]; then
      output_var "skip" "true"
      output_var "state" "open"
      echo "::warning::Circuit breaker open for $ECOSYSTEM (no recovery time set)"
      exit 0
    fi

    now=$(date -u +%s)
    opens_at_epoch=$(date -u -d "$opens_at" +%s 2>/dev/null || date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$opens_at" +%s 2>/dev/null || echo "0")

    if [ "$now" -lt "$opens_at_epoch" ]; then
      output_var "skip" "true"
      output_var "state" "open"
      echo "::warning::Circuit breaker open for $ECOSYSTEM until $opens_at"
    else
      # Recovery timeout elapsed, transition to half-open
      jq --arg eco "$ECOSYSTEM" '.circuit_breaker[$eco].state = "half-open"' \
        "$CONTROL_FILE" > "${CONTROL_FILE}.tmp" && mv "${CONTROL_FILE}.tmp" "$CONTROL_FILE"
      output_var "skip" "false"
      output_var "state" "half-open"
      echo "::notice::Circuit breaker half-open for $ECOSYSTEM (probe request)"
    fi
    ;;
  half-open)
    output_var "skip" "false"
    output_var "state" "half-open"
    echo "::notice::Circuit breaker half-open for $ECOSYSTEM (probe request)"
    ;;
  *)
    echo "::warning::Unknown circuit breaker state '$state' for $ECOSYSTEM, treating as closed"
    output_var "skip" "false"
    output_var "state" "closed"
    ;;
esac
