# Issue 1202 Summary

## What Was Implemented

Bash script that populates data/priority-queue.json by fetching Homebrew analytics data and assigning tiers based on download counts and a curated list.

## Changes Made
- `scripts/seed-queue.sh`: New script with curl+jq, retry/backoff, tier assignment
- `docs/designs/DESIGN-priority-queue.md`: Strikethrough #1202, update diagram class
- `docs/designs/DESIGN-registry-scale-strategy.md`: Same updates

## Key Decisions
- Used 30-day analytics API with 40K threshold (equivalent to >10K/week)
- Tier 1 list includes ~35 curated tools across categories (CLI, build, languages, infra)
- Only fetches analytics (not full formula.json) since analytics already has formula names ranked

## Trade-offs Accepted
- Tier 1 list is hardcoded; will need manual updates as tool priorities change
- No metadata field in output (formula/tap info) to keep output minimal

## Known Limitations
- Only homebrew source supported
- Tier 1 list is static; no external config file
