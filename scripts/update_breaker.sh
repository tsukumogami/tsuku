#!/bin/bash
# update_breaker.sh - Update circuit breaker state after processing
# Usage: ./scripts/update_breaker.sh <ecosystem> <outcome>
#
# Arguments:
#   ecosystem - The ecosystem name (e.g., homebrew, cargo)
#   outcome   - "success" or "failure"
#
# State transitions:
#   CLOSED + success  -> CLOSED (reset failures to 0)
#   CLOSED + failure  -> CLOSED (increment failures) or OPEN (if failures >= 5)
#   HALF-OPEN + success -> CLOSED (reset failures)
#   HALF-OPEN + failure -> OPEN (fresh timeout)
#   OPEN              -> no change (should not be called in open state)

set -euo pipefail

ECOSYSTEM="${1:?Usage: $0 <ecosystem> <success|failure>}"
OUTCOME="${2:?Usage: $0 <ecosystem> <success|failure>}"
CONTROL_FILE="${CONTROL_FILE:-batch-control.json}"
FAILURE_THRESHOLD="${FAILURE_THRESHOLD:-5}"
RECOVERY_MINUTES="${RECOVERY_MINUTES:-60}"

state=$(jq -r --arg eco "$ECOSYSTEM" '.circuit_breaker[$eco].state // "closed"' "$CONTROL_FILE")

case "$OUTCOME" in
  success)
    jq --arg eco "$ECOSYSTEM" \
      '.circuit_breaker[$eco].state = "closed" |
       .circuit_breaker[$eco].failures = 0' \
      "$CONTROL_FILE" > "${CONTROL_FILE}.tmp" && mv "${CONTROL_FILE}.tmp" "$CONTROL_FILE"
    echo "Circuit breaker closed for $ECOSYSTEM (success)"
    ;;
  failure)
    now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    case "$state" in
      closed)
        failures=$(jq -r --arg eco "$ECOSYSTEM" '.circuit_breaker[$eco].failures // 0' "$CONTROL_FILE")
        failures=$((failures + 1))

        if [ "$failures" -ge "$FAILURE_THRESHOLD" ]; then
          opens_at=$(date -u -d "+${RECOVERY_MINUTES} minutes" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || \
                     date -u -v "+${RECOVERY_MINUTES}M" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || \
                     echo "")
          jq --arg eco "$ECOSYSTEM" --arg now "$now" --arg opens "$opens_at" --argjson f "$failures" \
            '.circuit_breaker[$eco].state = "open" |
             .circuit_breaker[$eco].failures = $f |
             .circuit_breaker[$eco].last_failure = $now |
             .circuit_breaker[$eco].opens_at = $opens' \
            "$CONTROL_FILE" > "${CONTROL_FILE}.tmp" && mv "${CONTROL_FILE}.tmp" "$CONTROL_FILE"
          echo "::error::Circuit breaker OPEN for $ECOSYSTEM ($failures failures, recovery at $opens_at)"
        else
          jq --arg eco "$ECOSYSTEM" --arg now "$now" --argjson f "$failures" \
            '.circuit_breaker[$eco].state = "closed" |
             .circuit_breaker[$eco].failures = $f |
             .circuit_breaker[$eco].last_failure = $now' \
            "$CONTROL_FILE" > "${CONTROL_FILE}.tmp" && mv "${CONTROL_FILE}.tmp" "$CONTROL_FILE"
          echo "Circuit breaker: $ECOSYSTEM failure $failures/$FAILURE_THRESHOLD"
        fi
        ;;
      half-open)
        opens_at=$(date -u -d "+${RECOVERY_MINUTES} minutes" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || \
                   date -u -v "+${RECOVERY_MINUTES}M" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || \
                   echo "")
        failures=$(jq -r --arg eco "$ECOSYSTEM" '.circuit_breaker[$eco].failures // 0' "$CONTROL_FILE")
        failures=$((failures + 1))
        jq --arg eco "$ECOSYSTEM" --arg now "$now" --arg opens "$opens_at" --argjson f "$failures" \
          '.circuit_breaker[$eco].state = "open" |
           .circuit_breaker[$eco].failures = $f |
           .circuit_breaker[$eco].last_failure = $now |
           .circuit_breaker[$eco].opens_at = $opens' \
          "$CONTROL_FILE" > "${CONTROL_FILE}.tmp" && mv "${CONTROL_FILE}.tmp" "$CONTROL_FILE"
        echo "::warning::Circuit breaker reopened for $ECOSYSTEM (half-open probe failed)"
        ;;
      open)
        echo "::warning::update_breaker called for $ECOSYSTEM in open state (unexpected)"
        ;;
    esac
    ;;
  *)
    echo "Error: outcome must be 'success' or 'failure', got '$OUTCOME'" >&2
    exit 1
    ;;
esac
