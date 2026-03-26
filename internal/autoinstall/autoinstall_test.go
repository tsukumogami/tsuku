package autoinstall

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// mockProjectVersionResolver is a test double for ProjectVersionResolver.
type mockProjectVersionResolver struct {
	versions map[string]string // command -> version
	err      error
}

func (m *mockProjectVersionResolver) ProjectVersionFor(_ context.Context, command string) (string, bool, error) {
	if m.err != nil {
		return "", false, m.err
	}
	v, ok := m.versions[command]
	return v, ok, nil
}

// mockInstaller records install calls.
type mockInstaller struct {
	called bool
	recipe string
	ver    string
	err    error
}

func (m *mockInstaller) Install(_ context.Context, recipe, version string) error {
	m.called = true
	m.recipe = recipe
	m.ver = version
	return m.err
}

// execRecorder records exec calls instead of replacing the process.
type execRecorder struct {
	called bool
	binary string
	args   []string
}

func (e *execRecorder) exec(binary string, args []string, _ []string) error {
	e.called = true
	e.binary = binary
	e.args = args
	return nil
}

// newTestRunner creates a Runner with captured stdout/stderr for testing.
func newTestRunner(t *testing.T) (*Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		HomeDir:    tmpDir,
		CacheDir:   filepath.Join(tmpDir, "cache"),
		CurrentDir: filepath.Join(tmpDir, "tools", "current"),
	}
	// Create the current dir so binary path construction works.
	os.MkdirAll(cfg.CurrentDir, 0755)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	r := NewRunner(cfg, stdout, stderr)
	return r, stdout, stderr
}

func TestNewRunner(t *testing.T) {
	r, _, _ := newTestRunner(t)
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	if r.cfg == nil {
		t.Fatal("Runner.cfg is nil")
	}
}

func TestParseModeValid(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{"confirm", ModeConfirm},
		{"suggest", ModeSuggest},
		{"auto", ModeAuto},
	}
	for _, tt := range tests {
		m, ok := ParseMode(tt.input)
		if !ok {
			t.Errorf("ParseMode(%q) returned ok=false", tt.input)
		}
		if m != tt.want {
			t.Errorf("ParseMode(%q) = %v, want %v", tt.input, m, tt.want)
		}
	}
}

func TestParseModeInvalid(t *testing.T) {
	_, ok := ParseMode("invalid")
	if ok {
		t.Error("ParseMode(\"invalid\") returned ok=true")
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeConfirm, "confirm"},
		{ModeSuggest, "suggest"},
		{ModeAuto, "auto"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestMockProjectVersionResolver(t *testing.T) {
	resolver := &mockProjectVersionResolver{
		versions: map[string]string{"jq": "1.7.1"},
	}

	v, ok, err := resolver.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for pinned command")
	}
	if v != "1.7.1" {
		t.Errorf("got version %q, want %q", v, "1.7.1")
	}

	_, ok, err = resolver.ProjectVersionFor(context.Background(), "curl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for unpinned command")
	}
}

// --- Runner.Run tests ---

func TestRun_ModeSuggest(t *testing.T) {
	r, stdout, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	err := r.Run(context.Background(), "jq", nil, ModeSuggest, nil)
	if !errors.Is(err, ErrSuggestOnly) {
		t.Fatalf("expected ErrSuggestOnly, got %v", err)
	}
	if !strings.Contains(stdout.String(), "tsuku install jq") {
		t.Errorf("stdout should contain install instruction, got %q", stdout.String())
	}
}

func TestRun_ModeConfirm_Yes(t *testing.T) {
	r, stdout, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.ConsentReader = strings.NewReader("y\n")

	err := r.Run(context.Background(), "jq", []string{"."}, ModeConfirm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Install jq?") {
		t.Errorf("stdout should contain prompt, got %q", stdout.String())
	}
	if !installer.called {
		t.Error("installer should have been called")
	}
	if !execRec.called {
		t.Error("exec should have been called")
	}
}

