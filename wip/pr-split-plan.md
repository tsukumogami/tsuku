# PR Split Plan: `docs/system-lib-backfill`

Generated: 2026-03-01

## Summary

Split 1983 changed files into 18 PRs.

| PR | Count | Description |
|-----|-------|-------------|
| 1 | 34 + 411 non-recipe | Non-recipe files + modified recipes |
| 2 | 103 | Crates.io / cargo_install recipes |
| 3 | 83 | RubyGems / gem_install recipes |
| 4 | 23 | Homebrew recipes (batch 1) |
| 5 | 99 | Homebrew recipes (batch 2) |
| 6 | 99 | Homebrew recipes (batch 3) |
| 7 | 100 | Homebrew recipes (batch 4) |
| 8 | 100 | Homebrew recipes (batch 5) |
| 9 | 100 | Homebrew recipes (batch 6) |
| 10 | 100 | Homebrew recipes (batch 7) |
| 11 | 100 | Homebrew recipes (batch 8) |
| 12 | 100 | Homebrew recipes (batch 9) |
| 13 | 100 | Homebrew recipes (batch 10) |
| 14 | 100 | Homebrew recipes (batch 11) |
| 15 | 100 | Homebrew recipes (batch 12) |
| 16 | 100 | Homebrew recipes (batch 13) |
| 17 | 100 | Homebrew recipes (batch 14) |
| 18 | 31 | Homebrew recipes (batch 15) |

## Bundling Constraint

Each dependency node (recipe depended upon by others) is bundled with
at least min(3, N) of its N dependents in the same PR, ensuring
libraries can be validated at runtime.

## PR 1: Non-recipe files + modified recipes

**34 recipes + 411 non-recipe files**

### Non-recipe files (411)

<details>
<summary>Click to expand</summary>

