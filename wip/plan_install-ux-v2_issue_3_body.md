---
complexity: simple
complexity_rationale: One-line change at a single call site; the flag already exists and the error path is unchanged
---

## Goal

Change the `RunToolVerification` call in `install_deps.go` from `Verbose: true` to `Verbose: false`. This suppresses the 8–12 sub-step `printInfo` lines that verify.go emits per tool during the post-install check. The `tsuku verify` command continues to default to `Verbose: true` — only the post-install path changes.

## Acceptance Criteria

- `install_deps.go` passes `Verbose: false` to `RunToolVerification` (was `Verbose: true`)
- `tsuku verify <tool>` still prints full sub-step output (its call site is unchanged)
- `go test ./...` passes with no new failures

## Dependencies

<<ISSUE:1>>
