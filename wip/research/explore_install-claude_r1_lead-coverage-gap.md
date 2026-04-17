# Lead: Coverage gap analysis

## Findings

### Registry scale and composition

The `recipes/` directory contains 1,405 TOML recipe files organized alphabetically (`a/` through `z/`). Of these, 187 are handcrafted (no `tier`, `llm_validation`, or `requires_sudo` fields; use `github_archive` or direct download actions) and 1,218 are batch-generated (carry `tier = 0`, `llm_validation = "skipped"`, blank version fields, and use the `homebrew` bottle action). The `discovery/` subdirectory holds 823 JSON stubs — naming a tool's source and builder but providing no install instructions.

### Tool-by-tool coverage across 50+ high-profile tools

**Language runtimes and version managers**

| Tool | Recipe? | Quality |
|------|---------|---------|
| Go | `golang.toml` | Handcrafted — downloads from `go.dev/dl`, dual binary (`go`, `gofmt`), version pattern verified |
| Node.js | Missing | Discovery entry only (`node.json`, 2.6M Homebrew downloads) |
| Python 3 | Missing | Discovery as `python@3.13` (2.9M downloads), `python@3.12` (1.07M) |
| Rust (rustup) | `rustup.toml` | Batch-generated; uses `homebrew` action, `version_format = ""`, blank version fields |
| Ruby | Missing | No recipe, no discovery entry |
| Java (openjdk) | Missing | No recipe; `liberica.toml` (JDK distribution) is handcrafted |
| Bun | `bun.toml` | Handcrafted |
| fnm | `fnm.toml` | Handcrafted, direct GitHub archive |
| mise | `mise.toml` | Handcrafted, direct GitHub archive |
| pyenv | Missing | No recipe, no discovery entry |
| nvm | Missing | No recipe, no discovery entry |

**Cloud CLIs**

| Tool | Recipe? | Quality |
|------|---------|---------|
| AWS CLI | Missing | Discovery only (`awscli.json`, 2.2M Homebrew downloads) |
| Google Cloud SDK (`gcloud`) | Missing | No recipe, no discovery entry |
| Azure CLI | Missing | No recipe; `azure-dev.toml` (azd) is batch-generated |
| Cloudflare (`wrangler`) | `wrangler.toml` | Handcrafted, npm-based |
| DigitalOcean (`doctl`) | `doctl.toml` | Handcrafted |

**Editors and IDEs**

| Tool | Recipe? | Quality |
|------|---------|---------|
| Neovim | Missing | Discovery only (`neovim.json`, 541K downloads) |
| Helix | `helix.toml` | Handcrafted, direct GitHub archive, multi-platform |
| Zed | Missing | No recipe, no discovery entry |
| Vim | Missing | No recipe, no discovery entry |
| VSCode (`code`) | Missing | No recipe, no discovery entry |

**Infrastructure and Kubernetes**

| Tool | Recipe? | Quality |
|------|---------|---------|
| Terraform | `terraform.toml` | Handcrafted, direct HashiCorp download, checksum verified |
| kubectl | `kubernetes-cli.toml` | Batch-generated; Homebrew-only, Linux-only (`supported_os = ["linux"]`), no version resolution |
| Helm | `helm.toml` | Batch-generated; Homebrew-only, Linux-only, no version resolution |
| k9s | `k9s.toml` | Handcrafted, direct GitHub archive |
| minikube | `minikube.toml` | Handcrafted, direct GitHub binary |
| kind | `kind.toml` | Handcrafted |
| k3d | `k3d.toml` | Handcrafted |
| kubectx / kubens | Handcrafted | Both present, direct GitHub archive |
| kubeseal | `kubeseal.toml` | Handcrafted |
| kustomize | `kustomize.toml` | Handcrafted |
| Bazel | `bazel.toml` | Batch-generated; hardcodes binary name `bin/bazel-9.0.0`, macOS unsupported |

**Developer utilities (modern CLI tools)**

