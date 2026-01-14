package actions

import (
	"runtime"
	"testing"
)

func TestAppBundleAction_Name(t *testing.T) {
	action := &AppBundleAction{}
	if action.Name() != "app_bundle" {
		t.Errorf("Name() = %q, want %q", action.Name(), "app_bundle")
	}
}

func TestAppBundleAction_IsDeterministic(t *testing.T) {
	action := &AppBundleAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() should return true")
	}
}

func TestAppBundleAction_Preflight_RequiredParams(t *testing.T) {
	action := &AppBundleAction{}

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
	}{
		{
			name:       "all params missing",
			params:     map[string]interface{}{},
			wantErrors: 3, // url, checksum, app_name
		},
		{
			name: "only url provided",
			params: map[string]interface{}{
				"url": "https://example.com/app.zip",
			},
			wantErrors: 2, // checksum, app_name
		},
		{
			name: "only url and checksum provided",
			params: map[string]interface{}{
				"url":      "https://example.com/app.zip",
				"checksum": "sha256:abc123",
			},
			wantErrors: 1, // app_name
		},
		{
			name: "all required params provided",
			params: map[string]interface{}{
				"url":      "https://example.com/app.zip",
				"checksum": "sha256:abc123",
				"app_name": "MyApp.app",
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %d, want %d; errors: %v", len(result.Errors), tt.wantErrors, result.Errors)
			}
		})
	}
}

func TestAppBundleAction_Preflight_macOSWarning(t *testing.T) {
	action := &AppBundleAction{}

	params := map[string]interface{}{
		"url":      "https://example.com/app.zip",
		"checksum": "sha256:abc123",
		"app_name": "MyApp.app",
	}

	result := action.Preflight(params)

	// On non-macOS, should have a warning
	if runtime.GOOS != "darwin" {
		if len(result.Warnings) == 0 {
			t.Error("Preflight() should warn about macOS-only action on non-darwin")
		}
	}
}

func TestAppBundleAction_Registered(t *testing.T) {
	action := Get("app_bundle")
	if action == nil {
		t.Error("app_bundle action not registered")
	}
	if action.Name() != "app_bundle" {
		t.Errorf("registered action Name() = %q, want %q", action.Name(), "app_bundle")
	}
}

func TestAppBundleAction_Dependencies(t *testing.T) {
	action := &AppBundleAction{}
	deps := action.Dependencies()

	// BaseAction returns empty deps by default
	if len(deps.InstallTime) != 0 || len(deps.Runtime) != 0 {
		t.Error("Dependencies() should return empty ActionDeps")
	}
}

func TestDetectArchiveFormatFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/app.zip", "zip"},
		{"https://example.com/app.ZIP", "zip"},
		{"https://example.com/app.dmg", "dmg"},
		{"https://example.com/app.DMG", "dmg"},
		{"https://example.com/app.tar.gz", "tar.gz"},
		{"https://example.com/app.tgz", "tar.gz"},
		{"https://example.com/app.bin", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := detectArchiveFormatFromURL(tt.url)
			if got != tt.want {
				t.Errorf("detectArchiveFormatFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractDMG_NonMacOS(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("test only runs on non-macOS")
	}

	// On non-macOS, extractDMG should fail because hdiutil is not available
	err := extractDMG("/nonexistent.dmg", "/tmp/dest")
	if err == nil {
		t.Error("extractDMG should fail on non-macOS")
	}
}

func TestExtractDMG_NonExistentFile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("test only runs on macOS")
	}

	// Even on macOS, should fail for non-existent file
	err := extractDMG("/nonexistent/path/to/file.dmg", t.TempDir())
	if err == nil {
		t.Error("extractDMG should fail for non-existent file")
	}
}
