package shellenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
)

// setupProject creates a temp directory with a .tsuku.toml and optionally
// creates tool bin directories under a fake $TSUKU_HOME.
func setupProject(t *testing.T, tomlContent string, installedTools map[string]string) (projectDir string, cfg *config.Config) {
	t.Helper()

	projectDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".tsuku.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	tsukuHome := t.TempDir()
	cfg = &config.Config{
		HomeDir:  tsukuHome,
		ToolsDir: filepath.Join(tsukuHome, "tools"),
	}

	for name, version := range installedTools {
		binDir := cfg.ToolBinDir(name, version)
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	return projectDir, cfg
}

func TestComputeActivation_ProjectFound(t *testing.T) {
	toml := `
[tools]
go = "1.22"
node = "20.16.0"
`
	projectDir, cfg := setupProject(t, toml, map[string]string{
		"go":   "1.22",
		"node": "20.16.0",
	})

	t.Setenv("PATH", "/usr/bin:/bin")
	// Prevent LoadProjectConfig from stopping at $HOME ceiling.
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	if !result.Active {
		t.Error("expected Active=true")
	}
	if result.Dir != projectDir {
		t.Errorf("Dir = %q, want %q", result.Dir, projectDir)
	}
	if result.PrevPath != "/usr/bin:/bin" {
		t.Errorf("PrevPath = %q, want %q", result.PrevPath, "/usr/bin:/bin")
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped = %v, want empty", result.Skipped)
	}

	// PATH should contain both tool bin dirs prepended to the original.
	goBin := cfg.ToolBinDir("go", "1.22")
	nodeBin := cfg.ToolBinDir("node", "20.16.0")
	if !strings.Contains(result.PATH, goBin) {
		t.Errorf("PATH missing go bin dir %q", goBin)
	}
	if !strings.Contains(result.PATH, nodeBin) {
		t.Errorf("PATH missing node bin dir %q", nodeBin)
	}
	if !strings.HasSuffix(result.PATH, ":/usr/bin:/bin") {
		t.Errorf("PATH should end with original PATH, got %q", result.PATH)
	}
}

func TestComputeActivation_SameDirectory(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := setupProject(t, toml, map[string]string{"go": "1.22"})

	// When cwd == curDir, should return nil (no-op).
	result, err := ComputeActivation(projectDir, "/usr/bin", projectDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for same directory, got %+v", result)
	}
}

func TestComputeActivation_NoConfig(t *testing.T) {
	// Directory without .tsuku.toml.
	dir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		ToolsDir: filepath.Join(t.TempDir(), "tools"),
	}

	// Prevent walking up to find a real config.
	t.Setenv("HOME", dir)

	result, err := ComputeActivation(dir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil when no .tsuku.toml, got %+v", result)
	}
}

func TestComputeActivation_SkippedTools(t *testing.T) {
	toml := `
[tools]
go = "1.22"
node = "20.16.0"
python = "3.12"
`
	// Only install go; node and python are missing.
	projectDir, cfg := setupProject(t, toml, map[string]string{
		"go": "1.22",
	})

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}

	// node and python should be skipped (sorted).
	if len(result.Skipped) != 2 {
		t.Fatalf("Skipped = %v, want 2 entries", result.Skipped)
	}
	if result.Skipped[0] != "node" || result.Skipped[1] != "python" {
		t.Errorf("Skipped = %v, want [node python]", result.Skipped)
	}

	// PATH should still contain the installed go bin dir.
	goBin := cfg.ToolBinDir("go", "1.22")
	if !strings.Contains(result.PATH, goBin) {
		t.Errorf("PATH missing go bin dir %q", goBin)
	}
}

func TestComputeActivation_EmptyVersion(t *testing.T) {
	toml := `
[tools]
jq = ""
`
	projectDir, cfg := setupProject(t, toml, nil)

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	// jq with empty version should be skipped.
	if len(result.Skipped) != 1 || result.Skipped[0] != "jq" {
		t.Errorf("Skipped = %v, want [jq]", result.Skipped)
	}
}

