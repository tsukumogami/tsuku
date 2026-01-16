package builders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCaskBuilder_Name(t *testing.T) {
	b := NewCaskBuilder(nil)
	if got := b.Name(); got != "cask" {
		t.Errorf("Name() = %q, want %q", got, "cask")
	}
}

func TestCaskBuilder_RequiresLLM(t *testing.T) {
	b := NewCaskBuilder(nil)
	if got := b.RequiresLLM(); got != false {
		t.Errorf("RequiresLLM() = %v, want false", got)
	}
}

func TestCaskBuilder_CanBuild_ValidCask(t *testing.T) {
	// Mock server returning a valid cask with app artifact
	mockResp := map[string]interface{}{
		"token":    "iterm2",
		"version":  "3.5.0",
		"sha256":   "abc123def456",
		"url":      "https://iterm2.com/downloads/stable/iTerm2-3_5_0.zip",
		"name":     []string{"iTerm2"},
		"desc":     "Terminal emulator",
		"homepage": "https://iterm2.com/",
		"artifacts": []map[string]interface{}{
			{"app": []string{"iTerm.app"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cask/iterm2.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "iterm2"}

	canBuild, err := b.CanBuild(context.Background(), req)
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true for valid cask with app artifact")
	}
}

func TestCaskBuilder_CanBuild_PkgCask(t *testing.T) {
	// Mock server returning a cask with pkg artifact
	mockResp := map[string]interface{}{
		"token":   "some-pkg-app",
		"version": "1.0.0",
		"sha256":  "abc123",
		"url":     "https://example.com/app.pkg",
		"artifacts": []map[string]interface{}{
			{"pkg": []string{"app.pkg"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "some-pkg-app"}

	canBuild, err := b.CanBuild(context.Background(), req)
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for cask with pkg artifact")
	}
}

func TestCaskBuilder_CanBuild_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "nonexistent-cask"}

	canBuild, err := b.CanBuild(context.Background(), req)
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for nonexistent cask")
	}
}

func TestCaskBuilder_CanBuild_InvalidName(t *testing.T) {
	b := NewCaskBuilder(nil)
	req := BuildRequest{Package: "../invalid/path"}

	canBuild, err := b.CanBuild(context.Background(), req)
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for invalid cask name")
	}
}

func TestCaskBuilder_ExtractArtifacts_AppOnly(t *testing.T) {
	b := NewCaskBuilder(nil)
	artifacts := []map[string]interface{}{
		{"app": []interface{}{"Firefox.app"}},
	}

	appName, binaries, hasApp, err := b.extractArtifacts(artifacts)
	if err != nil {
		t.Fatalf("extractArtifacts() error = %v", err)
	}
	if appName != "Firefox.app" {
		t.Errorf("appName = %q, want %q", appName, "Firefox.app")
	}
	if len(binaries) != 0 {
		t.Errorf("binaries = %v, want empty", binaries)
	}
	if !hasApp {
		t.Error("hasApp = false, want true")
	}
}

func TestCaskBuilder_ExtractArtifacts_AppWithBinary(t *testing.T) {
	b := NewCaskBuilder(nil)
	artifacts := []map[string]interface{}{
		{"app": []interface{}{"Visual Studio Code.app"}},
		{"binary": []interface{}{"{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code"}},
	}

	appName, binaries, hasApp, err := b.extractArtifacts(artifacts)
	if err != nil {
		t.Fatalf("extractArtifacts() error = %v", err)
	}
	if appName != "Visual Studio Code.app" {
		t.Errorf("appName = %q, want %q", appName, "Visual Studio Code.app")
	}
	if len(binaries) != 1 {
		t.Fatalf("len(binaries) = %d, want 1", len(binaries))
	}
	if binaries[0] != "Contents/Resources/app/bin/code" {
		t.Errorf("binaries[0] = %q, want %q", binaries[0], "Contents/Resources/app/bin/code")
	}
	if !hasApp {
		t.Error("hasApp = false, want true")
	}
}

func TestCaskBuilder_ExtractArtifacts_BinaryOnly(t *testing.T) {
	b := NewCaskBuilder(nil)
	// Some casks have only binary artifacts (rare but possible)
	artifacts := []map[string]interface{}{
		{"binary": []interface{}{"/usr/local/bin/sometool"}},
	}

	appName, binaries, hasApp, err := b.extractArtifacts(artifacts)
	if err != nil {
		t.Fatalf("extractArtifacts() error = %v", err)
	}
	if appName != "" {
		t.Errorf("appName = %q, want empty", appName)
	}
	if len(binaries) != 1 {
		t.Fatalf("len(binaries) = %d, want 1", len(binaries))
	}
	if binaries[0] != "sometool" {
		t.Errorf("binaries[0] = %q, want %q", binaries[0], "sometool")
	}
	if hasApp {
		t.Error("hasApp = true, want false")
	}
}

