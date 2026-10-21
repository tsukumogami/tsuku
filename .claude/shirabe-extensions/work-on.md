# work-on extension: tsuku

tsuku's verification map for shirabe's `/work-on` definition-of-done gate is
`verification-map.json` beside this file, read at the merge-base with `main`. **The JSON is the
source**: which paths select which commands, the default list, and the exact text of every
command live there and nowhere else. This file only says why the commands look the way they do,
keyed by each command's id in the JSON. Schema: `skills/work-on/references/verification-map.md`
in the shirabe repo. This file is `@`-imported on every `/work-on` run, so it stays short; the
longer reasons are in its commit history.

Commands copy the PR CI steps named below, and those steps carry a comment pointing back here:
change the CI step, its command in the JSON and its note here together. Every deliberate
difference from CI is noted with its reason and, where one exists, its issue. Copying a CI step
means running what it runs: a green CI run is not evidence that any check here executed.

The gate runs each command as argv from the repo root with no shell, so a command that needs one
is `bash -c` with the step's text. Commands that can reach a read-only public endpoint with no
credentials (the Go toolchain fetching modules or tools, `npm ci`, `cargo` fetching its pinned
toolchain, `workflow-policy.py` calling `gh`) are marked `network: true` and `unattended: true`:
a deliberate exception, approved by the maintainer, to keeping these commands offline, as CI
does. An unreachable network or a GitHub 403 rate limit is cannot-verify, not a failed change.

The gate's globs need a `/` before a `**/` pattern's tail, so the JSON lists the root form
(`*.go`, `Cargo.toml`, `*.fish`, `Dockerfile*`) beside each `**/` one. This file and the JSON
are knowingly left to the default.

## Paths with no local check

