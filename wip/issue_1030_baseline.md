# Issue #1030 Baseline

## Branch
`chore/1030-integration-test-job`

## Test Results
All tests pass (go test ./...)

## Current State
- release.yml has: create-draft-release, build-rust, build-rust-musl, release, finalize-release
- finalize-release depends on: create-draft-release, release, build-rust, build-rust-musl
- No integration-test job exists yet

## Task
Add integration-test job that:
1. Depends on all build jobs (release, build-rust, build-rust-musl)
2. Runs on 4 native platforms (not musl - those are alternate binaries)
3. Downloads artifacts from draft release
4. Validates version strings for tsuku and tsuku-dltest
5. finalize-release depends on integration-test instead of build jobs directly
