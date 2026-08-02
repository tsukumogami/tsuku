package datamigration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateNvm(t *testing.T) {
	tests := []struct {
		name string
		// seed populates $TSUKU_HOME before the migration runs.
		seed func(t *testing.T, home string)
		// check asserts on the outcome.
		check func(t *testing.T, home string, result Result)
	}{
		{
			// The population that predates the shell.d fix: NVM_DIR was never exported,
			// so nvm self-located to where it was sourced from and installed there.
			name: "moves data stranded in share/shell.d",
			seed: func(t *testing.T, home string) {
				shellD := filepath.Join(home, "share", "shell.d")
				write(t, filepath.Join(shellD, "versions", "node", "v20.0.0", "bin", "node"), "node-binary")
				write(t, filepath.Join(shellD, "alias", "default"), "20")
				write(t, filepath.Join(shellD, "nvm@0.40.3.bash"), "# tsuku's own fragment")
			},
			check: func(t *testing.T, home string, result Result) {
				data := NvmDataDir(home)
				if got := read(t, filepath.Join(data, "versions", "node", "v20.0.0", "bin", "node")); got != "node-binary" {
					t.Errorf("Node binary = %q, want %q", got, "node-binary")
				}
				if got := read(t, filepath.Join(data, "alias", "default")); got != "20" {
					t.Errorf("default alias = %q, want %q", got, "20")
				}
				// tsuku's own fragments are files, and only directories move.
				fragment := filepath.Join(home, "share", "shell.d", "nvm@0.40.3.bash")
				if _, err := os.Stat(fragment); err != nil {
					t.Errorf("tsuku's shell fragment was moved out from under it: %v", err)
				}
			},
		},
		{
			// The population created once the export started working: data landed in the
			// versioned tool directory, which tsuku reclaims.
			name: "moves data out of a versioned tool directory",
			seed: func(t *testing.T, home string) {
				toolDir := filepath.Join(home, "tools", "nvm-0.40.3")
				write(t, filepath.Join(toolDir, "versions", "node", "v22.0.0", "bin", "node"), "node-binary")
				write(t, filepath.Join(toolDir, "alias", "default"), "22")
				write(t, filepath.Join(toolDir, "nvm.sh"), "# the nvm program")
			},
			check: func(t *testing.T, home string, result Result) {
				data := NvmDataDir(home)
				if got := read(t, filepath.Join(data, "versions", "node", "v22.0.0", "bin", "node")); got != "node-binary" {
					t.Errorf("Node binary = %q, want %q", got, "node-binary")
				}
				// The program half is not user data and must stay where it is; moving it
				// would empty the directory rollback depends on.
				program := filepath.Join(home, "tools", "nvm-0.40.3", "nvm.sh")
				if _, err := os.Stat(program); err != nil {
					t.Errorf("the nvm program itself was moved: %v", err)
				}
			},
		},
		{
			name: "merges both legacy locations into one root",
			seed: func(t *testing.T, home string) {
				write(t, filepath.Join(home, "share", "shell.d", "versions", "node", "v18.0.0", "bin", "node"), "old")
				write(t, filepath.Join(home, "tools", "nvm-0.40.3", "versions", "node", "v22.0.0", "bin", "node"), "new")
			},
			check: func(t *testing.T, home string, result Result) {
				data := NvmDataDir(home)
				for _, v := range []string{"v18.0.0", "v22.0.0"} {
					if _, err := os.Stat(filepath.Join(data, "versions", "node", v, "bin", "node")); err != nil {
						t.Errorf("%s did not survive the merge: %v", v, err)
					}
				}
			},
		},
		{
			name: "does nothing when there is no legacy data",
			seed: func(t *testing.T, home string) {
				write(t, filepath.Join(home, "tools", "nvm-0.40.3", "nvm.sh"), "# the nvm program")
			},
			check: func(t *testing.T, home string, result Result) {
				if result.Merged() {
					t.Errorf("moved %v, want nothing", result.Moved)
				}
			},
		},
		{
			// A directory named nvm-something that is not an nvm data root must not be
			// mistaken for one just because the glob matched it.
			name: "ignores a tool directory holding no nvm data",
			seed: func(t *testing.T, home string) {
				write(t, filepath.Join(home, "tools", "nvm-unrelated", "README"), "not nvm data")
			},
			check: func(t *testing.T, home string, result Result) {
				if result.Merged() {
					t.Errorf("moved %v, want nothing", result.Moved)
				}
				if got := read(t, filepath.Join(home, "tools", "nvm-unrelated", "README")); got != "not nvm data" {
					t.Errorf("unrelated directory was disturbed: %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tt.seed(t, home)

			result, err := MigrateNvm(home)
			if err != nil {
				t.Fatalf("MigrateNvm: %v", err)
			}
			tt.check(t, home, result)
		})
	}
}

func TestMigrateNvmIsIdempotent(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "tools", "nvm-0.40.3", "versions", "node", "v22.0.0", "bin", "node"), "node-binary")

	if _, err := MigrateNvm(home); err != nil {
		t.Fatalf("first MigrateNvm: %v", err)
	}
	second, err := MigrateNvm(home)
	if err != nil {
		t.Fatalf("second MigrateNvm: %v", err)
	}

	if second.Merged() {
		t.Errorf("second run moved %v, want nothing", second.Moved)
	}
	if len(second.Conflicts) != 0 {
		t.Errorf("second run reported conflicts: %+v", second.Conflicts)
	}
	if got := read(t, filepath.Join(NvmDataDir(home), "versions", "node", "v22.0.0", "bin", "node")); got != "node-binary" {
		t.Errorf("data after second run = %q, want %q", got, "node-binary")
	}
}

func TestMigrateNvmSkipsWhenSourceIsTheDestination(t *testing.T) {
	// Once migrated, the data root itself holds versions/ and alias/. Nothing should
	// try to merge that directory into itself.
	home := t.TempDir()
	data := NvmDataDir(home)
	write(t, filepath.Join(data, "versions", "node", "v22.0.0", "bin", "node"), "node-binary")

	result, err := MigrateNvm(home)
	if err != nil {
		t.Fatalf("MigrateNvm: %v", err)
	}
	if result.Merged() || len(result.Conflicts) != 0 {
		t.Errorf("expected an empty result, got %+v", result)
	}
	if got := read(t, filepath.Join(data, "versions", "node", "v22.0.0", "bin", "node")); got != "node-binary" {
		t.Errorf("data root was disturbed: %q", got)
	}
}
