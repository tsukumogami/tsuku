## Summary

Commit 87fe4dfd (#1872) unified `ExtractBinaries()` in `internal/recipe/types.go` so all gem executables map to `bin/<name>`. This is correct for `gem_install` (which uses `--bindir <installDir>/bin/`) and `gem_exec` lock_data mode (which creates wrappers at `<installDir>/bin/`). However, it breaks the `install_gem_direct` action, which still installs gems to `<installDir>/.gem/bin/<name>`.

The result: bundler (and any gem that decomposes through `install_gem_direct`) fails at verification time because the install manager expects the binary at `<toolDir>/bin/bundler` but the file is actually at `<toolDir>/.gem/bin/bundler`.

## Reproduction

This reproduces on any branch that includes commit 87fe4dfd and runs the Gem Builder Tests workflow:

1. `tsuku create bundler --from rubygems --yes --force`
2. `tsuku install --force bundler`

The install step fails with:

```
Could not compute binary checksums: failed to resolve binary path bin/bundler:
  lstat <toolDir>/bin: no such file or directory

Verifying bundler (version 4.0.6)...
  Step 1: Verifying installation via symlink...
    Running: bundler --version
Error: installation verification failed: exit status 127
Output: <toolDir>/bundler: 3: exec: <toolDir>/bin/bundler: not found
```

Failing CI run: https://github.com/tsukumogami/tsuku/actions/runs/22285733960

## Root Cause

The change in `internal/recipe/types.go` (line ~896) removed the `gem_install` special case from `ExtractBinaries()`:

```go
// Before (worked):
if step.Action == "gem_install" {
    destPath = filepath.Join(".gem", "bin", binaryName)
} else {
    destPath = filepath.Join("bin", binaryName)
}

// After (broke install_gem_direct path):
destPath := filepath.Join("bin", binaryName)
```

This is correct for `gem_install`'s direct execution and `gem_exec`'s lock_data mode, both of which now place binaries at `bin/`. But `gem_install` decomposes to `install_gem_direct` for bundler, and that action still installs to `.gem/bin/`.

The flow:
1. Recipe has a `gem_install` step with `executables: ["bundler"]`
2. During plan generation, `gem_install.Decompose()` returns `install_gem_direct` for bundler
3. `install_gem_direct.Execute()` installs to `<installDir>/.gem/bin/bundler`
4. `ExtractBinaries()` runs on the **original recipe** and returns `["bin/bundler"]`
5. The install manager creates a wrapper pointing to `<toolDir>/bin/bundler` -- which doesn't exist

## Fix

Update `install_gem_direct.go` to install gems directly to `<installDir>` (using `--install-dir <installDir>` and `--bindir <installDir>/bin/`), matching the convention used by `gem_install` and `gem_exec`. This eliminates the `.gem/` subdirectory and aligns all gem installation paths to use `bin/`.

## Affected Components

- `internal/actions/install_gem_direct.go` (installs to wrong path)
- `internal/recipe/types.go` (`ExtractBinaries` removed the compensating special case)

## Impact

All PRs that trigger the Gem Builder Tests workflow (by touching `internal/builders/**`) will fail CI until this is fixed. The gem builder workflow on main itself is not directly affected because the commit that caused this (`87fe4dfd`) only changed `internal/actions/` files, not `internal/builders/`.
