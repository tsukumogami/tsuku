# Security Review: tsuku-homebrew-dylib-chaining

## Dimension Analysis

### External Artifact Handling
**Applies:** Yes

The design changes how Homebrew bottles are patched after download. Bottles
themselves come from `homebrew/core` GHCR and are not new attack surface for
this design — the sha256-pinned download path is unchanged. The new attack
surface is the patching logic: a recipe-controlled list
(`chained_lib_dependencies`) drives `patchelf --add-rpath` /
`install_name_tool -add_rpath` invocations that produce arguments derived
from recipe-name and resolved-version strings.

**Risks (after reading the source):**

1. **Path injection into RPATH via dep name (medium).** The current
   `fixLibraryDylibRpaths` constructs the RPATH target with:

   ```go
   depLibPath := filepath.Join(ctx.LibsDir, fmt.Sprintf("%s-%s", depName, depVersion), "lib")
   ```

   `filepath.Join` calls `filepath.Clean`, so a dep name like `../../etc`
   plus any `depVersion` collapses upward and yields a path outside
   `$TSUKU_HOME/libs/`. The patched RPATH would then point to an
   attacker-chosen filesystem location. The new `fixDylibRpathChain` /
   `fixElfRpathChain` per the design inherit this exact construction. The
   existing `set_rpath` action already mitigates this with
   `validateRpath(rpath, ctx.LibsDir)` (see `internal/actions/set_rpath.go`
   lines 404-460), which rejects absolute paths outside `LibsDir`. The new
   chain functions need the same gate.

2. **Option-injection into patchelf / install_name_tool (low).** Go's
   `exec.Command(name, args...)` uses `execve(2)` directly, so shell
   metacharacters (`;`, `|`, `$`, backticks) in arguments are passed
   verbatim — there is no shell interpretation. However, both `patchelf`
   and `install_name_tool` parse their own arguments. A dep name beginning
   with `-` would, when joined into the target path, produce a path the
   tool may interpret as a flag (e.g., `--remove-rpath` masquerading as a
   filename, or `-id` being eaten as an option). This is mitigated in
   practice because `filepath.Join(ctx.LibsDir, ...)` always prepends an
   absolute LibsDir, so the constructed argument starts with `/`, not `-`.
   The risk reappears only if `LibsDir` is empty/relative; the existing
   homebrew_relocate code falls back to `~/.tsuku/libs` when LibsDir is
   unset, which keeps the leading `/`.

3. **Loose recipe-name validation (low, structural).** `validateMetadata`
   in `internal/recipe/validator.go` (lines 167-208) only warns on
   uppercase, errors on spaces, and rejects dangerous schemes in the
   `homepage` URL. It does NOT reject `..`, `/`, or null bytes in the
   `metadata.name` field — those checks live in
   `internal/distributed/cache.go::validateRecipeName` and
   `internal/index/rebuild.go::isValidRecipeName` and are only applied at
   the registry-cache and index-rebuild boundaries. A malicious recipe
   uploaded to a third-party tap with name `../../foo` could, in
   principle, slip past metadata validation and be consumed by any code
   path that doesn't go through the cache or index. The new
   `chained_lib_dependencies` field reads these names and joins them into
   filesystem paths, so it inherits the same gap.

**Severity: medium.** Mitigations below; design needs an explicit
validation step before merge.

### Permission Scope
**Applies:** Yes

The change writes only to `$TSUKU_HOME/{tools,libs}/<recipe>-<version>/`,
which is the existing permission scope. No sudo, no system files. The
RPATH entries computed by the design are `$ORIGIN`-relative (Linux) or
`@loader_path`-relative (macOS), anchored to the binary's own directory.

**Risks:**

1. **Relative-path escape via `..` count (medium).** A dep name like
   `../../../foo` would pass through `fmt.Sprintf("%s-%s", ...)` and into
   `filepath.Rel`. The resulting `$ORIGIN/...` string can encode arbitrary
   numbers of `..` segments and so escape the install dir at load time.
   The runtime linker resolves `$ORIGIN/../../../../../etc/something/lib`
   without complaint. Combined with the path-injection risk above, this
   means a malicious recipe (or a typo) can cause the patched binary to
   load shared libraries from anywhere on the user's filesystem on every
   subsequent invocation. The same `validateRpath`-style gate fixes both.