func TestRun_ModeConfirm_No(t *testing.T) {
	r, _, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.ConsentReader = strings.NewReader("n\n")

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, nil)
	if !errors.Is(err, ErrUserDeclined) {
		t.Fatalf("expected ErrUserDeclined, got %v", err)
	}
}

func TestRun_ModeAuto_HappyPath(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }

	// Create a config file with 0600 so the permission check passes.
	configPath := filepath.Join(r.cfg.HomeDir, "config.toml")
	os.WriteFile(configPath, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !installer.called {
		t.Error("installer should have been called")
	}

	// Verify audit log was written.
	auditPath := filepath.Join(r.cfg.HomeDir, "audit.log")
	data, readErr := os.ReadFile(auditPath)
	if readErr != nil {
		t.Fatalf("audit log not written: %v", readErr)
	}
	if !strings.Contains(string(data), `"auto-install"`) {
		t.Errorf("audit log should contain auto-install entry, got %q", string(data))
	}
}

func TestRun_RootGuard(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must not run as root")
	}
	// Root guard only triggers when euid==0, which we can't fake in tests
	// without capabilities. This test verifies the non-root path doesn't
	// trigger the guard.
	r, _, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	err := r.Run(context.Background(), "jq", nil, ModeSuggest, nil)
	if errors.Is(err, ErrForbidden) {
		t.Fatal("root guard should not trigger for non-root user")
	}
}

func TestRun_ConfigPermissionFallback(t *testing.T) {
	r, _, stderr := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }
	// Give consent since mode falls back to confirm.
	r.ConsentReader = strings.NewReader("y\n")

	// Create config with permissive permissions (0644).
	configPath := filepath.Join(r.cfg.HomeDir, "config.toml")
	os.WriteFile(configPath, []byte(""), 0644)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "permissions are too open") {
		t.Errorf("stderr should warn about permissions, got %q", stderr.String())
	}
}

func TestRun_VerificationGateFallback(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return false } // no verification
	r.ConsentReader = strings.NewReader("y\n")

	// Config with correct permissions so gate 2 doesn't trigger.
	configPath := filepath.Join(r.cfg.HomeDir, "config.toml")
	os.WriteFile(configPath, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have fallen back to confirm (prompted and got "y").
	if !installer.called {
		t.Error("installer should have been called")
	}
}

func TestRun_ConflictGateFallback(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{
			{Recipe: "jq", Command: "jq"},
			{Recipe: "jq-alt", Command: "jq"},
		}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }
	r.ConsentReader = strings.NewReader("y\n")

	configPath := filepath.Join(r.cfg.HomeDir, "config.toml")
	os.WriteFile(configPath, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have fallen back to confirm due to multiple matches.
	if installer.recipe != "jq" {
		t.Errorf("should install first match, got %q", installer.recipe)
	}
}

func TestRun_InstallFailure(t *testing.T) {
	r, _, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = &mockInstaller{err: errors.New("download failed")}
	r.ConsentReader = strings.NewReader("y\n")

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, nil)
	if err == nil {
		t.Fatal("expected error for install failure")
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("error should contain cause, got %v", err)
	}
}

func TestRun_IndexNotBuilt(t *testing.T) {
	r, _, stderr := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return nil, index.ErrIndexNotBuilt
	}

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, nil)
	if !errors.Is(err, ErrIndexNotBuilt) {
		t.Fatalf("expected ErrIndexNotBuilt, got %v", err)
	}
	if !strings.Contains(stderr.String(), "update-registry") {
		t.Errorf("stderr should mention update-registry, got %q", stderr.String())
	}
}

func TestRun_AlreadyInstalled_ExecImmediately(t *testing.T) {
	r, _, _ := newTestRunner(t)
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq", Installed: true}}, nil
	}
	r.Exec = execRec.exec

	err := r.Run(context.Background(), "jq", []string{"."}, ModeConfirm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !execRec.called {
		t.Error("exec should have been called immediately")
	}
	expectedBinary := filepath.Join(r.cfg.CurrentDir, "jq")
	if execRec.binary != expectedBinary {
		t.Errorf("exec binary = %q, want %q", execRec.binary, expectedBinary)
	}
}
