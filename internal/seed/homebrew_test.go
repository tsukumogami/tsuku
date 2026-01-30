package seed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHomebrewSource_Fetch(t *testing.T) {
	fixture := analyticsResponse{
		Items: []analyticsItem{
			{Formula: "jq", Count: "500,000"},
			{Formula: "wget", Count: "200,000"},
			{Formula: "unknown-tool", Count: "50,000"},
			{Formula: "obscure-thing", Count: "100"},
		},
	}
	data, _ := json.Marshal(fixture)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	// Override the URL for testing by using a custom client that redirects
	src := &HomebrewSource{
		Client:       srv.Client(),
		AnalyticsURL: srv.URL,
	}

	packages, err := src.Fetch(3)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(packages))
	}

	// jq is tier 1 (curated)
	if packages[0].Tier != 1 {
		t.Errorf("jq tier = %d, want 1", packages[0].Tier)
	}
	if packages[0].ID != "homebrew:jq" {
		t.Errorf("jq id = %q, want homebrew:jq", packages[0].ID)
	}

	// wget is tier 1 (curated)
	if packages[1].Tier != 1 {
		t.Errorf("wget tier = %d, want 1", packages[1].Tier)
	}

	// unknown-tool with 50K installs is tier 2
	if packages[2].Tier != 2 {
		t.Errorf("unknown-tool tier = %d, want 2", packages[2].Tier)
	}

	// All should be pending
	for _, p := range packages {
		if p.Status != "pending" {
			t.Errorf("%s status = %q, want pending", p.Name, p.Status)
		}
		if p.Source != "homebrew" {
			t.Errorf("%s source = %q, want homebrew", p.Name, p.Source)
		}
	}
}

func TestHomebrewSource_FetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := &HomebrewSource{
		Client:       srv.Client(),
		AnalyticsURL: srv.URL,
	}

	_, err := src.Fetch(10)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
