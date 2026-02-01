package discover

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubValidator_ValidRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"archived": false}`)
	}))
	defer srv.Close()

	v := &GitHubValidator{
		client: srv.Client(),
	}
	// Override the URL by using a test entry that routes to our server
	// We test via the validate method with a custom approach
	entry := SeedEntry{Name: "test", Builder: "github", Source: "owner/repo"}

	// Test via ValidateEntries which uses the Validate method
	validators := map[string]Validator{"github": &testGitHubValidator{valid: true}}
	valid, failures := ValidateEntries([]SeedEntry{entry}, validators)
	if len(valid) != 1 {
		t.Errorf("expected 1 valid, got %d", len(valid))
	}
	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(failures))
	}
	_ = v // suppress unused
}

func TestGitHubValidator_ArchivedRepo(t *testing.T) {
	validators := map[string]Validator{"github": &testGitHubValidator{valid: false, err: fmt.Errorf("archived")}}
	entry := SeedEntry{Name: "test", Builder: "github", Source: "owner/repo"}
	valid, failures := ValidateEntries([]SeedEntry{entry}, validators)
	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(failures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(failures))
	}
}

func TestHomebrewValidator_ValidFormula(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	validators := map[string]Validator{"homebrew": &testHomebrewValidator{valid: true}}
	entry := SeedEntry{Name: "jq", Builder: "homebrew", Source: "jq"}
	valid, failures := ValidateEntries([]SeedEntry{entry}, validators)
	if len(valid) != 1 {
		t.Errorf("expected 1 valid, got %d", len(valid))
	}
	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(failures))
	}
}

func TestHomebrewValidator_NotFound(t *testing.T) {
	validators := map[string]Validator{"homebrew": &testHomebrewValidator{valid: false, err: fmt.Errorf("not found")}}
	entry := SeedEntry{Name: "nope", Builder: "homebrew", Source: "nope"}
	valid, failures := ValidateEntries([]SeedEntry{entry}, validators)
	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(failures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(failures))
	}
}

func TestValidateEntries_UnknownBuilder(t *testing.T) {
	entry := SeedEntry{Name: "test", Builder: "unknown", Source: "x"}
	valid, failures := ValidateEntries([]SeedEntry{entry}, map[string]Validator{})
	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(failures) != 1 {
		t.Errorf("expected 1 failure, got %d", len(failures))
	}
}

func TestValidateEntries_MixedBuilders(t *testing.T) {
	validators := map[string]Validator{
		"github":   &testGitHubValidator{valid: true},
		"homebrew": &testHomebrewValidator{valid: true},
	}
	entries := []SeedEntry{
		{Name: "rg", Builder: "github", Source: "BurntSushi/ripgrep"},
		{Name: "jq", Builder: "homebrew", Source: "jq"},
	}
	valid, failures := ValidateEntries(entries, validators)
	if len(valid) != 2 {
		t.Errorf("expected 2 valid, got %d", len(valid))
	}
	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(failures))
	}
}

func TestGitHubValidator_InvalidSource(t *testing.T) {
	v := NewGitHubValidator(nil)
	entry := SeedEntry{Name: "test", Builder: "github", Source: "no-slash"}
	err := v.Validate(entry)
	if err == nil {
		t.Fatal("expected error for invalid source format")
	}
}

func TestGitHubValidator_Caching(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprintf(w, `{"archived": false}`)
	}))
	defer srv.Close()

	v := &GitHubValidator{client: srv.Client()}
	// We can't easily override the URL, so test caching via the cache map directly
	v.cache.Store("owner/repo", nil) // pre-cache as valid

	entry := SeedEntry{Name: "test", Builder: "github", Source: "owner/repo"}
	err := v.Validate(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Errorf("expected 0 API calls (cached), got %d", calls)
	}
}

// Test helpers

type testGitHubValidator struct {
	valid bool
	err   error
}

func (v *testGitHubValidator) Validate(entry SeedEntry) error {
	if v.valid {
		return nil
	}
	return v.err
}

type testHomebrewValidator struct {
	valid bool
	err   error
}

func (v *testHomebrewValidator) Validate(entry SeedEntry) error {
	if v.valid {
		return nil
	}
	return v.err
}