func TestCaskBuilder_ExtractArtifacts_UnsupportedPkg(t *testing.T) {
	b := NewCaskBuilder(nil)
	artifacts := []map[string]interface{}{
		{"pkg": []interface{}{"SomeApp.pkg"}},
	}

	_, _, _, err := b.extractArtifacts(artifacts)
	if err == nil {
		t.Fatal("extractArtifacts() error = nil, want error for pkg artifact")
	}

	var unsupportedErr *CaskUnsupportedArtifactError
	if _, ok := err.(*CaskUnsupportedArtifactError); !ok {
		t.Errorf("error type = %T, want *CaskUnsupportedArtifactError", err)
	} else {
		unsupportedErr = err.(*CaskUnsupportedArtifactError)
		if unsupportedErr.ArtifactType != "pkg" {
			t.Errorf("ArtifactType = %q, want %q", unsupportedErr.ArtifactType, "pkg")
		}
	}
}

func TestCaskBuilder_ExtractArtifacts_UnsupportedPreflight(t *testing.T) {
	b := NewCaskBuilder(nil)
	artifacts := []map[string]interface{}{
		{"preflight": map[string]interface{}{}},
		{"app": []interface{}{"SomeApp.app"}},
	}

	_, _, _, err := b.extractArtifacts(artifacts)
	if err == nil {
		t.Fatal("extractArtifacts() error = nil, want error for preflight artifact")
	}

	if unsupportedErr, ok := err.(*CaskUnsupportedArtifactError); ok {
		if unsupportedErr.ArtifactType != "preflight" {
			t.Errorf("ArtifactType = %q, want %q", unsupportedErr.ArtifactType, "preflight")
		}
	} else {
		t.Errorf("error type = %T, want *CaskUnsupportedArtifactError", err)
	}
}

func TestCaskBuilder_NormalizeBinaryPath(t *testing.T) {
	tests := []struct {
		name    string
		binPath string
		appName string
		want    string
	}{
		{
			name:    "appdir placeholder with app name",
			binPath: "{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code",
			appName: "Visual Studio Code.app",
			want:    "Contents/Resources/app/bin/code",
		},
		{
			name:    "already relative Contents path",
			binPath: "Contents/MacOS/firefox",
			appName: "Firefox.app",
			want:    "Contents/MacOS/firefox",
		},
		{
			name:    "absolute binary path",
			binPath: "/usr/local/bin/sometool",
			appName: "",
			want:    "sometool",
		},
		{
			name:    "appdir without app name in path",
			binPath: "{{appdir}}/Contents/MacOS/tool",
			appName: "App.app",
			want:    "Contents/MacOS/tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBinaryPath(tt.binPath, tt.appName)
			if got != tt.want {
				t.Errorf("normalizeBinaryPath(%q, %q) = %q, want %q", tt.binPath, tt.appName, got, tt.want)
			}
		})
	}
}