- `.github/CODEOWNERS`
- `.github/ci-batch-config.json`
- `.github/scripts/checks/ci-patterns-lint.sh`
- `.github/workflows/batch-generate.yml`
- `.github/workflows/build-essentials.yml`
- `.github/workflows/cargo-builder-tests.yml`
- `.github/workflows/check-artifacts.yml`
- `.github/workflows/drift-check.yml`
- `.github/workflows/gem-builder-tests.yml`
- `.github/workflows/integration-tests.yml`
- `.github/workflows/lint-workflows.yml`
- `.github/workflows/npm-builder-tests.yml`
- `.github/workflows/platform-integration.yml`
- `.github/workflows/pypi-builder-tests.yml`
- `.github/workflows/recipe-validation-core.yml`
- `.github/workflows/release.yml`
- `.github/workflows/sandbox-tests.yml`
- `.github/workflows/test-changed-recipes.yml`
- `.github/workflows/test-recipe.yml`
- `.github/workflows/test.yml`
- `.github/workflows/update-queue-status.yml`
- `.github/workflows/validate-golden-execution.yml`
- `.github/workflows/validate-recipe-golden-files.yml`
- `.github/workflows/website-ci.yml`
- `.goreleaser.yaml`
- `CONTRIBUTING.md`
- `Makefile`
- `README.md`
- `batch-control.json`
- `cmd/queue-maintain/main.go`
- `cmd/tsuku/create.go`
- `cmd/tsuku/create_test.go`
- `cmd/tsuku/helpers.go`
- `cmd/tsuku/info.go`
- `cmd/tsuku/install.go`
- `cmd/tsuku/install_deps.go`
- `cmd/tsuku/install_lib.go`
- `cmd/tsuku/install_sandbox.go`
- `cmd/tsuku/install_sandbox_test.go`
- `cmd/tsuku/install_test.go`
- `cmd/tsuku/llm.go`
- `cmd/tsuku/llm_test.go`
- `cmd/tsuku/main.go`
- `cmd/tsuku/plan_install.go`
- `cmd/tsuku/update_registry.go`
- `cmd/tsuku/validate.go`
- `cmd/tsuku/verify.go`
- `container-images.json`
- `data/README.md`
- `data/dep-mapping.json`
- `data/failures/batch-2026-02-22T22-42-09Z.jsonl`
- `data/failures/batch-2026-02-23T02-26-39Z.jsonl`
- `data/failures/homebrew-2026-02-22T22-25-55Z.jsonl`
- `data/failures/homebrew-2026-02-23T02-08-32Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T15-32-17Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T16-31-54Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T17-31-24Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T18-41-07Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T19-33-18Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T20-40-59Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T21-32-24Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T22-30-24Z.jsonl`
- `data/metrics/batch-runs-2026-02-21T23-25-13Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T02-01-26Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T04-14-17Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T04-22-46Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T05-56-07Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T06-47-48Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T07-37-58Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T08-32-27Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T09-32-54Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T10-25-55Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T11-22-37Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T12-46-32Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T13-45-18Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T14-27-18Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T15-25-37Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T16-31-31Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T17-27-00Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T18-35-26Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T19-26-10Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T20-29-30Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T22-42-09Z.jsonl`
- `data/metrics/batch-runs-2026-02-22T23-38-42Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T02-26-40Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T04-29-44Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T06-10-42Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T08-05-13Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T08-50-06Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T09-56-08Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T10-52-52Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T11-41-46Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T13-03-37Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T14-14-21Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T15-48-44Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T16-58-58Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T18-05-19Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T19-12-59Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T19-22-15Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T20-45-22Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T21-46-15Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T22-42-47Z.jsonl`
- `data/metrics/batch-runs-2026-02-23T23-48-23Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T02-00-15Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T04-21-08Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T06-01-37Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T07-08-16Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T08-47-01Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T09-54-25Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T10-49-36Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T11-58-16Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T13-05-31Z.jsonl`
- `data/metrics/batch-runs-2026-02-24T14-20-41Z.jsonl`
- `data/queues/priority-queue.json`
- `data/schemas/failure-record.schema.json`
- `docs/ENVIRONMENT.md`
- `docs/GUIDE-local-llm.md`
- `docs/ci-patterns.md`
- `docs/designs/DESIGN-registry-scale-strategy.md`
- `docs/designs/DESIGN-sandbox-image-unification.md`
- `docs/designs/current/DESIGN-batch-platform-validation.md`
- `docs/designs/current/DESIGN-batch-recipe-generation.md`
- `docs/designs/current/DESIGN-binary-name-discovery.md`
- `docs/designs/current/DESIGN-ci-job-consolidation.md`
- `docs/designs/current/DESIGN-ci-macos-batching.md`
- `docs/designs/current/DESIGN-container-image-digest-pinning.md`
- `docs/designs/current/DESIGN-discovery-registry-bootstrap.md`
- `docs/designs/current/DESIGN-ecosystem-name-resolution.md`
- `docs/designs/current/DESIGN-embedded-recipe-musl-coverage.md`
- `docs/designs/current/DESIGN-gem-exec-wrappers.md`
- `docs/designs/current/DESIGN-golden-plan-testing.md`
- `docs/designs/current/DESIGN-library-recipe-generation.md`
- `docs/designs/current/DESIGN-local-llm-runtime.md`
- `docs/designs/current/DESIGN-merge-job-completion.md`
- `docs/designs/current/DESIGN-pipeline-dashboard-overhaul.md`
- `docs/designs/current/DESIGN-priority-queue.md`
- `docs/designs/current/DESIGN-r2-golden-storage.md`
- `docs/designs/current/DESIGN-recipe-builders.md`
- `docs/designs/current/DESIGN-recipe-coverage-system.md`
- `docs/designs/current/DESIGN-recipe-registry-separation.md`
- `docs/designs/current/DESIGN-requeue-on-recipe-merge.md`
- `docs/designs/current/DESIGN-sandbox-ci-integration.md`
- `docs/designs/current/DESIGN-structured-error-subcategories.md`
- `docs/designs/current/DESIGN-system-lib-backfill.md`
- `docs/deterministic-builds/ecosystem_cargo.md`
- `docs/friction-log-library-recipes.md`
- `docs/library-backfill-ranked.md`
- `docs/runbooks/batch-operations.md`
- `docs/workflow-validation-guide.md`
- `internal/actions/action.go`
- `internal/actions/cargo_build.go`
- `internal/actions/cargo_build_test.go`
- `internal/actions/cargo_install.go`
- `internal/actions/cmake_build.go`
- `internal/actions/cmake_build_test.go`
- `internal/actions/composites.go`
- `internal/actions/composites_test.go`
- `internal/actions/configure_make.go`
- `internal/actions/configure_make_test.go`
- `internal/actions/download.go`
- `internal/actions/download_cache.go`
- `internal/actions/download_file.go`
- `internal/actions/gem_common.go`
- `internal/actions/gem_exec.go`
- `internal/actions/gem_exec_test.go`
- `internal/actions/gem_install.go`
- `internal/actions/install_binaries.go`
- `internal/actions/install_binaries_test.go`
- `internal/actions/install_gem_direct.go`
- `internal/actions/meson_build.go`
- `internal/actions/meson_build_test.go`
- `internal/actions/util.go`
- `internal/actions/util_test.go`
- `internal/batch/bootstrap.go`
- `internal/batch/orchestrator.go`
- `internal/batch/orchestrator_test.go`
- `internal/batch/results.go`
- `internal/blocker/blocker.go`
- `internal/blocker/blocker_test.go`
- `internal/builders/artifact.go`
- `internal/builders/artifact_test.go`
- `internal/builders/baseline_test.go`
- `internal/builders/binary_names.go`
- `internal/builders/binary_names_test.go`
- `internal/builders/builder.go`
- `internal/builders/cargo.go`
- `internal/builders/cargo_test.go`
- `internal/builders/cask.go`
- `internal/builders/cpan.go`
- `internal/builders/gem.go`
- `internal/builders/gem_artifact_test.go`
- `internal/builders/gem_test.go`
- `internal/builders/github_release.go`
- `internal/builders/go.go`
- `internal/builders/go_test.go`
- `internal/builders/homebrew.go`
- `internal/builders/homebrew_test.go`
- `internal/builders/llm_integration_test.go`
- `internal/builders/npm.go`
- `internal/builders/npm_test.go`
- `internal/builders/orchestrator.go`
- `internal/builders/pypi.go`
- `internal/builders/pypi_test.go`
- `internal/builders/pypi_wheel_test.go`
- `internal/containerimages/container-images.json`
- `internal/containerimages/containerimages.go`
- `internal/containerimages/containerimages_test.go`
- `internal/dashboard/dashboard.go`
- `internal/dashboard/dashboard_test.go`
- `internal/dashboard/failures.go`
- `internal/dashboard/failures_test.go`
- `internal/discover/llm_discovery.go`
- `internal/executor/executor.go`
- `internal/executor/executor_test.go`
- `internal/executor/plan.go`
- `internal/executor/plan_cache.go`
- `internal/executor/plan_cache_test.go`
- `internal/executor/plan_generator.go`
- `internal/executor/plan_test.go`
- `internal/executor/plan_verify.go`
- `internal/executor/plan_verify_test.go`
- `internal/executor/system_deps.go`
- `internal/executor/system_deps_test.go`
- `internal/llm/addon/manager.go`
- `internal/llm/addon/manager_test.go`
- `internal/llm/addon/prompter.go`
- `internal/llm/addon/prompter_test.go`
- `internal/llm/factory.go`
- `internal/llm/factory_test.go`
- `internal/llm/local.go`
- `internal/llm/local_e2e_test.go`
- `internal/llm/local_test.go`
- `internal/progress/spinner.go`
- `internal/progress/spinner_test.go`
- `internal/recipe/coverage.go`
- `internal/recipe/coverage_test.go`
- `internal/recipe/loader.go`
- `internal/recipe/loader_test.go`
- `internal/recipe/policy_test.go`
- `internal/recipe/recipes/bash.toml`
- `internal/recipe/recipes/gcc-libs.toml`
- `internal/recipe/recipes/go.toml`
- `internal/recipe/recipes/nodejs.toml`
- `internal/recipe/recipes/openssl.toml`
- `internal/recipe/recipes/patchelf.toml`
- `internal/recipe/recipes/perl.toml`
- `internal/recipe/recipes/python-standalone.toml`
- `internal/recipe/recipes/ruby.toml`
- `internal/recipe/recipes/rust.toml`
- `internal/recipe/recipes/zig.toml`
- `internal/recipe/satisfies_test.go`
- `internal/recipe/types.go`
- `internal/recipe/types_test.go`
- `internal/recipe/validate.go`
- `internal/recipe/validate_test.go`
- `internal/recipe/validator.go`
- `internal/recipe/validator_test.go`
- `internal/recipe/writer.go`
- `internal/recipe/writer_test.go`
- `internal/registry/manifest.go`
- `internal/registry/manifest_test.go`
- `internal/reorder/reorder.go`
- `internal/reorder/reorder_test.go`
- `internal/requeue/requeue.go`
- `internal/requeue/requeue_test.go`
- `internal/sandbox/container_spec.go`
- `internal/sandbox/container_spec_test.go`
- `internal/sandbox/executor.go`
- `internal/sandbox/executor_test.go`
- `internal/sandbox/requirements.go`
- `internal/sandbox/requirements_test.go`
- `internal/sandbox/sandbox_integration_test.go`
- `internal/telemetry/client.go`
- `internal/telemetry/event.go`
- `internal/telemetry/event_test.go`
- `internal/testutil/testutil.go`
- `internal/testutil/testutil_test.go`
- `internal/validate/eval_plan_integration_test.go`
- `internal/validate/executor.go`
- `internal/validate/executor_test.go`
- `internal/validate/source_build_test.go`
- `renovate.json`
- `scripts/check-pipeline-links.sh`
- `scripts/fix-recipe-errors.py`
- `scripts/generate-registry.py`
- `scripts/regenerate-golden.sh`
- `scripts/requeue-unblocked.sh`
- `scripts/test_generate_registry.py`
- `test-matrix.json`
- `test/functional/features/create.feature`
- `test/scripts/test-checksum-pinning.sh`
- `testdata/golden/plans/embedded/bash/v5.3.9-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/bash/v5.3.9-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/bash/v5.3.9-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/bash/v5.3.9-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/bash/v5.3.9-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/ca-certificates/v2025-12-02-darwin-amd64.json`
- `testdata/golden/plans/embedded/ca-certificates/v2025-12-02-darwin-arm64.json`
- `testdata/golden/plans/embedded/ca-certificates/v2025-12-02-linux-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-darwin-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-darwin-arm64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/gcc-libs/v15.2.0-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/gcc-libs/v15.2.0-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/gcc-libs/v15.2.0-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/gcc-libs/v15.2.0-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/gcc-libs/v15.2.0-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/go/v1.25.7-darwin-amd64.json`
- `testdata/golden/plans/embedded/go/v1.25.7-darwin-arm64.json`
- `testdata/golden/plans/embedded/go/v1.25.7-linux-amd64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-darwin-amd64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-darwin-arm64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/libyaml/v0.2.5-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-darwin-amd64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-darwin-arm64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/make/v4.4.1-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/meson/v1.9.2-darwin-amd64.json`
- `testdata/golden/plans/embedded/meson/v1.9.2-darwin-arm64.json`
- `testdata/golden/plans/embedded/meson/v1.9.2-linux-amd64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-darwin-amd64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-darwin-arm64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/ninja/v1.13.2-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-darwin-amd64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-darwin-arm64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/openssl/v3.6.1-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-darwin-amd64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-darwin-arm64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/patchelf/v0.18.0-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-darwin-amd64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-darwin-arm64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/perl/v5.42.0.0-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-darwin-amd64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-darwin-arm64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/pkg-config/v2.5.1-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-darwin-amd64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-darwin-arm64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/python-standalone/v20251217-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-darwin-amd64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-darwin-arm64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/ruby/v4.0.0-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-darwin-amd64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-darwin-arm64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/rust/v1.93.1-linux-suse-amd64.json`
- `testdata/golden/plans/embedded/zig/v0.14.1-darwin-amd64.json`
- `testdata/golden/plans/embedded/zig/v0.14.1-darwin-arm64.json`
- `testdata/golden/plans/embedded/zig/v0.14.1-linux-amd64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-darwin-amd64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-darwin-arm64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-linux-alpine-amd64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-linux-arch-amd64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/zlib/v1.3.1-linux-suse-amd64.json`
- `website/coverage/coverage.json`
- `website/pipeline/blocked.html`
- `website/pipeline/curated.html`
- `website/pipeline/dashboard.json`
- `website/pipeline/disambiguations.html`
- `website/pipeline/failure.html`
- `website/pipeline/failures.html`
- `website/pipeline/index.html`
- `website/pipeline/package.html`
- `website/pipeline/pending.html`
- `website/pipeline/requires_manual.html`
- `website/pipeline/run.html`
- `website/pipeline/runs.html`
- `website/pipeline/success.html`

