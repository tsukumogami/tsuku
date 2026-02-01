---
summary:
  constraints:
    - Reuse existing HTTP client and error patterns from internal/registry
    - Discovery registry URL follows same base URL pattern as recipes
    - File goes to $TSUKU_HOME/registry/discovery.json (flat, no letter subdirs)
    - Simple overwrite, no TTL/metadata sidecar needed
  integration_points:
    - cmd/tsuku/update_registry.go — add discovery fetch call alongside recipe refresh
    - internal/registry/registry.go — add FetchDiscoveryRegistry method reusing client
    - internal/discover/registry.go — already has LoadRegistry/ParseRegistry
  risks:
    - Discovery registry URL must match where the file actually lives in the repo
    - Must not break existing recipe refresh flow
  approach_notes: |
    Add a FetchDiscoveryRegistry method to Registry that fetches
    {BaseURL}/recipes/discovery.json and writes it to CacheDir/discovery.json.
    Call it from the update-registry command alongside the recipe refresh.
    The discover package already loads and parses this file.
---

# Implementation Context: Issue #1312

**Source**: docs/designs/DESIGN-discovery-resolver.md

## Key Facts

- `recipes/discovery.json` already exists in the repo (seed with 1 entry: jq)
- `internal/discover/registry.go` already has `LoadRegistry` and `ParseRegistry`
- `internal/registry/registry.go` has `DefaultRegistryURL = "https://raw.githubusercontent.com/tsukumogami/tsuku/main"`
- The URL for discovery.json will be `{BaseURL}/recipes/discovery.json`
- Functional tests already seed discovery.json into the test home (suite_test.go:84)
- `Registry.CacheDir` is `$TSUKU_HOME/registry/`