func TestCaskBuilder_GenerateRecipe(t *testing.T) {
	// Mock server returning VS Code cask
	mockResp := map[string]interface{}{
		"token":    "visual-studio-code",
		"version":  "1.96.4",
		"sha256":   "abc123def456789",
		"url":      "https://update.code.visualstudio.com/1.96.4/darwin-arm64/stable",
		"name":     []string{"Visual Studio Code", "VS Code"},
		"desc":     "Code editing. Redefined.",
		"homepage": "https://code.visualstudio.com/",
		"artifacts": []map[string]interface{}{
			{"app": []interface{}{"Visual Studio Code.app"}},
			{"binary": []interface{}{"{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cask/visual-studio-code.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "visual-studio-code"}

	result, err := b.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	recipe := result.Recipe

	// Verify metadata
	if recipe.Metadata.Name != "visual-studio-code" {
		t.Errorf("Metadata.Name = %q, want %q", recipe.Metadata.Name, "visual-studio-code")
	}
	if recipe.Metadata.Description != "Code editing. Redefined." {
		t.Errorf("Metadata.Description = %q, want %q", recipe.Metadata.Description, "Code editing. Redefined.")
	}
	if recipe.Metadata.Homepage != "https://code.visualstudio.com/" {
		t.Errorf("Metadata.Homepage = %q, want %q", recipe.Metadata.Homepage, "https://code.visualstudio.com/")
	}

	// Verify version section
	if recipe.Version.Source != "cask" {
		t.Errorf("Version.Source = %q, want %q", recipe.Version.Source, "cask")
	}
	if recipe.Version.Cask != "visual-studio-code" {
		t.Errorf("Version.Cask = %q, want %q", recipe.Version.Cask, "visual-studio-code")
	}

	// Verify steps
	if len(recipe.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(recipe.Steps))
	}

	step := recipe.Steps[0]
	if step.Action != "app_bundle" {
		t.Errorf("Step.Action = %q, want %q", step.Action, "app_bundle")
	}

	// Verify step parameters
	if url, ok := step.Params["url"].(string); !ok || url != "{{version.url}}" {
		t.Errorf("Step.Params[url] = %v, want %q", step.Params["url"], "{{version.url}}")
	}
	if checksum, ok := step.Params["checksum"].(string); !ok || checksum != "{{version.checksum}}" {
		t.Errorf("Step.Params[checksum] = %v, want %q", step.Params["checksum"], "{{version.checksum}}")
	}
	if appName, ok := step.Params["app_name"].(string); !ok || appName != "Visual Studio Code.app" {
		t.Errorf("Step.Params[app_name] = %v, want %q", step.Params["app_name"], "Visual Studio Code.app")
	}

	binaries, ok := step.Params["binaries"].([]string)
	if !ok {
		t.Fatalf("Step.Params[binaries] type = %T, want []string", step.Params["binaries"])
	}
	if len(binaries) != 1 || binaries[0] != "Contents/Resources/app/bin/code" {
		t.Errorf("Step.Params[binaries] = %v, want [Contents/Resources/app/bin/code]", binaries)
	}

	// Verify verify section uses first binary
	if recipe.Verify.Command != "Contents/Resources/app/bin/code --version" {
		t.Errorf("Verify.Command = %q, want %q", recipe.Verify.Command, "Contents/Resources/app/bin/code --version")
	}

	// Verify result source
	if result.Source != "cask:visual-studio-code" {
		t.Errorf("Result.Source = %q, want %q", result.Source, "cask:visual-studio-code")
	}
}

func TestCaskBuilder_GenerateRecipe_AppOnly(t *testing.T) {
	// Mock server returning Firefox cask (app only, no binary)
	mockResp := map[string]interface{}{
		"token":    "firefox",
		"version":  "133.0",
		"sha256":   "abc123",
		"url":      "https://download.mozilla.org/...",
		"name":     []string{"Mozilla Firefox"},
		"desc":     "Web browser",
		"homepage": "https://www.mozilla.org/firefox/",
		"artifacts": []map[string]interface{}{
			{"app": []interface{}{"Firefox.app"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "firefox"}

	result, err := b.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	recipe := result.Recipe

	// For app-only cask, verify command should check app exists
	if !contains(recipe.Verify.Command, "test -d") {
		t.Errorf("Verify.Command = %q, want test -d command for app-only cask", recipe.Verify.Command)
	}

	// Should have app_name but no binaries
	step := recipe.Steps[0]
	if _, ok := step.Params["binaries"]; ok {
		t.Error("Step.Params has binaries, want none for app-only cask")
	}
	if _, ok := step.Params["app_name"]; !ok {
		t.Error("Step.Params missing app_name for app-only cask")
	}
}

func TestCaskBuilder_GenerateRecipe_NoChecksum(t *testing.T) {
	// Mock server returning cask with :no_check
	mockResp := map[string]interface{}{
		"token":   "some-app",
		"version": "1.0.0",
		"sha256":  ":no_check",
		"url":     "https://example.com/app.dmg",
		"artifacts": []map[string]interface{}{
			{"app": []interface{}{"SomeApp.app"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "some-app"}

	result, err := b.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should have warning about missing checksum
	if len(result.Warnings) == 0 {
		t.Error("len(Warnings) = 0, want warning about missing checksum")
	}

	foundWarning := false
	for _, w := range result.Warnings {
		if contains(w, "checksum") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("Warnings = %v, want warning containing 'checksum'", result.Warnings)
	}
}

func TestIsValidCaskName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid simple", "firefox", true},
		{"valid with hyphen", "visual-studio-code", true},
		{"valid with underscore", "some_app", true},
		{"valid with at sign", "openssl@3", true},
		{"valid with dot", "app.v2", true},
		{"invalid uppercase", "Firefox", false},
		{"invalid path traversal", "../etc/passwd", false},
		{"invalid forward slash", "foo/bar", false},
		{"invalid backslash", "foo\\bar", false},
		{"invalid starts with hyphen", "-app", false},
		{"invalid empty", "", false},
		{"invalid too long", string(make([]byte, 129)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCaskName(tt.input)
			if got != tt.want {
				t.Errorf("isValidCaskName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCaskBuilder_Session(t *testing.T) {
	// Mock server returning a valid cask
	mockResp := map[string]interface{}{
		"token":    "iterm2",
		"version":  "3.5.0",
		"sha256":   "abc123",
		"url":      "https://iterm2.com/downloads/stable/iTerm2-3_5_0.zip",
		"name":     []string{"iTerm2"},
		"desc":     "Terminal emulator",
		"homepage": "https://iterm2.com/",
		"artifacts": []map[string]interface{}{
			{"app": []interface{}{"iTerm.app"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	b := NewCaskBuilderWithBaseURL(nil, server.URL)
	req := BuildRequest{Package: "iterm2"}

	session, err := b.NewSession(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.Close()

	result, err := session.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if result.Recipe == nil {
		t.Fatal("Generate() returned nil recipe")
	}
	if result.Recipe.Metadata.Name != "iterm2" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "iterm2")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
