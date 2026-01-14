# Research: Homebrew Tap Metadata Access

## Summary

The `formulae.brew.sh` API only covers official homebrew/core and homebrew/cask repositories. Third-party taps require either GitHub API access (with rate limiting) or direct Git repository cloning.

## Current tsuku Homebrew Integration

### Version Provider (`internal/version/homebrew.go`)
- Uses `https://formulae.brew.sh/api/formula/{formula}.json`
- Extracts version from `versions.stable` field
- Validates formula names against injection attacks
- Response size limited to 1MB

### Homebrew Action (`internal/actions/homebrew.go`)
- Downloads bottles from GHCR: `ghcr.io/homebrew/core/{formula}`
- Formula `@` converts to `/` for GHCR paths (e.g., `python@3.12` → `python/3.12`)
- Obtains anonymous GHCR tokens
- Verifies SHA256 checksums

### Key Limitation
**formulae.brew.sh does NOT support third-party taps.** The API is hardcoded to homebrew/core and homebrew/cask only.

## Homebrew Tap Structure

Taps are Git repositories at `github.com/{user}/homebrew-{name}`:

```
homebrew-tap/
├── Formula/           # Formula definitions (.rb files)
│   └── foo.rb
├── Casks/             # Cask definitions
├── formula_renames.json
├── tap_migrations.json
└── README.md
```

## Metadata Access Options

### Option A: GitHub REST API
- Enumerate `Formula/` directory contents
- Fetch individual `.rb` files
- **Rate limits**: 60/hour unauth, 5,000/hour with token
- **Pro**: No local storage needed
- **Con**: Rate limiting, must parse Ruby files

### Option B: Git Clone + Local Cache
- Clone tap repository locally
- Parse `.rb` files with Ruby parser or regex
- **Pro**: Full access, no rate limits
- **Con**: Storage, staleness, complexity

### Option C: GitHub Raw Content
- Access raw file URLs: `raw.githubusercontent.com/{user}/homebrew-{name}/HEAD/Formula/foo.rb`
- **Pro**: Simple HTTP GET
- **Con**: Still rate limited, no directory listing

## Bottle Access for Taps

**Official taps (homebrew/core)**: Bottles on GHCR at `ghcr.io/homebrew/core/{formula}`

**Third-party taps**:
- May NOT have bottles (source-only)
- If bottles exist, URL specified in formula's `bottle` block
- Custom bottle mirrors possible (GitHub Releases, S3, etc.)

## Rate Limiting Summary

| Source | Unauth | Auth |
|--------|--------|------|
| formulae.brew.sh | Unlimited? | N/A |
| GitHub API | 60/hr | 5,000/hr |
| GitHub Raw | Same as API | Same as API |
| GHCR | Reasonable | Higher |

## Recommendations

1. **Formulas from third-party taps**: Use GitHub API with caching
2. **Bottles**: Extract bottle URL from formula, handle missing bottles gracefully
3. **Authentication**: Support optional GitHub token for higher rate limits
4. **Caching**: Cache tap metadata locally to reduce API calls
