# Lead R1 — Homebrew bottle install patterns across the recipe registry

Research lead: enumerate Homebrew bottle install patterns across recipes, grouped by SHAPE of the steps. The distribution tells us which patterns are reused enough to deserve being a primitive.

## Aggregate counts

- Total recipe files: **1,439**
- Total recipes using `action = "homebrew"` (any curation): **1,168**
- **Curated** recipes total: **95**
- Curated recipes using `action = "homebrew"`: **10** (10.5%)
- Curated recipes NOT using `action = "homebrew"`: **85** (89.5%)
- Curated recipes using `action = "set_rpath"`: **1** (curl)
- Recipes (any curation) using BOTH `homebrew` AND `set_rpath`: **0** ← important: Pattern B as defined ("homebrew + install_binaries + set_rpath") does not exist anywhere in the registry today.
- Curated recipes with `[metadata.satisfies] homebrew = [...]`: **6** (utf8proc, bazel, libnghttp3, libevent, pcre2, openjdk)
- Curated recipes with `type = "library"`: **4** (utf8proc, libnghttp3, libevent, pcre2 — all also use homebrew)

### `install_mode` distribution

Across the entire `recipes/` tree, every literal `install_mode` value is `"directory"`. There are zero explicit `install_mode = "binaries"` declarations — when a recipe wants `binaries` mode, it omits the field (default is `"binaries"`, per `internal/actions/composites.go:412-413`).

- All recipes (any curation): **323** explicit `install_mode = "directory"` lines (231 in non-curated, 89 in curated, ~3 in CLAUDE.local.md docs).
- Curated homebrew recipes: 17 explicit `install_mode = "directory"` declarations across the 10 recipes; **1 of 10** (docker) uses the default `"binaries"` mode in its single install_binaries step on darwin.
- Non-curated homebrew recipes: **149 of 1,158** use `install_mode = "directory"` (~13%); the remaining ~87% rely on the default `"binaries"` mode.

The valid mode set per the action validator: `"binaries"`, `"directory"`, `"directory_wrapped"`. `directory_wrapped` does not appear in any recipe TOML.

## Pattern classification (curated recipes only)

The lead defines five patterns. Mapping the 10 curated homebrew recipes to those patterns:

| Recipe | Platforms with homebrew | install_mode | set_rpath | Step-level `dependencies` | `[metadata.satisfies] homebrew` | `type=library` | Pattern |
|--------|-------------------------|--------------|-----------|----------------------------|----------------------------------|----------------|---------|
| docker  | darwin only         | default ("binaries") | no | no  | no  | no  | **A** (basic, default-binaries) |
| pyenv   | linux+darwin        | directory            | no | no  | no  | no  | **A** (basic, directory) |
| tmux    | linux only          | directory            | no | no  | no  | no  | **A** (basic, directory) |
| openjdk | linux+darwin        | directory            | no | no  | yes | no  | **A** (basic, directory; multi-binary symlink) |
| libevent    | linux+darwin    | directory + explicit lib outputs | no | no | yes | yes | **C** (publishes dylibs for chaining) |
| libnghttp3  | linux+darwin    | directory + explicit lib outputs | no | no | yes | yes | **C** |
| pcre2       | darwin only     | directory + explicit lib outputs | no | no | yes | yes | **C** |
| utf8proc    | linux+darwin    | directory + explicit lib outputs | no | no | yes | yes | **C** |
| git   | linux(glibc)+darwin | directory | no | yes (darwin: `dependencies = ["pcre2"]`) | no | no | **E** (deps-on-homebrew-step; relies on built-in dylib patching, NOT set_rpath) |
| wget  | linux(glibc)+darwin | directory | no | yes (darwin: `dependencies = ["openssl","gettext","libidn2","libunistring"]`) | no | no | **E** (same as git, with comment confirming the homebrew action patches `@rpath`) |

### Pattern A — homebrew + install_binaries (basic shape)

Bottle + symlink. Either `install_mode="binaries"` (default, single binary symlinked) or `install_mode="directory"` (whole bottle copied, named outputs symlinked). 4 of 10 curated.

Representative samples:
- `recipes/d/docker.toml` (darwin only, default binaries mode, single binary)
- `recipes/p/pyenv.toml` (both platforms, directory mode, single binary)
- `recipes/t/tmux.toml` (linux only, directory mode, single binary)
- `recipes/o/openjdk.toml` (both platforms, directory mode, multi-binary: java/javac/jar)

### Pattern B — homebrew + install_binaries + set_rpath

**Zero recipes** (curated or non-curated) use this combination. The pattern is suggested by curl's source-build flow (`recipes/c/curl.toml:44-48`), but curl does NOT use homebrew on darwin — its darwin coverage is excluded entirely (`supported_os = ["linux"]`) precisely because the homebrew curl bottle has @rpath references to libnghttp3 that aren't currently chainable. So Pattern B as written ("curl pattern with homebrew") is the gap the exploration is investigating, not an existing primitive.

### Pattern C — homebrew + install_binaries with explicit lib outputs (chainable dylibs)

Library recipes that ship a homebrew bottle for the dylib AND list the dylib (plus `.a`, pkg-config, headers) in `outputs` so consumers can chain them. 4 of 10 curated. All 4 also have `type = "library"` and `[metadata.satisfies] homebrew = [...]`.

Representative samples:
- `recipes/p/pcre2.toml` — provides `libpcre2-8.0.dylib` for git's @rpath
- `recipes/u/utf8proc.toml` — provides `libutf8proc.3.dylib` for tmux's @rpath
- `recipes/l/libnghttp3.toml` — provides `libnghttp3.9.dylib` for curl's @rpath
- `recipes/l/libevent.toml` — provides `libevent-2.1.7.dylib` for tmux's @rpath

