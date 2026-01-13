## Problem

Recipes can have a `[version]` source configured but use `download_file` action with hardcoded versions in URLs. This is inconsistent - if you have a version source, you should be using `download` action with `{version}` placeholders.

Example from `testdata/recipes/bash-source.toml`:

```toml
[version]
source = "homebrew"
formula = "bash"

[[steps]]
action = "download_file"
url = "https://ftpmirror.gnu.org/gnu/bash/bash-5.3.tar.gz"  # Hardcoded!
checksum = "0d5cd86..."
```

The current hardcoded version detection (#656) skips `download_file` because it's designed for static assets. But when a recipe has a version source, using `download_file` with a version in the URL is suspicious.

## Affected Recipes

- `testdata/recipes/bash-source.toml`
- `testdata/recipes/readline-source.toml`
- `testdata/recipes/python-source.toml`
- `testdata/recipes/sqlite-source.toml`

## Proposed Solution

Add validation that warns when:
1. Recipe has a `[version]` source configured, AND
2. Recipe uses `download_file` action, AND
3. The `download_file` URL contains a version-like pattern

The warning should suggest using `download` action with `{version}` placeholder instead.

## Related

- #656 - Hardcoded version detection (implemented)
- #660 - SQLite non-standard version format