</details>

### Recipes (34)

<details>
<summary>Click to expand</summary>

- abseil, angle-grinder, ansifilter, anycable-go, anyquery, apr-util
- bash, brotli
- cairo, cuda-runtime
- expat
- fontconfig
- geos, gettext, giflib, git, gmp
- jpeg-turbo
- libcurl, libnghttp2, libnghttp3, libngtcp2, libpng, libssh2, libxml2
- mesa-vulkan-drivers
- pcre2, pngcrush, proj
- readline
- spatialite, sqlite
- vulkan-loader
- zstd

</details>

## PR 2: Crates.io / cargo_install recipes

**103 recipes**

### Recipes (103)

<details>
<summary>Click to expand</summary>

- b3sum, bindgen, bitcoin, blake3, boring, boringtun, bsdiff, bzip2
- cadence, capnp, cargo-expand, cargo-geiger, cargo-generate, cargo-hack, cargo-llvm-cov, cargo-nextest, cargo-outdated, cargo-release, cargo-sweep, cargo-udeps, cargo-update, cargo-zigbuild, cbc, cidr, codex, cpio
- datafusion, diesel, diskus, dlib, dotenv-linter, dotslash
- espflash, evtx
- fastrace, frizbee
- gifski, gleam, googletest, grok
- h2, hashlink, hidapi, hstr
- jack, jid
- keyring, killport, komac
- librespot, limine, litra, livekit, lnk
- mac, maturin, mcap, mdbook, mimalloc, minisign, mpack
- nb, neofetch, neon, nom, numpy
- openh264, oxipng
- pfetch, pinocchio, pipewire, pmtiles, probe-rs-tools, py-spy
- radicle, rav1e, resvg, retry, rocksdb
- scc, scip, secp256k1, semver, sequoia-sq, snap, snowflake, spirv-cross, sqlx-cli, swc
- taplo, termshark, try-rs, turso, typst, typstfmt
- v8, vcpkg
- wasm-bindgen, wasmer
- yap, yara-x, yoke
- zopfli

