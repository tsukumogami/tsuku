## Problem

18 recipe tests are failing in CI with segmentation faults (SIGSEGV):

```
Error: installation verification failed: installation verification failed: exit status 139
```

Exit code 139 = 128 + 11 (SIGSEGV signal).

## Affected Recipes

- act
- buf
- cliproxyapi
- cloudflared
- fabric-ai
- gh
- git-lfs
- go-task
- goreman
- grpcurl
- jfrog-cli
- license-eye
- mkcert
- oh-my-posh
- tailscale
- temporal
- terragrunt
- witr

## Likely Cause

These recipes install pre-built binaries (via `homebrew` action). The binaries crash with SIGSEGV when run in the CI environment, which strongly suggests a **glibc version mismatch**.

Pre-built binaries are typically compiled against a specific glibc version. When run on a system with an older glibc, they crash because required symbols are missing.

Example from affected recipes:
```toml
supported_libc = ["glibc"]
[[steps]]
  action = "homebrew"
  formula = "gh"
```

## Suggested Investigation

1. Check the glibc version on CI runners vs what the Homebrew bottles were built against
2. Use `ldd --version` or inspect binary requirements with `objdump -p <binary> | grep GLIBC`
3. Consider whether CI should use a container with a newer glibc, or if these tests should be skipped on older runners

## Notes

The recipes already have `unsupported_platforms` entries for various Linux distros, suggesting this glibc compatibility issue was anticipated for some environments.
