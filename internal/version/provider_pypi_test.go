package version

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pypiTestRelease pairs a version with its requires_python value for
// the mock server fixtures below.
type pypiTestRelease struct {
	Version        string
	RequiresPython string
}

// newMockPyPIServer returns an httptest server that serves a single
// package's PyPI JSON. info.version is set from the first release in
// the list (which the mock treats as the absolute latest, regardless
// of any later filtering).
func newMockPyPIServer(t *testing.T, pkg string, releases []pypiTestRelease) *httptest.Server {
	t.Helper()
	type fileDict struct {
		RequiresPython string `json:"requires_python"`
	}
	type response struct {
		Info struct {
			Version string `json:"version"`
			Name    string `json:"name"`
		} `json:"info"`
		Releases map[string][]fileDict `json:"releases"`
	}
	resp := response{}
	resp.Info.Name = pkg
	if len(releases) > 0 {
		resp.Info.Version = releases[0].Version
	}
	resp.Releases = make(map[string][]fileDict, len(releases))
	for _, r := range releases {
		resp.Releases[r.Version] = []fileDict{{RequiresPython: r.RequiresPython}}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// ansibleStyleReleases mirrors ansible-core's actual requires_python
// progression at the time of writing: 2.20+ requires Python 3.12+,
// 2.18+ requires 3.11+, 2.17.x is the last 3.10-compatible line.
func ansibleStyleReleases() []pypiTestRelease {
	return []pypiTestRelease{
		{Version: "2.20.5", RequiresPython: ">=3.12"},
		{Version: "2.20.0", RequiresPython: ">=3.12"},
		{Version: "2.19.2", RequiresPython: ">=3.11"},
		{Version: "2.18.0", RequiresPython: ">=3.11"},
		{Version: "2.17.14", RequiresPython: ">=3.10"},
		{Version: "2.17.0", RequiresPython: ">=3.10"},
	}
}

func TestPyPIProvider_ResolveLatest_FiltersByPython(t *testing.T) {
	server := newMockPyPIServer(t, "ansible-core", ansibleStyleReleases())
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "ansible-core", "3.10")

	got, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}
	if got.Version != "2.17.14" {
		t.Errorf("ResolveLatest() = %q, want %q (newest 3.10-compatible)", got.Version, "2.17.14")
	}
}

func TestPyPIProvider_ResolveLatest_NoCompatibleRelease(t *testing.T) {
	releases := []pypiTestRelease{
		{Version: "3.0.0", RequiresPython: ">=3.13"},
		{Version: "2.0.0", RequiresPython: ">=3.12"},
		{Version: "1.0.0", RequiresPython: ">=3.11"},
	}
	server := newMockPyPIServer(t, "future-tool", releases)
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "future-tool", "3.10")

	_, err := provider.ResolveLatest(context.Background())
	if err == nil {
		t.Fatal("ResolveLatest expected ErrTypeNoCompatibleRelease, got nil")
	}
	var rerr *ResolverError
	if !errors.As(err, &rerr) {
		t.Fatalf("error is not *ResolverError: %v", err)
	}
	if rerr.Type != ErrTypeNoCompatibleRelease {
		t.Errorf("error type = %d, want ErrTypeNoCompatibleRelease (%d)", rerr.Type, ErrTypeNoCompatibleRelease)
	}
	// Message must name the package, the bundled Python, and the
	// latest version.
	msg := rerr.Error()
	for _, want := range []string{"future-tool", "3.10", "3.0.0", ">=3.13"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestPyPIProvider_ResolveLatest_NullRequiresPythonIsCompatible(t *testing.T) {
	// Older release predates PEP 345; requires_python is null.
	// Pip treats this as compatible — tsuku must too.
	releases := []pypiTestRelease{
		{Version: "2.20.5", RequiresPython: ">=3.12"},
		{Version: "1.0.0", RequiresPython: ""}, // null/empty
	}
	server := newMockPyPIServer(t, "legacy-tool", releases)
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "legacy-tool", "3.10")

	got, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("ResolveLatest() = %q, want %q (null requires_python treated as compatible)", got.Version, "1.0.0")
	}
}

