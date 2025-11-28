# Issue 6 Implementation Plan

## Summary

Create Go-based integration tests with `//go:build integration` tag that run tsuku inside a Docker container, leveraging the existing test-matrix.json and Dockerfile.

## Approach

Create a new `integration_test.go` file that:
1. Uses `//go:build integration` build tag
2. Reads test cases from `test-matrix.json`
3. Builds a Docker image from the existing Dockerfile (with Go added)
4. Runs each test case inside a container
5. Reports pass/fail for each tool installation

### Key Design Decisions

1. **Modify Dockerfile**: Add Go to the test image so we can build tsuku inside the container
2. **Test structure**: One test function that iterates through test-matrix.json
3. **Docker execution**: Use `docker build` and `docker run` via exec.Command
4. **Parallelization**: Run tests in parallel using t.Parallel() and subtests

### Alternatives Considered
- **testcontainers-go**: More complex, adds dependency - not chosen
- **Direct host execution**: Doesn't match CI environment - not chosen
- **Separate test binary**: More complex to maintain - not chosen

## Files to Modify
- `Dockerfile` - Add Go installation for building tsuku inside container

## Files to Create
- `integration_test.go` - Main integration test file with build tag

## Implementation Steps
- [x] Create Dockerfile.integration (separate from Vagrant Dockerfile)
- [x] Create integration_test.go with build tag
- [x] Parse test-matrix.json in test
- [x] Implement Docker build and run logic
- [x] Add individual subtests for each tool
- [x] Test locally with `go test -tags=integration -v ./...`
- [x] Verify unit tests still work without the tag

## Testing Strategy
- Run `go test ./...` to verify unit tests unaffected
- Run `go test -tags=integration -v ./...` to run integration tests
- Test at least one tool installation manually in Docker

## Success Criteria
- [ ] `go test ./...` runs only unit tests (fast)
- [ ] `go test -tags=integration ./...` runs integration tests in Docker
- [ ] Integration tests use same test-matrix.json as CI
- [ ] Tests can be run locally with Docker installed

## Risks and Mitigations
- **Docker not installed**: Test will skip with clear message
- **Slow tests**: Use t.Parallel() and allow filtering by tool name
- **Network issues**: Tests may be flaky - accept as inherent to integration tests

## Open Questions
None - requirements are clear.
