# Architect Review: Issue 5 — test(hooks): add container shell integration tests

## Findings

### [BLOCKING] Container image lookup bypasses the containerimages package

`internal/hook/integration_test.go:35-53` — The `debianImage` helper reads
`container-images.json` by walking the filesystem from the repo root and parsing
the JSON inline. The codebase already has `internal/containerimages` for exactly
this purpose: `containerimages.DefaultImage()` returns the pinned debian image
reference from the embedded JSON, with schema validation at init time.

The integration tests in `internal/sandbox` and `internal/validate` uniformly
use `containerimages.DefaultImage()` or `containerimages.ImageForFamily()`. The
new test introduces a second reader that parses the same file at a different path
(`container-images.json` at repo root, reached via filesystem walk), using a
different struct shape (anonymous `map[string]struct{ Image string }`), with no
schema validation.

The structural impact is concrete: if `container-images.json` schema changes
(it already evolved to include `infra_packages`), `internal/containerimages`
enforces the invariants at init time and all callers get the update for free.
The inline reader in `debianImage` stays out of sync silently. And because
`debianImage` is a local helper, the next engineer who needs an image reference
in this test file will copy it rather than reach for the package.

Fix: replace `debianImage` with `containerimages.DefaultImage()` directly.
The `repoRoot` walk and the inline JSON parsing can be deleted entirely.

### [BLOCKING] Test file is in `internal/hook` but shell scripts live in `internal/hooks`

`internal/hook/integration_test.go` (package `hook_test`) tests the shell
scripts in `internal/hooks/` by mounting the repo into a container and sourcing
the files at `/repo/internal/hooks/tsuku.bash`. The test belongs to the `hook`
package, but the artefacts it exercises (the `.bash`, `.zsh`, `.fish` files and
the new `testdata/mock_tsuku`) live in the sibling package `hooks`.

This is a package boundary problem. `internal/hooks` owns the shell script
files and already has its own test file (`hooks_test.go`). Placing container
tests for those scripts in a different package (`hook`) makes the test location
wrong relative to what it tests, and leaves `testdata/mock_tsuku` stranded in
`internal/hooks/testdata/` while its only consumer is in `internal/hook/`.

The split also means that `go test ./internal/hooks/...` does not run the
container tests for the bash hook, even though those scripts are owned by that
package.

The correct placement for shell-script integration tests is inside the package
that owns the scripts: `internal/hooks/`. The `TestHookBash_UninstallRestores`
test at the bottom of the file, which exercises `hook.Install` and
`hook.Uninstall` directly, belongs in `internal/hook/` (it already tests the
right package). It should be separated into its own file there.

Fix: move the Docker-based tests (`TestHookBash_*`, `TestHookZsh`, `TestHookFish`)
to a new `integration_test.go` file inside `internal/hooks/`. Move
`TestHookBash_UninstallRestores` to `internal/hook/` (already its natural home).
`testdata/mock_tsuku` stays in `internal/hooks/testdata/` where it is used.

### [ADVISORY] `repoRoot` uses `runtime.Caller` to find the source tree at test time

`internal/hook/integration_test.go:18-32` — The helper walks from the compiled
test binary's source path (via `runtime.Caller`) to find `go.mod`, then mounts
that directory into the container. This works when `go test` is run from a
checkout with source present, but fails silently if tests are run from a
compiled test binary outside the source tree (e.g. `go test -c` + running the
binary elsewhere).

Other container tests in the repo (`internal/sandbox/sandbox_integration_test.go`,
`internal/validate/`) avoid this pattern entirely: they use
`containerimages.DefaultImage()` for the image reference and pass test fixture
data directly rather than mounting a live source tree. The hook tests need the
mount because they source the shell scripts; if the tests moved to
`internal/hooks/` the `repoRoot` walk would still be needed, but it should be
noted as fragile. An alternative is to read the hook scripts via the existing
`hooks.HookFiles` embed FS, write them to a temp dir before the docker run, and
mount that temp dir instead of the live source tree — making the tests
independent of source layout.

This is advisory because it currently works and the fragility is contained to
the test helper function.

## Summary

Two blocking issues: the image lookup bypasses `internal/containerimages`
(introducing a parallel JSON reader for data the package already provides), and
the test file is placed in the wrong package (`hook`) for the artefacts it
actually tests (`hooks`). Both will be copied: the inline JSON reader will
spread to other test files that need container images, and future hook tests
will be placed in whichever package happens to already have the Docker
infrastructure. One advisory: the `runtime.Caller` source-walk pattern for
finding the repo root is fragile compared to using embedded file data, but is
self-contained.
