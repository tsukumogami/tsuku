#!/bin/bash
# Verify all GitHub Actions are pinned to commit SHAs

set -euo pipefail

workflows=(.github/workflows/*.yml)
failed=0

for workflow in "${workflows[@]}"; do
    echo "Checking $workflow..."

    # Find all 'uses:' lines and extract the reference
    while IFS= read -r line; do
        # Extract the action reference (after 'uses:')
        ref=$(echo "$line" | sed -n 's/.*uses: *\([^ ]*\).*/\1/p')

        if [[ -n "$ref" && "$ref" != ./* ]]; then
            # Allow dtolnay/rust-toolchain to use branches (stable, master)
            # This action is designed to track rust versions dynamically
            if [[ "$ref" =~ ^dtolnay/rust-toolchain@(stable|master)$ ]]; then
                continue
            fi

            # Check if it's a commit SHA (40 hex chars after @)
            if ! echo "$ref" | grep -qE '@[0-9a-f]{40}'; then
                echo "  ❌ Not pinned to SHA: $ref"
                failed=1
            fi
        fi
    done < <(grep "uses:" "$workflow")
done

if [[ $failed -eq 0 ]]; then
    echo "✅ All actions are pinned to commit SHAs"
    exit 0
else
    echo "❌ Some actions are not pinned to SHAs"
    exit 1
fi
