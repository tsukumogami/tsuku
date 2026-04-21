# Implementation Plan — Issue #2283

## Scope & approach

Same pattern as #2281 and #2282. 3 tools need validation + curated flag + libc-split (eksctl, skaffold, velero). 2 tools are batch-generated homebrew recipes that must be rewritten from scratch using `github_archive` with real upstream release URLs (cilium-cli, istioctl).

Dispatch 5 parallel validation agents; for the batch tools, additionally ask each agent for a complete proposed TOML. Apply all edits, validate strict, eval-check all 4 platforms per tool, commit.

## Special notes

- **skaffold** uses `github_file` + `binary` (singular) — confirm modern schema, consider upgrading to `binaries = [{src, dest}]` form used by cosign.
- **velero** uses `strip_dirs = 1` (has a wrapping `velero-v{version}-{os}-{arch}/` dir). Validate this.
- **cilium-cli** and **istioctl** batch output must be replaced with handcrafted github_archive recipes targeting cilium/cilium-cli and istio/istio respectively.
