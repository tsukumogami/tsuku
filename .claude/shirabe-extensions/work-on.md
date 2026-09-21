# work-on extension: tsuku

tsuku's verification map for shirabe's `/work-on` definition-of-done gate. Schema:
`skills/work-on/references/verification-map.md` in the shirabe repo. The default runs only when
no entry matches any changed file. Commands copy the PR CI steps they name, which point back
here: change both together. Every deliberate difference from CI is annotated where it appears,
with its reason and, where one exists, its issue. Run every command from the repo
root. The unit suite, the recipe-tree tests and telemetry's `npm ci` reach read-only public
endpoints (GitHub, the npm registry) with no credentials, as CI does: a deliberate exception,
approved by the maintainer, to keeping these commands offline. An unreachable network or a
GitHub 403 rate limit is cannot-verify, not a failed change. Paths that carry behavior where no
command here checks what they do have their own entry at the end, which makes the gate
cannot-verify rather than letting another entry pass them; this file is knowingly left to the
default. This file is `@`-imported on every `/work-on` run, so it stays short; the reasons are
in its commit history. Copying a CI step means running what it runs: a green CI run is not
evidence that any check here executed.

## Verification map

- `**/*.go`, `go.mod`, `go.sum`, `Makefile`, `cmd/**`, `internal/**`, `testdata/**`,
  `container-images.json` -> every default command below
- `docs/**` -> all of:
  - `B=$(git merge-base origin/main HEAD) && git diff --name-only --diff-filter=ACMR "$B" -- :/docs/ | grep -vE "(^|/)(evals|tests)/fixtures/" | xargs -r shirabe validate --visibility=public` (errors out when `origin/main` is missing: that is cannot-verify, not a failed change)
  - `shirabe validate --visibility=public --lifecycle . --mode=draft` (not `ready`: an in-flight `/execute` chain keeps its PLAN, which the ready posture rejects)
  - `go test -count=1 -run 'TestGatesTableMatchesTheRecord|TestDerivation' .` (the only Go tests
    that read `docs/`. They pin one document's tables and fail on every way of breaking it, but
    that is their whole scope: another doc emptied or deleted is caught by the changed-docs
    `shirabe validate` above, not by this command)
- `recipes/**`, `internal/recipe/recipes/**` -> all of (validate-recipe-structure.yml and the
  Validate Recipes job in test.yml; nothing is installed, so a download URL or checksum change
  passes):
  - `[ -d internal/recipe/recipes ] && [ -z "$(find internal/recipe/recipes -mindepth 1 -maxdepth 1 -type d)" ] && [ -n "$(find internal/recipe/recipes -maxdepth 1 -name '*.toml')" ]`
  - `[ -d recipes ] && [ -z "$(find recipes -maxdepth 1 -name '*.toml')" ] && [ -z "$(find recipes -mindepth 1 -maxdepth 1 -type d ! -name '[a-z]' ! -name discovery)" ]`
  - `[ -z "$(find recipes -mindepth 2 -maxdepth 2 -name '*.toml' | awk -F/ '{ if (substr($3, 1, 1) != $2) print }')" ]`
  - `[ -z "$(comm -12 <(find internal/recipe/recipes -maxdepth 1 -name '*.toml' -exec basename {} .toml \; | sort) <(find recipes -mindepth 2 -maxdepth 2 -name '*.toml' -exec basename {} .toml \; | sort))" ]`
  - `T=$(mktemp -d) && go build -o "$T/tsuku" ./cmd/tsuku && for r in internal/recipe/recipes/*.toml; do o=$("$T/tsuku" validate --check-libc-coverage --json "$r" 2>/dev/null); e=$(printf '%s' "$o" | jq -er '[.errors[]?.message] | join("; ")') && [ -z "$e" ] || { echo "$r: ${e:-unreadable output}"; exit 1; }; done`
    (unlike CI: output jq cannot parse fails instead of passing)
  - `T=$(mktemp -d) && go build -o "$T/tsuku" ./cmd/tsuku && for r in internal/recipe/recipes/*.toml recipes/*/*.toml; do TSUKU_HOME="$T" TSUKU_NO_TELEMETRY=1 TSUKU_REGISTRY_URL="$PWD" "$T/tsuku" validate --strict "$r" >/dev/null || { echo "failed: $r"; exit 1; }; done`
    (unlike CI: dependencies resolve against this checkout's recipes, not the live registry, so a
    branch is checked against itself and the command needs no network; nothing is installed, so a
    download URL or checksum change still passes)
  - `TSUKU_NO_TELEMETRY=1 go test -count=1 ./internal/recipe/ ./internal/sonameindex/ ./internal/indexfixture/`
    (the Go tests that read the recipe tree. They check name collisions and the tree's shape, so a
    stub, empty or gutted recipe file passes them; the strict validation above is what catches
    those. This entry holds because that sibling command holds, not on its own)
