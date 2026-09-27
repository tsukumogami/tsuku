package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// stubRelease is one entry of a stubbed /releases listing.
type stubRelease struct {
	tag        string
	draft      bool
	prerelease bool
	assets     int
}

// releaseStub stubs the GitHub API endpoints the github provider reads:
// /tags, /releases (both paginated), /releases/latest and /git/ref/tags/.
// A nil releases slice makes /releases answer 404; latest is the tag
// /releases/latest returns, or "" for 404. Requests are counted per endpoint.
type releaseStub struct {
	tags     [][]string
	releases [][]stubRelease
	latest   string

	mu    sync.Mutex
	calls map[string]int
}

func (s *releaseStub) count(endpoint string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[endpoint]
}

func (s *releaseStub) resolver(t *testing.T) *Resolver {
	t.Helper()
	const base = "/api/v3/repos/owner/repo"
	s.calls = map[string]int{}

	type asset struct {
		Name string `json:"name"`
	}
	type release struct {
		TagName    string  `json:"tag_name"`
		Draft      bool    `json:"draft"`
		Prerelease bool    `json:"prerelease"`
		Assets     []asset `json:"assets"`
	}
	toJSON := func(r stubRelease) release {
		out := release{TagName: r.tag, Draft: r.draft, Prerelease: r.prerelease, Assets: []asset{}}
		for i := 0; i < r.assets; i++ {
			out.Assets = append(out.Assets, asset{Name: fmt.Sprintf("asset-%d.tar.gz", i)})
		}
		return out
	}
	// page serves one page of a paginated listing, with a Link header when
	// more pages follow.
	page := func(w http.ResponseWriter, r *http.Request, path string, pages int) int {
		n := 1
		if p := r.URL.Query().Get("page"); p != "" {
			n, _ = strconv.Atoi(p)
		}
		if n < pages {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?per_page=100&page=%d>; rel="next"`, r.Host, path, n+1))
		}
		return n
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		count := func(endpoint string) {
			s.mu.Lock()
			s.calls[endpoint]++
			s.mu.Unlock()
		}
		switch {
		case r.URL.Path == base+"/tags":
			count("tags")
			n := page(w, r, r.URL.Path, len(s.tags))
			type tag struct {
				Name string `json:"name"`
			}
			out := []tag{}
			if n >= 1 && n <= len(s.tags) {
				for _, name := range s.tags[n-1] {
					out = append(out, tag{Name: name})
				}
			}
			_ = json.NewEncoder(w).Encode(out)

		case r.URL.Path == base+"/releases":
			count("releases")
			if s.releases == nil {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
				return
			}
			n := page(w, r, r.URL.Path, len(s.releases))
			out := []release{}
			if n >= 1 && n <= len(s.releases) {
				for _, rel := range s.releases[n-1] {
					out = append(out, toJSON(rel))
				}
			}
			_ = json.NewEncoder(w).Encode(out)

		case r.URL.Path == base+"/releases/latest":
			count("latest")
			for _, pg := range s.releases {
				for _, rel := range pg {
					if rel.tag == s.latest {
						_ = json.NewEncoder(w).Encode(toJSON(rel))
						return
					}
				}
			}
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)

		case strings.HasPrefix(r.URL.Path, base+"/git/ref/tags/"):
			count("ref")
			name := strings.TrimPrefix(r.URL.Path, base+"/git/ref/tags/")
			for _, pg := range s.tags {
				for _, tag := range pg {
					if tag == name {
						fmt.Fprintf(w, `{"ref":"refs/tags/%s","object":{"type":"commit","sha":"abc"}}`, name)
						return
					}
				}
			}
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)

		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(WithGitHubBaseURL(srv.URL+"/", srv.URL+"/"))
}

// kotoMidRelease is a repository caught mid-release: v0.14.0 is tagged and its
// release is still a draft with no assets, while v0.13.0 is published.
func kotoMidRelease() *releaseStub {
	return &releaseStub{
		tags: [][]string{{"v0.14.0", "v0.13.0", "v0.12.0"}},
		releases: [][]stubRelease{{
			{tag: "v0.14.0", draft: true},
			{tag: "v0.13.0", assets: 5},
			{tag: "v0.12.0", assets: 5},
		}},
		latest: "v0.13.0",
	}
}

// "latest" skips a newest tag whose release is a draft. Before the fix the
// prefixed path read only /tags and resolved to 0.14.0, whose download 404s.
func TestGitHubProvider_LatestSkipsDraftRelease(t *testing.T) {
	stub := kotoMidRelease()
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := p.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest: %v", err)
	}
	if info.Version != "0.13.0" || info.Tag != "v0.13.0" {
		t.Errorf("got %s (%s), want 0.13.0 (v0.13.0)", info.Version, info.Tag)
	}
	if got := stub.count("releases") + stub.count("tags"); got != 1 {
		t.Errorf("listing requests = %d, want 1 (releases=%d tags=%d)", got, stub.count("releases"), stub.count("tags"))
	}
}

// A caller without push access doesn't see the draft at all: the tag exists
// but has no release. The result is the same.
func TestGitHubProvider_LatestSkipsTagWithoutRelease(t *testing.T) {
	stub := kotoMidRelease()
	stub.releases[0] = stub.releases[0][1:]
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := p.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest: %v", err)
	}
	if info.Version != "0.13.0" {
		t.Errorf("got %s, want 0.13.0", info.Version)
	}
}

// A range pin (koto@0, the install that failed in the field) skips the draft too.
func TestGitHubProvider_RangePinSkipsDraftRelease(t *testing.T) {
	stub := kotoMidRelease()
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "0")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(0): %v", err)
	}
	if info.Version != "0.13.0" || info.Tag != "v0.13.0" {
		t.Errorf("got %s (%s), want 0.13.0 (v0.13.0)", info.Version, info.Tag)
	}
}

// An exact pin to the unpublished version still resolves to its tag, so the
// install fails loudly at download instead of quietly installing another
// version or reporting the version missing.
func TestGitHubProvider_ExactPinToDraftStillResolves(t *testing.T) {
	stub := kotoMidRelease()
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "0.14.0")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(0.14.0): %v", err)
	}
	if info.Tag != "v0.14.0" {
		t.Errorf("got tag %s, want v0.14.0", info.Tag)
	}
}

// When every release is published with assets, the answer is the one the
// tag listing gave before.
func TestGitHubProvider_AllPublishedUnchanged(t *testing.T) {
	stub := kotoMidRelease()
	stub.releases[0][0] = stubRelease{tag: "v0.14.0", assets: 5}
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := p.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest: %v", err)
	}
	if info.Version != "0.14.0" {
		t.Errorf("got %s, want 0.14.0", info.Version)
	}
}

// A repository whose releases never carry assets (the recipe downloads from a
// vendor CDN) resolves to its newest published release, as before.
func TestGitHubProvider_AssetlessReleasesStillCount(t *testing.T) {
	stub := &releaseStub{
		tags: [][]string{{"v1.16.4", "v1.16.3"}},
		releases: [][]stubRelease{{
			{tag: "v1.16.3"},
			{tag: "v1.16.4"},
		}},
	}
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := p.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest: %v", err)
	}
	if info.Version != "1.16.4" {
		t.Errorf("got %s, want 1.16.4", info.Version)
	}
}

// A repository with no releases at all (golang/go style) falls back to tags.
func TestGitHubProvider_NoReleasesFallsBackToTags(t *testing.T) {
	for name, releases := range map[string][][]stubRelease{
		"empty list": {{}},
		"404":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			stub := &releaseStub{
				tags:     [][]string{{"v2.1.0", "v2.0.0"}},
				releases: releases,
			}
			p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

			info, err := p.ResolveLatest(context.Background())
			if err != nil {
				t.Fatalf("ResolveLatest: %v", err)
			}
			if info.Version != "2.1.0" {
				t.Errorf("got %s, want 2.1.0", info.Version)
			}
		})
	}
}

// A range pin whose only releases are past the first 100 pages through the
// releases rather than giving up or falling back to tags.
func TestGitHubProvider_RangePinBeyondFirstReleasePage(t *testing.T) {
	var newer []stubRelease
	var tags []string
	for i := 100; i > 0; i-- {
		tag := fmt.Sprintf("v1.%d.0", i)
		newer = append(newer, stubRelease{tag: tag, assets: 1})
		tags = append(tags, tag)
	}
	stub := &releaseStub{
		// v0.10.0 is tagged but its release is a draft; v0.9.0 is published.
		tags: [][]string{tags, {"v0.10.0", "v0.9.0"}},
		releases: [][]stubRelease{
			newer,
			{{tag: "v0.10.0", draft: true}, {tag: "v0.9.0", assets: 1}},
		},
	}
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "0")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(0): %v", err)
	}
	if info.Version != "0.9.0" {
		t.Errorf("got %s, want 0.9.0", info.Version)
	}
	// Page 1 for the first pass, then pages 1 and 2 for the deeper one.
	if got := stub.count("releases"); got != 3 {
		t.Errorf("release listing requests = %d, want 3", got)
	}
	if got := stub.count("tags"); got != 0 {
		t.Errorf("tag listing requests = %d, want 0", got)
	}
}

// A range pin on a line that was tagged but never released still resolves
// from the tags, as it did before releases were consulted.
func TestGitHubProvider_RangePinOnTagOnlyLine(t *testing.T) {
	stub := &releaseStub{
		tags:     [][]string{{"v2.0.0", "v1.0.0", "v0.3.1", "v0.3.0"}},
		releases: [][]stubRelease{{{tag: "v2.0.0", assets: 1}, {tag: "v1.0.0", assets: 1}}},
	}
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "0.3")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(0.3): %v", err)
	}
	if info.Version != "0.3.1" {
		t.Errorf("got %s, want 0.3.1", info.Version)
	}
}

// Without a tag prefix, "latest" comes from /releases/latest. When that
// release was published before its assets were uploaded, the newest earlier
// release with assets wins.
func TestGitHubProvider_NoPrefixLatestWithoutAssets(t *testing.T) {
	stub := &releaseStub{
		releases: [][]stubRelease{{
			{tag: "v2.0.0"},
			{tag: "v1.9.0", assets: 3},
			{tag: "v1.8.0", assets: 3},
			{tag: "v3.0.0-rc1", prerelease: true, assets: 3},
		}},
		latest: "v2.0.0",
	}
	p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)

	info, err := p.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest: %v", err)
	}
	if info.Tag != "v1.9.0" {
		t.Errorf("got %s, want v1.9.0", info.Tag)
	}
}

// ... but a repository whose releases never carry assets keeps the latest
// release, and a latest release with assets costs no extra request.
func TestGitHubProvider_NoPrefixLatestUnchanged(t *testing.T) {
	t.Run("assetless repo", func(t *testing.T) {
		stub := &releaseStub{
			releases: [][]stubRelease{{{tag: "v1.16.4"}, {tag: "v1.16.3"}}},
			latest:   "v1.16.4",
		}
		p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)
		info, err := p.ResolveLatest(context.Background())
		if err != nil {
			t.Fatalf("ResolveLatest: %v", err)
		}
		if info.Tag != "v1.16.4" {
			t.Errorf("got %s, want v1.16.4", info.Tag)
		}
	})
	t.Run("latest has assets", func(t *testing.T) {
		stub := &releaseStub{
			releases: [][]stubRelease{{{tag: "v2.0.0", assets: 2}, {tag: "v1.9.0", assets: 2}}},
			latest:   "v2.0.0",
		}
		p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)
		info, err := p.ResolveLatest(context.Background())
		if err != nil {
			t.Fatalf("ResolveLatest: %v", err)
		}
		if info.Tag != "v2.0.0" {
			t.Errorf("got %s, want v2.0.0", info.Tag)
		}
		if got := stub.count("latest") + stub.count("releases") + stub.count("tags"); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
	})
}
