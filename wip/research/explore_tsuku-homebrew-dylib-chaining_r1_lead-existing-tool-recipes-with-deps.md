# Existing curated tool recipes with homebrew + dylib deps

## Lead

Survey curated tool recipes (NOT `type = "library"`) that use the `homebrew` action AND declare `runtime_dependencies` (or per-step `dependencies`). These are recipes that have already been hand-coded to acknowledge the dylib chaining gap.

## Headline numbers

- **Curated recipes total**: 95 (`grep -rl 'curated = true' recipes/`)
- **Curated non-library tool recipes using `action = "homebrew"`**: 6
- **Of those, declaring deps (`runtime_dependencies` or per-step `dependencies`)**: **2** -- `git`, `wget`
- **Of those, declaring NO deps**: 4 -- `docker`, `openjdk`, `pyenv`, `tmux`

## The 6 curated tool recipes that use the homebrew action

### Declares deps (the workaround pattern)

| Recipe | Deps | macOS supported? | CI status at merge |
|---|---|---|---|
| `recipes/g/git.toml` | `runtime_dependencies = ["pcre2"]`; macOS step also has `dependencies = ["pcre2"]` | yes | PR #2376 merged 2026-05-03 -- Test Recipe macOS arm64 + x86_64 SUCCESS |
| `recipes/w/wget.toml` | `runtime_dependencies = ["openssl", "gettext", "libidn2", "libunistring"]`; macOS step also has `dependencies = [...]` | yes | PR #2337 merged 2026-04-28 -- Test Recipe macOS arm64 + x86_64 SUCCESS |

### Declares no deps

| Recipe | Why no deps? | macOS supported? |
|---|---|---|
| `recipes/d/docker.toml` | Static binary on Linux (`download_archive`); macOS uses homebrew but `docker` CLI is statically linked Go, no dylib chain | yes |
| `recipes/o/openjdk.toml` | macOS bottle ships full JDK tree under `libexec/openjdk.jdk/`, self-contained -- no external lib deps | yes |
| `recipes/p/pyenv.toml` | pyenv is pure shell scripts, no compiled binary -- no dylib chain at all | macOS yes (no real dylibs) |
| `recipes/t/tmux.toml` | `supported_os = ["linux"]` -- tmux's macOS bottle needs `libutf8proc.3.dylib` and `libevent`; comment explicitly says "darwin coverage is excluded until these deps are supported" | **no** -- excluded |

`tmux` is the smoking gun: it had to drop macOS entirely instead of using the workaround pattern, because libevent isn't installable on macOS via tsuku yet (even though it's a curated `type = "library"` recipe, see #2333).

## Why git and wget pass CI today

The chaining gap is in `internal/actions/homebrew_relocate.go:103`:

```go
if ctx.Recipe != nil && ctx.Recipe.Metadata.Type == "library" && runtime.GOOS == "darwin" {
    if err := a.fixLibraryDylibRpaths(ctx, installPath, reporter); err != nil {
```

`fixLibraryDylibRpaths` only runs for `Type == "library"`. So how do tool recipes git and wget get away with it on macOS?

### git -- pcre2 is curated and ships as a `type = "library"` recipe

- `recipes/p/pcre2.toml` declares `type = "library"` and `curated = true`.
- When `git` declares `runtime_dependencies = ["pcre2"]`, the executor installs pcre2 first.
- pcre2's own homebrew install hits the `Type == "library"` branch -- its own dylibs are RPATH-fixed.
- The git binary's load chain references `libpcre2-8.0.dylib`. The git PR comment (lines 32-39 of `recipes/g/git.toml`) explains:
  > "the homebrew action [for git] can resolve install-time deps during decomposition; without the step entry, the macOS plan would not see them at install time."
- Translation: the *executor* sees the dependency and installs pcre2's libs in a tsuku-managed path the git bottle's `@rpath` already expects. There is no chaining of pcre2's install dir into git's RPATH happening here -- it works because the bottle's relative `@rpath` happens to land in the right place after homebrew_relocate's placeholder substitution (`@@HOMEBREW_PREFIX@@` etc.). Subtle but real.

### wget -- same pattern, more deps

