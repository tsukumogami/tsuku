# Library Backfill: Ranked Missing Dependencies

Generated from failure JSONL data in `data/failures/`. Aggregated `blocked_by` fields
across all `missing_dep` failures.

**Source data**: 50 failure files from batch runs (2026-02-07 through 2026-02-21)
**Total missing_dep failures**: 22 (all from homebrew ecosystem)
**Unique blocking libraries**: 15

## Ranked List

| Library | Block Count | Status | Category |
|---------|------------|--------|----------|
| gmp | 5 | Recipe exists, has satisfies | Math |
| libgit2 | 2 | No recipe | Version control |
| openssl@3 | 2 | Resolved via satisfies (openssl) | Crypto |
| bdw-gc | 2 | No recipe | Memory |
| ada-url | 1 | No recipe | Networking |
| dav1d | 1 | No recipe | Media |
| oniguruma | 1 | No recipe | Text processing |
| pcre2 | 1 | Recipe exists, no satisfies needed (name matches) | Text processing |
| glib | 1 | No recipe | Core |
| tree-sitter@0.25 | 1 | No recipe (tree-sitter recipe needs satisfies alias) | Parsing |
| libevent | 1 | No recipe | Networking |
| libidn2 | 1 | No recipe | Networking |
| gettext | 1 | Recipe exists, has satisfies | Internationalization |
| aarch64-elf-binutils | 1 | Recipe exists (toolchain, not library) | Toolchain |
| notmuch | 1 | No recipe | Mail |

## Already Resolved (no new recipe needed)

These libraries are already handled after the satisfies backfill (#1865):

- **gmp**: Recipe exists with satisfies entry. Tools blocked by gmp should resolve on next pipeline run.
- **openssl@3**: Resolved via `openssl` recipe's `satisfies = { homebrew = ["openssl@3"] }`.
- **gettext**: Recipe exists with satisfies entry.
- **pcre2**: Recipe exists, canonical name matches Homebrew formula. No name mismatch.
- **aarch64-elf-binutils**: Recipe exists. This is a toolchain package, not a library.

## Needs New Recipe (priority order)

These libraries need recipes created, ordered by block count:

1. **libgit2** (2 blocks) — Version control library. Blocks: bat, eza
2. **bdw-gc** (2 blocks) — Boehm garbage collector. Blocks: a2ps
3. **ada-url** (1 block) — URL parser. Blocks: node
4. **dav1d** (1 block) — AV1 decoder. Blocks: ffmpeg
5. **oniguruma** (1 block) — Regex library. Blocks: jq
6. **glib** (1 block) — GLib core library. Blocks: imagemagick
7. **tree-sitter** (1 block) — Parser generator. Blocks: neovim. Needs `@0.25` satisfies alias.
8. **libevent** (1 block) — Event notification library. Blocks: tmux
9. **libidn2** (1 block) — IDN library. Blocks: wget
10. **notmuch** (1 block) — Mail indexer. Blocks: aerc

## Tools Affected

| Tool | Blocked By | Ecosystem |
|------|-----------|-----------|
| aarch64-elf-gcc | gmp | homebrew |
| aarch64-elf-gdb | gmp | homebrew |
| coreutils | gmp | homebrew |
| shellcheck | gmp | homebrew |
| bat | libgit2 | homebrew |
| eza | libgit2 | homebrew |
| afflib | openssl@3 | homebrew |
| gitui | openssl@3 | homebrew |
| a2ps | bdw-gc | homebrew |
| node | ada-url | homebrew |
| ffmpeg | dav1d | homebrew |
| jq | oniguruma | homebrew |
| ripgrep | pcre2 | homebrew |
| imagemagick | glib | homebrew |
| neovim | tree-sitter@0.25 | homebrew |
| tmux | libevent | homebrew |
| wget | libidn2 | homebrew |
| vim | gettext | homebrew |
| aerc | notmuch | homebrew |

## Notes

- All current missing_dep failures are from the homebrew ecosystem. Other ecosystems
  (crates.io, rubygems, npm, pypi) have failures but in different categories
  (validation_failed, deterministic_insufficient, recipe_not_found).
- The queue has 2,830 pending entries (2,645 homebrew, 102 crates.io, 83 rubygems).
  More missing libraries will surface as the pipeline processes these entries.
- After creating these 10 library recipes, `queue-maintain` will auto-requeue
  blocked packages on merge.
