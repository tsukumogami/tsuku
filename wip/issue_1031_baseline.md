# Issue #1031 Baseline

## Branch
`chore/1031-artifact-verification`

## Test Results
All tests pass (go test ./...)

## Current State
- finalize-release job exists, depends on integration-test
- Currently only publishes release (removes draft status)
- No artifact verification
- No checksum generation

## Task
Add to finalize-release:
1. Verify all 12 expected artifacts are present
2. Download all artifacts
3. Generate SHA256 checksums.txt
4. Upload checksums.txt to release
5. Publish release (only after verification passes)
