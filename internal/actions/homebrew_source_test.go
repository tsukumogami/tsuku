package actions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestHomebrewSourceAction_Name(t *testing.T) {
	t.Parallel()
	action := &HomebrewSourceAction{}
	if action.Name() != "homebrew_source" {
		t.Errorf("Name() = %q, want %q", action.Name(), "homebrew_source")
	}
}

func TestHomebrewSourceAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &HomebrewSourceAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe:     &recipe.Recipe{},
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'formula' parameter is missing")
	}
}

func TestHomebrewSourceAction_Decompose_MissingParams(t *testing.T) {
	t.Parallel()
	action := &HomebrewSourceAction{}

	evalCtx := &EvalContext{
		Context: context.Background(),
	}

	_, err := action.Decompose(evalCtx, map[string]interface{}{})
	if err == nil {
		t.Error("Decompose() should fail when 'formula' parameter is missing")
	}
}

func TestHomebrewSourceAction_Decompose(t *testing.T) {
	t.Parallel()
	// Create a mock Homebrew API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "jq",
			"urls": map[string]interface{}{
				"stable": map[string]interface{}{
					"url":      "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-1.7.1.tar.gz",
					"checksum": "478c9ca129fd2e3443fe27314b455e211e0d8c60bc8ff7df703f25571c92f67",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	action := &HomebrewSourceAction{
		HomebrewAPIURL: server.URL,
	}

	evalCtx := &EvalContext{
		Context: context.Background(),
	}

	params := map[string]interface{}{
		"formula": "jq",
	}

	steps, err := action.Decompose(evalCtx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	// Should return 2 steps: download and extract
	if len(steps) != 2 {
		t.Fatalf("Decompose() returned %d steps, want 2", len(steps))
	}

	// First step should be download
	if steps[0].Action != "download" {
		t.Errorf("steps[0].Action = %q, want %q", steps[0].Action, "download")
	}

	// Verify download URL
	url, ok := steps[0].Params["url"].(string)
	if !ok || url != "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-1.7.1.tar.gz" {
		t.Errorf("steps[0].Params[url] = %v, want URL", steps[0].Params["url"])
	}

	// Verify checksum is set
	if steps[0].Checksum == "" {
		t.Error("steps[0].Checksum should be set")
	}

	// Second step should be extract
	if steps[1].Action != "extract" {
		t.Errorf("steps[1].Action = %q, want %q", steps[1].Action, "extract")
	}

	// Verify format is detected
	format, ok := steps[1].Params["format"].(string)
	if !ok || format != "tar.gz" {
		t.Errorf("steps[1].Params[format] = %v, want tar.gz", steps[1].Params["format"])
	}
}

func TestHomebrewSourceAction_Decompose_StripDirs(t *testing.T) {
	t.Parallel()
	// Create a mock Homebrew API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "jq",
			"urls": map[string]interface{}{
				"stable": map[string]interface{}{
					"url":      "https://example.com/jq.tar.gz",
					"checksum": "abc123",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	action := &HomebrewSourceAction{
		HomebrewAPIURL: server.URL,
	}

	evalCtx := &EvalContext{
		Context: context.Background(),
	}

	// Test default strip_dirs (1)
	params := map[string]interface{}{
		"formula": "jq",
	}

	steps, err := action.Decompose(evalCtx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	stripDirs, ok := steps[1].Params["strip_dirs"].(int)
	if !ok || stripDirs != 1 {
		t.Errorf("default strip_dirs = %v, want 1", steps[1].Params["strip_dirs"])
	}

	// Test custom strip_dirs
	params["strip_dirs"] = 2
	steps, err = action.Decompose(evalCtx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	stripDirs, ok = steps[1].Params["strip_dirs"].(int)
	if !ok || stripDirs != 2 {
		t.Errorf("custom strip_dirs = %v, want 2", steps[1].Params["strip_dirs"])
	}
}

func TestHomebrewSourceAction_Decompose_APIError(t *testing.T) {
	t.Parallel()
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	action := &HomebrewSourceAction{
		HomebrewAPIURL: server.URL,
	}

	evalCtx := &EvalContext{
		Context: context.Background(),
	}

	params := map[string]interface{}{
		"formula": "nonexistent",
	}

	_, err := action.Decompose(evalCtx, params)
	if err == nil {
		t.Error("Decompose() should fail when API returns error")
	}
}

func TestHomebrewSourceAction_Decompose_NoSourceURL(t *testing.T) {
	t.Parallel()
	// Create a mock server that returns formula without source URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "jq",
			"urls": map[string]interface{}{
				"stable": map[string]interface{}{
					"url":      "",
					"checksum": "",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	action := &HomebrewSourceAction{
		HomebrewAPIURL: server.URL,
	}

	evalCtx := &EvalContext{
		Context: context.Background(),
	}

	params := map[string]interface{}{
		"formula": "jq",
	}

	_, err := action.Decompose(evalCtx, params)
	if err == nil {
		t.Error("Decompose() should fail when formula has no source URL")
	}
}

func TestHomebrewSourceAction_DetectArchiveFormat(t *testing.T) {
	t.Parallel()
	action := &HomebrewSourceAction{}

	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/file.tar.gz", "tar.gz"},
		{"https://example.com/file.tgz", "tar.gz"},
		{"https://example.com/file.TAR.GZ", "tar.gz"},
		{"https://example.com/file.tar.xz", "tar.xz"},
		{"https://example.com/file.tar.bz2", "tar.bz2"},
		{"https://example.com/file.zip", "zip"},
		{"https://example.com/file.ZIP", "zip"},
		{"https://example.com/file", "tar.gz"}, // Default
		{"https://example.com/file.unknown", "tar.gz"},
	}

	for _, tc := range tests {
		result := action.detectArchiveFormat(tc.url)
		if result != tc.expected {
			t.Errorf("detectArchiveFormat(%q) = %q, want %q", tc.url, result, tc.expected)
		}
	}
}

func TestHomebrewSourceAction_ImplementsDecomposable(t *testing.T) {
	t.Parallel()
	action := &HomebrewSourceAction{}

	// Verify action implements Decomposable interface
	var _ Decomposable = action
}
