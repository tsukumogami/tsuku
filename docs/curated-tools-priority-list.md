# Curated Tools Priority List

This document ranks the top 100 developer tools by popularity and maps each one to its current tsuku coverage status. It serves as the authoring queue for recipe work, so maintainers can see at a glance what still needs to be built.

Coverage was determined by inspecting `recipes/` directly. Discovery-only entries mean tsuku can surface the tool but has no installation recipe yet.

## Sources

Popularity rankings are based on Homebrew download analytics, GitHub star counts, Stack Overflow developer surveys, and JetBrains developer survey data (2023–2024). The list reflects what developers actually install daily, weighted toward tools that appear consistently across multiple signals.

## Action Summary

### No action needed (handcrafted — full platform support)
git, docker, terraform, gh, golang, fzf, btop, curl, httpie, lazygit, k9s, stern, kubectx, direnv, mise, asdf, pyenv, eksctl, flux, skaffold, kustomize, velero, vault, packer, bun, yarn, deno, pnpm, nvm, claude, gemini, trivy, grype, cosign, syft, actionlint, golangci-lint, ruff, black, tflint, pulumi, caddy, age, consul, vagrant, lazydocker, jq, wget, tmux

### Review coverage (batch — may need platform expansion or full handcrafting)
helm, ripgrep, fd, eza, zoxide, htop, cilium-cli, istioctl, bazel, ollama, act, earthly, goreleaser, shellcheck, shfmt, prettier, infracost, terragrunt, mkcert

### Author recipe (missing or discovery-only — needs a recipe)
node, python, kubectl, aws-cli, rust, bat, starship, neovim, delta, rbenv, gcloud, azure-cli, argocd, ansible, cmake, ninja-build, meson, make, gradle, maven, sbt, aider, copilot, ko, dive, hadolint, pre-commit, lefthook, checkov, sops, step, eslint

## Tool Ranking

| Rank | Tool | Coverage Status | Action |
|------|------|-----------------|--------|
| 1 | git | handcrafted | no action needed |
| 2 | node | discovery-only | author recipe |
| 3 | python | missing | author recipe |
| 4 | docker | handcrafted | no action needed |
| 5 | kubectl | batch | review coverage |
| 6 | terraform | handcrafted | no action needed |
| 7 | aws-cli | discovery-only | author recipe |
| 8 | helm | batch | review coverage |
| 9 | gh | handcrafted | no action needed |
| 10 | golang | handcrafted | no action needed |
| 11 | rust | discovery-only | author recipe |
| 12 | jq | handcrafted | no action needed |
| 13 | ripgrep | batch | review coverage |
| 14 | fd | batch | review coverage |
| 15 | fzf | handcrafted | no action needed |
| 16 | bat | discovery-only | author recipe |
| 17 | eza | batch | review coverage |
| 18 | zoxide | batch | review coverage |
| 19 | starship | discovery-only | author recipe |
| 20 | neovim | discovery-only | author recipe |
| 21 | tmux | handcrafted | no action needed |
| 22 | htop | batch | review coverage |
| 23 | btop | handcrafted | no action needed |
| 24 | curl | handcrafted | no action needed |
| 25 | wget | handcrafted | no action needed |
| 26 | httpie | handcrafted | no action needed |
| 27 | delta | discovery-only | author recipe |
| 28 | lazygit | handcrafted | no action needed |
| 29 | lazydocker | handcrafted | no action needed |
| 30 | k9s | handcrafted | no action needed |
| 31 | stern | handcrafted | no action needed |
| 32 | kubectx | handcrafted | no action needed |
| 33 | direnv | handcrafted | no action needed |
| 34 | mise | handcrafted | no action needed |
| 35 | asdf | handcrafted | no action needed |
| 36 | pyenv | handcrafted | no action needed |
| 37 | nvm | handcrafted | no action needed |
| 38 | rbenv | discovery-only | author recipe |
| 39 | gcloud | missing | author recipe |
| 40 | azure-cli | discovery-only | author recipe |
| 41 | eksctl | handcrafted | no action needed |
| 42 | flux | handcrafted | no action needed |
| 43 | argocd | discovery-only | author recipe |
| 44 | skaffold | handcrafted | no action needed |
| 45 | kustomize | handcrafted | no action needed |
| 46 | cilium-cli | batch | review coverage |
| 47 | istioctl | batch | review coverage |
| 48 | velero | handcrafted | no action needed |
| 49 | vault | handcrafted | no action needed |
| 50 | consul | handcrafted | no action needed |
| 51 | packer | handcrafted | no action needed |
| 52 | vagrant | handcrafted | no action needed |
| 53 | ansible | discovery-only | author recipe |
| 54 | cmake | discovery-only | author recipe |
| 55 | ninja-build | discovery-only | author recipe |
| 56 | meson | discovery-only | author recipe |
| 57 | make | discovery-only | author recipe |
| 58 | bazel | batch | review coverage |
| 59 | gradle | discovery-only | author recipe |
| 60 | maven | discovery-only | author recipe |
| 61 | sbt | discovery-only | author recipe |
| 62 | bun | handcrafted | no action needed |
| 63 | deno | handcrafted | no action needed |
| 64 | pnpm | handcrafted | no action needed |
| 65 | yarn | handcrafted | no action needed |
| 66 | claude | handcrafted | no action needed |
| 67 | gemini | handcrafted | no action needed |
| 68 | aider | missing | author recipe |
| 69 | ollama | batch | review coverage |
| 70 | copilot | missing | author recipe |
| 71 | act | batch | review coverage |
| 72 | earthly | batch | review coverage |
| 73 | goreleaser | batch | review coverage |
| 74 | ko | discovery-only | author recipe |
| 75 | dive | discovery-only | author recipe |
| 76 | trivy | handcrafted | no action needed |
| 77 | grype | handcrafted | no action needed |
| 78 | cosign | handcrafted | no action needed |
| 79 | syft | handcrafted | no action needed |
| 80 | hadolint | discovery-only | author recipe |
| 81 | shellcheck | batch | review coverage |
| 82 | shfmt | batch | review coverage |
| 83 | pre-commit | discovery-only | author recipe |
| 84 | lefthook | discovery-only | author recipe |
| 85 | actionlint | handcrafted | no action needed |
| 86 | golangci-lint | handcrafted | no action needed |
| 87 | ruff | handcrafted | no action needed |
| 88 | black | handcrafted | no action needed |
| 89 | prettier | batch | review coverage |
| 90 | eslint | missing | author recipe |
| 91 | tflint | handcrafted | no action needed |
| 92 | checkov | discovery-only | author recipe |
| 93 | infracost | batch | review coverage |
| 94 | terragrunt | batch | review coverage |
| 95 | pulumi | handcrafted | no action needed |
| 96 | caddy | handcrafted | no action needed |
| 97 | mkcert | batch | review coverage |
| 98 | age | handcrafted | no action needed |
| 99 | sops | discovery-only | author recipe |
| 100 | step | discovery-only | author recipe |
