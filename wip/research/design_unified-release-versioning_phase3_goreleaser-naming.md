# Phase 3 Research: GoReleaser Naming

## Questions Investigated
- What produces the current CLI artifact names with version suffix?
- What's needed to produce `tsuku-{os}-{arch}` instead?
- What downstream consumers reference CLI artifact names?

## Findings

### Current GoReleaser Config
`.goreleaser.yaml` line 11: `binary: tsuku-{{ .Os }}-{{ .Arch }}`

This produces the binary name `tsuku-linux-amd64`. However, the release assets show `tsuku-linux-amd64_0.5.0_linux_amd64`. The `_0.5.0_linux_amd64` suffix is appended by GoReleaser's archive naming for `format: binary` archives. GoReleaser's default archive `name_template` when format is `binary` is `{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}`.

### Fix
Add explicit `name_template` to the archives section:

```yaml
archives:
  - format: binary
    name_template: "{{ .Binary }}"
```

This produces `tsuku-linux-amd64` (just the binary name, no version or os/arch suffix).

### Downstream Consumers
- `release.yml` finalize-release (line ~260): References `tsuku-linux-amd64_${VERSION}_linux_amd64` etc. Must be updated.
- `release.yml` integration-test: Downloads CLI binary by versioned name. Must be updated.
- No tsuku self-install recipe exists (`recipes/t/tsuku.toml` not found).
- `recipes/t/tsuku-dltest.toml`: Uses `tsuku-dltest-{os}-{arch}` -- already version-free, no change needed.
- Website install script (if any): Would reference download URLs. Needs checking.

### v0.5.0 Release Assets (actual)
```
tsuku-darwin-amd64_0.5.0_darwin_amd64
tsuku-darwin-arm64_0.5.0_darwin_arm64
tsuku-linux-amd64_0.5.0_linux_amd64
tsuku-linux-arm64_0.5.0_linux_arm64
tsuku-dltest-{os}-{arch}              (6 variants, no version)
checksums.txt
```

### After Naming Change
```
tsuku-darwin-amd64
tsuku-darwin-arm64
tsuku-linux-amd64
tsuku-linux-arm64
tsuku-dltest-{os}-{arch}              (unchanged)
tsuku-llm-{os}-{arch}[-{backend}]     (new, after pipeline merge)
checksums.txt
```

## Implications for Design
The GoReleaser change is a one-line addition to `archives.name_template`. The real work is updating `release.yml` references (finalize-release expected artifacts, integration-test binary download). The naming change MUST be coordinated with the pipeline because finalize-release validates exact artifact names.

## Surprises
The binary field already produces clean names (`tsuku-linux-amd64`). The version duplication comes entirely from GoReleaser's default archive naming template, not from an intentional configuration choice. This makes the fix simpler than expected.

## Summary
GoReleaser's default archive naming appends `_VERSION_OS_ARCH` to binaries. Adding `name_template: "{{ .Binary }}"` to the archives section fixes this. The main coordination point is updating `release.yml` finalize-release and integration-test to reference the new shorter names.