</details>

## PR 3: RubyGems / gem_install recipes

**83 recipes**

### Recipes (83)

<details>
<summary>Click to expand</summary>

- bettercap, bullet, bup
- cassandra, cdo, chamber, chisel, chroma, cog, conduit, cowsay, cql
- dict, doggo, dug
- exiv2
- fastlane, flash
- gitingest, glassfish, global, gsl
- haproxy, hercules, htmldoc, httpx, hydra, hyperkit
- imgproxy, influxdb, ipfs
- jsonlint
- kor
- lemon, less, libnotify, lsof, lxc
- mailcatcher, mdv, medusa, meli, memcached, mktorrent, multimarkdown
- neo4j, nkf, notifiers, nut, nyan
- ocp, overmind
- pandocomatic, parallel, passenger, pastel, pdftohtml, pidof, pioneer, pngcheck, progress, pwned
- quickjs
- re2, redis, rename, rsync
- scalingo, sesh, snappy, solargraph, spek, swag
- terminator, tin, tldr, toxiproxy
- uni, unicorn
- webhook, whois
- xsv
- yamllint

</details>

## PR 4: Homebrew recipes (batch 1)

**23 recipes**

### Recipes (23)

<details>
<summary>Click to expand</summary>

- a2ps, abook, argtable3, astroterm, asymptote
- bdw-gc, beagle
- coturn
- fstrm
- gflags
- hiredis, hwloc
- i2pd
- lbdb, libevent, libgrape-lite, libpaper
- miniupnpc, mmv, mrbayes
- open-mpi, opencoarrays
- transmission-cli

</details>

### Dependency nodes (11)

- **abook**: 1/1 dependents [ok]
- **argtable3**: 1/1 dependents [ok]
- **bdw-gc**: 3/3 dependents [ok]
- **beagle**: 1/1 dependents [ok]
- **gflags**: 1/1 dependents [ok]
- **hiredis**: 1/1 dependents [ok]
- **hwloc**: 1/1 dependents [ok]
- **libevent**: 4/3 dependents [ok]
- **libpaper**: 1/1 dependents [ok]
- **miniupnpc**: 2/2 dependents [ok]
- **open-mpi**: 3/3 dependents [ok]

## PR 5: Homebrew recipes (batch 2)

**99 recipes**

### Recipes (99)

<details>
<summary>Click to expand</summary>

- atomicparsley, avrdude, avro-c
- bazarr, bedops, berkeley-db
- capstone, cfitsio, chrony, cjson, cln, ctags-lsp
- dbus, desktop-file-utils, dfu-programmer, dfu-util, diff-pdf, diffnav, dump1090-fa
- eiffelstudio, enter-tex
- ffmpeg, ffmpegthumbnailer, ffms2, fltk
- gerbv, get-iplayer, ginac, git-credential-libsecret, git-delta, glfw, glib, glslviewer, gnucobol, gnutls, gsasl, gtk-gnutella, gtk-vnc, gtranslator
- healpix, hopenpgp-tools
- inetutils
- jansson, jq, json-c, json-glib, jsonrpc-glib
- libaacs, libassuan, libbladerf, libgcrypt, libgee, libgit2, libgit2-glib, libgpg-error, libgsf, libidn2, libjwt, libmicrohttpd, libnfc, libpsl, libpst, libqalculate, librealsense, librist, librtlsdr, libsecret, libunistring, libusb, libusb-compat, libvncserver, libxmlsec1, limesuite, lnav
- minipro, mpg123, msmtp
- nettle, newsboat
- oath-toolkit, onedrive-cli, oniguruma, open-ocd, operator-sdk
- pcb2gcode, pianobar, pianod
- qalculate-gtk
- rdfind, readsb, rmlint, rtl-433
- srecord, srt
- tsduck
- undercutf1, universal-ctags
- wcslib
- x11vnc

</details>

### Dependency nodes (41)

- **atomicparsley**: 1/1 dependents [ok]
- **berkeley-db**: 1/1 dependents [ok]
- **capstone**: 1/1 dependents [ok]
- **cfitsio**: 2/2 dependents [ok]
- **cjson**: 1/1 dependents [ok]
- **cln**: 1/1 dependents [ok]
- **dbus**: 2/2 dependents [ok]
- **ffmpeg**: 7/3 dependents [ok]
- **fltk**: 1/1 dependents [ok]
- **gerbv**: 1/1 dependents [ok]
- **git-delta**: 1/1 dependents [ok]
- **glfw**: 2/2 dependents [ok]
- **glib**: 20/3 dependents [ok]
- **gnutls**: 7/3 dependents [ok]
- **jansson**: 4/3 dependents [ok]
- **json-c**: 3/3 dependents [ok]
- **json-glib**: 3/3 dependents [ok]
- **libassuan**: 1/1 dependents [ok]
- **libbladerf**: 1/1 dependents [ok]
- **libgcrypt**: 7/3 dependents [ok]
- **libgee**: 1/1 dependents [ok]
- **libgit2**: 2/2 dependents [ok]
- **libgpg-error**: 4/3 dependents [ok]
- **libgsf**: 1/1 dependents [ok]
- **libidn2**: 3/3 dependents [ok]
- **libmicrohttpd**: 1/1 dependents [ok]
- **libqalculate**: 1/1 dependents [ok]
- **librist**: 1/1 dependents [ok]
- **librtlsdr**: 3/3 dependents [ok]
- **libsecret**: 1/1 dependents [ok]
- **libunistring**: 2/2 dependents [ok]
- **libusb**: 12/3 dependents [ok]
- **libusb-compat**: 2/2 dependents [ok]
- **libvncserver**: 1/1 dependents [ok]
- **libxmlsec1**: 1/1 dependents [ok]
- **mpg123**: 1/1 dependents [ok]
- **nettle**: 3/3 dependents [ok]
- **oniguruma**: 2/2 dependents [ok]
- **srecord**: 1/1 dependents [ok]
- **srt**: 1/1 dependents [ok]
- **universal-ctags**: 1/1 dependents [ok]

