# Baseline

## Environment
- Date: 2026-04-21T18:30Z
- Branch: feature/2282-k8s-core-clis-curated
- Base commit: post #2300 merge

## Test Results
- `go build -o tsuku ./cmd/tsuku`: PASS
- `go test ./...`: all packages PASS

## Scope
5 handcrafted Kubernetes CLI recipes — validate upstream assets + add `curated = true`:
- `recipes/k/k9s.toml`
- `recipes/f/flux.toml`
- `recipes/s/stern.toml`
- `recipes/k/kubectx.toml`
- `recipes/k/kustomize.toml`