- `plugins/**` -> both of (validate-skill-content.yml):
  - `for p in tsuku-recipes tsuku-user; do [ ! -f "plugins/$p/hooks.json" ] || exit 1; done`
  - `T=$(mktemp -d) && go build -o "$T/tsuku" ./cmd/tsuku && P=$(grep -oE 'recipes/[^[:space:]]+\.toml' plugins/tsuku-recipes/skills/recipe-author/references/exemplar-recipes.md | sort -u) && [ -n "$P" ] && for p in $P; do [ -f "$p" ] && TSUKU_HOME="$T" TSUKU_NO_TELEMETRY=1 TSUKU_REGISTRY_URL="$PWD" "$T/tsuku" validate "$p" >/dev/null || { echo "failed: $p"; exit 1; }; done`
    (unlike CI: resolves against this checkout rather than the live registry, for the same reason
    as the recipe entry; an empty exemplar list fails rather than passing over nothing)
- `telemetry/**` -> `(cd telemetry && npm ci && npm run typecheck && npm run test:coverage)`
  (telemetry-ci.yml; `npm ci` needs the npm registry, so on a disconnected machine this cannot
  run at all: that is cannot-verify, not a passing change)
- `website/pipeline/*.html`, `scripts/check-pipeline-links.sh` -> `ls website/pipeline/*.html >/dev/null 2>&1 && bash scripts/check-pipeline-links.sh`
  (website-ci.yml; unlike CI, fails when there are no pages to check: the script loops over a
  glob, so with the pages gone it would pass having examined nothing)
- `container-images.json`, `internal/containerimages/**` -> `jq -e 'length > 0 and all(.[]; has("image"))' container-images.json >/dev/null && cmp container-images.json internal/containerimages/container-images.json`
  (drift-check.yml; unlike CI, compares instead of regenerating, because the `go:generate` step
  is a plain `cp`, so comparing proves the same thing without writing into the tree. The `jq`
  check is the subject guard: `cmp` alone passes when both copies are empty or `{}`, which is
  identical and checks nothing)
- `testdata/golden/exclusions.json`, `testdata/golden/code-validation-exclusions.json`,
  `scripts/validate-golden-exclusions.sh` ->
  `for f in testdata/golden/exclusions.json testdata/golden/code-validation-exclusions.json; do jq -e '.exclusions | type == "array"' "$f" >/dev/null || exit 1; done && ./scripts/validate-golden-exclusions.sh && ./scripts/validate-golden-exclusions.sh --file testdata/golden/code-validation-exclusions.json`
  (the golden-file workflows, without `--check-issues`, which needs a token; the golden-plan
  comparison itself needs credentials and is left out. The `jq` loop is the subject guard: the
  script treats a file with no `exclusions` key, or a mistyped one, as having nothing to check
  and exits 0, so `{}` would pass. A malformed or missing file fails either way; an empty
  exclusion list still passes, since having no exclusions is a valid state)
- `**/*.rs`, `**/Cargo.toml`, `**/rust-toolchain.toml` -> `(cd tsuku-llm && cargo fmt --all --check) && (cd cmd/tsuku-dltest && cargo fmt --all --check)` (check-rustfmt.yml)
- `.github/workflows/**`, `test-matrix.json`, `.github/labels.yml`,
  `.github/escalation-registry.yml` -> `.github/scripts/checks/retired-runners.sh`,
  `.github/scripts/checks/ci-patterns-lint.sh`,
  `.github/scripts/checks/matrix-recipe-passthrough.sh`,
  `.github/scripts/checks/label-references.py`,
  `.github/scripts/checks/label-references_test.sh`,
  `.github/scripts/checks/workflow-policy.py` and
  `.github/scripts/checks/workflow-policy_test.sh` (lint-workflows.yml).
  `matrix-recipe-passthrough.sh` reads the jq projections out of `scheduled-tests.yml`
  rather than restating them, so it cannot drift from what runs, and it pins the set of
  tests that declare a `recipe` so a deleted declaration fails instead of silently leaving
  the comparison. `label-references.py` resolves every label a workflow names against
  `.github/labels.yml`, including labels reached through a shell variable, and fails on any
  reference it cannot reduce to a literal rather than skipping it.
  `label-references_test.sh` is that check's self-test: it runs the frozen pre-repair
  fixture and six mutations, and voids any mutation row whose target did not change. Its
  header records three cases it does not catch, one of which is a false negative rather
  than a skip. `workflow-policy.py` checks every scheduled workflow declares an escalation
  policy and compares the set declaring `issue` against `.github/escalation-registry.yml`
  in both directions; it pins the number of scheduled workflows, so adding one is a
  deliberate edit rather than a silent change. `workflow-policy_test.sh` is its self-test
  and pins both directions of that comparison separately, since a two-way check silently
  becoming one-way still passes.
  Note that lint-workflows.yml carries no `paths:` filter, so these run on every pull
  request regardless of what it touches.