| Tool | Recipe? | Quality |
|------|---------|---------|
| jq | `jq.toml` | Batch-generated; Homebrew-only, macOS explicitly unsupported |
| fzf | `fzf.toml` | Handcrafted, direct GitHub archive, version pattern verified |
| ripgrep (`rg`) | `ripgrep.toml` | Batch-generated; Homebrew-only, blank version fields |
| bat | Missing | Discovery only (`bat.json`, GitHub builder) |
| fd | `fd.toml` | Batch-generated; Homebrew-only, blank version fields |
| delta (git-delta) | `git-delta.toml` | Batch-generated; Homebrew-only, arm64/Linux unsupported |
| eza | `eza.toml` | Batch-generated; Homebrew-only, blank version fields |
| starship | Missing | Discovery only (`starship.json`, GitHub builder) |
| direnv | `direnv.toml` | Handcrafted, direct GitHub binary |
| zoxide | `zoxide.toml` | Batch-generated; Homebrew-only, blank version fields |
| gh (GitHub CLI) | `gh.toml` | Handcrafted, dual-OS archive, well-formed |
| lazygit | `lazygit.toml` | Handcrafted |
| gum | `gum.toml` | Handcrafted |

**Build tools**

| Tool | Recipe? | Quality |
|------|---------|---------|
| CMake | Missing | Discovery only (1.58M Homebrew downloads) |
| Meson | Missing | No recipe, no discovery entry |
| Bazel | `bazel.toml` | Batch-generated (see above) |

**Container tools**

| Tool | Recipe? | Quality |
|------|---------|---------|
| Docker | `docker.toml` | Handcrafted; multi-distro (brew-cask, apt, dnf, pacman, apk, zypper) |
| Docker Compose | `docker-compose.toml` | Handcrafted |
| Podman | `podman.toml` | Batch-generated; macOS-only despite `supported_libc = ["glibc"]` contradiction |
| nerdctl | Missing | No recipe, no discovery entry |
| Colima | `colima.toml` | Handcrafted |

**AI and LLM tools**

| Tool | Recipe? | Quality |
|------|---------|---------|
| Claude Code (`claude`) | Missing | No recipe, no discovery entry |
| Ollama | `ollama.toml` | Batch-generated; Homebrew-only, macOS amd64 unsupported |
| Gemini CLI | Missing | Discovery only (384K downloads) |
| aichat | `aichat.toml` | Batch-generated; Homebrew-only |
| localai | `localai.toml` | Batch-generated |
| tsuku-llm | `tsuku-llm.toml` | Handcrafted; GPU-aware variants, conditional steps |
| Aider | Missing | No recipe, no discovery entry |

**Database tools**

| Tool | Recipe? | Quality |
|------|---------|---------|
| pgcli | Missing | No recipe, no discovery entry |
| redis-cli | Missing | No recipe, no discovery entry |

### The 13 most significant gaps

Ranked by estimated user impact (installation frequency × failure cost):

1. **Node.js** (`node`) — Discovery only, 2.6M Homebrew downloads. Foundational runtime for huge swaths of tooling (including Claude Code). Auto-generation would likely produce a Homebrew-only recipe; handcrafted should download from nodejs.org directly with platform-correct archives.

2. **AWS CLI** (`awscli`) — Discovery only, 2.2M downloads. The main cloud management tool for the largest cloud provider. Complex install (Python-bundled on Linux, pkg on macOS), making auto-generation unreliable.

3. **Claude Code** (`@anthropic-ai/claude-code`) — No recipe, no discovery entry. The motivating example for this exploration. Installs via npm; canonical name is `claude`. Five unrelated npm packages match on `tsuku install claude`. High failure cost: it's the tool tsuku users are most likely to install after setting up tsuku itself.

4. **Neovim** — Discovery only (541K downloads). The most popular terminal editor in developer tooling. Releases official binaries from GitHub; handcrafted `github_archive` recipe would be straightforward.

5. **CMake** — Discovery only (1.58M Homebrew downloads). Most widely-used C/C++ build tool. Downloads from `cmake.org` with platform-specific installers.

