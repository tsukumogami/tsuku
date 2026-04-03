# tsuku

Public monorepo for the tsuku package manager. Contains the CLI, recipe registry, marketing website, and telemetry service.

## Monorepo Structure

```
tsuku/
├── cmd/tsuku/           # CLI entry point
├── internal/            # CLI internal packages
├── recipes/             # Recipe registry
├── website/             # Marketing site (tsuku.dev)
├── telemetry/           # Telemetry worker
├── testdata/            # Test fixtures
└── .github/workflows/   # CI/CD pipelines
```

## Components

| Component | Description | Tech Stack |
|-----------|-------------|------------|
| CLI (root) | Package manager binary | Go |
| recipes/ | TOML recipe definitions | TOML, validation CI |
| website/ | tsuku.dev marketing site | Static HTML, Cloudflare Pages |
| telemetry/ | Usage analytics worker | TypeScript, Cloudflare Workers |

## Build, Test, Lint

```bash
# Build
go build -o tsuku ./cmd/tsuku

# Test
go test ./...

# Install locally
go install ./cmd/tsuku

# Lint
go vet ./...
golangci-lint run --timeout=5m ./...
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `tsuku install <tool>` | Install a tool |
| `tsuku remove <tool>` | Remove a tool |
| `tsuku list` | List installed tools |
| `tsuku update <tool>` | Update tool to latest version |
| `tsuku recipes` | List available recipes |
| `tsuku search <query>` | Search for tools |
| `tsuku info <tool>` | Get information about a tool |
| `tsuku versions <tool>` | List available versions for a tool |
| `tsuku verify <tool>` | Verify tool installation |
| `tsuku outdated` | Check for outdated tools |
| `tsuku update-registry` | Refresh recipe cache |

## Key Internal Packages

| Package | Description |
|---------|-------------|
| actions/ | Action executors: build systems, compilers, package managers, archives, patching, binary install |
| autoinstall/ | Install-then-exec flow for `tsuku run`; consent mode, binary index lookup |
| config/ | Core configuration management (`$TSUKU_HOME` paths) |
| containerimages/ | Linux family to container image mapping (embedded at build time) |
| distributed/ | Distributed recipe discovery, GitHub API, caching, registry management |
| executor/ | Plan generation, step resolution, dependency expansion, plan execution |
| hook/ | Shell hook install/uninstall/status for bash/zsh/fish |
| index/ | Binary-to-recipe reverse lookup via SQLite (binary-index.db) |
| install/ | Tool installation orchestration, state management, version pinning |
| notices/ | User notification system (update availability, failures) |
| platform/ | OS/arch/libc detection, target resolution |
| project/ | `.tsuku.toml` parsing, parent directory walk, tool requirements |
| recipe/ | Recipe TOML types, loader, validator, embedded recipes |
| registry/ | Recipe registry caching, update, provider chain |
| sandbox/ | Containerized installation testing, family mapping |
| search/ | Tool and recipe search |
| secrets/ | API key resolution via env vars or config.toml `[secrets]` |
| shellenv/ | Per-directory PATH activation, init cache, doctor checks |
| telemetry/ | Usage analytics events and client |
| updates/ | Background update checks, auto-apply, self-update, throttle, GC |
| userconfig/ | User config.toml management (`tsuku config` command) |
| validate/ | Recipe validation, pre-download, golden file support |
| verify/ | Tool verification, library integrity, soname extraction |
| version/ | Version resolution, providers, factory, pin semantics |

## Development

### Docker Development (Recommended)

```bash
# Start interactive development container
./docker-dev.sh shell

# Inside container:
go build -o tsuku ./cmd/tsuku
./tsuku install serve
```

### Integration Tests

```bash
# Build tsuku first
go build -o tsuku ./cmd/tsuku

# Run integration test
./tsuku install gh
gh --version
```

## Release Process

Releases are automated via GitHub Actions using GoReleaser:

1. Push a version tag: `git tag -a v0.1.0 -m "Release v0.1.0"`
2. Push tag to remote: `git push origin v0.1.0`
3. GitHub Actions builds binaries and creates the release

Pre-releases: Tags with hyphens (e.g., `v1.0.0-rc.1`) are marked as pre-releases.

## Conventions

- All Go code must pass `gofmt` formatting
- Linting uses `golangci-lint` (see `.golangci.yaml`)
- CI runs tests and linting on every PR
- Component-specific context is in subdirectory CLAUDE.local.md files

### Use `$TSUKU_HOME` in documentation

When referring to the tsuku installation directory in code comments or documentation, use `$TSUKU_HOME` rather than the literal `~/.tsuku`. While `~/.tsuku` is the default, users can customize the location via the `$TSUKU_HOME` environment variable. Using the variable name keeps documentation accurate for all configurations.

This applies to code comments, design documents, README and other documentation, and error messages that reference paths.

## Plugin Maintenance

Skills in `plugins/` guide agents through recipe authoring, testing, and end-user workflows. They drift silently when tsuku internals change without a corresponding skill update.

| Skill | Path | Scope |
|-------|------|-------|
| recipe-author | plugins/tsuku-recipes/skills/recipe-author/ | Recipe TOML writing |
| recipe-test | plugins/tsuku-recipes/skills/recipe-test/ | Recipe testing workflow |
| tsuku-user | plugins/tsuku-user/skills/tsuku-user/ | CLI usage, project config, shell integration, updates |

**After completing any source change in the areas below, assess the relevant skills:**

1. **Broken contracts** -- read the diff and each affected skill's SKILL.md plus reference files: does anything documented no longer match the code?
2. **New surface** -- does this change add behavior that no skill mentions? If so, update the relevant skill in the same PR.

| Source Area | What to check | Relevant Skill |
|-------------|---------------|----------------|
| internal/actions/ -- action names, params, `Dependencies()` | New or renamed actions, changed parameters | recipe-author |
| internal/version/ -- provider types, source values | New version providers, changed resolution logic | recipe-author |
| internal/recipe/ -- TOML structure, when clauses, validation | Changed recipe fields, new clause types | recipe-author |
| internal/executor/ -- plan generation, decomposition | Changed step ordering, new decomposition rules | recipe-test |
| cmd/tsuku/validate.go -- validation rules, exit codes | New validation checks, changed exit semantics | recipe-test |
| cmd/tsuku/ -- CLI commands, flags, exit codes | New commands, changed flags or output format | tsuku-user |
| internal/project/ -- .tsuku.toml parsing, pin resolution | Changed config fields, new pin levels | tsuku-user |
| internal/shellenv/ -- shell integration, doctor checks | Changed PATH setup, new doctor diagnostics | tsuku-user |
| internal/userconfig/ -- config.toml structure | New settings, changed defaults | tsuku-user |
| internal/updates/ -- background checks, auto-apply, self-update | Changed update behavior, new env overrides | tsuku-user |
