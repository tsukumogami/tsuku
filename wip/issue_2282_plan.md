# Implementation Plan — Issue #2282

## Scope

Curate 5 already-handcrafted Kubernetes CLI recipes:
- `recipes/k/k9s.toml` — `github_archive`, `derailed/k9s` (note: no arch_mapping — relies on identity)
- `recipes/f/flux.toml` — `github_archive`, `fluxcd/flux2`
- `recipes/s/stern.toml` — `github_archive`, `stern/stern`
- `recipes/k/kubectx.toml` — `github_archive`, `ahmetb/kubectx` (arch_mapping uses `x86_64`)
- `recipes/k/kustomize.toml` — `github_archive`, `kubernetes-sigs/kustomize` (tag_prefix-unusual: `kustomize/v{version}`)

## Approach

Dispatch 5 parallel validation agents — same pattern as #2281. Each:
1. Fetches the latest upstream release via WebFetch.
2. Verifies recipe's asset pattern matches published assets for all 4 target platforms.
3. Reports PASS / FIX with recommended TOML changes.

Then apply the `curated = true` flag, split `github_archive` steps into per-OS if the strict libc coverage check demands it, and validate with `tsuku validate --strict --check-libc-coverage` + `tsuku eval` spot-checks.

## Likely gotchas

- **k9s** doesn't declare `arch_mapping`. Need to check if upstream names assets with `amd64`/`arm64` or `x86_64`/`aarch64`.
- **kustomize** is in the `kubernetes-sigs/kustomize` monorepo with multiple products; tag_prefix is likely `kustomize/v`. Need to verify.
- Libc split will be needed for any recipe using `os_mapping` (same pattern as #2281).

## Testing

- `tsuku validate --strict --check-libc-coverage` for each recipe
- `tsuku eval` across 4 platforms per tool
- CI green before merge

## Risks / deferrals

- If any recipe has an upstream infrastructure issue similar to trivy's aquasecurity IP allow list (#2301), drop that one tool from the batch and file a follow-up.