6. **bat** — Discovery only (GitHub builder). One of the 20 tools in `curated.jsonl` by source (via `sharkdp/bat`), yet no recipe exists. Clear GitHub release pattern matching ripgrep/fd.

7. **starship** — Discovery only (GitHub builder). Among the most installed shell prompt tools. Has GitHub releases with `starship-{arch}-{os}.tar.gz` pattern.

8. **Google Cloud SDK / gcloud** — No recipe, no discovery entry. Required for all GCP work. Installs from `dl.google.com` with interactive scripts — non-trivial to package.

9. **kubectl** (`kubernetes-cli.toml`) — Exists but is weak: Homebrew-only, Linux-only, no version resolution, does not install on macOS. Direct binary downloads from `dl.k8s.io` are straightforward.

10. **Helm** (`helm.toml`) — Same problem as kubectl: Homebrew-only, Linux-only, no version resolution. Official release tarballs from `get.helm.sh` or GitHub releases exist.

11. **Python 3 / pyenv** — No recipe for either. Python is complex to package directly; pyenv has clean GitHub releases. A handcrafted `pyenv.toml` would cover the use case without requiring a full Python build.

12. **Gemini CLI** — Discovery only (384K downloads). npm-based like Claude Code; mirrors the same gap. With Claude Code as a concrete reference, Gemini CLI becomes a natural second case.

13. **ripgrep / fd / eza / zoxide** — Present but batch-generated with Homebrew-only delivery and no version resolution. These four tools are in the curated.jsonl source list (or are equivalents) and deserve handcrafted recipes using direct GitHub releases, matching the quality of `fzf.toml`.

### Patterns by category

**Well-covered:** Kubernetes ecosystem tooling (k9s, kubectx, kubens, kubeseal, kustomize, kind, k3d, minikube), HashiCorp stack (terraform, vault, nomad, consul, boundary), Go developer tools (golangci-lint, gopls, goreleaser), Cloudflare tooling (wrangler, cloudflared).

**Thin coverage (batch-generated, Homebrew-only):** Modern Unix CLI replacements (ripgrep, fd, eza, bat, delta, zoxide), popular infra tools (helm, kubectl), some AI tools (ollama, aichat).

**Missing entirely:** Language runtimes (node, python, ruby, java), major cloud CLIs (aws, gcloud, azure), editors (neovim, zed, vim), AI coding assistants (claude, aider, gemini), build tools (cmake, meson), database CLIs (pgcli, redis-cli).

**Coverage quality is inversely correlated with tool complexity.** Simple Go binaries from GitHub releases are well-covered (handcrafted). Tools requiring custom download sources, runtime dependencies, or non-standard install flows (node, aws, python, claude) are absent or thin.

## Implications

### Batch-generated recipes for widely-used tools are actively misleading

A `kubernetes-cli.toml` that only works on Linux gives false confidence. A user on macOS types `tsuku install kubernetes-cli`, sees no error at search time, then fails at install. This is arguably worse than a missing recipe — it surfaces in search results and promises coverage it can't deliver. The same applies to `helm.toml`, `podman.toml`, and several others.

### The handcrafted vs. batch-generated split is meaningful for these tools

The high-priority tools above share a characteristic: their canonical install source differs from what Homebrew uses. Homebrew builds from source and serves bottles. tsuku's value proposition is zero-dependency binary installation. Handcrafted recipes can point directly to official release tarballs (terraform's HashiCorp download page, Node's nodejs.org, kubectl's dl.k8s.io). Batch-generated recipes default to Homebrew bottles because that's what the batch generator knows.

### The claude disambiguation case generalizes

The problem "tsuku install claude returns five unrelated packages" repeats across every AI tool. Gemini CLI, Aider, and Ollama (the npm package, not the Homebrew formula) all share the name-collision problem. A consistent resolution strategy (recipe name = canonical npm package name stripped of scope) would address the whole category, not just claude.

### Discovery entries without recipes create user friction

Neovim, bat, starship, cmake, and aws have discovery entries that let them appear in `tsuku search` results but offer no install path. A user who finds `neovim` in search and tries `tsuku install neovim` will get an error. Discovery-only coverage is worse than no coverage from a UX perspective because it generates failed install attempts rather than "tool not found" messages.

