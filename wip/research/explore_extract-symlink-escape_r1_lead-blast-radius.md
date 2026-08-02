# Lead: What is the blast radius of making `extract`'s containment checks stricter?

## Findings

### 0. Method

No archive fixtures exist anywhere in the repo (`find` for `*.tar.gz|*.tgz|*.zip|*.tar|*.tar.xz|*.tar.bz2|*.tar.zst` under the worktree returns nothing). Instead I surveyed the machine's real tsuku download cache, `~/.tsuku/cache/downloads` — the actual bytes tsuku has fed to `ExtractAction` on this host. Nothing was downloaded. 146 archives were listed successfully; 89 contain at least one symlink; 49,590 symlink entries total. Scratch scripts live in `/home/dgazineu/.claude/jobs/1013267d/tmp/` (`survey.sh`, `classify2.py`, `through.py`).

### 1. Who actually reaches `extract`

Only 18 registry recipes name the action directly (`recipes/{a/apr,a/awscli,b/boundary,c/cargo-nextest,c/consul,c/curl,h/helm,l/libcurl-source,n/ncurses,n/nomad,o/ollama,p/packer,p/pcre2,r/rbenv,t/terraform,v/vagrant,v/vault,w/waypoint}.toml`), but that badly understates reach. `ExtractAction` is instantiated by three composites and emitted as a plan primitive by four:

| Caller | Execute path | Decomposed step |
|---|---|---|
| `download_archive` / `github_archive` | `internal/actions/composites.go:272`, `:612` | `:381`, `:724` |
| `homebrew` (GHCR bottles) | `internal/actions/homebrew.go:115` | `:560` |
| `fossil_archive` | `internal/actions/fossil_archive.go:93` | `:156` |

`extract` is registered as a primitive at `internal/executor/plan.go:177` and `internal/actions/decomposable.go:141`. Practically every binary-distribution recipe in the 1,449-recipe registry goes through it.

**The zip path has no symlink branch at all.** `extractZip` (`internal/actions/extract.go:390-462`) handles only `IsDir()` and everything-else-as-regular-file; a zip entry with `os.ModeSymlink` in its mode is written as a regular file whose contents are the target string. So the entire symlink question — both #2473 and #2275 — is tar-only. Seven of the 18 direct-`extract` recipes are zip (terraform, vault, consul, packer, boundary, waypoint, vagrant, awscli), and none of them can currently carry a symlink through extraction.

### 2. Which real archives contain symlinks, and of what shape

Classifying all 49,590 entries lexically against the **archive root** (before `strip_dirs`):

| Shape | Count |
|---|---|
| Relative, no `..`, stays inside | 44,355 |
| Relative, contains `..`, still lands inside root | 5,235 |
| Absolute (`/usr/lib/...`, `/etc/passwd`) | **0** |
| Escapes the archive root | **0** |
| `@@HOMEBREW_PREFIX@@`-style placeholder in the linkname | **0** |

That last row matters: Homebrew's `@@HOMEBREW_PREFIX@@` placeholders live in *file contents* (which `homebrew_relocate` rewrites), not in tar linknames. Nothing in the symlink path ever sees a placeholder.

Re-running the classification with each recipe's **actual** `strip_dirs` value, **zero archives in the cache produce an escaping symlink**. The interesting near-misses:

- **node** (`internal/recipe/recipes/nodejs.toml:28`, `strip_dirs = 1`) — `node-v26.5.1-linux-x64/bin/npm -> ../lib/node_modules/npm/bin/npm-cli.js` and the same for `npx`. These escape at `strip_dirs >= 2`; at 1 they resolve to `lib/node_modules/...`, inside.
- **Homebrew bottles** (`internal/actions/homebrew.go:117`, hardcoded `strip_dirs = 2`) — the git bottle has 154 symlinks, 147 of the form `git/2.54.0/libexec/git-core/git-add -> ../../bin/git`; the make bottle has `make/4.4.1/libexec/gnubin/make -> ../../bin/gmake` and `make/4.4.1/libexec/gnuman/man1/make.1 -> ../../../share/man/man1/gmake.1`; openjdk has 243, including `openjdk/25.0.2/bin/java -> ../libexec/openjdk.jdk/Contents/Home/bin/java`. All escape at `strip_dirs >= 3`, all land inside at 2.
- **JDK tarballs** (Adoptium, BellSoft Liberica) — ~205 links each of the form `jdk-25.0.3+9/legal/jdk.jshell/LICENSE -> ../java.base/LICENSE`. Escape at `strip_dirs >= 3`.
- **ollama** (`recipes/o/ollama.toml:27`, `strip_dirs = 1`, `files = ["ollama"]`) — 36 symlinks including `lib/ollama/mlx_cuda_v13/libcublas.so.13.1.1.3 -> ../cuda_v13/libcublas.so.13.1.1.3`. Escape only at `strip_dirs >= 3`, and the `files` filter drops them anyway.
- **git source tarball** — `git-2.55.0/subprojects/git-gui -> ../git-gui` and `-> ../gitk-git`: symlinks **to directories**, escaping at `strip_dirs >= 2`.

Dangling links: checked node, the git bottle, both openjdk bottles, and ollama by cross-referencing every linkname against the archive's own entry list. **Zero dangling** — every target is materialized by the same archive.

### 3. The #2275 shape, and why it is absent from the cache

Issue #2275 quotes `libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14`. That path is *post*-strip; in the tarball it is `awscli/<ver>/libexec/bin/python3.14`, and five `..` from `awscli/<ver>/libexec/bin/` reaches two levels **above** the archive root. So this one escapes even at `strip_dirs = 0` — it is a genuinely-outside-pointing, **dangling** link into the Homebrew opt prefix. It is missing from my sample because this is a Linux host and the cache holds only 16 formulas, none of them Python-bearing: bazel, curl, expat, gcc, git, libcap, libevent, libyaml, make, openjdk, openjdk@21, openssl@3, pcre2, pkgconf, socat, zlib.

The two issues are cleanly separable along exactly the axis the proposed rule uses. **#2275 is about where a link points. #2473 is about what gets written through a link.** Nothing in #2275's bottle is ever written through `libexec/bin/python3.14`.

### 4. What each candidate rule would break

| Rule | Observed breakage in 146 real archives | Notes |
|---|---|---|
| *(today)* reject absolute linkname — `extract.go:41` | 0 | No cached archive has an absolute linkname at all. |
| *(today)* lexically-resolved target must stay inside `destPath` — `extract.go:39-55` | 0 in cache, **but breaks #2275** | The bug being fixed and the bug being requested share this line. |
| `EvalSymlinks(target)` must stay inside `destPath` | Breaks every dangling link, i.e. **re-breaks #2275** and adds fragility to tar-ordering (a link declared before its target is transiently dangling) | Strictly worse than today. |
| **No archive entry may be written through a symlinked path component** | **0 of 146** | Measured directly, see below. |

I checked the last row empirically: for every archive, walk entries in tar order, remember each symlink location, and flag any later entry whose path starts with a remembered symlink location. Result: `scanned 146 archives; 0 contain write-through-symlink entries`. Not one real archive relies on the behavior #2473 exploits.

This is the blast-radius headline. The rule "a symlink may point anywhere, but no entry may be written through a symlinked path that leaves the destination" has **zero observed impact on real archives**, and the weaker form "no entry may be written through a symlinked path *at all*" also has zero observed impact — so there is room to pick whichever is simpler to implement without paying a compatibility cost. Note that `install_program_files.go:147,177` already establishes the `syscall.O_NOFOLLOW` precedent in this package (with no build tags, so the codebase is already unix-only in practice).

### 5. CI surfaces

**Golden plans do not gate this file.** `.github/workflows/validate-golden-code.yml` says so explicitly:

```
# Note: The following files do NOT trigger this workflow because they contain
# only execution logic (Execute methods) and do not affect plan generation:
#   - internal/actions/extract.go
```