2. **Defense-in-depth: validate post-Rel form (low).** The design says the
   relative path is computed via `filepath.Rel` after `EvalSymlinks` on
   both ends. After computing, the function should verify the relative
   path resolves back to a child of `LibsDir` (i.e., `filepath.Join(loaderDir, relPath)`
   stays within `$TSUKU_HOME`). This catches both the symlink-confusion
   case and the dep-name-traversal case in one check.

### Supply chain / dependency trust
**Applies:** Yes

Recipes are the trust boundary. A malicious recipe in a third-party tap
could declare `chained_lib_dependencies = ["../../etc"]` or
`chained_lib_dependencies = ["valid-name; rm -rf /"]`. Because
`exec.Command` does not invoke a shell, the second form is a non-issue
for command injection — the literal string `valid-name; rm -rf /` would
be passed to `patchelf`/`install_name_tool` as one argument (most likely
producing an error from the tool, not executing anything). The first
form is the real risk and is covered above.

**Risks (specific to the prompt's questions):**

- **`["../../etc/passwd"]` writing somewhere unintended.** The chain
  patches binaries inside `$TSUKU_HOME/tools/<recipe>-<version>/`, not
  any path derived from the dep name — the dep name only appears as RPATH
  *content*, not as a write target. So the write-target side is safe.
  But the RPATH content side can point anywhere on the filesystem and
  cause the binary to load arbitrary `.so` / `.dylib` files at runtime.
  Net: not a write primitive, but a load-path-redirection primitive.

- **`["valid-name"; rm -rf /]` shell-command injection.** Not exploitable.
  Go's `exec.Command` does not invoke a shell, so `;` is a literal
  character in the argument. The string would be passed to `patchelf` as
  a single rpath value; patchelf would either accept it (unusable rpath
  is a runtime concern, not an exec concern) or reject it.

- **Shell metacharacter in version string.** Same answer: `exec.Command`
  doesn't invoke a shell; metacharacters become literal argv bytes.
  Version strings come from version providers and are constrained by the
  provider — a malicious version on PyPI/NPM/crates.io would land in the
  argv as a literal, not as shell syntax. The risk shifts to whether
  patchelf/install_name_tool handles `-`-prefixed strings as options
  (covered above).

- **`$ORIGIN` / `@loader_path` prefix escape.** Yes, possible, as
  described in Permission Scope #1. The dep-name `..` count translates
  directly to RPATH `..` segments after `filepath.Rel`. Mitigated by
  validating dep names before path construction.

- **Validation via `ValidateBinaryPath` or equivalent.** There is no
  function with that exact name in the tree. The closest existing
  primitives are `isValidRecipeName` (`internal/index/rebuild.go:149`,
  rejects `/`, `..`, null) and `validateRecipeName`
  (`internal/distributed/cache.go:63`, rejects `..`, `/`, OS separator).
  Both should be promoted to the recipe validator and applied to every
  entry in `chained_lib_dependencies` at validate-time, plus enforced
  again at install-time defense-in-depth (consistent with how
  `validateRpath` defends `set_rpath`).

### Data exposure
**Applies:** No

The design explicitly chose `$ORIGIN`/`@loader_path`-relative RPATHs over
absolute paths. The patched binary embeds e.g.
`$ORIGIN/../../libs/libevent-2.1.12/lib`, not `/home/alice/.tsuku/...`.
No user-specific data (home dir, hostname, username) is embedded. The
relative path does encode the recipe name and version of each chained
dep, but that's the same information already carried by the recipe TOML
and not user-private.

The Mach-O install_name fix (`@rpath/libfoo.dylib`) similarly avoids
absolute paths.

### Project-specific (public-repo conventions)
**Applies:** Yes

The design and any added Security Considerations text must follow the
public-repo conventions in `public/CLAUDE.md`: no internal references,
no competitor names, professional tone. The proposed text below complies.

## Recommended Outcome

**Considerations worth documenting** — the design is fundamentally sound
(relative paths, no sudo, recipe-only trust boundary), but two concrete
validation gaps need to be called out and mitigated before implementation.
These do not require a new design loop; they fit naturally into Phase 1
(schema + validator) and Phase 2/3 (the `fixDylibRpathChain` /
`fixElfRpathChain` functions). Adding the Security Considerations text
below records the threat model and the mitigations the implementation
must include.

The two mitigations:

1. **Validate every `chained_lib_dependencies` entry at recipe-load time**
   in `internal/recipe/validator.go`: reject entries containing `/`,
   `\`, `..`, leading `-`, null bytes, or any non-`[a-z0-9._-]`
   character. This is the same shape as the existing
   `isValidRecipeName` (`internal/index/rebuild.go:149`); promote it
   into a shared helper and apply to both `metadata.name` and
   `chained_lib_dependencies`.
2. **Defense-in-depth at patch time** in the new
   `fixDylibRpathChain`/`fixElfRpathChain`: after computing the relative
   RPATH, verify `filepath.Join(loaderDir, relPath)` is still inside
   `$TSUKU_HOME/libs/` using the same logic as
   `validateRpath`/`validatePathWithinDir` in
   `internal/actions/set_rpath.go`. Reject the entry (skip-and-warn or
   fail-fast — pick one, document the choice) if validation fails.

## Recommended Security Considerations text

```markdown
## Security Considerations

The chain mechanism extends the trust boundary that already governs
`homebrew` bottles: bottle contents come from `homebrew/core` GHCR with
unchanged sha256 verification, and patches are applied only inside
`$TSUKU_HOME/{tools,libs}/<recipe>-<version>/`. The new field
(`chained_lib_dependencies`) is recipe-controlled, so a recipe is the
trust boundary for the chain content.

### Threat model

A malicious or buggy recipe in a third-party tap could declare an entry
that, after string interpolation into a filesystem path, escapes
`$TSUKU_HOME/libs/`. For example, `chained_lib_dependencies =
["../../etc"]` would, with `filepath.Join`'s `Clean` semantics, collapse
upward and produce an RPATH target outside the install root. The
runtime linker resolves the resulting `$ORIGIN`/`@loader_path`-relative
RPATH on every invocation, so this is a load-path-redirection primitive
(the patched binary loads shared libraries from an attacker-chosen
location), not a write primitive.

Argument injection into `patchelf` or `install_name_tool` via shell
metacharacters is not a concern — Go's `exec.Command` uses `execve(2)`
directly and does not invoke a shell. Option injection (a dep name
beginning with `-` being interpreted as a tool flag) is mitigated by the
fact that the constructed argument always starts with the absolute
`LibsDir` path, but the validation below removes the dependency on that
indirect mitigation.

### Mitigations

1. **Strict validation of `chained_lib_dependencies` entries at recipe
   load time.** Each entry must match `^[a-z0-9._-]+$`, reject `/`,
   `\`, `..`, leading `-`, and null bytes. This matches existing
   recipe-name validation in the registry cache and index-rebuild paths
   and should be promoted to a shared helper applied uniformly.

2. **Defense-in-depth at patch time.** Both `fixDylibRpathChain`
   (macOS) and `fixElfRpathChain` (Linux) compute the relative RPATH
   via `filepath.Rel` after `EvalSymlinks`, then verify
   `filepath.Join(loaderDir, relPath)` resolves back into
   `$TSUKU_HOME/libs/`. An entry that fails this check is rejected
   (the install fails with a clear error rather than producing a binary
   with an out-of-tree RPATH).

3. **No user-specific data is embedded in patched binaries.** The
   chosen RPATH form is `$ORIGIN`/`@loader_path`-relative, so the
   binary does not embed `$TSUKU_HOME` or any absolute user path. This
   preserves portability across `$TSUKU_HOME` moves and avoids leaking
   home-directory paths through binaries that may be copied or shared.

### Out of scope

Trust in the bottle contents themselves (a compromised
`homebrew/core` upload) is governed by the existing sha256 verification
in the homebrew action and is not changed by this design. Trust in the
recipe registry (a compromised recipe in `tsuku/recipes` or a
third-party tap) is the same trust boundary as every other recipe field
and is mitigated by the validator rules above.
```

---

## YAML summary

```yaml
outcome: considerations_worth_documenting
findings_count: 3
severity_max: medium
report_file: /home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/wip/research/design_tsuku-homebrew-dylib-chaining_phase5_security.md
```