## PR 6: Homebrew recipes (batch 3)

**99 recipes**

### Recipes (99)

<details>
<summary>Click to expand</summary>

- aom, audiowaveform
- cdrdao, chronograf, clazy, clifm, clisp, concurrencykit, confuse, console-bridge, coreutils, cryfs, cunit
- docker-machine, docker-machine-driver-vmware, dwarfutils
- e2fsprogs, elan-init, etcd
- fastp, fatsort, feh, fheroes2, file-formula, fmt, fq, fwup
- gnu-getopt, goaccess, gopass, gopass-jsonapi, govulncheck, gptfdisk, gputils
- help2man
- ic-wasm, icp-cli, ideviceinstaller, imlib2, innoextract, isa-l
- jdupes, jlog, joern, jp2a, jpeg-xl, jruby
- kapacitor, katago, kcov
- lame, ldid, ldid-procursus, libcaca, libcbor, libcdio, libcdio-paranoia, libconfig, libdeflate, libdnet, libdvdcss, libexif, libffcall, libfido2, libfixposix, libid3tag, libimobiledevice, libimobiledevice-glue, libirecovery, libiscsi, libjodycode, libmagic, libmaxminddb, libmodbus, libplist, libsigsegv, libsndfile, libusbmuxd, libvmaf, libzip, lsdvd
- mad, mbpoll, mender-artifact, mplayer, mtools
- php, popt, pstoedit, pyenv-virtualenv
- rabbitmq-c
- sdcc, shairport-sync
- tcpreplay, tinyxml2
- urdfdom
- vitess
- xk6, xmlto

</details>

### Dependency nodes (50)

- **concurrencykit**: 1/1 dependents [ok]
- **confuse**: 1/1 dependents [ok]
- **console-bridge**: 1/1 dependents [ok]
- **coreutils**: 4/3 dependents [ok]
- **cunit**: 1/1 dependents [ok]
- **docker-machine**: 1/1 dependents [ok]
- **dwarfutils**: 1/1 dependents [ok]
- **e2fsprogs**: 1/1 dependents [ok]
- **etcd**: 1/1 dependents [ok]
- **fmt**: 1/1 dependents [ok]
- **gnu-getopt**: 1/1 dependents [ok]
- **gopass**: 1/1 dependents [ok]
- **govulncheck**: 1/1 dependents [ok]
- **gputils**: 1/1 dependents [ok]
- **help2man**: 1/1 dependents [ok]
- **ic-wasm**: 1/1 dependents [ok]
- **imlib2**: 2/2 dependents [ok]
- **innoextract**: 1/1 dependents [ok]
- **isa-l**: 1/1 dependents [ok]
- **jlog**: 1/1 dependents [ok]
- **jpeg-xl**: 1/1 dependents [ok]
- **kapacitor**: 1/1 dependents [ok]
- **lame**: 1/1 dependents [ok]
- **libcaca**: 1/1 dependents [ok]
- **libcbor**: 1/1 dependents [ok]
- **libcdio**: 1/1 dependents [ok]
- **libconfig**: 1/1 dependents [ok]
- **libdeflate**: 1/1 dependents [ok]
- **libdnet**: 1/1 dependents [ok]
- **libdvdcss**: 1/1 dependents [ok]
- **libexif**: 2/2 dependents [ok]
- **libffcall**: 1/1 dependents [ok]
- **libfixposix**: 1/1 dependents [ok]
- **libid3tag**: 1/1 dependents [ok]
- **libimobiledevice**: 1/1 dependents [ok]
- **libimobiledevice-glue**: 2/2 dependents [ok]
- **libjodycode**: 1/1 dependents [ok]
- **libmagic**: 2/2 dependents [ok]
- **libmaxminddb**: 1/1 dependents [ok]
- **libmodbus**: 1/1 dependents [ok]
- **libplist**: 6/3 dependents [ok]
- **libsigsegv**: 1/1 dependents [ok]
- **libsndfile**: 1/1 dependents [ok]
- **libvmaf**: 1/1 dependents [ok]
- **libzip**: 3/3 dependents [ok]
- **mad**: 2/2 dependents [ok]
- **mtools**: 1/1 dependents [ok]
- **php**: 1/1 dependents [ok]
- **popt**: 3/3 dependents [ok]
- **tinyxml2**: 1/1 dependents [ok]

## PR 7: Homebrew recipes (batch 4)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- aerc
- baresip
- coin3d, core-lightning
- djvulibre, dovecot, dvisvgm
- erlang-language-platform
- fancy-cat
- groff, gromacs
- inspectrum
- karchive, kdoctools
- libbluray, libebur128, libgeotiff, libmng, libmpdclient, libngspice, libogg, libomp, libpcap, libpg-query, libpipeline, libptytty, libre, libsodium, libtasn1, libtiff, libtommath, libtorrent-rakshasa, libudfread, libxc, lighttpd, little-cms2, lmfit, lz4
- man-db, min-lang, mmseqs2, msgpack, mujs
- ncmpcpp, neovim-qt, ngrep, ngspice, nim, notmuch, notmuch-mutt, nspr, nss
- ocaml, openldap, osmcoastline, osmium-tool
- p11-kit, pandoc, pandoc-crossref, pandoc-plot, par2, pixman, postgres-language-server, potrace, protobuf, protobuf-c, protoc-gen-doc, protoc-gen-go, protoc-gen-grpc-swift, pure-ftpd
- qtbase, qtcharts, qtdatavis3d, qtdeclarative, qtlottie, qtmultimedia, qtquick3d, qtquick3dphysics, qtquickeffectmaker, qtserialbus, qtserialport, qtshadertools, qtsvg
- rebar3, rlwrap, rocq, rtorrent
- samba, sambamba, speex, sqlite-analyzer, swift-protobuf
- tcl-tk, tcpdump, termscp
- uchardet
- vorbis-tools, votca
- xmlrpc-c, xorg-server

