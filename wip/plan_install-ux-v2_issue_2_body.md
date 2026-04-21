---
complexity: testable
complexity_rationale: Reclassifying ~20 fmt.Printf/printInfof calls changes non-TTY CI output shape — needs test coverage to confirm correct Log/Status split and that no lines are accidentally silenced
---

## Goal

Replace all remaining `printInfof`/`fmt.Printf` output calls in `install_deps.go` and `install_lib.go` with Reporter calls using the Decision 2 classification: `reporter.Log()` for install-start ("Installing X@Y...") and install-done ("X@Y installed"), `reporter.Status()` for intermediate orchestration labels (dep-checking, plan generation, already-installed notices), and `reporter.DeferWarn()` for PATH guidance. Remove emoji completion lines (`📍 Installed to:`, `🔗 Wrapped N binaries:`).

## Acceptance Criteria

- `install_deps.go` and `install_lib.go` contain no direct `printInfof` or `fmt.Printf` calls for progress output (warnings to stderr via `fmt.Fprintf(os.Stderr, ...)` are acceptable)
- "Installing X@Y..." line uses `reporter.Log()`
- "X@Y installed" line uses `reporter.Log()`
- "Verifying X@Y" line (already reporter.Log from prior work) remains as-is
- Intermediate labels (dep-checking, plan generation, already-installed) use `reporter.Status()`
- PATH guidance ("To use the installed tool...") uses `reporter.DeferWarn()`
- Emoji `📍` / `🔗` completion lines are removed
- `go test ./...` passes with no new failures

## Dependencies

<<ISSUE:1>>