The files that ARE on that workflow's `paths:` allowlist: `cmd/tsuku/eval.go`, `internal/executor/{plan_generator,plan,plan_conversion}.go`, `internal/actions/{decomposable,action,composites,download}.go`, `internal/recipe/{types,loader,platform}.go`, `internal/actions/{homebrew,cargo_install,npm_install,pipx_install,gem_install,go_install,nix_install,fossil_archive,apply_patch}.go`, `internal/version/*.go` (minus tests), the workflow file itself, and `testdata/golden/code-validation-exclusions.json`. Touching only `extract.go` + `extract_test.go` skips golden-code validation entirely. Touching `homebrew.go` (e.g. to change the bottle `strip_dirs`) does **not** skip it.

**`validate-golden-execution.yml`** triggers only on `testdata/golden/plans/embedded/**`, `recipes/**/*.toml`, the exclusions JSON, and two scripts — also not on `extract.go`.

**Unit tests run `-short` on PRs** (`.github/workflows/test.yml`, job `unit-tests`):

```yaml
      - name: Run tests
        # PRs: fast feedback without race detector or coverage
        # Push to main: full validation with race detector and coverage
        run: |
          if [ "${{ github.event_name }}" = "push" ]; then
            go test -short -race -coverprofile=coverage.out ./...
          else
            go test -short ./...
          fi
```

`-short` is passed in **both** branches. No test in `internal/actions/*_test.go` calls `testing.Short()`, so every extract test runs on PRs regardless.

**The unit-test job does fail on a dirty tree.** Verbatim:

```yaml
      - name: Check for test artifacts
        run: |
          # Fail if tests left any files in the source tree
          if [ -n "$(git status --porcelain)" ]; then
            echo "::error::Tests left artifacts in the source tree. Tests must clean up after themselves."
            echo "Changed files:"
            git status --porcelain
            exit 1
          fi
```

A regression test that builds a malicious tarball must write it under `t.TempDir()`, and — critically for #2473 — the *escape* it provokes lands in `dest`'s **parent**, so the parent must also be a temp dir, not the repo.

**Lint** is a separate job, `lint-tests`, running `go test -v -run 'Test(GolangCILint|GoFmt|GoModTidy|GoVet|Govulncheck)' .`. Note `lint_test.go:15-19` skips golangci-lint under `-short`, so the `unit-tests` job never lints; only `lint-tests` does.

From `.golangci.yaml`:

- `errcheck` is enabled with **no `tests: false`** — test files are linted. The `exclude-functions` list is: `(*os.File).Close`, `(io.Closer).Close`, `(*compress/gzip.Reader).Close`, `(*compress/gzip.Writer).Close`, `(*archive/tar.Reader).Close`, `(*archive/zip.ReadCloser).Close`, `(io.ReadCloser).Close`, `(*io.ReadCloser).Close`, `os.Remove`, `os.RemoveAll`, `(*github.com/gofrs/flock.Flock).Close`, `os.Setenv`, `fmt.Fprintf`. **`os.WriteFile`, `os.MkdirAll`, and `os.Symlink` are not excluded** — a fixture-building test must check every one of them. The only `issues.exclusions.presets` are `comments` and `std-error-handling`, neither of which covers those three.
- `dupl` threshold is `250` tokens. Relevant because the tar and zip loops in `extract.go` are already near-duplicates; adding a parallel containment helper to both could trip it.
- `gosec` excludes `G305` with the comment `# Package manager must extract archives - path traversal is validated in code`. That comment is currently false in the way #2473 describes; the fix makes it true, and the comment is worth revisiting in the same PR.

**Which jobs actually extract something on a PR touching `extract.go`:**

| Workflow / job | Triggers on `**/*.go`? | Exercises extraction |
|---|---|---|
| `test.yml` → `unit-tests` | yes | `internal/actions/extract_test.go` (873 lines, ~14 symlink/traversal cases) |
| `test.yml` → `integration-linux` | yes | real installs: actionlint, btop, argo-cd, bombardier, golang, **nodejs**, ruff, perl, waypoint-tap |
| `test.yml` → `integration-macos` | yes | actionlint, argo-cd, golang, **nodejs**, iterm2 (cask/app_bundle) |
| `test.yml` → `functional-tests` | yes | `make test-functional-critical` on PRs |
| `sandbox-tests.yml` → `sandbox-linux` | yes | same matrix, containerized, via `tsuku eval` + `install --plan --sandbox` |
| `integration-tests.yml` → `checksum-pinning` | yes | debian/rhel/arch/alpine/suse |
| `test.yml` → `validate-recipes` | only on recipe/validator changes | `tsuku validate --strict` only — no extraction |
| `container-tests.yml` | **no** — its PR `paths:` is only `.github/workflows/container-tests.yml` | runs on push to main only |

