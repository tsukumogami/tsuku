# Lead: Registry-of-registries patterns

## Findings

### 1. Homebrew Taps

**Adding:** `brew tap owner/repo` clones `github.com/owner/homebrew-repo` into a local directory. Also supports `brew tap owner/repo <URL>` for non-GitHub sources. One command, zero config files to edit.

**Listing:** `brew tap` lists all tapped repositories.

**Removing:** `brew untap owner/repo`.

**Conflict resolution:** If a formula exists in both a tap and homebrew/core, you must use the fully qualified name `owner/repo/formula` to install the tap version. The core repository always wins by default.

**Persistence:** Git clones stored under `$(brew --repository)/Library/Taps/`. No separate state file; the presence of the clone is the state.

**UX friction:** One step. `brew tap owner/repo` and then `brew install formula` works. Can even skip the tap step entirely with `brew install owner/repo/formula` (implicit tap).

**Convention:** Repository must be named `homebrew-<something>` on GitHub; the prefix is stripped in CLI usage.

### 2. Cargo Alternative Registries

**Adding:** Edit `~/.cargo/config.toml` manually:
```toml
[registries]
my-registry = { index = "https://my-intranet:8080/git/index" }
```
No CLI command to add registries.

**Listing:** No built-in command. You read the config file.

**Removing:** Delete the entry from config.toml.

**Conflict resolution:** Dependencies must explicitly declare which registry they come from via the `registry` key in Cargo.toml. There's no implicit search across registries. A `registry.default` key can change the default from crates.io.

**Persistence:** `.cargo/config.toml` (hierarchical: project, user, system).

**UX friction:** High. Requires manual config file editing. Then each dependency must annotate its registry. No implicit discovery.

### 3. npm / .npmrc

**Adding:** Edit `.npmrc` (project or user level) or use `npm config set`:
```
@scope:registry=https://npm.pkg.github.com/
```

**Listing:** `npm config list` shows config, but no dedicated registry list command.

**Removing:** Edit `.npmrc` or `npm config delete`.

**Conflict resolution:** Scope-based routing. A scope (`@org/pkg`) maps to exactly one registry. Unscoped packages go to the default registry. One scope cannot map to multiple registries.

**Persistence:** `.npmrc` files (project, user, global levels).

**UX friction:** Medium. Requires understanding scoping model. Most developers copy-paste `.npmrc` snippets. The scope-to-registry binding is rigid but predictable.

### 4. Docker Registries

**Adding:** Edit `/etc/docker/daemon.json` for mirrors. For non-default registries, just use fully qualified image names (e.g., `ghcr.io/owner/image`).

**Listing:** No built-in command for listing configured registries.

**Removing:** Edit daemon.json, restart daemon.

**Conflict resolution:** Image names are inherently namespaced by registry hostname. `docker pull nginx` goes to Docker Hub; `docker pull ghcr.io/owner/nginx` goes to GitHub. No ambiguity because the registry is part of the identifier.

**Persistence:** `/etc/docker/daemon.json` for mirrors; image names carry their registry inline.

**UX friction:** For mirrors: high (daemon restart). For alternate registries: zero (just use the full image name). The "registry is in the name" pattern eliminates the need for registration entirely.

### 5. APT / Debian Repositories

**Adding:** Historically `add-apt-repository`. Modern approach: drop a `.sources` file into `/etc/apt/sources.list.d/` and a GPG key into `/etc/apt/keyrings/`. Requires sudo.

**Listing:** Read `/etc/apt/sources.list` and `/etc/apt/sources.list.d/*.sources`.

**Removing:** Delete the file from sources.list.d.

**Conflict resolution:** APT uses pinning (priority numbers). Default priority is 500. Users can assign higher priority to preferred repositories. Same package from multiple repos is handled by version comparison and pin priority.

**Persistence:** Files in `/etc/apt/sources.list.d/`.

**UX friction:** Very high. Multiple steps: download GPG key, store it, create sources file referencing the key, run `apt update`. Modern package managers' trust story is deliberately friction-heavy for security.

**Trust model:** Per-repository GPG keys. The shift from global trusted keyring to per-repo `signed-by` directives is notable -- it scopes trust to individual repositories rather than granting blanket trust.

### 6. Helm Chart Repositories

**Adding:** `helm repo add <name> <url>` -- single command with a local alias.

**Listing:** `helm repo list`.

**Removing:** `helm repo remove <name>`.

**Conflict resolution:** Charts are referenced as `repo-name/chart-name`. The repo name is always explicit in install commands.

**Persistence:** Managed by Helm in its local config directory.

