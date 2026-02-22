# Container Image Version Updates: Dependabot and Renovate Capabilities

Research date: 2026-02-22

## Summary

Dependabot handles Dockerfiles and a few YAML-based formats natively, but cannot update arbitrary files (Go source, JSON, TOML). Renovate's regex custom manager fills that gap -- it can match and update container image versions in any file type, including Go source code, as long as you provide a regex pattern. The practical recommendation depends on how much flexibility you need.

---

## 1. GitHub Actions Workflow Files

### Dependabot

**Not supported.** Dependabot's `github-actions` ecosystem updates action references (`uses: owner/repo@v1`) but does not touch `container:` or `services:` image references in workflow YAML files. This is tracked as [dependabot-core#5819](https://github.com/dependabot/dependabot-core/issues/5819) (open since September 2022, explicitly deprioritized by maintainers) and [dependabot-core#5541](https://github.com/dependabot/dependabot-core/issues/5541).

### Renovate

**Partially supported.** The built-in `github-actions` manager only handles `uses: docker://image:tag` syntax. For `container: image:tag` and `services:` blocks, you need a regex custom manager:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "managerFilePatterns": ["/.github/workflows/.+\\.ya?ml$/"],
      "matchStrings": [
        "image:\\s+(?<depName>[^:\\s]+):(?<currentValue>[^\\s@]+)"
      ],
      "datasourceTemplate": "docker"
    }
  ]
}
```

**Verdict: Requires Renovate with a custom regex manager.**

---

## 2. Go Source Files

### Dependabot

**Not supported.** Dependabot's `gomod` ecosystem handles Go module dependencies in `go.mod`/`go.sum`. It has no ability to parse Go source code for string constants containing image references. There is no feature request or planned support for this.

### Renovate

**Supported via regex custom manager.** You can target `.go` files and write a regex that matches your specific pattern. For a map like:

```go
var familyToBaseImage = map[string]string{
    "alpine": "alpine:3.19",
}
```

The Renovate configuration would be:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "managerFilePatterns": ["**/*.go"],
      "matchStrings": [
        "\"(?<depName>[a-z][a-z0-9./-]+):(?<currentValue>[a-z0-9][a-z0-9._-]+)\""
      ],
      "datasourceTemplate": "docker"
    }
  ]
}
```

This is a broad pattern that would match any `"image:tag"` string in Go files. A more targeted approach uses Renovate's comment-annotation convention:

```go
// renovate: datasource=docker
var familyToBaseImage = map[string]string{
    "alpine": "alpine:3.19",
}
```

With a matching regex that keys off the `// renovate:` comment. This reduces false positives.

**Verdict: Requires Renovate with a custom regex manager. Works well but needs careful regex tuning to avoid false positives.**

---

## 3. Shared Config Files (JSON, TOML, YAML)

### Dependabot

**Not supported for arbitrary config files.** Dependabot's `docker` ecosystem only scans specific file formats:
- Dockerfiles (`Dockerfile`, `Containerfile`)
- `docker-compose.yml` / `compose.yml`
- Kubernetes manifests (Deployment YAML, Helm `Chart.yaml`, `values.yaml`)

It cannot parse arbitrary JSON, TOML, or custom YAML config files for image references.

### Renovate

**Supported via regex custom manager.** A shared config file like `container-images.json`:

```json
{
  "alpine": "alpine:3.19",
  "ubuntu": "ubuntu:22.04"
}
```

Can be matched with:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "managerFilePatterns": ["container-images.json"],
      "matchStrings": [
        "\"(?<depName>[a-z][a-z0-9./-]+):\\s*(?<currentValue>[a-z0-9][a-z0-9._-]+)\""
      ],
      "datasourceTemplate": "docker"
    }
  ]
}
```

Similarly for TOML (`container-images.toml`):

```toml
[images]
alpine = "alpine:3.19"
ubuntu = "ubuntu:22.04"
```

The same regex approach works -- Renovate matches against file content regardless of the config format.

**Verdict: Requires Renovate with a custom regex manager. This approach has the advantage of centralizing image versions in one file that both Go code and CI can read, and Renovate can update.**

---

## 4. Dockerfile References

### Dependabot

**Supported natively.** This is Dependabot's primary Docker use case. Configure it with:

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "docker"
    directory: "/"
    schedule:
      interval: "weekly"
```

Dependabot scans for `FROM` directives and proposes version bumps. Limitations:
- Only updates the **first** `FROM` in multi-stage builds (per some reports; behavior may vary)
- Does not update images referenced via `ARG` directives
- Supports `Dockerfile` and `Containerfile` naming patterns

### Renovate

**Supported natively.** The built-in `dockerfile` manager handles:
- `FROM image:tag` directives
- `COPY --from=image:tag` references
- `RUN --mount=type=...from=image:tag` references
- Syntax directives (`# syntax=docker/dockerfile:1`)

File patterns matched: `**/[Dd]ockerfile*`, `**/[Cc]ontainerfile*`

