# Test Plan: Sandbox Image Unification

Generated from: docs/designs/DESIGN-sandbox-image-unification.md
Issues covered: 4
Total scenarios: 14

---

## Scenario 1: container-images.json exists with correct content
**ID**: scenario-1
**Testable after**: #1901
**Commands**:
- `test -f container-images.json`
- `jq -e '.debian and .rhel and .arch and .alpine and .suse' container-images.json`
- `jq -r '.debian' container-images.json`
- `jq -r '.alpine' container-images.json`
- `jq -r '.suse' container-images.json`
**Expected**: File exists and contains all five families. Values are "debian:bookworm-slim", "fedora:41", "archlinux:base", "alpine:3.21", "opensuse/tumbleweed" -- fixing the existing drift from alpine:3.19 and opensuse/leap:15.
**Status**: pending

---

## Scenario 2: containerimages Go package provides correct API
**ID**: scenario-2
**Testable after**: #1901
**Commands**:
- `go doc ./internal/containerimages/`
**Expected**: Output lists `ImageForFamily` function with signature `func ImageForFamily(family string) (string, bool)` and `DefaultImage` function with signature `func DefaultImage() string`. These are the two public entry points the sandbox package uses.
**Status**: pending

---

## Scenario 3: go generate copies embedded JSON and it matches root
**ID**: scenario-3
**Testable after**: #1901
**Commands**:
- `go generate ./internal/containerimages/...`
- `diff container-images.json internal/containerimages/container-images.json`
**Expected**: `go generate` succeeds with exit code 0. `diff` shows no differences (exit code 0), confirming the embedded copy matches the root file exactly.
**Status**: pending

---

## Scenario 4: containerimages unit tests pass
**ID**: scenario-4
**Testable after**: #1901
**Commands**:
- `go test -v ./internal/containerimages/...`
**Expected**: All tests pass. Tests cover: ImageForFamily returns correct image for each known family, ImageForFamily returns ("", false) for unknown families, DefaultImage returns "debian:bookworm-slim", and the embedded JSON contains all expected families.
**Status**: pending

---

## Scenario 5: familyToBaseImage map removed from container_spec.go
**ID**: scenario-5
**Testable after**: #1901
**Commands**:
- `! grep -q 'familyToBaseImage' internal/sandbox/container_spec.go`
**Expected**: Exit code 0 (grep finds no match). The hardcoded map has been removed and replaced with calls to containerimages.ImageForFamily.
**Status**: pending

---

## Scenario 6: DefaultSandboxImage constant removed from requirements.go
**ID**: scenario-6
**Testable after**: #1901
**Commands**:
- `! grep -q 'const DefaultSandboxImage' internal/sandbox/requirements.go`
- `grep -q 'SourceBuildSandboxImage' internal/sandbox/requirements.go`
**Expected**: DefaultSandboxImage constant no longer exists (first command exit 0). SourceBuildSandboxImage constant ("ubuntu:22.04") still exists (second command exit 0) since it is a build variant, not a family-level image.
**Status**: pending

---

## Scenario 7: all Go tests pass after refactoring
**ID**: scenario-7
**Testable after**: #1901
**Commands**:
- `go test ./...`
**Expected**: Exit code 0. All existing sandbox tests pass with the updated image values (alpine:3.21, opensuse/tumbleweed) and the new containerimages package wiring. The sandbox public API has no signature changes.
**Status**: pending

---

## Scenario 8: go vet passes with no issues
**ID**: scenario-8
**Testable after**: #1901
**Commands**:
- `go vet ./...`
**Expected**: Exit code 0, no output. No lint issues introduced by the refactoring.
**Status**: pending

---

## Scenario 9: Makefile build target runs go generate before go build
**ID**: scenario-9
**Testable after**: #1901
**Commands**:
- `grep -A5 '^build:' Makefile`
**Expected**: The build target includes `go generate ./internal/containerimages/...` before `go build`, ensuring the embedded copy stays fresh during local development.
**Status**: pending

---

## Scenario 10: no hardcoded image strings remain in CI workflow files
**ID**: scenario-10
**Testable after**: #1902
**Commands**:
- `grep -rEn '"(debian:bookworm-slim|fedora:(39|41)|archlinux:base|opensuse/(tumbleweed|leap:15)|alpine:3\.(19|21))"' .github/workflows/recipe-validation-core.yml .github/workflows/test-recipe.yml .github/workflows/batch-generate.yml .github/workflows/validate-golden-execution.yml .github/workflows/platform-integration.yml .github/workflows/release.yml test/scripts/test-checksum-pinning.sh; echo "exit:$?"`
**Expected**: grep returns exit code 1 (no matches). Zero hardcoded container image strings for the five supported families appear in any of the seven target files. All image references now derive from container-images.json via jq.
**Status**: pending

---

## Scenario 11: test-checksum-pinning.sh reads images from config file
**ID**: scenario-11
**Testable after**: #1902
**Commands**:
- `grep -q 'jq' test/scripts/test-checksum-pinning.sh`
- `grep -q 'container-images.json' test/scripts/test-checksum-pinning.sh`
- `! grep -qE 'fedora:39|alpine:3\.19' test/scripts/test-checksum-pinning.sh`
**Expected**: The script uses jq to read from container-images.json. The stale fedora:39 and alpine:3.19 references are eliminated. The script validates jq availability early with a clear error if missing.
**Status**: pending

---

## Scenario 12: renovate.json exists with valid config for container-images.json
**ID**: scenario-12
**Testable after**: #1903
**Commands**:
- `jq . renovate.json`
- `jq -e '.customManagers' renovate.json`
**Expected**: renovate.json is valid JSON. It contains a customManagers array with a regex custom manager that targets container-images.json using the docker datasource to propose automated version bumps.
**Status**: pending

---

## Scenario 13: drift-check CI job detects stale embedded copy
**ID**: scenario-13
**Testable after**: #1903
**Environment**: manual
**Commands**:
- `echo '{}' > internal/containerimages/container-images.json`
- `go generate ./internal/containerimages/...`
- `git diff --exit-code internal/containerimages/container-images.json`
**Expected**: After deliberately writing an empty JSON to the embedded copy, running go generate restores it. The git diff shows changes (exit code 1), which is what the CI drift-check job would detect and fail on. After restoring: `go generate ./internal/containerimages/... && git diff --exit-code internal/containerimages/container-images.json` exits 0 (no diff) confirming the check passes on clean state.
**Status**: pending

---

## Scenario 14: CODEOWNERS protects container-images.json
**ID**: scenario-14
**Testable after**: #1904
**Commands**:
- `grep 'container-images.json' .github/CODEOWNERS`
**Expected**: Output shows `/container-images.json` with `@tsukumogami/core-team @tsukumogami/security-team` as reviewers, matching the same teams that protect workflow files. A comment explains why the file is protected.
**Status**: pending