</details>

### Dependency nodes (46)

- **groff**: 1/1 dependents [ok]
- **gromacs**: 1/1 dependents [ok]
- **karchive**: 1/1 dependents [ok]
- **libmpdclient**: 1/1 dependents [ok]
- **libngspice**: 1/1 dependents [ok]
- **libogg**: 2/2 dependents [ok]
- **libomp**: 4/3 dependents [ok]
- **libpcap**: 2/2 dependents [ok]
- **libpg-query**: 1/1 dependents [ok]
- **libpipeline**: 1/1 dependents [ok]
- **libptytty**: 1/1 dependents [ok]
- **libre**: 1/1 dependents [ok]
- **libsodium**: 2/2 dependents [ok]
- **libtasn1**: 1/1 dependents [ok]
- **libtiff**: 3/3 dependents [ok]
- **libtommath**: 2/2 dependents [ok]
- **libtorrent-rakshasa**: 1/1 dependents [ok]
- **libudfread**: 1/1 dependents [ok]
- **libxc**: 1/1 dependents [ok]
- **little-cms2**: 1/1 dependents [ok]
- **lmfit**: 1/1 dependents [ok]
- **lz4**: 3/3 dependents [ok]
- **msgpack**: 1/1 dependents [ok]
- **mujs**: 1/1 dependents [ok]
- **nim**: 1/1 dependents [ok]
- **notmuch**: 2/2 dependents [ok]
- **nspr**: 1/1 dependents [ok]
- **ocaml**: 1/1 dependents [ok]
- **openldap**: 2/2 dependents [ok]
- **pandoc**: 2/2 dependents [ok]
- **pixman**: 1/1 dependents [ok]
- **potrace**: 1/1 dependents [ok]
- **protobuf**: 5/3 dependents [ok]
- **qtbase**: 16/3 dependents [ok]
- **qtdeclarative**: 6/3 dependents [ok]
- **qtquick3d**: 3/3 dependents [ok]
- **qtserialport**: 1/1 dependents [ok]
- **qtshadertools**: 3/3 dependents [ok]
- **qtsvg**: 2/2 dependents [ok]
- **rebar3**: 1/1 dependents [ok]
- **samba**: 1/1 dependents [ok]
- **speex**: 1/1 dependents [ok]
- **swift-protobuf**: 1/1 dependents [ok]
- **tcl-tk**: 1/1 dependents [ok]
- **uchardet**: 1/1 dependents [ok]
- **xmlrpc-c**: 1/1 dependents [ok]

## PR 8: Homebrew recipes (batch 5)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- a52dec, aarch64-elf-gcc, aarch64-elf-gdb, ada-url, afflib, aircrack-ng, alda, ali, allureofthestars, aoe, apache-polaris, apt, argocd-autopilot, argocd-vault-plugin, argyll-cms, aribb24, arm-linux-gnueabihf-binutils, arm-none-eabi-binutils, arm-none-eabi-gcc, arm-none-eabi-gdb, arp-scan-rs, asc, asnmap, aspell, assimp, astrometry-net, atlas, audacious, auditbeat, autobrr, avro-cpp, aws-lc, aws-rotate-key, azqr, aztfexport, azure-dev, azurehound
- baidupcs-go, bareos-client, bashdb, bat-extras, bazel, bbtools, bcftools, bchunk, beads-viewer, bedtools, benthos, bento4, biber, bigloo, binutils, bkcrack, bnfc, bochs, boost-build, bosh-cli, bowtie2, brev, brpc, bstring, build2, buildifier, bulk-extractor, bwa, byacc
- c2048, c3c, cabal-install, cabextract, cadence-workflow, cagent, calcurse, calicoctl, carapace, cargo-flamegraph, cariddi, cbonsai, cc65, ccache, ccat, ccextractor, clang-uml
- fastfetch, freetds
- libdc1394
- ocicl, open-simh
- prog8
- sbcl, sdl12-compat
- tass64
- unixodbc
- vde
- wuppiefuzz, wxmaxima, wxwidgets
- yaml-cpp, yyjson
- z3

</details>

### Dependency nodes (9)

- **sbcl**: 1/1 dependents [ok]
- **sdl12-compat**: 1/1 dependents [ok]
- **tass64**: 1/1 dependents [ok]
- **unixodbc**: 1/1 dependents [ok]
- **vde**: 1/1 dependents [ok]
- **wxwidgets**: 1/1 dependents [ok]
- **yaml-cpp**: 1/1 dependents [ok]
- **yyjson**: 1/1 dependents [ok]
- **z3**: 1/1 dependents [ok]

## PR 9: Homebrew recipes (batch 6)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- ccls, ccrypt, cdogs-sdl, cdrtools, cdxgen, certigo, cf-terraforming, cfr-decompiler, cgns, chainloop-cli, chapel, chart-releaser, chart-testing, chdig, checkmake, chezmoi, chkrootkit, chocolate-doom, choose-rust, chsrc, cilium-cli, clang-build-analyzer, claws-mail, clinfo, cljfmt, cloud-provider-kind, cloud-sql-proxy, cloudflare-quiche, cloudfox, cloudlist, cloudquery, clusterawsadm, clusterctl, cmark, cmix, coccinelle, code-cli, coder, codex-acp, colmap, config-file-validator, conftest, consul-template, container-structure-test, convmv, corsixth, cp2k, cppi, cppunit, cpufetch, cracklib, cri-tools, crosstool-ng, cscope, csvq, csvtk, cyclonedx-gomod
- dagger, dalfox, damask-grid, dar, darcs, darkice, dash-shell, datamash, dav1d, dblab, dcmtk, ddclient, ddcutil, ddns-go, ddrescue, deck, dependabot, depot, dhall-lsp-server, dhall-yaml, diffoci, diffutils, difi, discount, dissent, distcc, dnscontrol, dnsperf, dnspyre, dnsx, docker-gen, docker-language-server, docker-ls, dockerfilegraph, dockerfmt, doltgres, doppler, dosbox-staging, dosbox-x, double-conversion, dovi-tool, dra, driftctl

</details>