- `.github/**`, `website/**` (except `website/pipeline/*.html`), `blog/**`, `scripts/**` (except
  the two scripts above), `sandbox/**`, `Dockerfile*`, `test/scripts/**`,
  `test/functional/**`, `internal/hooks/**`, `**/*.fish`, `tsuku-llm/**`, `cmd/tsuku-dltest/**`,
  `testdata/golden/execution-exclusions.json`, `data/**`, `proto/**`, `.goreleaser*`, `codecov.yml`, `renovate.json`, `batch-control.json`,
  `.claude/settings.json` -> no local check exists. Still run every other selected command; if
  one fails the outcome is failed, otherwise it is cannot-verify: no command here checks what
  these files do, so the run stops for a person to check them. The hook tests run without
  Docker, which covers hook output, quoting and refusal cadence but never runs a hook in a real
  shell, where shell-specific breakage lives. CI's queue-data check watches a path the queue
  no longer lives at (tsukumogami/tsuku#2578). (shirabe's schema has no form for such an entry
  yet: tsukumogami/shirabe#373.)

### Checks this map leaves out, and why

- The sandbox and multi-family container jobs, the platform matrix and the macOS legs: they need
  Docker, several distributions or a macOS host.
- The install jobs: they write a real `$TSUKU_HOME`. The website deploy and the GPU and LLM
  suites: credentials or hardware this does not have.
- The functional suite: it installs real tools over the network.
- The golden-plan comparison: needs credentials. The structural exclusion checks above do not
  replace it.
- CI's govulncheck step: it passes having checked nothing whenever it cannot fetch its data.
- CI's queue-data check: it watches a path the queue no longer lives at, so it examines nothing
  today (tsukumogami/tsuku#2578).
- CI's intermediate-artifact check: a `/work-on` run still holds its staging directory when this
  gate runs, so copying that check would fail runs whose correct verdict is passed. It stays a
  pre-merge check.
- The PR-context checks (PR body, closing issues, diagram status): they read a pull request, not
  a file a change touches.

### Default verification command (when no map entry matches; all must pass)

The Linux steps of the Unit Tests and Lint Tests jobs in `.github/workflows/test.yml`.

- `go mod tidy -diff` (unlike CI: reports an untidy module without rewriting `go.mod` or `go.sum`)
- `F=$(git ls-files --cached --others --exclude-standard "*.go") && [ -n "$F" ] && out=$(printf "%s\n" "$F" | xargs gofmt -l) && [ -z "$out" ]`
  (unlike CI: checks only the repo's own Go files, because `gofmt -l .` also walks gitignored
  nested worktrees under `.claude/worktrees/`; an empty list fails rather than passing over nothing)
- `go vet ./...`
- `GOLANGCI_LINT_CACHE=$(mktemp -d) go test -count=1 -run '^TestGolangCILint$' .` (a private
  cache, since a shared one replays findings from other checkouts; the test fetches its tool
  through `go run`. "parallel golangci-lint is running" means another run holds the tool's
  host-wide lock, which the private cache does not isolate: cannot-verify, not a failed change)
- `go test -count=1 -run '^TestNoStdlibLog$' .` (the one lint gate `-short` skips that no other
  command here covers; gofmt, tidy, vet and golangci-lint are declared separately above, and
  govulncheck is left out because it passes having checked nothing when it cannot fetch)
- `H=$(git rev-parse HEAD) && S=$(git status --porcelain) && env -u LD_LIBRARY_PATH DOCKER_HOST=unix:///nonexistent TSUKU_NO_TELEMETRY=1 go test -short ./... && [ "$(git rev-parse HEAD)" = "$H" ] && [ "$(git status --porcelain)" = "$S" ]`
  (the unit suite inside CI's test-artifact tripwire. Unlike CI: Docker is hidden, because the
  hook container tests install packages from the network and take minutes; fish cases skip
  where fish is not installed, since `TSUKU_REQUIRE_FISH` is not set; telemetry is off, so no
  run posts events. `TSUKU_REGISTRY_URL` is not set here, because the loader tests in
  `internal/recipe` and `internal/registry` expect it unset. `LD_LIBRARY_PATH` is cleared, which
  CI gets for free by never setting it: whatever a developer's own system put there -- a distro
  profile script, CUDA, Conda, Nix, an IDE -- reaches `internal/verify`'s helper-sanitiser
  tests, which then fail on a value the change never set (tsukumogami/tsuku#2585). Not tsuku's
  doing: nothing in tsuku exports either loader variable into an interactive shell, and the
  per-binary wrapper scripts that do export one (`generateWrapperScript` in
  `internal/install/manager.go`) are
  process-local and exec their tool immediately. So the suite fails on a developed machine and
  passes in CI, the reverse of the usual reading that a red local run means a broken machine.
  The sanitiser used to emit the variable twice -- copying the inherited value and then
  appending its own -- which reached no child, since `os/exec` de-duplicates `cmd.Env`
  last-wins, but did mislead anything reading the slice, which is what those tests do. It now
  emits each loader variable once and the tests set their own value, so clearing here is belt
  and braces rather than the thing standing between you and a false red. The separate defect in
  tsukumogami/tsuku#1090, where a system library wins over tsuku's, had a different cause: the
  path added was `$TSUKU_HOME/libs` while libraries live at
  `$TSUKU_HOME/libs/<name>-<version>/lib`, so the directory named contained no `.so` files at
  all.
  CI's govulncheck step is left out:
  it passes having checked nothing when it cannot fetch.)