nodejs is the one CI-matrix tool whose archive carries symlinks (`bin/npm`, `bin/npx`). **No Homebrew-bottle recipe is in the CI matrix**, so the bottle path — the one #2275 is about — is not covered by any PR-triggered job. Bottles are exercised only through `internal/builders/homebrew_test.go` and the nightly/scheduled workflows.

### 6. Plugin skills

Root `CLAUDE.md:136-162` is the "Plugin Maintenance" table. The relevant row:

```
| internal/actions/ -- action names, params, `Dependencies()` | New or renamed actions, changed parameters | tsuku-recipe-author |
```

with the standing instruction at `:146` — *"After completing any source change in the areas below, assess the relevant skills"* — and the two checks: broken contracts, and new surface ("does this change add behavior that no skill mentions? If so, update the relevant skill in the same PR").

Here is the awkward part: **`extract` has no section anywhere in the recipe-author skill.** `references/action-reference.md` headings are `Step Phases`, `Download and Archive Composites`, `Ecosystem Composites`, `Build System Primitives`, `Homebrew`, `System Package Managers`, `File Operations`, `Shell Integration`, `Special Actions`. `extract` appears only as a decomposition target — `action-reference.md:69` and `:169` both read *"Decomposes to: download_file + extract + install_binaries"* — plus `strip_dirs` as a `github_archive`/`download_archive` parameter at `:62`, and TOML examples at `platform-reference.md:126-128` and `dependencies-reference.md:94-95`. `SKILL.md:30-38` lists the composites in a table but not the primitive. There is **no documented statement anywhere in `plugins/` of what archives `extract` accepts or how it treats symlinks.**

Files that would need touching if the accepted-archive set changes:

| File | Section | Why |
|---|---|---|
| `plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md` | new subsection under `## File Operations` (line 404) | the only reference doc with per-action parameter tables; `extract` is the sole primitive missing one |
| `plugins/tsuku-recipes/skills/recipe-author/SKILL.md` | `### Download and Archive` table, lines 30-38 | add the `extract` row so authors can find the reference entry |
| `plugins/tsuku-recipes/skills/recipe-test/SKILL.md` | `## Common Failure Patterns`, the exit-code-6 paragraph at line 126 | today "bad archive" is the entire diagnosis; a new rejection class needs its own bullet |
| `docs/guides/GUIDE-actions-and-primitives.md` | line 22, `| extract | Decompress archives (tar, zip, gzip, etc.) | Fully deterministic |` | repo-side twin of the same claim |

Draft text for the new `action-reference.md` subsection, assuming the rule is *"a symlink may point anywhere, but no archive entry may be written through a symlinked path that leaves the destination"* (**drafted, not applied**):

> ### extract
>
> Unpacks a downloaded archive into the work directory. Composites emit this
> step; recipes name it directly when they need to extract without installing
> binaries in the same step.
>
> | Parameter | Type | Required | Description |
> |-----------|------|----------|-------------|
> | `archive` | string | Yes | Archive filename in the work dir |
> | `format` | string | Yes | `tar.gz`, `tar.xz`, `tar.bz2`, `tar.zst`, `tar.lz`, `tar`, `zip`, or `auto` |
> | `dest` | string | No | Destination relative to work dir (default: work dir) |
> | `strip_dirs` | int | No | Leading path components to strip (default: 0) |
> | `files` | []string | No | Extract only these paths, matched after `strip_dirs` |
>
> **Symlinks.** Tar symlink entries are recreated as symlinks; the target is
> stored verbatim, so a link may point outside the extraction directory or at a
> path the archive never ships. Homebrew bottles rely on this — a bottle's
> `libexec/bin/python3.14` points up and across into the Homebrew opt prefix,
> and the link is meant to dangle until relocation runs.
>
> What extraction refuses is *writing through* a symlink. Once an entry has
> created a symlink, no later entry may be written along a path that passes
> through it and lands outside the destination. An archive that stages
> `b -> a/..` and then writes `b/payload` is rejected, because the write would
> land in the destination's parent. Legitimate archives do not do this: the
> link and the file it shadows are always both inside the tree.
>
> Zip archives carry no symlinks — a zip entry marked as a symlink is written as
> a regular file whose contents are the target path.

