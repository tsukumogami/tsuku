---
summary:
  constraints:
    - Must reuse the create pipeline forwarding from #1337 (set package-level vars, call runCreate)
    - Discovery only runs when no recipe exists AND no --from flag is set
    - runDiscovery() already exists in create.go — need to call it from install.go
    - --deterministic-only and --yes flags must be forwarded to both discovery and create
  integration_points:
    - cmd/tsuku/install.go - add fallback path after recipe-not-found error
    - cmd/tsuku/create.go - runDiscovery() function (lines ~659-688)
    - internal/discover/ - ChainResolver, RegistryLookup, DiscoveryResult types
    - internal/registry/ - RegistryError with ErrTypeNotFound for detecting "recipe not found"
  risks:
    - runDiscovery() is in create.go, not exported — need to either call it directly (same package) or extract
    - The DiscoveryResult.Builder + Source must map correctly to createFrom syntax (builder:source)
    - Error classification in classifyInstallError needs to handle discovery-specific errors
  approach_notes: |
    In install.go's normal install path, catch the "recipe not found" error from
    runInstallWithTelemetry. When that happens (and --from is not set), call
    runDiscovery(toolName) from create.go. If discovery succeeds, set createFrom
    to "builder:source" from the result, then forward to the create+install pipeline
    (same mechanism as --from). If discovery fails, show the NotFoundError message
    which already has actionable guidance.
---