func TestComputeActivation_UsesPrevPath(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := setupProject(t, toml, map[string]string{"go": "1.22"})

	t.Setenv("PATH", "/something/modified:/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	// When prevPath is provided, it should be used as the base instead of $PATH.
	result, err := ComputeActivation(projectDir, "/original/bin:/usr/bin", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	if result.PrevPath != "/original/bin:/usr/bin" {
		t.Errorf("PrevPath = %q, want %q", result.PrevPath, "/original/bin:/usr/bin")
	}
	if !strings.HasSuffix(result.PATH, ":/original/bin:/usr/bin") {
		t.Errorf("PATH should use prevPath as base, got %q", result.PATH)
	}
}

func TestFormatExports_Bash(t *testing.T) {
	result := &ActivationResult{
		PATH:     "/tools/go-1.22/bin:/usr/bin",
		Dir:      "/home/user/project",
		PrevPath: "/usr/bin",
		Active:   true,
	}

	output := FormatExports(result, "bash")

	// Asserted on the variable's value after the shell has read the line, not
	// on the emitted literal. These three assertions previously compared
	// against the double-quoted form %q produced, which meant they pinned the
	// defect: a correct quoter made them fail. An assertion on an exact
	// emitted string is satisfied by whatever implementation generated the
	// expectation, so it cannot distinguish a safe quoter from an unsafe one.
	for _, tc := range []struct{ name, want string }{
		{"PATH", "/tools/go-1.22/bin:/usr/bin"},
		{"_TSUKU_DIR", "/home/user/project"},
		{"_TSUKU_PREV_PATH", "/usr/bin"},
	} {
		if got := evalAndRead(t, "bash", output, tc.name); got != tc.want {
			t.Errorf("%s = %q after eval, want %q\noutput:\n%s", tc.name, got, tc.want, output)
		}
	}
}

func TestFormatExports_Zsh(t *testing.T) {
	result := &ActivationResult{
		PATH:     "/tools/go-1.22/bin:/usr/bin",
		Dir:      "/home/user/project",
		PrevPath: "/usr/bin",
		Active:   true,
	}

	output := FormatExports(result, "zsh")

	// zsh uses the same syntax as bash.
	if !strings.Contains(output, "export PATH=") {
		t.Errorf("missing export keyword in:\n%s", output)
	}
}

func TestFormatExports_Fish(t *testing.T) {
	result := &ActivationResult{
		PATH:     "/tools/go-1.22/bin:/usr/bin",
		Dir:      "/home/user/project",
		PrevPath: "/usr/bin",
		Active:   true,
	}

	output := FormatExports(result, "fish")

	// Read back, not just substring-matched. A `set -gx PATH` line can be
	// present and still hand fish something other than these two entries, and
	// the substring checks below cannot tell the difference -- which is why
	// this test could not have caught the read-back defect that the
	// deactivation case did.
	for _, tc := range []struct{ name, want string }{
		{"PATH", "/tools/go-1.22/bin:/usr/bin"},
		{"_TSUKU_DIR", "/home/user/project"},
		{"_TSUKU_PREV_PATH", "/usr/bin"},
	} {
		if got := evalAndRead(t, "fish", output, tc.name); got != tc.want {
			t.Errorf("%s = %q after eval, want %q", tc.name, got, tc.want)
		}
	}

	if !strings.Contains(output, "set -gx PATH") {
		t.Errorf("missing fish PATH in:\n%s", output)
	}
	if !strings.Contains(output, "set -gx _TSUKU_DIR") {
		t.Errorf("missing fish _TSUKU_DIR in:\n%s", output)
	}
	if !strings.Contains(output, "set -gx _TSUKU_PREV_PATH") {
		t.Errorf("missing fish _TSUKU_PREV_PATH in:\n%s", output)
	}
	// Should not contain "export".
	if strings.Contains(output, "export") {
		t.Errorf("fish output should not contain 'export':\n%s", output)
	}
}

func TestFormatExports_Nil(t *testing.T) {
	output := FormatExports(nil, "bash")
	if output != "" {
		t.Errorf("expected empty output for nil result, got %q", output)
	}
}

func TestComputeActivation_Deactivation(t *testing.T) {
	// Directory without .tsuku.toml, but prevPath is set (was in a project).
	dir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		ToolsDir: filepath.Join(t.TempDir(), "tools"),
	}

	t.Setenv("HOME", dir)

	result, err := ComputeActivation(dir, "/original/bin:/usr/bin", "/some/project", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected deactivation result, got nil")
	}
	if result.Active {
		t.Error("expected Active=false for deactivation")
	}
	if result.PATH != "/original/bin:/usr/bin" {
		t.Errorf("PATH = %q, want %q", result.PATH, "/original/bin:/usr/bin")
	}
	if result.Dir != "" {
		t.Errorf("Dir = %q, want empty", result.Dir)
	}
	if result.PrevPath != "" {
		t.Errorf("PrevPath = %q, want empty (should be unset)", result.PrevPath)
	}
}

func TestComputeActivation_NoOpWithoutPriorActivation(t *testing.T) {
	// Directory without .tsuku.toml and no prevPath -- should be nil (no-op).
	dir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		ToolsDir: filepath.Join(t.TempDir(), "tools"),
	}

	t.Setenv("HOME", dir)

	result, err := ComputeActivation(dir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for no config and no prior activation, got %+v", result)
	}
}

