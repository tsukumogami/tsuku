# Security Review: tsuku-homebrew-dylib-chaining (revised design)

This review supersedes the prior round-1 review. The design was substantially
revised after round-2 docker-based exploration: the proposed
`chained_lib_dependencies` field is **dropped** (Decision 2 reversed); the
existing `metadata.runtime_dependencies` field is reused; a new SONAME
completeness scan with auto-include is added (Decision 4); RPATH writer
switches to `patchelf --force-rpath --set-rpath` (DT_RPATH) on Linux; the
`Type == "library"` gate is lifted; a new `internal/install/soname_index.go`
module is introduced. The trust-boundary core (recipe registry is trusted;
bottle contents are sha256-pinned) is unchanged.

The previous review's two recommended mitigations (recipe-name validator,
`filepath.Join` defense-in-depth at patch time) are now both written into
the design (Phases 1, 3, and 4; "Mitigations" section). They still apply
verbatim — only the field they target moved from `chained_lib_dependencies`
to `runtime_dependencies`.

## Dimension Analysis

### External Artifact Handling
**Applies:** Yes

Two new flows:

1. **Bottle binaries fed to `readelf -d` / `otool -L`.** The bottle is
   already a sha256-pinned trusted input. `readelf` and `otool` are
   well-tested system tools that parse ELF / Mach-O headers — both formats
   are bounded, length-prefixed, and well-defined. A pathological bottle
   could declare:
   - SONAMES with embedded path separators, null bytes, or shell
     metacharacters (e.g., `NEEDED libfoo.so;rm -rf /`)
   - Extremely long SONAMES (kilobyte strings)
   - Many NEEDED entries (hundreds)
   - SONAMES that are valid filesystem-traversal strings (e.g.,
     `../../../etc/ld.so.conf`)

   The classification step looks each SONAME up in a Go map keyed by
   string. Map lookups are inert with respect to the SONAME's content —
   a `..`-shaped SONAME would simply not match any provider entry and
   fall through to the "coverage gap" log line. The auto-include path
   (below) only fires when the SONAME *matched* a tsuku library, so the
   path it generates is built from the **provider's** known recipe name +
   version, not from the SONAME string. This means the SONAME itself
   never reaches `filepath.Join` as a path component; it only ever
   reaches a logger and a map lookup.

   That property is load-bearing. The design needs to make it explicit
   so an implementer doesn't accidentally interpolate the SONAME into a
   path or a shell argument later (e.g., for a friendlier error message).

2. **`readelf` / `otool` output parsed as text.** Both tools' output
   formats are stable, but unknown-format binaries or corrupted ELF
   headers could produce diagnostic text that the parser misclassifies.
   The classifier should treat parse failures as "skip this binary" with
   a warning, not as "no NEEDED entries" (the latter would silently
   miss SONAMES the binary actually needs).

**Severity: low.** The design's structure already isolates SONAME content
from path construction; the recommendation is to document that invariant.

### Recipe-controlled inputs
**Applies:** Yes

The trust boundary moved from a new field to an existing field. The
existing `runtime_dependencies` field has **no validator today** beyond
TOML well-formedness. The wrapper-PATH consumer that already reads it
treats entries as opaque recipe-name strings; if a recipe declared
`runtime_dependencies = ["../../etc"]` today, the wrapper-PATH consumer
would attempt to resolve a recipe by that name and fail at the registry
boundary (where `validateRecipeName` and `isValidRecipeName` already
reject `..` and `/`).

The new chain consumer joins each entry into a filesystem path
(`filepath.Join(ctx.LibsDir, fmt.Sprintf("%s-%s", depName, depVersion), "lib")`),
which is exactly the construction the previous review flagged. With
`filepath.Clean` semantics, a name containing `..` collapses upward and
escapes `LibsDir`. The design's Phase 1 calls out the validator addition
explicitly:

> Validate `runtime_dependencies` entry name pattern (`^[a-z0-9._-]+$`);
> reject empty strings, `..`, `/`, null bytes, leading `-`. Validate each
> entry resolves to an installable recipe. Promote `isValidRecipeName`
> from `internal/distributed/cache.go` and `internal/index/rebuild.go`
> into a shared helper.

This addresses the path-injection vector at recipe-load time and is the
right place for it.

**Risks (as before, now anchored on `runtime_dependencies`):**

1. **Path injection via dep name (medium).** `runtime_dependencies =
   ["../../etc"]` produces an RPATH outside `$TSUKU_HOME/libs/`. Mitigated
   by the Phase 1 validator + the Phases 3/4 `filepath.Join` post-check.

2. **Option injection into patchelf / install_name_tool (low).** A name
   beginning with `-` could be eaten as a tool flag if it ever ended up
   un-prefixed. The constructed argument always starts with the absolute
   `LibsDir`, which keeps the leading `/`. The validator's "reject
   leading `-`" rule removes the dependency on that indirect mitigation.

3. **Version-string injection (low).** `depVersion` is also interpolated
   into the path. Versions come from version providers (PyPI, npm,
   crates.io, GitHub release tags). Currently no validator constrains
   them. A version like `../foo` would have the same effect as a name
   like `../foo`. This is a pre-existing issue the design inherits but
   does not introduce. Worth flagging — the `filepath.Join` post-check
   in Phases 3/4 catches it as defense-in-depth, so the structural risk
   is bounded, but a version validator at provider boundaries would be
   a cleaner long-term fix. **(Out of scope for this design.)**

### NEW: SONAME index construction
**Applies:** Yes

The new `internal/install/soname_index.go` module walks every installable
library recipe and parses each recipe's `outputs` lists for `lib/lib*.so.*`
and `lib/lib*.*.dylib` patterns. New attack surface:

1. **Malicious `outputs` entry escaping `lib/`.** A library recipe with
   `outputs = ["../../etc/passwd"]` could in principle have its "SONAME"
   parsed as something the index would map to `etc-passwd` or similar.
   The index doesn't fetch or open the file — it parses the **string** of
   the output entry to derive a SONAME basename. So the index itself is
   safe from filesystem traversal (no I/O on the malicious path). But:
   - The derived "SONAME" could be a confusing string that, when later
     written to a log, misleads a human reading the warning.
   - If the parser is permissive and accepts paths outside `lib/`, a
     library recipe could inject mappings for SONAMES it doesn't
     legitimately ship.

   Mitigation: the index parser should reject any `outputs` entry whose
   path is not exactly under `lib/` (i.e., starts with `lib/` and does
   not contain `..`). The basename it extracts as the SONAME should
   itself match the SONAME regex (`^lib[a-zA-Z0-9._+-]+\.(so|dylib)(\.[0-9.]+)?$`
   or similar). Anything that doesn't match is dropped from the index
   with a one-line warning at index-build time.

   **Severity: low** — this is a recipe-author trust-boundary issue
   (compromised library recipe), but the same principle as the
   `runtime_dependencies` validator applies: validate at the boundary
   instead of trusting the input.

