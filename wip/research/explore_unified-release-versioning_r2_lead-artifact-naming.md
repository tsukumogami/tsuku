# Lead: What artifact naming convention should be standardized across all three binaries?

## Findings

### Current naming is inconsistent
- **tsuku CLI**: GoReleaser format `tsuku-{os}-{arch}_{version}_{os}_{arch}` (version embedded, duplicated os/arch)
- **tsuku-dltest**: Simple format `tsuku-dltest-{os}-{arch}` (no version in filename)
- **tsuku-llm**: `tsuku-llm-v{version}-{platform}` (version embedded with "v" prefix)

### github_file action makes version in filenames redundant
The `github_file` action matches assets within a specific release tag. Since the release tag already identifies the version, embedding it in the filename is redundant. The inconsistency is what creates the #1791 mismatch.

### Option A: No version in any filename (recommended)
Format: `{tool}-{os}-{arch}[-{backend}]`
- `tsuku-linux-amd64`, `tsuku-dltest-linux-amd64`, `tsuku-llm-linux-amd64-cuda`
- Requires: update GoReleaser config (remove version suffix), update release.yml verification, update tsuku-llm recipe asset_pattern
- Breaking: existing release artifacts become obsolete (but users install via tsuku, not direct download)
- Simplest pattern, consistent across all tools

### Option B: Version in all filenames
Format: `{tool}-{version}-{os}-{arch}[-{backend}]`
- More complex, requires updating all recipes and workflows
- GoReleaser config already supports this but uses a different format

### Option C: Keep current mix
- Most fragile, requires recipes to handle heterogeneous patterns
- Not recommended for unified releases

## Implications

Option A aligns with how `github_file` actually works (finds assets within a tagged release), eliminates the #1791 mismatch, and gives all three tools a consistent pattern. The main change is GoReleaser config for the CLI.

## Surprises

The version in GoReleaser's default naming includes duplicated os/arch: `tsuku-linux-amd64_0.5.0_linux_amd64`. This is GoReleaser's default template, not intentional design.

## Open Questions

- Does any tooling or documentation link directly to release asset URLs with the current naming?
- Can GoReleaser's naming template be customized without breaking the release job?

## Summary
Artifact naming is inconsistent: CLI embeds version (GoReleaser default), dltest omits it, llm embeds it differently. Standardizing on no-version filenames (`{tool}-{os}-{arch}[-{backend}]`) is simplest since `github_file` already resolves within a tagged release. This requires updating GoReleaser config and the llm recipe's asset_pattern.
