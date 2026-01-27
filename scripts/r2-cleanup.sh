#!/usr/bin/env bash
# R2 Cleanup Script
#
# Orchestrates R2 golden file cleanup: orphan detection, version retention,
# soft delete (quarantine), and hard delete.
#
# Usage:
#   ./scripts/r2-cleanup.sh [options]
#
# Options:
#   --dry-run              Report actions without performing them (default)
#   --execute              Actually perform cleanup actions
#   --max-deletions N      Maximum files to quarantine per run (default: 100)
#   --hard-delete          Remove quarantined files older than 7 days
#   --skip-retention       Skip version retention check (orphans only)
#   --skip-orphans         Skip orphan detection (retention only)
#   --json                 Output JSON format
#   -h, --help             Show this help message
#
# Environment Variables:
#   R2_BUCKET_URL          - Required. R2 bucket endpoint URL
#   R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
#   R2_ACCESS_KEY_ID       - Required. R2 access key ID (write access for --execute)
#   R2_SECRET_ACCESS_KEY   - Required. R2 secret access key
#
# Workflow:
#   1. Run R2 health check to verify availability
#   2. Run orphan detection (deleted recipes)
#   3. Run retention check (excess versions)
#   4. Soft delete: copy files to quarantine/{date}/ prefix, then delete original
#   5. Hard delete: remove quarantine items older than 7 days
#
# Quarantine Path Convention:
#   plans/a/ack/v3.8.0/linux-amd64.json
#   → quarantine/2026-01-24/plans/a/ack/v3.8.0/linux-amd64.json
#
# Exit Codes:
#   0 - Success
#   1 - Error (missing credentials, API failure)
#   2 - Invalid arguments

set -euo pipefail

BUCKET_NAME="${R2_BUCKET_NAME:-tsuku-golden-registry}"
DRY_RUN=true
MAX_DELETIONS=100
HARD_DELETE=false
SKIP_RETENTION=false
SKIP_ORPHANS=false
JSON_OUTPUT=false
QUARANTINE_DAYS=7

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --execute)
            DRY_RUN=false
            shift
            ;;
        --max-deletions)
            MAX_DELETIONS="$2"
            shift 2
            ;;
        --hard-delete)
            HARD_DELETE=true
            shift
            ;;
        --skip-retention)
            SKIP_RETENTION=true
            shift
            ;;
        --skip-orphans)
            SKIP_ORPHANS=true
            shift
            ;;
        --json)
            JSON_OUTPUT=true
            shift
            ;;
        -h|--help)
            cat <<'EOF'
R2 Cleanup Script

Orchestrates R2 golden file cleanup: orphan detection, version retention,
soft delete (quarantine), and hard delete.

Usage:
  ./scripts/r2-cleanup.sh [options]

Options:
  --dry-run              Report actions without performing them (default)
  --execute              Actually perform cleanup actions
  --max-deletions N      Maximum files to quarantine per run (default: 100)
  --hard-delete          Remove quarantined files older than 7 days
  --skip-retention       Skip version retention check (orphans only)
  --skip-orphans         Skip orphan detection (retention only)
  --json                 Output JSON format
  -h, --help             Show this help message

Environment Variables:
  R2_BUCKET_URL          - Required. R2 bucket endpoint URL
  R2_BUCKET_NAME         - Optional. Bucket name (default: tsuku-golden-registry)
  R2_ACCESS_KEY_ID       - Required. R2 access key ID (write for --execute)
  R2_SECRET_ACCESS_KEY   - Required. R2 secret access key

Workflow:
  1. Health check to verify R2 availability
  2. Detect orphaned files (deleted recipes)
  3. Detect excess versions (retention policy)
  4. Soft delete to quarantine/ prefix
  5. Hard delete quarantined files older than 7 days

Examples:
  # Dry-run report
  ./scripts/r2-cleanup.sh

  # Execute cleanup with limit
  ./scripts/r2-cleanup.sh --execute --max-deletions 50

  # Only check retention, skip orphans
  ./scripts/r2-cleanup.sh --skip-orphans

  # Hard delete old quarantine items
  ./scripts/r2-cleanup.sh --execute --hard-delete
EOF
            exit 0
            ;;
        *)
            echo "Error: Unknown option: $1" >&2
            echo "Use --help for usage information" >&2
            exit 2
            ;;
    esac
done

# Validate required environment variables
if [[ -z "${R2_BUCKET_URL:-}" ]]; then
    echo "Error: R2_BUCKET_URL environment variable is required" >&2
    exit 1
fi

if [[ -z "${R2_ACCESS_KEY_ID:-}" ]]; then
    echo "Error: R2_ACCESS_KEY_ID environment variable is required" >&2
    exit 1
fi