2. **SONAME collision between two library recipes (medium).** Two
   library recipes could both legitimately or maliciously claim the same
   SONAME (e.g., recipe `openssl-3` and recipe `openssl-evil` both
   declare `lib/libssl.so.3`). The auto-include path then has to pick
   one. If the index uses a Go map and the second insert silently
   overwrites the first, the chosen provider depends on iteration order
   — non-deterministic and exploitable: a recipe author could arrange
   for their lookalike to be the one selected at install time for any
   tool that needs `libssl.so.3` and didn't declare it.

   The design says the index maps `SONAME → providing recipe + version`
   (singular). It needs to specify what happens on collision. Options:
   - **Error at index build.** Refuse to construct the index if any
     SONAME has multiple providers. Explicit, but may break legitimate
     cases where two recipes ship overlapping SONAMES (e.g., `openssl@1`
     and `openssl@3`).
   - **Multi-valued map + heuristic selection.** Map SONAME → list of
     providers; auto-include picks the one already in the recipe's
     `runtime_dependencies` if any, else falls back to a deterministic
     order (alphabetical recipe name, or "no match — log coverage gap
     with all candidates"). Conservative and deterministic.
   - **Require explicit declaration when ambiguous.** If the SONAME has
     multiple providers and the recipe didn't declare any of them, log
     a coverage gap (don't auto-include); require the recipe author to
     pick.

   This is a real gap in the design; it should be addressed before
   implementation. **Recommended:** the third option (require explicit
   declaration on ambiguity), which preserves auto-include's "fix the
   common case" property without giving the SONAME index a deterministic
   handle on which library is silently chained.

3. **SONAME shadowing system libraries (low).** A library recipe could
   declare `outputs = ["lib/libc.so.6"]` (or `libpthread.so.0`,
   `libdl.so.2`, etc.). If the bottle's NEEDED list references
   `libc.so.6`, the system-library check normally short-circuits ("yes,
   the system provides this — no action"). But the design's
   classification order matters here: if the system check is
   "resolves via ldconfig", a tsuku-shipped libc would not be selected
   (system shadows it, by design). Inverted: if the system check is
   skipped or fails, a malicious library recipe could insert itself
   into the chain for libc-class libraries.

   Mitigation: keep the system-library check as the **first** filter
   (per the design's step-3 order); ensure the check uses the runtime
   linker's actual resolution behavior (e.g., `ldconfig -p`), not a
   tsuku-internal allowlist that could drift. A defensive deny-list of
   universally-system SONAMES (`libc.so.*`, `libpthread.so.*`, `ld-*`,
   `libm.so.*`, `libdl.so.*`) at the SONAME-index parser would close
   the gap even if the runtime check were ever bypassed. **Severity: low.**

### NEW: Auto-include path
**Applies:** Yes

When the SONAME scanner finds an under-declared SONAME, it auto-includes
the provider's `lib/` dir in the chained RPATH. This subtly shifts the
trust boundary: a malicious bottle could declare NEEDED entries for
SONAMES that map to tsuku libraries the recipe author didn't pick.

**Concrete scenario.** A bottle for the recipe `tool-x` declares
`runtime_dependencies = ["openssl-3"]` and ships a binary whose NEEDED
list is `[libssl.so.3, libfoo.so.1]`. The SONAME index has a mapping
`libfoo.so.1 → recipe foo`. Auto-include adds `$TSUKU_HOME/libs/foo-N/lib`
to the RPATH. The recipe author never asked for `foo`, but the bottle
caused it to be chained. If `foo` is a recipe an attacker controls or
that contains an exploitable lib, the binary now loads attacker-influenced
code.

But this scenario requires (a) a compromised bottle (already outside our
trust model — bottles are sha256-pinned to homebrew/core) **and** (b) a
compromised tsuku library recipe (also outside our trust model — recipe
registry is the trust boundary). Both ends are already-trusted; the
auto-include path doesn't widen the boundary.

What auto-include **does** introduce is a new way for a recipe-author bug
(not malice) to produce an unexpected install: a typo in a library
recipe's `outputs` list could cause that library to advertise itself as
the provider of a popular SONAME, which then auto-includes that library
into many tools' chains. The fix is the SONAME-index validation in the
previous section — reject malformed `outputs` entries, flag collisions,
require explicit declaration when ambiguous.

**Severity: low** under the existing trust model. The mitigations from
the SONAME-index dimension cover the bug-rather-than-malice cases.

The design should also document that **`--strict` mode disables
auto-include** (or equivalently, treats any auto-include event as a hard
error). This makes the strict invariant "no chain entries beyond what
the recipe declared" auditable in CI: if the strict-mode install passes,
the recipe is fully self-described; the auto-include surface is closed.

### NEW: `--strict` mode
**Applies:** Yes

`--strict` promotes the under-declaration warning to a hard error. Two
small concerns:

1. **DoS via aggressive errors blocking installs (low).** Without
   `--strict`, the auto-include path keeps installs working when recipes
   are under-declared. With `--strict`, an under-declared recipe fails
   the install. If `--strict` were ever made the default, the existing
   under-declared recipes (`git`, `wget`, `coreutils`, plus the 316
   auto-generated batch recipes) would all break. The design should
   explicitly state `--strict` is opt-in and that the default is
   warn-and-auto-include. Phase 5's wording says "Optionally, in
   `--strict` mode" which is fine; the validator additions section
   says "In `--strict` mode, [under-declared] is treated as a hard
   failure. In default mode it's a warning + auto-include." This is
   correctly specified.

2. **`--strict` should also gate on "no auto-includes happened",
   not just "no warnings".** Suggested above. As written the design
   uses "under-declaration warning → hard error" — auto-include and
   warning are emitted together for the same event, so the two
   formulations are equivalent. Fine as specified; calling out the
   equivalence in the security text would help future maintainers.

### Defense-in-depth at patch time
**Applies:** Yes

The previous review's recommendation (compute the relative RPATH via
`filepath.Rel` after `EvalSymlinks`, then verify
`filepath.Join(loaderDir, relPath)` resolves back into
`$TSUKU_HOME/libs/`) is now in the design at Phases 3 and 4 and in the
Mitigations section. The wording is correct:

> After `filepath.Rel`, verify that `filepath.Join(loaderDir, relPath)`
> resolves back inside `$TSUKU_HOME/libs/` — fail the install with a
> clear error if not (defense in depth against any path-traversal that
> slipped past the validator).

**One small gap.** The design says "fail the install with a clear error
if not." It doesn't say what happens to the binary that was already
partially patched when the failure fires — the existing `fixMachoRpath`
already wrote `@rpath`-prefixed install_names before the chain step
runs. A failure here leaves the bottle in an intermediate state; the
caller (homebrew action's relocate phase) needs to either:
- Validate **all** dep entries before patching any, or
- Treat the work_dir as disposable and fail the install cleanly.

The second is the existing behavior of the homebrew action (work_dir is
a temp dir; partial patching never reaches `$TSUKU_HOME` because
`install_binaries` runs after relocate). So the gap is bounded by
existing invariants, but the security text should call it out: validate
every chain entry's resolved path before invoking `patchelf` /
`install_name_tool` for any of them, so failures don't leave half-patched
binaries in a partially-trusted state. **Severity: low.**

### Permission scope
**Applies:** Yes

Unchanged from the previous review. The new SONAME scanner runs
`readelf` / `otool` (read-only on bottle binaries), reads the SONAME
index (in-memory map), and feeds results into the chain walk. No new
write paths beyond the existing patchelf / install_name_tool invocations
on bottle binaries inside `$TSUKU_HOME/{tools,libs}/<recipe>-<version>/`.
No sudo, no system files.

### Data exposure
**Applies:** No

Unchanged. `$ORIGIN` / `@loader_path`-relative RPATHs do not embed
`$TSUKU_HOME` or any user-specific data. The SONAME index is
in-memory at plan generation; no persistence, no logging of file
contents. Coverage-gap log lines reference SONAMES (which are public
recipe outputs) and recipe names (also public).

### Project-specific (public-repo conventions)
**Applies:** Yes

Design and Security Considerations text follow the public-repo conventions
(no internal references, no competitor names, professional tone).
Compliant.

## Verdict

**Findings to address before plan.** Three concrete gaps need to be added
to the design before implementation; all are small additions, none
require a new design loop.

1. **SONAME-index validation (medium).**
   - **Description.** The new `internal/install/soname_index.go` parses
     library recipes' `outputs` lists for `lib/lib*.so.*` and
     `lib/lib*.*.dylib` patterns. The design doesn't specify that the
     parser must reject `outputs` entries outside `lib/`, entries
     containing `..`, or entries that don't match the SONAME basename
     shape. A malicious or buggy library recipe could inject arbitrary
     mappings into the index.
   - **Mitigation.** Add to Phase 2: "The parser rejects any `outputs`
     entry whose path is not exactly under `lib/` (must start with
     `lib/`, no `..` segments). The basename extracted as the SONAME
     must match `^lib[a-zA-Z0-9._+-]+\.(so|dylib)(\.[0-9.]+)?$`. Entries
     that fail validation are dropped with a warning at index-build
     time." Mirror the recipe-name validator pattern (Phase 1) for
     consistency.
   - **Where to add.** Solution Architecture → Components row for the
     SONAME index module; Phase 2 acceptance criteria; Security
     Considerations → Mitigations as a fourth bullet.

2. **SONAME-collision policy (medium).**
   - **Description.** Two library recipes can claim the same SONAME
     (legitimately or via typo/malice). The design says the index maps
     `SONAME → providing recipe + version` (singular) without specifying
     collision behavior. Silent overwrite would make the auto-include
     non-deterministic.
   - **Mitigation.** Specify: when multiple library recipes claim the
     same `(platform, SONAME)` pair, the index records all candidates.
     Auto-include only fires when exactly one candidate exists OR when
     one of the candidates is already in the recipe's
     `runtime_dependencies`. If multiple candidates exist and none are
     declared, log a coverage gap listing the candidates and skip
     auto-include — the recipe author must declare the dep explicitly.
   - **Where to add.** Decision 4's Chosen-option spec (step 3, after
     "look it up in soname_index"); Phase 2 acceptance ("collisions
     are surfaced; tests assert deterministic behavior on overlapping
     SONAMES"); Security Considerations text as part of the SONAME
     index mitigation.

3. **All-or-nothing chain validation (low).**
   - **Description.** The defense-in-depth `filepath.Join` post-check is
     specified per-entry, not as a pre-pass over all entries. A failure
     mid-chain would leave the bottle partially patched in the temp
     work_dir. This is bounded by the existing "work_dir is disposable"
     invariant, but the security text should make the invariant
     explicit so a future refactor doesn't regress.
   - **Mitigation.** Add to Phases 3 and 4: "Validate every chain
     entry's `filepath.Join(loaderDir, relPath)` post-check **before**
     invoking `patchelf` / `install_name_tool` for any entry. A single
     failed entry fails the entire chain step; no partial patching."
     Add a parallel sentence to the Security Considerations Mitigations
     bullet 2: "Validation is performed for all entries before any
     patching call, so a failure leaves the work_dir untouched."
   - **Where to add.** Phases 3 and 4 acceptance; Security
     Considerations Mitigations bullet 2.

## Out of scope but flagging

- **Version-string validation at provider boundaries.** `depVersion` is
  interpolated into the same path as `depName`. A version-provider bug
  that produced `../foo` as a version string would have the same
  consequence as a malicious dep name. The Phase 3/4 `filepath.Join`
  defense-in-depth catches it as a structural backstop, but a version
  validator at the provider boundaries (PyPI, npm, GitHub releases) is
  a cleaner long-term fix. Track separately; not blocking this design.

- **`tsuku doctor` RPATH validation.** Already noted in the design's
  Consequences as a follow-up. A user-facing way to inspect a binary's
  RPATH chain and confirm it resolves cleanly inside `$TSUKU_HOME/libs/`
  would surface drift after `$TSUKU_HOME` moves and accidental
  recipe-author errors. Not blocking; natural Phase 8 follow-up.

- **Authoring missing library recipes (Phase 7).** The design correctly
  scopes the work of authoring `libuuid`, `libacl`, `libattr`, etc. as
  out of scope. These coverage gaps will be surfaced by the SONAME
  scanner; closing each is a separate library-recipe PR. Confirmed
  appropriate scoping.

## YAML summary

```yaml
outcome: findings_to_address_before_plan
findings_count: 3
severity_max: medium
report_file: /home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/wip/research/design_tsuku-homebrew-dylib-chaining_phase5_security.md
```