And for `recipe-test/SKILL.md`, a new bullet beside the existing exit-code-6 paragraph:

> **"archive entry would be written through a symlink" during extraction.** The
> archive creates a symlink and then writes a file underneath it, landing outside
> the extraction directory. Almost always a malformed or tampered upstream
> archive rather than a recipe bug. Re-download with `--force` to rule out a
> corrupted cache entry; if it reproduces, check the upstream release against its
> published checksum before touching the recipe.

### 7. Error surface

The path from an action error to the user is short and unstyled. `internal/executor/executor.go:620` and `:625` wrap with `fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)`; `cmd/tsuku/install.go:635` prints `fmt.Fprintln(os.Stderr, "Error:", err.Error())`; the process exits `6` (`ExitInstallFailed`, `cmd/tsuku/exitcodes.go:27`). So today a user whose bottle trips `validateSymlinkTarget` sees exactly what #2275 quotes:

```
Error: step 2 (extract) failed: symlink target escapes destination directory
```

No hint about which archive, no suggested action, no doc link. If extraction starts rejecting an archive that used to work, that is the entire user experience.

Two things worth knowing about that string. First, telemetry classifies errors by substring in a fixed order (`internal/telemetry/event.go:519-541`), and `strings.Contains(msg, "extract")` is tested **before** `strings.Contains(msg, "symlink")`. Because the executor's wrapper injects the literal action name `extract` into every message, symlink rejections from this action already bucket as `ErrorTypeExtractionFailed`, never `ErrorTypeSymlinkFailed`. Whatever wording a new error uses, its taxonomy bucket is decided by the wrapper, not by the message — so no telemetry change is needed, and none is possible without changing that ordering.

Second, there is no user-facing doc that would need to mention a new rejection. `docs/runbooks/` holds only `batch-operations.md`; `docs/guides/GUIDE-troubleshooting-verification.md` covers verification, not extraction. The only prose in the repo describing what a failed extraction looks like is `plugins/tsuku-recipes/skills/recipe-test/SKILL.md:126` — *"Exit code 6 -- installation failed. The install step itself failed (bad archive, build error, container failure in sandbox mode)."* That sentence is the whole runbook, and it is in a plugin skill rather than in `docs/`.

## Implications

1. **The compatibility risk in #2275 is real but narrow, and orthogonal to #2473.** #2275 is a rule about link *targets*; #2473 is a rule about *writes*. A fix framed as "links may point anywhere; writes may not traverse them out of the destination" resolves both at once and costs nothing measurable — 0 of 146 real archives write through a symlink. A fix framed around `EvalSymlinks` on the target instead of on the write path would re-break #2275 and add ordering fragility.
2. **Nothing about this file is golden-gated.** `extract.go` is on `validate-golden-code.yml`'s explicit exclusion list, and `validate-golden-execution.yml` never triggers on Go changes. The PR gates are unit tests, lint, and the real-install integration/sandbox matrices.
3. **The CI matrix will not catch a bottle regression.** No Homebrew recipe is in `test-matrix.json`'s `ci.linux` or `ci.macos` lists. nodejs is the only matrix tool whose archive carries symlinks. If the fix breaks bottles, PR CI stays green and the failure lands in nightly or in a user's install. A table-driven unit test over synthetic bottle-shaped archives is the only affordable coverage here.
4. **The regression test has two lint and one CI trap.** `errcheck` covers test files and does not exclude `os.WriteFile`/`os.MkdirAll`/`os.Symlink`; `dupl` fires at 250 tokens across the near-identical tar and zip paths; and the `git status --porcelain` gate means the *escape target* — which by construction lands outside `dest` — must still be inside a `t.TempDir()`.
5. **Documentation debt gets called due by this change.** The plugin-maintenance rule in `CLAUDE.md:146-149` asks whether a change "adds behavior that no skill mentions". `extract`'s symlink behavior is not mentioned by any skill today, so a "no new surface" answer is not available — the honest answer is that the surface was always undocumented and this PR is where it gets written down.
6. **Zip's silent symlink-flattening is a latent inconsistency.** Seven of the direct-`extract` recipes are zip. If any future recipe depends on a symlink in a zip it will get a text file, not a link — no error, no warning. Worth a sentence in whatever doc gets written, even if the code is left alone.