if [[ -z "${R2_SECRET_ACCESS_KEY:-}" ]]; then
    echo "Error: R2_SECRET_ACCESS_KEY environment variable is required" >&2
    exit 1
fi

# Export AWS credentials for subcommands
export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
export AWS_ENDPOINT_URL="$R2_BUCKET_URL"

# Get script directory for calling sibling scripts
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Temporary files
ORPHANS_FILE=$(mktemp)
RETENTION_FILE=$(mktemp)
COMBINED_FILE=$(mktemp)
QUARANTINE_LIST=$(mktemp)
trap "rm -f '$ORPHANS_FILE' '$RETENTION_FILE' '$COMBINED_FILE' '$QUARANTINE_LIST'" EXIT

# Tracking variables
ORPHAN_COUNT=0
RETENTION_COUNT=0
QUARANTINED_COUNT=0
HARD_DELETED_COUNT=0

TODAY=$(date +%Y-%m-%d)

echo "=== R2 Cleanup ===" >&2
echo "Mode: $([ "$DRY_RUN" = true ] && echo 'DRY RUN' || echo 'EXECUTE')" >&2
echo "Max deletions: $MAX_DELETIONS" >&2
echo "Hard delete: $HARD_DELETE" >&2
echo "" >&2

# Step 1: Health check (skip in dry-run to allow offline testing)
if [[ "$DRY_RUN" = false ]]; then
    echo "Step 1: Health check..." >&2
    if ! "$SCRIPT_DIR/r2-health-check.sh" >&2; then
        echo "Error: R2 health check failed, aborting cleanup" >&2
        exit 1
    fi
    echo "" >&2
else
    echo "Step 1: Health check (skipped in dry-run)" >&2
fi

# Step 2: Orphan detection
if [[ "$SKIP_ORPHANS" = false ]]; then
    echo "Step 2: Orphan detection..." >&2
    if "$SCRIPT_DIR/r2-orphan-detection.sh" > "$ORPHANS_FILE" 2>&2; then
        ORPHAN_COUNT=$(wc -l < "$ORPHANS_FILE" | tr -d ' ')
        echo "Found $ORPHAN_COUNT orphaned files" >&2
    else
        echo "Warning: Orphan detection failed, continuing without orphan data" >&2
    fi
else
    echo "Step 2: Orphan detection (skipped)" >&2
fi
echo "" >&2

# Step 3: Retention check
if [[ "$SKIP_RETENTION" = false ]]; then
    echo "Step 3: Retention check..." >&2
    if "$SCRIPT_DIR/r2-retention-check.sh" > "$RETENTION_FILE" 2>&2; then
        RETENTION_COUNT=$(wc -l < "$RETENTION_FILE" | tr -d ' ')
        echo "Found $RETENTION_COUNT excess versions" >&2
    else
        echo "Warning: Retention check failed, continuing without retention data" >&2
    fi
else
    echo "Step 3: Retention check (skipped)" >&2
fi
echo "" >&2

# Step 4: Combine and deduplicate
echo "Step 4: Combining results..." >&2
cat "$ORPHANS_FILE" "$RETENTION_FILE" 2>/dev/null | sort -u > "$COMBINED_FILE"
TOTAL_TO_CLEANUP=$(wc -l < "$COMBINED_FILE" | tr -d ' ')
echo "Total unique files to cleanup: $TOTAL_TO_CLEANUP" >&2
echo "" >&2

# Step 5: Soft delete (quarantine)
if [[ "$TOTAL_TO_CLEANUP" -gt 0 ]]; then
    echo "Step 5: Soft delete (quarantine)..." >&2

    # Respect max deletions limit
    REMAINING=$MAX_DELETIONS

    while IFS= read -r object_key && [[ "$REMAINING" -gt 0 ]]; do
        [[ -z "$object_key" ]] && continue

        # Build quarantine path
        quarantine_key="quarantine/${TODAY}/${object_key}"

        if [[ "$DRY_RUN" = true ]]; then
            echo "Would quarantine: $object_key → $quarantine_key" >&2
        else
            # Copy to quarantine
            if aws s3api copy-object \
                --bucket "$BUCKET_NAME" \
                --copy-source "${BUCKET_NAME}/${object_key}" \
                --key "$quarantine_key" \
                --output text > /dev/null 2>&1; then

                # Delete original
                if aws s3api delete-object \
                    --bucket "$BUCKET_NAME" \
                    --key "$object_key" \
                    --output text > /dev/null 2>&1; then
                    echo "Quarantined: $object_key" >&2
                else
                    echo "Warning: Failed to delete original after copy: $object_key" >&2
                fi
            else
                echo "Warning: Failed to copy to quarantine: $object_key" >&2
                continue
            fi
        fi

        QUARANTINED_COUNT=$((QUARANTINED_COUNT + 1))
        REMAINING=$((REMAINING - 1))
    done < "$COMBINED_FILE"

    if [[ "$REMAINING" -eq 0 && "$TOTAL_TO_CLEANUP" -gt "$MAX_DELETIONS" ]]; then
        echo "Reached max deletions limit ($MAX_DELETIONS). Remaining: $((TOTAL_TO_CLEANUP - MAX_DELETIONS))" >&2
    fi
