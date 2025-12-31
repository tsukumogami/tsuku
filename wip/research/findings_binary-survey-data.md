# Binary Survey Data

Survey of 50+ popular developer tools from GitHub Releases to understand binary variant distribution.

## Survey Methodology

- Used `gh api repos/OWNER/REPO/releases/latest --jq '.assets[].name'` to fetch release assets
- Analyzed naming patterns, libc variants, architecture coverage
- Verified linking type using `file` command on sample binaries

## Survey Results

| Tool | Repo | glibc | musl | static | amd64 | arm64 | Naming Pattern |
|------|------|:-----:|:----:|:------:|:-----:|:-----:|----------------|
| ripgrep | BurntSushi/ripgrep | Y | Y | N | Y | Y | `{name}-{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| fd | sharkdp/fd | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| bat | sharkdp/bat | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| eza | eza-community/eza | Y | Y | N | Y | Y | `{name}_{arch}-unknown-linux-{libc}.tar.gz` |
| jq | jqlang/jq | Y | N | Y | Y | Y | `{name}-linux-{arch}` (raw binary) |
| yq | mikefarah/yq | Y | N | Y | Y | Y | `{name}_linux_{arch}` (Go static) |
| fzf | junegunn/fzf | Y | N | Y | Y | Y | `{name}-{ver}-linux_{arch}.tar.gz` (Go static) |
| delta | dandavison/delta | Y | Y | N | Y | N | `{name}-{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| dust | bootandy/dust | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| procs | dalance/procs | Y | N | N | Y | Y | `{name}-v{ver}-{arch}-linux.zip` |
| sd | chmln/sd | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| hyperfine | sharkdp/hyperfine | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| helm | helm/helm | Y | N | Y | Y | Y | `{name}-v{ver}-linux-{arch}.tar.gz` (Go static) |
| k9s | derailed/k9s | Y | N | Y | Y | Y | `{name}_Linux_{arch}.tar.gz` (Go static) |
| lazydocker | jesseduffield/lazydocker | Y | N | Y | Y | Y | `{name}_{ver}_Linux_{arch}.tar.gz` (Go static) |
| dive | wagoodman/dive | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.tar.gz` (Go static) |
| trivy | aquasecurity/trivy | Y | N | Y | Y | Y | `{name}_{ver}_Linux-{bit}.tar.gz` (Go static) |
| terraform | hashicorp/terraform | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.zip` (Go static) |
| vault | hashicorp/vault | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.zip` (Go static) |
| gh | cli/cli | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.tar.gz` (Go static) |
| git-lfs | git-lfs/git-lfs | Y | N | Y | Y | Y | `{name}-linux-{arch}-v{ver}.tar.gz` (Go static) |
| lazygit | jesseduffield/lazygit | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.tar.gz` (Go static) |
| starship | starship/starship | Y | Y | N | Y | Y | `{name}-{arch}-unknown-linux-{libc}.tar.gz` |
| zoxide | ajeetdsouza/zoxide | N | Y | Y | Y | Y | `{name}-{ver}-{arch}-unknown-linux-musl.tar.gz` |
| direnv | direnv/direnv | Y | N | Y | Y | Y | `{name}.linux-{arch}` (Go static) |
| just | casey/just | N | Y | Y | Y | Y | `{name}-{ver}-{arch}-unknown-linux-musl.tar.gz` |
| ninja | ninja-build/ninja | Y | N | N | Y | Y | `{name}-linux.zip` |
| cmake | Kitware/CMake | Y | N | N | Y | Y | `{name}-{ver}-linux-{arch}.tar.gz` |
| go | golang/go | Y | N | Y | Y | Y | `go{ver}.linux-{arch}.tar.gz` (static runtime) |
| deno | denoland/deno | Y | N | N | Y | Y | `{name}-{arch}-unknown-linux-gnu.zip` |
| bun | oven-sh/bun | Y | Y | N | Y | Y | `{name}-linux-{arch}[-musl].zip` |
| btop | aristocratos/btop | N | Y | Y | Y | Y | `{name}-{arch}-unknown-linux-musl.tbz` |
| glow | charmbracelet/glow | Y | N | Y | Y | Y | `{name}_{ver}_Linux_{arch}.tar.gz` (Go static) |
| gum | charmbracelet/gum | Y | N | Y | Y | Y | `{name}_{ver}_Linux_{arch}.tar.gz` (Go static) |
| age | FiloSottile/age | Y | N | Y | Y | Y | `{name}-v{ver}-linux-{arch}.tar.gz` (Go static) |
| sops | getsops/sops | Y | N | Y | Y | Y | `{name}-v{ver}.linux.{arch}` (Go static) |
| cosign | sigstore/cosign | Y | N | Y | Y | Y | `{name}-linux-{arch}` (Go static) |
| grype | anchore/grype | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.tar.gz` (Go static) |
| syft | anchore/syft | Y | N | Y | Y | Y | `{name}_{ver}_linux_{arch}.tar.gz` (Go static) |
| hugo | gohugoio/hugo | Y | N | Y | Y | Y | `{name}_{ver}_linux-{arch}.tar.gz` (Go static) |
| task | go-task/task | Y | N | Y | Y | Y | `{name}_linux_{arch}.tar.gz` (Go static) |
| nushell | nushell/nushell | Y | Y | N | Y | Y | `nu-{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| helix | helix-editor/helix | Y | N | N | Y | Y | `{name}-{ver}-{arch}-linux.tar.xz` |
| neovim | neovim/neovim | Y | N | N | Y | Y | `nvim-linux-{arch}.tar.gz` |
| lsd | lsd-rs/lsd | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| vivid | sharkdp/vivid | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| pastel | sharkdp/pastel | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| diskus | sharkdp/diskus | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| exa | ogham/exa | Y | Y | N | Y | N | `{name}-linux-{arch}[-musl]-v{ver}.zip` |
| gitui | extrawurst/gitui | Y | N | N | Y | Y | `{name}-linux-{arch}.tar.gz` |
| hexyl | sharkdp/hexyl | Y | Y | N | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| fnm | Schniz/fnm | Y | N | N | Y | N | `{name}-linux.zip` |
| volta | volta-cli/volta | Y | N | N | Y | N | `{name}-{ver}-linux.tar.gz` |
| xsv | BurntSushi/xsv | N | Y | Y | Y | N | `{name}-{ver}-{arch}-unknown-linux-musl.tar.gz` |
| mdBook | rust-lang/mdBook | Y | Y | Y | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-{libc}.tar.gz` |
| xh | ducaale/xh | N | Y | Y | Y | Y | `{name}-v{ver}-{arch}-unknown-linux-musl.tar.gz` |

## Legend

- **glibc**: Provides glibc-linked Linux binary
- **musl**: Provides musl-linked Linux binary
- **static**: Binary is statically linked (no external libc dependencies)
- **amd64**: Supports x86_64 architecture
- **arm64**: Supports aarch64 architecture

## Notes on Specific Tools

### HashiCorp Tools (terraform, vault, packer, consul)
- Distributed via releases.hashicorp.com, not GitHub Releases
- All are Go static binaries with no libc dependency
- Simple naming: `{name}_{ver}_linux_{arch}.zip`

### Go Tools Pattern
Most Go tools ship statically linked binaries that work on any Linux:
- gh, helm, k9s, dive, trivy, task, hugo, age, sops, cosign, grype, syft
- These typically have no libc indicator in the name since they don't need one

### Rust Tools Pattern (sharkdp ecosystem)
Tools from sharkdp (fd, bat, hyperfine, hexyl, vivid, pastel, diskus) and similar Rust projects:
- Follow Rust target triple naming: `{arch}-unknown-linux-{libc}`
- Consistently ship both glibc and musl variants
- Use `x86_64`, `aarch64`, `arm`, `i686` arch names

### Musl-Only Tools
Some tools only ship musl binaries:
- just (Rust)
- zoxide (Rust)
- btop (C++)
- xsv (Rust)
- xh (Rust)

### Tools with Complex Variants
- **bun**: Ships both glibc and musl, plus "baseline" CPU variants
- **hugo**: Ships "extended" variants with additional features
- **neovim**: Ships AppImage alongside tarball
