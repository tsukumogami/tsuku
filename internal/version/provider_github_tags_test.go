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

// tagStub is a stub of the GitHub API endpoints the github provider uses to
// find tags: the paginated tag listing and the single-ref lookup. Pages hold
// tag names in the order GitHub would return them, and requests are counted
// per endpoint so tests can assert how many calls a resolution made.
type tagStub struct {
	pages [][]string

	mu        sync.Mutex
	tagPages  []int    // page numbers requested from /tags, in order
	refLookup []string // tag names requested from /git/ref/tags/
}

func (s *tagStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	const base = "/api/v3/repos/owner/repo"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == base+"/tags":
			page := 1
			if p := r.URL.Query().Get("page"); p != "" {
				page, _ = strconv.Atoi(p)
			}
			s.mu.Lock()
			s.tagPages = append(s.tagPages, page)
			s.mu.Unlock()
			if page < len(s.pages) {
				w.Header().Set("Link", fmt.Sprintf(`<http://%s%s/tags?per_page=100&page=%d>; rel="next"`, r.Host, base, page+1))
			}
			type tag struct {
				Name string `json:"name"`
			}
			var out []tag
			if page >= 1 && page <= len(s.pages) {
				for _, name := range s.pages[page-1] {
					out = append(out, tag{Name: name})
				}
			}
			_ = json.NewEncoder(w).Encode(out)

		case strings.HasPrefix(r.URL.Path, base+"/git/ref/tags/"):
			name := strings.TrimPrefix(r.URL.Path, base+"/git/ref/tags/")
			s.mu.Lock()
			s.refLookup = append(s.refLookup, name)
			s.mu.Unlock()
			for _, page := range s.pages {
				for _, tag := range page {
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
	return srv
}

func (s *tagStub) resolver(t *testing.T) *Resolver {
	srv := s.server(t)
	return New(WithGitHubBaseURL(srv.URL+"/", srv.URL+"/"))
}

// fillerTags returns n tag names that sort ahead of every 1.x tag by name,
// the way casey/just's v0.9.x tags fill its first page.
func fillerTags(n int) []string {
	tags := make([]string, n)
	for i := range tags {
		tags[i] = fmt.Sprintf("v0.9.%d", n-i)
	}
	return tags
}

// justLikeStub has 250 tags over three pages, with the current releases only
// on pages 2 and 3.
func justLikeStub() *tagStub {
	return &tagStub{pages: [][]string{
		fillerTags(100),
		append([]string{"1.46.1", "1.45.0"}, fillerTags(98)...),
		{"1.46.0", "1.44.0"},
	}}
}

// A pinned version whose tag is not on the first page of /tags resolves.
// Before the fix this returned "version 1.46.0 not found", because only page
// 1 was ever read.
func TestGitHubProvider_ExactPinBeyondFirstPage(t *testing.T) {
	stub := justLikeStub()
	p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "1.46.0")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(1.46.0): %v", err)
	}
	if info.Tag != "1.46.0" || info.Version != "1.46.0" {
		t.Errorf("got tag %q version %q, want 1.46.0/1.46.0", info.Tag, info.Version)
	}
	// Found by direct lookup: the listing is read once, never paged.
	if len(stub.tagPages) != 1 {
		t.Errorf("tag listing requests = %v, want only page 1", stub.tagPages)
	}
}

// The same holds with a "v" tag prefix and a pin written without it.
func TestGitHubProvider_ExactPinBeyondFirstPage_TagPrefix(t *testing.T) {
	stub := &tagStub{pages: [][]string{
		fillerTags(100),
		fillerTags(100),
		{"v2.0.0"},
	}}
	p := NewGitHubProviderWithPrefix(stub.resolver(t), "owner/repo", "v", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "2.0.0")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(2.0.0): %v", err)
	}
	if info.Tag != "v2.0.0" || info.Version != "2.0.0" {
		t.Errorf("got tag %q version %q, want v2.0.0/2.0.0", info.Tag, info.Version)
	}
}

// A prefix pin needs a listing, so it pages through the tags and picks the
// highest match across all pages.
func TestGitHubProvider_MinorPinBeyondFirstPage(t *testing.T) {
	stub := justLikeStub()
	p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "1.46")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(1.46): %v", err)
	}
	if info.Tag != "1.46.1" {
		t.Errorf("got tag %q, want 1.46.1", info.Tag)
	}
	if got := stub.tagPages; len(got) != 4 || got[1] != 1 || got[3] != 3 {
		t.Errorf("tag listing requests = %v, want page 1, then pages 1-3", got)
	}
}

// A pinned version that does not exist still fails, and an exact pin does not
// page through the whole listing to find that out.
func TestGitHubProvider_ExactPinMissing(t *testing.T) {
	stub := justLikeStub()
	p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)

	_, err := ResolveWithinBoundary(context.Background(), p, "9.9.9")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ResolveWithinBoundary(9.9.9) error = %v, want not found", err)
	}
	if len(stub.tagPages) != 1 {
		t.Errorf("tag listing requests = %v, want only page 1", stub.tagPages)
	}
	if want := []string{"9.9.9", "v9.9.9"}; strings.Join(stub.refLookup, ",") != strings.Join(want, ",") {
		t.Errorf("ref lookups = %v, want %v", stub.refLookup, want)
	}
}

// A pin that matches on the first page resolves as it did before, without
// paging.
func TestGitHubProvider_PinOnFirstPage(t *testing.T) {
	stub := justLikeStub()
	p := NewGitHubProvider(stub.resolver(t), "owner/repo", nil)

	info, err := ResolveWithinBoundary(context.Background(), p, "0.9")
	if err != nil {
		t.Fatalf("ResolveWithinBoundary(0.9): %v", err)
	}
	if info.Tag != "v0.9.100" {
		t.Errorf("got tag %q, want v0.9.100", info.Tag)
	}
	for _, page := range stub.tagPages {
		if page != 1 {
			t.Errorf("tag listing requests = %v, want only page 1", stub.tagPages)
			break
		}
	}
}

func TestGitHubTagCandidates(t *testing.T) {
	tests := []struct {
		prefix, version string
		want            []string
	}{
		{"", "1.46.0", []string{"1.46.0", "v1.46.0"}},
		{"", "v1.46.0", []string{"v1.46.0", "1.46.0"}},
		{"v", "2.0.0", []string{"v2.0.0", "2.0.0"}},
		{"v", "v2.0.0", []string{"v2.0.0", "2.0.0"}},
		{"ruby-", "3.3.10", []string{"ruby-3.3.10", "3.3.10"}},
	}
	for _, tt := range tests {
		got := githubTagCandidates(tt.prefix, tt.version)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("githubTagCandidates(%q, %q) = %v, want %v", tt.prefix, tt.version, got, tt.want)
		}
	}
}