**UX friction:** Low. One command to add, then install with `repo/chart` syntax. Very similar to Homebrew taps but with explicit naming.

### 7. Nix Flake Registry

**Adding:** `nix registry add <name> <flake-ref>`.

**Listing:** `nix registry list`.

**Removing:** `nix registry remove <name>`.

**Conflict resolution:** Three registry levels with precedence: user > system > global. User entries override system entries which override the global (downloaded) registry.

**Persistence:** JSON file. The global registry is fetched from a URL and cached. User and system registries are local JSON files.

**UX friction:** Low for adding. The three-layer precedence model adds complexity but is transparent. `nix registry pin` locks a flake to a specific revision, which is a nice touch for reproducibility.

### 8. WinGet Sources

**Adding:** `winget source add --name <name> <url>` -- requires admin privileges.

**Listing:** `winget source list`.

**Removing:** `winget source remove --name <name>`.

**Conflict resolution:** When a package exists in multiple sources, WinGet searches all sources. The `--source` flag disambiguates.

**Persistence:** System-managed (not a user-editable file).

**UX friction:** Medium. Admin privileges required for add/remove. Search spans all sources automatically.

### 9. Flatpak Remotes

**Adding:** `flatpak remote-add <name> <url>` -- supports `--if-not-exists` flag. System-wide vs per-user distinction.

**Listing:** `flatpak remotes` or `flatpak remote-list`.

**Removing:** `flatpak remote-delete <name>`.

**Conflict resolution:** Apps are identified by their app ID which includes the remote name. Subsets can be configured per remote.

**Persistence:** Files in `/etc/flatpak/remotes.d/` (system) or user config.

**UX friction:** Low. One command. Removing a remote doesn't uninstall apps already installed from it.

### 10. Go Module Proxy (GOPROXY)

**Adding:** Set environment variable: `GOPROXY=https://proxy1.example.com,https://proxy2.example.com,direct`

**Listing:** `go env GOPROXY`.

**Removing:** Edit the environment variable.

**Conflict resolution:** Ordered fallback. Go tries each proxy in order. A proxy returning 404/410 causes fallback to the next. The `direct` keyword means "fetch from source."

**Persistence:** Environment variable or `go env -w GOPROXY=...`.

**UX friction:** Low once understood, but the comma-separated env var model is less discoverable than CLI subcommands. `GOPRIVATE` provides an escape hatch for modules that should bypass proxies.

## Cross-system Comparison

| System | Add mechanism | Config format | Conflict model | Implicit add? | Steps to use |
|--------|--------------|---------------|----------------|---------------|-------------|
| Homebrew | CLI (`brew tap`) | Git clones on disk | Qualified name to disambiguate | Yes (`brew install owner/repo/pkg`) | 1 (or 0) |
| Cargo | Manual config edit | TOML | Explicit per-dependency annotation | No | 2+ |
| npm | Config edit / CLI | .npmrc | Scope-based routing | No | 2 |
| Docker | Name carries registry | daemon.json (mirrors only) | Namespaced by hostname | N/A (inline) | 0 |
| APT | File drop + GPG key | .sources/.list files | Pin priority | No | 3+ |
| Helm | CLI (`helm repo add`) | Internal state | Prefixed `repo/chart` | No | 1 |
| Nix | CLI (`nix registry add`) | JSON | Three-layer precedence | No | 1 |
| WinGet | CLI (`winget source add`) | System-managed | `--source` flag | No | 1 (admin) |
| Flatpak | CLI (`flatpak remote-add`) | Config files | App ID includes remote | No | 1 |
| Go Proxy | Env var | Env var | Ordered fallback | N/A (fallback) | 1 |

## Implications

### For tsuku's UX design

1. **The one-command-add pattern dominates.** Homebrew, Helm, Nix, WinGet, and Flatpak all use `<tool> <verb> add <name> <url>`. This should be tsuku's baseline: `tsuku registry add <name> <source>`.

2. **Implicit registration is a power-user convenience, not a default.** Only Homebrew supports it (`brew install owner/repo/formula` auto-taps). Most systems require explicit registration first. For tsuku, explicit-first is safer. Implicit could be added later as sugar.

3. **Conflict resolution splits into two camps:**
   - **Namespace-based** (Docker, Helm, npm scopes): the source is part of the package identifier. Clear but verbose.
   - **Priority/fallback** (APT, Go proxy, Nix): registries are searched in order. Convenient but can surprise users.
   - **Recommendation for tsuku:** Use qualified names (`registry/recipe`) when ambiguous, with the default registry (tsuku/recipes) searched first. This combines Homebrew's ergonomics with Helm's clarity.

