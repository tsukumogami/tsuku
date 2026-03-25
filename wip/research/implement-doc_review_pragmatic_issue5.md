# Pragmatic Review: Issue 5 — test(hooks): add container shell integration tests

## Findings

### [BLOCKING] `debianImage` re-parses `container-images.json` from disk; `containerimages` package already provides this

`internal/hook/integration_test.go:35-53` -- `debianImage()` reads and JSON-parses `container-images.json` from the repo root, duplicating what `internal/containerimages` already provides via `containerimages.DefaultImage()`. Replace the function and the `repoRoot` call it requires with a single `containerimages.DefaultImage()` call. This also eliminates the `repoRoot` dependency from the four container-based tests (it would still be needed only for the volume-mount path).

**Fix:** delete `debianImage()`, import `internal/containerimages`, call `containerimages.DefaultImage()` in `runBashInContainer` and the three zsh/fish tests that duplicate the docker-run setup.

### [BLOCKING] `runBashInContainer` is a single-caller abstraction that hides the zsh and fish tests' divergence

`internal/hook/integration_test.go:66-82` -- `runBashInContainer` is called by exactly four bash tests. The zsh test (`TestHookZsh`, line 173) and fish test (`TestHookFish`, line 206) do not use it — they each inline their own `exec.CommandContext` block with identical boilerplate. Either the helper should cover all container-based tests (making it general enough to be justified), or the helper should be deleted and each test should inline the three lines. As written, the abstraction is incomplete and its existence implies a consistency that doesn't hold.

**Fix:** either extend `runBashInContainer` to accept the image reference and have zsh/fish call it, removing the duplicated exec blocks; or delete the helper and inline in all callers.

### [ADVISORY] `repoRoot` uses `runtime.Caller` to find the source file at test time

`internal/hook/integration_test.go:18-32` -- `runtime.Caller(0)` returns the compile-time path embedded in the binary, which breaks when tests are run from a different working directory or with `-trimpath`. The standard pattern in this repo's other container tests would be to use `os.Getwd()` and walk up, or rely on a known `testdata` anchor. Not blocking because the tests will work correctly in CI and in the normal `go test ./...` invocation, but the approach is fragile.

### [ADVISORY] `TestHookBash_UninstallRestores` doesn't need Docker but lives in the same file as Docker-gated tests

`internal/hook/integration_test.go:239-272` -- This test exercises `hook.Install`/`hook.Uninstall` at the Go level with no container. It will be skipped in `-short` mode only if the Docker tests above also call `t.Skip`, which they do via `skipIfNoDocker`. But `TestHookBash_UninstallRestores` has no `skipIfNoDocker` call and no short-mode guard, so it runs in every test execution including the fast unit-test pass. That's actually correct behavior, but placing it in `integration_test.go` alongside container tests is misleading naming — readers will expect Docker is required. Move to `install_test.go` (which already exists) or add a comment distinguishing it from the container-based tests.

## Summary

Two blocking findings: the `debianImage` helper re-implements functionality the `containerimages` package already provides (dead duplication), and `runBashInContainer` is an incomplete abstraction — it covers four of six container-launching tests, leaving zsh and fish to duplicate the same exec boilerplate. Both need to be resolved before merge. Two advisory items: fragile `runtime.Caller` path resolution and a mislocated non-Docker test.