## Surprises

1. **kubectl and helm — both in the registry, both Linux-only.** Two of the five most-installed tools for Kubernetes practitioners exist as batch-generated Homebrew-only recipes that explicitly exclude macOS. Most Kubernetes users work primarily on macOS. The discovery entries include download counts of 571K and substantial Homebrew traffic.

2. **bat is in curated.jsonl but has no recipe.** The curation criteria research found that `bat` is explicitly in the 20-tool curated list with source `sharkdp/bat`. The discovery entry exists. Yet no recipe was ever authored. This is a direct pipeline gap: curated source resolution happened, but the recipe authoring step never ran for bat.

3. **The ripgrep recipe is macOS-compatible but uses Homebrew for Linux too.** Checking `ripgrep.toml` more carefully: it has no `supported_os` restriction, suggesting it could work on macOS (Homebrew runs on macOS). But the `version_format = ""` and blank `[version]` section mean `tsuku versions ripgrep` returns nothing, and any version pinning breaks. A handcrafted recipe would take 15 lines.

4. **No recipe for Python but `ruff`, `black`, `poetry`, `pipx`, and `uv` all have recipes.** The Python tooling ecosystem is well-covered, but Python itself is absent. This creates a dependency chain gap: if a user installs `black` via tsuku, they need Python available, but tsuku can't provide it.

5. **AI coding tools have zero handcrafted coverage.** Despite tsuku-llm (a custom tsuku project tool) being among the most sophisticated handcrafted recipes, the AI coding assistant category — claude, aider, gemini, opencode — has no recipes at all. These are precisely the tools that tsuku's target users install first.

## Open Questions

1. For tools like Node.js that ship official binaries from their own CDN (nodejs.org), should the recipe point there directly, or should it use a version manager (fnm, mise) as a dependency? The direct approach is simpler; the version manager approach is more flexible.

2. kubectl's recipe name is `kubernetes-cli` following Homebrew's naming. The command is `kubectl`. Should tsuku add a `kubectl.toml` alias, or fix `kubernetes-cli.toml` to work cross-platform and name the binary correctly?

3. For the four tools in curated.jsonl that lack recipes (bat is the confirmed case; others may exist), is there a systematic reason the recipe generation step was skipped, or are these just execution gaps?

4. The discovery entries with high Homebrew download counts (cmake at 1.58M, awscli at 2.2M, node at 2.6M) represent the clearest prioritization signal. Should discovery download counts drive an automated alert or queue-priority bump when no recipe exists above a threshold (e.g., 500K downloads)?

5. For the claude-specific case: the npm package `@anthropic-ai/claude-code` installs the `claude` binary. Should the recipe name be `claude-code` (matching the npm package short name) or `claude` (matching what users type)? The existing `wrangler.toml` pattern (recipe name = command name) suggests `claude`, but that name is ambiguous without disambiguation.

## Summary (3 sentences)

Of ~50 sampled high-profile developer tools, roughly 20 are entirely absent from the recipe registry (including node, AWS CLI, neovim, cmake, starship, bat, claude, gcloud, pyenv, and all major AI coding assistants), while another 8–10 exist only as batch-generated Homebrew-only recipes with no version resolution and platform restrictions that exclude macOS (kubectl, helm, ripgrep, fd, eza, zoxide, bazel, podman). Handcrafted recipe quality correlates tightly with install-mechanism simplicity: tools with direct GitHub release tarballs (fzf, gh, terraform, k9s, minikube) are well-covered, while tools requiring custom CDNs, npm installs, or complex platform variants (node, awscli, claude, gcloud) are absent or wrong. The claude disambiguation bug is one instance of a systematic pattern — AI tools, npm-installed CLIs, and tools with ecosystem-package name collisions lack both discovery entries and recipes — and the resolution approach (canonical recipe name = invoked command name, install path = official binary source) should be defined once and applied across claude, gemini-cli, ollama, and future additions.
