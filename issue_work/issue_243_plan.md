# Issue 243 Implementation Plan

## Goal
Add appropriate `runtime_dependencies` overrides to recipes that need them based on the dependency pattern design.

## Analysis Summary

### npm_install recipes (8 total)
All these are JavaScript/TypeScript tools that need Node.js at runtime:
- amplify, cdk, netlify-cli, serverless, serve, turbo, vercel, wrangler

**Decision**: These all need Node.js at runtime - no overrides needed (default behavior is correct).

Note: turbo has native binaries for performance, but the npm package still requires Node.js to work. Same for vercel which has @vercel/ncc compiled parts but runs on Node.

### pipx_install recipes (4 total)
| Recipe | Description | Needs Python at runtime? |
|--------|-------------|-------------------------|
| black | Python formatter | YES - pure Python |
| httpie | HTTP client | YES - pure Python |
| poetry | Python package manager | YES - pure Python |
| ruff | Python linter (Rust binary) | NO - compiled Rust |

**Decision**: Only `ruff` needs `runtime_dependencies = []` override.

### go_install recipes (8 total)
All these compile to standalone binaries - no runtime deps needed:
- cobra-cli, dlv, gofumpt, goimports, gopls, gore, mockgen, staticcheck

**Special case**: `gore` (Go REPL) needs Go at runtime! Currently has `dependencies = ["go"]` which should be `runtime_dependencies = ["golang"]`.

**Decision**:
- `gore` needs `runtime_dependencies = ["golang"]` (and remove `dependencies = ["go"]`)

### cargo_install recipes (3 total)
All Rust tools compile to standalone binaries:
- cargo-audit, cargo-edit, cargo-watch

**Decision**: No overrides needed - default (no runtime deps) is correct.

## Implementation Tasks

1. Add `runtime_dependencies = []` to `ruff.toml` (compiled Rust binary via pip)
2. Change `gore.toml` from `dependencies = ["go"]` to `runtime_dependencies = ["golang"]`
3. Validate all modified recipes
4. Run integration tests for affected recipes

## Validation
- `./tsuku validate --strict` on each modified recipe
- Integration tests: `ruff`, `gore` (if in test matrix)
