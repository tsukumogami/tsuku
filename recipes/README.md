# tsuku/recipes

Recipe registry for tsuku. Contains TOML recipe files that define how to install tools.

## Structure

```
recipes/
├── a/
│   ├── actionlint.toml
│   ├── age.toml
│   └── ...
├── b/
│   ├── bat.toml
│   └── ...
└── ...
```

## Recipe Format

Each recipe is a TOML file with the following structure:

```toml
[metadata]
name = "tool-name"
description = "Tool description"
homepage = "https://tool-homepage.com"
version_format = "semver"  # semver, calver, or custom

[version]
github_repo = "owner/repo"
tag_prefix = "v"  # Optional prefix to strip from tags

[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool-v{version}-{os}-{arch}.tar.gz"
archive_format = "tar.gz"
strip_dirs = 1
binaries = ["tool-name"]
os_mapping = { darwin = "darwin", linux = "linux" }
arch_mapping = { amd64 = "amd64", arm64 = "arm64" }

# Post-install steps run after the main installation (e.g., shell integration).
# The phase field controls when a step executes: "install" (default) or "post-install".
[[steps]]
action = "install_shell_init"
phase = "post-install"
source_command = "{install_dir}/bin/tool-name init {shell}"
target = "tool-name"
shells = ["bash", "zsh"]

[verify]
command = "tool-name --version"
pattern = "{version}"
```

## Adding a Recipe

1. Create `{first-letter}/{tool-name}.toml`
2. Follow the recipe format above
3. Test locally: `tsuku install --recipe-file path/to/recipe.toml`
4. Submit PR

## Validation

CI automatically validates all recipes on PR:
- Valid TOML syntax
- Required fields present (`[metadata]`, `[[steps]]`)
- Metadata includes name and description
- No hardcoded paths or secrets

## Actions Reference

| Action | Description | Common Parameters |
|--------|-------------|-------------------|
| `github_archive` | GitHub release tar.gz/zip | `repo`, `asset_pattern`, `archive_format`, `strip_dirs` |
| `github_file` | Single binary from GitHub | `repo`, `asset_pattern` |
| `download` | Download from any URL | `url`, `dest` |
| `extract` | Extract archive | `archive`, `format`, `strip_dirs` |
| `run_command` | Execute shell script | `command`, `description` |
| `install_binaries` | Register binaries for symlinking | `binaries`, `install_mode` |
| `hashicorp_release` | HashiCorp tools | - |
| `homebrew_bottle` | Homebrew bottles | - |
| `npm_install` | npm packages | - |
| `pipx_install` | Python packages | - |
| `cargo_install` | Rust crates | - |
| `gem_install` | Ruby gems | - |
| `nix_install` | Nix packages | - |
| `install_shell_init` | Install shell init scripts to `$TSUKU_HOME/share/shell.d/` | `source_file` or `source_command`, `target`, `shells` |
| `install_completions` | Install shell completion scripts to `$TSUKU_HOME/share/completions/` | `source_file` or `source_command`, `target`, `shells` |
