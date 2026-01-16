# Implementation Plan: Issue #874

## Summary

Issue #874 adds short form parsing support (`tap:owner/repo/formula`) for the tap provider factory integration. Most infrastructure is already in place from sibling issues (#872, #873, #875).

## What's Already Done

- `TapSourceStrategy` struct implementing `ProviderStrategy` interface
- Strategy registered in `NewProviderFactory()` at `PriorityKnownRegistry` (100)
- `CanHandle` returns true when `r.Version.Source == "tap"` with tap and formula fields (explicit form)
- `TapProvider` implementation with `ResolveLatest()`, `ResolveVersion()`
- Template variable population via `VersionInfo.Metadata` map (bottle_url, checksum, tap, formula)
- Unit tests for `TapSourceStrategy.CanHandle()` with explicit form sources

## What Needs Implementation

1. **Short form parsing** - Handle `source = "tap:owner/repo/formula"` syntax
2. **Unit tests for short form** - Test the parsing logic
3. **Tap recipe examples** - Add recipes that use tap provider (per user request)

## Design Decisions

### Short Form Syntax

The short form `tap:owner/repo/formula` is parsed as:
- `owner` = GitHub organization or user (e.g., `hashicorp`)
- `repo` = Tap name without `homebrew-` prefix (e.g., `tap`)
- `formula` = Formula name (e.g., `terraform`)

This maps to tap `{owner}/{repo}` which GitHub sees as `{owner}/homebrew-{repo}`.

### Implementation Approach

1. Add `parseTapShortForm()` function in `provider_factory.go`
2. Modify `TapSourceStrategy.CanHandle()` to also match `strings.HasPrefix(r.Version.Source, "tap:")`
3. Modify `TapSourceStrategy.Create()` to parse short form when detected

## Files to Modify

| File | Changes |
|------|---------|
| `internal/version/provider_factory.go` | Add short form parsing, update `CanHandle` and `Create` |
| `internal/version/provider_factory_test.go` | Add unit tests for short form parsing |

## Implementation Steps

### Step 1: Add `parseTapShortForm` function

Location: `internal/version/provider_factory.go` (near `TapSourceStrategy`)

```go
// parseTapShortForm parses the short form tap source syntax.
// Input: "tap:owner/repo/formula" (e.g., "tap:hashicorp/tap/terraform")
// Returns: tap ("hashicorp/tap"), formula ("terraform"), or error
func parseTapShortForm(source string) (tap, formula string, err error) {
    if !strings.HasPrefix(source, "tap:") {
        return "", "", fmt.Errorf("not a tap short form source: %s", source)
    }

    path := strings.TrimPrefix(source, "tap:")
    parts := strings.Split(path, "/")
    if len(parts) != 3 {
        return "", "", fmt.Errorf("invalid tap short form: expected tap:owner/repo/formula, got %s", source)
    }

    owner := parts[0]
    repo := parts[1]
    formula = parts[2]

    if owner == "" || repo == "" || formula == "" {
        return "", "", fmt.Errorf("invalid tap short form: empty component in %s", source)
    }

    tap = owner + "/" + repo
    return tap, formula, nil
}
```

### Step 2: Update `TapSourceStrategy.CanHandle`

```go
func (s *TapSourceStrategy) CanHandle(r *recipe.Recipe) bool {
    // Explicit form: source = "tap" with tap and formula fields
    if r.Version.Source == "tap" {
        return r.Version.Tap != "" && r.Version.Formula != ""
    }
    // Short form: source = "tap:owner/repo/formula"
    if strings.HasPrefix(r.Version.Source, "tap:") {
        _, _, err := parseTapShortForm(r.Version.Source)
        return err == nil
    }
    return false
}
```

### Step 3: Update `TapSourceStrategy.Create`

```go
func (s *TapSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
    var tap, formula string

    if strings.HasPrefix(r.Version.Source, "tap:") {
        // Short form: parse from source
        var err error
        tap, formula, err = parseTapShortForm(r.Version.Source)
        if err != nil {
            return nil, err
        }
    } else {
        // Explicit form: use fields
        tap = r.Version.Tap
        formula = r.Version.Formula
    }

    if tap == "" {
        return nil, fmt.Errorf("no tap specified for Tap version source")
    }
    if formula == "" {
        return nil, fmt.Errorf("no formula specified for Tap version source")
    }

    return NewTapProvider(resolver, tap, formula), nil
}
```

### Step 4: Add unit tests

Add to `internal/version/provider_factory_test.go`:

```go
func TestParseTapShortForm(t *testing.T) {
    tests := []struct {
        name        string
        source      string
        wantTap     string
        wantFormula string
        wantErr     bool
    }{
        {"valid hashicorp/tap/terraform", "tap:hashicorp/tap/terraform", "hashicorp/tap", "terraform", false},
        {"valid github/gh/gh", "tap:github/gh/gh", "github/gh", "gh", false},
        {"missing tap prefix", "hashicorp/tap/terraform", "", "", true},
        {"too few parts", "tap:hashicorp/terraform", "", "", true},
        {"too many parts", "tap:a/b/c/d", "", "", true},
        {"empty owner", "tap:/tap/terraform", "", "", true},
        {"empty repo", "tap:hashicorp//terraform", "", "", true},
        {"empty formula", "tap:hashicorp/tap/", "", "", true},
        {"just tap:", "tap:", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tap, formula, err := parseTapShortForm(tt.source)
            if (err != nil) != tt.wantErr {
                t.Errorf("parseTapShortForm() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if tap != tt.wantTap {
                t.Errorf("parseTapShortForm() tap = %q, want %q", tap, tt.wantTap)
            }
            if formula != tt.wantFormula {
                t.Errorf("parseTapShortForm() formula = %q, want %q", formula, tt.wantFormula)
            }
        })
    }
}

func TestTapSourceStrategy_CanHandle_ShortForm(t *testing.T) {
    tests := []struct {
        name   string
        source string
        want   bool
    }{
        {"valid short form", "tap:hashicorp/tap/terraform", true},
        {"invalid short form - too few parts", "tap:hashicorp/terraform", false},
        {"invalid short form - empty", "tap:", false},
        {"not short form", "github", false},
    }

    strategy := &TapSourceStrategy{}
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            r := &recipe.Recipe{
                Version: recipe.VersionSection{
                    Source: tt.source,
                },
            }
            if got := strategy.CanHandle(r); got != tt.want {
                t.Errorf("CanHandle() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Out of Scope

Adding actual tap-based recipes to the production recipe registry is out of scope for this issue. The user's request to "add tap-provided tools to the test matrix" would require:
1. Creating recipe files in `internal/recipe/recipes/`
2. Generating golden files for those recipes
3. This is significant additional scope beyond the short form parsing feature

The current implementation enables users to create tap-based recipes manually. A follow-up issue could add curated tap recipes.

## Testing Strategy

1. Run `go test -v ./internal/version/... -run TestParseTapShortForm`
2. Run `go test -v ./internal/version/... -run TestTapSourceStrategy`
3. Run full test suite: `go test -v -test.short ./...`
4. Build: `go build -o tsuku ./cmd/tsuku`

## Acceptance Criteria Mapping

| Criterion | Implementation |
|-----------|----------------|
| CanHandle returns true when source starts with "tap:" | Step 2 |
| Short form parsing extracts owner, repo, formula | Step 1 |
| Short form parsing handles edge cases | Step 1, 4 |
| Unit tests for short form parsing logic | Step 4 |
