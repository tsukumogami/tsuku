# Issue 281 Implementation Plan

## Summary

Implement the GitHub Release Builder that fetches GitHub release data, calls the LLM client to extract asset patterns, and generates recipes using `github_archive` or `github_file` actions.

## Approach

Create `internal/builders/github_release.go` following the pattern established by other builders (cargo.go, npm.go, etc.). The builder will:

1. Parse the `owner/repo` from `req.SourceArg`
2. Fetch multiple releases from GitHub API for pattern inference
3. Fetch repo metadata (description, homepage)
4. Fetch README proactively
5. Call LLM client with release context
6. Transform the `AssetPattern` response into a `recipe.Recipe`

### Alternatives Considered

- **Direct pattern inference without LLM**: Not chosen because the LLM approach is the core of this milestone and enables handling diverse naming conventions.
- **Single release instead of multiple**: Not chosen because multiple releases help the LLM identify version patterns reliably.

## Files to Create

- `internal/builders/github_release.go` - Main builder implementation
- `internal/builders/github_release_test.go` - Unit tests with mocked HTTP and LLM

## Files to Modify

None - the builder will be registered in application code when the CLI is updated (issue #282)

## Implementation Steps

- [x] Create `GitHubReleaseBuilder` struct with `httpClient` and `llmClient` fields
- [x] Implement `Name()` returning "github"
- [x] Implement `CanBuild()` checking for valid owner/repo format in SourceArg
- [x] Implement GitHub API client: fetch releases, repo metadata, README
- [x] Implement `Build()` orchestrating: fetch context → call LLM → generate recipe
- [x] Implement recipe generation from `AssetPattern` supporting both `github_archive` and `github_file`
- [x] Add unit tests with mock HTTP server for GitHub API
- [x] Add unit tests for recipe generation from AssetPattern

## Testing Strategy

- **Unit tests**: Mock GitHub API responses and test:
  - Parsing of release data
  - README fetching
  - Recipe generation from AssetPattern
  - Error handling (404, rate limits, invalid responses)

- **Integration test**: Skip if no ANTHROPIC_API_KEY; test against a real repo like FiloSottile/age

## Data Flow

```
Build(ctx, BuildRequest{Package: "gh", SourceArg: "cli/cli"})
    │
    ├── parseRepo("cli/cli") → owner="cli", repo="cli"
    │
    ├── fetchReleases() → []Release with tags and asset names
    │
    ├── fetchRepoMeta() → description, homepage
    │
    ├── fetchREADME() → README content
    │
    ├── llmClient.GenerateRecipe(ctx, &llm.GenerateRequest{...})
    │       │
    │       └── Returns AssetPattern with:
    │           - Mappings: [{asset, os, arch, format}, ...]
    │           - Executable: "gh"
    │           - VerifyCommand: "gh --version"
    │           - StripPrefix (optional)
    │
    └── generateRecipe(pattern, repoMeta) → recipe.Recipe
            │
            └── github_archive (if format=tar.gz/zip) or github_file (if format=binary)
```

## Recipe Generation

From AssetPattern to Recipe:

```go
// For archives (tar.gz, zip)
Step{
    Action: "github_archive",
    Params: {
        "repo": "cli/cli",
        "asset_pattern": derived from mappings,
        "archive_format": "tar.gz",
        "strip_dirs": 1 (or StripPrefix),
        "binaries": [pattern.Executable],
        "os_mapping": derived from mappings,
        "arch_mapping": derived from mappings,
    },
}

// For standalone binaries
Step{
    Action: "github_file",
    Params: {
        "repo": "cli/cli",
        "asset_pattern": derived from mappings,
        "binary": pattern.Executable,
        "os_mapping": derived from mappings,
        "arch_mapping": derived from mappings,
    },
}
```

## Risks and Mitigations

- **GitHub rate limits**: Use GITHUB_TOKEN if available; document rate limit errors clearly
- **LLM returns invalid pattern**: Validate the pattern before generating recipe; return warning
- **Empty releases**: Return clear error if repo has no releases

## Success Criteria

- [x] `GitHubReleaseBuilder` implements `Builder` interface
- [x] Fetches last 5 releases from GitHub API
- [x] Fetches README proactively
- [x] Generates valid recipes for both archive and binary formats
- [x] Unit tests pass
- [x] LLM cost included in warnings

## Open Questions

None - requirements are clear from the design doc and existing builder patterns.
