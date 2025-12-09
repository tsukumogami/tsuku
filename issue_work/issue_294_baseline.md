# Issue 294 Baseline

## Environment
- Date: 2025-12-09
- Branch: feature/294-multi-version-state-schema
- Base commit: 72d46ee28ab383ad2cebda639d0a570965250576

## Test Results
- Total: 18 packages tested
- Passed: 17 packages
- Failed: 1 package (pre-existing, unrelated)

## Build Status
Pass - `go build ./...` completes without errors

## Pre-existing Issues

### TestGovulncheck failure
The `TestGovulncheck` test in the root package fails due to Go stdlib vulnerabilities (GO-2025-4175, GO-2025-4155) in crypto/x509. This requires updating Go from 1.25.3 to 1.25.5 and is unrelated to issue #294.

All other tests pass successfully.
