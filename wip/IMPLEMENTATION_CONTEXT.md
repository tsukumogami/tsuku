---
summary:
  constraints:
    - Reads data/failures/*.json files (failure record schema)
    - Exit 0 on matches, 1 on no matches, 2 on error
    - Must support --blocked-by, --ecosystem, --environment, --data-dir, --help flags
  integration_points:
    - data/failures/*.json (input)
    - data/schemas/failure-record.schema.json (defines input format)
  risks:
    - None significant; read-only script
  approach_notes: |
    Bash + jq to parse failure records, filter by blocked_by field,
    optionally filter by ecosystem/environment, output matching packages.
---

# Implementation Context: Issue #1203

Gap analysis script that queries failure data to find packages blocked by
specific missing dependencies. Enables operators to prioritize which
dependencies to add to the registry.
