# CLAUDE.md Content Catalog

## Sections to Promote from CLAUDE.local.md

All of these are universally useful and should move to committed CLAUDE.md:

1. Monorepo Structure (directory tree)
2. Component Overview (table: CLI, recipes, website, telemetry with tech stacks)
3. Quick Reference (build, test, install, lint commands)
4. Commands (table of all tsuku CLI commands)
5. Development > Docker Development (recommended setup)
6. Development > Integration Tests (standard procedure)
7. Release Process (GoReleaser automation)
8. Key Points (gofmt, golangci-lint, CI)
9. Conventions > $TSUKU_HOME Usage

## Sections to Keep in CLAUDE.local.md

Only these workspace-specific sections stay:

1. **Repo Visibility: Public** -- tsukumogami workflow config
2. **Default Scope: Tactical** -- tsukumogami planning scope
3. **QA Configuration** -- workspace-specific test binary naming, home override
4. **Environment** -- .local.env credential sourcing (personal)

## New Sections Needed

### Key Internal Packages

38 packages identified. Top ones for CLAUDE.md:

| Package | Description |
|---------|-------------|
| actions/ | Action executors: build systems, compilers, package managers, archives, patching, binary install |
| autoinstall/ | Install-then-exec flow for tsuku run; consent mode, binary index lookup |
| config/ | Core configuration management ($TSUKU_HOME paths) |
| containerimages/ | Linux family to container image mapping (embedded at build time) |
| distributed/ | Distributed recipe discovery, GitHub API, caching, registry management |
| executor/ | Plan generation, step resolution, dependency expansion, plan execution |
| hook/ | Shell hook install/uninstall/status for bash/zsh/fish |
| index/ | Binary-to-recipe reverse lookup via SQLite (binary-index.db) |
| install/ | Tool installation orchestration, state management, version pinning |
| notices/ | User notification system (update availability, failures) |
| platform/ | OS/arch/libc detection, target resolution |
| project/ | .tsuku.toml parsing, parent directory walk, tool requirements |
| recipe/ | Recipe TOML types, loader, validator, embedded recipes |
| registry/ | Recipe registry caching, update, provider chain |
| sandbox/ | Containerized installation testing, family mapping |
| search/ | Tool and recipe search |
| secrets/ | API key resolution via env vars or config.toml [secrets] |
| shellenv/ | Per-directory PATH activation, init cache, doctor checks |
| telemetry/ | Usage analytics events and client |
| updates/ | Background update checks, auto-apply, self-update, throttle, GC |
| userconfig/ | User config.toml management (tsuku config command) |
| validate/ | Recipe validation, pre-download, golden file support |
| verify/ | Tool verification, library integrity, soname extraction |
| version/ | Version resolution, providers, factory, pin semantics |

### Plugin Maintenance Protocol

(See plan_skills_plugin-infrastructure-catalog.md for draft text)

## settings.json vs settings.local.json Split

**Committed settings.json:**
- enabledPlugins: tsuku-recipes@tsuku, tsuku-user@tsuku, shirabe@shirabe
- extraKnownMarketplaces: tsuku (file source), shirabe (GitHub source)

**Local settings.local.json (gitignored):**
- enabledPlugins: tsukumogami@tsukumogami (private plugin)
- env: GH_TOKEN, other credentials
- hooks: PreToolUse, Stop (local script paths)
- permissions: defaultMode

## Summary

Promote ~90% of CLAUDE.local.md content. Add key internal packages table (~24 most important of 38) and plugin maintenance protocol section. CLAUDE.local.md shrinks to 4 workspace-specific sections.