fi
echo "" >&2

# Step 6: Hard delete (quarantine cleanup)
if [[ "$HARD_DELETE" = true ]]; then
    echo "Step 6: Hard delete (quarantine cleanup)..." >&2

    # List quarantine items and find ones older than QUARANTINE_DAYS
    CUTOFF_DATE=$(date -d "$QUARANTINE_DAYS days ago" +%Y-%m-%d 2>/dev/null || \
                  date -v-${QUARANTINE_DAYS}d +%Y-%m-%d 2>/dev/null || \
                  echo "")

    if [[ -z "$CUTOFF_DATE" ]]; then
        echo "Warning: Could not calculate cutoff date, skipping hard delete" >&2
    else
        echo "Cutoff date: $CUTOFF_DATE (files older than this will be deleted)" >&2

        # List quarantine objects
        CONTINUATION_TOKEN=""
        while true; do
            if [[ -z "$CONTINUATION_TOKEN" ]]; then
                RESPONSE=$(aws s3api list-objects-v2 \
                    --bucket "$BUCKET_NAME" \
                    --prefix "quarantine/" \
                    --output json 2>&1) || break
            else
                RESPONSE=$(aws s3api list-objects-v2 \
                    --bucket "$BUCKET_NAME" \
                    --prefix "quarantine/" \
                    --continuation-token "$CONTINUATION_TOKEN" \
                    --output json 2>&1) || break
            fi

            # Process each quarantine item
            echo "$RESPONSE" | jq -r '.Contents[]?.Key // empty' | while read -r qkey; do
                [[ -z "$qkey" ]] && continue

                # Extract date from quarantine path: quarantine/{date}/plans/...
                if [[ "$qkey" =~ ^quarantine/([0-9]{4}-[0-9]{2}-[0-9]{2})/ ]]; then
                    qdate="${BASH_REMATCH[1]}"

                    # Compare dates (string comparison works for YYYY-MM-DD format)
                    if [[ "$qdate" < "$CUTOFF_DATE" ]]; then
                        if [[ "$DRY_RUN" = true ]]; then
                            echo "Would hard delete: $qkey (quarantined: $qdate)" >&2
                        else
                            if aws s3api delete-object \
                                --bucket "$BUCKET_NAME" \
                                --key "$qkey" \
                                --output text > /dev/null 2>&1; then
                                echo "Hard deleted: $qkey" >&2
                            else
                                echo "Warning: Failed to hard delete: $qkey" >&2
                            fi
                        fi
                        HARD_DELETED_COUNT=$((HARD_DELETED_COUNT + 1))
                    fi
                fi
            done

            # Check for more pages
            IS_TRUNCATED=$(echo "$RESPONSE" | jq -r '.IsTruncated // false')
            if [[ "$IS_TRUNCATED" == "true" ]]; then
                CONTINUATION_TOKEN=$(echo "$RESPONSE" | jq -r '.NextContinuationToken')
            else
                break
            fi
        done
    fi
fi
echo "" >&2

# Output summary
echo "=== Summary ===" >&2
echo "Orphans detected: $ORPHAN_COUNT" >&2
echo "Excess versions detected: $RETENTION_COUNT" >&2
echo "Files quarantined: $QUARANTINED_COUNT" >&2
echo "Files hard deleted: $HARD_DELETED_COUNT" >&2

if [[ "$JSON_OUTPUT" = true ]]; then
    jq -n \
        --arg mode "$([ "$DRY_RUN" = true ] && echo 'dry-run' || echo 'execute')" \
        --argjson orphan_count "$ORPHAN_COUNT" \
        --argjson retention_count "$RETENTION_COUNT" \
        --argjson quarantined_count "$QUARANTINED_COUNT" \
        --argjson hard_deleted_count "$HARD_DELETED_COUNT" \
        --argjson max_deletions "$MAX_DELETIONS" \
        --arg date "$TODAY" \
        '{
            mode: $mode,
            date: $date,
            summary: {
                orphans_detected: $orphan_count,
                excess_versions_detected: $retention_count,
                files_quarantined: $quarantined_count,
                files_hard_deleted: $hard_deleted_count,
                max_deletions: $max_deletions
            }
        }'
fi

exit 0