The last entry binds paths that carry behavior no command here checks (`.github/**`,
`website/**`, `scripts/**`, `tsuku-llm/**` and the rest) to `no-local-check`, a command marked
`unattended: false` that the gate never starts. Any change touching one of them settles as
`verification/needs-person`, a cannot-verify stop for a person, which is the intended outcome:
nothing checks what these files do. Until shirabe's schema has a form for such an entry
(tsukumogami/shirabe#373), this has two side effects. Nothing else in that change runs either,
so a change that would also fail another command reads cannot-verify rather than failed. And the
schema has no exceptions, so `website/**` and `scripts/**` stay whole: the pipeline pages and
the two scripts that have their own entries also stop for a person, as do the `.github/` paths
whose lint and self-test commands are listed. Run those entries' commands by hand when reviewing
such a change. The hook tests run without Docker, which covers hook output, quoting and refusal
cadence but never runs a hook in a real shell, where shell-specific breakage lives. CI's
queue-data check watches a path the queue no longer lives at (tsukumogami/tsuku#2578).

## Default list

The Linux steps of the Unit Tests and Lint Tests jobs in `.github/workflows/test.yml`. The Go
entry (`*.go`, `go.mod`, `cmd/**`, `internal/**`, `testdata/**` and the rest) binds the same
six commands.

- `go-mod-tidy`: unlike CI, reports an untidy module without rewriting `go.mod` or `go.sum`.
- `gofmt`: unlike CI, checks only the repo's own Go files, because `gofmt -l .` also walks
  gitignored nested worktrees under `.claude/worktrees/`; an empty list fails rather than
  passing over nothing.
- `golangci-lint`: a private cache, since a shared one replays findings from other checkouts;
  the test fetches its tool through `go run`. "parallel golangci-lint is running" means another
  run holds the tool's host-wide lock, which the private cache does not isolate: cannot-verify,
  not a failed change.
- `no-stdlib-log`: the one lint gate `-short` skips that no other command covers. govulncheck is
  left out because it passes having checked nothing when it cannot fetch.
- `unit-tests`: the unit suite inside CI's test-artifact tripwire. Unlike CI: Docker is hidden,
  because the hook container tests install packages from the network and take minutes; fish
  cases skip where fish is not installed, since `TSUKU_REQUIRE_FISH` is not set; telemetry is
  off, so no run posts events. `TSUKU_REGISTRY_URL` is not set, because the loader tests in
  `internal/recipe` and `internal/registry` expect it unset. `LD_LIBRARY_PATH` is cleared, which
  CI gets for free by never setting it: a value a developer's system put there (a distro
  profile, CUDA, Conda, Nix, an IDE) reaches `internal/verify`'s helper-sanitiser tests, which
  would fail on a value the change never set (tsukumogami/tsuku#2585). The sanitiser now emits
  each loader variable once and the tests set their own value, so clearing it is belt and
  braces. `max_procs` is raised to 1024 because `go test ./...` runs compilers and test
  binaries in parallel.

## Notes on the other entries

- `docs-validate-changed`, `docs-lifecycle`, `docs-go-tests` (`docs/**`): the first errors out
  when `origin/main` is missing, which is cannot-verify, not a failed change. The lifecycle check
  uses `--mode=draft`, not `ready`, since an in-flight `/execute` chain keeps its PLAN. The Go
  tests are the only ones that read `docs/`; they pin one document's tables, and another doc
  emptied or deleted is caught by the changed-docs validation, not by them.
- `recipes-*` (validate-recipe-structure.yml and the Validate Recipes job in test.yml): nothing
  is installed, so a download URL or checksum change passes. Unlike CI, `recipes-libc-coverage`
  fails on output jq cannot parse instead of passing, and `recipes-strict` resolves dependencies
  against this checkout's recipes rather than the live registry, so a branch is checked against
  itself. `recipes-go-tests` checks name collisions and the tree's shape, so a stub or gutted
  recipe passes it; it holds because `recipes-strict` does.
- `plugins-no-hooks`, `plugins-exemplars` (validate-skill-content.yml): unlike CI, the exemplars
  resolve against this checkout, and an empty exemplar list fails rather than passing over
  nothing.
- `telemetry` (telemetry-ci.yml): `npm ci` needs the npm registry, so on a disconnected machine
  it is cannot-verify, not a passing change.
- `website-pipeline-links` (website-ci.yml): unlike CI, fails when there are no pages, since the
  script loops over a glob and would otherwise pass having examined nothing.
- `container-images-drift` (drift-check.yml): unlike CI, compares instead of regenerating, since
  the `go:generate` step is a plain `cp`. The `jq` check is the subject guard: `cmp` alone passes
  when both copies are empty or `{}`.
- `golden-exclusions` (the golden-file workflows): without `--check-issues`, which needs a
  token. The `jq` loop is the subject guard, since the script reads a file with no `exclusions`
  key as having nothing to check; an empty exclusion list is valid and passes.
- `rustfmt` (check-rustfmt.yml).
- `retired-runners` through `escalation-hygiene-test` (lint-workflows.yml, which has no `paths:`
  filter and runs them on every pull request): `matrix-recipe-passthrough.sh` reads the jq
  projections out of `scheduled-tests.yml` rather than restating them, and pins the tests that
  declare a `recipe`. `label-references.py` resolves every label a workflow names against
  `.github/labels.yml` and fails on a reference it cannot reduce to a literal. Its self-test runs
  the frozen pre-repair fixture and six mutations; its header records three cases it does not
  catch. `workflow-policy.py` checks every scheduled workflow declares an escalation policy,
  compares the `issue` set against `.github/escalation-registry.yml` both ways and pins the
  number of scheduled workflows; its self-test pins each direction separately.
  `escalation-hygiene.py` asserts no step suppresses the failure of the command that files or
  edits an issue, and that no workflow holds an empty GitHub expression; its self-test proves
  each assertion can fail.
- `escalate-sweep-test`, `escalate-owned-test`, `escalate-branch-test` (lint-workflows.yml):
  all replace `gh` on PATH with a stub. The branch cases construct both directions of #2686;
  the owned cases assert where a run went, not only that nothing new was filed.
- `escalation-backstop-test` (lint-workflows.yml): its `gh` stub filters by the query's own
  parameters and declarations come from a sandbox repository. The live check runs as the
  separate `Escalation Backstop (repository health)` job, whose red is about the repository.
- `coverage-receipt-test` (lint-workflows.yml): asserts on the counts and outcomes each piece
  reports and runs Nightly Registry Validation's steps as written, under `bash -e`. It doesn't
  run a golden comparison with a real `tsuku`.

## Checks this map leaves out, and why

- The sandbox and multi-family container jobs, the platform matrix and the macOS legs: they need
  Docker, several distributions or a macOS host.
- The install jobs: they write a real `$TSUKU_HOME`. The website deploy and the GPU and LLM
  suites: credentials or hardware this does not have.
- The functional suite: it installs real tools over the network.
- The golden-plan comparison: needs credentials. The structural exclusion checks do not replace
  it.
- CI's govulncheck step: it passes having checked nothing whenever it cannot fetch its data.
- CI's queue-data check: it watches a path the queue no longer lives at (tsukumogami/tsuku#2578).
- CI's intermediate-artifact check: a `/work-on` run still holds its staging directory when the
  gate runs, so copying it would fail runs whose correct verdict is passed. It stays a pre-merge
  check.
- The PR-context checks (PR body, closing issues, diagram status): they read a pull request, not
  a file a change touches.
