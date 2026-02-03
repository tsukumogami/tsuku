#!/usr/bin/env bash
set -euo pipefail

METRICS_FILE="data/metrics/batch-runs.jsonl"
LAST=10
ECOSYSTEM=""
PLATFORM=""
SINCE=""

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Display batch pipeline metrics from $METRICS_FILE.

Options:
  --last N            Show last N runs (default: 10)
  --ecosystem NAME    Filter by ecosystem (e.g., homebrew)
  --platform NAME     Show detail for one platform (e.g., linux-x86_64)
  --since DATE        Show runs since DATE (ISO 8601, e.g., 2026-01-15)
  --help              Show this help message
EOF
  exit 0
}

while [ $# -gt 0 ]; do
  case "$1" in
    --last) LAST="$2"; shift 2 ;;
    --ecosystem) ECOSYSTEM="$2"; shift 2 ;;
    --platform) PLATFORM="$2"; shift 2 ;;
    --since) SINCE="$2"; shift 2 ;;
    --help) usage ;;
    *) echo "Unknown option: $1" >&2; usage ;;
  esac
done

if [ ! -f "$METRICS_FILE" ]; then
  echo "No metrics file found at $METRICS_FILE"
  echo "Run the batch pipeline to generate metrics."
  exit 0
fi

TOTAL_RUNS=$(wc -l < "$METRICS_FILE" | tr -d ' ')
if [ "$TOTAL_RUNS" -eq 0 ]; then
  echo "No batch runs recorded yet."
  exit 0
fi

# Build jq filter
FILTER="."
if [ -n "$ECOSYSTEM" ]; then
  FILTER="$FILTER | select(.ecosystem == \"$ECOSYSTEM\")"
fi
if [ -n "$SINCE" ]; then
  FILTER="$FILTER | select(.timestamp >= \"$SINCE\")"
fi

# Apply filters, take last N
FILTERED=$(jq -c "$FILTER" "$METRICS_FILE" | tail -n "$LAST")

if [ -z "$FILTERED" ]; then
  echo "No runs match the given filters."
  exit 0
fi

COUNT=$(echo "$FILTERED" | wc -l | tr -d ' ')

if [ -n "$PLATFORM" ]; then
  # Platform detail mode
  echo "Platform Detail: $PLATFORM (last $COUNT runs)"
  echo ""
  printf "%-30s  %7s  %7s  %7s  %8s\n" "Batch ID" "Tested" "Passed" "Failed" "Rate"
  printf "%-30s  %7s  %7s  %7s  %8s\n" "------------------------------" "-------" "-------" "-------" "--------"

  echo "$FILTERED" | while IFS= read -r line; do
    BATCH_ID=$(echo "$line" | jq -r '.batch_id')
    TESTED=$(echo "$line" | jq -r ".platforms.\"$PLATFORM\".tested // 0")
    PASSED=$(echo "$line" | jq -r ".platforms.\"$PLATFORM\".passed // 0")
    FAILED=$(echo "$line" | jq -r ".platforms.\"$PLATFORM\".failed // 0")
    if [ "$TESTED" -gt 0 ]; then
      RATE=$(awk "BEGIN {printf \"%.1f%%\", ($PASSED / $TESTED) * 100}")
    else
      RATE="N/A"
    fi
    printf "%-30s  %7d  %7d  %7d  %8s\n" "$BATCH_ID" "$TESTED" "$PASSED" "$FAILED" "$RATE"
  done
else
  # Summary mode
  echo "Batch Pipeline Metrics (last $COUNT runs)"
  echo ""
  printf "%-30s  %5s  %6s  %8s  %11s  %8s  %8s\n" "Batch ID" "Total" "Merged" "Excluded" "Constrained" "Rate" "Duration"
  printf "%-30s  %5s  %6s  %8s  %11s  %8s  %8s\n" "------------------------------" "-----" "------" "--------" "-----------" "--------" "--------"

  echo "$FILTERED" | while IFS= read -r line; do
    BATCH_ID=$(echo "$line" | jq -r '.batch_id')
    TOTAL=$(echo "$line" | jq -r '.total')
    MERGED=$(echo "$line" | jq -r '.merged')
    EXCLUDED=$(echo "$line" | jq -r '.excluded')
    CONSTRAINED=$(echo "$line" | jq -r '.constrained')
    DURATION=$(echo "$line" | jq -r '.duration_seconds')
    if [ "$TOTAL" -gt 0 ]; then
      RATE=$(awk "BEGIN {printf \"%.1f%%\", ($MERGED / $TOTAL) * 100}")
    else
      RATE="N/A"
    fi
    DURATION_FMT="${DURATION}s"
    printf "%-30s  %5d  %6d  %8d  %11d  %8s  %8s\n" "$BATCH_ID" "$TOTAL" "$MERGED" "$EXCLUDED" "$CONSTRAINED" "$RATE" "$DURATION_FMT"
  done

  # Platform summary
  echo ""
  echo "Platform Averages:"
  for platform in linux-x86_64 linux-arm64 darwin-arm64 darwin-x86_64; do
    TOTALS=$(echo "$FILTERED" | jq -r ".platforms.\"$platform\".tested // 0" | paste -sd+ | bc)
    PASSES=$(echo "$FILTERED" | jq -r ".platforms.\"$platform\".passed // 0" | paste -sd+ | bc)
    if [ "$TOTALS" -gt 0 ]; then
      AVG=$(awk "BEGIN {printf \"%.1f%%\", ($PASSES / $TOTALS) * 100}")
    else
      AVG="N/A"
    fi
    printf "  %-20s avg %s pass rate (%d tested, %d passed)\n" "$platform:" "$AVG" "$TOTALS" "$PASSES"
  done
fi
