# Distributed Recipes Guide

tsuku recipes don't have to live in the central registry. Anyone can host recipes in a GitHub repository, and users can install directly from them. This guide covers the full workflow.

## What Are Distributed Recipes?

A distributed recipe is a standard tsuku recipe TOML file hosted in a GitHub repository's `.tsuku-recipes/` directory. The recipe format is identical to what you'd find in the central registry. The only difference is where tsuku fetches it from.

This is useful when:
- Your organization maintains internal tools not suitable for the public registry
- A tool author wants to ship recipes alongside their source code
- You want to test recipes before submitting them upstream

## Installing from a Distributed Source

Use the `owner/repo` syntax to install from a GitHub repository:

```bash
# Install a specific recipe from a repo
tsuku install acme-corp/internal-tools:deploy-cli

# Install a specific version
tsuku install acme-corp/internal-tools:deploy-cli@2.1.0

# If the repo contains only one recipe, skip the recipe name
tsuku install acme-corp/my-tool

# With a version
tsuku install acme-corp/my-tool@1.0.0
```

### Format Variants

| Syntax | Meaning |
|--------|---------|
| `owner/repo` | Install the only recipe (or default) from the repo |
| `owner/repo:recipe` | Install a named recipe from the repo |
| `owner/repo@version` | Install a specific version of the default recipe |
| `owner/repo:recipe@version` | Install a specific version of a named recipe |

### Trust and Confirmation

The first time you install from a source you haven't used before, tsuku asks for confirmation:

```
Source acme-corp/internal-tools is not in your registries.
Install deploy-cli from this source? [y/N]
```

To skip the prompt (useful in CI or scripts), pass `-y`:

```bash
tsuku install -y acme-corp/internal-tools:deploy-cli
```

If you install from the same source often, add it as a trusted registry to avoid repeated prompts. See [Managing Registries](#managing-registries) below.

## Managing Registries

The `tsuku registry` commands let you manage which distributed sources you trust.

### List Registries

```bash
tsuku registry list
```

Shows all configured registries, including the central registry and any distributed sources you've added.

### Add a Registry

```bash
tsuku registry add acme-corp/internal-tools
```

After adding, installs from this source won't prompt for confirmation.

### Remove a Registry

```bash
tsuku registry remove acme-corp/internal-tools
```

Removing a registry doesn't uninstall tools that came from it. Those tools continue to work, but future updates will prompt for confirmation again.

### Registry Storage

Registries are stored in `$TSUKU_HOME/config.toml`. You can also edit this file directly:

```toml
[[registries]]
source = "acme-corp/internal-tools"

[[registries]]
source = "another-org/dev-tools"
```

## Strict Registries Mode

For CI pipelines or team environments where you want to control exactly which sources are allowed, enable strict mode:

```toml
# In $TSUKU_HOME/config.toml
strict_registries = true
```

With strict mode on, tsuku only installs from:
- The central registry
- Registries explicitly listed in your config

Any other `owner/repo` source is rejected outright. No confirmation prompt, just an error. This prevents accidental installation from untrusted sources in automated environments.

## How Existing Commands Work with Distributed Tools

Once installed, distributed tools behave like any other tsuku-managed tool. A few commands are worth calling out.

### update and outdated

`tsuku update` and `tsuku outdated` check the original distributed source for new versions, not the central registry. If the recipe author publishes a new version to their repo, you'll see it.

```bash
tsuku outdated
# deploy-cli  2.1.0  ->  2.3.0  (acme-corp/internal-tools)
```

### verify

`tsuku verify` uses the cached recipe from the distributed source. See the [Recipe Verification Guide](GUIDE-recipe-verification.md#source-directed-verification) for details.

### list and info

`tsuku list` and `tsuku info` show the source for each tool:

```bash
tsuku list
#   deploy-cli  2.1.0  (acme-corp/internal-tools)
#   kubectl     1.29.0 (registry)
```

### recipes

`tsuku recipes` includes recipes from your configured distributed registries alongside central registry entries.

### update-registry

`tsuku update-registry` refreshes caches for all configured sources, including distributed registries. This pulls down the latest recipe definitions without installing anything.

```bash
# Refresh all sources
tsuku update-registry

# Refresh only distributed sources
tsuku update-registry --distributed
```

## What's Next

If you want to host your own recipes, see the [Distributed Recipe Authoring Guide](GUIDE-distributed-recipe-authoring.md).
