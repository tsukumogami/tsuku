package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
)

// A dependency the executor installs directly gets a hidden state entry, which
// makes this the first production code to set IsHidden. Hidden is a state a
// tool leaves through ExposeHidden, and ExposeHidden links exactly the binaries
// the entry records -- falling back to guessing bin/<toolname> when it records
// none. AtomicSymlink does not check that its target exists, so an entry with
// no binaries would turn `tsuku install <dep>` into a dangling symlink and an
// install that silently did nothing.
func exposeHome(t *testing.T) (*config.Config, *Manager) {
	t.Helper()

	home := t.TempDir()
	cfg := &config.Config{
		HomeDir:               home,
		ToolsDir:              filepath.Join(home, "tools"),
		CurrentDir:            filepath.Join(home, "tools", "current"),
		RecipesDir:            filepath.Join(home, "recipes"),
		RegistryDir:           filepath.Join(home, "registry"),
		LibsDir:               filepath.Join(home, "libs"),
		AppsDir:               filepath.Join(home, "apps"),
		CacheDir:              filepath.Join(home, "cache"),
		VersionCacheDir:       filepath.Join(home, "cache", "versions"),
		DownloadCacheDir:      filepath.Join(home, "cache", "downloads"),
		CargoRegistryCacheDir: filepath.Join(home, "cache", "cargo-registry"),
		KeyCacheDir:           filepath.Join(home, "cache", "keys"),
		TapCacheDir:           filepath.Join(home, "cache", "taps"),
		ShareDir:              filepath.Join(home, "share"),
		ConfigFile:            filepath.Join(home, "config.toml"),
	}
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}
	return cfg, New(cfg)
}

func TestCheckAndExposeHidden_DependencyEntry(t *testing.T) {
	tests := []struct {
		name string
		// binaryPath is where the dependency actually puts its executable,
		// relative to the tool directory. "zig" ships its binary at the tool
		// root rather than under bin/, which is what makes the guessed
		// bin/<toolname> fallback wrong rather than merely redundant.
		binaryPath  string
		recorded    []string
		wantExposed bool
	}{
		{
			name:        "binaries recorded, so exposing links the right file",
			binaryPath:  "mydep",
			recorded:    []string{"mydep"},
			wantExposed: true,
		},
		{
			name:        "no binaries recorded, so exposing is declined",
			binaryPath:  "mydep",
			recorded:    nil,
			wantExposed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, mgr := exposeHome(t)

			depDir := filepath.Join(cfg.ToolsDir, "mydep-1.0.0")
			if err := os.MkdirAll(depDir, 0755); err != nil {
				t.Fatalf("creating dep dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(depDir, tt.binaryPath), []byte("#!/bin/sh\n"), 0755); err != nil {
				t.Fatalf("writing dep binary: %v", err)
			}
			if err := mgr.EnsureDependencyEntry("mydep", "1.0.0", "parent", tt.recorded); err != nil {
				t.Fatalf("EnsureDependencyEntry() error = %v", err)
			}

			exposed, err := CheckAndExposeHidden(context.Background(), mgr, "mydep")
			if err != nil {
				t.Fatalf("CheckAndExposeHidden() error = %v", err)
			}
			if exposed != tt.wantExposed {
				t.Fatalf("CheckAndExposeHidden() = %v, want %v", exposed, tt.wantExposed)
			}

			link := filepath.Join(cfg.CurrentDir, "mydep")
			if !tt.wantExposed {
				// Declining has to leave nothing behind. A symlink here with
				// exposed=false is the worst outcome: the caller goes on to
				// install properly while a broken link sits on PATH.
				if _, err := os.Lstat(link); !os.IsNotExist(err) {
					target, _ := os.Readlink(link)
					t.Errorf("declining to expose still created %s -> %s", link, target)
				}
				return
			}
			if _, err := os.Stat(link); err != nil {
				target, _ := os.Readlink(link)
				t.Errorf("%s does not resolve (points at %s): %v", link, target, err)
			}
		})
	}
}

// A visible install is the user asking for the tool by name. Leaving IsHidden
// set would keep it out of `tsuku list` and out of update checks for a tool
// they just installed themselves.
func TestInstallWithOptions_ClearsHiddenOnVisibleInstall(t *testing.T) {
	_, mgr := exposeHome(t)

	if err := mgr.EnsureDependencyEntry("mydep", "1.0.0", "parent", nil); err != nil {
		t.Fatalf("EnsureDependencyEntry() error = %v", err)
	}
	before, err := mgr.GetToolState("mydep")
	if err != nil || before == nil || !before.IsHidden {
		t.Fatalf("setup: expected a hidden entry, got %+v (err %v)", before, err)
	}

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".install", "bin"), 0755); err != nil {
		t.Fatalf("creating staging dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".install", "bin", "mydep"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("writing staged binary: %v", err)
	}

	opts := DefaultInstallOptions()
	opts.Binaries = []string{filepath.Join("bin", "mydep")}
	if err := mgr.InstallWithOptions(context.Background(), "mydep", "2.0.0", workDir, opts); err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	after, err := mgr.GetToolState("mydep")
	if err != nil || after == nil {
		t.Fatalf("GetToolState() = %v, %v", after, err)
	}
	if after.IsHidden {
		t.Error("the tool is still hidden after the user installed it by name")
	}
}
