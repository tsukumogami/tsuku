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

	fmt.Printf("Installing %s@%s via gem install\n", gemName, version)

	// Find gem command
	gemPath, err := exec.LookPath("gem")
	if err != nil {
		// Try tsuku's ruby
		rubyBin := filepath.Join(ctx.ToolsDir, "ruby-*", "bin", "gem")
		matches, _ := filepath.Glob(rubyBin)
		if len(matches) > 0 {
			gemPath = matches[0]
		} else {
			return fmt.Errorf("gem command not found")
		}
	}

	// Install gem to tsuku's directory
	installDir := ctx.InstallDir
	gemHome := filepath.Join(installDir, ".gem")
	if err := os.MkdirAll(gemHome, 0755); err != nil {
		return fmt.Errorf("failed to create gem home: %w", err)
	}

	// Run gem install
	cmd := exec.CommandContext(ctx.Context, gemPath, "install", gemName, "--version", version, "--install-dir", gemHome)
	cmd.Env = append(os.Environ(),
		"GEM_HOME="+gemHome,
		"GEM_PATH="+gemHome,
	)

	fmt.Printf("Running: gem install %s --version %s --install-dir %s\n", gemName, version, gemHome)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem install failed: %w\nOutput: %s", err, string(output))
	}

	// Find where gem installed the executables
	binDir := filepath.Join(gemHome, "bin")
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		return fmt.Errorf("gem bin directory not found at %s", binDir)
	}

	// Create symlinks for executables
	tsukuBinDir := filepath.Join(filepath.Dir(ctx.InstallDir), "..", "bin")
	if err := os.MkdirAll(tsukuBinDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	for _, exe := range executables {
		source := filepath.Join(binDir, exe)
		target := filepath.Join(tsukuBinDir, exe)

		// Check if executable exists
		if _, err := os.Stat(source); os.IsNotExist(err) {
			return fmt.Errorf("expected executable %s not found at %s", exe, source)
		}

		// Remove existing symlink if present
		os.Remove(target)

		// Create symlink
		if err := os.Symlink(source, target); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", exe, err)
		}
	}

	fmt.Printf("✓ Installed %s@%s with %d executable(s)\n", gemName, version, len(executables))
	return nil
}

func init() {
	Register(&InstallGemDirectAction{})
}
