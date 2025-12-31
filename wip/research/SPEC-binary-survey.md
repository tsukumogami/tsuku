# Research Spec P1-B: GitHub Binary Release Survey

## Objective

Understand what binary variants popular developer tools actually ship on GitHub Releases, to inform whether tsuku needs distro-specific binary selection or can rely on "universal" Linux binaries.

## Scope

Survey GitHub Releases for 50+ popular developer tools that tsuku would install.

### Tool Categories to Cover

| Category | Example Tools |
|----------|---------------|
| CLI utilities | ripgrep, fd, bat, exa, jq, yq, fzf |
| Version managers | nvm, pyenv, rbenv, goenv |
| Build tools | cmake, ninja, meson, bazel |
| Container tools | docker, podman, kubectl, helm, k9s |
| Cloud CLIs | aws-cli, gcloud, az, terraform |
| Language tools | go, rust (rustup), node, python, ruby |
| Dev tools | gh, git-lfs, lazygit, delta, htop, btop |
| Security tools | trivy, cosign, grype, syft |

## Research Questions

For each tool, document:

### 1. Release Asset Naming
- What Linux variants are provided?
- Examples: `linux-amd64`, `linux-musl`, `linux-gnu`, `unknown-linux-gnu`
- Is there a pattern or is it inconsistent?

### 2. Libc Variants
- Does the tool ship both glibc and musl builds?
- If only one, which one?
- Are binaries statically linked?

### 3. Architecture Coverage
- amd64/x86_64 only, or also arm64/aarch64?
- Any other architectures? (arm, riscv64)

### 4. Naming Conventions
- What naming pattern is used?
- `{tool}-{version}-{os}-{arch}`?
- `{tool}-{os}-{arch}`?
- Inconsistent?

### 5. Compression/Packaging
- Tarball, zip, raw binary, AppImage, deb, rpm?
- Does this vary by platform?

## Methodology

1. **GitHub API Queries**: Use `gh api` to fetch release assets for each tool
2. **Pattern Extraction**: Extract and categorize asset naming patterns
3. **Binary Analysis** (sample): For 5-10 tools, download and check:
   - `file <binary>` - ELF type, linking
   - `ldd <binary>` - Dynamic dependencies
   - Run on Alpine container - Does it work?

## Deliverables

### 1. Asset Survey Spreadsheet (`findings_binary-survey-data.md`)

| Tool | Repo | glibc | musl | static | amd64 | arm64 | naming pattern |
|------|------|-------|------|--------|-------|-------|----------------|
| ripgrep | BurntSushi/ripgrep | | | | | | |
| fd | sharkdp/fd | | | | | | |
| ... | | | | | | | |

### 2. Pattern Analysis (`findings_binary-patterns.md`)

- Most common naming conventions
- Percentage shipping musl variants
- Percentage with static builds
- Percentage supporting arm64

### 3. Compatibility Test Results (`findings_binary-compatibility.md`)

For sampled binaries:
- Which "generic linux" binaries work on Alpine?
- Which fail and why?
- What does `ldd` show?

### 4. Recommendations (`findings_binary-recommendations.md`)

Based on findings:
- Can tsuku assume "linux-amd64" binaries work everywhere?
- Should tsuku prefer static/musl builds when available?
- What detection/fallback strategy is needed?

## Output Location

All deliverables go in: `wip/research/`

## Sample Script

```bash
# Example: Survey a tool's releases
gh api repos/BurntSushi/ripgrep/releases/latest --jq '.assets[].name' | grep -i linux
```

## Time Box

- 30 mins: Set up survey methodology and script
- 2-3 hours: Survey 50+ tools
- 1 hour: Sample binary compatibility testing
- 30 mins: Write up findings

## Dependencies

None - this track runs independently.

## Handoff

Findings feed into:
- Phase 2 binary compatibility deep dive
- Decision on whether tsuku needs libc-aware targeting
