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
	// Set up environment - t.Setenv handles cleanup automatically
	t.Setenv("TAVILY_API_KEY", "test-tavily-key")

	p, err := NewSearchProvider("tavily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "tavily" {
		t.Errorf("expected provider name 'tavily', got %q", p.Name())
	}
}

func TestNewSearchProvider_ExplicitTavilyMissingKey(t *testing.T) {
	// Ensure no key is set - save and restore any existing value
	if orig, exists := os.LookupEnv("TAVILY_API_KEY"); exists {
		t.Setenv("TAVILY_API_KEY", orig) // This will restore after test
		os.Unsetenv("TAVILY_API_KEY")    //nolint:errcheck // intentionally ignoring
	}

	_, err := NewSearchProvider("tavily")
	if err == nil {
		t.Fatal("expected error for missing TAVILY_API_KEY")
	}
	if !strings.Contains(err.Error(), "TAVILY_API_KEY") {
		t.Errorf("expected error to mention TAVILY_API_KEY, got: %v", err)
	}
}

func TestNewSearchProvider_ExplicitBrave(t *testing.T) {
	// Set up environment - t.Setenv handles cleanup automatically
	t.Setenv("BRAVE_API_KEY", "test-brave-key")

	p, err := NewSearchProvider("brave")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "brave" {
		t.Errorf("expected provider name 'brave', got %q", p.Name())
	}
}

func TestNewSearchProvider_ExplicitBraveMissingKey(t *testing.T) {
	// Ensure no key is set - save and restore any existing value
	if orig, exists := os.LookupEnv("BRAVE_API_KEY"); exists {
		t.Setenv("BRAVE_API_KEY", orig) // This will restore after test
		os.Unsetenv("BRAVE_API_KEY")    //nolint:errcheck // intentionally ignoring
	}

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
	t.Setenv("TAVILY_API_KEY", "test-tavily-key")
	t.Setenv("BRAVE_API_KEY", "test-brave-key")

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
	// Ensure TAVILY_API_KEY is not set
	if orig, exists := os.LookupEnv("TAVILY_API_KEY"); exists {
		t.Setenv("TAVILY_API_KEY", orig) // This will restore after test
		os.Unsetenv("TAVILY_API_KEY")    //nolint:errcheck // intentionally ignoring
	}
	t.Setenv("BRAVE_API_KEY", "test-brave-key")

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
	if orig, exists := os.LookupEnv("TAVILY_API_KEY"); exists {
		t.Setenv("TAVILY_API_KEY", orig) // This will restore after test
		os.Unsetenv("TAVILY_API_KEY")    //nolint:errcheck // intentionally ignoring
	}
	if orig, exists := os.LookupEnv("BRAVE_API_KEY"); exists {
		t.Setenv("BRAVE_API_KEY", orig) // This will restore after test
		os.Unsetenv("BRAVE_API_KEY")    //nolint:errcheck // intentionally ignoring
	}

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