## Surprises

- **Zero archives — none of 146 — write an entry through a symlink.** I expected at least a handful of framework-style layouts (macOS `.framework`, `Versions/Current`) to do it. The proposed strict rule appears to cost literally nothing in compatibility.
- **Zero absolute symlink targets anywhere**, across 49,590 entries. The `filepath.IsAbs` rejection at `extract.go:41` — the check the tests spend the most cases on — has never fired on a real archive on this host.
- **`@@HOMEBREW_PREFIX@@` never appears in a linkname.** I expected relocation placeholders in link targets; they are exclusively in file contents. This means `homebrew_relocate` cannot be part of a "re-validate after relocation" strategy the way #2275's suggested direction implies.
- **The zip extractor has no symlink handling at all**, and nothing anywhere documents that.
- **`container-tests.yml` will not run on this PR.** Its `pull_request:` trigger lists exactly one path, its own filename — so the sandbox integration tests it guards only ever execute on push to main.
- **Symlink errors already telemeter as `extraction_failed`**, not `symlink_failed`, purely because the executor's wrapper puts the word "extract" into the message before the classifier sees it.

## Open Questions

1. Does any **macOS** bottle in the CI-relevant set carry a symlink that escapes at `strip_dirs = 2`? My cache is Linux-only and contains no Python-bearing formula, so #2275's exact shape is unverified against a real file. Fetching one macOS bottle (awscli or python@3.14) would settle it; I did not, per the no-download instruction.
2. Is the intended rule "no write through a symlink that **leaves** the destination" or the simpler "no write through a symlink **at all**"? Both measure zero breakage here. The stricter form is easier to implement correctly (no resolution needed, just `Lstat` the parent chain or use `O_NOFOLLOW` per component) and easier to prove.
3. Should the absolute-linkname rejection at `extract.go:41` survive the change? Under a write-path rule it is redundant, and dropping it would let a bottle ship `/opt/homebrew/...`-style absolute links — which #2275's framing arguably wants. But dropping it also deletes the behavior four existing tests assert.
4. Do `install_program_files`'s `syscall.O_NOFOLLOW` and `EvalSymlinks` (`internal/actions/install_program_files.go:119-177`) constitute the house pattern the fix should copy, or was that a one-off for an action that could afford it? That is the #2467 lead's territory, but it directly determines whether `extract.go` needs an `openat`-style rewrite or a narrower per-entry parent check.
5. Nothing gates plugin-skill freshness in CI (`validate-skill-content.yml` exists — I did not read it). Is the plugin update enforced, or purely reviewer discipline under the `CLAUDE.md` rule?

## Summary

Across 146 real archives from the local tsuku download cache — 89 with symlinks, 49,590 symlink entries — not one absolute link target, not one link escaping at the `strip_dirs` its recipe actually uses, and, decisively, **not one archive that writes an entry through a symlink**, so the proposed "links may point anywhere, but no write may traverse one out of the destination" rule has zero measured compatibility cost while resolving #2275 and #2473 together. The gates that matter are unit tests plus the real-install integration and sandbox matrices — `extract.go` is on `validate-golden-code.yml`'s explicit exclusion list and no Homebrew recipe is in the CI matrix at all, so a bottle regression would pass PR CI silently; the regression test must also survive `errcheck` on `os.WriteFile`/`os.MkdirAll`/`os.Symlink` in test files, `dupl` at 250 tokens, and a `git status --porcelain` cleanliness gate that the escape itself could trip. The biggest open question is #2275's exact shape: it is a macOS Python bottle and my Linux-only cache does not contain one, so the one symlink the fix must keep working is the one I could not observe first-hand.
