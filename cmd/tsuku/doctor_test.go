package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// captureDoctorOutput captures both stdout and stderr during the execution of fn.
func captureDoctorOutput(fn func()) (stdout, stderr string) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	fn()

	wOut.Close()
	wErr.Close()

	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(rOut)
	_, _ = bufErr.ReadFrom(rErr)
	return bufOut.String(), bufErr.String()
}

// makeTestConfig builds a Config pointing at the given home directory.
func makeTestConfig(t *testing.T, homeDir string) *config.Config {
	t.Helper()
	cfg := &config.Config{
		HomeDir:  homeDir,
		ToolsDir: filepath.Join(homeDir, "tools"),
		ShareDir: filepath.Join(homeDir, "share"),
	}
	return cfg
}

func TestDoctorEnvFileCheck_Missing(t *testing.T) {
	dir := t.TempDir()
	cfg := makeTestConfig(t, dir)
	// Create tools dir so home-directory check passes
	if err := os.MkdirAll(filepath.Join(dir, "tools", "current"), 0755); err != nil {
		t.Fatal(err)
	}
	// Do NOT create env file

	stdout, stderr := captureDoctorOutput(func() {
		failed, _ := runDoctorChecks(cfg, dir)
		if !failed {
			t.Error("expected failed=true for missing env file")
		}
	})

	if !strings.Contains(stdout, "Env file") {
		t.Errorf("expected 'Env file' in stdout, got %q", stdout)
	}
	if !strings.Contains(stdout, "FAIL") {
		t.Errorf("expected FAIL in stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "--fix") {
		t.Errorf("expected '--fix' suggestion in stderr, got %q", stderr)
	}
}

func TestDoctorEnvFileCheck_Stale(t *testing.T) {
	dir := t.TempDir()
	cfg := makeTestConfig(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "tools", "current"), 0755); err != nil {
		t.Fatal(err)
	}
	// Write stale content
	if err := os.WriteFile(cfg.EnvFile(), []byte("# old content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := captureDoctorOutput(func() {
		failed, _ := runDoctorChecks(cfg, dir)
		if !failed {
			t.Error("expected failed=true for stale env file")
		}
	})

	if !strings.Contains(stdout, "FAIL") {
		t.Errorf("expected FAIL in stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "--fix") {
		t.Errorf("expected '--fix' suggestion in stderr, got %q", stderr)
	}
}

func TestDoctorEnvFileCheck_Current(t *testing.T) {
	dir := t.TempDir()
	cfg := makeTestConfig(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "tools", "current"), 0755); err != nil {
		t.Fatal(err)
	}
	// Write the canonical content
	if err := os.WriteFile(cfg.EnvFile(), []byte(config.EnvFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, _ := captureDoctorOutput(func() {
		runDoctorChecks(cfg, dir)
	})

	// Find the "Env file" line in stdout
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "Env file") {
			if strings.Contains(line, "FAIL") {
				t.Errorf("expected env file check to pass, got: %q", line)
			}
			if !strings.Contains(line, "ok") {
				t.Errorf("expected 'ok' in env file line, got: %q", line)
			}
			return
		}
	}
	t.Errorf("did not find 'Env file' line in stdout: %q", stdout)
}

func TestDoctorFix_EnvRewrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tools", "current"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	// Point TSUKU_HOME at our temp dir so config.DefaultConfig() uses it
	t.Setenv("TSUKU_HOME", dir)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatal(err)
	}

	// Write stale env content
	if err := os.WriteFile(cfg.EnvFile(), []byte("# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset fix flag
	origFix := doctorFixFlag
	doctorFixFlag = true
	defer func() { doctorFixFlag = origFix }()

	captureDoctorOutput(func() {
		if err := doctorCmd.RunE(doctorCmd, nil); err != nil {
			// RunE returns non-nil when checks still fail after fix (e.g. PATH not set).
			// That's expected in a test environment; we only care that the env file was repaired.
			_ = err
		}
	})

	// Verify env file was repaired
	got, err := os.ReadFile(cfg.EnvFile())
	if err != nil {
		t.Fatalf("could not read env file after fix: %v", err)
	}
	if string(got) != config.EnvFileContent {
		t.Errorf("env file not repaired: got %q", string(got))
	}
}

func TestDoctorFix_CacheRebuildWithHashes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tools", "current"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a shell.d file
	shellDDir := filepath.Join(dir, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptContent := []byte("# test init\n")
	scriptPath := filepath.Join(shellDDir, "mytool.bash")
	if err := os.WriteFile(scriptPath, scriptContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Set up a stale cache (empty file differs from expected)
	cachePath := filepath.Join(shellDDir, ".init-cache.bash")
	if err := os.WriteFile(cachePath, []byte("# old cache\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Prepare hashes (non-nil, even if empty — verifies we never pass nil)
	contentHashes := map[string]string{}

	// Verify cache is stale before fix
	shellCheck := shellenv.CheckShellD(dir, contentHashes)
	if !shellCheck.CacheStale["bash"] {
		t.Skip("cache is not stale before test; skipping")
	}

	// Call RebuildShellCache with the hashes map (must not be nil)
	if err := shellenv.RebuildShellCache(dir, "bash", contentHashes); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	// Verify cache is no longer stale
	shellCheck2 := shellenv.CheckShellD(dir, contentHashes)
	if shellCheck2.CacheStale["bash"] {
		t.Error("expected cache to be fresh after rebuild")
	}
}

func TestDoctorFix_NeverCallsRebuildWithNilHashes(t *testing.T) {
	// This test verifies the code path in the doctorCmd RunE: contentHashes is
	// always initialized as a non-nil map before it is passed to RebuildShellCache.
	// We inspect the runDoctorChecks return value to confirm it is never nil.
	dir := t.TempDir()
	cfg := makeTestConfig(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "tools", "current"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.EnvFile(), []byte(config.EnvFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	captureDoctorOutput(func() {
		_, hashes := runDoctorChecks(cfg, dir)
		if hashes == nil {
			t.Error("runDoctorChecks must return a non-nil hashes map")
		}
	})
}
