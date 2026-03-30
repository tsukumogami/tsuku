package version

import (
	"context"
	"testing"
)

// resolveTestResolver is a VersionResolver (not VersionLister) for testing fallback paths.
type resolveTestResolver struct {
	latestVersion   string
	resolvedVersion string
}

func (r *resolveTestResolver) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return &VersionInfo{Version: r.latestVersion, Tag: "v" + r.latestVersion}, nil
}

func (r *resolveTestResolver) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	v := r.resolvedVersion
	if v == "" {
		v = version
	}
	return &VersionInfo{Version: v, Tag: "v" + v}, nil
}

func (r *resolveTestResolver) SourceDescription() string {
	return "resolve-test"
}

func TestResolveWithinBoundary_Empty(t *testing.T) {
	provider := &resolveTestResolver{latestVersion: "22.3.0"}
	info, err := ResolveWithinBoundary(context.Background(), provider, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "22.3.0" {
		t.Errorf("got version %q, want %q", info.Version, "22.3.0")
	}
}

func TestResolveWithinBoundary_Latest(t *testing.T) {
	provider := &resolveTestResolver{latestVersion: "22.3.0"}
	info, err := ResolveWithinBoundary(context.Background(), provider, "latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "22.3.0" {
		t.Errorf("got version %q, want %q", info.Version, "22.3.0")
	}
}

func TestResolveWithinBoundary_MajorPin_Lister(t *testing.T) {
	provider := &mockVersionLister{
		versions: []string{"22.3.0", "22.2.0", "20.18.1", "20.17.0", "18.20.4", "18.20.3"},
	}
	info, err := ResolveWithinBoundary(context.Background(), provider, "20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "20.18.1" {
		t.Errorf("got version %q, want %q", info.Version, "20.18.1")
	}
}

func TestResolveWithinBoundary_MinorPin_Lister(t *testing.T) {
	provider := &mockVersionLister{
		versions: []string{"1.30.0", "1.29.5", "1.29.4", "1.29.3", "1.28.0"},
	}
	info, err := ResolveWithinBoundary(context.Background(), provider, "1.29")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "1.29.5" {
		t.Errorf("got version %q, want %q", info.Version, "1.29.5")
	}
}

func TestResolveWithinBoundary_ExactPin_Lister(t *testing.T) {
	provider := &mockVersionLister{
		versions: []string{"1.29.5", "1.29.4", "1.29.3"},
	}
	info, err := ResolveWithinBoundary(context.Background(), provider, "1.29.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "1.29.3" {
		t.Errorf("got version %q, want %q", info.Version, "1.29.3")
	}
}

func TestResolveWithinBoundary_NoMatch_Lister(t *testing.T) {
	provider := &mockVersionLister{
		versions: []string{"22.3.0", "22.2.0"},
	}
	_, err := ResolveWithinBoundary(context.Background(), provider, "18")
	if err == nil {
		t.Fatal("expected error for no matching version")
	}
}

func TestResolveWithinBoundary_DotBoundary(t *testing.T) {
	provider := &mockVersionLister{
		versions: []string{"10.0.0", "1.5.0", "1.0.0"},
	}
	info, err := ResolveWithinBoundary(context.Background(), provider, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "1" should match "1.5.0", NOT "10.0.0"
	if info.Version != "1.5.0" {
		t.Errorf("got version %q, want %q (dot boundary should prevent matching 10.0.0)", info.Version, "1.5.0")
	}
}

func TestResolveWithinBoundary_ResolverOnly(t *testing.T) {
	provider := &resolveTestResolver{resolvedVersion: "18.20.4"}
	info, err := ResolveWithinBoundary(context.Background(), provider, "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "18.20.4" {
		t.Errorf("got version %q, want %q", info.Version, "18.20.4")
	}
}

func TestResolveWithinBoundary_ListFailsFallback(t *testing.T) {
	provider := &mockVersionLister{
		versions:    nil,
		shouldError: true,
		errorMsg:    "network error",
	}
	// When list fails, falls back to ResolveVersion which returns the requested string
	info, err := ResolveWithinBoundary(context.Background(), provider, "18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "18" {
		t.Errorf("got version %q, want %q", info.Version, "18")
	}
}

func TestResolveWithinBoundary_InvalidRequested(t *testing.T) {
	provider := &resolveTestResolver{latestVersion: "1.0.0"}
	_, err := ResolveWithinBoundary(context.Background(), provider, "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for invalid requested string")
	}
}
