# work-on extension: tsuku

tsuku's verification map for shirabe's `/work-on` definition-of-done gate. Schema:
`skills/work-on/references/verification-map.md` in the shirabe repo. The default runs only when
no entry matches any changed file. Commands copy the PR CI steps they name, which point back
here: change both together. Deliberate differences are marked. Run every command from the repo
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
  - `go test -count=1 -run 'TestGatesTableMatchesTheRecord|TestDerivation' .` (the only Go tests that read `docs/`)
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
    (unlike CI: dependencies resolve against this checkout's recipes, not the live registry)
  - `TSUKU_NO_TELEMETRY=1 go test -count=1 ./internal/recipe/ ./internal/sonameindex/ ./internal/indexfixture/` (the Go tests that read the recipe tree)
- `plugins/**` -> both of (validate-skill-content.yml):
  - `for p in tsuku-recipes tsuku-user; do [ ! -f "plugins/$p/hooks.json" ] || exit 1; done`
  - `T=$(mktemp -d) && go build -o "$T/tsuku" ./cmd/tsuku && P=$(grep -oE 'recipes/[^[:space:]]+\.toml' plugins/tsuku-recipes/skills/recipe-author/references/exemplar-recipes.md | sort -u) && [ -n "$P" ] && for p in $P; do [ -f "$p" ] && TSUKU_HOME="$T" TSUKU_NO_TELEMETRY=1 TSUKU_REGISTRY_URL="$PWD" "$T/tsuku" validate "$p" >/dev/null || { echo "failed: $p"; exit 1; }; done`
    (unlike CI: resolves against this checkout, and an empty exemplar list fails)
- `telemetry/**` -> `(cd telemetry && npm ci && npm run typecheck && npm run test:coverage)`
  (telemetry-ci.yml; `npm ci` needs the npm registry, so on a disconnected machine this cannot
  run at all: that is cannot-verify, not a passing change)
- `website/pipeline/*.html`, `scripts/check-pipeline-links.sh` -> `ls website/pipeline/*.html >/dev/null 2>&1 && bash scripts/check-pipeline-links.sh`
  (website-ci.yml; unlike CI, fails when there are no pages to check: the script loops over a
  glob, so with the pages gone it would pass having examined nothing)
- `container-images.json`, `internal/containerimages/**` -> `cmp container-images.json internal/containerimages/container-images.json`
  (drift-check.yml; unlike CI, compares instead of regenerating, since the `go:generate` step is a plain copy)
- `testdata/golden/exclusions.json`, `testdata/golden/code-validation-exclusions.json`,
  `scripts/validate-golden-exclusions.sh` ->
  `./scripts/validate-golden-exclusions.sh && ./scripts/validate-golden-exclusions.sh --file testdata/golden/code-validation-exclusions.json`
  (the golden-file workflows, without `--check-issues`, which needs a token; the golden-plan
  comparison itself needs credentials and is left out. A malformed or missing exclusions file
  fails; an empty exclusion list passes, since having no exclusions is a valid state)
- `**/*.rs`, `**/Cargo.toml`, `**/rust-toolchain.toml` -> `(cd tsuku-llm && cargo fmt --all --check) && (cd cmd/tsuku-dltest && cargo fmt --all --check)` (check-rustfmt.yml)
- `.github/workflows/**` -> `.github/scripts/checks/retired-runners.sh` and `.github/scripts/checks/ci-patterns-lint.sh` (lint-workflows.yml)
- `.github/**`, `website/**` (except `website/pipeline/*.html`), `blog/**`, `scripts/**` (except
  the two scripts above), `sandbox/**`, `Dockerfile*`, `test/scripts/**`, `test-matrix.json`,
  `test/functional/**`, `internal/hooks/**`, `**/*.fish`, `tsuku-llm/**`, `cmd/tsuku-dltest/**`,
  `testdata/golden/execution-exclusions.json`, `data/**`, `proto/**`, `.goreleaser*`, `codecov.yml`, `renovate.json`, `batch-control.json`,
  `.claude/settings.json` -> no local check exists. Still run every other selected command; if
  one fails the outcome is failed, otherwise it is cannot-verify: no command here checks what
  these files do, so the run stops for a person to check them. The hook tests run without
  Docker, which covers hook output, quoting and refusal cadence but never runs a hook in a real
  shell, where shell-specific breakage lives. CI's queue-data check watches a path the queue
  no longer lives at (tsukumogami/tsuku#2578). (shirabe's schema has no form for such an entry
  yet: tsukumogami/shirabe#373.)

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
  CI gets for free: with tsuku's shell integration active it points at the developer's own
  installed libraries, and `internal/verify`'s helper-sanitiser tests then fail on a value the
  change never set. CI's govulncheck step is left out:
  it passes having checked nothing when it cannot fetch.)
