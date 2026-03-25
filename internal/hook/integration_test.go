package hook_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/hook"
)

// repoRoot walks up from the test file location until go.mod is found.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// debianImage reads the debian image reference from container-images.json at the repo root.
func debianImage(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "container-images.json"))
	if err != nil {
		t.Fatalf("read container-images.json: %v", err)
	}
	var m map[string]struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse container-images.json: %v", err)
	}
	img, ok := m["debian"]
	if !ok || img.Image == "" {
		t.Fatal("container-images.json missing debian image")
	}
	return img.Image
}

// skipIfNoDocker skips the test if docker is not available.
func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

// runBashInContainer runs the given bash script inside a debian container with
// the repo mounted read-only at /repo and TSUKU_HOME set to /tmp/tsuku-test.
// Returns combined stdout+stderr output.
func runBashInContainer(t *testing.T, script string) (string, error) {
	t.Helper()
	root := repoRoot(t)
	image := debianImage(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", root+":/repo:ro",
		"-e", "TSUKU_HOME=/tmp/tsuku-test",
		image,
		"bash", "-c", script,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestHookBash_NoPreExistingHandler verifies that tsuku.bash installs a
// command_not_found_handle that calls tsuku suggest when there is no
// pre-existing handler (scenario-21).
func TestHookBash_NoPreExistingHandler(t *testing.T) {
	skipIfNoDocker(t)

	script := `set -e
mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks
cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku
chmod +x /tmp/bin/tsuku
cp /repo/internal/hooks/tsuku.bash /tmp/tsuku-test/share/hooks/
export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
source /tmp/tsuku-test/share/hooks/tsuku.bash
jq 2>&1 || true`

	out, _ := runBashInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
}

// TestHookBash_WrapsExistingHandler verifies that tsuku.bash wraps a
// pre-existing command_not_found_handle without discarding it (scenario-22).
func TestHookBash_WrapsExistingHandler(t *testing.T) {
	skipIfNoDocker(t)

	script := `set -e
mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks
cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku
chmod +x /tmp/bin/tsuku
cp /repo/internal/hooks/tsuku.bash /tmp/tsuku-test/share/hooks/
export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
command_not_found_handle() { echo "original-handler-called"; }
source /tmp/tsuku-test/share/hooks/tsuku.bash
jq 2>&1 || true`

	out, _ := runBashInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
	if !strings.Contains(out, "original-handler-called") {
		t.Errorf("expected output to contain \"original-handler-called\"; got:\n%s", out)
	}
}

// TestHookBash_RecursionGuard verifies that tsuku.bash's inner command -v tsuku
// guard prevents any call to tsuku suggest when tsuku is not in PATH (scenario-23).
func TestHookBash_RecursionGuard(t *testing.T) {
	skipIfNoDocker(t)

	script := `mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks
cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku
chmod +x /tmp/bin/tsuku
cp /repo/internal/hooks/tsuku.bash /tmp/tsuku-test/share/hooks/
export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
source /tmp/tsuku-test/share/hooks/tsuku.bash
export PATH="/usr/bin:/bin"
jq 2>&1 || true`

	out, _ := runBashInContainer(t, script)
	if strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("recursion guard should prevent tsuku suggest call; got:\n%s", out)
	}
}

// TestHookBash_DoubleSource verifies that sourcing tsuku.bash twice results in
// the suggest output appearing exactly once per invocation (scenario-24).
func TestHookBash_DoubleSource(t *testing.T) {
	skipIfNoDocker(t)

	script := `set -e
mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks
cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku
chmod +x /tmp/bin/tsuku
cp /repo/internal/hooks/tsuku.bash /tmp/tsuku-test/share/hooks/
export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
source /tmp/tsuku-test/share/hooks/tsuku.bash
source /tmp/tsuku-test/share/hooks/tsuku.bash
jq 2>&1 || true`

	out, _ := runBashInContainer(t, script)
	count := strings.Count(out, "Command 'jq' not found.")
	if count != 1 {
		t.Errorf("expected \"Command 'jq' not found.\" exactly once; got %d times in:\n%s", count, out)
	}
}

// TestHookZsh verifies that tsuku.zsh installs a command_not_found_handler in
// a zsh session that calls tsuku suggest (scenario-25).
func TestHookZsh(t *testing.T) {
	skipIfNoDocker(t)

	root := repoRoot(t)
	image := debianImage(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	script := `apt-get update -q && apt-get install -y -q zsh && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.zsh /tmp/tsuku-test/share/hooks/ && ` +
		`export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" && ` +
		`zsh -c 'export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.zsh; ` +
		`jq' 2>&1 || true`

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", root+":/repo:ro",
		"-e", "TSUKU_HOME=/tmp/tsuku-test",
		image,
		"bash", "-c", script,
	)
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", string(out))
	}
}

// TestHookFish verifies that tsuku.fish installs a fish_command_not_found
// handler that calls tsuku suggest (scenario-26).
func TestHookFish(t *testing.T) {
	skipIfNoDocker(t)

	root := repoRoot(t)
	image := debianImage(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	script := `apt-get update -q && apt-get install -y -q fish && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.fish /tmp/tsuku-test/share/hooks/ && ` +
		`fish -c 'set -x PATH /tmp/bin /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.fish; ` +
		`jq' 2>&1 || true`

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", root+":/repo:ro",
		"-e", "TSUKU_HOME=/tmp/tsuku-test",
		image,
		"bash", "-c", script,
	)
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", string(out))
	}
}

// TestHookBash_UninstallRestores verifies that after install and uninstall, the
// rc file is byte-for-byte identical to the original (scenario-29).
// This test runs at the Go level without Docker since it exercises the hook package directly.
func TestHookBash_UninstallRestores(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := t.TempDir()

	rcFile := filepath.Join(homeDir, ".bashrc")
	original := "# existing content\n"
	if err := os.WriteFile(rcFile, []byte(original), 0644); err != nil {
		t.Fatalf("write initial .bashrc: %v", err)
	}

	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("read .bashrc after install: %v", err)
	}
	if !strings.Contains(string(data), "# tsuku hook") {
		t.Fatalf("marker not present after install; content:\n%s", string(data))
	}

	if err := hook.Uninstall("bash", homeDir); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	after, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("read .bashrc after uninstall: %v", err)
	}
	if string(after) != original {
		t.Errorf(".bashrc not restored to original after uninstall\nwant: %q\ngot:  %q", original, string(after))
	}
}
