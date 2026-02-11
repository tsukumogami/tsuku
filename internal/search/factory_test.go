package search

import (
	"os"
	"strings"
	"testing"
)

func TestNewSearchProvider_ExplicitDDG(t *testing.T) {
	p, err := NewSearchProvider("ddg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ddg" {
		t.Errorf("expected provider name 'ddg', got %q", p.Name())
	}
}

func TestNewSearchProvider_ExplicitTavily(t *testing.T) {
	// Set up environment
	os.Setenv("TAVILY_API_KEY", "test-tavily-key")
	defer os.Unsetenv("TAVILY_API_KEY")

	p, err := NewSearchProvider("tavily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "tavily" {
		t.Errorf("expected provider name 'tavily', got %q", p.Name())
	}
}

func TestNewSearchProvider_ExplicitTavilyMissingKey(t *testing.T) {
	// Ensure no key is set
	os.Unsetenv("TAVILY_API_KEY")

	_, err := NewSearchProvider("tavily")
	if err == nil {
		t.Fatal("expected error for missing TAVILY_API_KEY")
	}
	if !strings.Contains(err.Error(), "TAVILY_API_KEY") {
		t.Errorf("expected error to mention TAVILY_API_KEY, got: %v", err)
	}
}

func TestNewSearchProvider_ExplicitBrave(t *testing.T) {
	// Set up environment
	os.Setenv("BRAVE_API_KEY", "test-brave-key")
	defer os.Unsetenv("BRAVE_API_KEY")

	p, err := NewSearchProvider("brave")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "brave" {
		t.Errorf("expected provider name 'brave', got %q", p.Name())
	}
}

func TestNewSearchProvider_ExplicitBraveMissingKey(t *testing.T) {
	// Ensure no key is set
	os.Unsetenv("BRAVE_API_KEY")

	_, err := NewSearchProvider("brave")
	if err == nil {
		t.Fatal("expected error for missing BRAVE_API_KEY")
	}
	if !strings.Contains(err.Error(), "BRAVE_API_KEY") {
		t.Errorf("expected error to mention BRAVE_API_KEY, got: %v", err)
	}
}

func TestNewSearchProvider_InvalidProvider(t *testing.T) {
	_, err := NewSearchProvider("invalid")
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
	if !strings.Contains(err.Error(), "invalid search provider") {
		t.Errorf("expected 'invalid search provider' error, got: %v", err)
	}
}

func TestNewSearchProvider_AutoSelectTavily(t *testing.T) {
	// Set up environment - Tavily takes priority
	os.Setenv("TAVILY_API_KEY", "test-tavily-key")
	os.Setenv("BRAVE_API_KEY", "test-brave-key")
	defer os.Unsetenv("TAVILY_API_KEY")
	defer os.Unsetenv("BRAVE_API_KEY")

	p, err := NewSearchProvider("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "tavily" {
		t.Errorf("expected auto-selected provider 'tavily', got %q", p.Name())
	}
}

func TestNewSearchProvider_AutoSelectBrave(t *testing.T) {
	// Set up environment - only Brave key
	os.Unsetenv("TAVILY_API_KEY")
	os.Setenv("BRAVE_API_KEY", "test-brave-key")
	defer os.Unsetenv("BRAVE_API_KEY")

	p, err := NewSearchProvider("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "brave" {
		t.Errorf("expected auto-selected provider 'brave', got %q", p.Name())
	}
}

func TestNewSearchProvider_AutoSelectDDG(t *testing.T) {
	// Ensure no API keys are set
	os.Unsetenv("TAVILY_API_KEY")
	os.Unsetenv("BRAVE_API_KEY")

	p, err := NewSearchProvider("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ddg" {
		t.Errorf("expected auto-selected provider 'ddg', got %q", p.Name())
	}
}

func TestValidProviders(t *testing.T) {
	providers := ValidProviders()
	if len(providers) != 3 {
		t.Errorf("expected 3 providers, got %d", len(providers))
	}

	expected := map[string]bool{"ddg": true, "tavily": true, "brave": true}
	for _, p := range providers {
		if !expected[p] {
			t.Errorf("unexpected provider %q in list", p)
		}
	}
}
