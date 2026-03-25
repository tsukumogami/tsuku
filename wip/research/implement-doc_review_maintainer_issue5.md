# Maintainer Review: Issue 5 — test(hooks): add container shell integration tests

## Findings

### [BLOCKING] Test file is in `internal/hook` but tests files from `internal/hooks`

`internal/hook/integration_test.go` is in package `hook_test` and lives under `internal/hook/`. It reaches across a package boundary to copy files from `internal/hooks/testdata/mock_tsuku` and `internal/hooks/tsuku.bash` via hardcoded paths rooted at `/repo/internal/hooks/`. The next developer who adds a new shell hook script, renames a file in `internal/hooks/`, or reorganizes the testdata directory will get a silent test failure — the Docker `cp` command will fail at runtime with a non-zero exit code, but because `TestHookBash_*` tests call `runBashInContainer` and discard the error return (`out, _ := runBashInContainer(...)`), the test will pass while the assertion `strings.Contains(out, "Command 'jq' not found.")` silently fails to match an empty or error-only output. The developer will see a confusing assertion failure ("expected output to contain X; got: ...error about missing file...") that points at the wrong cause. The fix is to check the error return from `runBashInContainer` in every test that uses it, so a container startup or copy failure surfaces immediately rather than manifesting as a content assertion failure.

### [BLOCKING] `runBashInContainer` error is silently discarded in four tests

`TestHookBash_NoPreExistingHandler`, `TestHookBash_WrapsExistingHandler`, `TestHookBash_RecursionGuard`, and `TestHookBash_DoubleSource` all call:

```go
out, _ := runBashInContainer(t, script)
```

`TestHookZsh` and `TestHookFish` inline the `exec.CommandContext` call and also discard the error (`out, _ := cmd.CombinedOutput()`). Any infrastructure failure — docker not pulling the image, a mount failure, a script syntax error causing non-zero exit — returns a non-nil error. The tests then assert on `out`, which will be an error message from docker rather than a shell session output. The next developer will read "expected output to contain 'Command jq not found'; got: docker: Error response from daemon..." and spend time debugging hook logic when the real problem is container setup. The two tests that install packages (`TestHookZsh`, `TestHookFish`) are especially at risk: `apt-get` failures are common in CI and produce non-zero exits that the test silently absorbs.

The fix is consistent: `if err != nil { t.Fatalf("container run: %v\n%s", err, out) }` after each `runBashInContainer` call.

### [ADVISORY] `TestHookZsh` and `TestHookFish` duplicate `runBashInContainer` inline

`runBashInContainer` exists precisely to encapsulate the docker invocation. `TestHookZsh` (line 192) and `TestHookFish` (line 224) bypass it and construct the `exec.CommandContext` call inline, duplicating the `--rm`, `-v root:/repo:ro`, `-e TSUKU_HOME`, and `CombinedOutput` logic. The difference from `runBashInContainer` is only the script string. The next developer who changes the container flags (e.g., adds a network flag or changes the TSUKU_HOME value) will update `runBashInContainer` and miss the two inline copies, producing inconsistent test environments. Either extend `runBashInContainer` to accept the script as a parameter (it already does), or note in a comment why these tests need a different container configuration.

Looking more carefully: `runBashInContainer` already accepts a `script string` parameter. These two tests don't call it — there is no functional reason apparent in the code. The next developer will assume the duplication is intentional and copy the pattern for new shell tests. Add a comment if there is a reason, or replace the inline calls with `runBashInContainer`.

### [ADVISORY] `TestHookBash_RecursionGuard` uses a different `set -e` posture than the other bash tests

`TestHookBash_NoPreExistingHandler`, `TestHookBash_WrapsExistingHandler`, and `TestHookBash_DoubleSource` all begin with `set -e`. `TestHookBash_RecursionGuard` does not. This looks intentional — the test needs to continue after `jq` exits 127 — but the next developer won't know that. Without `set -e`, a misconfigured setup step (e.g., `cp` failing because the source path changed) will silently pass through and produce an empty `out`, making the negative assertion (`if strings.Contains(out, ...)`) vacuously pass. The test would report green while testing nothing. A short comment — `# no set -e: jq exits 127 which would abort the script before the guard check` — removes the ambiguity.

### [ADVISORY] `container-tests.yml` path filter only triggers on changes to the workflow file itself

```yaml
pull_request:
  paths:
    - '.github/workflows/container-tests.yml'
```

The `hook-integration-tests` job is not triggered when `internal/hook/**` or `internal/hooks/**` change. A developer who modifies the hook scripts or adds a new test in `integration_test.go` will get no CI feedback on their PR. Only pushes to `main` run the full suite. The two existing jobs (`sandbox-tests`, `validate-tests`) have the same pattern, so this may be an intentional project-wide policy. If so, a comment in the workflow file explaining the policy would prevent future developers from "fixing" the path filter and accidentally breaking the intentional design.

### [ADVISORY] `debianImage` function name does not reflect that it fatally terminates on any non-debian entry

`debianImage` reads `container-images.json` and calls `t.Fatal` if the `"debian"` key is missing. The name implies it returns an image reference. The next developer adding a `ubuntuImage` or `alpineImage` helper will copy this pattern correctly, but the fatal behavior is invisible from the call site — `image := debianImage(t)` looks like a pure lookup. This is a minor point given the `t *testing.T` parameter makes side effects plausible in Go test helpers, but renaming to `mustDebianImage` would make the fatal path explicit at the call site.

## Summary

Two blocking issues, both about error handling: the integration tests discard the error return from container execution, which means infrastructure failures (bad paths, docker errors, apt failures) manifest as confusing assertion failures rather than clear "container failed" messages. The next developer hitting a flaky CI failure in `TestHookZsh` will spend time debugging zsh hook logic when the real problem is that `apt-get install zsh` timed out and the error was swallowed.

Four advisory issues: two inline duplications of `runBashInContainer` in the zsh and fish tests; a missing comment explaining why `TestHookBash_RecursionGuard` omits `set -e`; a CI path filter that skips hook-related PRs; and a helper function name that doesn't signal its fatal behavior.

The `mock_tsuku` script and the `tsuku.bash` double-source guard are clear and well-commented. The `TestHookBash_UninstallRestores` test structure (no Docker, direct Go call) is clean and idiomatic.