4. **Persistence should be a config file, not hidden state.** Cargo's TOML and npm's .npmrc are inspectable and version-controllable. APT's file-per-repo approach is also clean. A `~/.tsuku/registries.toml` or entries in an existing config file would be ideal.

5. **Trust scoping matters.** APT's shift from global keyring to per-repo `signed-by` is instructive. Each registry should carry its own trust boundary. For tsuku, "registering" a registry is the trust boundary -- you've opted in.

6. **Removal should be clean.** Flatpak's pattern (removing a remote doesn't uninstall apps from it) is the right default. Installed tools should keep working even if their source registry is removed.

### Recommended UX for tsuku

```
# Add a registry (explicit trust boundary)
tsuku registry add company-tools https://github.com/company/tsuku-recipes

# List registered registries
tsuku registry list

# Remove a registry
tsuku registry remove company-tools

# Install from default registry (unchanged)
tsuku install ripgrep

# Install from specific registry (qualified)
tsuku install company-tools/internal-tool

# Search across all registries
tsuku search http-server
```

**Steps from "I found a registry" to "I can install from it":** Two. One to register, one to install. This matches the modal pattern of Helm, Nix, and Flatpak.

## Surprises

1. **Docker's "registry in the name" pattern eliminates the registration problem entirely.** It's arguably the simplest model, but only works because Docker images are inherently URL-like. For tsuku, recipe names aren't URL-like, so this doesn't map directly -- but it suggests that `owner/repo/recipe` as a fully qualified name could work without pre-registration.

2. **Cargo has no CLI for registry management.** Despite being a modern tool, it requires manual TOML editing. And it forces every dependency to declare its registry explicitly. This is the highest-friction model surveyed.

3. **Homebrew's implicit tap is both loved and risky.** Running `brew install owner/repo/formula` silently clones arbitrary GitHub repos. This is convenient but has no trust gate. Several security-focused discussions question this pattern.

4. **APT's trust model is deliberately high-friction.** The multi-step process (download key, store key, create source file, reference key in source) is intentional. Debian views the friction as a feature -- it forces users to think about what they're trusting. For tsuku (developer tools, not OS packages), this level of friction is probably excessive.

5. **Nix's three-layer registry (global + system + user) with a remotely-fetched global registry** is an interesting pattern. The global registry acts like a curated directory that can be overridden locally.

6. **WinGet requires admin privileges** to add sources, treating source management as a system-level security decision rather than a user preference.

## Open Questions

1. **Should tsuku support a GitHub convention like Homebrew's `homebrew-` prefix?** For example, repos named `tsuku-recipes-*` could be auto-discovered. This enables the short form `tsuku registry add owner/name` without a full URL.

2. **How should registry updates work?** When a user runs `tsuku update-registry`, should it update all registered registries or just the default? Homebrew's `brew update` updates all taps. This seems right.

3. **Should there be a global/curated registry-of-registries?** Nix fetches a global registry list from a URL. tsuku could maintain a directory of known third-party registries, enabling `tsuku registry add --from-directory company-tools`. This adds discoverability but also a centralization point.

4. **What's the persistence format?** Options:
   - Entries in `$TSUKU_HOME/state.json` (already exists for tool state)
   - A separate `$TSUKU_HOME/registries.toml`
   - Individual files per registry (APT-style)

   The first is simplest; the second is most inspectable; the third is most modular.

5. **Should `tsuku install owner/recipe` implicitly register, or require explicit registration?** The research suggests explicit-first is safer and more common. Implicit registration could be a future convenience feature with a confirmation prompt.

6. **How does this interact with recipe namespacing?** If two registries have a `ripgrep` recipe, what happens? Options: first-registered wins (Go proxy model), error requiring qualification (Homebrew model), or configurable priority (APT model).

7. **What authentication model for private registries?** Cargo uses tokens per registry in config. Homebrew clones via Git (SSH keys or HTTPS tokens). For tsuku, Git clone auth (leveraging existing SSH/HTTPS credentials) is probably the lowest-friction path.

## Summary

Every major package manager that supports third-party sources uses explicit registration with a single CLI command (`add`/`tap`/`remote-add`), persists the list in a config or state file, and resolves conflicts through either qualified names or ordered precedence. For tsuku, the strongest pattern is `tsuku registry add <name> <source>` with qualified `registry/recipe` names for disambiguation, matching the dominant Homebrew/Helm/Nix model while keeping installation from the default registry unchanged. The biggest open question is whether `tsuku install owner/recipe` should implicitly register (Homebrew-style convenience) or always require explicit `tsuku registry add` first (the more common and safer pattern across the ecosystem).
