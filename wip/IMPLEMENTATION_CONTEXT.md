## What happens

When `TSUKU_HOME` is set to a relative path (e.g., `TSUKU_HOME=.tsuku-test`), any recipe that uses a decomposable action (`cargo_install` -> `cargo_build`, or `gem_install` -> `gem_exec`) fails with:

```
fork/exec .tsuku-test/tools/rust-1.93.1/bin/cargo: no such file or directory
```

or for gems:
```
bundle config failed: fork/exec .tsuku-test/tools/ruby-4.0.1/bin/bundle: no such file or directory
```

## Root cause

The decomposed actions (`cargo_build`, `gem_exec`) resolve tool paths via `ResolveCargo()` or `findBundler()`, which use `GetToolsDir()`. `GetToolsDir()` reads `TSUKU_HOME` from the environment and joins it with `/tools`:

```go
// internal/actions/util.go:323
func GetToolsDir() string {
    if tsukuHome := os.Getenv("TSUKU_HOME"); tsukuHome != "" {
        return filepath.Join(tsukuHome, "tools")
    }
    // ...
}
```

This produces a **relative path** like `.tsuku-test/tools`. The resolved cargo/bundle path (e.g., `.tsuku-test/tools/rust-1.93.1/bin/cargo`) is then passed to `exec.CommandContext()`.

However, decomposed actions change the working directory to a temp directory before executing:

```go
// internal/actions/cargo_build.go:431-433
fetchCmd := exec.CommandContext(ctx.Context, cargoPath, fetchArgs...)
fetchCmd.Dir = crateDir  // This is a temp directory like /tmp/tsuku-cargo-build-*
```

Since the cargo/bundle path is relative, it's now evaluated relative to the temp directory, where it doesn't exist.

## Why non-decomposed actions work

The non-decomposed `cargo_install` action (direct `gem install` / `cargo install`) also uses the same path resolution, but it doesn't change `cmd.Dir` to a temp directory -- it runs from the original working directory where the relative path is valid.

## Reproduction

```bash
cd /path/to/tsuku
go build -o tsuku-test ./cmd/tsuku

# This FAILS (relative path):
TSUKU_HOME=.tsuku-test ./tsuku-test install --recipe recipes/c/cargo-expand.toml

# This WORKS (absolute path):
TSUKU_HOME="$(pwd)/.tsuku-test" ./tsuku-test install --recipe recipes/c/cargo-expand.toml
```

## Affected code paths

- `internal/actions/util.go` -- `GetToolsDir()`, `ResolveCargo()`, `ResolveGem()`
- `internal/actions/cargo_build.go` -- `executeLockDataMode()` sets `cmd.Dir` to temp dir
- `internal/actions/gem_exec.go` -- `executeLockDataMode()` and `findBundler()` same issue

## Suggested fix

`GetToolsDir()` (or the caller) should resolve TSUKU_HOME to an absolute path early:

```go
func GetToolsDir() string {
    if tsukuHome := os.Getenv("TSUKU_HOME"); tsukuHome != "" {
        if !filepath.IsAbs(tsukuHome) {
            if abs, err := filepath.Abs(tsukuHome); err == nil {
                tsukuHome = abs
            }
        }
        return filepath.Join(tsukuHome, "tools")
    }
    // ...
}
```

## Impact

All decomposed actions (cargo_build, gem_exec) fail when TSUKU_HOME is relative. This affects CI test runners and any user who sets TSUKU_HOME to a relative path.
