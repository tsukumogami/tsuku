# Baseline

## Environment
- Date: 2026-04-21T19:12Z
- Branch: feature/2281-security-scanners-curated
- Base commit: (post #2298 merge on main)

## Test Results
- `go build -o tsuku ./cmd/tsuku`: PASS
- `go test ./...`: all packages PASS

## Scope
5 security scanner recipes, all already handcrafted:
- `recipes/t/trivy.toml`
- `recipes/g/grype.toml`
- `recipes/c/cosign.toml`
- `recipes/s/syft.toml`
- `recipes/t/tflint.toml`

Work per recipe: validate upstream assets against latest release, add `curated = true`.
