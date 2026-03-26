package hooks_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/containerimages"
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

// skipIfNoDocker skips the test if docker is not available.
func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

// runInContainer runs the given script inside a debian container with the repo
// mounted read-only at /repo and TSUKU_HOME set to /tmp/tsuku-test.
// The script is run with bash -c. Calls t.Fatalf if the container exits non-zero
// (infrastructure failure: image not found, mount error, etc.).
func runInContainer(t *testing.T, script string) string {
	t.Helper()
	root := repoRoot(t)
	image := containerimages.DefaultImage()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", root+":/repo:ro",
		"-e", "TSUKU_HOME=/tmp/tsuku-test",
		image,
		"bash", "-c", script,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("container run failed: %v\n%s", err, out)
	}
	return string(out)
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

	out := runInContainer(t, script)
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

	out := runInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
	if !strings.Contains(out, "original-handler-called") {
		t.Errorf("expected output to contain \"original-handler-called\"; got:\n%s", out)
	}
}

// TestHookBash_RecursionGuard verifies that tsuku.bash's inner command -v tsuku
// guard prevents any call to tsuku suggest when tsuku is not in PATH (scenario-23).
// set -e is intentionally omitted: jq exits 127 and we rely on || true for the
// script to exit 0; set -e would abort before || true fires in some bash versions.
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

	out := runInContainer(t, script)
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

	out := runInContainer(t, script)
	count := strings.Count(out, "Command 'jq' not found.")
	if count != 1 {
		t.Errorf("expected \"Command 'jq' not found.\" exactly once; got %d times in:\n%s", count, out)
	}
}

// TestHookZsh verifies that tsuku.zsh installs a command_not_found_handler in
// a zsh session that calls tsuku suggest (scenario-25).
func TestHookZsh(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q zsh && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.zsh /tmp/tsuku-test/share/hooks/ && ` +
		`export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" && ` +
		`zsh -c 'export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.zsh; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
}

// TestHookZsh_DetectAndWrap verifies that tsuku.zsh wraps a pre-existing
// command_not_found_handler without discarding it (scenario-26z).
func TestHookZsh_DetectAndWrap(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q zsh && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.zsh /tmp/tsuku-test/share/hooks/ && ` +
		`export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" && ` +
		`zsh -c 'export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; ` +
		`command_not_found_handler() { echo "original-handler-called"; }; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.zsh; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
	if !strings.Contains(out, "original-handler-called") {
		t.Errorf("expected output to contain \"original-handler-called\"; got:\n%s", out)
	}
}

// TestHookZsh_RecursionGuard verifies that tsuku.zsh's inner command -v tsuku
// guard prevents any call to tsuku suggest when tsuku is not in PATH (scenario-27z).
func TestHookZsh_RecursionGuard(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q zsh && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.zsh /tmp/tsuku-test/share/hooks/ && ` +
		`export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" && ` +
		`zsh -c 'export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.zsh; ` +
		`export PATH="/usr/bin:/bin"; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	if strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("recursion guard should prevent tsuku suggest call; got:\n%s", out)
	}
}

// TestHookZsh_DoubleSource verifies that sourcing tsuku.zsh twice results in
// the suggest output appearing exactly once per invocation (scenario-28z).
func TestHookZsh_DoubleSource(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q zsh && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.zsh /tmp/tsuku-test/share/hooks/ && ` +
		`export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" && ` +
		`zsh -c 'export PATH="/tmp/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.zsh; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.zsh; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	count := strings.Count(out, "Command 'jq' not found.")
	if count != 1 {
		t.Errorf("expected \"Command 'jq' not found.\" exactly once; got %d times in:\n%s", count, out)
	}
}

// TestHookFish verifies that tsuku.fish installs a fish_command_not_found
// handler that calls tsuku suggest (scenario-26).
func TestHookFish(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q fish && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.fish /tmp/tsuku-test/share/hooks/ && ` +
		`fish -c 'set -x PATH /tmp/bin /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.fish; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
}

// TestHookFish_DetectAndWrap verifies that tsuku.fish wraps a pre-existing
// fish_command_not_found handler without discarding it (scenario-26f).
func TestHookFish_DetectAndWrap(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q fish && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.fish /tmp/tsuku-test/share/hooks/ && ` +
		`fish -c 'set -x PATH /tmp/bin /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; ` +
		`function fish_command_not_found; echo "original-handler-called"; end; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.fish; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	if !strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("expected output to contain \"Command 'jq' not found.\"; got:\n%s", out)
	}
	if !strings.Contains(out, "original-handler-called") {
		t.Errorf("expected output to contain \"original-handler-called\"; got:\n%s", out)
	}
}

// TestHookFish_RecursionGuard verifies that tsuku.fish's inner command -q tsuku
// guard prevents any call to tsuku suggest when tsuku is not in PATH (scenario-27f).
func TestHookFish_RecursionGuard(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q fish && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.fish /tmp/tsuku-test/share/hooks/ && ` +
		`fish -c 'set -x PATH /tmp/bin /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.fish; ` +
		`set -x PATH /usr/bin /bin; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	if strings.Contains(out, "Command 'jq' not found.") {
		t.Errorf("recursion guard should prevent tsuku suggest call; got:\n%s", out)
	}
}

// TestHookFish_DoubleSource verifies that sourcing tsuku.fish twice results in
// the suggest output appearing exactly once per invocation (scenario-28f).
func TestHookFish_DoubleSource(t *testing.T) {
	skipIfNoDocker(t)

	script := `apt-get update -q && apt-get install -y -q fish && ` +
		`mkdir -p /tmp/bin /tmp/tsuku-test/share/hooks && ` +
		`cp /repo/internal/hooks/testdata/mock_tsuku /tmp/bin/tsuku && ` +
		`chmod +x /tmp/bin/tsuku && ` +
		`cp /repo/internal/hooks/tsuku.fish /tmp/tsuku-test/share/hooks/ && ` +
		`fish -c 'set -x PATH /tmp/bin /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.fish; ` +
		`source /tmp/tsuku-test/share/hooks/tsuku.fish; ` +
		`jq' 2>&1 || true`

	out := runInContainer(t, script)
	count := strings.Count(out, "Command 'jq' not found.")
	if count != 1 {
		t.Errorf("expected \"Command 'jq' not found.\" exactly once; got %d times in:\n%s", count, out)
	}
}
