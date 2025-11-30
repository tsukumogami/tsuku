package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsValidRubyGemsPackageName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid names
		{"simple", "bundler", true},
		{"with hyphen", "factory-bot", true},
		{"with underscore", "rspec_support", true},
		{"mixed case", "MyGem", true},
		{"with numbers", "rails5", true},

		// Invalid names
		{"empty", "", false},
		{"starts with number", "1gem", false},
		{"starts with hyphen", "-gem", false},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"contains dot", "my.gem", false},
		{"contains slash", "my/gem", false},
		{"contains at", "@scope", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidRubyGemsPackageName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidRubyGemsPackageName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestListRubyGemsVersions(t *testing.T) {
	// Create mock RubyGems API server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path
		if r.URL.Path != "/api/v1/versions/bundler.json" {
			http.NotFound(w, r)
			return
		}

		// Check headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept header application/json, got %s", r.Header.Get("Accept"))
		}

		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header to be set")
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		versions := []rubyGemsVersion{
			{Number: "2.5.33", Platform: "ruby", Prerelease: false},
			{Number: "2.5.32", Platform: "ruby", Prerelease: false},
			{Number: "2.5.31", Platform: "ruby", Prerelease: false},
			{Number: "2.6.0.pre", Platform: "ruby", Prerelease: true}, // Should be filtered
			{Number: "2.5.30", Platform: "java", Prerelease: false},   // Should be filtered (JRuby)
		}
		_ = json.NewEncoder(w).Encode(versions)
	}))
	defer server.Close()

	// Create resolver with mock server
	resolver := NewWithRubyGemsRegistry(server.URL)
	// Use the mock server's client to handle self-signed cert
	resolver.httpClient = server.Client()

	ctx := context.Background()
	versions, err := resolver.ListRubyGemsVersions(ctx, "bundler")
	if err != nil {
		t.Fatalf("ListRubyGemsVersions failed: %v", err)
	}

	// Should have 3 versions (excluding prerelease and java platform)
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d: %v", len(versions), versions)
	}

	// Should be sorted newest first
	if versions[0] != "2.5.33" {
		t.Errorf("expected first version to be 2.5.33, got %s", versions[0])
	}
}

func TestResolveRubyGems(t *testing.T) {
	// Create mock RubyGems API server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		versions := []rubyGemsVersion{
			{Number: "4.3.3", Platform: "ruby", Prerelease: false},
			{Number: "4.3.2", Platform: "ruby", Prerelease: false},
		}
		_ = json.NewEncoder(w).Encode(versions)
	}))
	defer server.Close()

	resolver := NewWithRubyGemsRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	info, err := resolver.ResolveRubyGems(ctx, "jekyll")
	if err != nil {
		t.Fatalf("ResolveRubyGems failed: %v", err)
	}

	if info.Version != "4.3.3" {
		t.Errorf("expected version 4.3.3, got %s", info.Version)
	}
}

func TestListRubyGemsVersions_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := NewWithRubyGemsRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ListRubyGemsVersions(ctx, "nonexistent-gem")
	if err == nil {
		t.Error("expected error for nonexistent gem")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", resolverErr.Type)
	}
}

func TestListRubyGemsVersions_RateLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	resolver := NewWithRubyGemsRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ListRubyGemsVersions(ctx, "bundler")
	if err == nil {
		t.Error("expected error for rate limit")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeRateLimit {
		t.Errorf("expected ErrTypeRateLimit for rate limit, got %v", resolverErr.Type)
	}
}

func TestListRubyGemsVersions_InvalidGemName(t *testing.T) {
	resolver := New()
	ctx := context.Background()

	_, err := resolver.ListRubyGemsVersions(ctx, "invalid;gem")
	if err == nil {
		t.Error("expected error for invalid gem name")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation, got %v", resolverErr.Type)
	}
}

func TestListRubyGemsVersions_HTTPSEnforcement(t *testing.T) {
	// Create a resolver with HTTP URL (should fail)
	resolver := NewWithRubyGemsRegistry("http://rubygems.org")

	ctx := context.Background()
	_, err := resolver.ListRubyGemsVersions(ctx, "bundler")
	if err == nil {
		t.Error("expected error for HTTP URL")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation for HTTPS enforcement, got %v", resolverErr.Type)
	}
}

func TestRubyGemsProvider_ResolveVersion(t *testing.T) {
	// Create mock server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		versions := []rubyGemsVersion{
			{Number: "2.5.33", Platform: "ruby", Prerelease: false},
			{Number: "2.5.32", Platform: "ruby", Prerelease: false},
			{Number: "2.4.10", Platform: "ruby", Prerelease: false},
		}
		_ = json.NewEncoder(w).Encode(versions)
	}))
	defer server.Close()

	resolver := NewWithRubyGemsRegistry(server.URL)
	resolver.httpClient = server.Client()
	provider := NewRubyGemsProvider(resolver, "bundler")

	ctx := context.Background()

	// Test exact match
	info, err := provider.ResolveVersion(ctx, "2.5.32")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}
	if info.Version != "2.5.32" {
		t.Errorf("expected version 2.5.32, got %s", info.Version)
	}

	// Test fuzzy match
	info, err = provider.ResolveVersion(ctx, "2.5")
	if err != nil {
		t.Fatalf("ResolveVersion fuzzy failed: %v", err)
	}
	if info.Version != "2.5.33" {
		t.Errorf("expected fuzzy version 2.5.33, got %s", info.Version)
	}

	// Test not found
	_, err = provider.ResolveVersion(ctx, "9.9.9")
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func TestRubyGemsProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewRubyGemsProvider(resolver, "bundler")

	desc := provider.SourceDescription()
	if desc != "rubygems:bundler" {
		t.Errorf("expected 'rubygems:bundler', got %s", desc)
	}
}