## PR 10: Homebrew recipes (batch 7)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- drogon, drone-cli, dropbear, dstask, dyff, dylibbundler, dynare
- e1s, earthly, easyrpg-player, ecflow-ui, edbrowse, editorconfig, efm-langserver, egctl, emscripten, enca, enpass-cli, ente-cli, entr, epinio, epstool, ettercap, evans, evil-helix, excalidraw-converter, exempi, exploitdb
- faac, faad2, fail2ban, faircamp, fake-gcs-server, falco, falcoctl, fastly, fastnetmon, fceux, fcgi, fcrackzip, fdclone, fdk-aac, fdupes, feedgnuplot, felinks, ferron, fetchmail, ffmpeg-full, fig2dev, filebeat, firefoxpwa, flagd, flarectl, flow-control, fluent-bit, fluid-synth, fontforge, fortio, fourmolu, fping, freeradius-server, frei0r, fribidi, fricas, frotz, futhark, fwupd
- g-ls, gabo, gambit-scheme, game-music-emu, gammaray, gastown, gau, gauche, geeqie, gensio, geoipupdate, getparty, ghalint, ghcup, ghidra, ghorg, ghostunnel, ghz, gibo, git-credential-oauth, git-crypt, git-flow-next, git-pages-cli, git-sizer, git-spice, git-town, git-xet, gitea, gitea-mcp-server, gitlab-ci-linter, gitlogue, gitmux, gitsign

</details>

## PR 11: Homebrew recipes (batch 8)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- glm, glooctl, glpk, gmailctl, gnirehtet, gnmic, gnome-builder, gnome-papers, gnu-indent, gnu-typist, gnu-units, gnu-which, gnuastro, gnuradio, go-air, go-critic, go-feature-flag-relay-proxy, go-jira, go-jsonnet, go-md2man, go-parquet-tools, go-passbolt-cli, go-size-analyzer, goclone, goctl, gogcli, gojq, golang-migrate, golangci-lint-langserver, golines, google-authenticator-libpam, gops, goreleaser, gotests, gotestsum, gource, govc, gowall, gperf, grafana-alloy, granted, graphite2, greenmask, grep, grokj2k, grpcui, gsoap, gti, gtk-doc, guile, gumbo-parser, gupnp-tools, gwenhywfar, gwyddion, gyb, gzip
- haskell-language-server, haskell-stack, hatari, havener, hcledit, hcxtools, hdf5-mpi, headscale-cli, helm-docs, helm-ls, helmfile, helmify, hexedit, highway, hl, hledger, hlint, htmltest, httpd, humanlog
- i2c-tools, i386-elf-gdb, i686-elf-binutils, i686-elf-gcc, i686-elf-grub, iam-policy-json-to-terraform, ibazel, icann-rdap, icarus-verilog, icoutils, iir1, ijq, ike-scan, imagemagick-full, imapsync, imath, imessage-ruby, immich-go, include-what-you-use, incus, infisical, infracost, inframap, ingress2gateway

</details>

## PR 12: Homebrew recipes (batch 9)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- inih, iniparser, inotify-tools, intltool, ios-webkit-debug-proxy, ipinfo-cli, ipmitool, ipmiutil, ipsw, ipv6toolkit, ircii, irssi, isl, istioctl, ivykis
- jackett, jags, javacc, jbig2enc, jd, jemalloc, jet, jhead, jhipster, jimtcl, jjui, joker, jpeginfo, jq-lsp, jqfmt, jsoncpp, jsonnet-bundler, jsonschema2pojo, jwt-cli, jython
- k3sup, k8sgpt, kafkactl, kakoune, karmadactl, kbld, kepubify, kiota, kitex, kn, knot-resolver, kompose, kops, kosli-cli, kotlin-language-server, koto, krakend, krb5, kube-bench, kube-linter, kube-score, kubebuilder, kubecm, kubecolor, kubeconform, kubectl-ai, kubectl-cnpg, kubefirst, kubefwd, kubekey, kubelogin, kubent, kubeone, kubergrunt, kubescape, kubesess, kubeshark, kubetail, kubevela, kubevpn, kumactl, kyma-cli, kyverno
- lacework-cli, lanraragi, latex2html, latexindent, latexml, lavat, lazydocker, lazyjournal, lc0, lcdf-typetools, ldc, ldcli, ldns, legitify, leptonica, leveldb, lftp, libaec, libb2, libcoap, libcue, libdap

</details>

## PR 13: Homebrew recipes (batch 10)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- libde265, libepoxy, libev, libewf, libfyaml, libgphoto2, libgr, libheif, libidn, libiodbc, libiptcdata, liblo, libmatio, libnatpmp, libnet, libopenmpt, libpaho-mqtt, librasterlite2, libressl, librime, librttopo, libsamplerate, libsixel, libspiro, libssh, libtensorflow, libtpms, libtrace, libultrahdr, libuninameslist, libuv, libvidstab, libvpx, libvterm, libwapcaplet, libwebm, libwmf, libxcrypt, libxls, libxlsxwriter, libxmp, libxpm, linkerd, livekit-cli, llama.cpp, lld, llgo, lmdb, localai, log4cpp, logdy, logstalgia, lpeg, ltex-ls, luajit-openresty, lzip, lzlib, lzo
- m4, macpine, mafft, mailpit, mailutils, makedepend, malcontent, mapserver, mariadb-connector-c, marisa, mark, markdown-oxide, masscan, massdriver, mcp-publisher, mcphost, mcptools, md4c, mediamtx, memtester, mender-cli, mesheryctl, metals, metis, mgba, micromamba, microsocks, midnight-commander, mimirtool, minicom, minimal-racket, minio, minio-mc, minio-warp, minizip, mintoolkit, mlt, mmctl, moarvm, mob, mods, mongo-c-driver

</details>

