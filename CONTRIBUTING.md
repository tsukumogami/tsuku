# Contributing to tsuku

Thank you for your interest in contributing to tsuku! This document provides guidelines and workflows for development.

## Pre-PR Checklist

Before creating a Pull Request, please run the following checks:

### 1. Run Linter

The project uses golangci-lint for code quality checks:

```bash
# Quick check (catches most issues)
go vet ./...

# Full lint (if golangci-lint is installed)
golangci-lint run --timeout=5m ./...
```

### 2. Run Tests

Ensure all tests pass:

```bash
go test ./...
```

For coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 3. Build Successfully

Verify the code compiles:

```bash
go build -o tsuku ./cmd/tsuku
```

## CI/CD Checks

The project has GitHub Actions workflows that run automatically on PRs:

1. **Lint** - Runs golangci-lint (must pass before merge)
2. **Test** - Runs the full test suite (must pass before merge)

Monitor PR status with:
```bash
gh pr checks --watch
```

## Testing Requirements

- Add tests for new functionality
- Maintain or improve code coverage
- Ensure existing tests continue to pass
- Test edge cases and error conditions

## Commit Standards

- Write clear, descriptive commit messages
- Reference issue numbers when applicable (e.g., "Fix version parsing (#123)")
- Include context about "why" not just "what"

## Common Issues

### Linter Failures

If golangci-lint fails in CI:
- Check the CI logs for specific issues
- Update golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

### Test Failures

If tests fail:
- Check test logs for specific failures
- Use Docker for isolated testing: `./test-in-docker.sh <recipe>`
- Check for race conditions: `go test -race ./...`

## Development Resources

- [README.md](README.md) - Project overview and usage
- [ROADMAP.md](ROADMAP.md) - Development roadmap and planned features

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones
