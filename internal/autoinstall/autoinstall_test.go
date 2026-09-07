package autoinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/project"
)

// mockDeclarationResolver is a test double for ProjectDeclarationResolver,
// for the single-provider cases where the fixture would add nothing: the
// declared recipe is the only match, so index ranking and declaration cannot
// disagree and there is no narrowing to get wrong. Anything with more than one
// provider goes through internal/indexfixture instead -- see candidates_test.go.
//
// versions is keyed on the recipe name, which is what the real resolver keys
// on. Every case here uses a command and a recipe of the same name.
type mockDeclarationResolver struct {
	versions map[string]string // recipe -> declared version
	err      error
}

func (m *mockDeclarationResolver) DeclarationsFor(_ context.Context, matches []index.BinaryMatch) ([]project.ProjectDeclaration, error) {
	if m.err != nil {
		return nil, m.err
	}
	var set []project.ProjectDeclaration
	for _, match := range matches {
		version, ok := m.versions[match.Recipe]
		if !ok {
			continue
		}
		set = append(set, project.ProjectDeclaration{
			Recipe:     match.Recipe,
			Version:    version,
			ConfigKey:  match.Recipe,
			ConfigPath: declaredConfigPath,
		})
	}
	return set, nil
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
		// The file the configuration-permission gate guards, which is the
		// one userconfig.Load reads. DefaultConfig fills it in; a hand-built
		// Config has to as well, or the gate has no path to check and fires
		// rather than passing.
		ConfigFile: filepath.Join(tmpDir, "config.toml"),
	}
	// Create the current dir so binary path construction works.
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	r := NewRunner(cfg, stdout, stderr)
	// A terminal is attached unless a case says otherwise. Confirm mode is
	// what most of the cases below drive, and confirm mode with no terminal
	// now refuses instead of prompting -- which is the point of the check, and
	// would otherwise cut short every case that answers a prompt.
	r.IsTerminal = func() bool { return true }
	return r, stdout, stderr
}

func TestNewRunner(t *testing.T) {
	r, _, _ := newTestRunner(t)
	// NewRunner always returns a non-nil runner; verify fields are set.
	if r.cfg == nil {
		t.Fatal("Runner.cfg is nil")
	}
	if r.stdout == nil {
		t.Fatal("Runner.stdout is nil")
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

// The five spellings are the record's, fixed by the requirement that names
// them, so they are pinned rather than left to whoever next edits the switch.
func TestOriginString(t *testing.T) {
	tests := []struct {
		origin Origin
		want   string
	}{
		{OriginUnset, "unset"},
		{OriginDefault, "default"},
		{OriginFlag, "flag"},
		{OriginEnvironment, "environment"},
		{OriginConfig, "config"},
		{OriginProject, "project"},
	}
	for _, tt := range tests {
		if got := tt.origin.String(); got != tt.want {
			t.Errorf("Origin(%d).String() = %q, want %q", tt.origin, got, tt.want)
		}
	}
}

func TestRun_ModeSuggest(t *testing.T) {
	r, stdout, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	err := r.Run(context.Background(), "jq", nil, ModeSuggest, OriginFlag, nil)
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

	err := r.Run(context.Background(), "jq", []string{"."}, ModeConfirm, OriginFlag, nil)
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

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginFlag, nil)
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
	configPath := r.cfg.ConfigFile
	_ = os.WriteFile(configPath, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil)
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
	if !strings.Contains(string(data), `"mode":"auto"`) {
		t.Errorf("audit log should record this install as auto, got %q", string(data))
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

	err := r.Run(context.Background(), "jq", nil, ModeSuggest, OriginFlag, nil)
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
	configPath := r.cfg.ConfigFile
	_ = os.WriteFile(configPath, []byte(""), 0644)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), gateConfigPermissions) {
		t.Errorf("stderr should name the %s gate, got %q", gateConfigPermissions, stderr.String())
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
	configPath := r.cfg.ConfigFile
	_ = os.WriteFile(configPath, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have fallen back to confirm (prompted and got "y").
	if !installer.called {
		t.Error("installer should have been called")
	}
}

func TestRun_ConflictGateFallback(t *testing.T) {
	// Multi-provider case, so it comes from the shared fixture rather than a
	// hand-written match slice: the pair this test used to name has to keep
	// existing in the published registry for the test to mean anything, and
	// nothing guarantees that.
	fx := indexfixture.New(t)

	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = fx.Lookup
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }
	r.ConsentReader = strings.NewReader("y\n")

	configPath := r.cfg.ConfigFile
	_ = os.WriteFile(configPath, []byte(""), 0600)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have fallen back to confirm due to multiple matches.
	if installer.recipe != indexfixture.RecipeDupFirst {
		t.Errorf("should install first match %q, got %q", indexfixture.RecipeDupFirst, installer.recipe)
	}
}

func TestRun_InstallFailure(t *testing.T) {
	r, _, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = &mockInstaller{err: errors.New("download failed")}
	r.ConsentReader = strings.NewReader("y\n")

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginFlag, nil)
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

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginFlag, nil)
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

	err := r.Run(context.Background(), "jq", []string{"."}, ModeConfirm, OriginFlag, nil)
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

func TestRun_InstalledTool_ProjectPinOverridesGlobalVersion(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	// Set ToolsDir so ToolBinDir resolves to an absolute path in the temp dir.
	r.cfg.ToolsDir = filepath.Join(r.cfg.HomeDir, "tools")

	// Tool is installed (some version), so it would normally fast-path
	// to tools/current/jq. But the resolver pins a specific version.
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq", Installed: true}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec

	// Create the pinned version's bin directory so the stat check passes.
	pinnedBinDir := r.cfg.ToolBinDir("jq", "1.6")
	if err := os.MkdirAll(pinnedBinDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a dummy binary so the exec path resolves.
	if err := os.WriteFile(filepath.Join(pinnedBinDir, "jq"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	resolver := &mockDeclarationResolver{
		versions: map[string]string{"jq": "1.6"},
	}

	err := r.Run(context.Background(), "jq", []string{"."}, ModeConfirm, OriginFlag, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !execRec.called {
		t.Fatal("exec should have been called")
	}

	// Should exec from the pinned version's bin, NOT from tools/current/.
	expectedBinary := filepath.Join(pinnedBinDir, "jq")
	if execRec.binary != expectedBinary {
		t.Errorf("exec binary = %q, want %q (should use pinned version, not tools/current/)", execRec.binary, expectedBinary)
	}

	// Should NOT have called the installer (pinned version already exists).
	if installer.called {
		t.Error("installer should not be called when pinned version is already installed")
	}
}

func TestRun_InstalledTool_ProjectPinInstallsIfMissing(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	// Set ToolsDir so ToolBinDir resolves to an absolute path in the temp dir.
	r.cfg.ToolsDir = filepath.Join(r.cfg.HomeDir, "tools")

	// Tool is installed (some version), but the pinned version is NOT installed.
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq", Installed: true}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }

	// Ensure config.toml exists with safe permissions for auto mode.
	_ = os.WriteFile(r.cfg.ConfigFile, []byte(""), 0600)

	// Resolver pins version 1.6. Its bin dir does NOT exist in the temp dir.
	resolver := &mockDeclarationResolver{
		versions: map[string]string{"jq": "1.6"},
	}

	// The origin is load-bearing here and is not filler: no ConsentReader is
	// wired, so the install only happens because the declaration raises the
	// unset default. An explicit origin makes this case fail at the prompt.
	err := r.Run(context.Background(), "jq", []string{"."}, ModeConfirm, OriginDefault, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called the installer with the pinned version.
	if !installer.called {
		t.Fatal("installer should be called to install the pinned version")
	}
	if installer.ver != "1.6" {
		t.Errorf("installer version = %q, want %q", installer.ver, "1.6")
	}
}

func TestRun_DeclaredVersionFlowsThrough(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.ConsentReader = strings.NewReader("y\n")

	resolver := &mockDeclarationResolver{
		versions: map[string]string{"jq": "1.7.1"},
	}

	err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginFlag, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if installer.ver != "1.7.1" {
		t.Errorf("installer got version %q, want %q", installer.ver, "1.7.1")
	}
}

func TestRun_ModeAuto_AuditLogNDJSON(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }

	_ = os.WriteFile(r.cfg.ConfigFile, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(r.cfg.HomeDir, "audit.log"))
	if readErr != nil {
		t.Fatalf("audit log not written: %v", readErr)
	}

	var entry struct {
		Timestamp string `json:"ts"`
		Action    string `json:"action"`
		Recipe    string `json:"recipe"`
		Version   string `json:"version"`
		Mode      string `json:"mode"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("audit log is not valid NDJSON: %v\nraw: %s", err, data)
	}
	// The action names the event and the mode field below names the consent.
	// It said "auto-install" while the entry was written only on the auto
	// path; now that confirm installs are recorded too, an action carrying the
	// mode would be the mode field written twice.
	if entry.Action != "install" {
		t.Errorf("action = %q, want %q", entry.Action, "install")
	}
	if entry.Recipe != "jq" {
		t.Errorf("recipe = %q, want %q", entry.Recipe, "jq")
	}
	if entry.Mode != "auto" {
		t.Errorf("mode = %q, want %q", entry.Mode, "auto")
	}
	if _, parseErr := time.Parse(time.RFC3339, entry.Timestamp); parseErr != nil {
		t.Errorf("timestamp %q is not RFC-3339: %v", entry.Timestamp, parseErr)
	}
}

func TestRun_NilRecipeHasVerification_FallsBackToConfirm(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = nil // not wired
	r.ConsentReader = strings.NewReader("y\n")

	_ = os.WriteFile(r.cfg.ConfigFile, []byte(""), 0600)

	err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have fallen back to confirm (prompted for consent).
	if !installer.called {
		t.Error("installer should have been called after consent")
	}
}

// --- Project elevation tests, over the single-provider mock ---
//
// The bounded elevation's own criteria run against the fixture, in
// elevation_test.go and candidates_test.go, where a declared recipe and the
// index's preference can disagree. These are the older mock-based cases and
// stay as the check that the elevation is reached at all through the plain
// single-provider path.

func TestRun_DeclaredCommand_RaisesTheUnsetDefault(t *testing.T) {
	r, _, _ := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return true }

	// Good config permissions so security gate 2 passes.
	_ = os.WriteFile(r.cfg.ConfigFile, []byte(""), 0600)

	resolver := &mockDeclarationResolver{
		versions: map[string]string{"jq": "1.7.1"},
	}

	// The unset default: confirm, from an origin of default, which is the one
	// mode a declaration raises. No ConsentReader is set, so if it falls
	// through to confirm it will fail.
	err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginDefault, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !installer.called {
		t.Error("installer should have been called")
	}
	if installer.ver != "1.7.1" {
		t.Errorf("installer version = %q, want %q", installer.ver, "1.7.1")
	}
}

func TestRun_UndeclaredCommand_ModeUnchanged(t *testing.T) {
	r, stdout, _ := newTestRunner(t)

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	// Nothing declared -- mode should stay as suggest.
	resolver := &mockDeclarationResolver{
		versions: map[string]string{}, // no entries
	}

	err := r.Run(context.Background(), "jq", nil, ModeSuggest, OriginFlag, resolver)
	if !errors.Is(err, ErrSuggestOnly) {
		t.Fatalf("expected ErrSuggestOnly, got %v", err)
	}
	if !strings.Contains(stdout.String(), "tsuku install jq") {
		t.Errorf("stdout should contain install instruction, got %q", stdout.String())
	}
}

func TestRun_NilResolver_ModeUnchanged(t *testing.T) {
	r, stdout, _ := newTestRunner(t)

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	err := r.Run(context.Background(), "jq", nil, ModeSuggest, OriginFlag, nil)
	if !errors.Is(err, ErrSuggestOnly) {
		t.Fatalf("expected ErrSuggestOnly, got %v", err)
	}
	if !strings.Contains(stdout.String(), "tsuku install jq") {
		t.Errorf("stdout should contain install instruction, got %q", stdout.String())
	}
}