func TestComputeActivation_ProjectToProjectSwitch(t *testing.T) {
	// Switching from project A to project B should preserve the original
	// prevPath (the PATH from before any activation).
	toml := `
[tools]
node = "20.16.0"
`
	projectB, cfg := setupProject(t, toml, map[string]string{
		"node": "20.16.0",
	})

	t.Setenv("PATH", "/project-a-modified:/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectB))

	// prevPath represents the original PATH before project A was activated.
	originalPath := "/original/bin:/usr/bin"
	result, err := ComputeActivation(projectB, originalPath, "/some/project-a", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	if !result.Active {
		t.Error("expected Active=true for project switch")
	}
	// PrevPath should be the original PATH, not project A's modified PATH.
	if result.PrevPath != originalPath {
		t.Errorf("PrevPath = %q, want %q (original, not project A's PATH)", result.PrevPath, originalPath)
	}
	// PATH should be built on the original PATH base, not the current $PATH.
	if !strings.HasSuffix(result.PATH, ":"+originalPath) {
		t.Errorf("PATH should end with original PATH, got %q", result.PATH)
	}
	// Dir should be project B.
	if result.Dir != projectB {
		t.Errorf("Dir = %q, want %q", result.Dir, projectB)
	}
}

func TestFormatExports_DeactivationBash(t *testing.T) {
	result := &ActivationResult{
		PATH:   "/original/bin:/usr/bin",
		Active: false,
	}

	output := FormatExports(result, "bash")

	if got := evalAndRead(t, "bash", output, "PATH"); got != "/original/bin:/usr/bin" {
		t.Errorf("PATH = %q after eval, want %q", got, "/original/bin:/usr/bin")
	}
	if !strings.Contains(output, `export PATH=`) {
		t.Errorf("missing PATH export in:\n%s", output)
	}
	if !strings.Contains(output, "unset _TSUKU_DIR _TSUKU_PREV_PATH") {
		t.Errorf("missing unset in:\n%s", output)
	}
	// Should NOT contain _TSUKU_DIR export.
	if strings.Contains(output, "export _TSUKU_DIR") {
		t.Errorf("deactivation should not export _TSUKU_DIR:\n%s", output)
	}
}

func TestFormatExports_DeactivationFish(t *testing.T) {
	result := &ActivationResult{
		PATH:   "/original/bin:/usr/bin",
		Active: false,
	}

	output := FormatExports(result, "fish")

	if got := evalAndRead(t, "fish", output, "PATH"); got != "/original/bin:/usr/bin" {
		t.Errorf("PATH = %q after eval, want %q", got, "/original/bin:/usr/bin")
	}
	if !strings.Contains(output, `set -gx PATH `) {
		t.Errorf("missing PATH set in:\n%s", output)
	}
	if !strings.Contains(output, "set -e _TSUKU_DIR") {
		t.Errorf("missing set -e _TSUKU_DIR in:\n%s", output)
	}
	if !strings.Contains(output, "set -e _TSUKU_PREV_PATH") {
		t.Errorf("missing set -e _TSUKU_PREV_PATH in:\n%s", output)
	}
	// Should not contain set -gx for tracking vars.
	if strings.Contains(output, "set -gx _TSUKU_DIR") {
		t.Errorf("deactivation should not set -gx _TSUKU_DIR:\n%s", output)
	}
}

// evalAndRead evaluates emitted output in a real shell and returns what the
// named variable ends up holding.
//
// The fish branch reads the variable back out of the *exported environment*
// rather than expanding it, and the reason is a false alarm this helper raised
// once already. Fish treats PATH as a list, splitting it on colons when it is
// set: `printf '%s' $PATH` then prints the elements with nothing between them,
// so a correctly emitted `set -gx PATH '/a:/b'` reads back as `/a/b` and looks
// like a lost separator. It is not one -- a child process receives
// `PATH=/a:/b` -- and the environment is both what actually matters to a
// consumer and immune to the list semantics. Do not "simplify" this back to an
// expansion.
//
// A skip when the shell is absent is fine for bash, which is everywhere. Fish
// is provisioned in CI precisely so its cases do not skip on every run and read
// as coverage, so TSUKU_REQUIRE_FISH turns a missing fish into a failure there
// -- the same guard internal/shellquote applies, for the same reason.
func evalAndRead(t *testing.T, shell, output, varName string) string {
	t.Helper()
	bin, err := exec.LookPath(shell)
	if err != nil {
		if shell == "fish" && os.Getenv("TSUKU_REQUIRE_FISH") != "" {
			t.Fatal("fish is required here (TSUKU_REQUIRE_FISH is set) but was not found. " +
				"CI provisions it; a skip on this surface would be indistinguishable from a pass.")
		}
		t.Skipf("%s not available", shell)
	}
	// Only fish's *path* variables are lists, and only they need the env
	// read-back. Using env for everything would be wrong in the other
	// direction: env output is line-oriented, so a value containing a newline
	// -- which is a fixture here, because it is a value %q corrupts -- would
	// come back truncated at the newline and look like a quoting failure.
	fishPathVar := shell == "fish" && (varName == "PATH" || varName == "CDPATH" || varName == "MANPATH")

	var script string
	switch {
	case fishPathVar:
		// `string join` is a builtin, which matters more than it looks: these
		// tests deliberately set PATH to hostile values, and an external
		// command -- `env`, the obvious choice -- cannot be found once PATH
		// points somewhere meaningless. It also writes straight to stdout
		// rather than through a command substitution, so a newline inside a
		// value survives instead of being split on.
		//
		// No `--` before the separator: fish rejects `string join -- : $PATH`
		// outright. The cost is that a value beginning with `-` would be read
		// as an option and fail here -- loudly, as a test error, not as a
		// silent wrong answer -- and no fixture has that shape.
		//
		// The trailing `true` is load-bearing: `string join` exits 1 when it
		// had nothing to join, which is the single-element case -- so a
		// perfectly correct read-back of a one-entry PATH would otherwise be
		// reported as fish rejecting the script. Measured, not guessed; the
		// first probe of this piped to `od` and never saw the status.
		script = output + "\nstring join : $" + varName + "\ntrue\n"
	case shell == "fish":
		script = output + "\nprintf '%s' $" + varName + "\n"
	default:
		script = output + "\nprintf '%s' \"$" + varName + "\"\n"
	}
	out, err := exec.Command(bin, "-c", script).Output()
	if err != nil {
		t.Fatalf("%s rejected the emitted output: %v\nscript:\n%s", shell, err, script)
	}
	if !fishPathVar {
		return string(out)
	}
	// `string join` terminates its output with exactly one newline. Trim that
	// one and nothing else, so a value that itself contains a newline -- which
	// is one of the fixtures -- comes back whole.
	return strings.TrimSuffix(string(out), "\n")
}

// TestFormatExports_HostileValuesDoNotExecute is the assertion the old tests
// could not make.
//
// It is also the rebase guard. A sibling change moves this file into a new
// package and rewrites the emitter around a shared exportLine helper; taking
// that side of the conflict compiles cleanly and silently reverts the quoting
// fix, because it still formats with %q. This test fails in that case --
// verified by mutation -- so the revert is loud rather than silent. If you are
// resolving that conflict: the correct result is this body plus their
// additions, not either side whole. Every value here is attacker-influenced in production: Dir is
// the directory holding the project config, named by whoever authored the
// cloned repository, and it reaches the emitted output with no validation and
// no existence check. The marker file is what separates "the string looks
// escaped" from "nothing ran".
func TestFormatExports_HostileValuesDoNotExecute(t *testing.T) {
	for _, shell := range []string{"bash", "fish"} {
		t.Run(shell, func(t *testing.T) {
			bin, err := exec.LookPath(shell)
			if err != nil {
				// Fail closed for fish under the CI gate. A skip on this surface
				// is indistinguishable from a pass, which is the whole reason the
				// variable exists; it was honored in evalAndRead and not here.
				if shell == "fish" && os.Getenv("TSUKU_REQUIRE_FISH") != "" {
					t.Fatal("fish is required here (TSUKU_REQUIRE_FISH is set) but was not found")
				}
				t.Skipf("%s not available", shell)
			}
			dir := t.TempDir()
			marker := filepath.Join(dir, "pwned")

			for _, hostile := range []string{
				"/tmp/proj/$(touch " + marker + ")",
				"/tmp/proj/`touch " + marker + "`",
				"/tmp/proj/$HOME",
				"/tmp/pro'j",
				"/tmp/pro\\\\j",
				// A newline is here because %q both corrupts it and, in doing so,
				// ends the emitted line early -- everything after it is read by the
				// shell as a fresh command. It is the fixture that fails loudest
				// against the defect and was missing from this loop.
				"/tmp/pro\nj",
				"/tmp/proj",
			} {
				for _, active := range []bool{true, false} {
					// Which variable carries the payload has to follow what the
					// branch actually emits. Deactivation emits PATH and nothing
					// else, so putting the value in Dir and PrevPath -- as this
					// loop used to -- meant every active=false iteration evaluated
					// a benign `export PATH='/usr/bin'` and proved nothing. Half
					// the runs were decoration.
					result := &ActivationResult{
						PATH: "/usr/bin", Dir: hostile, PrevPath: hostile, Active: active,
					}
					readBack := "_TSUKU_DIR"
					if !active {
						result.PATH = hostile
						readBack = "PATH"
					}

					output := FormatExports(result, shell)

					args := []string{"-c", output + "\ntrue\n"}
					if shell == "bash" {
						// The user's rc files are not part of what is under test,
						// and one of them exporting PATH would make a failure here
						// unreproducible on another machine.
						args = append([]string{"--norc", "--noprofile"}, args...)
					}
					if err := exec.Command(bin, args...).Run(); err != nil {
						t.Fatalf("%s rejected output for %q: %v\n%s", shell, hostile, err, output)
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatalf("value %q executed under %s (active=%v):\n%s",
							hostile, shell, active, output)
					}

					// Not executing is half the property. A quoter that dropped the
					// value, or mangled it into something else harmless, passes the
					// check above and is still wrong -- the variable has to come
					// back byte for byte.
					if got := evalAndRead(t, shell, output, readBack); got != hostile {
						t.Errorf("%s = %q after eval under %s (active=%v), want %q\n%s",
							readBack, got, shell, active, hostile, output)
					}
				}
			}
		})
	}
}
