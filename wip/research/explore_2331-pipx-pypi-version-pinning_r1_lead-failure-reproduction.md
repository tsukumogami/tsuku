# Failure reproduction: ansible-core and azure-cli via pipx_install

Research lead L3 report for /shirabe:explore on issue #2331.

## Setup

- **Build**: `make build-test` succeeded; produced `./tsuku-test` (CGO_ENABLED=0 with `.tsuku-test` home override). No fallback needed.
- **Recipes reconstructed from**: git history. The reverted recipes still exist on commits `dcb34719` (initial) and `ed8fc646` (ansible→ansible-core, azure-cli verify mode change). `git log --all --oneline -- recipes/a/ansible.toml` shows three commits: `dcb34719` (add) → `ed8fc646` (fix) → `511cd640` (delete and defer). Reverted versions match the issue body's description.
- **PR #2329 note**: Despite the issue body referencing PR #2329, that PR is the argocd-only backfill — its files list does not contain ansible.toml or azure-cli.toml. The actual ansible/azure-cli code landed in `dcb34719` and was deleted in `511cd640`. The `gh pr view 2329` body confirms ansible+azure-cli were "split into follow-ups" (#2331) — the issue body's "PR #2329 (both subsequently reverted)" is imprecise; the recipes were committed to `main` directly and reverted in `511cd640`.
- **Recipes saved to**: `/tmp/recipes-2331/ansible.toml` and `/tmp/recipes-2331/azure-cli.toml` (using the post-fix `ansible-core` package and `mode = "output"` azure-cli verify, which is what the issue body describes as the state at deferral time).
- **PyPI direct fetch blocked**: `curl https://pypi.org/...` is blocked by the workspace `gate-online` hook. The pip-download stderr below is itself the canonical PyPI metadata answer to "which versions support Python 3.10" — every version pip rejected is enumerated with its `Requires-Python` declaration, and every available version is enumerated in the "from versions:" list. No external fetch needed.

## ansible (package = ansible-core)

### Command

```
./tsuku-test eval --recipe /tmp/recipes-2331/ansible.toml --os linux --arch amd64
```

### Output (verbatim, abridged)

Stderr (single error message, exit code 1):

```
Error: failed to generate plan: failed to resolve step pipx_install: failed to decompose pipx_install: failed to decompose "pipx_install": failed to generate locked requirements: pip download failed: exit status 1
Output: ERROR: Ignored the following yanked versions: 2.14.0rc1.post0
ERROR: Ignored the following versions that require a different python version: 2.18.0 Requires-Python >=3.11; 2.18.0b1 Requires-Python >=3.11; ...
[truncated — every 2.18.x, 2.19.x version requires Python >= 3.11; every 2.20.x and 2.21.0 prerelease requires Python >= 3.12]
ERROR: Could not find a version that satisfies the requirement ansible-core==2.20.5 (from versions: 2.13.0b0, ..., 2.17.14)
[notice] A new release of pip is available: 26.0.1 -> 26.1
ERROR: No matching distribution found for ansible-core==2.20.5
```

The full "Ignored the following versions" list spans 2.18.0 through 2.21.0rc1 — every version pinned to Python >= 3.11 or >= 3.12. The "from versions" list of versions actually compatible with Python 3.10 ends at **2.17.14** (the last 2.17.x release).

### Failure layer

Layer: **decompose**, before plan generation.

The error chain is `eval → generate plan → resolve step → decompose pipx_install → generate locked requirements → pip download exit 1`. Source: `internal/actions/pipx_install.go:283` (`Decompose`) calls `generateLockedRequirements` (`pipx_install.go:402`) which calls `runPipDownloadWithHashes` (`pipx_install.go:460`); the wrapping error at line 333 is "failed to generate locked requirements". Eval never produces a plan JSON.

The version provider step (PyPI provider resolving "latest" → `2.20.5`) succeeded. The failure is one layer down: pip itself, run inside the bundled CPython 3.10.20, refuses every recent ansible-core release.

### Match to issue description

Issue claim: "ansible-core 2.20.5 (latest) requires Python >= 3.12 ... pip download rejects every 2.18+ version with `Requires-Python >=3.11/3.12`. Without a way to constrain to the 2.17.x line ... the recipe cannot resolve."

**Confirmed precisely.** The pip output enumerates exactly the versions the issue describes: 2.18.x and 2.19.x require >= 3.11; 2.20.x and 2.21.0 prereleases require >= 3.12; the resolvable ceiling under Python 3.10 is 2.17.14.

The issue also claims "in the test-recipe sandbox the install crashed before producing a result JSON (`exit null` on every glibc Linux family)" — this refers to a previous run with `package = "ansible"` (the meta-package, not `ansible-core`). I did not reproduce the sandbox install, but the eval-level pip-download failure is the same root cause: the resolved version is incompatible with Python 3.10.

