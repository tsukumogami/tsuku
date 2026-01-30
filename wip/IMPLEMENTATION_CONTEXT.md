---
summary:
  constraints:
    - Output must conform to data/schemas/priority-queue.schema.json (schema_version 1)
    - Each package needs id (source:name), source, name, tier (1-3), status, added_at
    - Tier 1 is hardcoded curation list, tier 2 is >10K weekly downloads, tier 3 is everything else
    - No additionalProperties allowed in output JSON
  integration_points:
    - data/priority-queue.json (output file)
    - data/schemas/priority-queue.schema.json (output format)
    - Homebrew API (formula list) and analytics API (install-on-request/30d.json)
  risks:
    - Homebrew analytics API format may change or be rate-limited
    - Large API responses may be slow to process in bash
    - id pattern must match ^[a-z0-9_-]+:[a-z0-9_@.+-]+$
  approach_notes: |
    Bash script using curl + jq. Fetch formula names from Homebrew API,
    fetch analytics for download counts, merge and assign tiers, output
    conformant JSON. Handle rate limits with retry/backoff.
---

# Implementation Context: Issue #1202

Queue seed script for Homebrew. Populates data/priority-queue.json by fetching
formula metadata and analytics from Homebrew's public API, assigning tiers
based on download counts.
