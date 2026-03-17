# Distributed Recipe Authoring Guide

This guide is for tool authors and organizations who want to host tsuku recipes in their own GitHub repositories.

## Setup

Create a `.tsuku-recipes/` directory at the root of your GitHub repository. That's it. No manifest file, no registration step, no special configuration.

```
my-repo/
├── .tsuku-recipes/
│   ├── my-tool.toml
│   └── my-other-tool.toml
├── src/
│   └── ...
└── README.md
```

Users can install your recipes with:

```bash
tsuku install your-org/my-repo:my-tool
```

## Recipe Format

Distributed recipes use exactly the same TOML format as the central registry. Everything that works in a central registry recipe works in a distributed one: actions, version providers, dependencies, verification, platform filtering.

Here's a minimal example:

```toml
[metadata]
name = "my-tool"
description = "A useful development tool"
homepage = "https://github.com/your-org/my-tool"

[version_provider]
type = "github"
owner = "your-org"
repo = "my-tool"

[[steps]]
action = "download"
url = "https://github.com/your-org/my-tool/releases/download/v{version}/my-tool-{os}-{arch}.tar.gz"

[[steps]]
action = "extract"

[[steps]]
action = "install_binaries"
binaries = ["my-tool"]

[verification]
command = "my-tool --version"
pattern = "my-tool {version}"
version_format = "strip_v"
```

See the [Actions and Primitives Guide](GUIDE-actions-and-primitives.md) for the full list of available actions and fields.

## Naming Conventions

Recipe filenames should use kebab-case and match the recipe's `metadata.name` field:

| Filename | `metadata.name` |
|----------|-----------------|
| `deploy-cli.toml` | `deploy-cli` |
| `my-tool.toml` | `my-tool` |

Mismatches between filename and `metadata.name` will confuse users. tsuku uses the filename for discovery and the `metadata.name` for installation state.

## Single-Recipe Repositories

If your repository contains just one recipe, users can skip the `:recipe` qualifier:

```bash
# These are equivalent when the repo has one recipe
tsuku install your-org/my-tool:my-tool
tsuku install your-org/my-tool
```

This works well for tool authors who ship a recipe alongside their source code.

## Multiple Recipes

A single repository can host any number of recipes. This makes sense for organizations with several internal tools:

```
.tsuku-recipes/
├── deploy-cli.toml
├── config-validator.toml
└── log-viewer.toml
```

Users install specific tools by name:

```bash
tsuku install your-org/internal-tools:deploy-cli
tsuku install your-org/internal-tools:config-validator
```

## How Caching Works

When a user runs `tsuku install` or `tsuku update-registry`, tsuku fetches your `.tsuku-recipes/` directory contents and caches them locally. Subsequent operations use the cache until the user refreshes it.

Cache refresh happens:
- Manually via `tsuku update-registry`
- Automatically when the cache TTL expires (configurable via `TSUKU_RECIPE_CACHE_TTL`)
- On `tsuku install` if the cache is stale

You don't need to do anything special to support caching. Just keep your `.tsuku-recipes/` directory up to date, and users will pick up changes on their next refresh.

## Testing Your Recipes

Before publishing, test your recipes locally:

```bash
# Validate the recipe file
tsuku validate .tsuku-recipes/my-tool.toml

# Strict validation checks verification best practices
tsuku validate --strict .tsuku-recipes/my-tool.toml

# Test installation in a sandbox
tsuku install --recipe .tsuku-recipes/my-tool.toml --sandbox
```

Sandbox testing runs the installation in an isolated container. See the README's sandbox testing section for details.

## Telling Users About Your Recipes

Add installation instructions to your project's README:

```markdown
## Install with tsuku

```bash
tsuku install your-org/my-tool
```

Don't have tsuku? Install it first:

```bash
curl -fsSL https://get.tsuku.dev/now | bash
```
```

If users install from your repo frequently, suggest they add it as a trusted registry:

```bash
tsuku registry add your-org/my-tool
```

This skips the first-install confirmation prompt on future installs.
