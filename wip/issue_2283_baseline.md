# Baseline

## Environment
- Date: 2026-04-21T18:55Z
- Branch: feature/2283-k8s-ecosystem-curated
- Base commit: post #2302 merge

## Scope
5 K8s ecosystem tools:
- `recipes/e/eksctl.toml` — handcrafted, github_archive
- `recipes/s/skaffold.toml` — handcrafted, github_file (single binary, not tarball)
- `recipes/v/velero.toml` — handcrafted, github_archive with strip_dirs=1
- `recipes/c/cilium-cli.toml` — batch (homebrew), needs full rewrite
- `recipes/i/istioctl.toml` — batch (homebrew, excludes darwin), needs full rewrite