- `runtime_dependencies = ["openssl", "gettext", "libidn2", "libunistring"]`
- All four are curated `type = "library"` recipes (`recipes/g/gettext.toml`, `recipes/l/libidn2.toml`, `recipes/l/libunistring.toml`, plus openssl).
- Recipe comment (lines 27-31): "tsuku's homebrew action patches these references to point at the matching tsuku-installed dependencies."
- Same mechanism as git: placeholder substitution in homebrew_relocate plus libs being installed at the path the bottle's load chain expects.
- The wget PR (#2337) explicitly extended `recipes/g/gettext.toml`'s macOS `install_binaries` step to expose `lib/libintl.8.dylib` (the versioned name the wget bottle's `@rpath` actually references). That manual fixup is itself evidence of the chaining gap -- the lib has to be installed to the exact path the consumer's `@rpath` expects, because nothing rewrites the consumer's RPATH.

### Linux-side note (not blocking these)

On the Linux glibc path both recipes hit the same homebrew action; the `fixLibraryDylibRpaths` gate is `runtime.GOOS == "darwin"`-only, so it's not the bottleneck. Linux glibc bottles that have unmet shared lib deps would simply not load -- but for git/wget on debian/rhel/etc., the system ships `libssl`, `libidn2`, `libunistring`, `libpcre2`, and `libintl` in standard locations the loader can find. **This is the "system libs mask the gap" effect the lead asked about** -- on a minimal container without `libidn2` or `libpcre2-8` present, these would be at risk. The nightly matrix runs alpine + debian + arch + rhel + suse, all of which package these common libs, so the gap doesn't surface there.

## Why so few?

The sample is intentionally narrow because the curated set is heavily biased toward:

1. **Pure static binaries from GitHub releases** (most of the 95 -- gh, kubectl, terraform, helm, etc. -- use `github_archive` / `github_file`, not `homebrew`).
2. **Tools where homebrew bottles are self-contained** (docker, openjdk, pyenv).
3. **A short list of "homebrew because nothing else works"** -- which is exactly where the chaining gap bites: git, wget, tmux, and the deferred curl (#2338).

So the existing curated cohort understates the blast radius. **What's missing from this sample is everything that hasn't been curated yet because it would hit the gap immediately.** The wget PR description names curl as deferred (#2338) for exactly this reason; tmux is also deferred (#2336 referenced libevent #2333). Both are open dependency-of-tool stories, not solved ones.

## Names + one-line summaries

- **git** (#2376) -- works on macOS by declaring `pcre2` runtime dep; pcre2 is a curated library so its dylibs land at a path the git bottle's `@rpath` happens to reach via homebrew_relocate's placeholder substitution.
- **wget** (#2337) -- works on macOS by declaring `openssl`/`gettext`/`libidn2`/`libunistring` deps and *manually patching gettext's recipe* to expose the versioned `libintl.8.dylib` the wget bottle expects. Effectively a workaround that requires recipe-side coordination per consumer.
- **docker** -- works on macOS because the docker CLI is a static Go binary; the homebrew bottle just ships that one binary.
- **openjdk** -- works on macOS because the bottle ships the entire JDK tree under `libexec/`; no external dylibs to chain.
- **pyenv** -- works on macOS because pyenv is pure shell, no compiled binary, no dylibs.
- **tmux** -- doesn't work on macOS; recipe sets `supported_os = ["linux"]` because libevent and libutf8proc dylib chaining isn't supported.

## Bottom line for the core question

The recipe-side workaround pattern (declare deps, hope library recipes ship dylibs at the right path, patch library recipes to expose versioned names) is **viable but fragile**. It worked for git (1 dep) and wget (4 deps, requiring an upstream library recipe edit). It already failed/was-deferred for curl and tmux. Each new tool that needs it is a hand-tuned coordination between a tool recipe and one or more library recipes, often requiring the library recipe to be edited to expose specific versioned dylib names.

The "system libs mask the gap on Linux" hypothesis is correct: nightly matrix passes for git and wget across debian/rhel/arch/suse/alpine because all of these ship the relevant libs (`libpcre2`, `libidn2`, `libunistring`, `libintl`, `libssl`) in the loader's default search path. A minimal scratch-style container would expose the gap on Linux too.

## Files referenced

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/git.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/w/wget.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/d/docker.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/o/openjdk.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/p/pyenv.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/t/tmux.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/p/pcre2.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/gettext.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libidn2.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libunistring.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/actions/homebrew_relocate.go` (gate at line 103, `fixLibraryDylibRpaths` at line 574)
