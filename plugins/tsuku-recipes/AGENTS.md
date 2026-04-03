# tsuku-recipes Plugin

Contextual guidance for authoring and testing tsuku recipes.

Tsuku is a package manager for developer tools. Recipes are TOML files that
define how to download, build, and verify a tool installation. They live in
the central registry (`recipes/`) or a distributed repo (`.tsuku-recipes/`).

## Recipe TOML Format

A recipe has four sections:

```toml
[metadata]
name = "my-tool"
description = "A useful tool"
homepage = "https://github.com/org/my-tool"
# Optional: dependencies, runtime_dependencies, extra_dependencies

[version_provider]
type = "github"           # or npm, pypi, crates_io, rubygems, etc.
owner = "org"
repo = "my-tool"

[[steps]]
action = "github_archive" # one action per step; see action reference below
owner = "org"
repo = "my-tool"

[[steps]]
action = "install_binaries"
binaries = ["my-tool"]

[verification]
command = "my-tool --version"
pattern = "{version}"
```

- **metadata** -- name (kebab-case), description, homepage, optional dependency lists.
- **version_provider** -- how to resolve the latest version. Often auto-detected from actions, so this section can be omitted.
- **steps** -- ordered actions. Each step has an `action` field plus action-specific parameters. Steps can include a `when` clause for platform conditionals (os, arch, libc, linux_family).
- **verification** -- command tsuku runs post-install. Version mode (default) compares versions; output mode just checks success.

## Action Reference

The full list of actions with parameter tables is in:

    skills/recipe-author/references/action-reference.md

The recipe-author SKILL.md also has a summary table of all action names
grouped by category (download, ecosystem, package managers, build systems,
file operations, special).

## Testing Workflow

Recipe testing has three phases:

### 1. Validate

```bash
tsuku validate path/to/recipe.toml
tsuku validate --strict path/to/recipe.toml
```

Checks TOML syntax, required fields, action parameters, and security rules
(HTTPS URLs, no path traversal). `--strict` treats warnings as errors.

### 2. Evaluate

```bash
tsuku eval --recipe path/to/recipe.toml
tsuku eval --recipe path/to/recipe.toml --os linux --arch amd64
```

Generates a resolved installation plan as JSON. Inspect the output to confirm
the right URLs, versions, and steps before running an actual install.

### 3. Sandbox Install

```bash
tsuku install --recipe path/to/recipe.toml --sandbox --force
```

Runs the full installation inside an isolated container (requires Docker or
Podman). Tests extraction, binary placement, and verification without
touching the host system.

For cross-family testing, evaluate with `--linux-family` and pipe to sandbox:

```bash
for family in debian rhel alpine arch suse; do
  tsuku eval --recipe recipe.toml --os linux --linux-family "$family" --arch amd64 | \
    tsuku install --plan - --sandbox --force
done
```

The recipe-test skill (skills/recipe-test/SKILL.md) covers golden file
validation, common failure patterns, and test infrastructure in detail.

## Guides

The following guides are available when working in the tsuku repo (they aren't
shipped with the plugin since sparsePaths only pulls the plugin directory):

| Guide | Topic |
|-------|-------|
| docs/guides/GUIDE-actions-and-primitives.md | Deep dive on action types |
| docs/guides/GUIDE-recipe-verification.md | Verification troubleshooting |
| docs/guides/GUIDE-hybrid-libc-recipes.md | glibc/musl split recipes |
| docs/guides/GUIDE-library-dependencies.md | Library auto-provisioning |
| docs/guides/GUIDE-distributed-recipe-authoring.md | Distributed recipe publishing |
| docs/guides/GUIDE-distributed-recipes.md | User guide for distributed installs |

External consumers who installed the plugin via settings.json can find
equivalent material in the bundled reference files under
`skills/recipe-author/references/`.
