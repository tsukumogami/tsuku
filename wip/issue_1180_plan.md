# Issue 1180 Implementation Plan

## Summary

Register the existing GoBuilder as a SessionBuilder by adding the missing interface methods and registering it in create.go.

## Files to Modify

1. `internal/builders/go.go` - Add SessionBuilder interface methods
2. `cmd/tsuku/create.go` - Register GoBuilder

## Implementation Steps

### Step 1: Update GoBuilder.CanBuild signature

The current signature is `CanBuild(ctx context.Context, packageName string)` but the SessionBuilder interface requires `CanBuild(ctx context.Context, req BuildRequest)`.

Change:
```go
func (b *GoBuilder) CanBuild(ctx context.Context, packageName string) (bool, error)
```
To:
```go
func (b *GoBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error)
```

Use `req.Package` instead of `packageName` internally.

### Step 2: Add RequiresLLM method

Add after the Name() method:
```go
// RequiresLLM returns false as this builder uses ecosystem APIs, not LLM.
func (b *GoBuilder) RequiresLLM() bool {
    return false
}
```

### Step 3: Add NewSession method

Add after CanBuild:
```go
// NewSession creates a new build session for the given request.
func (b *GoBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
    return NewDeterministicSession(b.Build, req), nil
}
```

### Step 4: Register in create.go

In `runCreate()`, add registration after the other ecosystem builders (around line 207):
```go
builderRegistry.Register(builders.NewGoBuilder(nil))
```

### Step 5: Update create.go help text

Add Go builder to the help text around line 69-72:
```
  go:module             Go modules from proxy.golang.org
```

And add example around line 83:
```
  tsuku create lazygit --from go:github.com/jesseduffield/lazygit
```

### Step 6: Update normalizeEcosystem

Add Go aliases around line 170:
```go
case "go", "golang", "goproxy":
    return "go"
```

## Testing Strategy

1. Run existing go_test.go tests to verify nothing broke
2. Build CLI and test `tsuku create --from go:github.com/jesseduffield/lazygit`
3. Verify help text shows Go option

## Risks

None - this is adding to an existing, well-tested pattern.