## PR 14: Homebrew recipes (batch 11)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- mongocli, moon-buggy, mp4v2, mpdecimal, mpfr, mq, msc-generator, msitools, multitail, mupdf-tools, murex, musikcube, mutt, mydumper, mysql-client
- naabu, nanobot, nats-server, nauty, nbping, nbsdgames, ncftp, neo4j-mcp, neomutt, net-snmp, netatalk, netcdf-fortran, newrelic-cli, newrelic-infra-agent, nextdns, nexttrace, nfpm, nikto, ninvaders, nip4, nixfmt, nmrpflash, noseyparker, nudoku, nvtop, nwchem
- oasdiff, oauth2c, oauth2l, obfs4proxy, octomap, oils-for-unix, okta-aws-cli, okteto, omniorb, open-image-denoise, openal-soft, openbao, opencore-amr, openfga, openfortivpn, openmsx, openrtsp, opensc, opensearch-dashboards, opkssh, opus, ormolu, osm2pgrouting, osm2pgsql, osslsigncode, osv-scanner
- packetbeat, partio, payload-dumper-go, pbzip2, pc6001vx, pcapmirror, pcl, pcsc-lite, pdfcpu, pdfcrack, pdfpc, pdftk-java, pdftoipe, pdnsrec, pdsh, percona-server, percona-toolkit, percona-xtrabackup, pg-partman, pgbackrest, pgstream, pgweb, phoneinfoga, phpbrew, phpstan, physfs, picat, picocom, picoruby, pigz, pinact, pipes-sh, pixlet

</details>

## PR 15: Homebrew recipes (batch 12)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- pixz, pjproject, pkcs11-helper, pkcs11-tools, pkgx, pkl, plakar, pluto, po4a, podman-tui, polaris, polynote, ponyc, poppler-qt5, portaudio, porter, povray, powerline-go, ppsspp, premake, projectm, promtail, protolint, proxychains-ng, pscale, pulsarctl, pvetui, pyqt, python-freethreading, python-tabulate
- qalculate-qt, qbittorrent-cli, qca, qdmr, qhull, qmmp, qo, qttools
- radare2, ragel, rakudo-star, rattler-build, rcm, re2c, readosm, regal, regclient, rekor-cli, reprepro, reproc, resterm, restic, retdec, rgbds, richgo, riscv64-elf-binutils, riscv64-elf-gcc, riscv64-elf-gdb, risor, rizin, rke, rkhunter, rom-tools, rosa-cli, rospo, roswell, rpds-py, rpm2cpio, rsgain, rtabmap, rtmpdump, rumdl, runit, rush-parallel, rxvt-unicode, ryelang
- s-lang, s-search, s5cmd, saml2aws, samtools, sane-backends, sc-im, scdoc, screenfetch, scummvm, scummvm-tools, sdl2-net, sdl3, seaweedfs, sentry-cli, sentry-native, seqkit, serie, sfsexp, sftpgo, shadowenv, shellshare, showkey, shtool

</details>

## PR 16: Homebrew recipes (batch 13)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- shuffledns, simdjson, simg2img, sing-box, sipcalc, sipsak, siril, skillshare, skopeo, skylighting, sleuthkit, smlfmt, sngrep, socat, soft-serve, softhsm, sonobuoy, sox-ng, spatialite-gui, spatialite-tools, speexdsp, spice-gtk, spin, spirv-llvm-translator, spoofdpi, spr, sq, sqlc, sqlcipher, sqlite-rsync, squashfs, sratoolkit, steampipe, stm32flash, strace, stress-ng, strongswan, stu, stuntman, stylish-haskell, suite-sparse, supertux, svt-av1, swift-format, swift-outdated, swiftdraw, swtpm, symfony-cli, synfig, syslog-ng
- tailwindcss-language-server, talhelper, talloc, talm, talosctl, tbls, tcpflow, tcsh, td, tdb, teamtype, tektoncd-cli, telegram-downloader, termshot, terraform-ls, terraform-provider-libvirt, terramaid, terramate, terratag, testdisk, testkube, texinfo, texmath, tf-summarize, tfcmt, tfstate-lookup, tftp-now, tfupdate, thors-anvil, threatcl, tidy-html5, tiger-vnc, timewarrior, tldx, tmuxai, tnftp, tock, todoist-cli, tofrodos, torsocks, tpix, transifex-cli, tre, tree-sitter, tronbyt-server, trzsz-go, trzsz-ssh, tty-clock, ttyd, tuios

</details>

## PR 17: Homebrew recipes (batch 14)

**100 recipes**

### Recipes (100)

<details>
<summary>Click to expand</summary>

- freeciv
- gabedit, gcab, gedit, gkrellm, gnu-apl, gplugin, gpredict, graphene, gsmartcontrol, gucharmap, gupnp, gupnp-av
- homebank
- klavaro
- lensfun, libdazzle, libdex, libfreenect, libksba, liblqr, libosinfo, libslirp, libtatsu, libxmlb
- mikutter
- twitch-cli, two-lame, two-ms, typos-lsp
- u-boot-tools, udunits, ugrep, umoci, uncrustify, unshield, unxip, urlview, utf8proc, utftex, util-linux, uutils-diffutils, uuu
- v2ray, vacuum, valgrind, vals, vcftools, vcluster, vespa-cli, vet, vgmstream, victorialogs, victoriametrics, vifm, virtctl, virustotal-cli, visp, vnstat, vsearch, vulkan-tools, vvenc
- wails, wakatime-cli, wavpack, werf, wgcf, wget2, whalebrew, whisper-cpp, whosthere, widelands, wifitui, wildmidi, wireguard-go, wireshark, woodpecker-cli
- x-cmd, x264, x265, x3270, xapian, xcsift, xdelta, xerces-c, xeyes, xfig, xorriso, xsel, xvid, xxhash
- youtubedr, ytt, yubico-piv-tool, yubikey-agent
- zeek, zimg, zlib-ng-compat, zls, zsh

</details>

## PR 18: Homebrew recipes (batch 15)

**31 recipes**

### Recipes (31)

<details>
<summary>Click to expand</summary>

- libsidplayfp
- navidrome
- openfpgaloader, openjph
- pdf2svg, pdfgrep, pioneers, powerman, pqiv, protoc-gen-go-grpc, protoc-gen-grpc-java
- qcachegrind, qtconnectivity, qtnetworkauth, qtquicktimeline, qtremoteobjects, qtscxml, qtsensors, qtwebchannel, qtwebsockets
- sdcv, shared-mime-info, sigrok-cli, sofia-sip, stlink, synergy-core
- tintin
- uhubctl, unbound, unpaper, usbredir

</details>

