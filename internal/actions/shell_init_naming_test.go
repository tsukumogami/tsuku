package actions

import (
	"sort"
	"strings"
	"testing"
)

// The mapping (target, version, shell) -> filename has to be injective, or one
// tool's install overwrites another tool's shell fragment -- and everything in
// share/shell.d is sourced by the user's shell on every new session.
func TestShellDFileName_Injective(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		version string
		shell   string
		want    string
	}{
		{"plain", "nvm", "0.40.6", "bash", "nvm@0.40.6.bash"},
		{"exports target", "00-env-nvm", "0.40.6", "zsh", "00-env-nvm@0.40.6.zsh"},
		{"hyphen in the target", "foo-1", "2", "bash", "foo-1@2.bash"},
		{"hyphen in the version", "foo", "1-2", "bash", "foo@1-2.bash"},
		{"dot in the target", "foo.bar", "1.0", "fish", "foo.bar@1.0.fish"},
		{"underscore leading target", "_foo", "1.0", "bash", "_foo@1.0.bash"},
	}

	seen := make(map[string]string)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellDFileName(tt.target, tt.version, tt.shell)
			if got != tt.want {
				t.Errorf("shellDFileName(%q, %q, %q) = %q, want %q",
					tt.target, tt.version, tt.shell, got, tt.want)
			}
			if want := "share/shell.d/" + tt.want; shellDCleanupPath(tt.target, tt.version, tt.shell) != want {
				t.Errorf("recorded path and written filename disagree: %q vs %q",
					shellDCleanupPath(tt.target, tt.version, tt.shell), want)
			}
		})

		key := tt.target + "\x00" + tt.version + "\x00" + tt.shell
		name := shellDFileName(tt.target, tt.version, tt.shell)
		if prior, collides := seen[name]; collides && prior != key {
			t.Errorf("filename %q is produced by two distinct inputs: %q and %q", name, prior, key)
		}
		seen[name] = key
	}

	// The awkward pair the "-" separator could not tell apart.
	if a, b := shellDFileName("foo-1", "2", "bash"), shellDFileName("foo", "1-2", "bash"); a == b {
		t.Errorf("tool foo-1 at version 2 and tool foo at version 1-2 collide on %q", a)
	}
}

// The shell cache concatenates share/shell.d alphabetically, and a tool's init
// script often reads variables its recipe exported. The target charset rule is
// what makes "exports first" hold without case analysis: init filenames start
// at 0x41 or above, export filenames start with "0" (0x30).
func TestShellDFileName_ExportsSortBeforeInitScripts(t *testing.T) {
	initTargets := []string{"Ansible", "_private", "aws", "nvm", "zoxide", "a.b-c"}
	envNames := []string{"aws", "nvm", "zoxide", "Ansible"}
	versions := []string{"0.0.1", "9.9.9", "2026.1"}

	var names []string
	exports := make(map[string]bool)
	for _, v := range versions {
		for _, target := range initTargets {
			if err := validateShellDTarget(target); err != nil {
				t.Fatalf("test uses an illegal target %q: %v", target, err)
			}
			names = append(names, shellDFileName(target, v, "bash"))
		}
		for _, n := range envNames {
			name := shellDFileName(EnvFilePrefix+n, v, "bash")
			exports[name] = true
			names = append(names, name)
		}
	}

	sort.Strings(names)

	seenInit := false
	for _, name := range names {
		if exports[name] {
			if seenInit {
				t.Fatalf("export fragment %q sorts after an init fragment; sorted order was %v", name, names)
			}
			continue
		}
		seenInit = true
	}
}

func TestValidateShellDTarget(t *testing.T) {
	tests := []struct {
		target string
		ok     bool
	}{
		{"nvm", true},
		{"Ansible", true},
		{"_private", true},
		{"a.b-c_9", true},
		{"", false},
		{"00-env-nvm", false}, // the reserved exports prefix, rejected by the leading-digit rule
		{"9lives", false},
		{"has space", false},
		{"has@at", false},
		{"has/slash", false},
		{"..", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			err := validateShellDTarget(tt.target)
			if tt.ok && err != nil {
				t.Errorf("validateShellDTarget(%q) = %v, want nil", tt.target, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("validateShellDTarget(%q) = nil, want an error", tt.target)
			}
		})
	}
}

// Preflight is what runs in recipe-validation CI, so the charset rule has to be
// enforced there and not only at execution time.
func TestInstallShellInitAction_PreflightRejectsIllegalTarget(t *testing.T) {
	a := &InstallShellInitAction{}

	for _, target := range []string{EnvFilePrefix + "nvm", "9lives", "has space", "has@at"} {
		t.Run(target, func(t *testing.T) {
			result := a.Preflight(map[string]interface{}{
				"source_file": "init.sh",
				"target":      target,
			})
			if !result.HasErrors() {
				t.Errorf("Preflight() accepted the illegal target %q", target)
			}
			if !strings.Contains(strings.Join(result.Errors, " "), "must match") {
				t.Errorf("expected a charset error for %q, got %v", target, result.Errors)
			}
		})
	}
}

// An empty version would collapse every version onto one filename, which is the
// bug the version key exists to remove.
func TestShellDVersion_RejectsEmpty(t *testing.T) {
	if _, err := shellDVersion(&ExecutionContext{}); err == nil {
		t.Error("shellDVersion() accepted an empty version")
	}
	got, err := shellDVersion(&ExecutionContext{Version: "1.0.0"})
	if err != nil || got != "1.0.0" {
		t.Errorf("shellDVersion() = %q, %v; want 1.0.0, nil", got, err)
	}
}
