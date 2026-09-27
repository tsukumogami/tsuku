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

// Upstream tags often carry a leading "v" (GitHub releases, Go modules) while
// users type the bare version, and the reverse holds for recipes whose
// tag_prefix strips it. Either spelling must reach the same list entry.
func TestResolveWithinBoundary_LeadingVEitherSpelling(t *testing.T) {
	vList := []string{"v2.38.0", "v2.37.1", "v2.37.0", "v2.0.0-rc1"}
	bareList := []string{"2.38.0", "2.37.1", "2.37.0", "2.0.0-rc1"}

	tests := []struct {
		name      string
		versions  []string
		requested string
		want      string
	}{
		{"bare exact pin on v list", vList, "2.37.1", "v2.37.1"},
		{"v exact pin on v list", vList, "v2.37.1", "v2.37.1"},
		{"bare minor pin on v list", vList, "2.37", "v2.37.1"},
		{"bare major pin on v list", vList, "2", "v2.38.0"},
		{"bare pre-release pin on v list", vList, "2.0.0-rc1", "v2.0.0-rc1"},
		{"v exact pin on bare list", bareList, "v2.37.1", "2.37.1"},
		{"bare exact pin on bare list", bareList, "2.37.1", "2.37.1"},
		{"v minor pin on bare list", bareList, "v2.37", "2.37.1"},
		{"v pre-release pin on bare list", bareList, "v2.0.0-rc1", "2.0.0-rc1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockVersionLister{versions: tt.versions}
			info, err := ResolveWithinBoundary(context.Background(), provider, tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// mockVersionLister.ResolveVersion echoes its input, so this is
			// the list entry handed to the provider.
			if info.Version != tt.want {
				t.Errorf("got %q, want %q", info.Version, tt.want)
			}
		})
	}
}

// A pin that already matches a list entry by exact spelling must resolve to
// that entry, even when the list also holds entries that would match once the
// leading "v" is ignored. Otherwise a deliberate pin could move to a different
// tag or version.
func TestResolveWithinBoundary_ExactSpellingWins(t *testing.T) {
	tests := []struct {
		name      string
		versions  []string
		requested string
		want      string
	}{
		{"v pin with both tags, v first", []string{"v1.0", "1.0"}, "v1.0", "v1.0"},
		{"v pin with both tags, bare first", []string{"1.0", "v1.0"}, "v1.0", "v1.0"},
		{"bare pin with both tags, v first", []string{"v1.0", "1.0"}, "1.0", "1.0"},
		{"bare pin with both tags, bare first", []string{"1.0", "v1.0"}, "1.0", "1.0"},
		// The repo switched from bare to v tags: pin "1" matched 1.0.0 before
		// and must keep matching it rather than jumping to v1.2.0.
		{"major pin on mixed-scheme list", []string{"v1.2.0", "1.0.0"}, "1", "1.0.0"},
		{"v major pin on mixed-scheme list", []string{"1.2.0", "v1.0.0"}, "v1", "v1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockVersionLister{versions: tt.versions}
			info, err := ResolveWithinBoundary(context.Background(), provider, tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.Version != tt.want {
				t.Errorf("got %q, want %q", info.Version, tt.want)
			}
		})
	}
}

// Only a "v" followed by a digit is a version prefix. A pin of "v" must not be
// stripped to "" (which means latest), and tags such as "vim-9.0" must not be
// read as "im-9.0".
func TestResolveWithinBoundary_LeadingVNeedsDigit(t *testing.T) {
	tests := []struct {
		name      string
		versions  []string
		requested string
	}{
		{"bare v pin", []string{"1.0.0"}, "v"},
		{"non-digit after v in list", []string{"vim-9.0"}, "im-9.0"},
		{"non-digit after v in pin", []string{"im-9.0"}, "vim-9.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockVersionLister{versions: tt.versions}
			info, err := ResolveWithinBoundary(context.Background(), provider, tt.requested)
			if err == nil {
				t.Fatalf("expected not-found error, got version %q", info.Version)
			}
		})
	}
}
