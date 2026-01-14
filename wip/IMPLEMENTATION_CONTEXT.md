## Goal

Fix cobra-cli golden file execution on darwin-arm64.

## Context

Observed in PR #858 CI run. The cobra-cli golden file execution fails on macOS darwin-arm64 runners.

cobra-cli is a Go-based CLI tool installed via `go_install` action. The specific error needs investigation from CI logs.

## Acceptance Criteria

- [ ] Investigate specific failure reason from CI logs
- [ ] Fix recipe or golden file as needed
- [ ] Golden file execution passes on darwin-arm64

## Dependencies

None
