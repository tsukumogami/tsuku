---
summary: Register GoBuilder in the builder registry by adding SessionBuilder interface methods
key_requirements:
  - Add RequiresLLM() returning false (deterministic builder)
  - Add NewSession() wrapping Build with DeterministicSession
  - Update CanBuild signature to match SessionBuilder interface (BuildRequest instead of string)
  - Register in cmd/tsuku/create.go alongside other ecosystem builders
integration_points:
  - internal/builders/builder.go - SessionBuilder interface, DeterministicSession
  - cmd/tsuku/create.go - builder registration
  - internal/builders/go.go - add interface methods
design_constraints:
  - Follow existing patterns from CargoBuilder, NpmBuilder, etc.
  - No LLM required - uses proxy.golang.org API directly
---

## Goal

Enable the Go builder for recipe generation by registering it in the builder registry.

## Context

The Go builder (`internal/builders/go.go`) is fully implemented and deterministic but not registered in `create.go`. This is a quick win for the registry scale strategy.

## Acceptance Criteria

- [ ] Add `RequiresLLM()` method returning `false`
- [ ] Add `NewSession()` method wrapping with `DeterministicSession`
- [ ] Register in builder registry in `create.go`
- [ ] `tsuku create --from go:<module>` works

## Dependencies

None

## Implementation Notes

The GoBuilder.CanBuild currently uses `(ctx, packageName string)` but SessionBuilder interface requires `(ctx, req BuildRequest)`. Need to update the signature and use `req.Package` internally.
