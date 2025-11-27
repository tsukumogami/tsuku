# tsuku

Public CLI for the tsuku package manager. This is the user-facing repository.

## Quick Reference

```bash
# Build
go build -o tsuku ./cmd/tsuku

# Test
go test ./...

# Install locally
go install ./cmd/tsuku

# Lint
golangci-lint run
```

## Structure

```
tsuku/
├── cmd/tsuku/           # CLI entry point
├── internal/
│   ├── actions/         # Installation actions
│   ├── executor/        # Recipe execution
│   ├── install/         # Installation manager
│   ├── recipe/          # Recipe parsing
│   └── version/         # Version resolution
└── bundled/             # Embedded recipes (fallback)
```

## Commands

| Command | Description |
|---------|-------------|
| `tsuku install <tool>` | Install a tool |
| `tsuku remove <tool>` | Remove a tool |
| `tsuku list` | List installed tools |
| `tsuku update <tool>` | Update to latest version |
| `tsuku recipes` | List available recipes |
| `tsuku update-registry` | Refresh recipe cache |

## Release Process

1. Update version in `cmd/tsuku/main.go`
2. Tag: `git tag -a v0.x.0 -m "Release v0.x.0"`
3. Push tag: `git push origin v0.x.0`
4. GitHub Actions builds and publishes release

## Contributing

See CONTRIBUTING.md for:
- Development setup
- How to add recipes (via tsuku-registry repo)
- PR process
- Code style guidelines