## azure-cli

### Command

```
./tsuku-test eval --recipe /tmp/recipes-2331/azure-cli.toml --os linux --arch amd64
```

### Output (key fields)

Eval **succeeded** with exit 0. JSON plan produced. Selected fields:

- `tool: "azure-cli"`, `version: "2.85.0"`
- `dependencies[0].tool: "python-standalone"`, `version: "20260414"` resolving to **CPython 3.10.20** (asset `cpython-3.10.20+20260414-x86_64-unknown-linux-gnu-install_only.tar.gz`)
- `steps[0].action: "pip_exec"` with `python_version: "3.10.20"`, `package: "azure-cli"`, `version: "2.85.0"`
- `steps[0].params.locked_requirements`: full hash-pinned requirements.txt with **~120 transitive packages** including `azure-cli==2.85.0`, `azure-cli-core==2.85.0`, `cryptography==47.0.0`, `cffi==2.0.0`, `bcrypt==5.0.0`, `pynacl==1.6.2`, `psutil==7.2.2`, etc.
- `steps[0].params.has_native_addons: true`
- `verify.command: "az --version"`

### Failure layer

Layer: **none reached at eval time**.

Eval/decompose/plan all succeed. `pip download` resolves the entire 120-package transitive closure against Python 3.10.20 without complaint and produces a hash-locked requirements list. The plan is well-formed.

### Match to issue description — partial

Issue claim 1: "azure-cli 2.85.0 declares `requires_python >= 3.10.0` and resolved fine at eval time." **Confirmed.** Eval resolves cleanly, plan is deterministic at the resolution layer (the JSON `deterministic: false` flag is structural — it tags the pip_exec step as non-evaluable — not a sign of resolution failure).

Issue claim 2: "`az --version` returned non-zero in the sandbox after an apparently successful install (`install_exit_code = 0`, `passed = false` after 128s). The most likely cause is a transitive-dependency compat mismatch with the bundled Python."

**Cannot reproduce here.** This is a post-install, in-sandbox failure mode. Reproducing it requires the test-recipe sandbox container infrastructure (`tsuku install --sandbox` with isolated `$TSUKU_HOME`), which is out of scope for an eval-only investigation. The eval-time evidence is consistent with the claim — many of the resolved transitive deps are recent C-extension versions (cryptography 47.0.0, cffi 2.0.0, bcrypt 5.0.0, pynacl 1.6.2) that may have runtime-import issues on cpython 3.10.20 even though their wheels' `Requires-Python` markers accept it — but I cannot empirically confirm "az --version returns non-zero" without a sandbox run.

The issue body's diagnosis ("transitive-dependency compat mismatch") is plausible but unverified by this report. A separate sandbox-install run would be needed to (a) reproduce the non-zero `az --version` exit, (b) capture the actual stderr (likely an ImportError or symbol-not-found from a compiled extension), and (c) confirm whether pinning azure-cli to an older release fixes it.

## Source layer summary

For both tools, the failure mechanism centers on `internal/actions/pipx_install.go`:

- `Decompose` (line 283) fans out to `generateLockedRequirements` (line 402) → `runPipDownloadWithHashes` (line 460).
- `pip download` is invoked against the *bundled* python-standalone interpreter (3.10.20). Whatever Requires-Python the resolver picks is constrained by that interpreter.
- There is no recipe primitive today to pass a version specifier (e.g., `ansible-core>=2.17,<2.18`) into this call. The pipx_install action only consumes `package` and `executables`; the version resolution is delegated to the recipe's `[version]` block, which has no PyPI specifier knob.

So the two empirical failure profiles are:

| Tool | Eval result | Layer | Root cause (verified) | Issue claim match |
|---|---|---|---|---|
| ansible (ansible-core) | exit 1 | decompose / pip download | latest 2.20.5 requires Python >= 3.12; bundled is 3.10.20 | Exact match |
| azure-cli | exit 0 | n/a (eval succeeds) | post-install `az --version` non-zero per issue; not reproduced here | Eval portion confirmed; runtime portion unverified |

## Files

- `/tmp/recipes-2331/ansible.toml` — reconstructed recipe (post-fix, ansible-core)
- `/tmp/recipes-2331/azure-cli.toml` — reconstructed recipe (post-fix, output-mode verify)
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/actions/pipx_install.go` — Decompose / generateLockedRequirements / runPipDownloadWithHashes (lines 283, 402, 460)
- Git refs: `dcb34719` (recipes added), `ed8fc646` (recipes fixed), `511cd640` (recipes deferred/deleted)