Notable: pcre2/libnghttp3/utf8proc are explicitly authored to publish the dylib that a downstream homebrew bottle's @rpath expects. The "explicit lib outputs" phrasing is slightly misleading — `install_mode="directory"` already copies the entire `lib/` tree; the named `outputs` array exists to register the named files for the install index (so downstream `install_binaries` and verify can find them) and acts as documentation. See pcre2.toml:69-78 for the canonical comment explaining this.

### Pattern D — homebrew NOT used (source build only)

85 of 95 curated recipes (89.5%) do not call homebrew at all. They cover the gamut: github_archive, hashicorp_release, configure_make source builds, cargo_install, npm_install, pipx_install, etc. Source-only recipes that explicitly bypass the homebrew bottle for a known reason:

- `recipes/c/curl.toml` — comment: "homebrew curl bottle has RPATH references to libnghttp3.9.dylib from a separate homebrew package; bundling transitive dylibs is not supported. darwin coverage is excluded until runtime_dependencies supports dylib chaining." This is the canonical Pattern D recipe and the smoking gun for the design discussion.
- The 85 also include: direnv, delta, deno, jq, fzf, ripgrep, gh, etc. — all use github releases or language-package-manager flows.

### Pattern E — anything else worth noting

Two recipes (git, wget) use a hybrid shape that isn't B or C:

- **`homebrew` + step-level `dependencies = [...]` + `install_binaries` (directory)** — no `set_rpath`. They rely on the homebrew action itself to patch @rpath references in the bottle's binary against the tsuku-installed dependencies. The wget comment is explicit: "tsuku's homebrew action patches these references to point at the matching tsuku-installed dependencies." (recipes/w/wget.toml:30-31). Git's comment is more nuanced: it duplicates `runtime_dependencies = ["pcre2"]` at the metadata level and `dependencies = ["pcre2"]` at the homebrew step because the metadata list isn't visible to the action during decomposition (recipes/g/git.toml:36-39).

This means dylib chaining is partly a **built-in behavior of the homebrew action** triggered by step-level `dependencies`, rather than something a recipe expresses with `set_rpath`. The two recipes that exercise it (git, wget) describe it as a working primitive — but neither curl nor tmux on darwin uses it, suggesting it has limits the curl/tmux comments allude to ("bundling transitive dylibs is not supported", "libevent itself is not installable on macOS via tsuku").

## Distribution summary table

| Pattern | Curated count | % of curated homebrew (n=10) | Examples |
|---------|---------------|-------------------------------|----------|
| A — basic homebrew + install_binaries | 4 | 40% | docker, pyenv, tmux, openjdk |
| B — homebrew + install_binaries + set_rpath | **0** | 0% | (does not exist) |
| C — homebrew + install_binaries with lib outputs | 4 | 40% | pcre2, libnghttp3, utf8proc, libevent |
| D — homebrew NOT used | 85 of 95 curated total | n/a | curl, direnv, gh, jq, fzf, ripgrep, ... |
| E — homebrew + step-level deps (built-in patching) | 2 | 20% | git, wget |

## Implications for the design discussion

1. **Pattern B does not actually exist** in the registry. The proposed "homebrew_with_chained_deps composite that wraps Pattern B" would not consolidate any existing recipes — it would be greenfield.
2. **Pattern E is the existing dylib-chaining primitive.** The homebrew action already patches @rpath when a step declares `dependencies = [...]`. Two recipes (git, wget) use it successfully. The curl/tmux/openjdk-on-darwin comments suggest it has gaps for transitive deps or specific bottles.
3. **Pattern C recipes are the supply side** of dylib chaining. They publish dylibs + pkg-config so Pattern E can consume them. The 4 Pattern C recipes are tightly coupled to specific Pattern E consumers (pcre2→git, utf8proc→tmux, libevent→tmux, libnghttp3→curl).
4. The natural design question is whether to:
   - **(a)** Strengthen the built-in homebrew-action patching so curl/tmux on darwin can use Pattern E (no recipe-author-facing change needed, just expand which @rpath patterns get rewritten), OR
   - **(b)** Introduce an explicit recipe-level Pattern B (homebrew + set_rpath) so authors can be explicit about chains the action's heuristic doesn't catch.
5. The blast radius of fixing this in tsuku-core is small for end users — only ~10 curated recipes use homebrew today, and only 2 of those exercise the chaining path. Most non-curated homebrew recipes (1,158) are batch-generated stubs that don't touch chaining. A core-side improvement to the homebrew action would unblock ~3 specific curated recipes (curl on darwin, tmux on darwin, anything depending on libevent on darwin) without recipe churn.

## Files referenced

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/c/curl.toml` (Pattern D, the gap)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/d/docker.toml` (Pattern A, default binaries)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/git.toml` (Pattern E)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libevent.toml` (Pattern C)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libnghttp3.toml` (Pattern C)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/o/openjdk.toml` (Pattern A, multi-binary)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/p/pcre2.toml` (Pattern C, has the canonical lib-outputs comment)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/p/pyenv.toml` (Pattern A)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/t/tmux.toml` (Pattern A, linux-only because of dylib chain gap)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/u/utf8proc.toml` (Pattern C)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/w/wget.toml` (Pattern E, has the "homebrew action patches these references" comment)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/actions/composites.go:412-432` (install_mode default + validation)
