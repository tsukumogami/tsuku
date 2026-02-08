#!/usr/bin/env bash
# Add a package to the batch generation priority queue
#
# Usage: ./scripts/add-to-queue.sh <ecosystem> <package-name> <tier>
#
# Arguments:
#   ecosystem: homebrew, npm, pypi, etc.
#   package-name: Name of the package (e.g., ripgrep, gh, jq)
#   tier: Priority tier (1=critical, 2=popular, 3=standard)
#
# Example:
#   ./scripts/add-to-queue.sh homebrew ripgrep 2

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

error() {
  echo -e "${RED}Error: $1${NC}" >&2
  exit 1
}

info() {
  echo -e "${GREEN}$1${NC}"
}

warn() {
  echo -e "${YELLOW}Warning: $1${NC}"
}

# Check arguments
if [ $# -ne 3 ]; then
  error "Usage: $0 <ecosystem> <package-name> <tier>

Arguments:
  ecosystem    Package ecosystem (homebrew, npm, pypi, rubygems, crates)
  package-name Name of the package
  tier         Priority tier (1=critical, 2=popular, 3=standard)

Example:
  $0 homebrew ripgrep 2"
fi

ECOSYSTEM="$1"
PACKAGE_NAME="$2"
TIER="$3"

# Validate ecosystem
VALID_ECOSYSTEMS="homebrew npm pypi rubygems crates"
if ! echo "$VALID_ECOSYSTEMS" | grep -qw "$ECOSYSTEM"; then
  error "Invalid ecosystem: $ECOSYSTEM
Valid options: $VALID_ECOSYSTEMS"
fi

# Validate tier
if ! [[ "$TIER" =~ ^[1-3]$ ]]; then
  error "Invalid tier: $TIER
Valid options: 1 (critical), 2 (popular), 3 (standard)"
fi

# Validate package name (alphanumeric, hyphens, underscores)
if ! [[ "$PACKAGE_NAME" =~ ^[a-zA-Z0-9_-]+$ ]]; then
  error "Invalid package name: $PACKAGE_NAME
Package names must contain only letters, numbers, hyphens, and underscores"
fi

# Find repository root
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
QUEUE_FILE="$REPO_ROOT/data/queues/priority-queue-$ECOSYSTEM.json"

# Check if jq is available
if ! command -v jq &> /dev/null; then
  error "jq is required but not installed. Install with: brew install jq"
fi

# Create queue file if it doesn't exist
if [ ! -f "$QUEUE_FILE" ]; then
  warn "Queue file not found, creating: $QUEUE_FILE"
  mkdir -p "$(dirname "$QUEUE_FILE")"
  cat > "$QUEUE_FILE" <<EOF
{
  "schema_version": 1,
  "updated_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "tiers": {
    "1": "Critical - manually curated high-impact tools",
    "2": "Popular - >10K weekly downloads (>40K/30d)",
    "3": "Standard - all other packages"
  },
  "packages": []
}
EOF
  info "Created new queue file: $QUEUE_FILE"
fi

# Check if package already exists in queue
PACKAGE_ID="${ECOSYSTEM}:${PACKAGE_NAME}"
if jq -e --arg id "$PACKAGE_ID" '.packages[] | select(.id == $id)' "$QUEUE_FILE" > /dev/null; then
  EXISTING_STATUS=$(jq -r --arg id "$PACKAGE_ID" '.packages[] | select(.id == $id) | .status' "$QUEUE_FILE")
  EXISTING_TIER=$(jq -r --arg id "$PACKAGE_ID" '.packages[] | select(.id == $id) | .tier' "$QUEUE_FILE")

  if [ "$EXISTING_STATUS" = "pending" ]; then
    if [ "$EXISTING_TIER" = "$TIER" ]; then
      warn "Package already in queue with same tier: $PACKAGE_ID (tier $TIER, status: $EXISTING_STATUS)"
      echo "No changes made."
      exit 0
    else
      warn "Package already in queue with different tier ($EXISTING_TIER). Updating to tier $TIER..."
      # Update tier
      jq --arg id "$PACKAGE_ID" --arg tier "$TIER" \
        '(.packages[] | select(.id == $id) | .tier) = ($tier | tonumber) |
         .updated_at = (now | strftime("%Y-%m-%dT%H:%M:%SZ"))' \
        "$QUEUE_FILE" > "${QUEUE_FILE}.tmp"
      mv "${QUEUE_FILE}.tmp" "$QUEUE_FILE"
      info "Updated $PACKAGE_ID to tier $TIER"
      exit 0
    fi
  else
    warn "Package exists with status: $EXISTING_STATUS. Re-adding as pending..."
    # Update status to pending and update tier
    jq --arg id "$PACKAGE_ID" --arg tier "$TIER" \
      '(.packages[] | select(.id == $id) | .status) = "pending" |
       (.packages[] | select(.id == $id) | .tier) = ($tier | tonumber) |
       (.packages[] | select(.id == $id) | .added_at) = (now | strftime("%Y-%m-%dT%H:%M:%SZ")) |
       .updated_at = (now | strftime("%Y-%m-%dT%H:%M:%SZ"))' \
      "$QUEUE_FILE" > "${QUEUE_FILE}.tmp"
    mv "${QUEUE_FILE}.tmp" "$QUEUE_FILE"
    info "Re-added $PACKAGE_ID as pending (tier $TIER)"
    exit 0
  fi
fi

# Add new package
jq --arg id "$PACKAGE_ID" \
   --arg ecosystem "$ECOSYSTEM" \
   --arg name "$PACKAGE_NAME" \
   --arg tier "$TIER" \
   '.packages += [{
     "id": $id,
     "source": $ecosystem,
     "name": $name,
     "tier": ($tier | tonumber),
     "status": "pending",
     "added_at": (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
   }] | .updated_at = (now | strftime("%Y-%m-%dT%H:%M:%SZ"))' \
   "$QUEUE_FILE" > "${QUEUE_FILE}.tmp"

mv "${QUEUE_FILE}.tmp" "$QUEUE_FILE"

info "Added $PACKAGE_ID to queue (tier $TIER)"
echo ""
echo "Next steps:"
echo "  1. Commit the queue file:"
echo "     git add $QUEUE_FILE"
echo "     git commit -m \"feat(batch): add $PACKAGE_NAME to $ECOSYSTEM queue\""
echo "     git push"
echo ""
echo "  2. Trigger a batch run:"
echo "     gh workflow run batch-generate.yml -f ecosystem=$ECOSYSTEM -f tier=$TIER -f batch_size=5"
