# Issue 895 Baseline

## Environment
- Date: 2026-01-14
- Branch: fix/895-cargo-builder-hyperfine-path
- Base commit: 5dcca5ccf14effb46c4ae19c8fda3f58f826ad27

## Test Results
- Total: 27 packages
- Passed: 20
- Failed: 7 (pre-existing, unrelated to this issue)

### Pre-existing Failures
- `internal/actions`: Download cache tests (environment-specific failures)
- `internal/sandbox`: Container/Docker integration tests (requires Docker setup)
- `internal/validate`: External API dependency (404 from GitHub)

## Build Status
Pass - `go build -o tsuku ./cmd/tsuku` succeeds without warnings

## Pre-existing Issues
The failing tests are environment-specific (Docker integration, external API dependencies) and not related to the CI workflow fix being implemented. These failures exist on main and are not introduced by this branch.
