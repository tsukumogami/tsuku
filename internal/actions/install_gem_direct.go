package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// InstallGemDirectAction installs a gem using `gem install` directly.
// This is used for bundler self-installation where bundle install cannot be used.
type InstallGemDirectAction struct{}

func (InstallGemDirectAction) Name() string {
	return "install_gem_direct"
}

func (InstallGemDirectAction) IsDeterministic() bool {
	return false // Network-dependent, non-reproducible
}

func (InstallGemDirectAction) RequiresNetwork() bool {
	return true
}

func (InstallGemDirectAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime: []string{"ruby"},
		Runtime:     []string{"ruby"},
	}
}

func (a *InstallGemDirectAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get required parameters
	gemName, ok := GetString(params, "gem")
	if !ok {
		return fmt.Errorf("install_gem_direct requires 'gem' parameter")
	}

	version, ok := GetString(params, "version")
	if !ok {
		return fmt.Errorf("install_gem_direct requires 'version' parameter")
	}

	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("install_gem_direct requires 'executables' parameter with at least one executable")
	}

	fmt.Printf("   Installing %s@%s via gem install\n", gemName, version)

	// Find gem command (prefer tsuku's ruby, fall back to system)
	gemPath := ResolveGem()
	if gemPath == "" {
		var err error
		gemPath, err = exec.LookPath("gem")
		if err != nil {
			return fmt.Errorf("gem command not found")
		}
	}

	// Install gem directly to install directory with bin/ at install root.
	// This matches the convention used by gem_install and gem_exec so that
	// ExtractBinaries() can uniformly return "bin/<name>" for all gem actions.
	installDir := ctx.InstallDir
	binDir := filepath.Join(installDir, "bin")

	cmd := exec.CommandContext(ctx.Context, gemPath, "install", gemName,
		"--version", version,
		"--no-document",
		"--install-dir", installDir,
		"--bindir", binDir,
	)
	cmd.Env = append(os.Environ(),
		"GEM_HOME="+installDir,
		"GEM_PATH="+installDir,
	)

	fmt.Printf("   Running: gem install %s --version %s --install-dir %s\n", gemName, version, installDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem install failed: %w\nOutput: %s", err, string(output))
	}

	// Verify executables exist and create self-contained wrapper scripts.
	// Wrappers set GEM_HOME/GEM_PATH so the gem runs in isolation.
	gemDir := filepath.Dir(gemPath)
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}

		if err := createGemWrapper(exePath, binDir, exe, gemDir, "."); err != nil {
			return fmt.Errorf("failed to create wrapper for %s: %w", exe, err)
		}
	}

	fmt.Printf("   Installed %s@%s with %d executable(s)\n", gemName, version, len(executables))
	return nil
}

func init() {
	Register(&InstallGemDirectAction{})
}
