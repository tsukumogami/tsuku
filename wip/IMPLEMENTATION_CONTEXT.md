## Goal

Fix Build Essentials CI failures caused by cmake 4.2.3 requiring `OPENSSL_3.2.0`, root-cause why this regression wasn't caught when introduced, and improve CI to prevent similar gaps.

## Context

PR #1223 triggered Build Essentials failures across 8 jobs (ninja, cmake, libsixel-source on Linux x86_64 and all sandbox containers). The root cause is:

```
/home/runner/.tsuku/tools/cmake-4.2.3/bin/cmake: /lib/x86_64-linux-gnu/libssl.so.3: version `OPENSSL_3.2.0' not found
```

cmake is a build dependency for ninja and libsixel-source. The cmake 4.2.3 Homebrew bottle links against a newer OpenSSL than the GitHub Actions runner provides. Build Essentials passes on main because it ran before the cmake bottle updated to 4.2.3.

A recent Go version bump in the repo may be related (different runner image or dependency resolution path).

The deeper question is why Build Essentials didn't catch this on the PR that introduced the change. Understanding that gap is necessary to prevent recurrence.

## Acceptance Criteria

- [ ] Build Essentials workflow passes for ninja, cmake, and libsixel-source on Linux x86_64
- [ ] Sandbox jobs (alpine, arch, debian, rhel, suse) pass for ninja
- [ ] Root cause documented: why wasn't this caught on the PR that introduced the regression?
- [ ] CI improvement implemented so similar OpenSSL/shared-library mismatches are caught at introduction time
- [ ] Fix validated on a PR branch before merging

## Dependencies

None