func TestPyPIProvider_ResolveLatest_UnparseableSpecifierSkipsRelease(t *testing.T) {
	// One release has an unsupported operator (~=); the walker must
	// skip it without aborting and pick the next compatible one.
	releases := []pypiTestRelease{
		{Version: "3.0.0", RequiresPython: "~=3.6"},  // unsupported, skip
		{Version: "2.0.0", RequiresPython: ">=3.10"}, // compatible
		{Version: "1.0.0", RequiresPython: ">=3.7"},  // compatible (older)
	}
	server := newMockPyPIServer(t, "mixed-tool", releases)
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "mixed-tool", "3.10")

	got, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}
	if got.Version != "2.0.0" {
		t.Errorf("ResolveLatest() = %q, want %q (skipped 3.0.0 due to unsupported ~=)", got.Version, "2.0.0")
	}
}

func TestPyPIProvider_ResolveLatest_EmptyPythonMajorMinorPreservesBehavior(t *testing.T) {
	// When pythonMajorMinor is empty, the provider must behave exactly
	// as today (return absolute latest).
	server := newMockPyPIServer(t, "ansible-core", ansibleStyleReleases())
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProvider(resolver, "ansible-core")

	got, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}
	if got.Version != "2.20.5" {
		t.Errorf("ResolveLatest() = %q, want %q (absolute latest)", got.Version, "2.20.5")
	}
}

func TestPyPIProvider_ListVersions_FiltersByPython(t *testing.T) {
	server := newMockPyPIServer(t, "ansible-core", ansibleStyleReleases())
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "ansible-core", "3.10")

	versions, err := provider.ListVersions(context.Background())
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	// Only the 2.17.x line should be returned.
	if len(versions) != 2 {
		t.Errorf("ListVersions() = %v (len %d), want 2 entries (2.17.14, 2.17.0)", versions, len(versions))
	}
	for _, v := range versions {
		if !strings.HasPrefix(v, "2.17.") {
			t.Errorf("ListVersions() returned %q, want 2.17.x only", v)
		}
	}
}

func TestPyPIProvider_ResolveVersion_UserPinUnaffected(t *testing.T) {
	// User pin to an incompatible version must succeed (return that
	// version) — explicit pins are authoritative.
	server := newMockPyPIServer(t, "ansible-core", ansibleStyleReleases())
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "ansible-core", "3.10")

	got, err := provider.ResolveVersion(context.Background(), "2.20.5")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}
	if got.Version != "2.20.5" {
		t.Errorf("ResolveVersion(\"2.20.5\") = %q, want \"2.20.5\" (user pin authoritative)", got.Version)
	}
}

func TestPyPIProvider_ErrorMessage_RendersCanonicalNotRawBytes(t *testing.T) {
	// Adversarial fixture: requires_python contains a non-ASCII byte
	// that the parser would reject. The error message must render
	// "<malformed>" via pep440.Canonical, never the raw bytes.
	releases := []pypiTestRelease{
		{Version: "1.0.0", RequiresPython: ">=3.12​"}, // zero-width space
	}
	server := newMockPyPIServer(t, "evil-tool", releases)
	defer server.Close()

	resolver := New(WithPyPIRegistry(server.URL))
	provider := NewPyPIProviderForPipx(resolver, "evil-tool", "3.10")

	_, err := provider.ResolveLatest(context.Background())
	if err == nil {
		t.Fatal("ResolveLatest expected ErrTypeNoCompatibleRelease, got nil")
	}
	msg := err.Error()
	if strings.ContainsRune(msg, '​') {
		t.Errorf("error message contains raw zero-width space: %q", msg)
	}
	if !strings.Contains(msg, "<malformed>") {
		t.Errorf("error message %q does not include canonical \"<malformed>\" placeholder", msg)
	}
}
