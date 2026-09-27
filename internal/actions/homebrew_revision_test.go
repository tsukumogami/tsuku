package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeBrewServer serves the formulae.brew.sh formula JSON and the GHCR
// tag listing for one formula. tagPages holds the tag listing split into
// pages; each page but the last links to the next one.
func fakeBrewServer(t *testing.T, formula, stable string, revision int, tagPages [][]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/formula/"+formula+".json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"versions": map[string]string{"stable": stable},
			"revision": revision,
		})
	})
	mux.HandleFunc("/v2/homebrew/core/"+formula+"/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		page := 0
		if p := r.URL.Query().Get("page"); p != "" {
			_, _ = fmt.Sscanf(p, "%d", &page)
		}
		if page+1 < len(tagPages) {
			w.Header().Set("Link", fmt.Sprintf(`</v2/homebrew/core/%s/tags/list?page=%d>; rel="next"`, formula, page+1))
		}
		var tags []string
		if page < len(tagPages) {
			tags = tagPages[page]
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "homebrew/core/" + formula, "tags": tags})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oldGHCR, oldAPI := ghcrBaseURL, formulaeAPIBaseURL
	ghcrBaseURL, formulaeAPIBaseURL = srv.URL, srv.URL
	t.Cleanup(func() { ghcrBaseURL, formulaeAPIBaseURL = oldGHCR, oldAPI })
	return srv
}

func TestResolveBottleVersion(t *testing.T) {
	// gedit's shape on GHCR: the formula is now 50.0 at revision 2, and
	// the older 49.0 was only ever published unrevised.
	geditTags := [][]string{{"48.1", "49.0", "50.0", "50.0_1", "50.0_2"}}

	cases := []struct {
		name     string
		stable   string
		revision int
		tags     [][]string
		version  string
		want     string
	}{
		{
			name:   "current version takes the formula API revision",
			stable: "50.0", revision: 2, tags: geditTags,
			version: "50.0", want: "50.0_2",
		},
		{
			name:   "older version keeps its own (absent) revision",
			stable: "50.0", revision: 2, tags: geditTags,
			version: "49.0", want: "49.0",
		},
		{
			name:   "older version takes its own highest revision",
			stable: "13.50", revision: 1,
			tags:    [][]string{{"13.40", "13.40_3", "13.50", "13.50_1"}},
			version: "13.40", want: "13.40_3",
		},
		{
			// 13.40_1 resolved before this fix; it keeps that bottle.
			name:   "older version whose implied tag exists keeps it",
			stable: "13.50", revision: 1,
			tags:    [][]string{{"13.40", "13.40_1", "13.40_3", "13.50", "13.50_1"}},
			version: "13.40", want: "13.40_1",
		},
		{
			name:   "revision tags spread across pages",
			stable: "2.0", revision: 1,
			tags:    [][]string{{"1.0", "2.0"}, {"1.0_2", "2.0_1"}},
			version: "1.0", want: "1.0_2",
		},
		{
			name:   "prefix of another version is not a match",
			stable: "1.10", revision: 0,
			tags:    [][]string{{"1.1_5", "1.10"}},
			version: "1.1", want: "1.1_5",
		},
		{
			// Nothing published for 47.0: the request stays what it
			// was, and getBlobSHA reports the missing manifest.
			name:   "unpublished older version keeps the implied request",
			stable: "50.0", revision: 2, tags: geditTags,
			version: "47.0", want: "47.0_2",
		},
		{
			name:   "revision-0 current version",
			stable: "1.13.2", revision: 0,
			tags:    [][]string{{"1.13.1", "1.13.2"}},
			version: "1.13.2", want: "1.13.2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeBrewServer(t, "gedit", tc.stable, tc.revision, tc.tags)
			a := &HomebrewAction{}
			got, err := a.resolveBottleVersion(context.Background(), "gedit", tc.version, "test-token")
			if err != nil {
				t.Fatalf("resolveBottleVersion: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveBottleVersion(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

func TestResolveBottleVersion_FormulaAPIDown(t *testing.T) {
	// With formulae.brew.sh unreachable, the tag listing still gives the
	// requested version its revision.
	fakeBrewServer(t, "gedit", "", 0, [][]string{{"49.0", "50.0_2"}})
	formulaeAPIBaseURL = "http://127.0.0.1:1"

	a := &HomebrewAction{}
	got, err := a.resolveBottleVersion(context.Background(), "gedit", "50.0", "test-token")
	if err != nil {
		t.Fatalf("resolveBottleVersion: %v", err)
	}
	if got != "50.0_2" {
		t.Errorf("resolveBottleVersion = %q, want 50.0_2", got)
	}
}

func TestSelectVersionTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tags    []string
		version string
		want    string
		wantOK  bool
	}{
		{[]string{"1.0", "1.0_1", "1.0_10", "1.0_9"}, "1.0", "1.0_10", true},
		{[]string{"1.0"}, "1.0", "1.0", true},
		{[]string{"1.0_abc", "1.0.1"}, "1.0", "", false},
		{nil, "1.0", "", false},
	}
	for _, tc := range cases {
		got, ok := selectVersionTag(tc.tags, tc.version)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("selectVersionTag(%v, %q) = (%q, %v), want (%q, %v)", tc.tags, tc.version, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestNextPageURL(t *testing.T) {
	t.Parallel()
	cur := "https://ghcr.io/v2/homebrew/core/gedit/tags/list"
	if got := nextPageURL(`</v2/homebrew/core/gedit/tags/list?last=49.0&n=100>; rel="next"`, cur); got != "https://ghcr.io/v2/homebrew/core/gedit/tags/list?last=49.0&n=100" {
		t.Errorf("nextPageURL = %q", got)
	}
	if got := nextPageURL("", cur); got != "" {
		t.Errorf("nextPageURL(empty) = %q, want empty", got)
	}
}