**Verdict: Both tools handle this well. Dependabot is sufficient if Dockerfiles are the only concern.**

---

## 5. Renovate's Regex Custom Manager

The regex custom manager is the key differentiator. It uses named capture groups in ECMAScript regex to extract dependency information from any file.

### Required Capture Groups

At minimum, Renovate needs to determine:
- **`depName`** (or `packageName`): the image name (e.g., `alpine`, `node`)
- **`currentValue`**: the current tag/version (e.g., `3.19`, `22.04`)
- **`datasource`**: where to look for updates (can be captured or set via `datasourceTemplate`)

### Optional Capture Groups

- `versioning`: version scheme (defaults to `semver-coerced` for Docker)
- `registryUrl`: custom registry (defaults to Docker Hub)
- `currentDigest`: existing SHA digest for pinning
- `depType`: classification
- `extractVersion`: version extraction from tag names

### Configuration Structure

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "managerFilePatterns": ["**/*.go"],
      "matchStrings": [
        "// renovate: datasource=(?<datasource>[a-z-]+)\\s+\"(?<depName>[^:]+):(?<currentValue>[^\"]+)\""
      ],
      "datasourceTemplate": "docker",
      "versioningTemplate": "docker"
    }
  ]
}
```

### Important Technical Notes

- Matching is **per-file**, not per-line. Use `(?:^|\n)` for line boundaries, not `^`/`$`.
- The regex engine does **not support backreferences or lookahead assertions**.
- Use triple-brace Handlebars syntax (`{{{versioning}}}`) in templates to avoid escaping issues.
- `managerFilePatterns` accepts both regex and glob patterns.

### Built-in Presets

Renovate ships several preset custom managers that demonstrate the pattern:
- `customManagers:dockerfileVersions` -- ENV/ARG variables in Dockerfiles
- `customManagers:helmChartYamlAppVersions` -- appVersion in Chart.yaml
- `customManagers:githubActionsVersions` -- version variables in workflow files
- `customManagers:makefileVersions` -- version variables in Makefiles

These presets use the `# renovate: datasource=... depName=...` comment convention.

---

## Comparison Table

| File Type | Dependabot | Renovate (built-in) | Renovate (regex) |
|-----------|-----------|---------------------|-------------------|
| Dockerfile `FROM` | Yes | Yes | Not needed |
| docker-compose.yml | Yes | Yes | Not needed |
| Kubernetes manifests | Yes | Yes | Not needed |
| Helm values.yaml | Yes | Yes | Not needed |
| GH Actions `container:` | No | No | Yes |
| GH Actions `services:` | No | No | Yes |
| Go source constants | No | No | Yes |
| Arbitrary JSON config | No | No | Yes |
| Arbitrary TOML config | No | No | Yes |
| Custom YAML config | No | No | Yes |

---

## Recommendations

### If only Dockerfiles need updating
Use Dependabot. It's built into GitHub, zero-config for this case, and works reliably.

### If Go source or custom config files need updating
Use Renovate with a custom regex manager. Two approaches:

**Option A: Comment-annotated source files.** Add `// renovate: datasource=docker` comments near image references in Go code. More explicit, fewer false positives, but requires maintaining annotations.

**Option B: Centralized config file.** Extract image versions into a dedicated file (`container-images.json` or similar) that both Go code reads at build/runtime and Renovate can parse with a simple regex. This keeps the Renovate config simple and avoids scanning all `.go` files. The Go code would need to read versions from the config file rather than hardcoding them.

**Option C: Broad regex on Go files.** Match all `"image:tag"` patterns in `.go` files. Simplest Renovate config but highest false-positive risk.

Option B is likely the cleanest separation of concerns. Option A works well when image references are scattered across multiple files and refactoring them into a shared config isn't practical.

---

## Sources

- [Dependabot supported ecosystems and repositories (GitHub Docs)](https://docs.github.com/en/code-security/dependabot/ecosystems-supported-by-dependabot/supported-ecosystems-and-repositories)
- [dependabot-core#5819: Update container image references in GitHub Action workflows](https://github.com/dependabot/dependabot-core/issues/5819)
- [dependabot-core#5541: Bump docker image references in GitHub Actions workflow](https://github.com/dependabot/dependabot-core/issues/5541)
- [Renovate: Custom Manager Support using Regex](https://docs.renovatebot.com/modules/manager/regex/)
- [Renovate: Docker documentation](https://docs.renovatebot.com/docker/)
- [Renovate: Managers overview](https://docs.renovatebot.com/modules/manager/)
- [Renovate: CustomManager Presets](https://docs.renovatebot.com/presets-customManagers/)
- [Renovate: Dockerfile manager](https://docs.renovatebot.com/modules/manager/dockerfile/)
- [Renovate Discussion #15305: Update Docker images in GitHub Actions workflows](https://github.com/renovatebot/renovate/discussions/15305)
- [Renovate Issue #16351: GitHub Actions service container image updates](https://github.com/renovatebot/renovate/issues/16351)
