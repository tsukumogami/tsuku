package shellenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckShellD_EmptyDirectory(t *testing.T) {
	tsukuHome := t.TempDir()

	result := CheckShellD(tsukuHome)

	if len(result.ActiveScripts) != 0 {
		t.Errorf("expected no active scripts, got %v", result.ActiveScripts)
	}
	if result.HasIssues() {
		t.Error("expected no issues for empty directory")
	}
}

func TestCheckShellD_ListsActiveScripts(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "starship.bash"), "# init\n")
	writeTestFile(t, filepath.Join(shellDDir, "starship.zsh"), "# init\n")
	writeTestFile(t, filepath.Join(shellDDir, "zoxide.bash"), "# init\n")

	result := CheckShellD(tsukuHome)

	bashScripts := result.ActiveScripts["bash"]
	if len(bashScripts) != 2 || bashScripts[0] != "starship" || bashScripts[1] != "zoxide" {
		t.Errorf("expected bash scripts [starship, zoxide], got %v", bashScripts)
	}

	zshScripts := result.ActiveScripts["zsh"]
	if len(zshScripts) != 1 || zshScripts[0] != "starship" {
		t.Errorf("expected zsh scripts [starship], got %v", zshScripts)
	}
}

func TestCheckShellD_DetectsSymlinks(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a real file and a symlink
	writeTestFile(t, filepath.Join(shellDDir, "real.bash"), "# ok\n")
	target := filepath.Join(t.TempDir(), "target.sh")
	writeTestFile(t, target, "# injected\n")
	if err := os.Symlink(target, filepath.Join(shellDDir, "evil.bash")); err != nil {
		t.Fatal(err)
	}

	result := CheckShellD(tsukuHome)

	if len(result.Symlinks) != 1 || result.Symlinks[0] != "evil.bash" {
		t.Errorf("expected symlink detected [evil.bash], got %v", result.Symlinks)
	}
	if !result.HasIssues() {
		t.Error("expected issues when symlinks detected")
	}
}

func TestCheckShellD_DetectsHashMismatch(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tampered\n")

	hashes := map[string]string{
		"share/shell.d/tool.bash": testHash("# original\n"),
	}

	result := CheckShellD(tsukuHome, ShellDSelection{Active: hashes})

	if len(result.HashMismatches) != 1 || result.HashMismatches[0] != "tool.bash" {
		t.Errorf("expected hash mismatch [tool.bash], got %v", result.HashMismatches)
	}
	if !result.HasIssues() {
		t.Error("expected issues when hash mismatches detected")
	}
}

func TestCheckShellD_DetectsSyntaxErrors(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a file with a syntax error
	writeTestFile(t, filepath.Join(shellDDir, "broken.bash"), "if then fi\n")

	result := CheckShellD(tsukuHome)

	if len(result.SyntaxErrors) != 1 {
		t.Errorf("expected 1 syntax error, got %d", len(result.SyntaxErrors))
	} else if result.SyntaxErrors[0].File != "broken.bash" {
		t.Errorf("expected syntax error for broken.bash, got %s", result.SyntaxErrors[0].File)
	}
	if !result.HasIssues() {
		t.Error("expected issues when syntax errors detected")
	}
}

func TestCheckShellD_CacheFreshness_Fresh(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# init\n")

	// Build the cache first
	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatal(err)
	}

	result := CheckShellD(tsukuHome)

	if result.CacheStale["bash"] {
		t.Error("expected cache to be fresh after rebuild")
	}
}

func TestCheckShellD_CacheFreshness_Stale(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# init\n")

	// Build cache, then modify the source file
	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# changed\n")

	result := CheckShellD(tsukuHome)

	if !result.CacheStale["bash"] {
		t.Error("expected cache to be stale after source file change")
	}
}

func TestCheckShellD_CacheFreshness_MissingCache(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# init\n")
	// Don't build cache

	result := CheckShellD(tsukuHome)

	if !result.CacheStale["bash"] {
		t.Error("expected cache to be stale when cache file is missing")
	}
}

func TestShellsFromPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{"no paths", nil, nil},
		{
			"bash only",
			[]string{"share/shell.d/starship@1.16.0.bash"},
			[]string{"bash"},
		},
		{
			"both shells, unsorted input",
			[]string{"share/shell.d/starship@1.16.0.zsh", "share/shell.d/starship@1.16.0.bash"},
			[]string{"bash", "zsh"},
		},
		{
			"legacy unversioned filenames still resolve",
			[]string{"share/shell.d/starship.bash"},
			[]string{"bash"},
		},
		{
			"a target that differs from the tool name still resolves",
			[]string{"share/shell.d/00-env-nvm@0.40.6.bash"},
			[]string{"bash"},
		},
		{
			"duplicates across versions collapse",
			[]string{"share/shell.d/nvm@0.40.5.bash", "share/shell.d/nvm@0.40.6.bash"},
			[]string{"bash"},
		},
		{
			"non-shell.d paths and unknown shells are ignored",
			[]string{"share/completions/nvm.bash", "share/shell.d/nvm@0.40.6.fish", "share/shell.d/noext"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShellsFromPaths(tt.paths)
			if len(got) != len(tt.want) {
				t.Fatalf("ShellsFromPaths(%v) = %v, want %v", tt.paths, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ShellsFromPaths(%v) = %v, want %v", tt.paths, got, tt.want)
				}
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		fileName string
		shell    string
		want     string
	}{
		{"nvm@0.40.6.bash", "bash", "nvm"},
		{"00-env-nvm@0.40.6.zsh", "zsh", "00-env-nvm"},
		{"starship.bash", "bash", "starship"},
		{"foo-1@2.bash", "bash", "foo-1"},
		{"foo@1-2.bash", "bash", "foo"},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			if got := DisplayName(tt.fileName, tt.shell); got != tt.want {
				t.Errorf("DisplayName(%q, %q) = %q, want %q", tt.fileName, tt.shell, got, tt.want)
			}
		})
	}
}
